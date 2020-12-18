// These are things that don't strictly belong in the mgm package and should
// eventually move. Particularly, mgm should not have to import "config".

package mgm

import (
	"fmt"
	"github.com/mysql/ndb-operator/pkg/ndb/config"
	"strconv"
	"strings"
)

// GetConfigVersion(): Fetch current config generation number.
// This is implemented using ShowConfig() for efficiency.
func (api *Client) GetConfigVersion() (int, error) {
	var gen int
	text, err := api.ShowConfig("SYSTEM", "ConfigGenerationNumber", 0)
	if err != nil {
		return gen, err
	}
	lines := strings.Split(text, "\n")
	parts := strings.SplitN(lines[1], "=", 2)
	return strconv.Atoi(parts[1])
}

// GetConfigVersionFromNode(): Asks mgm server for config from a data node;
// return that configuration's generation number.
func (api *Client) GetConfigVersionFromNode(nodeId int) (int, error) {
	base64str, err := api.GetConfig(nodeId, false)
	if err == nil {
		var conf *config.Configuration
		conf, err = config.NewConfigurationFromBase64(base64str)
		if err == nil {
			return conf.SystemSection().GenerationNumber(), err
		}
	}
	return -1, err
}

// Connect to an mgmd (presumably through a load-balancer), then check its
// node id. Retry until a connection to wantedNodeId is obtained.
// This may log a transient diagnostic error without returning it.
func (api *Client) ConnectToNodeId(connectstring string, wantedNodeId int) error {
	var connectError error
	var logged bool = false
	var connected bool = false

	for retries := 0; retries < 10; retries++ {

		connectError = nil
		err := api.Connect(connectstring)
		if err != nil {
			if !logged {
				fmt.Printf("Connect error: %s", err)
				logged = true
			}
			connectError = err
			continue
		}

		nodeid, err := api.GetMgmNodeId()
		if err != nil {
			api.Disconnect()
			connectError = err
			continue
		}

		if nodeid == wantedNodeId {
			connected = true
			break
		}
		api.Disconnect()
	}

	if !connected {
		if connectError != nil {
			return connectError
		}
		return fmt.Errorf("Failed to connect to management node %d", wantedNodeId)
	}

	return nil
}
