# Runbook: Initial Deployment

Bring apiloom-ce up in a clean Kubernetes namespace from a checked-out repo.

## Preconditions

- Kubernetes 1.27+ cluster, `kubectl` context pointing at the target cluster.
- Helm 3.12+.
- Namespace exists (or you have permission to create one).
- A reachable Postgres 16 instance, **or** a pre-installed `bitnami/postgresql`
  release co-deployed in the same namespace (see chart caveats in
  [README.md](./README.md)).
- Container image `flexgateway/flexgateway:<tag>` reachable from the
  cluster's image pull credentials.

## Steps

### 1. Set environment

```bash
export NS=apiloom
export RELEASE=apiloom
export CHART=./charts/flexgateway
export PG_PASSWORD="$(openssl rand -base64 24)"   # store in vault
```

### 2. Create namespace

```bash
kubectl create namespace "$NS"
```

### 3. Provision Postgres

**Option A — co-deployed Bitnami Postgres (fast path for staging):**

```bash
helm install "${RELEASE}-postgresql" bitnami/postgresql \
  --namespace "$NS" \
  --set auth.username=flexgw \
  --set auth.password="$PG_PASSWORD" \
  --set auth.database=flexgateway \
  --set primary.persistence.size=10Gi
```

Wait for ready:

```bash
kubectl rollout status statefulset/${RELEASE}-postgresql -n "$NS" --timeout=5m
```

**Option B — external Postgres (recommended for production):**

Have the connection string ready: `postgres://user:pw@host:5432/dbname?sslmode=require`.
Use `--set postgresql.external.dsn=...` in step 4 instead of password.

### 4. Install the chart

```bash
helm install "$RELEASE" "$CHART" \
  --namespace "$NS" \
  --set postgresql.auth.password="$PG_PASSWORD" \
  --set image.tag=<pinned-version> \
  --set replicaCount=2 \
  --set podDisruptionBudget.enabled=true \
  --set backup.enabled=true \
  --set backup.persistentVolume.enabled=true \
  --set backup.persistentVolume.size=20Gi
```

For external Postgres replace the password line with:
`--set postgresql.external.dsn="postgres://..."`.

### 5. Wait for rollout

```bash
kubectl rollout status deploy/$RELEASE -n "$NS" --timeout=5m
kubectl get pods -n "$NS" -l app.kubernetes.io/instance=$RELEASE
```

Expected: 2/2 pods `Running`, `READY 1/1`.

### 6. Verify health

```bash
POD=$(kubectl get pods -n "$NS" -l app.kubernetes.io/instance=$RELEASE \
  -o jsonpath='{.items[0].metadata.name}')

kubectl exec -n "$NS" "$POD" -- wget -qO- http://localhost:8080/healthz
kubectl exec -n "$NS" "$POD" -- wget -qO- http://localhost:8080/readyz
kubectl exec -n "$NS" "$POD" -- wget -qO- http://localhost:8080/healthz/deep
```

All three should return JSON with `"status":"ok"`. `/readyz` failing
means Postgres is unreachable — see [troubleshooting.md](./troubleshooting.md).

### 7. Verify metrics

```bash
kubectl exec -n "$NS" "$POD" -- wget -qO- http://localhost:8080/metrics \
  | grep -E '^flexgw_(requests_total|config_reload_total)'
```

You should see at least the metric definitions; counts will be near zero.

### 8. Smoke a request through the gateway

Port-forward and hit a configured route (or the default 404 path to confirm
the listener is alive):

```bash
kubectl port-forward -n "$NS" svc/$RELEASE 8080:8080 &
curl -i http://localhost:8080/  # expect 404 from gateway, not connection refused
kill %1
```

## Verification checklist

- [ ] `helm status $RELEASE -n $NS` reports `STATUS: deployed`
- [ ] All gateway pods `Ready 1/1`
- [ ] `/healthz`, `/readyz`, `/healthz/deep` return `"status":"ok"`
- [ ] `/metrics` returns Prometheus-format output
- [ ] PDB present: `kubectl get pdb -n $NS $RELEASE`
- [ ] Backup CronJob present (if enabled): `kubectl get cronjob -n $NS`

## Rollback

If install fails partway:

```bash
helm uninstall "$RELEASE" -n "$NS"
```

Postgres data persists on its PVC. Either reuse it on the next install
attempt or delete the PVC explicitly if you want a clean slate:

```bash
kubectl delete pvc -n "$NS" -l app.kubernetes.io/instance=${RELEASE}-postgresql
```
