# 在线更新 v1

- 日期：2026-10-11；在线更新已实现，**在线更新首次随 `0.1.0-dev.desktop.9` 打包；当前本地功能包为 `0.1.0-dev.desktop.11`，尚未公开发行或部署更新源**。旧 CLI alpha.6 和 desktop.8 不含更新入口；新包未做原生安装验收，构建结果见[桌面开发版](../release/desktop.md)。
- 基线：`e35a85a`；共享契约采用 `anchor-desktop-v1` 的增量扩展，由同一实现者维护 Go DTO、生成 TS、`desktop/contract/updates.json` 和前后端测试。
- 目标：缩短测试版本的投放／安装步骤，不做静默升级、数据库回滚、服务管理器接管或新的托管授权。

## 1. 使用流程

### 桌面

设置 → 在线更新 → 选择渠道 → 检查更新 → 下载更新 → 安装更新 → 明确确认。

- 更新页不要求核心在线，检查／下载不会停服，不请求模型上游；正在下载或提交时锁定操作与导航。
- 配置的渠道／源发生变化时不恢复其他来源的旧下载候选；切换渠道也清空前端安装候选。手动检查将选择的渠道写入本机 `updates/config.json`；未配置时为 `alpha`。支持 `dev`、`alpha`、`beta`、`rc`、`stable`，只查指定渠道，不跨渠道静默回退。
- 自定义测试源是宿主／CLI 能力，不向 JS 开放任意 URL 输入：`anchor update configure --channel dev --manifest <HTTPS_URL>`，或首次启动 `anchor-desktop --update-channel dev --update-manifest <HTTPS_URL>`。两个入口写入同一目录配置；使用隔离 `--data-dir` 时两者必须一致。配置不放密钥；HTTP 只允许字面量回环 IP，供本机隔离测试。
- 安装确认会重新读取 receipt 和校验安装包，拿桌面启动锁，核验本机核心身份／授权，正常请求停服，并等待实例锁释放（最多20秒）。身份不符、不支持 graceful_stop、恢复／退出超时均不强杀；安装失败后不自动重启核心，用户可重新打开旧程序。
- 宿主复制一个专用 helper 到本机 updates 目录，以独立进程运行；窗口退出后 helper 才拿更新／实例锁、重新校验并交接安装器；用户确认的版本与完整 SHA256 随交接固定，receipt 被另一进程换成其他版本或同版本不同文件时拒绝安装。最多等待窗口退出90秒；另一核心抢先启动则取消安装。
- Windows：运行已有当前用户 NSIS 安装器，继续遵循其文件占用拒绝与运行时授权规则；不静默安装。其他窗口须先关闭，安装完重新打开程序。
- Linux：`xdg-open` 将 `.deb` 交给系统关联的包安装器，提权由系统处理，Anchor 不调用 sudo 或写 `/usr/bin`。没有关联安装器时，用户关闭全部窗口后用 `sudo apt install <已校验的绝对路径>` 安装，再启动。交接成功不等于包管理器安装完成。
- helper 结果留在 `updates/install.log`；当前可执行文件已退出，因此失败不会伪报“已更新”。Linux helper 结束删除自身，Windows 执行中的 helper 可能无法自删；可在确认退出后删除下载缓存及 `anchor-update-helper-*`，不得删除主状态目录。
- 关窗不停服的普通行为不变；只有这次明确的“安装并停服”批准才停服。

### CLI

完整语法、输出、作用域见[最新指令大全](../cli/commands.md)。`update config/configure/check/download/apply` 是本机操作，拒绝 `--context`，不需要初始化数据库。

CLI 的 apply 只更新正在调用的 CLI 程序，不替换桌面程序、不停远程实例。用户先显式停本机核心，下载完成后 `update apply --approve`；先拿更新锁与本机实例锁，重新校验包，在目标目录复制新文件，旧程序改名为 `<executable>.previous` 后替换。普通替换失败会尝试恢复旧文件；若恢复也失败，错误提供备份路径。保留一份上一版二进制，后续安装更新会替换该备份。目标目录与文件须属于当前用户，拒绝符号链接／Windows reparse 路径；Linux 还要求不对组／其他用户开放写权限且不是硬链接。Windows 更新缓存使用受保护的当前用户 ACL；并不声称对任意安装目录完成了 ACL／硬链接全面审计。不可写的系统安装路径拒绝，不自动提权。需要用户自行重启核心，不自动执行数据库迁移或承诺运行失败自动回滚。

`update check/download --product desktop --current-version <实际桌面版本>` 可用 CLI 查询／下载桌面安装包，之后手动安装；不会把安装器覆盖到 CLI 路径。所有更新仅接受更高版本，不支持在线降级，同版本重复下载可复用完整且仍通过校验的缓存。

## 2. 来源与 Manifest

默认来源是 `candyflosssss/Anchor` 的 GitHub Releases API，按 SemVer 选择指定渠道的最高已发布版本，跳过 draft；不使用排除 prerelease 的 `/latest`。API 最多读5页，每页100项，无 GitHub token（可能受匿名限流）。无可兼容发行时返回 available=false，而不是假装找到了桌面版本。默认源要求安装包 URL 与同一 Release 的附件 URL 一致。

- 新 feed 附件：`update-cli.json`／`update-desktop.json`。
- 兼容已有 CLI `release.json` 的 standalone_artifacts，哈希与大小来自原发行清单、URL 来自相同 Release；**现有发行没有 desktop feed，不能用 CLI 冒充桌面包**。
- 自定义 feed 与包目录可由可信 HTTPS 静态站点托管。需要先上传不可变版本包并验证，再原子切换渠道清单；清单生成器只写文件，不部署或上传；当前 `.11` 打包未提供真实更新源地址，因此未生成发布清单。

```json
{
  "schema_version": 1,
  "product": "desktop",
  "version": "0.1.0-dev.desktop.12",
  "channel": "dev",
  "artifacts": [{
    "file": "anchor-desktop_0.1.0-dev.desktop.12_amd64.deb",
    "target": "linux-amd64",
    "kind": "deb",
    "url": "https://example.invalid/anchor/0.1.0-dev.desktop.12/linux-amd64/anchor-desktop_0.1.0-dev.desktop.12_amd64.deb",
    "sha256": "<64个小写十六进制字符>",
    "bytes": 123456
  }]
}
```

product 固定 cli/desktop；CLI kind=binary，桌面 Windows AMD64 kind=nsis/.exe、Linux AMD64 kind=deb/.deb。每平台只能出现一个附件；版本与渠道相符、不能重复目标、不能目录穿越。版本比较按数值处理 `.9`→`.10`，支持开发版 `X.Y.Z-dev.desktop.N`，当前不接受 `+build` 元数据。

TLS 校验证书，最多5次重定向，不允许 HTTPS 降级到 HTTP；允许 HTTPS CDN 的签名查询参数，但用户配置 URL 不接受 userinfo/query/fragment。元数据最多2 MiB、安装包最多256 MiB；检查30秒（桌面）／整个命令5分钟（CLI）、下载5分钟。采用磁盘流式下载、临时文件、实际字节数与 SHA256 校验；未通过校验不保存 receipt，失败不覆盖完整旧包。文件路径由宿主生成，不使用服务器文件名定位本机目录，不自动解压或运行下载内容。

SHA256 是完整性检查，不是独立签名认证。Manifest 和包来自同一个信任来源；测试安装器目前未签名，没有 Ed25519 或 Authenticode 发布门禁，务必只配置可信源。元数据与 receipt 不含凭据或模型密钥。

## 3. 快速测试投放

首次需要人工安装包含更新入口的版本（从 `.9` 开始，当前为 `.11`）；已有 desktop.8 无法自动获得本功能。`.11` 仅在本地打包，未上传安装包或更新清单；有更新入口不等于当前渠道已存在可安装版本。安装 `.11` 前须停止旧核心并关闭窗口，schema 2 会迁移到3，旧程序不能打开升级后的数据库。之后每次递增 `dev.desktop.N`，保留旧版包，上传新版与 feed 即可在线安装，无需每次人工寻找下载链接。

```bash
# 示例使用尚不存在的 .12；只打包，不上传。base URL 应指向未来实际发布的版本目录。
./scripts/package-desktop.sh --version 0.1.0-dev.desktop.12 \
  --update-base-url https://example.invalid/anchor/0.1.0-dev.desktop.12
# 输出根目录额外生成 update-desktop.json，URL 对应 target/文件名。

# 已有最终产物也可单独生成清单，输出必须是新文件。
python3 scripts/update_manifest.py --product desktop \
  --version 0.1.0-dev.desktop.12 --channel dev \
  --base-url https://example.invalid/anchor/0.1.0-dev.desktop.12 --layout target \
  --artifact linux-amd64=/absolute/path/linux-amd64/package.deb \
  --artifact windows-amd64=/absolute/path/windows-amd64/setup.exe \
  --output /absolute/path/update-desktop.json
```

脚本流式计算最终文件 SHA256／大小，拒绝重复目标、空文件、链接、版本／渠道不符与非 TLS 源；不执行安装，不触发 GitHub 工作流。GitHub 上使用准确的附件名；通过默认 Release 源时清单 URL 应与该 Release 的实际附件一致（通常使用生成器的 flat 布局）。自建测试站点可采用 target 布局。

## 4. 共享边界与验证范围

后端 `internal/updater` 供 CLI 与桌面共同调用；不经业务管理 API、不重写已有 core/state 规则。桌面增加四个受限方法：UpdateStatus、CheckUpdate(channel)、DownloadUpdate(version)、InstallUpdate(version/approved)，源和磁盘目标留在宿主。Go DTO 是规范源，前端依次经过 updates Model → 桌面适配器，hook Controller 管状态，View 不执行系统操作。

UpdateInfo phase 为 idle/checked/downloaded/installing；空 latest_version 表示尚未发现兼容版本。问题码新增 integrity/permission，其他复用 invalid/conflict/read_error/unknown/identity/unauthorized/unsupported。installing 只是 installer handoff，不是已安装。前端验证白名单、无绑定失败关闭、不回退 Mock；初始化请求及卸载后的旧结果不得覆盖新查询。

验证分层：共享 DTO/TS 漂移、Go 模拟更新服务器的检查／实际下载／校验／CLI 替换、真实 Go Bridge 读写下载、浏览器模拟绑定，以及 Windows／Linux 原生安装验收。前三类与模拟浏览器测试不替代最后一类；未发布或未运行原生安装时不能声称安装即用或线上升级已完成。
