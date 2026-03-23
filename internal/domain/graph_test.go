package domain

import (
	"testing"
)

func TestNodeType_Valid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		nt       NodeType
		wantValid bool
	}{
		{"API", NodeTypeAPI, true},
		{"Service", NodeTypeService, true},
		{"Route", NodeTypeRoute, true},
		{"Consumer", NodeTypeConsumer, true},
		{"Team", NodeTypeTeam, true},
		{"Org", NodeTypeOrg, true},
		{"Model", NodeTypeModel, true},
		{"Agent", NodeTypeAgent, true},
		{"Policy", NodeTypePolicy, true},
		{"Plugin", NodeTypePlugin, true},
		{"Gateway", NodeTypeGateway, true},
		{"Environment", NodeTypeEnvironment, true},
		{"Version", NodeTypeVersion, true},
		{"empty string", NodeType(""), false},
		{"unknown type", NodeType("unknown"), false},
		{"case sensitive", NodeType("API"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.nt.Valid(); got != tt.wantValid {
				t.Errorf("NodeType(%q).Valid() = %v, want %v", tt.nt, got, tt.wantValid)
			}
		})
	}
}

func TestNodeType_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		nt   NodeType
		want string
	}{
		{NodeTypeAPI, "api"},
		{NodeTypeService, "service"},
		{NodeTypeEnvironment, "environment"},
	}

	for _, tt := range tests {
		t.Run(string(tt.nt), func(t *testing.T) {
			t.Parallel()
			if got := tt.nt.String(); got != tt.want {
				t.Errorf("NodeType(%q).String() = %q, want %q", tt.nt, got, tt.want)
			}
		})
	}
}

func TestEdgeType_Valid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		et        EdgeType
		wantValid bool
	}{
		{"owns", EdgeTypeOwns, true},
		{"subscribes_to", EdgeTypeSubscribesTo, true},
		{"depends_on", EdgeTypeDependsOn, true},
		{"routes_to", EdgeTypeRoutesTo, true},
		{"applies_to", EdgeTypeAppliesTo, true},
		{"consumes", EdgeTypeConsumes, true},
		{"authorizes", EdgeTypeAuthorizes, true},
		{"budgets", EdgeTypeBudgets, true},
		{"data_flows_to", EdgeTypeDataFlowsTo, true},
		{"empty string", EdgeType(""), false},
		{"unknown", EdgeType("unknown"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.et.Valid(); got != tt.wantValid {
				t.Errorf("EdgeType(%q).Valid() = %v, want %v", tt.et, got, tt.wantValid)
			}
		})
	}
}

func TestGraphEdge_ValidateConfidence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		confidence float64
		wantErr    bool
	}{
		{"zero", 0.0, false},
		{"one", 1.0, false},
		{"mid", 0.5, false},
		{"negative", -0.1, true},
		{"above one", 1.1, true},
		{"large negative", -100.0, true},
		{"large positive", 100.0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := GraphEdge{Confidence: tt.confidence}
			err := e.ValidateConfidence()
			if (err != nil) != tt.wantErr {
				t.Errorf("GraphEdge{Confidence: %v}.ValidateConfidence() error = %v, wantErr %v",
					tt.confidence, err, tt.wantErr)
			}
		})
	}
}
