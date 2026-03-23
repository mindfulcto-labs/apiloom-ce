// Package config provides the Postgres-backed configuration store for FlexGateway.
package config

import "embed"

// Migrations contains the embedded SQL migration files.
//
//go:embed migrations/*.sql
var Migrations embed.FS
