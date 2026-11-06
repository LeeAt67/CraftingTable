# CraftingTable Agent Tool Runtime Design

**Status:** Approved for implementation

**Date:** 2026-11-06

**Scope:** AG-01 Tool Registry、Schema、ToolCall / ToolResult、执行器、持久化，以及三个 Today 只读工具

**Research baseline:** [AG-01 工具运行时深度调研](../../research/2026-11-06-ag-01-tool-runtime-deep-dive.md)

## 1. Goal

让 Agent 能在一次真实模型 Run 中完成以下闭环：

```text
Model Step
  -> ToolCall
  -> Schema validation
  -> Tool execution
  -> ToolResult persistence
  -> ToolResult回填模型上下文
  -> Model Step
  -> Final answer
```

首批工具固定为：

- `today_search`
- `today_get_item`
- `today_summary`

三个工具都只读，不访问外部网络，不执行 Shell，不写工作台数据。

## 2. Why a Minimal Follow-up Loop Is Included

AG-01 单独只能执行工具，不能把结果送回模型。为了让三个工具在真实 API 下可用，本次同时补一条最小工具续轮：

- 最多 4 个 Model Step。
- 最多 8 个 ToolCall。
- 工具串行执行。
- 不自动重试工具。
- 工具业务失败作为 `isError=true` 的 ToolResult 回填模型。
- 超出步数或工具数时 Run 失败，错误码稳定且可重试。

完整的停止预算、失败分类、审批暂停和工具 UI 仍属于 AG-02 / AG-03，不在本次实现。

## 3. Non-goals

- 不实现动态工具注册、MCP、Skills、Subagent 或 Multi-Agent。
- 不实现 Shell、文件系统、写数据库或工作台写操作工具。
- 不实现人工审批和沙箱。
- 不实现工具调用卡片、参数展开或工具时间线 UI。
- 不实现供应商归一化；只扩展现有 OpenAI-compatible Adapter。
- 不实现 Tool Result 图片、文件或 Artifact。

## 4. Architecture

```text
Composition Root
  -> today SQLite Reader
  -> Today Tool adapters
  -> immutable ToolRegistry
  -> ToolExecutor
  -> Agent Service / Coordinator
       -> OpenAI-compatible Provider
       -> SQLiteStore
       -> Event Journal
```

职责：

- `today` 提供只读查询能力和领域校验，不依赖 Agent。
- `agent/tools` 将 Today Reader 适配为 Agent Tool，拥有 Schema 和结果裁剪。
- `agent/core` 拥有 Registry、Executor、Provider 协议、Run 循环和错误映射。
- `SQLiteStore` 拥有 Tool Step 与 Tool Invocation 事务。
- `access` 只继续输出现有 text / reasoning UI stream；工具执行默认不展示。

## 5. Domain Contracts

### 5.1 ToolDefinition

```go
type ToolDefinition struct {
    Name           string
    Namespace      string
    Version        string
    Title          string
    Description    string
    InputSchema    json.RawMessage
    ReadOnly       bool
    Idempotent     bool
    Timeout        time.Duration
    MaxInputBytes  int
    MaxResultBytes int
}
```

约束：

- Name 最长 64 字符，只允许 `[A-Za-z0-9_-]`。
- Version 为非空稳定字符串。
- InputSchema 顶层必须是 object，且 `additionalProperties: false`。
- Registry 启动时编译全部 Schema；任何失败阻止服务启动。
- Catalog 按 Name 排序，Hash 在启动后稳定。

### 5.2 ToolCall

```go
type ToolCall struct {
    ID              string
    ProviderCallID  string
    RunID           string
    StepID          string
    Name            string
    Version         string
    Arguments       json.RawMessage
    ArgumentsHash   string
    Status          ToolCallStatus
    CreatedAt       string
    StartedAt       string
    FinishedAt      string
}
```

`ID` 由服务端生成。`ProviderCallID` 必须来自 Provider 且在一次 Run 内唯一。

### 5.3 ToolResult

```go
type ToolResultContent struct {
    Type string
    Text string
    JSON any
}

type ToolResult struct {
    CallID      string
    Content     []ToolResultContent
    IsError     bool
    Error       *ToolError
    Truncated   bool
    EncodedBytes int
    CreatedAt   string
}
```

首版只支持 `text` 和 `json` content block。

### 5.4 ToolError

```go
type ToolError struct {
    Code      string
    Kind      string
    Message   string
    Retryable bool
    Hint      string
}
```

稳定 Kind：

- `validation`
- `not_found`
- `timeout`
- `cancelled`
- `policy`
- `upstream`
- `internal`

Handler panic、无效结果编码和未预期错误统一映射为 `internal`。

## 6. Registry and Executor

### 6.1 Registry

`ToolRegistry` 是不可变结构：

- 构造时拒绝重复 Name。
- 构造时编译 JSON Schema。
- `Catalog()` 返回稳定排序的 definitions。
- `ProviderTools()` 返回 OpenAI-compatible function definitions。
- `Resolve(name, version)` 精确匹配版本。
- `Validate(name, version, arguments)` 返回规范化 JSON 或 ToolError。

使用 `github.com/santhosh-tekuri/jsonschema/v6` 编译 Draft 2020-12 Schema。Compiler 使用自定义 loader，任何外部 `$ref` 都失败，不允许文件或网络加载。

### 6.2 Executor

执行顺序：

```text
context
  -> enforce input bytes
  -> validate schema
  -> invoke handler
  -> recover panic
  -> bound result
  -> return ToolResult
```

规则：

- Handler 不执行直到 Schema 校验通过。
- 每个 ToolCall 使用独立 timeout，且不能超过 Run Context 剩余时间。
- `context.Canceled` 和 Run deadline 继续向上传播，不伪装成 ToolResult。
- Tool 自身的 timeout 返回 `isError=true`。
- 未知工具、非法参数和业务错误返回 `isError=true`。
- 结果超过 `MaxResultBytes` 时替换为截断摘要，并设置 `Truncated=true`。

## 7. Provider Protocol

### 7.1 Request

```go
type ModelRequest struct {
    Messages   []ProviderMessage
    Tools      []ProviderToolDefinition
    ToolChoice string
}
```

首版只支持 `ToolChoice: "auto"`。

`ProviderMessage` 扩展：

```go
type ProviderMessage struct {
    Role       string
    Content    string
    ToolCalls  []ProviderToolCall
    ToolCallID string
}
```

### 7.2 Streaming parser

Parser 按 `index` 聚合 `delta.tool_calls`：

- 首次分片设置 ID、type 和 function name。
- 后续分片拼接 arguments 字符串。
- 所有分片结束后统一解析 JSON。
- 缺少 ID/name、arguments 非法或重复字段冲突视为协议错误。
- 多 ToolCall 保持 Provider index 顺序。

终止映射：

- `finish_reason=stop` -> final answer。
- `finish_reason=tool_calls` -> 当前 Model Step 完成，执行工具。
- `finish_reason=length` -> Run 失败 `AGENT_OUTPUT_TRUNCATED`。
- 未知 finish reason -> 协议错误。
- 无 finish reason 的 `[DONE]` 保持兼容；存在完整 ToolCall 时按 tool_calls 处理。

### 7.3 Follow-up messages

工具结果回填顺序：

```text
assistant message with tool_calls
tool message 1
tool message 2
...
next model request
```

Tool message 使用同一个 `providerCallId`。模型只看到 ToolResult 内容或稳定错误摘要，不看到 Handler stack、SQL、内部路径或完整异常。

## 8. Persistence

### 8.1 Migration

新增 `005_agent_tools.sql`：

- `agent_steps.kind`，默认 `model`，允许 `model` / `tool`。
- `agent_steps.parent_step_id`。
- `agent_steps.error_code`。
- `agent_tool_invocations`。

### 8.2 Step

每次 Model 调用或 Tool 调用都有独立 Step：

- 初始 Model Step 继续使用 `step_index=0`。
- Tool Step 使用 Run 内下一递增 index。
- 工具 Step 的 `parent_step_id` 指向发起调用的 Model Step。
- 后续 Model Step 使用下一 index。

Run Finalize / Interrupt 更新所有仍为 `running` 的 Step。

### 8.3 Tool Invocation

`agent_tool_invocations` 字段：

- `id`
- `step_id`
- `run_id`
- `provider_call_id`
- `tool_name`
- `tool_version`
- `arguments_json`
- `arguments_hash`
- `result_json`
- `result_is_error`
- `result_truncated`
- `encoded_bytes`
- `created_at`
- `updated_at`

约束：

- `step_id` 唯一。
- `(run_id, provider_call_id)` 唯一。
- Tool Step 与 Invocation 在同一事务创建。
- Invocation 完成与 Step 完成在同一事务提交。

## 9. Today Tools

### 9.1 today_search

输入：

```json
{
  "date": "YYYY-MM-DD",
  "timezone": "Asia/Shanghai",
  "query": "标题或描述关键字",
  "status": "unplanned | planned | done",
  "categoryId": "category-id",
  "limit": 20
}
```

`date` 和 `timezone` 可省略，省略时按服务器当前时区计算今天。`query` 对标题和描述做不区分大小写的包含匹配。`limit` 默认 20，最大 50。

结果包含精简条目：ID、标题、状态、分类、开始时间、结束时间和完成时间。不返回完整描述。

### 9.2 today_get_item

输入：

```json
{ "id": "item-id" }
```

返回完整 Item，包括标题、描述、状态、分类和时间。已删除条目返回 `not_found`。

Today Repository 新增公开只读方法 `GetItem(ctx, id)`，SQLite 复用现有 `getItem`。

### 9.3 today_summary

输入：

```json
{
  "date": "YYYY-MM-DD",
  "timezone": "Asia/Shanghai"
}
```

返回：

- 总条目数。
- done / planned / unplanned 数量。
- 完成率。
- 已安排时长，按 startAt / endAt 的区间求和。
- 按分类聚合的条目数和完成数。

本次不计算专注时长；T-07 口径未确认前不把 event 数据混入 summary。

## 10. Errors

新增稳定 Agent 错误：

- `AGENT_TOOL_LIMIT_EXCEEDED`
- `AGENT_MODEL_STEP_LIMIT_EXCEEDED`
- `AGENT_TOOL_CALL_INVALID`
- `AGENT_TOOL_EXECUTION_FAILED`
- `AGENT_OUTPUT_TRUNCATED`

Tool 业务错误不直接失败 Run，而是作为 ToolResult 回填。只有协议错误、持久化错误、Run 取消、步数/工具数超限和不可恢复内部错误会失败 Run。

## 11. Configuration

新增环境变量：

- `CRAFTINGTABLE_AGENT_TOOLS_ENABLED`
  - 默认 `true`。
  - `false` 时不向模型发送工具，恢复纯文本行为。
- `CRAFTINGTABLE_AGENT_MAX_MODEL_STEPS`
  - 默认 `4`。
- `CRAFTINGTABLE_AGENT_MAX_TOOL_CALLS`
  - 默认 `8`。

非法配置阻止服务启动。

## 12. Testing

Unit：

- Registry 重复名、Schema 编译、稳定排序、版本解析。
- Executor 校验先于 Handler、timeout、panic、未知工具、结果截断。
- Parser tool_call 分片、多调用、非法 JSON、finish reason。
- Today 三工具参数、过滤、结果和 summary 口径。

Integration：

- Fake Provider 第一轮返回 `today_search`，第二轮返回最终回答。
- Tool Step / Invocation / Result 落库。
- `today_get_item` 和 `today_summary` 可在 Fake Provider 闭环中执行。
- 工具错误回填后模型仍能生成回答。
- 步数/工具数超限产生稳定失败。
- 纯文本与 Reasoning 测试不回归。

Frontend：

- 不新增工具 UI。
- 现有 text / reasoning 流测试必须继续通过；未知附加 API 字段不能破坏 Zod 解析。

## 13. Rollout

1. 先落 Registry / Executor 与 Today Tool unit tests。
2. 再落 Provider parser 与 request mapping。
3. 再落 migration 和 Step / Invocation transaction。
4. 再落最小续轮。
5. 最后接 composition root 与 integration tests。

任何一步失败都不得通过关闭测试绕过；工具开关只能作为运行时回退，不是测试替身。
