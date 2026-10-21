# 在线更新 v1

- 更新：2026-10-21。
- 当前线上桌面 dev 版：[0.1.0-dev.desktop.61](https://github.com/candyflosssss/Anchor/releases/tag/v0.1.0-dev.desktop.61)，发布于 `2026-10-21T12:02:05+08:00`；GitHub prerelease，非 latest，11 项附件同版投放，构建提交 `c6f56a407bce12608c0466abb7c67a1d7249d214`、SOURCE dirty=false。
- 修复历史 reasoning 中 null／非字符串／空字符串 encrypted_content 导致上游 400：协议层在发送前移除无效字段及对应 id，保留摘要、非空密文和其他历史；无额外请求，既有明确密文拒绝后的恢复继续保留。
- 同时包含 .60 后已提交的请求级协议翻译、Anthropic 顺序内容块增量输出、日志已知用量小计、连接核心版本展示及显式系统代理路由；具体契约见设计与 CLI 文档。
- protocol／core／cli 回归、双平台生产编译／打包、11 附件远端摘要和 3 条默认源实际下载均通过。入口：设置 → 在线更新 → dev · 快速测试 → 检查更新 → 下载更新 → 安装更新。
- 本轮仅在线投放，未替换本机运行的 .60，SSH 被控机另行更新；未调用真实付费推理，未验收双平台原生窗口、系统安装器或真实 SSH 升级。

## .61 在线投放（2026-10-21）

同版11附件完成上传与digest核对，Linux runtime、旧客户端deb和CLI三条默认源实际下载与大小／SHA256／版本检查通过。构建tag指向 `c6f56a407bce12608c0466abb7c67a1d7249d214`，dev prerelease、非latest。本机仍运行.60，本轮未安装或改动用户更新配置。完整改动与校验表见[发行记录](../release/desktop.md#native-reasoning-61)。

## .60 在线投放与本机升级（2026-10-21）

修复codex.response.metadata提前结束密文检查，前缀上限提高至2 MiB，新增安全类型哈希及字段存在性日志；同渠道仅一次清理重试，真实输出／工具／其他未知事件不重放。.59仍失败，用户已确认修正探针.3使原失败会话恢复。默认源真实下载Linux runtime、旧客户端deb及CLI，11附件摘要与构建tag通过，prerelease／非latest。详见[发行记录](../release/desktop.md#codex元信息与密文恢复-60已发布-dev本机已升级)。

本机于`2026-10-21T00:12:28.906214+08:00`将CLI/core .59.probe.3及GUI .59配套换为正式.60，备份`/home/mosaki/.config/anchor/backups/pre-desktop60-20261021-001215`；保留交接、运行摘要、ready/applied、schema9、配置／凭据／托管／恢复文件及dev更新配置核对通过。固定身份／端口更新检查available=false；未主动请求付费推理，未验收系统安装器／原生窗口／跨机SSH更新。

## .59 在线投放与本机升级（2026-10-20）

空思考容器不再提前结束密文检查，新增安全事件类别诊断；明确拒绝后仍仅同渠道一次清理重试，实际内容／工具／未知结构保持不重放。默认源真实下载Linux runtime、旧客户端deb和CLI，11附件摘要与tag核对通过，prerelease／非latest。完整边界及摘要见[发行记录](../release/desktop.md#空思考容器恢复-59已发布-dev本机已升级)。

本机备份 `/home/mosaki/.config/anchor/backups/pre-desktop59-20261020-234243`，保留交接`.58`→`.59`，运行摘要、ready/applied、schema9及配置／凭据／托管／恢复文件核验通过，dev更新配置保持，固定身份／端口更新检查available=false。用户随后确认.59仍失败，针对性修正探针.3后恢复，正式修复见上方.60；未主动请求付费推理，未执行系统安装器／原生窗口／跨机SSH更新验收。

## .58 在线投放与本机升级（2026-10-20）

默认 dev 源真实下载新版 Linux runtime、旧客户端兼容 deb 与同版 CLI，版本／目标／大小／SHA256核对通过，11附件远端摘要全部匹配，构建 tag 指向 `4ec5ea31637751a488f10211e7e9c59c13a7cad9`，prerelease／非 latest。安装器与runtime配套清单保留新旧客户端兼容。加密上下文识别等待原请求超时，日志标记检查结束原因和实际清理／恢复；详见[发行记录](../release/desktop.md#加密上下文延迟恢复-58已发布-dev本机已升级)。

本机经备份、保留交接完成`.57`→`.58`，核心／GUI运行摘要、ready／applied、schema9和配置／凭据／托管恢复文件核验通过，dev更新配置保持。备份 `/home/mosaki/.config/anchor/backups/pre-desktop58-20261020-222728`。身份／端口固定更新检查返回当前`.58`、available=false；本机只读模型目录核对通过。未执行付费推理、系统安装器／apply、原生窗口或跨机SSH传输验收。

## .57 在线投放验证（2026-10-20）

11 项附件包含两平台安装器、两平台 desktop runtime 与 CLI binary、两份 flat 更新清单、对应源码、许可／构建元数据和 SHA256SUMS。默认 dev 源实际下载 Linux runtime（42,996,784 字节）、旧客户端兼容 deb（29,110,236 字节）和 CLI（37,642,402 字节），大小与 SHA256 均匹配，下载程序版本为 `.57`；Windows 三个程序使用 GitHub digest 核对通过。完整摘要及验收边界见[发行记录](../release/desktop.md#newapi-授权与自动缓存-57已发布-dev)。本轮未更新真实运行实例。

### .57 本机安装（2026-10-20T21:52:31.182449+08:00）

已沿用用户目录安装方式完成 CLI／核心／GUI `.56`→`.57`，正常保留交接及配置核验通过，备份 `/home/mosaki/.config/anchor/backups/pre-desktop57-20261020-215227`。本机 dev 渠道已保存，保留更新源；通过与 SSH 相同的身份／端口固定参数执行 CLI `update check`，返回当前 `.57`、候选 `.57`、available=false。该主机即 `192.168.1.111`，已具备远端在线更新的 CLI 前置能力。没有重建或重新发布，没有验收跨机 SSH 更新传输；详见[本机安装记录](../release/desktop.md#newapi-授权与自动缓存-57已发布-dev)。

## .56 在线投放与本机升级（2026-10-20）

默认 dev 源真实下载 Linux deb：29055484 字节，SHA256 `c27ec18cd3d62d26e94f697378857b89789e28c8bde4736b58bad22515156991`；Windows digest `405070f824e42fd55c78a7a8b95414116f80c5ec8c76a73c2ac4f114a340db5c`。六附件大小／摘要、tag revision 和非 latest 核验通过。本机保留交接升级与状态验证通过，备份 `/home/mosaki/.config/anchor/backups/pre-desktop56-20261020-161041`。边界与附件见[发行记录](../release/desktop.md#codex-配置重建-56已发布-dev本机已升级)。

## .55 在线投放与本机升级（2026-10-20）

默认 GitHub dev 源以 `.54` 为当前版本真实发现并下载 `.55` Linux deb：29046240 字节，SHA256 `4032f537784ba076f405925344d88307cf51bcdd76dcc43154ecd50daef1a3f2`；Windows GitHub digest `c29e41b5abf67b7c0222c52777fdb453193f94f864d6fd16c7f689e376470618` 与本地一致。同版六附件和构建 tag 已核对，prerelease／非 latest。

本机采用用户目录二进制替换；备份 `/home/mosaki/.config/anchor/backups/pre-desktop55-20261020-145618`，正常 restart_handoff，核心／宿主运行哈希、schema9、数据库完整性、配置／凭据／托管文件和 applied 状态核对通过。未走系统安装器／应用内 apply。完整边界见[发行记录](../release/desktop.md#配置接管修复-55已发布-dev本机已升级)。

## .54 在线投放验证（2026-10-20）

Release tag 指向构建提交 `c81efd52e01ac2eaa801a267f2fe7880dd34ada5`，同版附件包括两平台安装器、`update-desktop.json`、对应源码、许可／构建元数据和 `SHA256SUMS`，六项均通过大小与 SHA256／GitHub digest 校验。`latest` 发布前后保持未改变。

以 `.53` 为当前版本，在隔离数据目录通过本轮源码 CLI 从默认 GitHub dev 源真实发现并下载 `.54` Linux `.deb`：29,045,388 字节，SHA256 `c29a698b5e4ff694034e27f3bcaef6d3e4eb782624cc5ebb00795912d203cec6`。Windows 远端 digest 与本地 SHA256 `790d301d05c17983192c1378b03ed437a07e1655fdcf9c32597fc50837aeacb9` 一致。未执行 `apply` 或安装器，未修改真实用户配置，未请求真实付费上游。

## 1. 使用流程

### 桌面

设置 → 在线更新 → **dev · 快速测试** → 检查更新 → 下载更新 → 安装更新 → 明确确认。

- 更新页不要求核心在线，检查／下载不会停服，不请求模型上游；正在下载或提交时锁定操作与导航。
- 配置的渠道／源发生变化时不恢复其他来源的旧下载候选；切换渠道也清空前端安装候选。手动检查将选择的渠道写入本机 `updates/config.json`；桌面首次未配置时按当前版本选择渠道（dev 包为 `dev`），无法识别版本时回退 `alpha`；CLI 未配置时仍为 `alpha`。支持 `dev`、`alpha`、`beta`、`rc`、`stable`，只查指定渠道，不跨渠道静默回退。
- 自定义测试源是宿主／CLI 能力，不向 JS 开放任意 URL 输入：`anchor update configure --channel dev --manifest <HTTPS_URL>`，或首次启动 `anchor-desktop --update-channel dev --update-manifest <HTTPS_URL>`。两个入口写入同一目录配置；使用隔离 `--data-dir` 时两者必须一致。配置不放密钥；HTTP 只允许字面量回环 IP，供本机隔离测试。
- 当前操作远程实例时，更新仍只针对本机桌面和本机核心，远端核心保持运行。安装确认会重新读取 receipt 和校验安装包，拿桌面启动锁，核验本机核心身份／授权，以 `StopForRestart` 请求保留交接，并等待实例锁释放（最多20秒）。要求 `restart_handoff`（不仅是 graceful_stop）及 `bindings_preserved:true` 确认；身份不符、能力缺失或退出超时均不强杀、不退回普通停服。交接保留当前 Agent 配置、模型目录、凭据与原始恢复日志，不暴露旧路由。安装失败后不自动重启核心，用户可重新打开兼容程序；放弃维护可离线 `recover` 条件恢复。此语义已随 desktop.35 发布，desktop.34 及更早版仍使用普通停服。
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

<a id="runtime-与-ssh-更新2026-10-20源码未发布"></a>

## Runtime 与 SSH 更新（2026-10-20，desktop.57 dev 已发布）

此节取代上方旧安装器流程中“必须先停服、安装后手动重开”的一般描述。旧宿主仍沿用原行为；desktop.57 已同版投放安装包、runtime 和 CLI feed，并通过隔离下载校验；未修改真实运行实例。

新桌面 feed 保留每平台原安装器，在 artifact 下可增加 `runtime`（同平台、kind=runtime、独立 URL／大小／SHA256）。旧宿主忽略新增字段，继续安装器流程；新版宿主优先选择 runtime。默认 GitHub 源同时核对安装器和 runtime 均为同一 Release 附件，自定义源遵循相同 HTTPS／完整性边界，不增加独立发布签名保证。`scripts/update_manifest.py --runtime TARGET=FILE` 生成该格式；Linux／Windows runtime 需要沿用支持的系统依赖，依赖缺失时预检失败并保留旧服务。

新版在当前用户 `updates/runtime/` 旁路目录中准备，不覆盖运行文件，不提权。下载、SHA256、文件权限和实际运行版本探测都在停服前完成；helper 也在停服前准备。最终用既有 `restart_handoff` 保留 Agent 配置、凭据和恢复基线，正常排空最多10秒，交接最多30秒，不强杀；确认相同实例、新版版本和 ready 后才写 active 启动记录。原系统安装路径仍为启动入口，新启动器转到已校验的更高版本。GUI 请求完成并退出后，helper 自动打开新版，新窗口复用已经恢复的核心，避免二次中断。旧请求／长流可能中断，无毫秒级停顿承诺；新版失败不自动数据库降级或重放变更。

`anchor update desktop --approve` 或 `anchor-desktop update desktop --approve` 提供 Linux 命令更新（同实现也支持 Windows）。已初始化实例必需；只升级核心／下次启动版本，运行中同数据目录桌面则通过已授权的当前用户 loopback IPC 请求重开。IPC 固定路径、Bearer 凭据、无 Origin、数据目录核验，不接受执行路径或任意命令。更新配置／版本缓存留在用户目录，系统包／注册表卸载版本仍为首次安装版本；卸载按既有规则保留数据。旧版启动入口尚无转发能力，需先用新安装器引导一次；旧源没有 runtime 时仍走安装器。

CLI 自身 `update upgrade --approve` 同样准备新程序、一次交接、核验并激活 CLI 启动入口，不改原有 `apply` 的手动停服／原位替换语义。下载／准备失败保留原服务。运行中核心版本高于目标时拒绝降级。交接后失败可能已经开始迁移，保持状态并报告，用户先检查 status，不盲目恢复旧二进制。

SSH 更新只作用于显式选择的既有 POSIX/Linux 被控机：桌面 `CheckRemoteUpdate(instance_id)` 返回 UpdateResult；`InstallRemoteUpdate({instance_id,version,approved})` 冻结上一轮检查的版本和完整 SHA256，只提交一次，清除候选，成功 phase=applied、available=false、current_version=实际新版。其余 UpdateInfo 结构不变。本机 `CheckUpdate/DownloadUpdate/InstallUpdate` 仍始终作用本机。所有前端接口均无 URL／路径／shell 参数。

2026-10-20 当前源码修正 SSH 更新失败提示（尚未打包）：`remote_anchor_unsupported` 明确要求先升级被控机 CLI 到 desktop.57 或更高版本；SSH 认证、主机密钥、设备身份等错误各自呈现，不当作更新源网络故障。检查结果未知与安装结果未知分别处理。更新发现与下载由被控机执行，使用远端更新配置及 SSH 非交互环境；本机设置的代理不会自动传递到远端。通用失败提示无法独立证明远端 GitHub 访问失败，须先核对实际错误类型。

连接层复用已认证 SSH 密码或密钥连接，调用被控机自己的 `update check` 和 `update upgrade --approve`，两者携带 instance/control-port，后者额外固定 version/SHA256；更新来源和平台由被控机决定。接收有界、严格 JSON，成功后经管理通道复核 instance/version/PID/ready。不重授权、不调用普通 stop、不更新主控机、不重复未知变更。旧远端 CLI 不支持新参数时显示 remote_anchor_unsupported，需一次手工引导；当前 shell 发现是 POSIX，Windows SSH 被控机不在范围。SSH 断开、错误响应或最终身份／版本不符均报告 unknown，不能宣称未执行或已回滚，用户重新检查。远端系统安装目录不可写不影响旁路更新，但账号必须有用户数据目录写入权限。

打包脚本为每平台额外生成唯一命名的 desktop runtime 与 CLI binary，并可生成 update-desktop/update-cli 两份清单。desktop.57 已把四个运行程序和两份清单与安装包／源码／许可元数据一并上传同版 Release，共 11 项附件；后续发布沿用该集合，不再假设旧的固定六附件数量。真实 SSH 升级与双平台原生窗口仍未在本轮验收。
