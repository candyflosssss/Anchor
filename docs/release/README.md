# Anchor CLI 发行流程

- 日期：2026-10-11
- 范围：Linux 发行机上的单文件核心/CLI 预发行程序，以及可选的文档/依赖声明归档；Linux amd64 和 Windows amd64。
- 当前入口：`scripts/release.sh`。**不自动提交、打标签、上传或公开发布。**
- 已配置 [GitHub Actions 自动打包与草稿发行](github-actions.md)，实际托管运行结果见任务记录；不会自动公开发布。桌面安装器、自启动、自动更新和签名尚未实现。当前源码默认版本仍为 0.1.0-dev，发行包注入指定版本，不需要改动开发版本常量。

## 一键安装

提供 [Linux / Windows 安装脚本](install.md)，自动下载并校验公开 Release 的单文件，默认用户级安装，可固定版本/显式替换，不改 Agent 或状态配置。Linux / Windows Alpha `0.1.0-alpha.6` 已公开，Windows 原生门禁及两平台匿名下载已验证，Linux 原始远程一键安装已实际验证成功，这不等于桌面安装器、自启动或自动升级。

## 只用一个程序

运行 Anchor 只需要本平台的一个文件，不需要安装 Go、Python、SQLite 驱动或额外资源目录。核心、CLI 和认证 helper 是同一个程序，不是三个组件。

- Linux：`anchor_<版本>_linux_amd64`；可放到固定路径并命名为 `anchor`，下载后 `chmod +x`。
- Windows：`anchor_<版本>_windows_amd64.exe`；可命名为 `anchor.exe`，在 PowerShell 调用。
- 本地构建在版本目录直接生成这些文件；CI 按平台各上传一个仅含可执行文件的 artifact。Actions 的浏览器下载仍由 GitHub 包成一层 ZIP，解开后只有程序，不再嵌套 tar.gz/ZIP。将来标签草稿也附带裸可执行文件。
- 原来的完整归档保留供文档、依赖许可/声明、源码信息和审计使用，**不是运行时依赖**。单文件交付不取消项目许可和公开发行门禁；再分发仍需履行适用许可义务。
- 数据库/配置/凭据与程序分离，首次使用仍需 `init` 和供应商/模型配置；不把秘密或用户数据塞进可执行文件。批准 Codex Binding 前先放到稳定路径，之后不要随意移动认证 helper 指向的程序。

## 1. 发行前检查

确认发行范围和更新说明；核对最新 CLI 大全与变更历史，不把未实现功能列入发行能力。

公开发行候选应来自已提交、可定位的代码和锁定的 go.mod/go.sum；选择明确的项目发行许可后才允许作为公开发行。脚本不会替项目拥有者选择 LICENSE。

项目已采用 [AGPL-3.0-only](../license.md)。公开候选不使用 `--internal`，并附对应源码和声明；内部评估仍使用 `--internal`；仅当本地工作树确有未提交改动时再加 `--allow-dirty`。其 Git revision 不是这批未提交代码的完整标识；release.json 同时记录实际构建输入摘要。

## 2. 开发者怎么执行

需要 Python 3.12+、满足 go.mod 的 Go 工具链、Git，以及 Go race 测试所需的 C 编译器。当前发行机限定 Linux amd64；本地 Windows 仅交叉构建；Actions 另在 Windows Server 2025 验收实际 EXE 和 ZIP，通过后才允许创建双平台草稿。

从仓库根目录执行：

```bash
# 只看计划：不编译、不测试、不创建产物
./scripts/release.sh --version 0.1.0-alpha.1 --plan

# 只做检查：不创建分发包
./scripts/release.sh --version 0.1.0-alpha.1 --internal --allow-dirty --check

# 构建两个系统的内部测试包
./scripts/release.sh --version 0.1.0-alpha.1 --internal --allow-dirty

# 从干净提交构建 Linux 公开候选（须使用未占用的新版本）
./scripts/release.sh --version 0.1.0-alpha.3 --targets linux-amd64
```

版本只接受 `X.Y.Z-alpha.N`、`X.Y.Z-beta.N` 或 `X.Y.Z-rc.N`，当前里程碑不接受稳定版。版本目录已存在时拒绝覆盖；每次需要新产物使用新版本，不删除已有版本掩盖替换。

| 选项 | 默认 / 行为 |
| --- | --- |
| `--version` | 必填；注入可执行文件及发行元数据 |
| `--targets` | linux-amd64,windows-amd64；只支持这两项，不可重复 |
| `--internal` | 默认关闭；明确内部评估，允许尚缺项目 LICENSE；产物不能被脚本标为公开发行就绪 |
| `--allow-dirty` | 默认关闭，只能配合 internal；记录 dirty 和构建输入摘要，不假装构建来自干净 Git tag |
| `--plan` | 默认关闭；只输出计划 |
| `--check` | 默认关闭；只做源代码检查，与 plan 互斥 |

当前机器自动通过 resource-guard 预计 2048 MiB/硬限 3072 MiB/Swap 256 MiB 串行执行；guard BLOCKED 即停止。Go 并发固定为 GOMAXPROCS=2、-p=1，Go/测试临时数据在 guard 的磁盘目录。不要绕过包装脚本直接执行重型发行任务。

## 3. 一条构建命令实际做什么

1. 检查版本、目标、Git 状态与许可；禁止静默覆盖现有发行目录。
2. 下载完整 Go 模块图后执行 `go mod verify`、Go race 全量测试、go vet、发行工具单元测试；失败不继续编译。
3. 清除环境交叉编译/任意构建标志，禁用外部 Go workspace 和自动切换工具链。
4. 按目标串行编译：CGO_ENABLED=0、trimpath、禁用自动 VCS 嵌入，通过 ldflags 注入本次版本。
5. 将构建出的程序原样复制为版本目录下的单文件产物，校验复制哈希并执行 Linux 单文件烟测；Windows 明确标为交叉编译。另为可选归档仅拷贝白名单文件：可执行文件、随包说明、指令大全、构建信息、项目许可/内部提示及依赖模块根目录许可/声明。**不遍历打包工作树，不拷贝用户状态或秘密。**依赖声明的收集不是法律完整性审查。
6. 另生成可选 Linux tar.gz / Windows zip；解压本机 Linux 的最终归档后，用真正的包内程序跑隔离状态/模拟上游/停服烟测，并核对注入版本。本地 Windows 标为交叉编译完成、原生验收待办；Actions 的后续 Windows job 完成准确产物验收后才可晋升，见 GitHub Actions 文档。
7. 非内部构建另外导出本次 revision 的已提交源码，拒绝链接/特殊文件，运行 `go mod vendor` 收集对应依赖源码，生成 `PUBLIC-BUILD.md` 和源码归档；输出独立 LICENSE 和 THIRD_PARTY_NOTICES.txt。再检查构建输入摘要未变化，生成 release.json、更新说明和 SHA256SUMS，以版本目录保留产物。

成功输出 `status: packaged` 表示本地打包成功，**不表示已上传或具备公开发行条件**。release.json 的 `public_publish_ready` 和 `publish_blockers` 明确记录检查边界；它们也不是许可、人工验收或发布授权的替代品。

打包前失败清理任务自己的临时目录；已有归档而后续验收失败时保留 `VERSION.failed-*`，manifest 标记 partial，退出非零，不能发布。进程被强制终止可能留下隐藏 staging/锁目录，确认没有相关发行进程后才按准确路径处理，不盲目删 dist。

## 4. 产物与校验

```text
dist/0.1.0-alpha.1/
  anchor_0.1.0-alpha.1_linux_amd64         # Linux 单文件核心
  anchor_0.1.0-alpha.1_windows_amd64.exe   # Windows 单文件核心
  anchor_0.1.0-alpha.1_linux_amd64.tar.gz
  anchor_0.1.0-alpha.1_windows_amd64.zip
  release.json
  RELEASE_NOTES.md
  SHA256SUMS
```

`dist/` 是本地忽略的构建产物目录，不是线上发布目录。release.json 记录版本、时间、源代码摘要、Go 版本、检查、每个包的大小/哈希/目标验证状态、依赖清单与发布阻塞项。

Linux 在产物目录运行 `sha256sum -c SHA256SUMS`。Windows 可用 PowerShell `Get-FileHash -Algorithm SHA256 文件路径`，与可信发布页的对应摘要比较。校验文件不等于签名；未做代码签名时不要声称已签名或要求用户关闭安全检查。

## 5. 公开发行与用户下载

这是**后续人工发布步骤，不由脚本自动执行**：

1. 项目拥有者确定发行许可；把本次源码、依赖锁、文档与发行说明提交，获得干净、可定位的版本。不要提交 dist 或用户状态。
2. 用新版本号重建发行候选，不直接改旧内部包的 manifest 或删除阻塞提示。
3. 对本次拟公开的各目标做真实运行验收；可先只发行 Linux，并明确排除未验收 Windows。记录准确的系统/架构、ACL/锁、启动、退出和状态目录结果；尚未验收的目标不得宣传为已验证。
4. 核对归档、校验、更新说明与随包指令版本一致；确定升级/迁移影响和已知限制。
5. 经拥有者确认后，在对应代码 revision 上创建版本标签；在 GitHub Releases 创建草稿，上传这批最终包、对应源码归档、LICENSE、THIRD_PARTY_NOTICES.txt、release.json、更新说明和校验文件。
6. 检查草稿的目标 commit、版本、资产、预发行标记和说明；显式确认后发布。发布页自动提供的源码 ZIP 不是用户可执行包。
7. 从发布页重新下载、核对哈希并验收，之后才认定线上发行完成。仅本地生成包不算线上发布。

用户优先下载对应平台的单文件程序，校验并运行；完整归档是可选形式。不需要安装 Go/拉取源码。公开 Release 可用后也可使用 [一键安装脚本](install.md) 完成下载/校验/落盘；桌面安装器和 UI 仍未实现。

## 6. 升级、回退与记录

手动升级/卸载说明见 [随包说明](package-readme.md)。核心原则：停止进程后保护并备份完整状态；数据库升级必须有明确迁移契约；不能把“替换二进制”当作所有版本都可回退的保证。

执行任务的开发者/agent应在每次实际构建分发包或线上发布后，按项目 record-task-update 规则生成一份 release 记录，包含 channel/version/build、产物、发布范围和真实验证；部分构建也记录 partial。发行脚本不替使用者自动生成机器专属更新记录，也不修改 Git。

本次任务的构建验证与产物位置以对应 docs/updates release 记录为准。已在同一脚本基础上添加 GitHub Actions 配置；托管执行结果见任务记录；项目许可已落实；仅 Linux 的候选不再携带 Windows 未验收阻塞，草稿仍须审核后公开。

公开构建还生成 `anchor_<版本>_source.tar.gz`、`LICENSE` 和 `THIRD_PARTY_NOTICES.txt`，全部纳入 SHA256SUMS。安装器只安装一个可执行文件；再分发时仍须履行许可义务。
