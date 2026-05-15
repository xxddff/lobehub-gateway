# Go Agent Gateway

[简体中文](./README.zh-CN.md)

`agent-gateway-go` is a Go implementation of the LobeHub Agent Gateway core protocol.

The reference Agent Gateway is implemented with Cloudflare Workers and Durable Objects. This project will reimplement the same gateway role as a single-instance, platform-neutral Go service with no external runtime dependencies.

## Scope

- Manage browser WebSocket sessions for Agent conversations.
- Route streaming Agent events from backend services to connected clients.
- Forward user input, tool confirmation, and interrupt messages from clients to backend services.
- Support service-token and JWT-based authentication where required by the protocol.
- Keep runtime state in memory for a single gateway instance.
- Keep the core `/api/operations/*` API and `/ws` WebSocket protocol compatible with the reference Agent Gateway.

## Deliberate Trade-offs

- Single process only. Backend API requests and browser WebSocket connections for the same `operationId` must reach the same instance.
- Runtime state is in memory. Operations, buffered events, pending requests, metrics, and errors are lost on restart.
- Admin APIs are intentionally not implemented. This includes `/api/admin/*`, `/admin/ws`, `ADMIN_TOKEN`, admin stats, admin metrics, admin errors, and admin observer WebSockets.
- Durable Object storage, alarms, and WebSocket hibernation are replaced by in-process maps and timers.

## API Compatibility

Implemented routes:

- `GET /health`
- `GET /ws?operationId={operationId}`
- `POST /api/operations/init`
- `POST /api/operations/push-event`
- `POST /api/operations/push-events`
- `POST /api/operations/update-status`
- `POST /api/operations/request-confirmation`
- `POST /api/operations/request-input`
- `POST /api/operations/tool-execute`
- `GET /api/operations/status?operationId={operationId}`

All operations APIs require `Authorization: Bearer {SERVICE_TOKEN}`.

## Configuration

Environment variables:

- `SERVICE_TOKEN` required.
- `JWKS_PUBLIC_KEY` required for JWT WebSocket auth.
- `LOBE_API_BASE_URL` optional, defaults to `https://app.lobehub.com`.
- `PORT` optional, defaults to `8787`.
- `READ_TIMEOUT`, `WRITE_TIMEOUT`, `SHUTDOWN_TIMEOUT` optional Go durations.

## Planned Direction

The Go implementation is expected to follow the same repository principles as `device-gateway-go`:

- Standard Go HTTP/WebSocket server.
- No Cloudflare Workers or Durable Objects dependency.
- No Redis, PostgreSQL, NATS, or other external runtime services.
- Compatible public API and WebSocket behavior where practical.

## Development

```bash
go test ./...
go run ./cmd/agent-gateway-go
```
