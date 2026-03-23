package domain

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// NodeType identifies the kind of vertex in the ontology graph.
type NodeType string

const (
	NodeTypeAPI         NodeType = "api"
	NodeTypeService     NodeType = "service"
	NodeTypeRoute       NodeType = "route"
	NodeTypeConsumer    NodeType = "consumer"
	NodeTypeTeam        NodeType = "team"
	NodeTypeOrg         NodeType = "org"
	NodeTypeModel       NodeType = "model"
	NodeTypeAgent       NodeType = "agent"
	NodeTypePolicy      NodeType = "policy"
	NodeTypePlugin      NodeType = "plugin"
	NodeTypeGateway     NodeType = "gateway"
	NodeTypeEnvironment NodeType = "environment"
	NodeTypeVersion     NodeType = "version"
)

// String returns the lowercase snake_case representation.
func (n NodeType) String() string {
	return string(n)
}

// Valid returns true for known node types.
func (n NodeType) Valid() bool {
	switch n {
	case NodeTypeAPI, NodeTypeService, NodeTypeRoute, NodeTypeConsumer,
		NodeTypeTeam, NodeTypeOrg, NodeTypeModel, NodeTypeAgent,
		NodeTypePolicy, NodeTypePlugin, NodeTypeGateway,
		NodeTypeEnvironment, NodeTypeVersion:
		return true
	default:
		return false
	}
}

// EdgeType identifies the kind of edge in the ontology graph.
type EdgeType string

const (
	EdgeTypeOwns         EdgeType = "owns"
	EdgeTypeSubscribesTo EdgeType = "subscribes_to"
	EdgeTypeDependsOn    EdgeType = "depends_on"
	EdgeTypeRoutesTo     EdgeType = "routes_to"
	EdgeTypeAppliesTo    EdgeType = "applies_to"
	EdgeTypeConsumes     EdgeType = "consumes"
	EdgeTypeAuthorizes   EdgeType = "authorizes"
	EdgeTypeBudgets      EdgeType = "budgets"
	EdgeTypeDataFlowsTo  EdgeType = "data_flows_to"
)

// String returns the lowercase snake_case representation.
func (e EdgeType) String() string {
	return string(e)
}

// Valid returns true for known edge types.
func (e EdgeType) Valid() bool {
	switch e {
	case EdgeTypeOwns, EdgeTypeSubscribesTo, EdgeTypeDependsOn,
		EdgeTypeRoutesTo, EdgeTypeAppliesTo, EdgeTypeConsumes,
		EdgeTypeAuthorizes, EdgeTypeBudgets, EdgeTypeDataFlowsTo:
		return true
	default:
		return false
	}
}

// GraphNode represents a vertex in the ontology graph.
type GraphNode struct {
	ID         uuid.UUID       `json:"id"`
	Type       NodeType        `json:"type"`
	OrgID      uuid.UUID       `json:"org_id"`
	Name       string          `json:"name"`
	Properties json.RawMessage `json:"properties,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
}

// GraphEdge represents a directed edge in the ontology graph.
type GraphEdge struct {
	ID         uuid.UUID       `json:"id"`
	SourceID   uuid.UUID       `json:"source_id"`
	TargetID   uuid.UUID       `json:"target_id"`
	Type       EdgeType        `json:"type"`
	OrgID      uuid.UUID       `json:"org_id"`
	Confidence float64         `json:"confidence"`
	Properties json.RawMessage `json:"properties,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
}

// ValidateConfidence returns an error if Confidence is outside [0.0, 1.0].
func (e *GraphEdge) ValidateConfidence() error {
	if e.Confidence < 0.0 || e.Confidence > 1.0 {
		return fmt.Errorf("confidence %v out of range [0.0, 1.0]", e.Confidence)
	}
	return nil
}

// ImpactResult represents a node affected by a change, along with the
// traversal path and hop depth from the origin.
type ImpactResult struct {
	AffectedNode GraphNode   `json:"affected_node"`
	Path         []GraphEdge `json:"path"`
	Depth        int         `json:"depth"`
}

// DependencyResult represents a dependency of a node, along with the
// traversal path and hop depth from the origin.
type DependencyResult struct {
	DependencyNode GraphNode   `json:"dependency_node"`
	Path           []GraphEdge `json:"path"`
	Depth          int         `json:"depth"`
}
