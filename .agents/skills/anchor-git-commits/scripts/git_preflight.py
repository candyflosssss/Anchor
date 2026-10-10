#!/usr/bin/env python3
"""Anchor 提交/推送前检查。

  staged  检查暂存区：拦截产物、本地状态和疑似密钥，提示大文件与工作流改动。
  push    推送前检查：快进、待推送提交、标签推送配置与 Actions 触发条件。

只读，不修改仓库（push 模式会执行 git fetch）。有 ERROR 时退出码为 1。
"""
from __future__ import annotations

import re
import subprocess
import sys

FORBIDDEN = [
    (r"^dist/", "发行产物目录"),
    (r"(^|/)node_modules/", "npm 依赖"),
    (r"^desktop/frontend/dist/", "前端构建产物"),
    (r"(^|/)(test-results|playwright-report)/", "测试产物"),
    (r"(^|/)state\.db", "本地 SQLite 状态"),
    (r"(^|/)owner\.token$", "本机拥有者令牌"),
    (r"(^|/)instance\.lock$", "实例锁"),
    (r"\.(pem|key|p12|pfx)$", "密钥/证书文件"),
    (r"(^|/)\.env($|\.(?!example$)(?!.*\.example$))", "环境变量文件"),
    (r"\.log$", "日志"),
    (r"(^|/)coverage\.out$|\.test$", "测试/覆盖率二进制"),
]
SECRETS = [
    (r"sk-(?:proj-|ant-)?[A-Za-z0-9_-]{20,}", "API key (sk-)"),
    (r"gh[pousr]_[A-Za-z0-9]{30,}", "GitHub token"),
    (r"github_pat_[A-Za-z0-9_]{20,}", "GitHub fine-grained token"),
    (r"AKIA[0-9A-Z]{16}", "AWS access key"),
    (r"xox[abprs]-[A-Za-z0-9-]{10,}", "Slack token"),
    (r"-----BEGIN [A-Z ]*PRIVATE KEY-----", "private key"),
    (r"(?i)bearer\s+[A-Za-z0-9._~+/-]{32,}", "bearer token"),
]
LARGE_WARN = 5 * 1024 * 1024
TAG_PATTERN = re.compile(r"^v.*-(alpha|beta|rc)\..*")

errors: list[str] = []
warnings: list[str] = []


def git(*args: str, check: bool = True) -> str:
    result = subprocess.run(["git", *args], capture_output=True, text=True)
    if check and result.returncode != 0:
        sys.exit(f"git {' '.join(args)} 失败：{result.stderr.strip()}")
    return result.stdout


def mask(value: str) -> str:
    return value[:6] + "…" + f"({len(value)} chars)"


def workflow_triggers(text: str) -> tuple[set[str], bool]:
    """返回 (事件集合, push 是否会因分支推送触发)。启发式解析 YAML 顶层 on。"""
    lines = text.splitlines()
    events: set[str] = set()
    push_on_branches = False
    for i, line in enumerate(lines):
        m = re.match(r"^['\"]?on['\"]?\s*:\s*(.*)$", line)
        if not m:
            continue
        inline = m.group(1).split("#")[0].strip()
        if inline:
            events |= {e.strip() for e in inline.strip("[]").split(",") if e.strip()}
            return events, "push" in events
        block: list[str] = []
        for nxt in lines[i + 1:]:
            if nxt.strip() and not nxt.startswith((" ", "\t")):
                break
            block.append(nxt)
        indent = min((len(b) - len(b.lstrip()) for b in block if b.strip() and not b.lstrip().startswith("#")), default=2)
        current = None
        push_body: list[str] = []
        for b in block:
            if not b.strip() or b.lstrip().startswith("#"):
                continue
            level = len(b) - len(b.lstrip())
            if level == indent:
                current = b.strip().split(":")[0].strip("'\"")
                events.add(current)
            elif current == "push":
                push_body.append(b.strip())
        if "push" in events:
            body = " ".join(push_body)
            has_tags = re.search(r"\btags(-ignore)?\s*:", body)
            has_branches = re.search(r"\bbranches(-ignore)?\s*:", body)
            push_on_branches = bool(has_branches) or not has_tags
        return events, push_on_branches
    return events, False


def check_workflows(ref: str | None) -> None:
    """ref=None 读取暂存区版本，否则读取指定提交中的版本。"""
    if ref is None:
        paths = git("ls-files", "--cached", "--", ".github/workflows").split()
    else:
        paths = git("ls-tree", "-r", "--name-only", ref, "--", ".github/workflows", check=False).split()
    for path in paths:
        name = path.rsplit("/", 1)[-1]
        if not name.endswith((".yml", ".yaml")):
            continue
        text = git("show", f"{ref or ''}:{path}")
        events, push_on_branches = workflow_triggers(text)
        if not events:
            warnings.append(f"无法解析 {name} 的触发条件，请人工核对")
            continue
        risky = events & {"pull_request", "pull_request_target", "schedule", "workflow_run", "merge_group"}
        if push_on_branches:
            errors.append(f"{name} 会在分支推送时运行（push 未限定为仅标签）；推送 main 会触发 CI，先征得用户同意")
        if risky:
            warnings.append(f"{name} 含有触发 {sorted(risky)}，合并/定时后可能运行")
        print(f"INFO 工作流 {name} 触发：{sorted(events)}")


def staged() -> None:
    files = [f for f in git("diff", "--cached", "--name-only", "-z", "--diff-filter=ACMR").split("\0") if f]
    if not files:
        errors.append("暂存区为空")
        return
    for path in files:
        for pattern, label in FORBIDDEN:
            if re.search(pattern, path):
                errors.append(f"{path}：{label}，不应提交")
                break
        size = git("cat-file", "-s", f":{path}", check=False).strip()
        if size.isdigit() and int(size) > LARGE_WARN:
            warnings.append(f"{path}：{int(size) / 1048576:.1f} MiB，确认确实需要入库")
        if path.startswith(".github/workflows/"):
            warnings.append(f"{path}：修改了工作流，推送前必须核对触发条件")
    current = None
    line_no = 0
    for line in git("diff", "--cached", "-U0", "--no-color", "--no-ext-diff").splitlines():
        if line.startswith("+++ "):
            current = line[6:] if line.startswith("+++ b/") else None
        elif line.startswith("@@"):
            m = re.search(r"\+(\d+)", line)
            line_no = int(m.group(1)) - 1 if m else 0
        elif line.startswith("+") and current:
            line_no += 1
            for pattern, label in SECRETS:
                m = re.search(pattern, line)
                if m:
                    errors.append(f"{current}:{line_no}：疑似 {label} {mask(m.group(0))}")
    print(f"INFO 暂存文件 {len(files)} 个")
    if any(f.startswith(".github/workflows/") for f in files):
        check_workflows(None)


def push() -> None:
    branch = git("rev-parse", "--abbrev-ref", "HEAD").strip()
    if branch != "main":
        warnings.append(f"当前分支是 {branch}，本 skill 默认只推送 main")
    git("fetch", "--quiet", "origin")
    remote = f"origin/{branch}"
    if git("rev-parse", "--verify", "--quiet", remote, check=False).strip():
        if subprocess.run(["git", "merge-base", "--is-ancestor", remote, "HEAD"]).returncode != 0:
            errors.append(f"{remote} 不是 HEAD 的祖先：不能快进。先 git pull --rebase 并重新验证，禁止 --force")
        commits = git("log", "--oneline", f"{remote}..HEAD").strip()
        print("INFO 将推送的提交：" + ("\n  " + commits.replace("\n", "\n  ") if commits else "无"))
        changed = git("diff", "--name-only", f"{remote}..HEAD", "--", ".github/workflows").split()
        if changed:
            warnings.append(f"待推送提交修改了工作流：{changed}")
    else:
        warnings.append(f"远端没有 {remote}，将创建新分支")
    if git("config", "--bool", "push.followTags", check=False).strip() == "true":
        errors.append("push.followTags=true：普通 push 也会带上标签，可能触发发行工作流")
    if git("config", "--get", "remote.origin.mirror", check=False).strip() == "true":
        errors.append("remote.origin.mirror=true：push 会同步全部引用（含标签）")
    refspecs = git("config", "--get-all", "remote.origin.push", check=False).split()
    if refspecs:
        warnings.append(f"remote.origin.push 自定义了 refspec：{refspecs}")
        if any("tags" in r for r in refspecs):
            errors.append("remote.origin.push 包含标签 refspec")
    remote_tags = {l.split("refs/tags/")[1].removesuffix("^{}") for l in git("ls-remote", "--tags", "origin").splitlines() if "refs/tags/" in l}
    pending = [t for t in git("tag", "--list").split() if t not in remote_tags]
    if pending:
        release = [t for t in pending if TAG_PATTERN.match(t)]
        warnings.append(f"本地有未推送的标签 {pending}（普通 push 不会推送）" + (f"；其中 {release} 会触发发行工作流，切勿推送" if release else ""))
    if git("status", "--porcelain").strip():
        warnings.append("工作区仍有未提交改动，它们不会被推送")
    check_workflows("HEAD")


def main() -> int:
    if len(sys.argv) != 2 or sys.argv[1] not in {"staged", "push"}:
        print(__doc__)
        return 2
    {"staged": staged, "push": push}[sys.argv[1]]()
    for w in warnings:
        print(f"WARN  {w}")
    for e in errors:
        print(f"ERROR {e}")
    print("FAILED" if errors else "OK")
    return 1 if errors else 0


if __name__ == "__main__":
    sys.exit(main())
