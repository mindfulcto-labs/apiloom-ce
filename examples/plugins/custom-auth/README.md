# Custom Auth Plugin

A native Go plugin that validates custom token authentication. Checks for a configurable prefix followed by base64-encoded data in the `X-Custom-Token` or `Authorization` header.

## How It Works

1. Extracts a token from `X-Custom-Token` (preferred) or `Authorization` header
2. Validates the token starts with the configured prefix (default `flex-`)
3. Optionally validates the payload after the prefix is valid base64
4. Returns 401 if the token is missing, has the wrong prefix, or fails base64 validation
5. Passes the request through to the backend if validation succeeds

## Configuration

```json
{
  "token_prefix": "flex-",
  "require_base64": true
}
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `token_prefix` | string | `flex-` | Required prefix on the token value |
| `require_base64` | bool | `false` | Whether to validate the payload as base64 |

## Build

```bash
go build -o custom-auth ./examples/plugins/custom-auth/
```

## Test

```bash
go test -race -count=1 ./examples/plugins/custom-auth/... -v
```

## Usage

Send a request with a valid token:

```bash
curl -H "X-Custom-Token: flex-dXNlcjpzZWNyZXQ=" http://localhost:8080/api/resource
```

A missing or invalid token returns a 401 ProblemError:

```json
{
  "type": "urn:flexgateway:error:missing-token",
  "title": "Unauthorized",
  "status": 401,
  "detail": "request must include a token in X-Custom-Token or Authorization header"
}
```
