# Runbook: Postgres Backup & Restore

The gateway is **stateless**; all durable state lives in Postgres.
Losing Postgres = losing every route, backend, consumer, and plugin
binding. RPO target: 5 minutes. RTO target: 15 minutes.

## Backup strategy

The chart ships two layers; use both in production:

1. **Logical daily dump** via `templates/cronjob-backup.yaml` (`pg_dump
   --format=custom`, retention 7 days). Cheap, easy to restore, but the
   RPO is the schedule interval (24h by default — bump to hourly for
   production).
2. **Cloud-provider snapshots** of the Postgres PVC. Out of scope for
   the chart, configured at the storage class level. Provides
   point-in-time-ish recovery between dumps.

### Chart caveats (must-fix before relying on the CronJob)

The shipped `cronjob-backup.yaml` references:

- secret `<release>-flexgateway-postgres` with key `dsn` — but
  `secret.yaml` creates `<release>-flexgateway` with keys
  `FLEXGW_DATABASE_URL` and `database-url`.
- `emptyDir` volume by default — the dump is lost when the Job pod is
  garbage collected.

**Until the chart is fixed**, run the manual backup procedure below
(works against the actually-existing secret) and disable the CronJob:

```bash
helm upgrade $RELEASE ./charts/flexgateway -n $NS --reuse-values \
  --set backup.enabled=false
```

A chart fix is tracked in the DevOps backlog.

## Manual backup (production-grade)

Run from any pod that can reach the Postgres service. Easiest is a
throwaway `postgres:16-alpine` pod scheduled with the gateway's secret
mounted:

```bash
export NS=apiloom
export RELEASE=apiloom
export STAMP=$(date -u +%Y%m%dT%H%M%SZ)

kubectl run pg-backup-$STAMP -n $NS --rm -i --restart=Never \
  --image=postgres:16-alpine \
  --overrides='
{
  "spec": {
    "containers": [{
      "name": "pg-backup",
      "image": "postgres:16-alpine",
      "command": ["/bin/sh","-c","pg_dump --format=custom --compress=9 --no-acl --no-owner \"$DSN\" -f /tmp/dump.pgdump && cat /tmp/dump.pgdump"],
      "env": [{
        "name": "DSN",
        "valueFrom": {"secretKeyRef":{"name":"'$RELEASE'-flexgateway","key":"FLEXGW_DATABASE_URL"}}
      }]
    }]
  }
}' > /tmp/apiloom-$STAMP.pgdump
```

Then **immediately** copy the dump to off-cluster object storage (S3,
GCS, R2). A backup that lives only on the cluster you are trying to
recover is not a backup:

```bash
aws s3 cp /tmp/apiloom-$STAMP.pgdump \
  s3://apiloom-backups/$NS/apiloom-$STAMP.pgdump \
  --sse=AES256
```

Verify the upload size matches and the file is readable:

```bash
aws s3 ls s3://apiloom-backups/$NS/apiloom-$STAMP.pgdump
```

## Verify a backup is restorable

A backup you have not restored is a wish. Quarterly, restore the most
recent dump into a throwaway namespace and run smoke tests. Never skip
this — most restore failures are discovered only at incident time.

```bash
export TEST_NS=apiloom-restore-test
kubectl create namespace $TEST_NS
# install Postgres in $TEST_NS (Bitnami subchart, see startup.md step 3)
# then restore dump using the procedure below, then run /readyz check
```

## Restore procedure

### 1. Stop writes

Scale gateway to 0 to prevent half-restored state from being served.
Controlplane too if it's running.

```bash
kubectl scale deploy/$RELEASE -n $NS --replicas=0
kubectl rollout status deploy/$RELEASE -n $NS --timeout=2m
```

### 2. Pull the dump from object storage

```bash
aws s3 cp s3://apiloom-backups/$NS/apiloom-<stamp>.pgdump /tmp/restore.pgdump
```

### 3. Drop and recreate the database

**Destructive.** Confirm namespace and dump file before proceeding.

```bash
kubectl run pg-restore -n $NS --rm -i --restart=Never \
  --image=postgres:16-alpine \
  --overrides='
{
  "spec": {
    "containers": [{
      "name": "pg-restore",
      "image": "postgres:16-alpine",
      "stdin": true,
      "command": ["/bin/sh","-c","psql \"$DSN\" -c \"DROP SCHEMA public CASCADE; CREATE SCHEMA public;\" && pg_restore --no-owner --no-acl --dbname=\"$DSN\" /dev/stdin"],
      "env": [{
        "name": "DSN",
        "valueFrom": {"secretKeyRef":{"name":"'$RELEASE'-flexgateway","key":"FLEXGW_DATABASE_URL"}}
      }]
    }]
  }
}' < /tmp/restore.pgdump
```

### 4. Bring the gateway back

```bash
kubectl scale deploy/$RELEASE -n $NS --replicas=2
kubectl rollout status deploy/$RELEASE -n $NS --timeout=5m
```

The gateway will:

1. Run migrations (idempotent — safe even on a freshly restored DB).
2. `InitialLoad` config from the restored tables.
3. Begin serving on `/readyz` 200.

### 5. Validate

```bash
for p in $(kubectl get pods -n $NS -l app.kubernetes.io/instance=$RELEASE -o name); do
  kubectl exec -n $NS "$p" -- wget -qO- http://localhost:8080/readyz
done
```

Hit a known route through the gateway to confirm config is the
expected one (route count, backends).

## Verification checklist

- [ ] Backup file exists in object storage with expected size
- [ ] Backup successfully restored to a test namespace within the last 90 days
- [ ] Restore RTO measured during quarterly test ≤ 15 minutes
- [ ] Off-site copy retention policy in place
- [ ] Backup encryption at rest verified (`aws s3api get-object-attributes`)
