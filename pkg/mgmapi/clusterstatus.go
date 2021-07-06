// Copyright (c) 2021, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package mgmapi

// NodeTypeEnum identifies the node type used in NodeStatus object
// there are more types defined in c-code, but they are not used here
type NodeTypeEnum int

const (
	// NodeTypeNDB identifies a data node
	NodeTypeNDB NodeTypeEnum = iota
	// NodeTypeAPI identifies a MySQL Server or generic API node
	NodeTypeAPI
	// NodeTypeMGM identifies a management server
	NodeTypeMGM
)

func (t NodeTypeEnum) toString() string {
	switch t {
	case NodeTypeMGM:
		return "MGM"
	case NodeTypeNDB:
		return "NDB"
	case NodeTypeAPI:
		return "API"
	default:
		panic("unrecognized node type")
	}
}

// NodeStatus describes the state of a node of any node type in cluster
type NodeStatus struct {
	// Type of node: data node, API or management server
	NodeType NodeTypeEnum

	// node id of the node described in status object
	NodeId int

	// isConnected reports if the node is fully started and connected to cluster
	IsConnected bool

	// NodeGroup reports which node group the node is in, -1 if unclear or wrong node type
	NodeGroup int

	// mysql version number as string in format x.y.z
	SoftwareVersion string
}

func (ns *NodeStatus) IsDataNode() bool {
	return ns.NodeType == NodeTypeNDB
}

func (ns *NodeStatus) IsMgmNode() bool {
	return ns.NodeType == NodeTypeMGM
}

func (ns *NodeStatus) IsAPINode() bool {
	return ns.NodeType == NodeTypeAPI
}

// setNodeTypeFromTLA sets the nodeId's NodeStatus' node type.
// Allowed TLAs are NDB, MGM, API
func (ns *NodeStatus) setNodeTypeFromTLA(TLA string) {
	// node type NDB, MGM, API
	switch TLA {
	case "NDB":
		ns.NodeType = NodeTypeNDB
	case "MGM":
		ns.NodeType = NodeTypeMGM
	case "API":
		ns.NodeType = NodeTypeAPI
	default:
		panic("unsupported node type : " + TLA)
	}
}

// ClusterStatus describes the state of all nodes in the cluster
type ClusterStatus map[int]*NodeStatus

// NewClusterStatus returns a new ClusterStatus object
func NewClusterStatus(nodeCount int) ClusterStatus {
	return make(ClusterStatus, nodeCount)
}

// IsClusterDegraded returns true if any data node or mgm node is down
func (cs ClusterStatus) IsClusterDegraded() bool {
	for _, ns := range cs {
		if ns.IsDataNode() || ns.IsMgmNode() {
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
//     (which divided by redundancyLevel is the number of potential node groups)
//
// It is currently the only way to decide if cluster is healthy during
// scaling where new nodes are configured but not started.
// This function is a bit weird as the mgm status report contains node group 0 for any
// node not connected (its set to -1 in mgmapi) - also during scaling
// Some guess-work is applied with the help of noOfReplicas.
func (cs ClusterStatus) NumberNodegroupsFullyUp(redundancyLevel int) (int, int) {

	//TODO: function feels a bit brute force
	// as node group numbers actually should not have gaps

	//TODO: same function basically in topology, but just counting
	nodeMap := make(map[int]int)
	mgmCount := 0
	scaleNodes := 0

	// collect number of nodes up in each node group
	// during scaling a started node group not created in cluster is marked -256
	for _, ns := range cs {

		if ns.IsMgmNode() && ns.IsConnected {
			mgmCount++
			continue
		}
		if !ns.IsDataNode() {
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
		if nodesLiveInNodeGroup == redundancyLevel {
			nodeGroupsFullyUp++
		}
	}

	return nodeGroupsFullyUp, scaleNodes
}

// ensureNode returns the NodeStatus entry for the given node.
// If one is not present for the given nodeId, it creates a new
// NodeStatus entry and returns it
func (cs ClusterStatus) ensureNode(nodeId int) *NodeStatus {
	nodeStatus, ok := cs[nodeId]
	if !ok {
		// create a new entry
		nodeStatus = &NodeStatus{
			NodeId: nodeId,
		}
		cs[nodeId] = nodeStatus
	}
	return nodeStatus
}
