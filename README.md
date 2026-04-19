# gochen-llm

`gochen-llm` 是基于 gochen 的可复用 LLM 领域模块/组件集合，用于在业务服务中统一管理多 Provider 端点配置、健康探测与请求分流等能力。

## 工程效率

- 代码检索/重复扫描：统一忽略 `.cache/.gocache`（仓库提供 `.ignore`；若你使用不读取 ignore 文件的工具，请在命令中显式加 `--ignore-dirs .cache,.gocache`）



## 管理接口契约

`gochen-llm/router` 对外暴露的管理/指标接口统一使用 gochen `httpx` `ResponseMessage`：

- 顶层固定为 `{ code, message, data }`；不再返回裸对象。
- `GET /admin/llm/config`：`data.configs` 为当前生效的 provider 配置列表。
- `GET /admin/llm/metrics`：
  - 默认聚合返回 `data.report`；
  - 当 `group_by=variant&ab_test_id=...` 时返回 `data.variants`。
- `GET /admin/llm/metrics/list`：返回 `data = { total, list, limit, offset }`。
- `GET /admin/llm/security/overview`：返回 `data = { policy, rate_limit, updated_at }`，其中 `rate_limit` 包含当前限流配置与最近窗口统计。
- 只返回确认语义的写接口（如 reload / convert 等）使用顶层 `message` 表达结果，不再把旧的 `status/message` 再塞进 `data`。

## 排障约定

- 客户端上游 HTTP 错误会在 `errors` 上下文里保留真实 `provider`（例如 `openai`、`openai_compatible`、`gemini`、`anthropic`），便于多 provider fallback 下的日志、指标与告警排查。
