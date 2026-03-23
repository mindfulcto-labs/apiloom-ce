package main

import (
	"github.com/spf13/cobra"
)

var (
	apiURL string
	orgID  string
)

var rootCmd = &cobra.Command{
	Use:   "fgctl",
	Short: "FlexGateway CLI",
	Long:  "fgctl is the command-line interface for managing FlexGateway routes, backends, and configurations.",
	SilenceUsage: true,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&apiURL, "api-url", "http://localhost:8081", "Control plane API URL")
	rootCmd.PersistentFlags().StringVar(&orgID, "org", "", "Organization ID or slug")
}
