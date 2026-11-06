# skills/

本目录是仓库内所有 agent skill 的**唯一事实源**。这里的内容入库；安装到各 agent 运行时的副本不入库。

## 安装

```bash
pnpm skills:check      # 只检查：有缺失或版本不同时退出码 1
pnpm skills:install    # 安装，交互选择用户级 / 项目级 / 两级都装
```

也可以直接调脚本指定范围：

```bash
node scripts/installAgentSkills.mjs --scope user      # ~/.agents/skills、~/.claude/skills
node scripts/installAgentSkills.mjs --scope project   # .github/skills、.claude/skills
node scripts/installAgentSkills.mjs --scope both
```

安装器按目录内容做 SHA-256 比对，幂等，已是最新时不写盘。

## 目录约定

```text
skills/
└── <skill-name>/
    ├── SKILL.md          必需，YAML frontmatter 含 name 与 description
    ├── scripts/          可选，skill 自用的可执行脚本
    └── references/       可选，按需读取的详细资料
```

`SKILL.md` 的 `description` 决定触发率：要同时写清「做什么」和「什么时候用」，并把用户的真实说法写进去。

## 不要放在这里

仓库级工具（安装器、构建脚本、测试辅助）放 `scripts/`，不要放进某个 skill。原因：

- 安装器要安装所有 skill，不属于任何一个。
- skill 目录会被整份复制到运行时目录，里面的脚本如果依赖仓库根路径就会失效。
- 先有鸡还是先有蛋：安装器必须在安装产物之外。

## 各运行时的读取位置

| 运行时 | 用户级 | 项目级 |
| --- | --- | --- |
| VS Code Copilot | `~/.agents/skills/` | `.github/skills/` |
| Claude Code | `~/.claude/skills/` | `.claude/skills/` |
| Codex | `~/.agents/skills/` | 无原生支持，由 `AGENTS.md` 指路 |
| Gemini CLI | `~/.gemini/skills/`（`.agents/skills/` 为跨 runtime 别名） | — |
| ChatGPT / API | 无文件系统，不读取 | — |

用户级对所有仓库生效；项目级只在本仓库生效，适合需要跟仓库版本绑定的 skill。

## 当前 skill

| skill | 用途 |
| --- | --- |
| `worktree-task-flow` | 新需求先建独立 worktree；用户要求时自动合并到主分支、按手册处理冲突并验证 |
