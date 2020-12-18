package status

import (
	"fmt"
	"strconv"
	"strings"
)

const InvalidTypeId = 0
const IntTypeId = 1
const StringTypeId = 2
const SectionTypeId = 3
const Int64TypeId = 4

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

type NodeType int

func (t NodeType) String() string {
	switch t {
	case DataNodeTypeID:
		return "Data Node"
	case APINodeTypeID:
		return "API Node"
	case MgmNodeTypeID:
		return "MGM Node"
	}
	return "**INVALID NODE TYPE**"
}

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

func (s NodeStatus) String() string {
	r := NodeType(s.NodeType).String()
	r += " ID " + strconv.Itoa(s.NodeID)
	r += " " + s.SoftwareVersion
	if s.IsConnected {
		r += " connected"
	}
	return r
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

// sz is number of nodes. map is returned from MGMAPI GetStatus()
// map is iterated in random order
func GetClusterStatusFromMap(sz int, cf map[string]string) (*ClusterStatus, error) {
	nss := NewClusterStatus(sz)

	for key, val := range cf {
		s2 := strings.SplitN(key, ".", 3)
		if len(s2) != 3 {
			err := fmt.Errorf("Format error in map key: %s", key)
			return nil, err
		}

		// node id
		nodeId, err := strconv.Atoi(s2[1])
		if err != nil {
			return nil, err
		}
		nodeStatus := nss.EnsureNode(nodeId)

		// type of the status value (e.g. softare version, connection status)
		valType := s2[2]

		// based on the type of value store status away
		switch valType {
		case "type":
			err = nss.SetNodeTypeFromTLA(nodeId, val)
			if err != nil {
				return nil, err
			}

		case "status":
			nodeStatus.IsConnected = false
			// CONNECTED or STARTED depends on node type, which may be unknown
			if val == "CONNECTED" || val == "STARTED" {
				nodeStatus.IsConnected = true
			}

		case "version":
			valint, err := strconv.Atoi(val)
			if err != nil {
				return nil, fmt.Errorf("Format error (can't extract software version)")
			}

			if valint == 0 {
				nodeStatus.SoftwareVersion = ""
			} else {
				major := (valint >> 16) & 0xFF
				minor := (valint >> 8) & 0xFF
				build := (valint >> 0) & 0xFF
				nodeStatus.SoftwareVersion = fmt.Sprintf("%d.%d.%d", major, minor, build)
			}

		case "node_group":
			valint, err := strconv.Atoi(val)
			if err != nil {
				return nil, fmt.Errorf("Format error (can't extract node group) ")
			}
			nodeStatus.NodeGroup = valint

		}
	}

	// correct some data based on other data in respective node status
	for _, ns := range *nss {

		// if node is not started then we don't really know its node group reliably
		ng := -1
		if ns.NodeType == DataNodeTypeID && ns.IsConnected {
			ng = ns.NodeGroup
		}
		ns.NodeGroup = ng
	}

	return nss, nil
}
