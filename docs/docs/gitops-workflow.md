# GitOps Workflow

Manage FlexGateway configuration as code using Git as the source of truth.

## Overview

FlexGateway supports a declarative configuration model where YAML manifests define your desired gateway state. The `fgctl` CLI reconciles live state against these manifests, making it straightforward to integrate with GitOps tools like ArgoCD, Flux, and CI/CD pipelines.

The core principle: **manifests committed to Git are the source of truth.** Any change to your gateway configuration starts as a pull request, gets reviewed, and is applied automatically on merge.

A FlexGateway manifest looks like this:

```yaml
apiVersion: flexgateway/v1
kind: GatewayConfig
metadata:
  organization: my-org
spec:
  routes:
    - name: get-users
      config:
        endpoint: /users
        method: GET
        backend: users-api
  backends:
    - name: users-api
      config:
        url: https://users.internal:8080
```

## The GitOps Pipeline

The workflow follows five steps:

### 1. Export current state

Capture your live gateway configuration into a YAML manifest:

```bash
fgctl export --org my-org -o gateway-config.yaml
```

This produces a complete manifest representing the current routes and backends. Commit this file to your Git repository as the baseline.

### 2. Commit manifests to Git

Store manifests in a dedicated directory within your repository:

```
infra/
  flexgateway/
    gateway-config.yaml
    staging-config.yaml
    production-config.yaml
```

### 3. Modify configuration in a PR

Make changes to the manifest in a feature branch. For example, adding a new route:

```yaml
# Add to spec.routes
- name: get-orders
  config:
    endpoint: /orders
    method: GET
    backend: orders-api
```

Open a pull request for review.

### 4. CI runs drift detection

Your CI pipeline runs `fgctl drift` on the PR to show what would change:

```bash
fgctl drift --org my-org -f gateway-config.yaml --json
```

The drift report shows any differences between the manifest and live state, including resources that will be created, modified, or removed. Post the results as a PR comment so reviewers can see the impact.

### 5. CD applies changes on merge

When the PR merges to `main`, your CD pipeline applies the changes:

```bash
fgctl plan --org my-org -f gateway-config.yaml --auto-approve
```

The `plan` command diffs the manifest against live state, displays the execution plan, and applies changes. The `--auto-approve` flag skips the interactive confirmation prompt, which is necessary in CI environments.

## ArgoCD Integration

ArgoCD can manage FlexGateway configuration in two ways.

### Option A: CRD-based sync

If you deploy FlexGateway CRDs to your Kubernetes cluster, ArgoCD can sync them directly. Point an ArgoCD Application at the directory containing your FlexRoute, FlexConsumer, and FlexPlugin resources:

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: flexgateway-config
  namespace: argocd
spec:
  project: default
  source:
    repoURL: https://github.com/your-org/infra-config.git
    path: flexgateway/manifests
    targetRevision: main
  destination:
    server: https://kubernetes.default.svc
    namespace: flexgateway
  syncPolicy:
    automated:
      selfHeal: true
      prune: true
```

See [examples/gitops/argocd-app.yaml](../../examples/gitops/argocd-app.yaml) for a complete example.

### Option B: Sync hook with fgctl

For non-Kubernetes deployments, use an ArgoCD PreSync or PostSync hook that runs `fgctl plan --auto-approve` against your manifest. This approach works when your FlexGateway control plane runs outside the cluster.

## Flux Integration

Flux can sync FlexGateway CRDs using a Kustomization resource pointing at the manifest directory in your Git repository:

```yaml
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: flexgateway-config
  namespace: flux-system
spec:
  interval: 5m
  sourceRef:
    kind: GitRepository
    name: infra-config
  path: ./flexgateway/manifests
  prune: true
  targetNamespace: flexgateway
```

Flux will watch the Git repository and apply any CRD changes it detects during each reconciliation cycle.

## GitHub Actions CI/CD

A typical GitHub Actions setup uses two jobs: drift detection on pull requests, and auto-apply on merge to `main`.

### Drift check on PRs

```yaml
on:
  pull_request:
    paths:
      - 'infra/flexgateway/**'

jobs:
  drift-check:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - run: go install github.com/flexgateway/flexgateway/cmd/fgctl@latest
      - name: Run drift detection
        run: fgctl drift --org ${{ vars.ORG_ID }} -f infra/flexgateway/gateway-config.yaml --json > drift-report.json
        env:
          FLEXGATEWAY_API_URL: ${{ vars.API_URL }}
          FLEXGATEWAY_API_KEY: ${{ secrets.API_KEY }}
      - name: Post drift report
        uses: actions/github-script@v7
        with:
          script: |
            const fs = require('fs');
            const report = JSON.parse(fs.readFileSync('drift-report.json', 'utf8'));
            const body = report.has_drift
              ? `### Drift Detected\n\n${report.summary}`
              : '### No Drift\nConfiguration is in sync.';
            github.rest.issues.createComment({
              issue_number: context.issue.number,
              owner: context.repo.owner,
              repo: context.repo.repo,
              body
            });
```

### Auto-apply on merge

```yaml
on:
  push:
    branches: [main]
    paths:
      - 'infra/flexgateway/**'

jobs:
  apply:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - run: go install github.com/flexgateway/flexgateway/cmd/fgctl@latest
      - name: Apply configuration
        run: fgctl plan --org ${{ vars.ORG_ID }} -f infra/flexgateway/gateway-config.yaml --auto-approve
        env:
          FLEXGATEWAY_API_URL: ${{ vars.API_URL }}
          FLEXGATEWAY_API_KEY: ${{ secrets.API_KEY }}
```

See [examples/gitops/github-actions.yaml](../../examples/gitops/github-actions.yaml) for a complete workflow file.

## Drift Detection

The `fgctl drift` command compares a YAML manifest against the current control plane state and reports any configuration drift.

### Exit codes

| Code | Meaning |
|------|---------|
| 0    | No drift -- manifest matches live state |
| 1    | Drift detected -- differences found |
| 2    | Error occurred (connection failure, parse error, etc.) |

### JSON output for CI

Use `--json` for machine-readable output:

```bash
fgctl drift --org my-org -f gateway-config.yaml --json
```

The JSON report includes:

```json
{
  "timestamp": "2026-01-15T10:30:00Z",
  "has_drift": true,
  "summary": "Drift detected: 1 missing, 0 extra, 1 modified",
  "entries": [
    {
      "kind": "route",
      "name": "get-orders",
      "action": "create",
      "category": "missing"
    },
    {
      "kind": "backend",
      "name": "users-api",
      "action": "update",
      "category": "modified"
    }
  ]
}
```

### Drift categories

- **missing** -- Resource exists in the manifest but not in the live state (will be created)
- **extra** -- Resource exists in the live state but not in the manifest (will be deleted if `--delete` is used)
- **modified** -- Resource exists in both but with different configuration (will be updated)

## Best Practices

**Keep manifests in a dedicated directory.** Store FlexGateway YAML files in a clearly named path like `infra/flexgateway/` rather than mixing them with application code.

**Run `fgctl plan` in CI before applying.** Always preview changes in a PR before they hit production. Treat drift reports like terraform plans -- review before merge.

**Schedule drift detection.** Run `fgctl drift` on a cron schedule (daily or hourly) to catch manual changes or external modifications that bypass the GitOps pipeline.

**Use `--delete` cautiously.** The `--delete` flag on `fgctl plan` removes resources that exist in the live state but not in the manifest. This is powerful but can cause outages if your manifest is incomplete. Start without `--delete` and add it once you are confident the manifest is the complete source of truth.

**Separate environments with separate manifests.** Maintain one manifest per environment (dev, staging, production) rather than using variable substitution. This keeps each environment's config auditable in Git history.

**Pin fgctl versions in CI.** Use a specific release tag or commit hash when installing fgctl in your pipeline to avoid unexpected behavior from version changes.
