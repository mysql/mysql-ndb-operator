// Copyright (c) 2020, 2021, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package mgmapi

import (
	"sort"
)

// nodeIDs contains the node ids of nodes in a ndb
type nodeIDs []int

// ClusterReplicas contains as many arrays of node ids as cluster has replicas
type ClusterReplicas []nodeIDs

// NewClusterTopologyByReplica creates a new topology description by ndb replica
func NewClusterTopologyByReplica(redundancyLevel int, nodeGroupCount int) *ClusterReplicas {
	ct := make(ClusterReplicas, redundancyLevel)
	for i := range ct {
		ct[i] = make(nodeIDs, nodeGroupCount)
	}
	return &ct
}

// GetNumberOfReplicas returns the number of cluster replicas (1, 2, 3 or 4)
func (cr *ClusterReplicas) GetNumberOfReplicas() int {
	return len(*cr)
}

// GetNumberOfNodeGroups returns the number of node groups
func (cr *ClusterReplicas) GetNumberOfNodeGroups() int {
	if cr.GetNumberOfReplicas() > 0 {
		return len((*cr)[0])
	}
	return 0
}

// GetNodeIDsFromReplica returns nodeids that belong to the replica with replicaID
func (cr *ClusterReplicas) GetNodeIDsFromReplica(replicaID int) *nodeIDs {
	if cr.GetNumberOfReplicas() >= 0 && replicaID < cr.GetNumberOfReplicas() {
		return &(*cr)[replicaID]
	}

	return nil
}

// CreateClusterTopologyByReplicaFromClusterStatus extracts
// a ClusterTopology from a ClusterStatus report object
func CreateClusterTopologyByReplicaFromClusterStatus(cs *ClusterStatus) *ClusterReplicas {

	// we use int for count of nodes
	tmpNodeGroups := make(map[int]nodeIDs)

	for _, ns := range *cs {
		if !ns.IsDataNode() {
			continue
		}

		if ns.NodeGroup < 0 || ns.NodeGroup > 144 {
			continue
		}

		_, ok := tmpNodeGroups[ns.NodeGroup]

		if !ok {
			tmpNodeGroups[ns.NodeGroup] = make(nodeIDs, 0, 4)
		}

		tmpNodeGroups[ns.NodeGroup] = append(tmpNodeGroups[ns.NodeGroup], ns.NodeId)
	}

	// sort by node group id and by node id within node group
	// - even though node groups should be without gaps
	nodeGroupIDs := make([]int, len(tmpNodeGroups))
	noOfReplicas := 0
	i := 0
	for k, nodeIDsInGrp := range tmpNodeGroups {

		// extract node group id into
		nodeGroupIDs[i] = k

		// sort the node ids within node group
		sort.Ints(nodeIDsInGrp)

		// all node groups should obviously have same node count 1..4
		if i > 0 && noOfReplicas != len(nodeIDsInGrp) {
			// error
			return nil
		}
		noOfReplicas = len(nodeIDsInGrp)
		i++
	}
	sort.Ints(nodeGroupIDs)

	noOfNodeGroups := len(nodeGroupIDs)

	ct := NewClusterTopologyByReplica(noOfReplicas, noOfNodeGroups)

	// copy over node ids in sorted order
	for _, nodeGroupID := range nodeGroupIDs {
		for replica := 0; replica < noOfReplicas; replica++ {
			// !! we have node groups with sorted replica nodeID
			(*ct)[replica][nodeGroupID] = tmpNodeGroups[nodeGroupID][replica]
		}

	}

	return ct
}
