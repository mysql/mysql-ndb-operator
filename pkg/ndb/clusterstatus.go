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

	for _, ns := range *cs {
		if ns.isDataNode() || ns.isMgmNode() {
			if !ns.IsConnected {
				return true
			}
		}
	}

	return false
}

// NumberNodegroupsFullyUp returns number of node groups fully up
// It returns
//   - the number of nodegroups with a node group
//   - and number of scaling nodes running/connected but with no node group created
//     (which diveded by reduncanyLevel is the number of potential node groups)
//
// Its currently the only way to decide if cluster is healthy during
// scaling where new node are configured but not started.
// This function is a bit weird as the mgm status report contains node group 0 for any
// node not connected (its set to -1 in mgmapi) - also during scaling
// Some guess-work is applied with the help of noOfReplicas.
func (cs *ClusterStatus) NumberNodegroupsFullyUp(reduncancyLevel int) (int, int) {

	//TODO: function feels a bit brute force
	// as node group numbers actually should not have gaps

	//TODO: same function basically in topology, but just counting
	nodeMap := make(map[int]int)
	mgmCount := 0
	scaleNodes := 0

	// collect number of nodes up in each node group
	// during scaling a started node group not created in cluster is marked -256
	for _, ns := range *cs {

		if ns.isMgmNode() && ns.IsConnected {
			mgmCount++
			continue
		}
		if !ns.isDataNode() {
			continue
		}

		if ns.NodeGroup == -256 {
			scaleNodes++
			continue
		}

		if ns.NodeGroup < 0 || ns.NodeGroup > 144 {
			continue
		}

		_, ok := nodeMap[ns.NodeGroup]
		if !ok {
			nodeMap[ns.NodeGroup] = 0
		}

		if ns.IsConnected {
			nodeMap[ns.NodeGroup]++
		}
	}

	nodeGroupsFullyUp := 0
	for _, nodesLiveInNodeGroup := range nodeMap {
		if nodesLiveInNodeGroup == reduncancyLevel {
			nodeGroupsFullyUp++
		}
	}

	return nodeGroupsFullyUp, scaleNodes
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
