# Anchor CLI 发行流程

- 日期：2026-10-10
- 范围：Linux 发行机上的本地 CLI 预发行包；Linux amd64 和 Windows amd64。
- 当前入口：`scripts/release.sh`。**不自动提交、打标签、上传或公开发布。**
- 已配置 [GitHub Actions 自动打包与草稿发行](github-actions.md)，实际托管运行结果见任务记录；不会自动公开发布。桌面安装器、自启动、自动更新和签名尚未实现。当前源码默认版本仍为 0.1.0-dev，发行包注入指定版本，不需要改动开发版本常量。

## 1. 发行前检查

确认发行范围和更新说明；核对最新 CLI 大全与变更历史，不把未实现功能列入发行能力。

公开发行候选应来自已提交、可定位的代码和锁定的 go.mod/go.sum；选择明确的项目发行许可后才允许作为公开发行。脚本不会替项目拥有者选择 LICENSE。

目前项目缺少 LICENSE，因此只制作显式的内部测试包：`--internal`；仅当本地工作树确有未提交改动时再加 `--allow-dirty`。其 Git revision 不是这批未提交代码的完整标识；release.json 同时记录实际构建输入摘要。

## 2. 开发者怎么执行

需要 Python 3.12+、满足 go.mod 的 Go 工具链、Git，以及 Go race 测试所需的 C 编译器。当前发行机限定 Linux amd64；Windows 是交叉构建目标，不代表已通过 Windows 验收。

从仓库根目录执行：

```bash
# 只看计划：不编译、不测试、不创建产物
./scripts/release.sh --version 0.1.0-alpha.1 --plan

# 只做检查：不创建分发包
./scripts/release.sh --version 0.1.0-alpha.1 --internal --allow-dirty --check

# 构建两个系统的内部测试包
./scripts/release.sh --version 0.1.0-alpha.1 --internal --allow-dirty

# 如只需本机 Linux 包
./scripts/release.sh --version 0.1.0-alpha.2 --targets linux-amd64 --internal --allow-dirty
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
5. 仅拷贝白名单文件：可执行文件、随包说明、指令大全、构建信息、项目许可/内部提示及依赖模块根目录许可/声明。**不遍历打包工作树，不拷贝用户状态或秘密。**依赖声明的收集不是法律完整性审查。
6. Linux tar.gz / Windows zip；解压本机 Linux 的最终归档后，用真正的包内程序跑隔离状态/模拟上游/停服烟测，并核对注入版本。Windows 标为交叉编译完成、真机验收待办。
7. 再检查构建输入摘要未变化，生成 release.json、更新说明和 SHA256SUMS，以版本目录保留产物。

成功输出 `status: packaged` 表示本地打包成功，**不表示已上传或具备公开发行条件**。release.json 的 `public_publish_ready` 和 `publish_blockers` 明确记录检查边界；它们也不是许可、人工验收或发布授权的替代品。

打包前失败清理任务自己的临时目录；已有归档而后续验收失败时保留 `VERSION.failed-*`，manifest 标记 partial，退出非零，不能发布。进程被强制终止可能留下隐藏 staging/锁目录，确认没有相关发行进程后才按准确路径处理，不盲目删 dist。

## 4. 产物与校验

```text
dist/0.1.0-alpha.1/
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
3. 对 Linux/Windows 各目标做真实运行验收，记录准确的系统/架构、ACL/锁、启动、退出和状态目录结果；尚未验收的目标不得宣传为已验证。
4. 核对归档、校验、更新说明与随包指令版本一致；确定升级/迁移影响和已知限制。
5. 经拥有者确认后，在对应代码 revision 上创建版本标签；在 GitHub Releases 创建草稿，上传这批最终包、release.json、更新说明和校验文件。
6. 检查草稿的目标 commit、版本、资产、预发行标记和说明；显式确认后发布。发布页自动提供的源码 ZIP 不是用户可执行包。
7. 从发布页重新下载、核对哈希并验收，之后才认定线上发行完成。仅本地生成包不算线上发布。

用户拿到对应平台的包，校验、解压、按随包说明运行；不需要安装 Go/拉取源码。安装器和桌面 UI 是后续发行形态，不与当前 CLI 便携包混淆。

## 6. 升级、回退与记录

手动升级/卸载说明见 [随包说明](package-readme.md)。核心原则：停止进程后保护并备份完整状态；数据库升级必须有明确迁移契约；不能把“替换二进制”当作所有版本都可回退的保证。

执行任务的开发者/agent应在每次实际构建分发包或线上发布后，按项目 record-task-update 规则生成一份 release 记录，包含 channel/version/build、产物、发布范围和真实验证；部分构建也记录 partial。发行脚本不替使用者自动生成机器专属更新记录，也不修改 Git。

本次任务的构建验证与产物位置以对应 docs/updates release 记录为准。已在同一脚本基础上添加 GitHub Actions 配置；托管执行结果见任务记录；许可、目标机验收和公开发布授权仍需落实。
