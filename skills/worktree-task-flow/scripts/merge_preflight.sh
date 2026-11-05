#!/usr/bin/env bash
# 合并前预检：找出会阻止 worktree 分支安全合并到主分支的问题。
# 只读操作，不修改仓库。输出 Markdown 摘要；存在阻塞项时退出码为 1。

set -uo pipefail

usage() {
  cat <<'USAGE'
用法：bash merge_preflight.sh [--main <主分支>] [--branch <待合并分支>] [--path <目录>]

  --main    主分支名。省略时自动探测（origin/HEAD → main/master → worktree 内分支）
  --branch  待合并分支。省略时使用 --path 目录的当前分支
  --path    待合并分支所在目录。省略时使用当前目录

退出码：0 可以合并；1 存在阻塞项；2 参数或仓库状态无法判断

Windows 下若 `bash` 被 WSL 接管，用 Git Bash 直接调用：
  "C:/Program Files/Git/bin/bash.exe" merge_preflight.sh --main main
USAGE
}

die() {
  printf '%s\n' "$*" >&2
  exit 2
}

git_at() {
  local dir="$1"
  shift
  git -C "$dir" "$@"
}

STATE_DIR=""
cleanup() {
  if [ -n "$STATE_DIR" ] && [ -d "$STATE_DIR" ]; then
    rm -rf "$STATE_DIR"
  fi
}
trap cleanup EXIT

MAIN_ARG=""
BRANCH_ARG=""
PATH_ARG=""

while [ $# -gt 0 ]; do
  case "$1" in
    --main)
      MAIN_ARG="${2:-}"
      shift 2 || die "--main 需要一个分支名"
      ;;
    --branch)
      BRANCH_ARG="${2:-}"
      shift 2 || die "--branch 需要一个分支名"
      ;;
    --path)
      PATH_ARG="${2:-}"
      shift 2 || die "--path 需要一个目录"
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      die "未知参数：$1（用 --help 查看用法）"
      ;;
  esac
done

START_DIR="${PATH_ARG:-$PWD}"
[ -d "$START_DIR" ] || die "目录不存在：$START_DIR"

ROOT="$(git -C "$START_DIR" rev-parse --show-toplevel 2>/dev/null)" ||
  die "不在 git 仓库内：$START_DIR"

STATE_DIR="$(mktemp -d 2>/dev/null || mktemp -d -t preflight)"

# 输出 "路径<TAB>分支"，detached 时分支为空
list_worktrees() {
  git_at "$ROOT" worktree list --porcelain | awk '
    /^worktree / {
      if (wt != "") printf "%s\t%s\n", wt, br
      wt = substr($0, 10); br = ""; next
    }
    /^branch / {
      br = substr($0, 8); sub(/^refs\/heads\//, "", br); next
    }
    END { if (wt != "") printf "%s\t%s\n", wt, br }
  '
}

detect_main_branch() {
  local remote_head branch
  remote_head="$(git_at "$ROOT" symbolic-ref --short refs/remotes/origin/HEAD 2>/dev/null)"
  if [ -n "$remote_head" ]; then
    printf '%s\n' "${remote_head#origin/}"
    return 0
  fi
  for branch in main master; do
    if git_at "$ROOT" show-ref --verify --quiet "refs/heads/$branch"; then
      printf '%s\n' "$branch"
      return 0
    fi
  done
  # 没有 main/master 时退而取任一非当前 worktree 的分支，交给用户用 --main 覆盖
  list_worktrees | awk -F'\t' -v skip="$START_DIR" 'NF > 1 && $2 != "" && $1 != skip { print $2; exit }'
}

changed_files() {
  local base="$1" ref="$2" filter="${3:-}"
  local -a args=(diff --name-only)
  [ -n "$filter" ] && args+=("--diff-filter=$filter")
  args+=("$base..$ref")
  git_at "$ROOT" "${args[@]}"
}

# 解析 `git status --porcelain -z`。必须用 -z：
# 普通格式首行以一个空格开头表示"未暂存修改"，
# 任何对整段输出的裁剪都会吃掉它并让路径少一个字符。
dirty_files() {
  local dir="$1" entry status extra
  [ -d "$dir" ] || return 0
  while IFS= read -r -d '' entry; do
    status="${entry:0:2}"
    printf '%s\n' "${entry:3}"
    case "$status" in
      *R*|*C*)
        if IFS= read -r -d '' extra; then
          printf '%s\n' "$extra"
        fi
        ;;
    esac
  done < <(git_at "$dir" status --porcelain -z 2>/dev/null)
}

merge_in_progress() {
  git_at "$1" rev-parse -q --verify MERGE_HEAD >/dev/null 2>&1
}

# 迁移文件按 "目录|序号" 归组，用来发现两侧新增了同序号迁移
migration_key() {
  local path="$1" base
  base="${path##*/}"
  if [[ $base =~ ^([0-9]+)[_-] ]]; then
    printf '%s|%s' "${path%/*}" "${BASH_REMATCH[1]}"
  fi
}

collect_migrations() {
  local ref="$1" out="$2" path key
  : >"$out"
  while IFS= read -r path; do
    [ -n "$path" ] || continue
    key="$(migration_key "$path")"
    [ -n "$key" ] || continue
    printf '%s\t%s\n' "$key" "$path" >>"$out"
  done < <(changed_files "$BASE" "$ref" A)
}

count_lines() {
  if [ ! -f "$1" ]; then
    printf '0'
    return 0
  fi
  awk 'END { print NR + 0 }' "$1"
}

print_list() {
  local limit=12 total shown
  total=$(count_lines "$1")
  if [ "$total" -eq 0 ]; then
    printf '（无）\n'
    return 0
  fi
  shown=$(head -n "$limit" <"$1")
  while IFS= read -r line; do
    [ -n "$line" ] && printf '  - `%s`\n' "$line"
  done <<<"$shown"
  if [ "$total" -gt "$limit" ]; then
    printf '  - …另有 %s 个\n' "$((total - limit))"
  fi
}

if [ -n "$MAIN_ARG" ]; then
  MAIN_BRANCH="$MAIN_ARG"
else
  MAIN_BRANCH="$(detect_main_branch)"
fi
[ -n "$MAIN_BRANCH" ] ||
  die "无法自动确定主分支，请用 --main <branch> 指定。"

if [ -n "$BRANCH_ARG" ]; then
  BRANCH="$BRANCH_ARG"
else
  BRANCH="$(git_at "$START_DIR" rev-parse --abbrev-ref HEAD 2>/dev/null)"
fi
[ -n "$BRANCH" ] && [ "$BRANCH" != "HEAD" ] ||
  die "当前处于 detached HEAD，请用 --branch <branch> 指定待合并分支。"

if ! git_at "$ROOT" rev-parse --verify --quiet "$MAIN_BRANCH" >/dev/null; then
  die "主分支不存在：$MAIN_BRANCH"
fi
if ! git_at "$ROOT" rev-parse --verify --quiet "$BRANCH" >/dev/null; then
  die "待合并分支不存在：$BRANCH"
fi

BASE="$(git_at "$ROOT" merge-base "$MAIN_BRANCH" "$BRANCH" 2>/dev/null)" ||
  die "无法计算 $MAIN_BRANCH 与 $BRANCH 的 merge-base。"

MAIN_WORKTREE="$(list_worktrees | awk -F'\t' -v target="$MAIN_BRANCH" '$2 == target { print $1; exit }')"

changed_files "$BASE" "$BRANCH" >"$STATE_DIR/ours"
changed_files "$BASE" "$MAIN_BRANCH" >"$STATE_DIR/theirs"
sort "$STATE_DIR/ours" >"$STATE_DIR/ours.sorted"
sort "$STATE_DIR/theirs" >"$STATE_DIR/theirs.sorted"
comm -12 "$STATE_DIR/ours.sorted" "$STATE_DIR/theirs.sorted" >"$STATE_DIR/overlap"

dirty_files "$START_DIR" >"$STATE_DIR/our_dirty"
if [ -n "$MAIN_WORKTREE" ]; then
  dirty_files "$MAIN_WORKTREE" >"$STATE_DIR/main_dirty"
else
  : >"$STATE_DIR/main_dirty"
fi
sort -u "$STATE_DIR/main_dirty" >"$STATE_DIR/main_dirty.sorted"
comm -12 "$STATE_DIR/ours.sorted" "$STATE_DIR/main_dirty.sorted" >"$STATE_DIR/dirty_overlap"

collect_migrations "$BRANCH" "$STATE_DIR/ours_migrations"
collect_migrations "$MAIN_BRANCH" "$STATE_DIR/theirs_migrations"
awk -F'\t' '
  NR == FNR { theirs[$1] = $2; next }
  ($1 in theirs) && theirs[$1] != $2 { printf "%s\t%s\t%s\n", $1, theirs[$1], $2 }
' "$STATE_DIR/theirs_migrations" "$STATE_DIR/ours_migrations" >"$STATE_DIR/migration_conflicts"

ALREADY_MERGED=0
if git_at "$ROOT" merge-base --is-ancestor "$BRANCH" "$MAIN_BRANCH" >/dev/null 2>&1; then
  ALREADY_MERGED=1
fi

MERGING=0
if merge_in_progress "$START_DIR"; then
  MERGING=1
fi

AHEAD=0
BEHIND=0
COUNTS="$(git_at "$ROOT" rev-list --left-right --count "$MAIN_BRANCH...$BRANCH" 2>/dev/null)"
if [ -n "$COUNTS" ]; then
  BEHIND="${COUNTS%%[[:space:]]*}"
  AHEAD="${COUNTS##*[[:space:]]}"
fi

printf '## 合并预检\n\n'
printf -- '- 仓库：`%s`\n' "$ROOT"
printf -- '- 主分支：`%s`\n' "$MAIN_BRANCH"
printf -- '- 待合并分支：`%s`（领先 %s，落后 %s）\n' "$BRANCH" "${AHEAD:-0}" "${BEHIND:-0}"
printf -- '- merge-base：`%s`\n' "${BASE:0:12}"
printf -- '- 主分支 worktree：`%s`\n\n' "${MAIN_WORKTREE:-未找到}"

printf '### 本分支改动（%s）\n' "$(count_lines "$STATE_DIR/ours")"
print_list "$STATE_DIR/ours"
printf '\n### 主分支独有改动（%s）\n' "$(count_lines "$STATE_DIR/theirs")"
print_list "$STATE_DIR/theirs"
printf '\n### 两侧都改的文件（%s）\n' "$(count_lines "$STATE_DIR/overlap")"
print_list "$STATE_DIR/overlap"

if [ -s "$STATE_DIR/main_dirty" ]; then
  printf '\n### 主工作区未提交改动（%s，不重叠时可继续）\n' "$(count_lines "$STATE_DIR/main_dirty")"
  print_list "$STATE_DIR/main_dirty"
fi

printf '\n### 迁移编号冲突（%s）\n' "$(count_lines "$STATE_DIR/migration_conflicts")"
if [ -s "$STATE_DIR/migration_conflicts" ]; then
  while IFS=$'\t' read -r key theirs ours; do
    printf '  - 编号 %s：主分支 `%s` ↔ 本分支 `%s`\n' "${key#*|}" "$theirs" "$ours"
  done <"$STATE_DIR/migration_conflicts"
else
  printf '  （无）\n'
fi

BLOCKERS=0
printf '\n### 结论\n\n'
if [ "$ALREADY_MERGED" -eq 1 ]; then
  printf -- '- 阻塞：分支 `%s` 已经包含在 `%s` 中，无需合并\n' "$BRANCH" "$MAIN_BRANCH"
  BLOCKERS=1
fi
if [ "$MERGING" -eq 1 ]; then
  printf -- '- 阻塞：该目录有未完成的合并：解决冲突后执行 `git add -A && git merge --continue`，或 `git merge --abort` 放弃\n'
  BLOCKERS=1
elif [ -s "$STATE_DIR/our_dirty" ]; then
  printf -- '- 阻塞：待合并分支所在目录有 %s 个未提交改动\n' "$(count_lines "$STATE_DIR/our_dirty")"
  BLOCKERS=1
fi
if [ -s "$STATE_DIR/dirty_overlap" ]; then
  printf -- '- 阻塞：主工作区有 %s 个未提交文件与本次改动重叠，合并会冲掉用户工作\n' "$(count_lines "$STATE_DIR/dirty_overlap")"
  BLOCKERS=1
fi
if [ -s "$STATE_DIR/migration_conflicts" ]; then
  printf -- '- 阻塞：迁移编号冲突，先重命名本分支的迁移到下一个未占用序号\n'
  BLOCKERS=1
fi

if [ "$BLOCKERS" -eq 1 ]; then
  printf '\n先解决以上阻塞项，再执行合并。\n'
  exit 1
fi

printf '\n可以合并。先在功能分支 `git merge <主分支>` 完成集成，再在主工作区 `git merge --ff-only <分支>`。\n'
exit 0
