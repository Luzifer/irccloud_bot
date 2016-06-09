package bot

import (
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
	res.Body.Close()
	return nil
}

func handleIdle(evt Event) error { return nil }
