# Go Agent Gateway

[English](./README.md)

`agent-gateway-go` 是 LobeHub Agent Gateway 核心协议的 Go 实现。

参考版 Agent Gateway 基于 Cloudflare Workers 与 Durable Objects 实现。本项目将用单实例、平台无关、无外部运行时依赖的 Go 服务重新实现相同的网关职责。

## 范围

- 管理 Agent 会话的浏览器 WebSocket 连接。
- 将后端服务产生的 Agent 流式事件路由到已连接客户端。
- 将用户输入、工具确认和中断消息从客户端转发给后端服务。
- 按协议需要支持 service-token 与 JWT 认证。
- 面向单网关实例，将运行时状态保存在内存中。
- 保持核心 `/api/operations/*` API 与 `/ws` WebSocket 协议兼容参考版 Agent Gateway。

## 明确取舍

- 只支持单进程。相同 `operationId` 的后端 API 请求和浏览器 WebSocket 必须到达同一个实例。
- 运行时状态保存在内存中。重启后 operation、事件缓冲、pending 请求、metrics 和 errors 都会丢失。
- 不实现 admin 模块。包括 `/api/admin/*`、`/admin/ws`、`ADMIN_TOKEN`、admin stats、admin metrics、admin errors 和 admin observer WebSocket。
- Durable Object storage、alarm 和 WebSocket hibernation 使用进程内 map 与 timer 替代。

## API 兼容范围

已实现路由：

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

所有 operations API 都要求 `Authorization: Bearer {SERVICE_TOKEN}`。

## 配置

环境变量：

- `SERVICE_TOKEN` 必填。
- `JWKS_PUBLIC_KEY` 用于 JWT WebSocket 认证。
- `LOBE_API_BASE_URL` 可选，默认 `https://app.lobehub.com`。
- `PORT` 可选，默认 `8787`。
- `READ_TIMEOUT`、`WRITE_TIMEOUT`、`SHUTDOWN_TIMEOUT` 可选，使用 Go duration 格式。

## 计划方向

Go 实现预计会沿用 `device-gateway-go` 的仓库原则：

- 标准 Go HTTP/WebSocket 服务。
- 不依赖 Cloudflare Workers 或 Durable Objects。
- 不依赖 Redis、PostgreSQL、NATS 或其他外部运行时服务。
- 在可行范围内保持公开 API 与 WebSocket 行为兼容。

## 开发

```bash
go test ./...
go run ./cmd/agent-gateway-go
```
