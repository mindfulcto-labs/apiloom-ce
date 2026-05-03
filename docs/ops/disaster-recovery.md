# Runbook: Disaster Recovery

End-to-end recovery procedures, RTO/RPO targets, and the standing DR
exercise log for apiloom-ce.

## Targets

| Metric | Target | Source |
|---|---|---|
| Gateway availability | 99.9% / month (≤ 43m of unavailability) | Customer SLA |
| Config recovery (controlplane restore) | < 15 min after controlplane back up | Internal |
| Data recovery (Postgres restore) | RTO ≤ 15 min, RPO ≤ 5 min | Internal |
| Helm upgrade window | Zero customer-visible downtime | Internal |

The gateway pipeline (auth + rate-limit) is benchmarked at < 1 ms P99 /
738 K RPS in apiloom-ce; DR procedures must not silently introduce a
regression. Verify with `flexgw_request_duration_seconds` after each
DR action.

## What "disaster" covers here

| Class | Examples | Runbook |
|---|---|---|
| Pod-level | Crashloop, OOM, single-node failure | Below: §Pod recovery |
| Postgres data loss | Corrupt DB, accidental DROP, PVC lost | [backup-restore.md](./backup-restore.md) |
| Controlplane outage | Crashloop, deploy gone bad | Below: §Controlplane outage |
| Cluster loss | AZ down, Kubernetes API gone | Below: §Cluster rebuild |
| Bad release | Latency / error spike post-deploy | [rollback.md](./rollback.md) |

## Architecture facts that constrain DR

- Gateway reads config from Postgres (`internal/config/store.go`). It
  does **not** call the controlplane in the request path.
- Config is held in memory via `atomic.Pointer` swap
  (`internal/config/loader.go:50`). On reload error, the previous
  config is preserved — gateway keeps serving the last known good
  config indefinitely if Postgres goes away.
- Liveness `/healthz` is intentionally a no-op (`health.go:64`). It
  will not flap pods during a DB outage.
- Readiness `/readyz` requires Postgres reachability. During a DB
  outage, pods will be removed from Service endpoints — connections
  draining is normal and expected.
- Postgres LISTEN/NOTIFY drives reload (`config/watcher.go`); on
  notify-channel disconnect, watcher reconnects with exponential
  backoff up to 30 s.

## Pod recovery

Single pod crash:

```bash
kubectl get pods -n $NS -l app.kubernetes.io/instance=$RELEASE
kubectl describe pod -n $NS <bad-pod>
kubectl logs -n $NS <bad-pod> --previous
```

Kubernetes restarts the pod automatically. PDB `minAvailable: 1`
guarantees at least one healthy replica during the restart.
**No action required** other than RCA if it crashed for a non-OOM,
non-deployment reason. If it OOMed, bump `resources.limits.memory`
and consider a heap-profile via `/debug/pprof` (not enabled by
default — feature request).

## Controlplane outage

Gateway is unaffected — it keeps serving the last known config.

Symptoms operators will see:
- Cannot push new routes / consumers.
- `flexgw_config_reload_total` flat (no new versions).
- `LISTEN` reconnect log lines on gateway pods (harmless).

Recovery:

```bash
kubectl rollout restart deploy/${RELEASE}-controlplane -n $NS
kubectl rollout status   deploy/${RELEASE}-controlplane -n $NS --timeout=5m
```

If the controlplane is wedged because of a Postgres issue, fix Postgres
first; the controlplane will come back when its DB does.

After controlplane returns: push a no-op config write to verify the
NOTIFY pipe end-to-end. Watch for
`flexgw_config_reload_total{status="success"}` to tick up across all
gateway replicas.

## Cluster rebuild (worst-case)

Used when the entire Kubernetes cluster is unrecoverable.

1. Stand up a new cluster (Terraform / cluster-bootstrap is out of
   scope for this runbook — owned by DevOps).
2. Restore Postgres from the most recent off-cluster dump
   ([backup-restore.md](./backup-restore.md)) into the new cluster's
   Postgres.
3. Run [startup.md](./startup.md) against the new cluster, pointing
   the chart at the restored Postgres via
   `--set postgresql.external.dsn=...`.
4. Cut DNS / load balancer to the new cluster.
5. Validate `/readyz` and a customer-facing route end-to-end.

Realistic RTO for a cold cluster rebuild is **hours, not minutes** —
the 15-minute RTO target above only applies to the data layer once
the cluster exists. Communicate accordingly to customers.

---

## DR Exercise — Reference Plan (VEN-39)

Each scenario has a written procedure, a measured outcome, and a
finding. Run all four against a clean apiloom-ce deployment in a
non-prod cluster before each major release.

> **Status of this iteration:** Procedures fully scripted below.
> Live execution against a Kubernetes cluster requires DevOps to
> stand up the staging cluster (tracked separately). Until then, the
> "Result" rows below capture **expected** outcomes derived from
> reading the chart and code. Update them with measured timings
> after the first live run; a follow-up issue is tracked in
> §Findings & follow-ups.

### Scenario 1 — Backup & restore

**Procedure**
1. Deploy apiloom-ce per [startup.md](./startup.md) with backup enabled.
2. Apply 50 sample routes via controlplane / `fgctl`.
3. Take a manual `pg_dump` per [backup-restore.md](./backup-restore.md).
4. Drop the `routes`, `backends`, `consumers`, `plugins` tables in Postgres.
5. Confirm gateway `/readyz` still returns 200 (config cached in memory).
6. Send a request — should still serve the cached config.
7. Restart a gateway pod — `/readyz` will fail until restore.
8. Restore from the dump.
9. Bring gateway pods back to ready.

**RTO target:** ≤ 15 min from step 7 to step 9.
**Expected result:** Restore completes in ~5 min for a small dataset;
gateway resumes serving immediately on `InitialLoad`.

### Scenario 2 — Pod / container restart

**Procedure**
1. Generate steady traffic at ~1 K RPS through a configured route.
2. `kubectl delete pod` one of the two gateway replicas.
3. Measure: time-to-`Ready` for replacement pod; count of failed
   requests during the window.

**RTO target:** New pod `Ready` ≤ 30 s; **zero** in-flight requests
lost beyond what is expected from graceful shutdown
(`cmd/gateway/main.go:222` sleeps 3 s before `httpServer.Shutdown` to
let endpoint removal propagate).
**Expected result:** With `replicaCount: 2` + PDB `minAvailable: 1` +
the 3-second drain delay, the surviving pod absorbs all traffic
during the cutover; no observable error spike.

### Scenario 3 — Controlplane outage

**Procedure**
1. With steady traffic flowing, `kubectl scale deploy
   ${RELEASE}-controlplane --replicas=0`.
2. Verify gateway pods continue to serve all configured routes
   without errors for 10 min.
3. Attempt to push a route via the controlplane API — should fail
   (controlplane is down).
4. Scale controlplane back to its previous replica count.
5. Wait for controlplane `/readyz` 200.
6. Push the route again; observe `flexgw_config_reload_total{status="success"}`
   increment on every gateway replica within debounce + a few seconds.

**RTO target:** Config sync resumes ≤ 15 min after controlplane is
back to ready.
**Expected result:** Gateway request path completely unaffected during
the outage. Reload latency ≈ debounce interval (100 ms default) +
LISTEN/NOTIFY round trip.

### Scenario 4 — Helm upgrade

**Procedure**
1. Install chart at version A. Apply 50 routes. Generate steady
   traffic.
2. Bump chart version to B (e.g. tag bump only — no schema change).
3. `helm upgrade ... --atomic --timeout 10m`.
4. Watch `flexgw_requests_total{status_code=~"5.."}` and
   `flexgw_request_duration_seconds` P99 throughout.

**RTO target:** Zero downtime, no 5xx spike attributable to the
rollout, P99 within 2× steady-state baseline during the window.
**Expected result:** Default chart settings (RollingUpdate strategy,
PDB, anti-affinity, 2 replicas, readiness gating on
`/readyz`) deliver zero observable disruption.

### Findings & follow-ups

| # | Finding | Severity | Owner / tracking |
|---|---|---|---|
| 1 | Helm chart does not template a Postgres StatefulSet despite `values.yaml` describing one. Operators must install Postgres separately. | High (deploy blocker for naive users) | DevOps backlog — ticket needed |
| 2 | `cronjob-backup.yaml` references secret name `<fullname>-postgres` and key `dsn`, but `secret.yaml` creates `<fullname>` with keys `FLEXGW_DATABASE_URL`/`database-url`. Backup CronJob will not run. | High | DevOps backlog — ticket needed |
| 3 | Default backup volume is `emptyDir`; backups vanish with the Job pod. | Medium | Documented workaround in `backup-restore.md`; production installs should set `backup.persistentVolume.enabled=true` |
| 4 | No `controlplane` Deployment template in chart — only a gateway Deployment exists. Scenario 3 cannot be run as written until the chart adds a controlplane workload (or operators are told to deploy it manually). | High | DevOps backlog — ticket needed |
| 5 | No live cluster available for this iteration's exercise; expected outcomes derived from code/chart inspection. Live timings TBD. | Medium | Track as follow-up SRE issue once cluster is provisioned |
| 6 | No `/debug/pprof` endpoint exposed for memory / goroutine profiling during incidents. | Low | Engineering backlog — feature request |

These findings should be filed as separate child issues so they have
owners and dates rather than living in a runbook footnote.

## Verification checklist (per exercise)

- [ ] All four scenarios executed against a clean reference deployment
- [ ] Timings recorded against RTO targets in the table above
- [ ] Any RTO miss has a follow-up issue with owner and date
- [ ] Findings logged here and filed as tickets
- [ ] Runbooks updated with anything learned during the exercise
