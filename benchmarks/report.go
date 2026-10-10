// Package benchmarks provides benchmark test suites and result reporting
// for FlexGateway performance validation.
package benchmarks

import (
	"fmt"
	"runtime"
	"strings"
	"time"
)

// BenchmarkResult holds the output from a single benchmark scenario.
type BenchmarkResult struct {
	Scenario   string
	RPS        float64
	NsPerOp    float64
	P50Us      float64
	P99Us      float64
	AllocsPerOp int64
	BytesPerOp  int64
}

// GenerateReport formats benchmark results into a markdown table with
// machine spec header, suitable for publishing in docs/BENCHMARKS.md.
func GenerateReport(results []BenchmarkResult) string {
	var sb strings.Builder

	// Machine spec header.
	sb.WriteString("## Test Environment\n\n")
	fmt.Fprintf(&sb, "- **CPUs:** %d\n", runtime.NumCPU())
	fmt.Fprintf(&sb, "- **GOMAXPROCS:** %d\n", runtime.GOMAXPROCS(0))
	fmt.Fprintf(&sb, "- **Go version:** %s\n", runtime.Version())
	fmt.Fprintf(&sb, "- **OS/Arch:** %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(&sb, "- **Date:** %s\n", time.Now().UTC().Format("2006-01-02"))
	sb.WriteString("\n")

	// Results table.
	sb.WriteString("## Results\n\n")
	sb.WriteString("| Scenario | RPS | p50 (us) | p99 (us) | Allocs/op | B/op |\n")
	sb.WriteString("|----------|-----|----------|----------|-----------|------|\n")

	for _, r := range results {
		fmt.Fprintf(&sb, "| %s | %s | %.0f | %.0f | %d | %d |\n",
			r.Scenario,
			formatRPS(r.RPS),
			r.P50Us,
			r.P99Us,
			r.AllocsPerOp,
			r.BytesPerOp,
		)
	}

	sb.WriteString("\n")
	return sb.String()
}

// formatRPS formats a requests-per-second value with comma separators.
func formatRPS(rps float64) string {
	if rps <= 0 {
		return "N/A"
	}
	n := int64(rps)
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var parts []string
	for len(s) > 3 {
		parts = append([]string{s[len(s)-3:]}, parts...)
		s = s[:len(s)-3]
	}
	parts = append([]string{s}, parts...)
	return strings.Join(parts, ",")
}
