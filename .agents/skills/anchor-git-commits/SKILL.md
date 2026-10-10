---
name: anchor-git-commits
description: 在 Anchor 仓库的合理节点提交与推送 Git：判断提交时机、按任务拆分暂存、提交前检查、推送 main，并避免意外触发发行 Actions 或带入他人改动。在 Anchor 中完成可验证的代码、文档、配置改动，或用户要求提交、上传、整理工作区时使用；不用于其他仓库，也不负责打标签发版。
---

# Anchor Git 提交节点

Anchor 是**公开仓库**（`candyflosssss/Anchor`，AGPL）。本 skill 是本项目对提交的明确授权：在下述节点可以自动提交并推送 `main`，不必每次询问。用户在当前对话中要求“不提交”“只本地提交”或“先别推送”时，以用户为准。通用的 `commit-push-private-repo` 因仓库公开而排除本项目，不能据此扩大或收窄本 skill 的规则。

## 会触发 Actions 的操作

唯一的工作流是 `.github/workflows/release.yml`，只在以下情况运行：

- 推送匹配 `v*-alpha.*`、`v*-beta.*`、`v*-rc.*` 的标签：构建双平台候选、执行 Windows 原生验收，并创建草稿 Release（`gh release create --draft --prerelease`）。
- 手动 `workflow_dispatch`（如 `gh workflow run`）：生成内部测试 artifact。

因此普通提交和 `git push origin main` 不会触发任何运行。除非用户明确要求本轮发版，否则不要执行：`git tag`、`git push --tags`、`git push --follow-tags`、推送任何 `refs/tags/*`、`gh workflow run`、`gh release *`。发版走 `docs/release/` 流程，不属于本 skill。

如果本轮修改了 `.github/workflows/`，推送前重新核对触发条件；一旦新增 branch push、`pull_request`、`schedule` 等触发，推送 `main` 就会运行 CI。这时要在回复中说明，并先征得用户同意再推送。

## 合理的提交节点

满足以下任一条件，且相应验证已经通过，就提交：

- 一个完整的功能或修复：实现、测试，以及按项目规则必须同步的 CLI 文档、设计文档放在同一个提交里。
- 长任务中一个可以独立回退、可用的里程碑。
- 开始高风险重构、依赖升级或切换到另一项工作之前，先保存已经验证过的成果。
- 任务结束：任务记录（`docs/updates/`）随最后一个提交一起提交。

不要提交：构建失败或测试失败的中间状态、逐个文件的碎片提交、未经用户确认的大规模格式化或无关改动。验证没通过时，除非用户明确接受，否则不提交。

一次积压了多个功能时，按功能拆成几个提交，每个都应能独立构建。验证中间提交时用独立 worktree（放在 `resource_guard.sh tempdir` 创建的目录里，用完 `git worktree remove`），**不要用 `git stash`、`reset`、`checkout` 收起工作区**：这些操作被中断时容易让其他改动暂时消失，或与并行任务冲突。

## 暂存与提交

1. 先 `git status --short` 和 `git diff`，确认哪些改动属于当前任务。同一工作区可能有其他 agent 或任务在并行修改；不属于本任务、或无法和本任务改动分开的文件，保留不提交，并在回复中说明。
2. 用明确路径暂存，不在存在无关改动时使用 `git add -A` 或 `git add .`。
3. 运行检查脚本（以仓库根目录为工作目录）：

   ```bash
   python3 .agents/skills/anchor-git-commits/scripts/git_preflight.py staged
   ```

   它会拦截被忽略的产物和本地状态（`dist/`、`node_modules/`、前端 `dist/`、`test-results/`、`state.db*`、`owner.token`、密钥文件、日志），扫描新增行里的常见密钥格式，并提示大文件和工作流改动。出现 ERROR 时先修正再提交，不要绕过。
4. 提交信息沿用仓库现有约定：英文 Conventional Commits，`type(scope): summary`，例如 `feat(core): …`、`fix(ci): …`、`docs: …`。正文简要列出要点；涉及未验收能力时（如原生 GUI），在正文中写明。
5. 不使用 `--no-verify`、`--amend` 修改已推送的提交，也不改写已推送的历史。

## 推送

在任务结束或里程碑节点推送，不必每个提交都推：

```bash
python3 .agents/skills/anchor-git-commits/scripts/git_preflight.py push
git push origin main
```

脚本会先 fetch，确认是快进推送、列出将推送的提交，并检查工作流触发条件、`push.followTags` 等会顺带推送标签的配置。远端已有新提交时，先 `git pull --rebase`（只 rebase 本地未推送的提交）后重新验证，绝不 `--force`。推送后用 `git ls-remote origin refs/heads/main` 核对远端 HEAD。

## 汇报

最终回复列出提交哈希与标题、是否已推送及远端 HEAD、未提交而保留的文件及原因，以及实际执行过的验证。不要把“已推送”写成“已发布”：推送 `main` 不会产生 Release。
