# Kubernetes CRD Reference

FlexGateway provides three Custom Resource Definitions (CRDs) for managing gateway configuration as Kubernetes-native resources.

## Installation

Apply the CRD definitions to your cluster:

```bash
kubectl apply -f internal/k8s/crds/flexroute.yaml
kubectl apply -f internal/k8s/crds/flexconsumer.yaml
kubectl apply -f internal/k8s/crds/flexplugin.yaml
```

Or apply all CRDs at once:

```bash
kubectl apply -f internal/k8s/crds/
```

> **Note:** These CRDs define the schema for FlexGateway resources. A reconciler/operator that watches these resources and syncs them to the control plane is planned for a future release. Currently, you can use ArgoCD or Flux to manage the CRD manifests in Git, and a sync hook or sidecar can call `fgctl apply` to push changes to the control plane.

## API Group

All FlexGateway CRDs use the API group `flexgateway.io/v1alpha1`.

---

## FlexRoute

A FlexRoute defines an API route that maps an HTTP endpoint to a backend service.

### Example

```yaml
apiVersion: flexgateway.io/v1alpha1
kind: FlexRoute
metadata:
  name: get-users
  namespace: flexgateway
spec:
  name: get-users
  endpoint: /users
  method: GET
  backend: users-api
  blocks:
    - name: rate-limit
      config:
        requests_per_minute: 100
  metadata:
    team: platform
    tier: public
```

### Spec Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Unique name for the route. |
| `endpoint` | string | Yes | URL path pattern (e.g., `/users`, `/orders/:id`). |
| `method` | string | Yes | HTTP method (`GET`, `POST`, `PUT`, `DELETE`, etc.). |
| `backend` | string | Yes | Name of the backend to forward requests to. Must match an existing backend. |
| `blocks` | array | No | List of blocks (middleware) applied to this route. Each block has a `name` (string, required) and optional `config` (object, free-form). |
| `metadata` | object | No | Arbitrary key-value string pairs for labeling and filtering. |

---

## FlexConsumer

A FlexConsumer represents an API consumer (a user, team, or external client) with optional rate limit overrides.

### Example

```yaml
apiVersion: flexgateway.io/v1alpha1
kind: FlexConsumer
metadata:
  name: acme-corp
  namespace: flexgateway
spec:
  name: acme-corp
  org: my-org
  apiKeys:
    - "key-abc123"
    - "key-def456"
  rateLimit: 500
  metadata:
    plan: enterprise
    contact: ops@acme.com
```

### Spec Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Unique name for the consumer. |
| `org` | string | Yes | Organization this consumer belongs to. |
| `apiKeys` | array of strings | No | API keys assigned to this consumer. |
| `rateLimit` | integer | No | Rate limit override in requests per minute. When set, overrides the default org-level limit. |
| `metadata` | object | No | Arbitrary key-value string pairs for labeling and filtering. |

---

## FlexPlugin

A FlexPlugin defines a plugin that can be attached to routes as a processing block.

### Example

```yaml
apiVersion: flexgateway.io/v1alpha1
kind: FlexPlugin
metadata:
  name: custom-auth
  namespace: flexgateway
spec:
  name: custom-auth
  type: grpc
  enabled: true
  config:
    address: localhost:9090
    timeout: 5s
  targets:
    - kind: FlexRoute
      name: get-users
    - kind: FlexRoute
      name: create-order
```

### Spec Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Unique name for the plugin. |
| `type` | string | Yes | Plugin type. Must be `native` (built-in block) or `grpc` (external gRPC plugin). |
| `enabled` | boolean | Yes | Whether the plugin is active. Set to `false` to disable without removing the resource. |
| `config` | object | No | Plugin-specific configuration. Free-form object -- the schema depends on the plugin type. |
| `targets` | array | No | Resources this plugin applies to. Each target has a `kind` (string, required: `FlexRoute`) and `name` (string, required: the target resource name). |

---

## Labels and Annotations

All FlexGateway CRDs support standard Kubernetes labels and annotations on their `metadata` field. Use them for organizing resources and integrating with tools like ArgoCD and Flux:

```yaml
metadata:
  name: get-users
  namespace: flexgateway
  labels:
    app.kubernetes.io/part-of: flexgateway
    environment: production
  annotations:
    flexgateway.io/owner: platform-team
```

## Status and Conditions

Status subresources and conditions are not yet implemented. A future release will add a status field with sync state, last-applied timestamp, and error conditions reported by the operator.
