package bot

import (
	"bufio"
	"bytes"
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

	lr := bufio.NewScanner(res.Body)
	for lr.Scan() {
		if lr.Text() == "[" || lr.Text() == "]" {
			continue
		}
		i.handleEvent(bytes.Trim(lr.Bytes(), ","), true)
	}

	return nil
}

func handleIdle(evt Event) error { return nil }
