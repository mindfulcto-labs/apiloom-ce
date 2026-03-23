# Terraform Provider

The FlexGateway Terraform provider lets you manage gateway configuration alongside your other infrastructure using HashiCorp Terraform.

> **Note:** CRUD operations in this provider are currently stubbed. The provider defines the resource schema and accepts plans, but full create/read/update/delete against the control plane API is coming in a future release.

## Provider Configuration

```hcl
terraform {
  required_providers {
    flexgateway = {
      source  = "flexgateway/flexgateway"
      version = "~> 0.1"
    }
  }
}

provider "flexgateway" {
  api_url = "https://api.flexgateway.io"
  org_id  = "my-org"
  api_key = var.flexgateway_api_key
}
```

### Provider Arguments

| Argument | Type | Required | Description |
|----------|------|----------|-------------|
| `api_url` | string | Yes | URL of the FlexGateway control plane API. Can also be set via `FLEXGATEWAY_API_URL` environment variable. |
| `org_id` | string | Yes | Organization identifier. Can also be set via `FLEXGATEWAY_ORG_ID` environment variable. |
| `api_key` | string | Yes | API key for authentication. Can also be set via `FLEXGATEWAY_API_KEY` environment variable. Treat as sensitive. |

---

## Resources

### flexgateway_route

Manages an API route in FlexGateway.

#### Example

```hcl
resource "flexgateway_route" "get_users" {
  name     = "get-users"
  endpoint = "/users"
  method   = "GET"
  backend  = flexgateway_backend.users_api.name

  block {
    name = "rate-limit"
    config = jsonencode({
      requests_per_minute = 100
    })
  }

  metadata = {
    team = "platform"
    tier = "public"
  }
}
```

#### Argument Reference

| Argument | Type | Required | Description |
|----------|------|----------|-------------|
| `name` | string | Yes | Unique name for the route. |
| `endpoint` | string | Yes | URL path pattern (e.g., `/users`). |
| `method` | string | Yes | HTTP method (`GET`, `POST`, `PUT`, `DELETE`). |
| `backend` | string | Yes | Name of the backend this route forwards to. |
| `block` | block list | No | Middleware blocks applied to the route. Each block has `name` (string, required) and `config` (JSON string, optional). |
| `metadata` | map of strings | No | Key-value pairs for labeling. |

#### Attributes

| Attribute | Description |
|-----------|-------------|
| `id` | Internal identifier assigned by the control plane. |
| `created_at` | Timestamp when the route was created. |
| `updated_at` | Timestamp of the last update. |

---

### flexgateway_backend

Manages a backend (upstream service) in FlexGateway.

#### Example

```hcl
resource "flexgateway_backend" "users_api" {
  name = "users-api"

  config = jsonencode({
    url     = "https://users.internal:8080"
    timeout = "5s"
  })

  metadata = {
    service = "users"
  }
}

resource "flexgateway_backend" "orders_api" {
  name = "orders-api"

  config = jsonencode({
    url     = "https://orders.internal:8080"
    timeout = "10s"
  })

  metadata = {
    service = "orders"
  }
}
```

#### Argument Reference

| Argument | Type | Required | Description |
|----------|------|----------|-------------|
| `name` | string | Yes | Unique name for the backend. |
| `config` | string (JSON) | Yes | Backend configuration as a JSON string. Must include `url`. |
| `metadata` | map of strings | No | Key-value pairs for labeling. |

#### Attributes

| Attribute | Description |
|-----------|-------------|
| `id` | Internal identifier assigned by the control plane. |
| `created_at` | Timestamp when the backend was created. |
| `updated_at` | Timestamp of the last update. |

---

### flexgateway_consumer

Manages an API consumer in FlexGateway.

#### Example

```hcl
resource "flexgateway_consumer" "acme" {
  name       = "acme-corp"
  org        = "my-org"
  api_keys   = ["key-abc123", "key-def456"]
  rate_limit = 500

  metadata = {
    plan    = "enterprise"
    contact = "ops@acme.com"
  }
}
```

#### Argument Reference

| Argument | Type | Required | Description |
|----------|------|----------|-------------|
| `name` | string | Yes | Unique name for the consumer. |
| `org` | string | Yes | Organization the consumer belongs to. |
| `api_keys` | list of strings | No | API keys assigned to this consumer. |
| `rate_limit` | number | No | Rate limit override in requests per minute. |
| `metadata` | map of strings | No | Key-value pairs for labeling. |

#### Attributes

| Attribute | Description |
|-----------|-------------|
| `id` | Internal identifier assigned by the control plane. |
| `created_at` | Timestamp when the consumer was created. |
| `updated_at` | Timestamp of the last update. |

---

## Import

Existing resources can be imported into Terraform state using `terraform import`. This will be fully supported in a future release.

```bash
terraform import flexgateway_route.get_users get-users
terraform import flexgateway_backend.users_api users-api
terraform import flexgateway_consumer.acme acme-corp
```

The import identifier is the resource `name` as it appears in the control plane.

## State Management

The provider stores resource state in your Terraform state file. When the full CRUD implementation ships, `terraform plan` will detect drift between your HCL configuration and the live control plane state, similar to how `fgctl drift` works for YAML manifests.
