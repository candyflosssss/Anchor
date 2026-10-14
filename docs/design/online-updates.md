# 在线更新 v1

- 日期：2026-10-14；**`0.1.0-dev.desktop.32` 已于北京时间 2026-10-14 23:12:40 发布到默认 GitHub Releases 的 dev 渠道**，两平台安装包与清单同版投放。已有 desktop.9～.31 可在应用内选择 dev 升级。本版修复 SSH 非交互环境发现用户目录 Anchor，并细分连接失败原因；发起连接的桌面端也须更新。继承 .31 的 UI 密码／密钥设备连接，远端须预装新版 CLI 并启用 SSH，自动安装尚未实现。本轮无 schema 变更；默认源 Linux 实际下载及六附件 digest 核对通过，本机 CLI／桌面二进制已更新、核心 ready。未执行浏览器、原生窗口、安装器或 apply 验收。详见[桌面开发版](../release/desktop.md#当前在线-dev-版-32)。
- 基线：`e35a85a`；共享契约采用 `anchor-desktop-v1` 的增量扩展，由同一实现者维护 Go DTO、生成 TS、`desktop/contract/updates.json` 和前后端测试。
- 目标：缩短测试版本的投放／安装步骤，不做静默升级、数据库回滚、服务管理器接管或新的托管授权。

## 1. 使用流程

### 桌面

设置 → 在线更新 → **dev · 快速测试** → 检查更新 → 下载更新 → 安装更新 → 明确确认。

- 更新页不要求核心在线，检查／下载不会停服，不请求模型上游；正在下载或提交时锁定操作与导航。
- 配置的渠道／源发生变化时不恢复其他来源的旧下载候选；切换渠道也清空前端安装候选。手动检查将选择的渠道写入本机 `updates/config.json`；桌面首次未配置时按当前版本选择渠道（dev 包为 `dev`），无法识别版本时回退 `alpha`；CLI 未配置时仍为 `alpha`。支持 `dev`、`alpha`、`beta`、`rc`、`stable`，只查指定渠道，不跨渠道静默回退。
- 自定义测试源是宿主／CLI 能力，不向 JS 开放任意 URL 输入：`anchor update configure --channel dev --manifest <HTTPS_URL>`，或首次启动 `anchor-desktop --update-channel dev --update-manifest <HTTPS_URL>`。两个入口写入同一目录配置；使用隔离 `--data-dir` 时两者必须一致。配置不放密钥；HTTP 只允许字面量回环 IP，供本机隔离测试。
- 当前操作远程实例时，更新仍只针对本机桌面和本机核心，远端核心保持运行。安装确认会重新读取 receipt 和校验安装包，拿桌面启动锁，核验本机核心身份／授权，正常请求停服，并等待实例锁释放（最多20秒）。身份不符、不支持 graceful_stop、恢复／退出超时均不强杀；安装失败后不自动重启核心，用户可重新打开旧程序。
- 宿主复制一个专用 helper 到本机 updates 目录，以独立进程运行；窗口退出后 helper 才拿更新／实例锁、重新校验并交接安装器；用户确认的版本与完整 SHA256 随交接固定，receipt 被另一进程换成其他版本或同版本不同文件时拒绝安装。最多等待窗口退出90秒；另一核心抢先启动则取消安装。
- Windows：运行已有当前用户 NSIS 安装器，继续遵循其文件占用拒绝与运行时授权规则；不静默安装。其他窗口须先点击「退出界面」，安装完重新打开程序。
- Linux：`xdg-open` 将 `.deb` 交给系统关联的包安装器，提权由系统处理，Anchor 不调用 sudo 或写 `/usr/bin`。没有关联安装器时，用户在全部窗口点击「退出界面」后用 `sudo apt install <已校验的绝对路径>` 安装，再启动。交接成功不等于包管理器安装完成。
- helper 结果留在 `updates/install.log`；当前可执行文件已退出，因此失败不会伪报“已更新”。Linux helper 结束删除自身，Windows 执行中的 helper 可能无法自删；可在确认退出后删除下载缓存及 `anchor-update-helper-*`，不得删除主状态目录。
- desktop.14 起普通关闭隐藏到系统托盘，无可用托盘时直接退出宿主；托盘菜单支持打开与退出界面。「退出界面」「停止核心」仍位于顶栏「本机实例」弹层。退出界面不停止核心；确认“安装并停服”才停止核心，安装回调绕过关闭隐藏并真正退出，以便 helper 继续。

### CLI

完整语法、输出、作用域见[最新指令大全](../cli/commands.md)。`update config/configure/check/download/apply` 是本机操作，拒绝 `--context`，不需要初始化数据库。

CLI 的 apply 只更新正在调用的 CLI 程序，不替换桌面程序、不停远程实例。用户先显式停本机核心，下载完成后 `update apply --approve`；先拿更新锁与本机实例锁，重新校验包，在目标目录复制新文件，旧程序改名为 `<executable>.previous` 后替换。普通替换失败会尝试恢复旧文件；若恢复也失败，错误提供备份路径。保留一份上一版二进制，后续安装更新会替换该备份。目标目录与文件须属于当前用户，拒绝符号链接／Windows reparse 路径；Linux 还要求不对组／其他用户开放写权限且不是硬链接。Windows 更新缓存使用受保护的当前用户 ACL；并不声称对任意安装目录完成了 ACL／硬链接全面审计。不可写的系统安装路径拒绝，不自动提权。需要用户自行重启核心，不自动执行数据库迁移或承诺运行失败自动回滚。

`update check/download --product desktop --current-version <实际桌面版本>` 可用 CLI 查询／下载桌面安装包，之后手动安装；不会把安装器覆盖到 CLI 路径。所有更新仅接受更高版本，不支持在线降级，同版本重复下载可复用完整且仍通过校验的缓存。

## 2. 来源与 Manifest

默认来源是 `candyflosssss/Anchor` 的 GitHub Releases API，按 SemVer 选择指定渠道的最高已发布版本，跳过 draft；不使用排除 prerelease 的 `/latest`。API 最多读5页，每页100项，无 GitHub token（可能受匿名限流）。无可兼容发行时返回 available=false，而不是假装找到了桌面版本。默认源要求安装包 URL 与同一 Release 的附件 URL 一致。

- 新 feed 附件：`update-cli.json`／`update-desktop.json`。
- 兼容已有 CLI `release.json` 的 standalone_artifacts，哈希与大小来自原发行清单、URL 来自相同 Release；alpha.6 只有 CLI 附件；desktop.12 已附独立 desktop feed，不混用产品。
- 自定义 feed 与包目录可由可信 HTTPS 静态站点托管。需要先上传不可变版本包并验证，再原子切换渠道清单；清单生成器只写文件，不部署或上传；`.9`／`.11` 当时只本地打包，`.12` 才完成默认 GitHub 源投放。

```json
{
  "schema_version": 1,
  "product": "desktop",
  "version": "0.1.0-dev.desktop.16",
  "channel": "dev",
  "artifacts": [{
    "file": "anchor-desktop_0.1.0-dev.desktop.16_amd64.deb",
    "target": "linux-amd64",
    "kind": "deb",
    "url": "https://example.invalid/anchor/0.1.0-dev.desktop.16/linux-amd64/anchor-desktop_0.1.0-dev.desktop.16_amd64.deb",
    "sha256": "<64个小写十六进制字符>",
    "bytes": 123456
  }]
}
```

product 固定 cli/desktop；CLI kind=binary，桌面 Windows AMD64 kind=nsis/.exe、Linux AMD64 kind=deb/.deb。每平台只能出现一个附件；版本与渠道相符、不能重复目标、不能目录穿越。版本比较按数值处理 `.9`→`.10`，支持开发版 `X.Y.Z-dev.desktop.N`，当前不接受 `+build` 元数据。

desktop.20 已交付更新代理修复：按每个请求及重定向实时读取对应协议的 HTTP_PROXY / HTTPS_PROXY（含小写），再读取 ALL_PROXY / all_proxy；NO_PROXY 对环境和系统代理均有效，回环始终直连。Windows 在未指定对应环境代理时通过 WinHTTP 读取当前用户配置，支持手动代理和 PAC（按 URL 解析），PAC 失败可回退手动代理；仅自动发现失败时按系统直连，单次解析等待最多8秒，后台最多保留一个原生解析，不自动发送 Windows 凭据。HTTP-only 环境变量不再抑制 HTTPS 系统代理。Linux 继续使用环境代理。

默认源优先 GitHub Releases API，API 阶段最多10秒；连接失败、超时或HTTP限流等错误时尝试同一仓库的官方 releases.atom，从同渠道最近条目按SemVer取最多5个候选，读取正式同版清单。Atom不是完整历史，未找到兼容清单时报告失败，不误报“没有更新”。不在无候选、无效/不一致元数据、校验失败或主动取消时回退；自定义源不改走GitHub。回退清单版本、平台、字节/SHA256和附件URL仍校验，附件必须属于同一仓库同一版本Release，不使用第三方镜像。

TLS 校验证书，最多5次重定向，不允许 HTTPS 降级到 HTTP；允许 HTTPS CDN 的签名查询参数，但用户配置 URL 不接受 userinfo/query/fragment。元数据最多2 MiB、安装包最多256 MiB；检查30秒（桌面）／整个命令5分钟（CLI）、下载5分钟。采用磁盘流式下载、临时文件、实际字节数与 SHA256 校验；未通过校验不保存 receipt，失败不覆盖完整旧包。文件路径由宿主生成，不使用服务器文件名定位本机目录，不自动解压或运行下载内容。

SHA256 是完整性检查，不是独立签名认证。Manifest 和包来自同一个信任来源；测试安装器目前未签名，没有 Ed25519 或 Authenticode 发布门禁，务必只配置可信源。元数据与 receipt 不含凭据或模型密钥。

## 3. 快速测试投放

执行本节的桌面迭代交付时使用项目 [anchor-desktop-release skill](../../.agents/skills/anchor-desktop-release/SKILL.md)，沿用已授权的投放范围，只做必要检查。

已有 `.9`～`.15` 选择 dev 即可发现并下载 `.16`，无需填写源地址；desktop.8 首次须人工安装包含更新入口的新版本。`.16` 已于北京时间 2026-10-12 01:43:37 发布为 GitHub prerelease，非 latest；新增真实请求／系统日志和可选显示内容，保留单 GUI／托盘与后台核心生命周期。Linux 已通过默认源真实下载和大小／SHA256校验，Windows GitHub digest 与本地一致。手动安装前须停止旧核心并退出全部窗口；应用内确认安装执行正常停服与宿主退出。首次启动将 state.db schema3→4（更旧状态先2→3），旧二进制不能打开迁移后的数据库，不提供数据库降级；日志独立 logs.db schema1，不补采升级前请求。之后递增 `dev.desktop.N`，保留旧版包。

```bash
# 自建 HTTPS 测试站点示例使用下一版 .17；此命令只打包，不上传。
./scripts/package-desktop.sh --version 0.1.0-dev.desktop.17 \
  --update-base-url https://example.invalid/anchor/0.1.0-dev.desktop.17
# 输出根目录额外生成 update-desktop.json，URL 对应 target/文件名。

# 默认 GitHub Release 源：已有最终产物后生成 flat 清单，输出必须是新文件。
python3 scripts/update_manifest.py --product desktop \
  --version 0.1.0-dev.desktop.16 --channel dev \
  --base-url https://github.com/candyflosssss/Anchor/releases/download/v0.1.0-dev.desktop.16 --layout flat \
  --artifact linux-amd64=/absolute/path/linux-amd64/package.deb \
  --artifact windows-amd64=/absolute/path/windows-amd64/setup.exe \
  --output /absolute/path/update-desktop.json
```

脚本流式计算最终文件 SHA256／大小，拒绝重复目标、空文件、链接、版本／渠道不符与非 TLS 源；不执行安装，不触发 GitHub 工作流。GitHub 上使用准确的附件名；通过默认 Release 源时清单 URL 应与该 Release 的实际附件一致（通常使用生成器的 flat 布局）。自建测试站点可采用 target 布局。

默认 dev 投放必须完成以下步骤，不能停在本地安装包：

1. 从干净提交生成新版本两平台安装包，保留源码 revision 与最终哈希；按实际附件名生成 flat 清单。
2. 给相同版本的 GitHub draft prerelease 上传两安装包、`update-desktop.json`、对应源码（含 Go vendor）、许可／构建元数据与校验清单；tag 指向实际构建提交。
3. 确认附件上传完成后公开该 prerelease，保持非 latest。`dev.desktop.N` 不触发现有 alpha/beta/rc 标签工作流。
4. 使用隔离临时目录调用现有 `update download --channel dev --product desktop --current-version <上一版>`，确认默认源发现新版并完成真实下载校验；不安装、不改用户配置。记录发布与验证结果。

`.16` 已完成这四步；Linux 真实下载与 SHA256／大小检查通过，Windows 附件的 GitHub digest 与本地一致。安装器交接和原生窗口行为未实机验收。

## 4. 共享边界与验证范围

后端 `internal/updater` 供 CLI 与桌面共同调用；不经业务管理 API、不重写已有 core/state 规则。桌面增加四个受限方法：UpdateStatus、CheckUpdate(channel)、DownloadUpdate(version)、InstallUpdate(version/approved)，源和磁盘目标留在宿主。Go DTO 是规范源，前端依次经过 updates Model → 桌面适配器，hook Controller 管状态，View 不执行系统操作。

UpdateInfo phase 为 idle/checked/downloaded/installing；空 latest_version 表示尚未发现兼容版本。问题码新增 integrity/permission，其他复用 invalid/conflict/read_error/unknown/identity/unauthorized/unsupported。installing 只是 installer handoff，不是已安装。前端验证白名单、无绑定失败关闭、不回退 Mock；初始化请求及卸载后的旧结果不得覆盖新查询。

验证分层：共享 DTO/TS 漂移、Go 模拟更新服务器的检查／实际下载／校验／CLI 替换、真实 Go Bridge 读写下载、浏览器模拟绑定，以及 Windows／Linux 原生安装验收。前三类与模拟浏览器测试不替代最后一类；未发布或未运行原生安装时不能声称安装即用或线上升级已完成。
