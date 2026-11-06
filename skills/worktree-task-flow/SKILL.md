---
name: worktree-task-flow
description: 为每个新需求先创建独立 git worktree，并在用户要求时自动合并到主分支、处理合并冲突并验证。当用户在新对话开头提出功能、修复、重构、文档或配置类新需求时使用（例如"帮我加一个…"、"实现…"、"修一下这个 bug"、"优化…"、"重构…"、"把 X 改成 Y"、"写个脚本…"），也用于用户说"合并到主分支"、"合回主干"、"merge 到 main"、"提交并合并"、"并入主分支"时。即使用户没有提到 worktree、分支或 worktree 这个词，只要任务会修改仓库文件，就应在动手前先建立隔离工作区；即使用户只描述了一个待实现的问题而没说"合并"，本条同样适用。
---

# Worktree 任务流

## 为什么这样做

一个仓库同时只检出一份工作区，意味着不相关的需求会互相踩：主工作区可能正跑着 dev server、留着别人未提交的实验代码、或者上一个需求的临时文件。等到要合并时才发现冲突，代价远高于一开始就隔离。

worktree 让每个需求拥有自己的目录和分支，可以独立构建、独立测试、随时丢弃。代价只是多一个目录，以及合并时要多做一次集成。

真正值得把这套流程固化的，是合并那一步：冲突往往不是"两边都改了同一行"这么直观，而是迁移文件编号撞车、锁文件版本漂移、行尾差异被当成内容改动。这些有确定的处理规则，值得每次都按同一套走。

## 适用判断

在改动任何文件之前先判断，这决定了后面所有步骤在哪执行。

| 情况 | 动作 |
| --- | --- |
| 新对话，首个实质请求是新需求 | 创建 worktree（本 skill 主路径） |
| 延续同一需求（本轮已建过 worktree，或用户说"继续"） | 复用已有 worktree，不新建 |
| 用户指定"在这个 worktree 里做" | 进入该目录，把 `worktree.owner` 改成自己，再开始 |
| 纯问答、只读排查、代码走读 | 不建 worktree |
| 用户说"就在当前目录改"、"不要建 worktree" | 遵从，跳过创建 |
| 对话发生在已有 worktree 内 | 不嵌套创建 |

用户指定了某个 worktree 时，**以用户指定的为准**，不要因为"当前目录是主工作区"就改在主工作区动手。先 `cd` 过去，确认分支和 `worktree.owner`，再开始。

判断"是否新需求"看意图而不是措辞：要求产出代码、文档、配置变更的是需求；问"这个函数干什么"、"为什么报错"且只需读代码的是排查。拿不准时用一句话说明你打算建 worktree，然后直接建——用户介意会当场纠正，成本很低。

## 第一步：创建 worktree

### 探测仓库状态

```bash
git rev-parse --show-toplevel
git rev-parse --abbrev-ref HEAD
git rev-parse --git-dir
git rev-parse --git-common-dir
```

最后两条结果不同，说明当前已经在 worktree 里，直接在当前目录工作，不要嵌套。

### 决定分支名和目录

分支名用 `<type>/<slug>`：

| type | 用于 |
| --- | --- |
| `feat` | 新功能、新能力 |
| `fix` | 修 bug |
| `docs` | 文档、设计、计划 |
| `chore` | 构建、依赖、配置、工具链 |
| `refactor` | 不改变行为的结构调整 |

`slug` 用英文 kebab-case、2–4 个词，直接来自用户意图（`quick-capture-category`、`today-date-navigation`）。避免 `temp`、`task1`、`update` 这类无信息量的词，因为分支名和目录名是几周后唯一还能看懂的线索。

目录与仓库同级：`<repo-parent>/<repo-name>-<slug>`。

### 执行

```bash
git worktree add -b feat/quick-capture-category "<repo-parent>/<repo-name>-quick-capture-category" <base-branch>
```

`<base-branch>` 用主分支（下一步探测），不要用另一个功能分支，否则合并时会把无关提交带进来。

创建后立刻把三件事告诉用户，后续回报都用这几个值：

- worktree 路径
- 分支名
- base commit（`git -C <worktree> rev-parse --short HEAD`）

如果目录或分支已存在，说明这个需求之前开过：先确认用户是要继续还是重开，不要换个名字再建一份。

### 登记负责人

创建后立刻写入归属信息，让下一个打开这个目录的会话知道"这里有主、意图是什么"。

```bash
git -C <worktree> config extensions.worktreeConfig true
git -C <worktree> config --worktree worktree.owner   "<self-label> @ <ISO-8601 时间>"
git -C <worktree> config --worktree worktree.purpose "<一句话需求>"
git -C <worktree> config --worktree worktree.base    "<base-branch> @ <short-sha>"
```

`<self-label>` 用当前运行时加会话短号，例如 `copilot-sdk/fde12d0d`。值存在 `.git/worktrees/<name>/config.worktree`，不影响主分支配置。

**这个字段是标签，不是访问控制。** 它没有强制力，任何能跑 git 的进程都能改。它的用途只有两个：让后来者知道该找谁确认、让长期闲置的 worktree 能被识别。不要把它当成权限系统，也不要因为写上了就以为自己有权改动别人的 scope。

读取方式：

```bash
git -C <worktree> config --worktree worktree.owner
```

接手别人的 worktree 时（用户明确要求你继续某个已有目录），先把 owner 改成自己再开始改文件，否则记录会一直指向已经离开的会话。

### 之后的工作方式

所有文件读写、命令执行都在自己的 worktree 路径内完成。命令用 `cd <worktree>; ...` 前缀，或显式指定工作目录，避免误改主工作区。

遇到这类报错时不要改代码——它们是编辑器索引问题，不是编译错误：

- `This file is within module "...", which is not included in your workspace`
- `use of internal package ... not allowed`
- 刚新增的包内符号报 `undefined:`

判断依据是真实命令：`go build ./...`、`pnpm typecheck`、`tsc --noEmit`。另外不要用 `go.work` 把两个 worktree 同时纳入——同一模块路径会在 workspace 里重复，Go 直接拒绝加载。

### 跨 worktree 边界

**写操作只允许在自己负责的 worktree 内。**

读操作有三个白名单用途，除此之外不要碰其他 worktree：

| 用途 | 读什么 | 为什么必须允许 |
| --- | --- | --- |
| 合并预检 | 主工作区的 `git status` | 判断未提交改动是否会被覆盖。这是防止静默丢失用户工作的唯一手段 |
| 推进主分支 | 在主工作区执行 `git merge --ff-only` | 合并到主分支本身就发生在主工作区 |
| 合并后验证 | 在主工作区跑构建与测试 | 确认主分支确实可用，而不是只在功能分支上可用 |

任何情况下都**不修改**其他 worktree 的工作区文件：不改文件、不 `checkout`、不 `reset`、不 `stash`、不清理它们的未跟踪文件。需要改动别人的目录才能完成任务时，停下来问用户。

注意不要在报告里省略这条边界。用户问"你动了哪里"时，答案必须能逐个列出路径。

## 第二步：合并到主分支

触发词：合并到主分支、合回主干、merge 到 main、并入主分支、提交并合并、可以合并了。

### 先跑预检

```bash
bash <skill-dir>/scripts/merge_preflight.sh --main <main-branch>
```

脚本输出主分支名、merge-base、两侧改动文件、重叠文件、迁移编号冲突、以及主工作区脏文件是否与本需求重叠。退出码 1 表示存在必须先解决的阻塞项，退出码 2 表示参数或仓库状态无法判断。

Windows 上 `bash` 可能被 WSL 接管并报 `execvpe(/bin/bash) failed`，此时直接调用 Git Bash：

```bash
"C:/Program Files/Git/bin/bash.exe" <skill-dir>/scripts/merge_preflight.sh --main <main-branch>
```

阻塞项只有三类，出现任何一类都先停下处理，不要带着它们开始 merge：

1. **本 worktree 有未提交改动** —— 属于本需求的先提交；不确定归属的先问。
2. **主工作区脏文件与本次改动重叠** —— merge 会覆盖或冲掉用户未提交的工作，这是唯一无法靠技术手段补救的情况。
3. **迁移文件编号冲突** —— 两侧都新增了同序号迁移，必须重命名后再合并。

主工作区有脏文件但与本次改动不重叠时可以继续；合并后提醒用户这些文件仍在。

### 解决迁移编号冲突

数据库迁移通常按文件名排序执行，账本里记了文件名与校验和，所以：

- 绝不修改**已应用**的迁移文件内容，校验和不匹配会让服务直接拒绝启动。
- 编号撞车时，把本分支的迁移重命名为下一个未占用的序号，并确认没有数据库应用过旧文件名（有 `data/*.sqlite` 时查 `schema_migrations` 表）。
- 重命名后同步更新文档里对文件名的引用。改文档时注意编码：PowerShell 5.1 的 `Set-Content` 默认不是 UTF-8，中文会变乱码；用 `[System.IO.File]::WriteAllText(path, text, (New-Object System.Text.UTF8Encoding($false)))`。

### 合并

先在功能分支做集成，这样冲突解决不会污染主工作区：

```bash
cd <worktree>
git merge <main-branch>
```

解决冲突后：

```bash
git add -A
git merge --continue
```

然后把这条 merge commit 的标题改掉：

```bash
git commit --amend -m "chore(merge): integrate <main-branch> before merging <branch>"
```

原因：最终是用快进把主分支推到这条 commit 上，保留 git 自动生成的 `Merge branch 'main' into feat/x` 会让人以为主分支被合进了功能分支——方向恰好相反，排查历史时会误导。用中性的 "integrate ... before merging ..." 描述真实发生的事。

### 冲突处理手册

机械冲突按表格自动解决并在报告里逐条说明依据；语义冲突停下来问用户，不要猜。

| 冲突类型 | 处理 |
| --- | --- |
| 迁移文件同编号 | 本分支重命名到下一个空号，确认无库已应用旧名，同步文档引用 |
| 已应用迁移内容被改 | 回退该改动，改为新增迁移 |
| 锁文件（`pnpm-lock.yaml`、`go.sum`） | 不手工编辑，用 `pnpm install` / `go mod tidy` 重新生成后提交 |
| 纯行尾或空白差异 | 以主分支为准；确认有 `.gitattributes` 且 `git ls-files --eol` 无文本 CRLF |
| 生成产物（PNG/SVG/打包物） | 跑生成脚本重出，不手工合并二进制 |
| 依赖版本两侧都改 | 取主分支版本；若本分支需要更高版本，显式说明理由 |
| 同一函数/同一段的逻辑改动 | 语义冲突，列出两侧意图后问用户 |
| 一侧删除、另一侧修改 | 语义冲突，问用户保留还是删除 |

### 验证闸门

合并前在 worktree 里跑一次，合并后在主工作区再跑一次。自动探测项目命令，不要硬编码：

| 探测到 | 命令 |
| --- | --- |
| `go.mod` | `go build ./...`、`go vet ./...`、`go test ./server/... ./tests/...`（按实际存在路径调整） |
| `package.json` 有 `typecheck` | `pnpm typecheck` 或 `npm run typecheck` |
| `package.json` 有 `test` | `pnpm test` |
| `package.json` 有 `test:go` | `pnpm test:go` |
| `playwright.config.ts` 且有 `test:e2e` | 仅在改动涉及 UI 时跑，并说明 |
| 都没有 | 跳过，并在报告里说明未验证 |

测试失败时不要继续合并。先判断是本次改动引入的还是主分支带来的；若无法在合理范围内修复，把失败原因和已完成的合并状态告诉用户，由用户决定。

### 推进主分支

```bash
cd <main-worktree>
git merge --ff-only <branch>
```

`--ff-only` 是刻意的：如果它拒绝执行，说明主分支在预检之后又前进了，有人同时在改。此时回到预检重新集成，而不是让 git 生成一个没人看过冲突解决的 merge commit。

### 报告

```markdown
## 合并结果
- 分支：<branch> → <main>
- 主分支 HEAD：<sha>
- 合并方式：ff-only

## 冲突与解决
| 文件 | 类型 | 处理 |
| --- | --- | --- |

## 验证
| 命令 | 结果 |
| --- | --- |

## 后续
- worktree：<path>（保留 / 已清理）
- 未处理项：<...>
```

没有冲突时明确写"无冲突"，不要省略这一节——沉默会让用户以为你没检查。

## 在 CraftingTable 仓库中

本仓库把 skill 的唯一事实源放在根目录 `skills/`，安装到运行时由脚本完成：

```bash
pnpm skills:install    # 交互选择用户级 / 项目级 / 两级都装
pnpm skills:check      # 只检查，有差异时退出码 1
```

安装位置：

| 范围 | 目标目录 | 生效范围 |
| --- | --- | --- |
| 用户级 | `~/.agents/skills/`、`~/.claude/skills/` | 所有仓库 |
| 项目级 | `.github/skills/`、`.claude/skills/` | 只在本仓库 |

项目级安装会在仓库内生成副本，这些副本已加入 `.gitignore`，不要提交；改 skill 只改 `skills/`，然后重新运行安装命令。

预检脚本因此位于 `skills/worktree-task-flow/scripts/merge_preflight.sh`。

## 清理

默认保留 worktree 和分支，方便用户回看和继续迭代。用户要求清理时：

```bash
git worktree remove <path>
git branch -d <branch>
```

用 `-d` 而不是 `-D`：未完全合并的分支删除失败是保护，不是错误。

## 安全边界

- 写操作只在自己负责的 worktree 内；其他 worktree 只允许预检、`merge --ff-only`、合并后验证三种读取用途，绝不修改其文件。
- 不在主分支上改写已有提交（不 `amend`、不 `rebase`、不 `reset --hard`）。
- 不 `git push --force`。
- 不删除未合并的分支或 worktree。
- 不 stash 或回退用户在主工作区的未提交工作；重叠就停下来问。
- 每次自动解决冲突都要能说出依据；说不清就交给用户。
- 合并过程中任何一步失败，都要报告"停在哪一步、仓库处于什么状态"，不要让用户面对一个半合并的仓库自己猜。
- `worktree.owner` 只是标签，不具备强制力，不要用它代替用户的明确授权。

