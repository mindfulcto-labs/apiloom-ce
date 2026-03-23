package main

import "os"

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
