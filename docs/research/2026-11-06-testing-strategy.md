# 测试策略调研：边界覆盖、框架、书写规范与正确性验证

日期：2026-11-06
状态：调研结论（未进入设计）
范围：Vitest 单测、Go 单元与集成测试、Playwright E2E 的职责划分、边界覆盖方法、框架组织、书写规范与测试自身正确性的验证
排除：性能测试、安全测试、Agent Evaluation Platform
关联需求：[Q-05](../requirements.md)、[Q-06](../requirements.md)
依据：本轮实测数据（`go test -cover`、`go vet`、`vitest`、静态扫描）、[工程化设计规范](../superpowers/specs/2026-11-05-craftingtable-engineering-design.md)、[非 Agent 工作台需求调研](./2026-11-05-non-agent-workbench-requirement-research.md)

> 本文回答四个问题：边界怎么才算覆盖全、框架怎么搭、用例怎么写、写完怎么知道它是对的。所有数字都来自本仓库当前提交 `a929464`。

## 1. 结论

四条结论，按重要性排序。

**第一，"覆盖所有边界"不是一个可以达成一次的状态，而是一套枚举方法。** 靠"看看哪些文件没测试"永远漏，因为一个文件可以既被测过又漏掉关键分支。本轮先给出可执行的边界分类法（§4.2），再用变异测试反向验证覆盖是否真实（§7.3）。

**第二，当前最大的测量问题是集成测试贡献零覆盖率。** 集成测试用 `exec.Command` 启动独立进程，`go test -cover` 统计不到子进程。所以 `store`、`httpapi`、`provider`、`domain` 显示 0%，但它们**确实被集成测试跑到了**。现在的覆盖率数字既低估真实覆盖，也无法指导补测。修法是 `go build -cover` + `GOCOVERDIR`（§5.4）。

**第三，Playwright 的 trace 从未被记录。** 配置写了 `trace: "on-first-retry"`，但没有配置 `retries`，默认值为 0，即永远不会有第一次重试。失败时拿不到 trace，只能靠日志猜。

**第四，八条规范里只有两条被普遍遵守。** 18 个 Go 测试文件里只有 2 个用表驱动、4 个用 `t.Parallel()`。不是写法错，是没有约定。

## 2. 现状实测

### 2.1 规模

| 层 | 文件 | 用例 | 断言 |
| --- | --- | --- | --- |
| Vitest（`tests/e2e/unit`） | 19 | 100 | 269 处 `expect()` |
| Go 测试（`server/` + `tests/`） | 27 | 122 个 `func Test` | — |
| Playwright（`tests/e2e/modules`） | 10 | 11 | — |

### 2.2 Go 覆盖率（`go test ./server/... -cover`）

| 包 | 覆盖率 | 说明 |
| --- | --- | --- |
| `agent/tools` | 65.9% | 本轮新增，最高 |
| `today` | 41.1% | 含 `query.go`、`timeframe.go` |
| `agent/core` | 35.8% | |
| `cmd/craftingtabled` | 16.5% | |
| `agent/access` | 1.1% | HTTP 层，主要靠集成测试 |
| `platform/store` | **0.0%** | 仅被集成测试的独立进程覆盖 |
| `platform/httpapi` | **0.0%** | 同上 |
| `agent/provider` | **0.0%** | 只有 `openai_provider_test.go` 在 `tests/` 下，不在包内 |
| `agent/domain` | **0.0%** | 纯类型，但 `error.go` 有逻辑 |

0% 不等于没跑。这是测量口径问题，不是质量问题（§5.4 给出修法）。

### 2.3 工具链缺口

| 项目 | 状态 | 影响 |
| --- | --- | --- |
| CI（`.github/workflows`） | 不存在 | 测试只在本地跑，无法防止回归合入 |
| `golangci-lint` | 未配置 | 只有 `go vet` |
| ESLint | 未配置 | 前端无静态检查，只有 `tsc` |
| `@vitest/coverage-v8` | 未安装 | 前端覆盖率无法测量 |
| `go test -race` | **可用**（已实测） | 未纳入脚本 |
| `go build -cover` | **可用**（已实测） | 未用于集成测试 |
| Playwright `retries` | 未配置（默认 0） | `trace: on-first-retry` 永不生效 |
| Playwright `workers` / `fullyParallel` | 1 / false | 测试串行，全量约 16 s；可接受但无并行收益 |

### 2.4 写法一致性

| 规范项 | 遵守情况 |
| --- | --- |
| 表驱动（`tests := []struct`） | 2 / 18 个 Go 文件 |
| 子测试（`t.Run`） | 2 / 18 |
| 并行（`t.Parallel()`） | 4 / 18 |
| 时间可注入 | 部分：`tools` 收 `now func() time.Time`；`today` 控制器直接 `time.Now()` |

## 3. 三层测试的职责边界

原则：**越靠下越快、越精确；越靠上越真实、越慢。每个断言只出现在它能最便宜地失败的那一层。**

| 检查什么 | 层 | 理由 |
| --- | --- | --- |
| 纯函数：排序、计数、时长、时区换算 | Go / TS 单测 | 无 I/O，毫秒级，边界可任意构造 |
| 状态机：Run 生命周期、流协议、Store 转换 | TS 单测 | 非法转移最容易在这一层枚举 |
| SQL 语义：软删过滤、迁移、索引 | Go 集成 | 需要真实 SQLite |
| HTTP 契约：状态码、错误码、JSON 形状 | Go 集成 | 需要真实路由与中间件 |
| 跨进程恢复、幂等、心跳 | Go 集成 | 需要真实进程与时钟 |
| 用户可见流程：创建、完成、刷新保留 | Playwright | 需要真实浏览器 |
| 响应式、无横向滚动 | Playwright | 需要真实布局引擎 |
| 无障碍名称、焦点 | Playwright + 组件测试 | 名称可单测，焦点顺序需真实 DOM |

判定口诀：**能用纯函数测的不要用集成测，能用集成测的不要用 E2E 测。** E2E 只保留"跨层契约"和"浏览器特有行为"两类。

对照本仓库的具体错配：

| 现状 | 问题 | 应为 |
| --- | --- | --- |
| 完成/取消完成只有 E2E 覆盖 | 分支逻辑放错层，每次验证要起三个进程 | 补 `todayStore` + 页面组件单测 |
| `query.go` 的负数时长保护无单测 | 纯函数却没测 | Go 单测 |
| `today_tools.go` 的错误路径无测试 | 契约映射，单测成本极低 | Go 单测（stub 已存在） |
| 主题 DOM 同步只在 E2E | `useThemeEffect` 是纯 DOM 效果 | jsdom 单测即可 |

## 4. 边界怎么才算覆盖全

### 4.1 先否定一种做法

上一轮我用"哪些文件没被 import"来判断覆盖率，结论是错的：`todayStore` 被 E2E 覆盖却判定为"空白"，`apiModel` 测了 1 条却判定为"已测试"。**文件级清单无法区分"测过"和"测全"。**

正确做法是按**分支点**枚举，并给每类边界一个明确的构造方式。

### 4.2 边界分类法（八类，逐类套用）

对每个被测单元，按下表逐类提问。每一类即使结论是"不适用"，也要走一遍。

| 类别 | 问什么 | 本仓库实例 |
| --- | --- | --- |
| B1 空与缺省 | 空集合、`nil`、`undefined`、空字符串、字段缺失时是什么行为 | `items=[]` 时 `CompletionRate` 是否除零；`Description == nil` 是否空指针 |
| B2 单元素与多元素 | 0 / 1 / 2 / N 的行为差异 | `SearchItems` limit 0 → 20，limit 1000 → 50；分类统计 0/1/2 个 |
| B3 边界值 | 上界、下界、恰好相等、刚越界 | `limit=1`、`limit=50`、`limit=51`；`endAt == startAt` 是否算有效 |
| B4 非法输入 | 类型错、格式错、超长、超限、未知枚举 | `date=2026-13-01`；`tz=Mars/Olympus`；JSON 无法反序列化 |
| B5 状态机非法转移 | 终态再来事件、跳步、回退 | `agentStreamReducer`：第二次终态事件必须拒绝；游标不回退 |
| B6 错误传播 | 依赖返回错误时的映射与保留 | `ListCategories` 失败是否连累条目加载；`internalError` 是否吞错 |
| B7 时间与时区 | 跨日、夏令时、时区偏移、时钟回拨 | `America/New_York` 2026-03-08 为 23 小时；`+08:00` 规范化 |
| B8 并发与时序 | 竞态、重入、陈旧响应、重复提交 | 「陈旧会话响应不覆盖最新选择」；`pending` 期间重复点击 |

再补两类**跨切面**检查，只在涉及对应能力时才用：

| 类别 | 问什么 |
| --- | --- |
| B9 国际化与展示 | 长文本截断、复数、日期格式、`Intl` 抛错（非法日期串会抛 `RangeError`） |
| B10 资源与上限 | 超大输入、超时、字节上限、分页游标耗尽 |

### 4.3 覆盖判定的实操形式

对每个被测文件产出一张**分支清单表**，作为补测的工单，也作为评审依据。格式：

| 分支位置 | 类别 | 期望行为 | 用例 | 状态 |
| --- | --- | --- | --- | --- |
| `query.go` `scheduledSeconds` 的 `!end.After(start)` | B3/B4 | 不计入时长 | — | 缺失 |
| `todayStore.setError` | B6 | 保留既有 items | — | 缺失 |

清单放在需求文档的子表或独立文件里，随代码走。**没有清单就没有"覆盖全"的判断依据**，只有"我觉得差不多了"。

### 4.4 什么时候算够

用三个下界替代"100%"这种不可达目标：

1. **每个导出的纯函数至少一个 `B1`~`B4` 用例。**
2. **每个 `if` / `case` / `?:` 分支至少被一个用例经过**（分支覆盖率，不是行覆盖率）。
3. **每个 `return err` 路径至少一个用例**，验证错误码与可重试性。

这三条可以在 CI 里用覆盖率报告近似检查，但真正的确认靠 §7.3 的变异测试。

## 5. 框架怎么搭

### 5.1 目录

现状已基本合理，只需两处调整：

```text
tests/e2e/
  unit/            Vitest：纯函数、Store、Effect、组件
    agent/
    today/         ← 新增：todayStore、useTodayEffect、summary 等
    models/
    theme/
  modules/         Playwright：按用户域分目录
  integration/     Go：真实进程 + 真实 SQLite
  support/         E2E 共享断言与数据构造
server/internal/<pkg>/*_test.go    Go 单元测试与被测代码同包
```

原则：**Go 单测与被测代码同包**（可测未导出符号），**跨包集成测试放 `tests/e2e/integration`**。当前 `tests/e2e/unit/agent/*_test.go` 放着两个 Go 测试，与 Vitest 混在同一目录，建议移至包内或 `tests/e2e/integration`。

### 5.2 时间必须可注入

`tools` 包已经做对（`now func() time.Time`），`today` 控制器没有：

```go
// controller.go
dayRange, message := ResolveDayRange(query.Get("date"), query.Get("tz"), time.Now())
```

这导致 HTTP 层无法测"跨日边界"和"时钟回拨"。最小改法：`Controller` 增加 `now func() time.Time` 字段，`NewController` 默认 `time.Now`，测试注入固定时钟。前端同理：涉及 `new Date()` 的组件改为接收 `now` 或使用可 mock 的模块。

### 5.3 测试数据构造

现状：`today.integration_test.go` 里手写 JSON 字符串，`uniqueTitle()` 用时间戳加随机数。两个改进方向：

- **Go**：给 `today.Item` 加一个测试构建器（`itemBuilder`，可选字段逐个设置），避免每次写全结构体。这比引入第三方库更轻。
- **TS**：在 `tests/e2e/support` 增加 `makeItem(partial)`，覆盖 `status`、`startAt`、`endAt` 的常用组合。
- **不要**共享可变夹具。当前每个用例 `testkit.OpenSQLite` 都建新库，这是对的，保持。

### 5.4 让集成测试贡献覆盖率

这是本轮最有价值的单点改进。步骤：

```bash
# 1. 用 -cover 构建被测二进制
go build -cover -o bin/craftingtabled ./server/cmd/craftingtabled

# 2. 指向独立目录，进程退出时写出覆盖率文件
export GOCOVERDIR="$(pwd)/coverage/raw"
mkdir -p "$GOCOVERDIR"

# 3. 跑集成测试（support_test.go 里构建命令换成上面的形式）
go test ./tests/e2e/integration/ -count=1

# 4. 合并并生成报告
go tool covdata percent -i "$GOCOVERDIR"
go tool covdata textfmt -i "$GOCOVERDIR" -o coverage/integration.out
go tool cover -func coverage/integration.out
```

已实测 `go build -cover` 在本机可用（Go 1.26.5）。改造点在 `support_test.go` 的 `TestMain`：构建参数加 `-cover`，启动子进程的环境变量加 `GOCOVERDIR`。

这一步做完，`store`、`httpapi`、`provider` 的覆盖率才会从 0% 变成真实值，否则任何"覆盖率阈值"都是假门禁。

### 5.5 E2E 的稳定性设置

| 项 | 现状 | 建议 |
| --- | --- | --- |
| `retries` | 未配置（0） | CI 设 2，本地 0；否则 `trace: on-first-retry` 不生效 |
| `trace` | `on-first-retry` | 配合 `retries` 才有效；或本地改 `retain-on-failure` |
| `workers` | 1 | 数据隔离做好后可提高；当前每个用例用 `uniqueTitle` 建独立数据，可并行 |
| 失败产物 | 无 | 增加 `screenshot: "only-on-failure"`、`video: "retain-on-failure"` |
| `testIgnore` | 排除 `reasoning.e2e.ts` | 该文件存在但从不运行，属于静默失效。要么修好纳入，要么删除并登记原因 |

最后一条是当前最隐蔽的问题：一个存在的测试文件被配置排除，任何人看目录都会以为它跑着。

## 6. 规范性书写

### 6.1 命名

用例名必须说明**场景 + 期望**，不写"应该正常工作"。

| 差 | 好 |
| --- | --- |
| `test create item` | `creates an unplanned task when no start time is given` |
| `test error` | `returns NOT_FOUND and keeps the original deletedAt on repeated delete` |
| `TestSearch` | `TestSearchItemsClampsLimitToDefaultAndMaximum` |

现状：Vitest 侧命名质量高（`rejects a cursor that does not advance`），Go 侧中等（`TestStatusForStart`），集成侧偏泛（`TestTodayAPIRoundTrip`）。

### 6.2 一个用例只验证一个行为

判据：如果断言失败，能否一句话说出哪里坏了。当前反例：

```go
// today.integration_test.go 的一个用例同时断言
// 状态码 + 标题 + 描述 + 分类
if status != http.StatusOK || updated.Data.Title != "..." || *updated.Data.Description != "..." {
```

一个 `t.Fatalf` 覆盖四种失败，排查时要重新跑。改法：拆成 table 或分段断言并各自带消息。

### 6.3 表驱动与子测试

Go 侧只有 2/18 用表驱动，但 `timeframe_test.go` 已经是很好的样板（`t.Run` + 并行 + 明确的 `want`）。建议：一个测试里出现 3 个以上同类分支时，必须转表驱动。

### 6.4 断言的强度

禁止只用真值断言。当前 `apiModel.test.ts` 只有一个 `rejects.toThrow("服务暂时不可用")`，它无法区分"服务端错误"和"响应体解析失败"。

| 弱 | 强 |
| --- | --- |
| `expect(x).toBeTruthy()` | `expect(x).toEqual({...})` |
| `expect(fn).toThrow()` | `expect(fn).rejects.toThrow("具体文案")` |
| `if status != 200 { Fatal }` | 加上 `error.Code`、`error.Kind`、`error.Retryable` 三项 |

### 6.5 禁止的写法

| 禁止 | 原因 | 替代 |
| --- | --- | --- |
| `time.Sleep` 等异步 | 慢且不稳 | 用 channel、`Eventually`、可控时钟 |
| 断言实现细节（私有函数名、内部调用次数） | 重构即碎 | 断言可观察行为 |
| 共享可变全局状态 | 顺序耦合 | 每用例新建（现状做得好：`testkit.OpenSQLite` 建新库） |
| 依赖真实外部网络与真实模型 | 不可重复 | 现状已用 Fake Provider，保持 |
| 一个测试文件测多个无关单元 | 定位困难 | 一文件一单元 |

## 7. 怎么验证测试用例本身是对的

这是最容易被跳过的一环。测过不等于测对。

### 7.1 先看它失败，且失败原因正确

TDD 的红灯必须"红在对的地方"。本轮实践：

```bash
go test ./server/internal/today/ -run Timeframe
# 期望：undefined: ResolveDayRange（编译失败 = 红灯正确）
```

如果红灯是因为拼写错误或 import 缺失，说明测试没在验证目标行为。**每次新增用例都要记录红灯原因**，这在提交信息或计划文档里留痕。

### 7.2 覆盖率是下界，不是目标

覆盖率高但断言弱，是最常见的假象。手册化的检查：随机挑 3 个高覆盖用例，把实现里的某个 `!=` 改成 `==`，看测试是否变红。不变红说明覆盖是虚的。

### 7.3 变异测试：验证断言是否有效

原理：自动在源码里注入小改动（把 `>=` 改 `>`、把 `true` 改 `false`、删掉一行），然后跑测试。测试应当失败，称为"杀死"；测试仍通过称为"存活"，说明该行为没有被真正断言。

官方定义与示例：<https://stryker-mutator.io/docs/>

落地方式，按成本递增：

1. **人工变异（零成本，立即可做）**：对关键函数（`scheduledSeconds`、`ResolveDayRange`、`SearchItems`）手工改一处，确认测试变红，然后还原。每次补完一组用例做一次。
2. **JS/TS 自动化**：Stryker 支持 Vitest，适合对 `agentStreamReducer`、`todayStore`、`apiModel` 这类纯逻辑做变异。
3. **Go 自动化**：可选社区工具（gremlins、ooze）。建议先做完 1 和 2，Go 侧人工变异即可覆盖核心函数。

不需要全仓库跑变异，只对"错了会静默产生错误数据"的函数做：

| 优先级 | 函数 | 错了会怎样 |
| --- | --- | --- |
| 高 | `today.scheduledSeconds` | 统计出错误时长 |
| 高 | `today.ResolveDayRange` | 查错日期的数据 |
| 高 | `today.SearchItems` 的过滤 | 返回不该返回的条目 |
| 高 | `agentStreamReducer` | 消息状态错乱 |
| 中 | `todayStore` 的 `replaceItem` | 界面显示陈旧数据 |
| 中 | `apiModel` 的错误分支 | 用户看到空白提示 |

### 7.4 检测不稳定用例

| 手段 | 命令 | 目的 |
| --- | --- | --- |
| 重复执行 | `go test ./... -count=10` | 暴露依赖顺序或残留状态 |
| 随机顺序 | `go test ./... -shuffle=on` | 暴露用例间耦合 |
| 数据竞争 | `go test ./... -race` | 已实测可用 |
| Vitest 重复 | `vitest run --retry 3` 或重复跑 | 暴露定时器依赖 |
| Playwright 重复 | `playwright test --repeat-each=3` | 暴露 E2E 时序问题 |

建议把 `-race` 和 `-shuffle=on` 加进 `test:go` 脚本，成本几乎为零。

### 7.5 元测试：测试的测试

对最关键的断言，写一个"故意错误"的实现来验证断言会失败。例如：

```go
func TestScheduledSecondsRejectsInvertedRange(t *testing.T) {
    // 若实现返回负数，本用例必须失败
    got := scheduledSeconds(itemWith(endBeforeStart))
    if got != 0 {
        t.Fatalf("scheduledSeconds() = %d, want 0", got)
    }
}
```

它的价值在于：把"这里有个保护"从隐含假设变成显式契约。

## 8. 落地路线

按投入产出排序，前两项收益最大。

### 阶段 0：修测量（半天）

1. `support_test.go` 改为 `go build -cover` + `GOCOVERDIR`，拿到真实集成覆盖率。
2. 安装 `@vitest/coverage-v8`，在 `vitest.config.ts` 加 `coverage: { provider: "v8", reporter: ["text", "json-summary"] }`。
3. Playwright 补 `retries`（CI 2 / 本地 0）与失败截图，使 `trace` 真正生效。
4. 处理 `reasoning.e2e.ts`：纳入或删除，并说明原因。

### 阶段 1：补高风险边界（1~2 天）

按 §4.3 的清单表补测，优先顺序：

1. `today.scheduledSeconds` 与 `SummarizeItems` 的除零（纯函数，防静默错数据）。
2. `today_tools.go` 的四条错误路径（stub 已存在，成本极低）。
3. `todayStore` 六个动作的不变量。
4. `apiModel` 五个错误/成功分支（含缺 `message` 的空白提示缺陷）。
5. `useTodayEffect` 的 `Promise.all` 失败组合与 `messageOf` 两分支。

### 阶段 2：规范化（1 天）

1. 把 `tests/e2e/unit/agent/*_test.go` 移出 Vitest 目录。
2. 3 个以上同类分支的测试转表驱动。
3. `Controller` 注入时钟。

### 阶段 3：门禁（半天）

`.github/workflows/test.yml` 至少包含：

```text
go vet ./... && go build ./...
go test ./server/... ./tests/e2e/integration/ -race -shuffle=on -count=1
pnpm typecheck
pnpm test
pnpm test:e2e
```

覆盖率作为**报告**而非阻塞（第一次先定基线，稳定后再设阈值）。

### 阶段 4：变异测试（持续）

先对 §7.3 表格里的 6 个函数做人工变异；再引入 Stryker 覆盖 TS 侧纯逻辑。

## 9. 命令对照

| 目的 | 命令 |
| --- | --- |
| Go 覆盖率 | `go test ./server/... -cover` |
| Go 分支级报告 | `go test ./server/... -coverprofile=cover.out && go tool cover -html=cover.out` |
| 集成覆盖率 | `GOCOVERDIR=coverage/raw go test ./tests/e2e/integration/ -count=1` 然后 `go tool covdata percent -i coverage/raw` |
| 竞态 | `go test ./server/... -race` |
| 顺序无关 | `go test ./server/... -shuffle=on` |
| 重复稳定 | `go test ./server/... -count=10` |
| 单用例 | `go test ./pkg -run TestName -v` |
| 前端覆盖率 | `vitest run --coverage`（需先装 `@vitest/coverage-v8`） |
| 前端单文件 | `vitest run tests/e2e/unit/today/todayStore.test.ts` |
| E2E 单用例 | `playwright test tests/e2e/modules/today -g "quick capture"` |
| E2E 稳定性 | `playwright test --repeat-each=3` |

## 10. 待决策

| 编号 | 问题 | 备选 |
| --- | --- | --- |
| Q-01 | 覆盖率阈值是否作为 CI 阻塞 | 不阻塞（先建基线）/ 阻塞在下降时 / 阻塞在绝对阈值 |
| Q-02 | Go 测试是否统一移到包内同包测试 | 全部同包 / 保持现状 / 只移单元测试 |
| Q-03 | 前端是否引入 ESLint | 引入 `@typescript-eslint` / 保持只有 `tsc` |
| Q-04 | Go 是否引入 golangci-lint | 引入（含 `errcheck`、`staticcheck`）/ 只用 `go vet` |
| Q-05 | 变异测试引入范围 | 仅 TS 侧 Stryker / TS + Go 全自动 / 只人工变异 |
| Q-06 | E2E 是否提高到多 worker | 保持 1 / 提高到 2~4（需确认数据隔离） |
| Q-07 | `reasoning.e2e.ts` 纳入还是删除 | 修好纳入 / 删除并登记原因 |

## 11. 参考

- Go 覆盖率与 `-cover` 构建：<https://pkg.go.dev/cmd/go>（`go help build`、`go help testflag`）
- Go 测试 flag（`-race`、`-shuffle`、`-count`、`-failfast`）已在本地 `go help testflag` 核实
- 变异测试定义与示例：<https://stryker-mutator.io/docs/>
- SQLite 相关边界（时区、夏令时）见 [今日服务端设计](../superpowers/specs/2026-11-05-craftingtable-today-server-design.md)
