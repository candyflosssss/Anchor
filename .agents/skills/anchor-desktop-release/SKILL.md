---
name: anchor-desktop-release
description: 交付 Anchor 桌面开发版安装包与 dev 在线更新。用户要求打包桌面测试版、加快迭代、走在线更新或发布桌面开发版时使用；不用于纯功能修改、CLI 发行或其他项目。
---

# Anchor 桌面开发版交付

把已授权的桌面迭代交付做到用户能取得新版本。已有在线更新入口时，在线交付应包含安装包和更新清单的实际投放；只生成本地包或推送 main 不算已发布。

## 范围与依据

- 沿用当前会话已确定的交付范围。用户已要求走在线更新时，完成打包、上传、发布和必要下载检查，不因这些步骤分开执行而重复要求确认。仅要求本地包、暂停发布或只改文档时遵守该范围；本 skill 本身不授予新的公开发布权限，也不自动安装到用户机器。
- 开发迭代使用 `dev` 渠道、`X.Y.Z-dev.desktop.N` 版本、GitHub prerelease、非 latest；alpha/beta/rc/stable 发行另按其流程，不能借用 dev 跳过其门禁。
- 打包参数、产物与平台边界读取[桌面发行说明](../../../docs/release/desktop.md)；默认源、清单格式和投放步骤读取[在线更新](../../../docs/design/online-updates.md)。复用现有脚本，不另写一套打包器或更新器。
- 涉及关闭窗口、退出或更新交接时读取[桌面生命周期](../../../docs/design/desktop-runtime.md)。这些产品行为留在设计文档维护，不扩成另一套发布规则。

## 完成交付

1. **确定新版本与源码。** 检查本地版本目录及远端已发布／草稿版本，递增版本号，保留旧包。按[Git skill](../anchor-git-commits/SKILL.md)提交本轮功能，从干净、可追溯的源码构建；不收起或混入其他任务改动。版本不写死为文档示例值。
2. **串行打包。** 使用 `scripts/package-desktop.sh --version <新版本>` 生成 Windows AMD64 安装器和 Ubuntu24.04 AMD64 `.deb`。本机遵循 `resource-guard`：该 wrapper 已接入 guard，其他重操作也受限执行，临时数据放磁盘目录。完成后读取本次 `SOURCE.json`、`BUILD.json` 和最终产物，不重验历史包。
3. **生成线上清单。** 默认源是 `candyflosssss/Anchor` 的 GitHub Releases，安装包与 `update-desktop.json` 必须属于同一版本 Release。用 `scripts/update_manifest.py --layout flat`，base URL 为该版本 Release 的 download 目录，文件名与实际附件一致。包含 runtime 更新能力的新版本必须为两平台安装器分别传 `--runtime TARGET=FILE`，保留旧客户端安装器兼容；同版独立 CLI binary 另生成 `update-cli.json`，供 SSH 被控机更新。不得只发布旧格式清单导致新版退回安装器。打包脚本的 `--update-base-url` 生成 target 子目录布局，不能直接当 GitHub 的 flat 附件地址使用。自定义源则按用户选定的可信地址和布局处理。
4. **上传并发布已授权版本。** 附件包括两平台安装包、两平台唯一命名的 desktop runtime 与 CLI binary、update-desktop/update-cli 清单、对应构建提交的源码（含 vendored Go 依赖）、许可／构建元数据和校验清单。先创建指向实际构建 revision 的 draft prerelease，上传齐全后检查附件状态，再公开为 prerelease 并保持 `--latest=false`。Release 正文用文件传给 `--notes-file`。不替换既有同名发布资产或移动旧标签；不把文档提交误当构建源码。构建元数据中的验证状态保留原始事实，发布状态另行记录。
5. **确认用户能在线取得。** 在 guard 创建的独立磁盘临时数据目录，用本轮源码 CLI 执行一次 `go run -p=2 ./cmd/anchor --data-dir <隔离目录> update download --channel dev --product desktop --current-version <上一版>`，Go 执行同样走 guard 并设置 `GOMAXPROCS=2`；不要误用尚无 update 命令的旧 alpha.6 CLI。它已包含检查、下载、大小与 SHA256 校验，不再额外重复 `check`。新版宿主选择 runtime 后，当前平台真实下载检查必须核对其 kind／版本与运行文件摘要，旧客户端仍需能取得安装器；CLI feed 也核对版本／目标／摘要。另一平台核对安装器与两个运行程序的 GitHub 附件 digest 与本地哈希；若远端不提供 digest，再下载并计算哈希。检查不使用真实数据目录，不调用 `apply` 或安装器。
6. **同步记录并交付。** 更新桌面发行说明、在线更新文档以及受影响的 README／CLI 覆盖状态；用 `record-task-update` 为同一轮打包及投放生成一份 release 记录。记录实际版本、build、artifact、投放范围与检查结果，再按 Git skill 提交推送。仅文档改动不重新打包。回复优先给版本和应用内更新路径，不再让已有更新入口的用户到本地找安装包。

## 只做必要检查

- 按改动选择编译、类型／契约或直接相关的定向检查；已有结果仍覆盖最终代码时复用。打包流程本身包含生产编译，不额外重复全套检查。
- dev 快速迭代不默认跑全量、race、浏览器或双平台原生安装验收。用户要求或具体失败证据确实需要时才扩大范围，未执行的验收如实写明。
- 发布后的默认源发现与下载完整性是在线交付的必要检查；下载成功不等于系统安装成功，编译通过不等于原生窗口行为已验收。不请求真实付费上游或改动真实 Agent 配置。
- 当前 dev 标签不匹配 alpha/beta/rc 发行工作流；执行前以实际工作流配置为准，不为快速交付额外运行 Actions。普通 main 推送与发布是不同动作。
- 上传或网络失败先确认已存在的 Release／资产状态，再从失败步骤恢复；不盲目新建同版发布、覆盖资产或重打包。内容不匹配、权限不足或无法恢复时保留已完成产物，报告具体阻塞；已有安装包但投放或必要下载检查未完成，release 记录使用 `partial`，不能写成在线交付成功。

## 用户入口

默认未配置渠道为 `alpha`，桌面 dev 版需要选择「设置 → 在线更新 → dev · 快速测试 → 检查更新 → 下载更新 → 安装更新」。不要为了验收修改用户的更新配置。已有入口可直接在线获取；没有入口的旧版才需要一次人工安装。已发布版本、数据库迁移限制和真实验收状态以本轮产物及发行文档为准。
