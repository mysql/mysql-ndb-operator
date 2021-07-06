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

// IsHealthy returns true if the MySQL Cluster is healthy,
// i.e., if all the data and management nodes are connected and ready
func (cs ClusterStatus) IsHealthy() bool {
	for _, ns := range cs {
		switch ns.NodeType {
		case NodeTypeMGM:
			if !ns.IsConnected {
				return false
			}
		case NodeTypeNDB:
			if !ns.IsConnected {
				// During online add the new data node slots will
				// have a nodegroup 65536 and they will not have a
				// valid nodegroup until they are inducted into
				// the cluster. So, any unconnected node without
				// that nodegroup is an unhealthy node.
				// TODO: Verify if this still holds true during the online add node.
				//       If a node is being started on the Node slot with NG 65536,
				//       it will change its NG to -256 once it gets connected. This
				//       method might wrongly report such a node as healthy before it
				//       reaches the connected state.
				if ns.NodeGroup != 65536 {
					return false
				}
			}
		}
	}
	return true
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
