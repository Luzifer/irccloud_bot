package bot

import (
	"encoding/json"
	"fmt"

	"golang.org/x/net/context/ctxhttp"
)

func init() {
	internalMessageHandlers["oob_include"] = handleOOBInclude
	internalMessageHandlers["idle"] = handleIdle
}

func handleOOBInclude(evt Event) error {
	i := evt["_conn"].(*IRCCloudBot)
	res, err := ctxhttp.Do(i.idleContext, i.HTTPClient, i.getAuthenticatedRequest("GET", evt["url"].(string), nil))
	if err != nil {
		return fmt.Errorf("Could not fetch oob_include: %s", err)
	}
	defer res.Body.Close()

	r := []Event{}
	if err := json.NewDecoder(res.Body).Decode(&r); err != nil {
		return err
	}

	for _, e := range r {
		e["_from_backlog"] = true
		i.eventChan <- e
	}

	return nil
}

func handleIdle(evt Event) error { return nil }
