# gochen-llm

`gochen-llm` 是基于 gochen 的可复用 LLM 领域模块/组件集合，用于在业务服务中统一管理多 Provider 端点配置、健康探测与请求分流等能力。

## 工程效率

- 代码检索/重复扫描：统一忽略 `.cache/.gocache`（仓库提供 `.ignore`；若你使用不读取 ignore 文件的工具，请在命令中显式加 `--ignore-dirs .cache,.gocache`）

