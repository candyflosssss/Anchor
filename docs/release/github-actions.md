# GitHub Actions 自动打包与草稿发行

日期：2026-10-11。配置入口：`.github/workflows/release.yml`；复用 `scripts/release.sh`，不另造一套编译/打包规则。

**本文件描述项目配置；实际提交、推送与 GitHub 运行结果以 `docs/updates/` 对应记录为准。项目已采用 AGPL-3.0-only，标签候选包含 Linux/Windows AMD64，必须通过两平台原生门禁。** 本机验证不替代托管 runner 验收。

## 1. 两个入口

| 入口 | 版本 | 结果 | 是否公开 Release |
| --- | --- | --- | --- |
| Actions → Anchor prerelease → Run workflow | `0.0.0-alpha.<run_number>` | 内部评估包，下载 Actions artifact | 否；即便手动选择标签也不创建 Release |
| 推送 `vX.Y.Z-alpha.N` / `vX.Y.Z-beta.N` / `vX.Y.Z-rc.N` 标签 | 去掉 `v` 的标签版本 | 验证、打包、上传 artifact，Windows 原生验收后再创建两平台草稿预发行，附裸程序、审计归档、对应源码和许可/校验文件 | 否；需要人工确认草稿后 Publish |

普通分支 push、PR 和稳定版标签不触发这个发行 workflow。本阶段只支持 Linux/Windows amd64 CLI 便携包，没有 macOS、ARM、桌面安装器、代码签名或自动更新。

## 2. 第一次接入

1. 项目拥有者确认并提交完整源码、发行脚本、`go.mod/go.sum`、CLI/随包文档及 workflow。不要只提交 workflow，不要提交 `dist/`、运行数据库、用户令牌和 `.env`。
2. 推送到默认分支；仓库 Settings → Actions → General 确认 Actions 和这些官方 action 未被组织/仓库策略禁用。workflow_dispatch 的界面入口需要 workflow 出现在默认分支。
3. Actions → **Anchor prerelease** → **Run workflow**，选择已包含配置的分支，先跑内部评估。不需要自建服务器、不需要添加个人 PAT。
4. 成功后在运行页面 **Artifacts** 优先下载 `anchor-linux-amd64-<版本>` 或 `anchor-windows-amd64-<版本>`；GitHub 浏览器下载仍为一层 ZIP，解开后只含一个对应平台程序，不再嵌套归档。Linux 下载文件需 `chmod +x`。`anchor-<run_id>-<run_attempt>` 作为可选完整审计包保留，包含裸程序、文档/依赖声明归档、`release.json`、发行说明和校验清单。artifact 保留 14 天，不是长期用户下载渠道。
5. 检查日志、元数据、实际下载内容，确认本机包和 CI 包的功能一致；哈希因编译环境/时间不同不保证与先前本地包相同。

如果未满足 LICENSE 检查，手动内部评估允许构建，但**标签发行会直接失败**，不会悄悄改为内部包或创建 Release。

## 3. 标签发行

以下操作只能在源码已提交推送、选好许可并确认发行范围后执行。版本示例是新候选，不会替换本地 `alpha.1`：

```bash
# 在仓库中检查；有改动则先完成提交，不要盲目 git add -A
git status --short

# 核对 HEAD 正是准备发行的提交，并已推送到 origin
git log -1 --oneline

# 创建并推送版本标签，只推这个标签
git tag -a v0.1.0-alpha.6 -m "Anchor 0.1.0-alpha.6"
git push origin v0.1.0-alpha.6
```

标签构建要求干净提交和 LICENSE；先下载完整模块图（冷缓存也能收集依赖许可），再串行执行依赖校验、Go race 测试、go vet、发行工具与 CI 门禁测试、两平台编译、对应源码导出（含 vendored 依赖）、Linux 单文件与最终归档程序烟测、SHA256 复核。

本阶段固定 GitHub 托管 `ubuntu-24.04`、Go `1.26.0`（与首批本地包一致）、Python `3.12`；Go 模块和 action 均有锁定来源。官方 actions 固定完整 commit SHA，更新 action/工具链时需要重新验收。Go `go.mod` 是最低语言版本，不代表 CI 自动跟随最新工具链。

构建成功后，独立的写权限 job 下载本次 artifact，复核校验和、目标文件、通过的检查、干净源码提交、版本和验收状态，再用 `GITHUB_TOKEN` 创建 **Draft + Prerelease**。只有这个 job 有 `contents: write`；checkout 不持久化凭据。没有加入个人凭据或额外 secrets。

草稿附 Linux/Windows 裸程序、可选 Linux tar.gz / Windows ZIP、对应源码 tar.gz、LICENSE、THIRD_PARTY_NOTICES.txt、`release.json`、`RELEASE_NOTES.md`、`SHA256SUMS`。`--verify-tag` 不会替你凭空新建标签，也不会静默覆盖已存在的 Release。

## 安装器回归门禁

Linux 安装器离线回归纳入现有发行工具测试；另有 `windows-2025` job，以 `powershell.exe`（Windows PowerShell 5.1）和 `pwsh`（PowerShell 7）矩阵对安装脚本进行原生语法及模拟下载/校验/落盘回归；测试入口通过 `ANCHOR_INSTALLER_TEST_POWERSHELL` 选择运行时，请求的运行时缺失即失败，不静默换用另一个。草稿 job 同时依赖打包、Windows 安装器矩阵与 Windows 核心原生验收成功，未增加公开发布权限或自动发布步骤。测试只用隔离目录和假程序，不请求真实上游，**不等于 Windows 核心运行验收**。配置门禁不代表已经通过；具体运行结果以 Actions 和对应任务记录为准。

公开 Release 可配合 [一键安装脚本](install.md)；Actions 内部 artifact 和草稿不是匿名安装源。

## 4. 从草稿到用户下载

1. 在仓库 Releases 打开新草稿；确认版本、源码提交、附件和更新说明。
2. 核对两平台产物、blockers 为空且 public_publish_ready=true；Windows 原生证据必须匹配本次 workflow run/attempt、源码、版本、输入 manifest 和实际 EXE/ZIP SHA256。内部包、脏代码包和缺少原生证据的包不能晋升。
3. 核对项目许可、依赖声明及本次发行范围；依赖根目录声明的收集不等于法律完整性审查。核对对应源码归档的 revision、源码和 vendored 依赖，并用包内构建指令重建；不能把删掉 Windows 阻塞项当作该平台通过。
4. 在说明中写明目标机实际验证结果与已知限制，保留 prerelease 标识，然后人工 **Publish release**。
5. 从用户可见页面重新下载附件，检查 SHA256 并运行单文件（若选归档再解压），记录本次发布验证。不要只验证 runner 工作目录里的文件。

脚本不自动公开发行，也不把“草稿创建成功”写成“完整产品已发布”。安装、升级、备份和回滚仍见 [随包说明](package-readme.md)。

## 5. 常见失败与重跑

- Actions 按钮/Run workflow 不出现：检查默认分支是否已有 workflow、仓库 Actions 是否启用、账号权限是否足够。
- 标签任务报未满足 LICENSE 检查：由项目拥有者确定许可后提交；对新的源码提交使用新的候选版本标签，不移动旧标签。
- 无法创建 Release：检查组织对 `GITHUB_TOKEN` 的限制及 `contents: write`；构建 artifact 仍可在运行页面下载，不要改为写入个人 token 规避策略。
- Windows 包能编译但不能用：原生门禁失败时不生成草稿；修复源码后用新标签重建，不移动旧标签。
- 包已生成但后续失败：build job 尽可能上传 `dist/` 下的 partial 诊断，失败包不创建 Release；若没有产物，upload 只警告，不掩盖原始错误。
- 同标签草稿已存在或上传中断：不会覆写。检查现有草稿和附件；部分草稿不应发布，人工处理后决定重试或推进新的候选版本。不要强推标签/自动删除草稿。
- 同一引用的任务串行，不取消正在运行的发行；重跑使用独立 artifact 名。旧运行的 artifact 不会被新运行误用。

CI 日志、Step summary、artifact 的 release.json/更新说明是每次构建的证据，失败日志也需要保留。此 workflow 不自动把生成包或记录提交回仓库，避免机器人提交与递归构建。

## 官方参考

- [GitHub Actions workflow syntax](https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-syntax)：触发条件、permissions、job 与 concurrency。
- [GitHub Actions workflow_dispatch](https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-syntax#onworkflow_dispatch)：默认分支与手动触发规则。
- [GitHub CLI gh release create](https://cli.github.com/manual/gh_release_create)：`--draft`、`--prerelease`、`--verify-tag`。

官方文档应以 GitHub 当前内容为准；本项目行为以 workflow 和发行脚本为准。

## 两平台原生验收与 AGPL 对应源码（2026-10-11）

项目采用 AGPL-3.0-only。手动运行仍生成两平台内部包，不创建草稿。标签生成干净的两平台公开候选，附对应源码（含 vendored 依赖）、LICENSE 和依赖声明。

Linux 构建结束时 Windows 仍是 `native runtime acceptance pending`，不能直接公开。独立 `native-windows` job 在 Windows Server 2025 AMD64 下载同次 run/attempt 的完整产物，先复核所有哈希与元数据，再对裸 EXE 和 ZIP 中相同字节的 EXE 分别运行隔离烟测：初始化/重入、私有状态 ACL、独占实例锁、启动/Context/status、中文供应商/模型映射、模拟网关、凭据日志脱敏、CTRL_BREAK 正常退出、监听释放和状态重开。没有请求付费上游，也不碰用户 Agent 配置。

`scripts/windows_acceptance.py` 只在原生 Windows AMD64 上产出证据。草稿 job 必须等到原生验收和两个 PowerShell 安装器矩阵成功，再下载同次证据；`ci_release.py finalize-windows` 核对 run/attempt、revision、版本、原始 manifest、两产物哈希及全部检查后，才完成 Windows 验收元数据并重算清单。默认 `verify` 拒绝未验收的公开双平台包。不接受其他阻塞项，不将旧内部包改名上传。

以上描述的是门禁配置，不将尚未执行的检查写成通过；具体发行结果见任务记录和对应 Release 的 release.json。没有代码签名、桌面 UI 或自启动，不表示所有 Windows 版本或真实供应商均已验收。
