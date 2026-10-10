package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/cobra"
)

var rollbackToVersion int

// configVersionItem matches the API list response item (no snapshot).
type configVersionItem struct {
	ID          string    `json:"id"`
	Version     int       `json:"version"`
	Author      string    `json:"author"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}

type configVersionListResponse struct {
	Data       []configVersionItem `json:"data"`
	NextCursor string              `json:"next_cursor,omitempty"`
	HasMore    bool                `json:"has_more"`
}

type configVersionFull struct {
	ID          string          `json:"id"`
	Version     int             `json:"version"`
	Snapshot    json.RawMessage `json:"snapshot"`
	Author      string          `json:"author"`
	Description string          `json:"description"`
	CreatedAt   time.Time       `json:"created_at"`
}

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage gateway configuration",
		Long:  "Commands for viewing config history and performing rollbacks.",
	}

	cmd.AddCommand(newConfigHistoryCmd())
	cmd.AddCommand(newConfigRollbackCmd())

	return cmd
}

func newConfigHistoryCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "history",
		Short: "Show config version history",
		Long:  "Display a table of config versions with version number, author, description, and date.",
		RunE:  runConfigHistory,
	}
}

func newConfigRollbackCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rollback",
		Short: "Rollback config to a previous version",
		Long:  "Create a new config version by restoring the snapshot from a previous version.",
		RunE:  runConfigRollback,
	}
	cmd.Flags().IntVar(&rollbackToVersion, "to-version", 0, "Target version number to rollback to (required)")
	_ = cmd.MarkFlagRequired("to-version")
	return cmd
}

func init() {
	rootCmd.AddCommand(newConfigCmd())
}

func runConfigHistory(cmd *cobra.Command, _ []string) error {
	if orgID == "" {
		return fmt.Errorf("--org flag is required")
	}

	client := NewControlPlaneClient(apiURL, orgID)
	resp, err := client.doRequest(cmd.Context(), http.MethodGet, "/api/v1/config/versions", nil)
	if err != nil {
		return fmt.Errorf("fetching config history: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if err := client.checkResponse(resp); err != nil {
		return err
	}

	var lr configVersionListResponse
	if err := json.NewDecoder(resp.Body).Decode(&lr); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}

	if len(lr.Data) == 0 {
		fmt.Println("No config versions found.")
		return nil
	}

	// Print table header.
	fmt.Printf("%-10s %-20s %-30s %s\n", "VERSION", "AUTHOR", "DESCRIPTION", "DATE")
	fmt.Printf("%-10s %-20s %-30s %s\n", "-------", "------", "-----------", "----")

	for _, v := range lr.Data {
		desc := v.Description
		if len(desc) > 28 {
			desc = desc[:28] + ".."
		}
		fmt.Printf("%-10d %-20s %-30s %s\n",
			v.Version,
			truncate(v.Author, 18),
			desc,
			v.CreatedAt.Format(time.RFC3339),
		)
	}

	return nil
}

func runConfigRollback(cmd *cobra.Command, _ []string) error {
	if orgID == "" {
		return fmt.Errorf("--org flag is required")
	}

	if rollbackToVersion < 1 {
		return fmt.Errorf("--to-version must be a positive integer")
	}

	client := NewControlPlaneClient(apiURL, orgID)
	payload := map[string]int{"to_version": rollbackToVersion}
	resp, err := client.doRequest(cmd.Context(), http.MethodPost, "/api/v1/config/rollback", payload)
	if err != nil {
		return fmt.Errorf("rolling back config: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if err := client.checkResponse(resp); err != nil {
		return err
	}

	var cv configVersionFull
	if err := json.NewDecoder(resp.Body).Decode(&cv); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}

	fmt.Printf("Rollback complete. New version: %d (restored from version %d)\n", cv.Version, rollbackToVersion)
	return nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-2] + ".."
}
