# Apiloom CE — Operator Runbooks

Operational runbooks for the Apiloom Gateway CE (apiloom-ce) Helm deployment.
Audience: SRE / on-call engineers running apiloom-ce in a Kubernetes cluster.

## Index

| Runbook | When to use |
|---|---|
| [startup.md](./startup.md) | First-time deployment from scratch |
| [upgrade.md](./upgrade.md) | Helm chart / image upgrade |
| [rollback.md](./rollback.md) | Roll back a bad release |
| [backup-restore.md](./backup-restore.md) | Take and restore a Postgres backup |
| [disaster-recovery.md](./disaster-recovery.md) | Full DR checklist + RTO/RPO targets + exercise log |
| [troubleshooting.md](./troubleshooting.md) | Common failure modes and remediation |

## Architecture (one-screen reference)

The CE deployment is two binaries and a database:

- **Gateway** (`cmd/gateway`, image `flexgateway/flexgateway:<tag>`) — data plane.
  Reads gateway config **directly from Postgres** at startup, then watches
  for changes via `LISTEN config_changes` on a dedicated pgx connection
  (`internal/config/watcher.go`). Reloads atomically; on reload error the
  previous config is preserved (`internal/config/loader.go:74`).
- **Controlplane** (`cmd/controlplane`) — admin/management API. Writes to
  Postgres and emits `NOTIFY config_changes` for gateway pods to pick up.
  **Not in the request path.** Gateway continues serving if controlplane
  is down.
- **Postgres 16** — single source of truth for routes, backends, consumers,
  plugins. The Helm chart expects a service named `<release>-postgresql`
  on port 5432 (see `charts/flexgateway/templates/_helpers.tpl:69`).

### Health endpoints (`internal/gateway/health.go`)

| Path | Purpose | What fails it |
|---|---|---|
| `/healthz` | Liveness — always 200 | Process dead only |
| `/readyz` | Readiness — config loaded AND Postgres reachable | DB unreachable, config not yet loaded |
| `/healthz/deep` | Diagnostic — DB latency, goroutines, heap | Always 200; status field reports degradation |

The split is deliberate: liveness MUST NOT depend on Postgres or
Kubernetes will cascade-restart pods during a DB outage and make
recovery harder.

### Metrics (Prometheus, scrape `/metrics`)

- `flexgw_requests_total{route,method,status_code}`
- `flexgw_request_duration_seconds{route,method,status_code}`
- `flexgw_active_connections`
- `flexgw_upstream_requests_total`, `flexgw_upstream_request_duration_seconds`
- `flexgw_config_reload_total{status}`

### Known chart caveats (as of chart 0.1.0)

These affect every runbook. Track in DevOps backlog before GA:

1. The chart **does not template a Postgres StatefulSet** — `values.yaml`
   has a `postgresql` section but `templates/` has no postgres workload.
   You must either set `postgresql.external.dsn` or install a Postgres
   sub-chart (e.g. `bitnami/postgresql`) with release-suffix
   `<release>-postgresql` so the helper DSN resolves.
2. `templates/cronjob-backup.yaml` references secret
   `<fullname>-postgres` with key `dsn`, but `templates/secret.yaml`
   creates `<fullname>` with keys `FLEXGW_DATABASE_URL` /
   `database-url`. Backup CronJob will fail until this mismatch is
   fixed; see [backup-restore.md](./backup-restore.md) for the
   workaround.
3. Backup `persistentVolume` defaults to disabled; with `emptyDir` the
   backup is lost when the Job pod terminates. Always set
   `backup.persistentVolume.enabled=true` in any environment you care
   about.

These are flagged in the DR exercise log
([disaster-recovery.md](./disaster-recovery.md#findings--followups)).
