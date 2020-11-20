package ndb

import (
	"fmt"
)

// node types used in NodeStatus object
// there are more types defined in c-code, but they are not used here

// InvalidSectionTypeID describes an unkown or illegal node type
const InvalidSectionTypeID = 0

// DataNodeTypeID describes a data node type
const DataNodeTypeID = 1

// APINodeTypeID describes a mysql server or general API node
const APINodeTypeID = 2

// MgmNodeTypeID is the management server node type
const MgmNodeTypeID = 3

// NodeStatus describes the state of a node of any node type in cluster
type NodeStatus struct {
	// Type of node: data node, API or management server
	NodeType int

	// node id of the node described in status object
	NodeID int

	// isConnected reports if the node is fully started and connected to cluster
	IsConnected bool

	// NodeGroup reports which node group the node is in, -1 if unclear or wrong node type
	NodeGroup int

	// mysql version number as string in format x.y.z
	SoftwareVersion string
}

// ClusterStatus describes the state of all nodes in the cluster
type ClusterStatus map[int]*NodeStatus

func (ns *NodeStatus) isDataNode() bool {
	return ns.NodeType == DataNodeTypeID
}

func (ns *NodeStatus) isMgmNode() bool {
	return ns.NodeType == MgmNodeTypeID
}

func (ns *NodeStatus) isAPINode() bool {
	return ns.NodeType == APINodeTypeID
}

// NewClusterStatus creates a new ClusterStatus objects
// and allocates memory for nodeCount status entries
func NewClusterStatus(nodeCount int) *ClusterStatus {
	cs := make(ClusterStatus, nodeCount)
	return &cs
}

// IsClusterDegraded returns true if any data node or mgm node is down
func (cs *ClusterStatus) IsClusterDegraded() bool {

	isDegraded := false
	for _, ns := range *cs {
		if ns.isDataNode() || ns.isMgmNode() {
			isDegraded = isDegraded || !ns.IsConnected
		}
	}

	return isDegraded
}

// EnsureNode makes sure there is a node entry for the given nodeID
// if there is no node status for this node id yet then create
func (cs *ClusterStatus) EnsureNode(nodeID int) *NodeStatus {

	var nodeStatus *NodeStatus
	var ok bool
	if nodeStatus, ok = (*cs)[nodeID]; !ok {
		nodeStatus = &NodeStatus{
			NodeID: nodeID,
		}
		(*cs)[nodeID] = nodeStatus
	}

	return nodeStatus
}

// SetNodeTypeFromTLA sets the internal type id for the node state of nodeID
// allowed TLAs are NDB, MGM, API
func (cs *ClusterStatus) SetNodeTypeFromTLA(nodeID int, tla string) error {

	ns := cs.EnsureNode(nodeID)

	if ns == nil {
		return fmt.Errorf("Invalid node id %d", nodeID)
	}

	// node type NDB, MGM, API
	nodeType := InvalidTypeId
	switch tla {
	case "NDB":
		nodeType = DataNodeTypeID
		break
	case "MGM":
		nodeType = MgmNodeTypeID
		break
	case "API":
		nodeType = APINodeTypeID
		break
	default:
	}

	if nodeType == InvalidTypeId {
		return fmt.Errorf("Invalid node type %s", tla)
	}
	ns.NodeType = nodeType

	return nil
}
