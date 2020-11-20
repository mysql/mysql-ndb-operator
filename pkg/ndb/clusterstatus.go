package ndb

import (
	"errors"
	"fmt"
)

const InvalidSectionTypeId = 0
const DataNodeTypeId = 1
const ApiNodeTypeId = 2
const MgmNodeTypeId = 3
const TcpTypeId = 4
const ShmTypeId = 5
const SystemSectionId = 6

type NodeStatus struct {
	NodeType        int
	NodeId          int
	IsConnected     bool
	NodeGroup       int
	SoftwareVersion string
}

type ClusterStatus map[int]*NodeStatus

func (ns *NodeStatus) isDataNode() bool {
	return ns.NodeType == DataNodeTypeId
}

func (ns *NodeStatus) isMgmNode() bool {
	return ns.NodeType == MgmNodeTypeId
}

func (ns *NodeStatus) isApiNode() bool {
	return ns.NodeType == ApiNodeTypeId
}

func NewClusterStatus(nodeCount int) *ClusterStatus {
	cs := make(ClusterStatus, nodeCount)
	return &cs
}

/*
	returns true if any data node or mgm node is down
*/
func (cs *ClusterStatus) IsClusterDegraded() bool {

	isDegraded := false
	for _, ns := range *cs {
		if ns.isDataNode() || ns.isMgmNode() {
			isDegraded = isDegraded || !ns.IsConnected
		}
	}

	return isDegraded
}

// ensures there is a node entry for the given nodeId
// if there is no node status for this node id yet then create
func (cs *ClusterStatus) EnsureNode(nodeId int) *NodeStatus {

	var nodeStatus *NodeStatus
	var ok bool
	if nodeStatus, ok = (*cs)[nodeId]; !ok {
		nodeStatus = &NodeStatus{
			NodeId: nodeId,
		}
		(*cs)[nodeId] = nodeStatus
	}

	return nodeStatus
}

func (cs *ClusterStatus) SetNodeTypeFromTLA(nodeId int, tla string) error {

	ns := cs.EnsureNode(nodeId)

	if ns == nil {
		return errors.New(fmt.Sprintf("Invalid node id %d", nodeId))
	}

	// node type NDB, MGM, API
	nodeType := InvalidTypeId
	switch tla {
	case "NDB":
		nodeType = DataNodeTypeId
		break
	case "MGM":
		nodeType = MgmNodeTypeId
		break
	case "API":
		nodeType = ApiNodeTypeId
		break
	default:
	}

	if nodeType == InvalidTypeId {
		return errors.New(fmt.Sprintf("Invalid node type %s", tla))
	}
	ns.NodeType = nodeType

	return nil
}
