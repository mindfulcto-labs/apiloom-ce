# Runbook: Rollback

Revert apiloom-ce to the previously known-good release.

## When to use

- Post-upgrade validation failed (5xx spike, latency regression, crashloop).
- New code path triggered an unrecoverable bug (config-load failure, panic
  loop visible in `kubectl logs`).
- Customer reports correlated with the deploy timestamp.

**Speed matters.** First commit to roll back, then write the postmortem.

## Decision tree

| Symptom | Action |
|---|---|
| New pods crashlooping at startup | Helm rollback (this runbook) |
| 5xx error rate > 1% sustained 5m post-deploy | Helm rollback |
| Latency P99 > 2x baseline post-deploy | Helm rollback |
| Postgres schema migration failed | **Stop.** Migration may need manual intervention before rollback. See [troubleshooting.md](./troubleshooting.md#schema-migration-failure) |
| `--atomic` already rolled back | Investigate; you are already at the previous version |

## Pre-rollback

```bash
export NS=apiloom
export RELEASE=apiloom

# Capture current bad state for postmortem
kubectl logs -n "$NS" deploy/$RELEASE --tail=500 > /tmp/${RELEASE}-rollback-logs-$(date +%s).log
kubectl describe pods -n "$NS" -l app.kubernetes.io/instance=$RELEASE > /tmp/${RELEASE}-rollback-describe-$(date +%s).log

# Identify previous revision
helm history "$RELEASE" -n "$NS"
```

The output looks like:

```
REVISION  STATUS      CHART              APP VERSION  DESCRIPTION
3         deployed    flexgateway-0.2.0  0.2.0        Upgrade complete
2         superseded  flexgateway-0.1.0  0.1.0        Upgrade complete
1         superseded  flexgateway-0.1.0  0.1.0        Install complete
```

Roll back to revision **2** (the last `superseded` known-good one).

## Execute rollback

```bash
helm rollback "$RELEASE" 2 -n "$NS" --wait --timeout 5m
```

`--wait` blocks until pods are ready or timeout. If it errors, do **not**
re-run blindly — read the error, then jump to
[troubleshooting.md](./troubleshooting.md).

## Watch

```bash
kubectl rollout status deploy/$RELEASE -n "$NS" --timeout=5m
kubectl get pods -n "$NS" -l app.kubernetes.io/instance=$RELEASE
```

## Post-rollback validation

Same checks as upgrade, in reverse:

```bash
for p in $(kubectl get pods -n "$NS" -l app.kubernetes.io/instance=$RELEASE -o name); do
  kubectl exec -n "$NS" "$p" -- wget -qO- http://localhost:8080/readyz
done

# Confirm image tag is the old one
kubectl get deploy -n "$NS" "$RELEASE" \
  -o jsonpath='{.spec.template.spec.containers[0].image}'
```

Watch error rate and latency for 15 minutes to confirm the regression
is gone. If symptoms persist, the bad version was a coincidence — escalate
to on-call engineering.

## Database considerations

Helm rollback does **not** undo database migrations. The gateway runs
`config.RunMigrations` at startup (`cmd/gateway/main.go:96`). If the bad
release added a column or table, that DDL is still in place after
rollback. The previous binary should tolerate extra columns (it queries
specific columns), but verify by tailing logs for SQL errors.

If the bad release **dropped or renamed** something, a Helm rollback
alone is insufficient — restore Postgres from backup
([backup-restore.md](./backup-restore.md)) and replay any config changes
made since.

## Verification checklist

- [ ] `helm history` shows the rolled-back revision as `deployed`
- [ ] All pods on the previous image
- [ ] `/readyz` 200 on every pod
- [ ] Error rate / latency back to baseline
- [ ] Logs captured for postmortem
- [ ] Incident channel updated with "rollback complete, monitoring"
- [ ] Postmortem doc opened within 48h of incident close
