# Webhook Plugin

A gRPC plugin that sends POST notifications to a configured URL whenever a request arrives. The webhook call is fire-and-forget -- the request continues to the backend regardless of whether the webhook POST succeeds or fails.

## How It Works

1. Receives the request via the gRPC `OnRequest` RPC
2. Spawns a goroutine to POST a JSON payload to the webhook URL
3. Returns `ACTION_CONTINUE` immediately so the request is not blocked
4. The `OnResponse` RPC is a no-op (returns `ACTION_CONTINUE`)

## Webhook Payload

```json
{
  "method": "POST",
  "path": "/api/users",
  "timestamp": "2026-01-15T10:30:00Z",
  "headers": {
    "Content-Type": "application/json",
    "Authorization": "Bearer ..."
  }
}
```

## Configuration

Set via environment variables when running the plugin process:

| Variable | Default | Description |
|----------|---------|-------------|
| `WEBHOOK_URL` | (required) | Target URL for POST notifications |
| `PLUGIN_PORT` | `50051` | gRPC listen port |

## Build

```bash
go build -o webhook-plugin ./examples/plugins/webhook/
```

## Run

```bash
WEBHOOK_URL=https://hooks.example.com/events PLUGIN_PORT=50051 ./webhook-plugin
```

Then register in the gateway config with the gRPC address `localhost:50051`.

## Test

```bash
go test -race -count=1 ./examples/plugins/webhook/... -v
```
