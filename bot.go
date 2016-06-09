package bot

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"golang.org/x/net/context"
	"golang.org/x/net/context/ctxhttp"
)

const (
	idleTimeout = 30 * time.Second
)

var (
	internalMessageHandlers = make(map[string]EventHandler)
)

// EventHandler is a type of function which can be registered as a handler for specific types of events
type EventHandler func(Event) error

// IRCCloudBot represents a single bot instance (may use different accounts / networks if available in that IRCCloud account)
type IRCCloudBot struct {
	HTTPClient          *http.Client
	AutoReconnect       bool
	YieldInternalEvents bool
	DropUnhandledEvents bool

	email, password string

	eventHandlers      map[string][]EventHandler
	errChan            chan error
	eventChan          chan Event
	parentContext      context.Context
	idleContext        context.Context
	idleContextCancel  context.CancelFunc
	idleContextTimeout *time.Timer
	session            string
}

// New creates an IRCCloudBot instance with background context and tries to log into IRCCloud with it
func New(email, password string) (*IRCCloudBot, error) {
	return WithContext(context.Background(), email, password)
}

// WithContext creates an IRCCloudBot instance with explicit set context and tries to log into IRCCloud with it
func WithContext(ctx context.Context, email, password string) (*IRCCloudBot, error) {
	i := &IRCCloudBot{
		HTTPClient:          &http.Client{},
		AutoReconnect:       true,
		YieldInternalEvents: false,
		DropUnhandledEvents: false,

		email:         email,
		password:      password,
		parentContext: ctx,

		eventHandlers: make(map[string][]EventHandler),
		errChan:       make(chan error),
		eventChan:     make(chan Event, 100),
	}

	return i, i.login()
}

// Events returns a read-only channel of Event objects (buffered for 100 events)
func (i *IRCCloudBot) Events() <-chan Event {
	return i.eventChan
}

// Err blocks until an error occurred and returns it afterwards
func (i *IRCCloudBot) Err() error {
	return <-i.errChan
}

// Start starts the stream listening
func (i *IRCCloudBot) Start() {
	i.idleContext, i.idleContextCancel = context.WithCancel(i.parentContext)

	i.idleContextTimeout = time.AfterFunc(idleTimeout, i.idleContextCancel)

	go func() {
		err := i.listenAndParseEvents()

		if i.parentContext.Err() == nil && i.AutoReconnect {
			i.Start()
		} else {
			i.errChan <- err
		}
	}()
}

// Join joins a channel on the specified connection ID
func (i *IRCCloudBot) Join(connectionID int, channel string) error {
	return i.authenticatedPost("/chat/join", url.Values{
		"cid":     []string{strconv.Itoa(connectionID)},
		"channel": []string{channel},
	})
}

// Part leaves a channel on the specified connection ID
func (i *IRCCloudBot) Part(connectionID int, channel string) error {
	return i.authenticatedPost("/chat/part", url.Values{
		"cid":     []string{strconv.Itoa(connectionID)},
		"channel": []string{channel},
	})
}

// Topic sets the topic of a  channel on the specified connection ID
func (i *IRCCloudBot) Topic(connectionID int, channel, topic string) error {
	return i.authenticatedPost("/chat/topic", url.Values{
		"cid":     []string{strconv.Itoa(connectionID)},
		"channel": []string{channel},
		"topic":   []string{topic},
	})
}

// Say posts a message to target on the specified connection ID
// Example: mybot.Say(2, "#mychannel", "ohai!")
func (i *IRCCloudBot) Say(connectionID int, target, message string) error {
	return i.authenticatedPost("/chat/say", url.Values{
		"cid": []string{strconv.Itoa(connectionID)},
		"to":  []string{target},
		"msg": []string{message},
	})
}

// Nick changes the own nickname on the specified connection ID
func (i *IRCCloudBot) Nick(connectionID int, nick string) error {
	return i.authenticatedPost("/chat/nick", url.Values{
		"cid":  []string{strconv.Itoa(connectionID)},
		"nick": []string{nick},
	})
}

// RegisterMessageHandler registers a new handler for eventType events
// Please note:
//   - Events handled with these handlers are not anymore sent to Events() stream
//   - You need to fetch events from Events() stream or set DropUnhandledEvents or the bot will freeze after 100 messages
func (i *IRCCloudBot) RegisterMessageHandler(eventType string, eh EventHandler) {
	if _, ok := i.eventHandlers[eventType]; !ok {
		i.eventHandlers[eventType] = []EventHandler{eh}
	} else {
		i.eventHandlers[eventType] = append(i.eventHandlers[eventType], eh)
	}
}

func (i *IRCCloudBot) authenticatedPost(path string, values url.Values) error {
	values.Set("session", i.session)

	req := i.getAuthenticatedRequest("POST", path, bytes.NewBufferString(values.Encode()))
	req.Header.Set("content-type", "application/x-www-form-urlencoded")
	res, err := ctxhttp.Do(i.parentContext, i.HTTPClient, req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	r := map[string]interface{}{}
	if err := json.NewDecoder(res.Body).Decode(&r); err != nil {
		return err
	}

	if !r["success"].(bool) {
		return fmt.Errorf("API call was not successful: %#v", r)
	}

	return nil
}

func (i *IRCCloudBot) listenAndParseEvents() error {
	req := i.getAuthenticatedRequest("GET", "/chat/stream", nil)
	res, err := ctxhttp.Do(i.idleContext, i.HTTPClient, req)
	if err != nil {
		return fmt.Errorf("Unable to request stream: %s", err)
	}
	defer res.Body.Close()

	lr := bufio.NewScanner(res.Body)
	for lr.Scan() {
		e := Event{
			"_conn": i,
		}
		if err := json.Unmarshal(lr.Bytes(), &e); err != nil {
			log.Printf("Got unparsable message: %s", lr.Text())
			continue
		}

		i.idleContextTimeout.Reset(idleTimeout)

		ih, internallyHandled := internalMessageHandlers[e["type"].(string)]
		if internallyHandled {
			if err := ih(e); err != nil {
				return err
			}
		}

		if !internallyHandled || i.YieldInternalEvents {
			if ehs, ok := i.eventHandlers[e["type"].(string)]; ok && len(ehs) > 0 {
				for _, eh := range ehs {
					if err := eh(e); err != nil {
						return err
					}
				}
			} else {
				if !i.DropUnhandledEvents {
					i.eventChan <- e
				}
			}
		}
	}
	if err := lr.Err(); err != nil {
		return fmt.Errorf("Encountered error while reading the stream: %s", err)
	}

	return nil
}

func (i *IRCCloudBot) getAuthenticatedRequest(method, urlPath string, body io.Reader) *http.Request {
	if i.session == "" {
		log.Fatalf("Login did not work, session is empty!")
	}

	req, _ := http.NewRequest(method, "https://www.irccloud.com"+urlPath, body)
	req.Header.Set("cookie", "session="+i.session)

	return req
}

func (i *IRCCloudBot) getAuthToken() (string, error) {
	authTokenRes, err := ctxhttp.Post(i.parentContext, i.HTTPClient, "https://www.irccloud.com/chat/auth-formtoken", "application/x-www-form-urlencoded", nil)
	if err != nil {
		return "", err
	}
	defer authTokenRes.Body.Close()

	at := map[string]interface{}{}
	if err := json.NewDecoder(authTokenRes.Body).Decode(&at); err != nil {
		return "", err
	}

	if !at["success"].(bool) || at["token"].(string) == "" {
		return "", errors.New("No auth-formtoken received")
	}

	return at["token"].(string), nil
}

func (i *IRCCloudBot) login() error {
	token, err := i.getAuthToken()
	if err != nil {
		return err
	}

	params := url.Values{
		"email":    []string{i.email},
		"password": []string{i.password},
		"token":    []string{token},
	}
	req, _ := http.NewRequest("POST", "https://www.irccloud.com/chat/login", bytes.NewBuffer([]byte(params.Encode())))
	req.Header.Set("content-type", "application/x-www-form-urlencoded")
	req.Header.Set("x-auth-formtoken", token)

	loginRes, err := ctxhttp.Do(i.parentContext, i.HTTPClient, req)
	if err != nil {
		return err
	}
	defer loginRes.Body.Close()

	ld := map[string]interface{}{}
	if err := json.NewDecoder(loginRes.Body).Decode(&ld); err != nil {
		return err
	}

	if !ld["success"].(bool) {
		return fmt.Errorf("Login was not successful: %#v", ld)
	}

	i.session = ld["session"].(string)
	return nil
}
