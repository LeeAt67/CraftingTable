#!/usr/bin/env node
// 把仓库 skills/ 目录下的所有 skill 安装到 agent 运行时。
// 支持用户级（全局，跨仓库生效）与项目级（只在本仓库生效）。
// 幂等：按目录内容哈希比对，已是最新时不写盘。

import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import crypto from "node:crypto";
import readline from "node:readline/promises";

const repoRoot = path.resolve(import.meta.dirname, "..");
const sourceRoot = path.join(repoRoot, "skills");

const SCOPES = {
  user: [
    { runtime: "agents", directory: path.join(os.homedir(), ".agents", "skills") },
    { runtime: "claude", directory: path.join(os.homedir(), ".claude", "skills") },
  ],
  project: [
    { runtime: "copilot", directory: path.join(repoRoot, ".github", "skills") },
    { runtime: "claude", directory: path.join(repoRoot, ".claude", "skills") },
  ],
};

const SCOPE_HELP = {
  user: "用户级：写入 ~/.agents/skills 与 ~/.claude/skills，对所有仓库生效",
  project: "项目级：写入本仓库 .github/skills 与 .claude/skills，只对本仓库生效",
  both: "两级都装：用户级 + 项目级",
};

function parseArguments(argv) {
  const options = { check: false, scope: "", help: false };
  for (let index = 0; index < argv.length; index += 1) {
    const value = argv[index];
    if (value === "--check") {
      options.check = true;
    } else if (value === "--help" || value === "-h") {
      options.help = true;
    } else if (value === "--scope") {
      options.scope = argv[index + 1] ?? "";
      index += 1;
    } else if (value.startsWith("--scope=")) {
      options.scope = value.slice("--scope=".length);
    }
  }
  return options;
}

function printHelp() {
  console.log(`用法：node scripts/installAgentSkills.mjs [--scope user|project|both] [--check]

  --scope  安装位置。省略时交互询问；非交互环境默认 user
  --check  只检查是否有缺失或版本不同，不写盘；有差异时退出码 1

安装范围：
  user     ${SCOPE_HELP.user}
  project  ${SCOPE_HELP.project}
  both     ${SCOPE_HELP.both}

运行时目录对照：
  VS Code Copilot  项目级 .github/skills/，用户级 ~/.agents/skills/
  Claude Code      项目级 .claude/skills/，用户级 ~/.claude/skills/
  Codex / Gemini   用户级 ~/.agents/skills/（项目级由 AGENTS.md 指路）`);
}

async function resolveScope(options) {
  if (options.scope) {
    if (!(options.scope in SCOPE_HELP)) {
      console.error(`未知的 --scope：${options.scope}（可选 user、project、both）`);
      return null;
    }
    return options.scope;
  }

  if (!process.stdin.isTTY || !process.stdout.isTTY) {
    return "user";
  }

  const prompt = readline.createInterface({
    input: process.stdin,
    output: process.stdout,
  });
  try {
    console.log("请选择安装位置：");
    console.log(`  1) user     ${SCOPE_HELP.user}`);
    console.log(`  2) project  ${SCOPE_HELP.project}`);
    console.log(`  3) both     ${SCOPE_HELP.both}`);
    const answer = (await prompt.question("输入 1 / 2 / 3（回车默认 1）：")).trim();
    if (answer === "2") return "project";
    if (answer === "3") return "both";
    return "user";
  } finally {
    prompt.close();
  }
}

function listFiles(directory, base = directory) {
  const entries = fs.readdirSync(directory, { withFileTypes: true });
  const files = [];
  for (const entry of entries.sort((left, right) => left.name.localeCompare(right.name))) {
    if (
      entry.name === "__pycache__" ||
      entry.name === "node_modules" ||
      entry.name === ".DS_Store"
    ) {
      continue;
    }
    const full = path.join(directory, entry.name);
    if (entry.isDirectory()) {
      files.push(...listFiles(full, base));
    } else if (entry.isFile()) {
      files.push(path.relative(base, full).split(path.sep).join("/"));
    }
  }
  return files;
}

function hashDirectory(directory) {
  const hash = crypto.createHash("sha256");
  for (const relative of listFiles(directory)) {
    hash.update(relative);
    hash.update("\0");
    hash.update(fs.readFileSync(path.join(directory, relative)));
    hash.update("\0");
  }
  return hash.digest("hex");
}

function copyDirectory(source, destination) {
  fs.rmSync(destination, { recursive: true, force: true });
  fs.mkdirSync(path.dirname(destination), { recursive: true });
  fs.cpSync(source, destination, { recursive: true });
}

function readSkillName(directory) {
  const skillFile = path.join(directory, "SKILL.md");
  if (!fs.existsSync(skillFile)) {
    return null;
  }
  const content = fs.readFileSync(skillFile, "utf8");
  const match = content.match(/^---[\s\S]*?^name:\s*(.+)$/m);
  return match ? match[1].trim() : null;
}

async function main() {
  const options = parseArguments(process.argv.slice(2));

  if (options.help) {
    printHelp();
    return 0;
  }

  if (!fs.existsSync(sourceRoot)) {
    console.error(`找不到仓库 skill 目录：${sourceRoot}`);
    return 2;
  }

  const skills = fs
    .readdirSync(sourceRoot, { withFileTypes: true })
    .filter((entry) => entry.isDirectory())
    .map((entry) => path.join(sourceRoot, entry.name))
    .filter((directory) => fs.existsSync(path.join(directory, "SKILL.md")));

  if (skills.length === 0) {
    console.log("skills/ 下没有可安装的 skill。");
    return 0;
  }

  const scope = await resolveScope(options);
  if (!scope) {
    return 2;
  }

  const targets =
    scope === "both" ? [...SCOPES.user, ...SCOPES.project] : SCOPES[scope];

  let drift = 0;
  const rows = [];

  for (const skill of skills) {
    const name = readSkillName(skill) ?? path.basename(skill);
    const sourceHash = hashDirectory(skill);

    for (const target of targets) {
      const destination = path.join(target.directory, name);
      const installed = fs.existsSync(path.join(destination, "SKILL.md"));
      const installedHash = installed ? hashDirectory(destination) : null;

      let state;
      if (!installed) {
        state = "缺失";
      } else if (installedHash === sourceHash) {
        state = "已是最新";
      } else {
        state = "版本不同";
      }

      if (state !== "已是最新") {
        drift += 1;
        if (!options.check) {
          copyDirectory(skill, destination);
          state = `${state} → 已安装`;
        }
      }

      rows.push({ skill: name, runtime: target.runtime, path: destination, state });
    }
  }

  console.log(`安装范围：${scope}（${SCOPE_HELP[scope]}）\n`);
  for (const row of rows) {
    console.log(`${row.skill} [${row.runtime}] ${row.state}\n  ${row.path}`);
  }

  if (scope === "project" || scope === "both") {
    console.log(
      "\n提示：项目级安装会在仓库内生成副本，这些副本已被 .gitignore 忽略。" +
        "仓库内的唯一事实源始终是 `skills/`，改 skill 请改那里再重新安装。",
    );
  }

  if (options.check && drift > 0) {
    console.log(`\n检测到 ${drift} 处需要安装或更新。去掉 --check 重新运行可完成安装。`);
    return 1;
  }

  if (drift === 0) {
    console.log("\n全部 skill 已是最新，无需安装。");
  } else if (!options.check) {
    console.log(`\n完成：处理了 ${drift} 处安装。`);
  }
  return 0;
}

process.exitCode = await main();
