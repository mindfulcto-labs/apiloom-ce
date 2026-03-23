package domain

import (
	"time"

	"github.com/google/uuid"
)

// QueryType classifies a natural language intelligence query for routing.
// INT-02: three-way classification (graph | metric | hybrid).
type QueryType string

const (
	// QueryTypeGraph routes to deterministic Cypher pattern matching or Graph RAG.
	// Covers structural questions about relationships, topology, dependencies, ownership.
	QueryTypeGraph QueryType = "graph"

	// QueryTypeMetric routes to metric SQL query builder.
	// Covers performance data, costs, counts, rates, percentiles, trends.
	QueryTypeMetric QueryType = "metric"

	// QueryTypeHybrid routes to BOTH graph traversal AND metric aggregation.
	// Example: "what is the latency of the payment chain?" needs graph topology + per-hop metrics.
	QueryTypeHybrid QueryType = "hybrid"
)

// QueryIntent holds the result of classifying a natural language question.
type QueryIntent struct {
	// Type is the three-way classification of the question.
	Type QueryType

	// Entities contains extracted entity names (e.g., "payment-api", "acme-corp").
	Entities []string

	// TimeRange is the extracted time window, or nil if no time reference found.
	TimeRange *TimeRange

	// MetricNames contains extracted metric names (for metric/hybrid types).
	MetricNames []string

	// RawQuestion is the original question verbatim.
	RawQuestion string

	// Confidence is the classification confidence [0.0, 1.0].
	// 0.9 = strong pattern match, 0.7 = single match, 0.5 = default fallback.
	Confidence float64

	// NeedsRAG is true for questions that require LLM synthesis even if
	// classified as QueryTypeGraph (e.g., "why", "explain", "recommend").
	// These route to GraphRAGSynthesizer instead of DeterministicCypherGenerator.
	NeedsRAG bool
}

// TimeRange is an extracted time window from a natural language question.
type TimeRange struct {
	Start time.Time
	End   time.Time
	// Label is the human-readable description (e.g., "last 24 hours", "today").
	Label string
}

// MetricDefinition is a structured descriptor for a single observable metric.
// INT-11: typed metric catalog, not a string blob.
type MetricDefinition struct {
	// Name is the machine-readable metric name (e.g., "api_request_latency_p99").
	Name string

	// Description is the human-readable explanation.
	Description string

	// Unit describes what the number means: "milliseconds" | "usd" | "percentage" |
	// "requests_per_second" | "tokens" | "count"
	Unit string

	// Dimensions are the filterable facets for this metric.
	// Example: ["route", "consumer", "method", "status_code"]
	Dimensions []string

	// Aggregations are the supported aggregation operations.
	// Example: ["avg", "max", "min", "p50", "p95", "p99", "sum", "trend"]
	Aggregations []string

	// QueryHint is a SQL pattern hint for the metric SQL query builder.
	// Example: "SELECT route_id, AVG(latency_ms) FROM request_metrics WHERE ..."
	QueryHint string
}

// EvidenceItem is a single piece of grounding evidence in an IntelligenceResult.
// Each item records what data was used to construct the answer.
type EvidenceItem struct {
	// Type is "graph_node" | "metric_row" | "config_fact"
	Type string

	// NodeID is the graph node UUID, populated when Type="graph_node".
	NodeID *uuid.UUID

	// Label is the human-readable label for the evidence item.
	Label string

	// Value is the serialized value (string representation of the data).
	Value string

	// Source is the data source: "age_graph" | "metric_store" | "config"
	Source string
}
