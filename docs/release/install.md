# 一键安装 Anchor

- 日期：2026-10-11；入口为 `scripts/install.sh` / `scripts/install.ps1`，不是新增的 `anchor install` 子命令。
- **`0.1.0-alpha.6` 已公开，提供 Linux AMD64 / Windows AMD64 单文件**，包含中文供应商 ID 支持；[发行页、对应源码与 SHA256](https://github.com/candyflosssss/Anchor/releases/tag/v0.1.0-alpha.6)。无需 GitHub 登录。ARM、macOS 等目标明确拒绝；不自动装其他架构程序。
- 公开附件已匿名下载并复核；Windows EXE/ZIP 已在 Windows Server 2025 原生验收，PowerShell 5.1/7 安装器各 9 个离线测试通过。Linux 原始远程命令已完成隔离下载/安装/烟测。未声称在所有 Windows 版本上运行过真实远程安装。
- 旧 `alpha.3` 只有 Linux；指定它安装 Windows 仍会因缺少附件而失败。Actions artifact 和 Draft Release 不作为匿名安装源。

## Linux

需要 POSIX `sh`、`curl`、`sha256sum` 和常见系统命令（awk/sed/grep/mktemp 等，以及支持 `-T` 的 ln/mv）；不需要 Go、Python、jq 或 sudo。

```bash
# 下载、校验并安装当前公开版本：
curl -fsSL https://raw.githubusercontent.com/candyflosssss/Anchor/main/scripts/install.sh | sh
```

默认安装到 `~/.local/bin/anchor`。脚本不能改变父终端的环境；若目录不在 PATH，会打印可复制的设置命令，例如：

```bash
export PATH="$HOME/.local/bin:$PATH"
anchor version
```

安装器不会自动编辑 `.profile` / `.bashrc`，需要跨终端保留 PATH 时由用户自行设置。

```bash
# 固定版本（示例版本必须先实际公开发布；也接受 v 前缀）
curl -fsSL https://raw.githubusercontent.com/candyflosssss/Anchor/main/scripts/install.sh | sh -s -- --version 0.1.0-alpha.6

# 已在源码目录时，可直接使用本地脚本；自定义目录必须是绝对路径
sh scripts/install.sh --version 0.1.0-alpha.6 --install-dir "$HOME/tools/bin"
sh scripts/install.sh --help
```

参数契约：`--version VERSION` 默认省略；`--install-dir DIR` 默认 `$HOME/.local/bin`，支持空格但拒绝换行/冒号；`--replace` 默认关闭；`-h/--help` 只显示帮助。未知参数、缺少值、非法版本、非支持平台均非零退出。

## Windows PowerShell

支持 Windows PowerShell 5.1 / PowerShell 7，AMD64 系统（可识别 x64 系统上的 32 位 PowerShell）；不需要管理员权限；SHA-256 使用 .NET 流式计算，不依赖 `Get-FileHash` 模块发现。

```powershell
# 下载、校验并安装；脚本在独立作用域执行：
& ([scriptblock]::Create((Invoke-RestMethod 'https://raw.githubusercontent.com/candyflosssss/Anchor/main/scripts/install.ps1')))

# 固定版本：
& ([scriptblock]::Create((Invoke-RestMethod 'https://raw.githubusercontent.com/candyflosssss/Anchor/main/scripts/install.ps1'))) -Version '0.1.0-alpha.6'
```

默认安装到 `%LOCALAPPDATA%\Anchor\bin\anchor.exe`。不自动改用户/系统 PATH；不在当前 PATH 时打印设置提示：

```powershell
$env:Path = "$env:LOCALAPPDATA\Anchor\bin;" + $env:Path
anchor version
```

本地脚本语法：`& .\scripts\install.ps1 [-Version VERSION] [-InstallDir DIR] [-Replace] [-Help]`。`-InstallDir` 必须是本地盘符开头的绝对路径，不支持 UNC、相对路径、换行和分号。未知参数或失败以 PowerShell 错误终止；帮助不下载文件。不要为安装关闭系统安全检查或全局修改执行策略；企业策略不允许脚本时使用 [手动单文件安装](package-readme.md)。

## 下载与验证规则

1. 未指定版本时，查询 GitHub `releases?per_page=1`，选取 API 返回的首个已公开 Release，**包括 alpha/beta/rc 预发行版本**。顺序按 GitHub Release 创建时间，不是语义版本大小/发布时间；不是 GitHub 仅稳定版的 `releases/latest`。本项目当前发行流程只产出预发行版。
2. 指定版本时跳过版本发现，下载 `v<版本>` 下的 `anchor_<版本>_<平台>_amd64[.exe]` 及 `SHA256SUMS`。接受 `X.Y.Z` 或 `X.Y.Z-alpha/beta/rc.N`，不接受任意标签或下载地址。
3. 原始下载与重定向均要求 HTTPS。缺少附件、请求失败/限流、校验项缺失/重复/格式异常或哈希不一致时失败，不回退到未校验安装、其他版本或 Actions artifact。
4. 校验通过后才安装。Linux 添加 0755 执行权限；两平台均在目标目录内暂存，使用同文件系统原子替换，并清理本轮暂存数据。并发安装有独占锁；不删除别人的锁。异常强杀遗留的锁/目录只在确认没有安装进程后按具体路径人工处理。
5. 目标已存在时默认拒绝；符号链接、Windows reparse point、目录目标即便允许替换仍拒绝。脚本不执行下载的程序、不初始化数据库、不修改 Codex 配置、不启动/停止后台服务。

SHA256SUMS 来自同一 HTTPS 发布源，提供完整性检查，**不是代码签名，也不能防护发布账号被攻破**。远程一行命令会执行该仓库提供的代码，只在信任来源时使用；可先下载/检查脚本，再本地执行。希望固定安装器本身时，把 raw URL 的 `main` 换成已公开并核对的版本标签或源码提交。

## 升级与卸载

- **先停止 Anchor、备份完整状态，再显式替换**：Linux 加 `--replace`，Windows 加 `-Replace`。安装器不自动停服/备份，也不声称二进制替换能完成数据库迁移或安全回退。Windows 上运行/锁定的程序可能导致替换失败，此时不强制结束进程。
- 稳定安装路径应在批准 Codex Binding **之前**确定；升级保留该路径，不随意移动认证 helper。
- 更换版本可指定 `--version` / `-Version`，但状态 schema 兼容性须按 [升级/回退说明](package-readme.md) 单独核对。卸载仅在停服后删除可执行文件，状态/凭据不自动删除。
- 成功输出安装路径和后续提示；退出成功只说明下载/校验/落盘完成，**不代表真实供应商、Codex、服务启动或 Windows 核心运行验收通过**。首次使用的初始化/供应商配置见 [CLI 指令大全](../cli/commands.md)。

## 实际验证边界

alpha.6 的构建、原生检查、公开下载与 Linux 真实远程安装结果见本轮 release 记录。Windows 两种 PowerShell 各执行 9 个离线安装器回归；Windows 原生核心对裸 EXE 和 ZIP 解出的程序分别验收初始化、状态 ACL、实例锁、中文供应商、模拟网关、正常退出与状态重开。公开下载后的文件 SHA256 与这批通过验收的文件一致。没有修改真实 Agent 配置、没有停止现有用户服务、没有请求真实付费供应商。

## 许可与源码

Anchor 使用 AGPL-3.0-only，允许商用；修改/再分发及网络服务的源码义务见 [许可说明](../license.md)。发行页提供对应源码包、LICENSE 和依赖声明。安装器不下载源码或声明到运行目录，不等于免除再分发义务。
