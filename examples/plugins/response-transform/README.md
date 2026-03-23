# Response Transform Plugin

A native Go plugin that modifies API responses by adding headers and data fields. Useful for injecting metadata like processing timestamps, version identifiers, or trace IDs.

## How It Works

1. Passes the request through to the backend unchanged
2. After receiving the backend response, adds any configured headers
3. Adds any configured fields to the response data map
4. Preserves the original status code and existing headers/data

## Configuration

```json
{
  "add_headers": {
    "X-Transformed-By": "flex-gateway",
    "X-Api-Version": "2.1"
  },
  "add_fields": {
    "transformed_at": "2026-01-01T00:00:00Z",
    "gateway_version": "1.0.0"
  }
}
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `add_headers` | map[string]string | `{}` | Headers to add to every response |
| `add_fields` | map[string]any | `{}` | Fields to add to every response's data |

## Build

```bash
go build -o response-transform ./examples/plugins/response-transform/
```

## Test

```bash
go test -race -count=1 ./examples/plugins/response-transform/... -v
```

## Usage

After configuring the plugin, responses will include the additional headers and fields:

```
HTTP/1.1 200 OK
X-Transformed-By: flex-gateway
Content-Type: application/json

{
  "data": "original response",
  "transformed_at": "2026-01-01T00:00:00Z"
}
```
