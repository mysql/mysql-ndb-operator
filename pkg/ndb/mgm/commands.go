// Package mgm manages a Client connection to an NDB management server.

package mgm

import "fmt"

// GetVersion obtains version information from the connected mgm server.
// Returns a versionId, which may be used in subsequent requests.
// The version array, if non-nil, will be filled in with the three parts
// of the version number (major, minor, build).
func (c *Client) GetVersion(version *[3]int) (int, error) {
	req := newRequest("get version")
	res := newResponse("version")
	var verId int

	res.bindInt("id", &verId)
	if version == nil {
		res.ignore("major")
		res.ignore("minor")
		res.ignore("build")
	} else {
		res.bindInt("major", &(version[0]))
		res.bindInt("minor", &(version[1]))
		res.bindInt("build", &(version[2]))
	}
	res.ignore("string")
	res.ignore("mysql_major")
	res.ignore("mysql_minor")
	res.ignore("mysql_build")

	err := c.exec(req, res)
	return verId, err
}

// Send a NodeId allocation request to the management server.
// Requires verId representing client version.
// Returns the assigned node id and any error.
func (c *Client) AllocNodeId(verId int, reqNodeId int) (int, error) {
	req := newRequest("get nodeid")
	res := newResponse("get nodeid reply")

	req.bindInt("version", verId)
	req.bindInt("nodetype", 1) // API
	req.bindInt("nodeid", reqNodeId)
	req.bindStr("user", "mysqld")
	req.bindStr("password", "mysqld")
	req.bindStr("public key", "a public key")
	req.bindStr("endian", "little")
	req.bindStr("name", "gogogo")
	req.bindInt("log_event", 1)

	var error_code, assigned_id int
	res.bindResultHeader()
	res.bindInt("error_code", &error_code)
	res.bindInt("nodeid", &assigned_id)

	err := c.exec(req, res)
	return assigned_id, err
}

// StopNodes. NodeIds points to an array of node ids to be stopped.
// Returns true if this client should disconnect to allow the mgmd to
// shut down.
func (c *Client) StopNodes(nodeIds *[]int) (bool, error) {
	var nodeList, sep string
	var disconnect int

	for node := 0; node < len(*nodeIds); node++ {
		nodeList += fmt.Sprintf("%s%d", sep, (*nodeIds)[node])
		sep = " "
	}

	req := newRequest("stop v2")
	res := newResponse("stop reply")

	req.bindStr("node", nodeList)
	req.bindInt("abort", 0)
	req.bindInt("force", 0)

	res.bindResultHeader()
	res.bindInt("disconnect", &disconnect)

	err := c.exec(req, res)
	return (disconnect == 1), err
}

// GetMgmNodeId. Returns the node id of the connected mgm server.
func (c *Client) GetMgmNodeId() (int, error) {
	req := newRequest("get mgmd nodeid")
	res := newResponse("get mgmd nodeid reply")

	var nodeId int
	res.bindInt("nodeid", &nodeId)

	err := c.exec(req, res)

	return nodeId, err
}

// GetConfig() fetches the cluster configuration from mgmd.
// If fromNodeId iszero, mgmd returns its own configuration object;
// if non-zero, it obtains a configuration from that node and returns it.
// If withTcpLinks is true, the returned configuration will include all
// node-to-node network link sections.
// Returns a string holding the base64-encoded configuration.
func (c *Client) GetConfig(fromNodeId int, withTcpLinks bool) (string, error) {
	req := newRequest("get config_v2")
	res := newResponse("get config reply")

	// version:  required, but ignored by the server
	// nodetype: if MGM, ConfigManager may return a non-confirmed config;
	//           otherwise only confirmed configuration will be sent.
	// node:     if 0, the config will contain TCP sections for *all* node-to-node
	//           links; if non-zero, only for links involving that node.
	var linksForNode int
	if withTcpLinks {
		linksForNode = 0
	} else {
		linksForNode = 1 // any node number will do
	}
	req.bindInt("version", 1) // This is
	req.bindStr("nodetype", "-1")
	req.bindInt("node", linksForNode)
	if fromNodeId > 0 {
		req.bindInt("from_node", fromNodeId)
	}

	var contentLength int
	res.bindResultHeader() // Require "Result: Ok"
	res.ignore("Content-Type")
	res.ignore("Content-Transfer-Encoding")
	res.bindInt("Content-Length", &contentLength)
	res.ExpectData = true

	err := c.exec(req, res)
	if len(res.Data) != contentLength {
		err = fmt.Errorf("Base64 data length %d invalid (expected %d)",
			len(res.Data), contentLength)
	}

	return res.Data, err
}

// GetStatus() asks mgmd for the status of all nodes.
// It places all response headers into the supplied status map.
// Returns nodeCount and any network error
func (c *Client) GetStatus(status *map[string]string) (int, error) {
	req := newRequest("get status")
	res := newResponse("node status")

	var nodeCount int
	res.bindInt("nodes", &nodeCount)
	res.bindMap(status)

	err := c.exec(req, res)
	return nodeCount, err
}

// ShowConfig() asks mgmd for a particular value from its configuration.
// Takes a section name & parameter name (either may be empty),
// and node id (which may be zero).  These are used as filters.
// Returns all of the text configuration data in the server's reply.
func (c *Client) ShowConfig(section string, param string, node int) (string, error) {
	req := newRequest("show config")
	res := newResponse("show config reply")

	if len(section) > 0 {
		req.bindStr("Section", section)
	}
	if len(param) > 0 {
		req.bindStr("Name", param)
	}
	if node > 0 {
		req.bindInt("NodeId", node)
	}

	// There are no headers; only bulk response data
	res.ExpectData = true

	err := c.exec(req, res)
	return res.Data, err
}
