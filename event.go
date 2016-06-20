package bot

import (
	"errors"
	"fmt"
)

// Event contains a message from IRCCloud as an interface map
// For a list of events see https://github.com/irccloud/irccloud-tools/wiki/API-Stream-Message-Reference
type Event map[string]interface{}

// Conn returns the IRCCloudBot object the message was received with
func (e Event) Conn() *IRCCloudBot {
	return e["_conn"].(*IRCCloudBot)
}

// FromBacklog indicates whether the message was fetched from the backlog or from the stream
func (e Event) FromBacklog() bool {
	return e["_from_backlog"].(bool)
}

// Reply is a shorthand to Event.Conn().Say(...) to send to the same channel the message was received from
func (e Event) Reply(message string) error {
	if e["type"] != "buffer_msg" {
		return fmt.Errorf("Cannot reply to type '%s'", e["type"].(string))
	}
	return e.Conn().Say(int(e["cid"].(float64)), e["chan"].(string), message)
}

// IsSelf returns whether the message was sent by ourselves
func (e Event) IsSelf() bool {
	v, err := e.Bool("self")
	return v && err == nil
}

// From returns the sender of the message
func (e Event) From() string {
	v, _ := e.Str("from")
	return v
}

// Chan returns the channel the message was sent to
func (e Event) Chan() string {
	v, _ := e.Str("chan")
	return v
}

// Returns the ConnectionID the message was sent through
func (e Event) ConnectionID() int {
	return int(e["cid"].(float64))
}

// Str returns the attribute as a string
func (e Event) Str(attributeName string) (string, error) {
	v, ok := e[attributeName].(string)
	if !ok {
		return "", errors.New("Field was not of expected type")
	}
	return v, nil
}

// Bool returns the attribute as a boolean
func (e Event) Bool(attributeName string) (bool, error) {
	v, ok := e[attributeName].(bool)
	if !ok {
		return false, errors.New("Field was not of expected type")
	}
	return v, nil
}
