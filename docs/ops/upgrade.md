# Runbook: Helm Chart Upgrade

Roll forward to a new chart or image version with zero customer-visible
downtime via Kubernetes RollingUpdate + PDB.

## When to use

- New gateway image (security patch, feature release).
- Chart values change (resource bumps, replica count, new probes).
- Chart version bump.

## Preconditions

- Current release is healthy (all pods `Ready`, `/readyz` 200).
- You have read the release notes / CHANGELOG for the target version.
- You have a rollback plan ([rollback.md](./rollback.md)).
- Off-peak window preferred for the first production upgrade.
- A Postgres backup taken in the last hour
  ([backup-restore.md](./backup-restore.md)).

## Pre-upgrade checks

```bash
export NS=apiloom
export RELEASE=apiloom

# Current state
helm status "$RELEASE" -n "$NS"
helm get values "$RELEASE" -n "$NS" > /tmp/${RELEASE}-current-values.yaml
kubectl get pods -n "$NS" -l app.kubernetes.io/instance=$RELEASE
kubectl get pdb -n "$NS" "$RELEASE"
```

Confirm `minAvailable: 1` (at least) on the PDB so the rolling update
keeps at least one pod up.

## Diff the change

```bash
helm diff upgrade "$RELEASE" ./charts/flexgateway \
  --namespace "$NS" \
  --values /tmp/${RELEASE}-current-values.yaml \
  --set image.tag=<new-tag>
```

Review every changed resource. Pay attention to:

- `Deployment` (image tag, env vars, probes, resources)
- `ConfigMap` (gateway timeouts — these need pod restart to take effect)
- `Secret` (DSN changes — verify password did not regenerate by accident)
- `CronJob` (backup schedule)

If you see a **secret being recreated with a new password**, abort: that
will break the gateway's connection to Postgres until the DB password is
also rotated.

## Apply

```bash
helm upgrade "$RELEASE" ./charts/flexgateway \
  --namespace "$NS" \
  --reuse-values \
  --set image.tag=<new-tag> \
  --atomic \
  --timeout 10m
```

`--atomic` rolls back automatically if the upgrade does not succeed
within `--timeout`. Use it for production unless you specifically want
to leave a partial state to debug.

## Watch the rollout

In another terminal:

```bash
kubectl rollout status deploy/$RELEASE -n "$NS" --timeout=10m
kubectl get pods -n "$NS" -l app.kubernetes.io/instance=$RELEASE -w
```

Expected behaviour: pods replaced one at a time, new pod must pass
`/readyz` before old pod is terminated. PDB blocks eviction if it would
take available below `minAvailable`.

## Post-upgrade validation

```bash
# Image actually rolled
kubectl get deploy -n "$NS" "$RELEASE" \
  -o jsonpath='{.spec.template.spec.containers[0].image}'

# All replicas updated
kubectl get deploy -n "$NS" "$RELEASE" \
  -o jsonpath='{.status.updatedReplicas}/{.status.replicas}'

# Health
for p in $(kubectl get pods -n "$NS" -l app.kubernetes.io/instance=$RELEASE -o name); do
  echo "== $p =="
  kubectl exec -n "$NS" "$p" -- wget -qO- http://localhost:8080/readyz
done

# No spike in error rate (Prom)
# flexgw_requests_total{status_code=~"5.."} should not jump
```

Monitor `flexgw_request_duration_seconds` P99 and
`flexgw_requests_total{status_code=~"5.."}` for at least 15 minutes.
A regression here is your signal to roll back.

## Verification checklist

- [ ] All pods on the new image
- [ ] `/readyz` 200 on every pod
- [ ] No 5xx spike in Prometheus over the rollout window
- [ ] No new ERROR-level logs (`kubectl logs -n $NS deploy/$RELEASE --tail=200 | grep -i error`)
- [ ] `flexgw_config_reload_total{status="error"}` not increasing

## If anything looks wrong

Use [rollback.md](./rollback.md) immediately. Do not "wait and see" —
the longer you sit on a bad rollout, the more in-flight state you
accumulate that complicates rollback.
