# Runbook: Troubleshooting

Most common failure modes for apiloom-ce, with the fastest known
remediation. Page-friendly; keep procedures short.

## Triage starting points

```bash
export NS=apiloom
export RELEASE=apiloom

kubectl get pods -n $NS -l app.kubernetes.io/instance=$RELEASE
kubectl get events -n $NS --sort-by=.lastTimestamp | tail -30
kubectl logs -n $NS deploy/$RELEASE --tail=200 --timestamps
```

Then:

```bash
POD=$(kubectl get pods -n $NS -l app.kubernetes.io/instance=$RELEASE -o name | head -1)
kubectl exec -n $NS "$POD" -- wget -qO- http://localhost:8080/healthz/deep
```

`healthz/deep` reports DB latency, goroutine count, heap. A degraded
status here usually points at the actual problem.

---

## Pod CrashLoopBackOff

### `FLEXGW_DATABASE_URL is required`

Symptom: pod logs `FLEXGW_DATABASE_URL is required` then exits.

Cause: Secret missing or the env var is not wired. The chart wires it
via `envFrom: secretRef` (`templates/deployment.yaml:50`). If you
overrode the deployment, the env var may be lost.

Fix:

```bash
kubectl get secret -n $NS ${RELEASE}-flexgateway -o jsonpath='{.data.FLEXGW_DATABASE_URL}' | base64 -d
```

If empty or wrong: re-run `helm upgrade` with the correct password
or external DSN.

### `connecting to database: ...`

Symptom: pod logs database connection error then exits.

Cause: Postgres unreachable, wrong credentials, or wrong DSN.

Diagnosis:

```bash
# From the gateway pod's network position
kubectl run pg-probe -n $NS --rm -i --restart=Never --image=postgres:16-alpine \
  -- pg_isready -h ${RELEASE}-postgresql -p 5432

# Verify the DSN the gateway is actually using
kubectl get secret -n $NS ${RELEASE}-flexgateway \
  -o jsonpath='{.data.FLEXGW_DATABASE_URL}' | base64 -d
```

Fix paths:
- Postgres pod down → see [disaster-recovery.md](./disaster-recovery.md#pod-recovery)
- Wrong password → rotate via `helm upgrade --reuse-values --set postgresql.auth.password=...`
- Wrong host → external DSN misconfigured; correct via
  `--set postgresql.external.dsn=...`

### Image pull failure

Symptom: `ImagePullBackOff` / `ErrImagePull`.

```bash
kubectl describe pod -n $NS <pod>
```

Look at the `Events:` section. Fix:
- Wrong tag → correct `image.tag` in values
- Private registry → ensure `imagePullSecrets` in the ServiceAccount

---

## `/readyz` failing

```bash
kubectl exec -n $NS $POD -- wget -qO- http://localhost:8080/readyz
```

Look at the `checks` map in the JSON response.

| Check failing | Meaning | Fix |
|---|---|---|
| `config: failing — config not loaded` | `InitialLoad` has not completed (or failed and was tolerated). Pod warning log: "initial config load failed". | Check Postgres reachability; restart pod. If DB is empty, this is expected on first install. |
| `postgres: failing — <error>` | Pool `Ping` failed inside 5s. | DB outage / network path; see "connecting to database" above. |

`/healthz` (liveness) deliberately ignores both — it stays 200 so
Kubernetes does not cascade-restart healthy processes during a DB
outage.

---

## Latency regression

Symptom: `flexgw_request_duration_seconds` P99 climbs above baseline.

1. Check `/healthz/deep` Postgres latency. If > 500 ms, status is
   "degraded" — DB is the suspect.
2. Check active connections (`flexgw_active_connections`) and
   goroutine count from `/healthz/deep`. Sudden growth implies a
   stuck upstream backing requests up.
3. Check upstream metrics:
   `flexgw_upstream_request_duration_seconds` per backend. A single
   slow backend usually explains a route-specific regression.
4. If the regression starts at a deploy timestamp →
   [rollback.md](./rollback.md).

---

## Config reload not propagating

Symptom: New routes pushed via controlplane / `fgctl` are not visible
on gateway. `flexgw_config_reload_total{status="success"}` not
incrementing.

1. Confirm the write hit Postgres:

   ```bash
   kubectl exec -n $NS ${RELEASE}-postgresql-0 -- \
     psql -U flexgw -d flexgateway -c "select count(*) from routes;"
   ```

2. Check that `NOTIFY config_changes` fires. The watcher
   (`internal/config/watcher.go`) listens on channel
   `config_changes`. If the controlplane is not emitting NOTIFY
   after writes, the gateway won't reload until the next forced
   reload.

3. Force a reload by restarting one gateway pod:

   ```bash
   kubectl delete pod -n $NS <one-gateway-pod>
   ```

   The new pod runs `InitialLoad` and picks up the latest config.
   If routes appear after restart but not via NOTIFY, the bug is in
   the controlplane's NOTIFY emission.

4. Check watcher reconnect log lines on the gateway. If you see a
   storm of reconnects, the dedicated pgx connection is being killed
   by Postgres / a load balancer (idle timeout). Bump
   `tcp_keepalives_*` on Postgres or shorten the watcher's keepalive.

---

## Schema migration failure

Symptom: gateway logs `running migrations: ...` then exits.

This is upgrade-time only (`cmd/gateway/main.go:96`). The pod will
crashloop until the migration succeeds.

1. **Do not blindly roll back** — partial migration may leave the
   schema in a state incompatible with both the old and new
   binaries.
2. Read the migration error verbatim. Most common causes:
   - Concurrent migration from two replicas → the migration tool
     should serialize; if it didn't, file a bug.
   - DDL conflict with manual changes to the schema → restore from
     backup and re-apply changes via the controlplane API.
3. Once the schema is consistent, rerun the upgrade.

---

## "It worked yesterday" baseline checks

Five things that explain 80% of mystery incidents:

1. **Cert expired** — TLS cert on the ingress, on Postgres `sslmode=verify-full`,
   or on an upstream. `openssl s_client -connect host:443 -servername host`.
2. **Disk full** — Postgres PVC near 100%; backup PVC near 100% (find +
   delete old dumps, then increase retention sanity).
3. **Image tag drifted** — `image.tag: latest` got a new digest you did
   not approve. Always pin tags in production.
4. **Secret rotated out of band** — DB password changed in Postgres but
   not in the gateway secret, or vice versa.
5. **DNS** — somewhere in the stack. Always DNS.

---

## When to escalate

Escalate to engineering on-call if:

- Crashloop persists after one rollback attempt.
- Schema migration failed and recovery requires manual SQL.
- 5xx error rate > 5% for 5 minutes and no recent deploy / config
  change explains it.
- Postgres restore from backup is required.
- You don't know what runbook to use — paging is correct.

Open a Sev-2 incident channel, link this runbook, capture logs and
events for the postmortem.
