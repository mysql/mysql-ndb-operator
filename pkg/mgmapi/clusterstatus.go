// Copyright (c) 2021, 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package mgmapi

import (
	"github.com/mysql/ndb-operator/config/debug"
	"sort"
)

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
		debug.Panic("unrecognized node type")
		return ""
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
		debug.Panic("unsupported node type : " + TLA)
	}
}

// ClusterStatus describes the state of all nodes in the cluster
type ClusterStatus map[int]*NodeStatus

// NewClusterStatus returns a new ClusterStatus object
func NewClusterStatus(nodeCount int) ClusterStatus {
	return make(ClusterStatus, nodeCount)
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

// Nodegroup of data nodes added to cluster through online add procedure
const (
	NodeGroupNewDisconnectedDataNode = 65536
	NodeGroupNewConnectedDataNode    = -256
)

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
				if ns.NodeGroup != NodeGroupNewDisconnectedDataNode {
					return false
				}
			}
		}
	}
	return true
}

// GetNodesGroupedByNodegroup returns an array of grouped
// and sorted node ids grouped based on node groups.
func (cs ClusterStatus) GetNodesGroupedByNodegroup() [][]int {

	// Map to store the nodes based on nodegroup
	nodesInNodeGroupMap := make(map[int][]int)
	for nodeId, node := range cs {
		if !node.IsDataNode() {
			// not a data node
			continue
		}
		nodegroup := node.NodeGroup
		if !node.IsConnected {
			if nodegroup == NodeGroupNewDisconnectedDataNode {
				// node is not inducted into cluster yet
				continue
			}
			// data node not connected
			debug.Panic("should be called only when the cluster is healthy")
			return nil
		}

		if nodegroup == NodeGroupNewConnectedDataNode {
			// ignore new connected data nodes without a nodegroup
			continue
		}

		// append the node id to the map based on nodegroup
		nodesInNodeGroupMap[nodegroup] = append(nodesInNodeGroupMap[nodegroup], nodeId)
	}

	// The map has the required node ids grouped under node groups
	// but nothing is sorted yet. Sort the output based on node groups
	// and then the sort the node ids under them
	nodeGroupIds := make([]int, 0, len(nodesInNodeGroupMap))
	for ng, nodeIdsInNodegroup := range nodesInNodeGroupMap {
		nodeGroupIds = append(nodeGroupIds, ng)

		// sort the node ids
		sort.Ints(nodeIdsInNodegroup)
	}
	// sort the node group
	sort.Ints(nodeGroupIds)

	// Copy out the sorted output in an [][]int and return
	nodesGroupedByNodegroup := make([][]int, 0, len(nodesInNodeGroupMap))
	for ng := range nodeGroupIds {
		nodesGroupedByNodegroup = append(nodesGroupedByNodegroup, nodesInNodeGroupMap[ng])
	}

	return nodesGroupedByNodegroup
}

// GetConnectedDataNodesWithNodeGroup returns a sorted list
// of all nodeIds that belong to the given nodegroup ng.
func (cs ClusterStatus) GetConnectedDataNodesWithNodeGroup(ng int) []int {
	// Map to store the nodes based on nodegroup
	var nodesInNodeGroup []int
	for nodeId, node := range cs {
		if !node.IsDataNode() || node.NodeGroup != ng {
			// not a data node or doesn't have the expected nodegroup
			continue
		}

		if !node.IsConnected {
			// data node not connected
			debug.Panic("should be called only when the cluster is healthy")
			return nil
		}

		// append the node id to the map based on nodegroup
		nodesInNodeGroup = append(nodesInNodeGroup, nodeId)
	}

	// sort the nodeIds
	sort.Ints(nodesInNodeGroup)
	return nodesInNodeGroup
}
