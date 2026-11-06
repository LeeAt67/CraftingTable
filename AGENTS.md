# AGENTS.md

CraftingTable 是一个单用户个人事务工作台：React + TypeScript + Vite 前端，Go 模块化单体后端，本地 SQLite。本文件面向在本仓库工作的 AI agent，说明初始化步骤和不可违反的约定。

## 初始化（每次会话开始前执行）

1. 读取本仓库的 `skills/` 目录，逐个读取各 skill 的 `SKILL.md` 中的 `name` 与 `description`，判断哪些与当前任务相关。
2. 校验 skill 是否已安装到 agent 运行时：

   ```bash
   pnpm skills:check
   ```

   退出码 0 表示已是最新；退出码 1 表示缺失或版本不同，此时询问用户装到哪一级：

   ```bash
   pnpm skills:install --scope user      # 用户级：所有仓库生效
   pnpm skills:install --scope project   # 项目级：只在本仓库生效
   pnpm skills:install --scope both      # 两级都装
   ```

   不带 `--scope` 时脚本会交互询问；非交互环境默认用户级。

3. 两级安装的区别：

   | 范围 | 目标目录 | 生效范围 | 是否入库 |
   | --- | --- | --- | --- |
   | 用户级 | `~/.agents/skills/`、`~/.claude/skills/` | 所有仓库 | 不入库 |
   | 项目级 | `.github/skills/`、`.claude/skills/` | 只在本仓库 | 已 gitignore，不入库 |

4. 安装是幂等的，按目录内容哈希比对，重复执行不会产生副作用。安装完成后，按相关 skill 的流程执行任务，不要凭记忆复述其内容。

仓库 skill 的唯一事实源是根目录 `skills/`（说明见 [skills/README.md](./skills/README.md)）。`~/.agents/skills/`、`~/.claude/skills/`、`.github/skills/`、`.claude/skills/` 都只是安装产物，不要直接改，也不要提交。

## 需求登记（发现新需求时必须执行）

[docs/requirements.md](./docs/requirements.md) 是全仓库唯一的需求清单。它只放一句话索引与状态，详情在链接的文档里。

出现以下任一情况，**先在该文档登记一行，再开始动手**：

- 用户明确提出新功能、改进点或缺陷修复。
- 用户在对话或文档里留下 TODO、"以后要做"、"先记一下"之类的意图。
- 设计与调研文档产出了新的待办项。
- 发现必须处理但本次不做的技术债。

登记方式：

1. 选一个未占用的编号，前缀见该文档 §3.1（`T` 今日、`S` 设置、`N` 导航、`A` API、`Q` 质量、`AG` Agent）。
2. 补齐四项：一句话需求、优先级 `P0`/`P1`/`P2`、状态、详细文档链接。
3. 详细文档还没写时链接写 `待补`，不要为填满而临时编造文档。
4. 需求来自某份调研或设计时，链接要带文档路径与章节名，便于反向追溯。
5. 实现完成后只改状态列，不删行。

纯提问、代码走读和排查结论不需要登记。判断标准是：这个结论会不会变成未来的工作项。

## 当前仓库 skill

| skill | 用途 |
| --- | --- |
| `worktree-task-flow` | 新需求先建独立 worktree；用户要求时自动合并到主分支、按手册处理冲突并验证 |

## 开发命令

```bash
pnpm typecheck        # tsc --noEmit
pnpm test             # Vitest
pnpm test:go          # go test ./server/... ./tests/e2e/integration/...
pnpm test:e2e         # Playwright
go build ./...        # 后端编译
go vet ./...          # 后端静态检查
```

改动后至少运行与被改范围对应的命令：后端改动跑 `go build ./...`、`go vet ./...` 与相关 Go 测试；前端改动跑 `pnpm typecheck` 与相关 Vitest 用例。

## 不可违反的约定

- 遵循 SDD：功能、行为或架构变化先更新设计与计划文档，再写失败测试，最后实现。
- 前端保持 MVC 依赖方向：View 读 Store、调用 Effect；Effect 调用 Model；Model 不感知 Store；Zustand 是业务状态唯一事实源。
- 后端保持模块化单体：`server/internal/agent/**` 的契约、Run 生命周期与迁移语义不得被非 Agent 需求修改。
- 数据库迁移只新增文件、按序号递增。绝不修改已应用的迁移内容，`schema_migrations` 会记录文件名与 SHA-256，校验不匹配会拒绝启动。
- 所有文本文件使用 LF 行尾，仓库通过 `.gitattributes` 强制；不要引入 CRLF。
- 提交信息用 `type(scope): summary` 形式，`type` 取 `feat`、`fix`、`docs`、`chore`、`refactor`、`test`。

## 相关文档

- [README](./README.md)
- [术语表](./docs/glossary.md)
- [工程化设计规范](./docs/superpowers/specs/2026-11-05-craftingtable-engineering-design.md)
- [Agent 设计](./docs/superpowers/specs/2026-11-05-craftingtable-agent-design.md)
- [Agent 架构图](./docs/architecture/README.md)
