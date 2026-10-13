# 桌面开发版构建与安装

- 更新：2026-10-13。
- 当前桌面 dev 版：`0.1.0-dev.desktop.21`，北京时间 2026-10-13 10:15:34 已发布到 GitHub Releases。Windows / Linux 安装包与 `update-desktop.json` 同版投放。
- 范围：Agent 托管页面与本机发现／登记／同步；sub2api / NewAPI / 原版 OneAPI 账户与计费兼容核心接口、类型化客户端及源码 CLI。余额／账单／倍率的桌面查询页面尚未接入；原版 OneAPI 倍率不支持，NewAPI 自动分组倍率未知。更新器沿用 `.20` 的代理和官方订阅回退。
- 已有 `.9`～`.20`：**设置 → 在线更新 → dev · 快速测试 → 检查更新 → 下载更新 → 安装更新**。旧版代理接入失败时须手动安装一次 `.21`，先停止核心并退出界面；旧 `.8` 无更新入口。见[在线更新](../design/online-updates.md)。
- 首次启动核心自动迁移 state.db schema4→5，保留既有供应商及 Key；更旧数据库按顺序升级。旧二进制不能打开升级后的数据库，不支持降级。
- 功能阶段 Go 回归及定向测试通过；本轮 Go/TS 契约检查、前端类型与生产编译、两平台打包、六附件大小／digest 校验及默认源真实 Linux 更新下载通过。**Windows PAC、双平台原生窗口、安装和 apply 未验收**；dev 预发行未签名。

## 1. 安装与使用行为

### Windows AMD64

运行 `Anchor-Desktop-<版本>-windows-amd64-setup.exe`：

- 无默认管理员提权，固定安装在当前用户 `%LOCALAPPDATA%\Programs\Anchor Desktop`。
- 注册“已安装的应用”卸载项和开始菜单 Anchor 入口；桌面快捷方式为可选项。
- 检测 Microsoft WebView2 Runtime。缺失时先询问是否运行 Microsoft 联网引导器；拒绝、安装失败或安装后仍未检测到运行时均终止安装。静默模式不代替用户同意安装运行时。引导器来自已锁定的 Wails 模块，外层开发安装器未签名。
- 安装完成页的“打开 Anchor”默认不勾选。安装本身不启动核心、不改 PATH、不注册服务或登录启动项。
- 从开始菜单／快捷方式打开的是 **GUI**，不是 CLI 窗口；供应商界面使用嵌入的生产资源，不依赖前端开发服务器。

目标为 Windows 10 或更新版本 AMD64；这是构建目标，不是本轮实测兼容性清单。旧 Windows、ARM64 原生支持、签名和静默自动升级不在此前打包范围；新源码的手动在线更新需新包启动版本。

### Linux AMD64

首个安装包面向 **Ubuntu 24.04 LTS AMD64 图形桌面**，不是所有 Linux 发行版通用包。

通过系统包安装工具打开 `.deb`，或使用包管理器解析运行库依赖：

```bash
sudo apt install ./anchor-desktop_0.1.0-dev.desktop.21_amd64.deb
```

文件安装到 `/usr/bin/anchor-desktop`，应用菜单入口位于 `/usr/share/applications/anchor-desktop.desktop`；包含 SVG / PNG 图标。包的 GTK3 / WebKitGTK4.1 / libc 等依赖从实际 ELF 用 `dpkg-shlibdeps` 提取。没有安装／卸载脚本自动启服务，没有 systemd unit 或自启动项。安装后从应用菜单打开 GUI，无需手动 `init` / `serve`。没有图形会话的服务器继续使用 CLI。

### 两平台共同行为与数据保留

打开窗口后，Go 宿主安全初始化缺失状态、启动独立后台核心或核验复用已运行实例，连接就绪后进入供应商页。供应商列表、新增／编辑、删除／复制／改名、模型映射、多 Key、显式测试和用量工作台均复用核心管理能力。从 `.14` 起，点击关闭或系统关闭会隐藏到系统托盘，点击托盘图标可恢复；无可用托盘时退出宿主。从 `.15` 起当前用户只保留一个 GUI／托盘，重复启动恢复已有窗口；「退出界面」真正退出宿主，以上操作均保留后台核心；停止核心必须在界面单独确认（或用兼容 CLI `stop --approve`）。从 `.13` 起，「退出界面」「停止核心」位于顶栏「本机实例」弹层。停服请求被接受不等于恢复完成。

**手动安装或卸载前，先明确停止核心并等待退出，再退出全部窗口（`.12` 起使用「退出界面」）。** 应用内确认安装会正常停止核心并真正退出当前宿主，其余窗口需要先手动退出。 Windows 安装器／卸载器拒绝替换或删除正在使用的程序，不强杀、不安排重启强删。Linux 由包管理器替换文件，不会自动升级或停止旧运行进程；跳过停服可能留下旧核心。卸载不是停服操作。

**从旧版升级，首次启动核心将 SQLite schema2 自动迁移到3，旧密钥保留为 default Key；`.16` 再从3迁移到4，保留 Key 的协议继承设置；`.21` 从4迁移到5，新增独立的平台身份与用户授权，保留供应商和 Key。旧二进制无法打开升级后的数据库，本版不提供数据库降级。**

卸载仅移除程序和快捷入口，保留用户的状态目录、数据库、秘密和恢复材料。安装不新增 Agent 授权；打开应用后仍可能执行此前已批准的托管策略。开机自启动为独立默认关闭的需求，当前没有可用设置开关，本轮不实现。

## 2. 构建入口（不执行验收）

打包与已授权的在线投放统一按 [anchor-desktop-release skill](../../.agents/skills/anchor-desktop-release/SKILL.md) 执行；本节脚本只负责生成安装包，线上交付还需发布安装包及同版清单。

### 同时生成两平台安装包

构建机要求：Linux AMD64、可用的 Linux AMD64 Docker daemon、Go ≥1.25、Python ≥3.11、Node22.22.1、npm9.2.0，以及按锁文件安装的前端依赖。图标已纳入源码；只有重生成图标时需要 Pillow。宿主无需 GTK / WebKit 开发头文件或 NSIS。

```bash
# 示例使用下一版，避免覆盖已存在的 .14：
env NODE_OPTIONS=--max-old-space-size=768 \
  ./scripts/package-desktop.sh --version 0.1.0-dev.desktop.16
```

每次重建使用新的 `X.Y.Z-dev.desktop.N`，脚本拒绝覆盖已有版本目录。此入口不接受 stable / alpha 发行，不运行测试、程序、安装器或上传动作。

流程：

1. 工具链前置检查；执行 Go 类型生成器并逐字节比对已提交的 TS 契约，不偷偷改生成文件，也不调用 `go test`。
2. TypeScript / Vite `desktop` 生产编译并拒绝 Mock 场景泄漏，交叉编译 Windows GUI 二进制。
3. 读取 Ubuntu24.04 镜像，记录实际 digest；创建任务唯一、用后删除的容器。开发库和 NSIS 只安装到容器，不改宿主系统包。
4. 容器中先编译 Linux，再依次生成 `.deb` 和 Windows NSIS 安装器（`-WX` 将编译警告视为错误）。仓库、Go 工具链和模块缓存只读挂载，构建写入独立临时目录和新版本输出目录，编译／打包以宿主普通用户 UID 执行。
5. 收集项目许可证、实际 Go 二进制模块及前端生产依赖通知；记录工具链、容器包版本、源码基线／dirty／输入摘要、产物 SHA256。两个 npm 归档遗漏的通知文件使用 `desktop/packaging/notices/` 中带来源说明的补充文件；未安装的可选平台依赖不计入，未知必需通知缺失仍停止构建。通知收集不是最终法律完整性审核，不替代公开发行的对应源码交付。

本机 wrapper：预计3072 MiB、硬限制3584 MiB、swap256 MiB，30分钟超时。Docker daemon 不受调用进程 cgroup 自动约束，因此容器另外限制内存2560 MiB、总内存+swap2816 MiB、CPU2、PID256；重任务串行。临时数据使用 guard 创建的磁盘目录。退出时清理本次容器和临时路径；基础镜像保留在 Docker 磁盘缓存，不清理其他容器、镜像或服务。

### 只编译开发可执行文件

原入口仍可用，不产出安装器：

```bash
./scripts/build-desktop.sh --target windows-amd64 --check
./scripts/build-desktop.sh --target linux-amd64 --check
# 使用尚不存在的版本号：
./scripts/build-desktop.sh --target windows-amd64 --version 0.1.0-dev.desktop.16
```

Linux 直接构建仍要求宿主 `pkg-config gtk+-3.0 webkit2gtk-4.1`；宿主高版本发行版编译的文件不能声称具有 Ubuntu24.04 最低兼容性。双平台安装包入口用容器解决开发库缺失与 glibc 基线问题，不绕过资源保护。

## 3. 产物、在线投放与实际构建状态

### 当前在线 dev 版 `.21`

`0.1.0-dev.desktop.21` 于北京时间 2026-10-13 10:15:34 公开为 dev prerelease（非 latest），构建提交 `38da0ecb07a5216a6f7b9479c22444890a0903d0`，独立 worktree 的 SOURCE.json 为 dirty=false。

三平台统一查询模型目录、账户余额、Key额度、逐请求账单、实际扣费、输入／输出和缓存 token、已返回的音频／图像／工具计费，以及当前分组倍率；保留原始 USD/quota 单位和未知字段。平台后台凭证独立持久化，复制供应商不复制用户授权。原版 OneAPI 分组倍率不支持，NewAPI 自动分组倍率未知；详见[平台兼容契约](../design/provider-interface.md#8-平台账户与计费兼容层2026-10-13当前源码)。平台查询为核心接口、客户端与源码 CLI，桌面余额／账单／倍率页面尚未接入；已安装桌面可执行程序不是通用 CLI，公开 alpha.6 CLI 不含这些命令。

本版另带入此前源码中的 Agent 托管页面：本机 discover/enroll/sync、Codex 默认与逐模型模板选择、批准应用及目录同步；真实配置和原生模型选择器本轮未验收。沿用 `.20` 的更新代理与官方订阅回退。首次启动核心将 state.db schema4→5，更旧版本顺序迁移，旧二进制不能打开升级后的数据库，不支持降级。

| 平台 | 安装包 | 字节数 |
| --- | --- | ---: |
| Windows AMD64 | `Anchor-Desktop-0.1.0-dev.desktop.21-windows-amd64-setup.exe` | 7,783,576 |
| Ubuntu24.04 AMD64 | `anchor-desktop_0.1.0-dev.desktop.21_amd64.deb` | 5,590,644 |

```text
63d83dde13dfae209beb2d71bc9723df2b0e2a78b01aa941eec53632e3a15ac8  Anchor-Desktop-0.1.0-dev.desktop.21-windows-amd64-setup.exe
777579c36efcb1bb71c05033fd40ab3b2101ddc11326efedb7028dead45d551c  anchor-desktop_0.1.0-dev.desktop.21_amd64.deb
```

产物位于 `dist/desktop/0.1.0-dev.desktop.21/`。六个附件包含两平台安装包、flat 更新清单、构建提交源码与 vendored Go 依赖、许可／构建元数据和 SHA256SUMS；全部远端大小／digest 与本地一致，标签指向实际构建提交。证据为 `publication/DRAFT_VERIFIED.json`、`PUBLISHED.json`、`TAG_VERIFIED.json` 与 `ONLINE_DOWNLOAD.json`。原始 BUILD.json 的未验收和未签名事实保持不变。

发布后用隔离数据目录执行本轮源码 `update download --channel dev --product desktop --current-version 0.1.0-dev.desktop.20 --json`，默认源发现 `.21` 并真实下载 Linux 包，大小／SHA256 通过；Windows 附件大小与 GitHub digest 核对通过。构建完成 Go→TS 契约比对、TypeScript/Vite、双平台编译、NSIS/deb。功能阶段已通过 Go 全量与补充定向测试，本轮未重复全量、race 或浏览器测试，未执行 apply、原生 GUI／PAC／安装验收，未改真实 Agent 配置或请求付费上游。

首次本地包因工作区已有未跟踪缓存标为 dirty，未发布；第二次尝试因依赖软链接布局主动终止，第三次在容器收集依赖许可时失败。最终将依赖复制进干净 worktree 后成功，前三次产物保留在独立 attempt 目录，均未上传。资源重任务串行受限；构建期间 Swap 约增加100 MiB，MemAvailable 仍约7 GiB、/tmp 42 MiB，无内存压力；任务目录交付后清理。

### 历史在线 dev 版 `.20`

`0.1.0-dev.desktop.20` 于北京时间 2026-10-12 20:12:14 公开为 dev prerelease（非 latest），从独立干净 worktree 的提交 `c7bfd78844e1a94d8b73fa2df1cf6c4eeaf60c4a` 构建。

每请求和重定向重新读取对应协议的 HTTP_PROXY / HTTPS_PROXY（含小写），再使用 ALL_PROXY / all_proxy；NO_PROXY 对环境和系统代理生效。HTTP-only 环境不再抑制 HTTPS 系统代理，非法明确配置报告失败。Windows 使用 WinHTTP 读取当前用户手动代理和 PAC，按 URL 解析、等待有界；PAC 失败可回退手动配置，仅自动发现失败可直连，不自动发送 Windows 凭据。

默认 GitHub API 阶段限时10秒，网络失败或限流后读取同仓库官方 releases.atom，按渠道与 SemVer 选择正式清单；无效元数据不回退，近期 feed 无兼容版本时报告失败，不误报无更新。附件限定同仓库同版本，保留 TLS、字节数和 SHA256 校验，不使用第三方镜像。旧 CLI alpha.6 release.json 兼容保留。旧版无法接入代理时，请先手动安装本版一次；更新页同步连接失败提示。公开 DTO 和命令参数不变，无新数据库迁移。

| 平台 | 安装包 | 字节数 |
| --- | --- | ---: |
| Windows AMD64 | `Anchor-Desktop-0.1.0-dev.desktop.20-windows-amd64-setup.exe` | 7,738,189 |
| Ubuntu24.04 AMD64 | `anchor-desktop_0.1.0-dev.desktop.20_amd64.deb` | 5,545,140 |

```text
f991ef18dc6e1db00bc4efb400a5d55f473e3e957ea0c424136e80960478dd23  Anchor-Desktop-0.1.0-dev.desktop.20-windows-amd64-setup.exe
d32da0927c5ecce1123694130efdd352aa4df1eaa62aa4be97d464014a9746d5  anchor-desktop_0.1.0-dev.desktop.20_amd64.deb
```

产物位于 `dist/desktop/0.1.0-dev.desktop.20/`。六个公开附件包含两安装包、flat 清单、对应提交与 vendored Go 依赖源码、许可／构建元数据、SHA256SUMS；远端大小／digest 与本地一致，清单 URL 与正式附件一致，标签指向实际源码提交。证据位于 `publication/DRAFT_VERIFIED.json`、`PUBLISHED.json` 和 `TAG_VERIFIED.json`，原始 BUILD.json 的未验收和未签名事实保持不变。

验证：`GOMAXPROCS=2 go test -p 1 ./internal/updater ./internal/desktop ./internal/cli` 通过，Windows updater 测试程序交叉编译成功；前端更新与传输30项测试、typecheck、lint 通过。真实隔离 TLS/CONNECT 代理覆盖版本 API、清单、安装包与跨域签名重定向；环境实时变化／优先级／绕过和官方 feed 限流／网络失败／非法元数据定向测试通过。打包包含 Go→TS 契约比对、TypeScript/Vite、双平台编译、NSIS/deb。

发布后在隔离目录用本轮源码 `update download --channel dev --product desktop --current-version 0.1.0-dev.desktop.19 --json`，从默认源发现 `.20` 并真实下载 Linux 包，大小／SHA256 校验通过；Windows 远端 digest 与本地一致。强制 API 返回429时，另从真实官方 feed 发现 `.20` 清单。证据为 `publication/ONLINE_DOWNLOAD.json` 和 `LIVE_FEED_CHECK_PUBLISHED.json`。未执行 apply、安装器、原生 PAC／GUI、race、全量或浏览器验收；未修改用户配置或请求付费模型。重操作经 resource-guard 串行限制，Swap 与 /tmp 保持稳定，交付后清理任务 worktree 与隔离下载目录。

### 历史在线 dev 版 `.19`

`0.1.0-dev.desktop.19` 于北京时间 2026-10-12 19:44:21 公开为 dev prerelease（非 latest），从独立干净 worktree 的提交 `54fa3c7b2e4d61ae26fb7ac37fdc9684769b5a58` 构建。

跨供应商加入同名公开别名会追加当前候选，保留共享策略、能力、展示及 Codex 元数据；编辑只改当前候选，删除只移除当前供应商候选及其价格。最后一个候选仍受 Binding 引用保护，真实并发 revision 冲突仍要求重读核对。

手动相对倍率范围 0.01..1000、默认1，权重为倍率倒数；倍率1与2约分配2:1请求。新模型默认加权轮询，旧模型保持顺序优先，可在供应商 → 模型 → 编辑映射切换。可用性筛选跳过停用、认证失效、冷却、半开已占用和满并发上游，保留原故障切换及流式／未知结果禁止重放边界。倍率尚不自动读取站点价格或分组。无新增数据库迁移。

| 平台 | 安装包 | 字节数 |
| --- | --- | ---: |
| Windows AMD64 | `Anchor-Desktop-0.1.0-dev.desktop.19-windows-amd64-setup.exe` | 7,699,390 |
| Ubuntu24.04 AMD64 | `anchor-desktop_0.1.0-dev.desktop.19_amd64.deb` | 5,515,384 |

```text
67a586f15d883177b69fe6cd9634f0b4dbbc68e375e0472f14ef16ef93071136  Anchor-Desktop-0.1.0-dev.desktop.19-windows-amd64-setup.exe
cbec13785a3afdf0e754e44f7dd4d8587eeb253f3a6ae4532a444645a15e7d13  anchor-desktop_0.1.0-dev.desktop.19_amd64.deb
```

产物位于 `dist/desktop/0.1.0-dev.desktop.19/`。六个公开附件包括两端安装包、flat 清单、对应构建提交与 vendored Go 依赖源码、许可／构建元数据、SHA256SUMS；GitHub 附件大小和 digest 全部一致，公开清单 URL 与附件一致，标签指向实际构建提交。证据保存为 `publication/DRAFT_VERIFIED.json`、`PUBLISHED.json`、`TAG_VERIFIED.json`，原始 BUILD.json 的未验收和未签名事实保持不变。

验证：`GOMAXPROCS=2 go test -p 1 ./internal/state ./internal/core ./internal/desktop ./internal/cli ./internal/client` 通过；前端四个文件78项测试、typecheck、lint 通过。隔离 HTTP 管理 API／CLI／Desktop Bridge 验证共享别名读写，网关模拟请求验证倍率1/2在12次请求中分配8/4，冷却跳过与恢复、并发跳过、权重更新、503切换、认证失败及全部Key不可用；删除保留其他候选价格和 Binding，最后候选删除受引用保护。打包执行 Go→TS 契约比对、TypeScript/Vite、Windows 与 Ubuntu24.04 Linux 编译和 NSIS/deb 安装包生成。

隔离目录使用源码 CLI `update download --channel dev --product desktop --current-version 0.1.0-dev.desktop.18 --json` 从默认 GitHub 源发现 `.19` 并真实下载 Linux 包，大小与 SHA256 校验通过，证据为 `publication/ONLINE_DOWNLOAD.json`。Windows 远端 digest 与本地一致。未执行 apply、安装器、原生 GUI、race、全量或浏览器验收，未请求真实付费模型或改动用户配置。打包与下载后 SwapUsed 4010 MiB、/tmp 42 MiB 保持不变，无异常增长；交付后清理任务 worktree 和隔离下载目录。

### 历史在线 dev 版 `.18`

`0.1.0-dev.desktop.18` 于北京时间 2026-10-12 18:16:23 公开为 dev prerelease（非 latest），从独立干净 worktree 的提交 `c7397cfd78ff89703139e2080b635f1ed21a1ad5` 构建。

本版将本会话已完成的模型目录兼容投放到桌面：NewAPI 等兼容服务的未版本化地址支持受限 `/v1/models` 回退，保留已有部署前缀及显式版本；支持 Anthropic / Gemini 分页。按选定 Key 返回的目录仍须显式导入映射并单独测试推理。桌面首次未配置更新渠道时按安装版本选择，dev 包默认 dev；已有渠道和自定义源保留。Windows 手动系统代理支持沿用 `.17`，PAC 未支持；Linux 使用代理环境变量。本版无新增 schema 迁移。

| 平台 | 安装包 | 字节数 |
| --- | --- | ---: |
| Windows AMD64 | `Anchor-Desktop-0.1.0-dev.desktop.18-windows-amd64-setup.exe` | 7,693,264 |
| Ubuntu24.04 AMD64 | `anchor-desktop_0.1.0-dev.desktop.18_amd64.deb` | 5,510,592 |

```text
3203dc6c45690eb979a54bbc511e4c8c118ba492687ff7dfd7c992646a79eea0  Anchor-Desktop-0.1.0-dev.desktop.18-windows-amd64-setup.exe
04ba30598845ba6aadd17c7f117194d7a280b1e3b1ec1d3eb3ce2555318ede95  anchor-desktop_0.1.0-dev.desktop.18_amd64.deb
```

产物位于 `dist/desktop/0.1.0-dev.desktop.18/`。六个公开附件包括安装包、flat 清单、对应提交与 vendored Go 依赖源码、许可／构建元数据及 SHA256SUMS；GitHub 大小与 digest 全部一致，清单 URL 与正式附件一致。发布状态保存在 `publication/PUBLISHED.json`，原始 BUILD.json 的未执行原生验收、未签名状态保持不变。

验证：`go test -p 1 ./internal/updater ./internal/desktop ./internal/catalog ./internal/core ./internal/client ./internal/cli`、前端四个测试文件共68项、lint 通过；打包执行 Go→TS 契约比对、TypeScript/Vite、Windows 与 Ubuntu24.04 Linux 编译、NSIS/deb。隔离目录用源码 CLI `update download --channel dev --product desktop --current-version 0.1.0-dev.desktop.17 --json` 发现并真实下载 `.18` Linux 包，大小／SHA256 校验通过；Windows 附件远端 digest 与本地一致。未执行 apply、安装器或原生 GUI 验收，未请求真实模型站点。

首次隔离打包在创建安装包前因 npm 命令链接未保留而停止；修复任务目录的依赖副本后同一提交成功打包，失败日志保存在 `desktop18-build-attempt1.log`。打包前后及下载后 SwapUsed 4049 MiB、/tmp 42 MiB 保持不变，没有异常增长。任务自有 worktree、依赖副本与隔离下载目录在交付后清理。

### 历史在线 dev 版 `.17`

`0.1.0-dev.desktop.17` 于 2026-10-12 公开为 dev prerelease（非 latest），从独立干净 worktree 的提交 `29cc97325a90a9025a0621df42bf4c18822e138c` 构建。

包含供应商主页／多请求地址（轮询）、移除供应商级协议、新增不填 Key、取消与关闭直接丢弃草稿；更新器在无代理环境变量时使用 Windows 当前用户系统代理及绕过列表（不执行 PAC）；NSIS 安装器检测到运行中的界面或核心时提供「重试」，提示「停止核心」→「退出界面」或 `anchor stop --approve`，不强杀进程。

| 平台 | 安装包 | 字节数 |
| --- | --- | ---: |
| Windows AMD64 | `Anchor-Desktop-0.1.0-dev.desktop.17-windows-amd64-setup.exe` | 7,689,842 |
| Ubuntu24.04 AMD64 | `anchor-desktop_0.1.0-dev.desktop.17_amd64.deb` | 5,504,156 |

```text
2cc5cc23b98a7db85031fab773ae65d9caee629d9fe73125ee178b5d1561753d  Anchor-Desktop-0.1.0-dev.desktop.17-windows-amd64-setup.exe
978b7d777385345f5de544dd399ebd6d75d7a4a74bb74e5b95656eca69dfa4f0  anchor-desktop_0.1.0-dev.desktop.17_amd64.deb
```

六项公开附件远端 digest 与本地 SHA256 一致，证据存 `publication/PUBLISHED.json` 与 `ONLINE_DOWNLOAD.log`。隔离目录使用源码 CLI `update download --channel dev --product desktop --current-version 0.1.0-dev.desktop.16` 发现并下载 `.17` Linux 包，校验通过；Windows 安装包直接下载 SHA256 一致。未调用安装器，未改真实 Agent 配置，未请求付费上游；原生窗口与安装、全量测试未执行。

### 历史在线 dev 版 `.16`

`0.1.0-dev.desktop.16` 于北京时间 2026-10-12 01:43:37 公开为 dev prerelease（非 latest），构建元数据渠道 `desktop-dev`。从独立干净 worktree 的提交 `7bd0b47fcc80789ecf80e363827f564ada6e9f32` 构建；后续发行文档提交不进入安装包。

请求与系统日志来自核心真实事件，GUI 与 CLI 共用查询、详情、JSONL 导出和保留配置。27 项列选择、顺序与三组预设保存在本机，隐藏列不影响采集。HTTP、Anchor 判定、生成状态与客户端交付分别显示；正文首字与 TPS 有明确观测条件，未知不补零。指标／隐私／有界队列与磁盘规则见[日志设计](../design/logging.md)和[字段清单](../design/log-columns.md)。本版另含已提交的供应商 Key／实际模型协议设置及列表详情同步修复，state schema3→4 自动迁移，旧二进制不能打开升级后的状态；日志独立 logs.db schema1，不补采旧请求。

| 平台 | 安装包 | 字节数 |
| --- | --- | ---: |
| Windows AMD64 | `Anchor-Desktop-0.1.0-dev.desktop.16-windows-amd64-setup.exe` | 7,682,939 |
| Ubuntu24.04 AMD64 | `anchor-desktop_0.1.0-dev.desktop.16_amd64.deb` | 5,499,456 |

```text
34ace8eec3f0ab53a99a43f4b7948e26833f082f02f6a831902ffb4365e2040d  Anchor-Desktop-0.1.0-dev.desktop.16-windows-amd64-setup.exe
f93a44ac28862410575bfb7e2e73c443d8d65ce54b33beb56a3295b56b66757b  anchor-desktop_0.1.0-dev.desktop.16_amd64.deb
```

产物位于 `dist/desktop/0.1.0-dev.desktop.16/`。六项公开附件包含两安装包、flat 清单、对应构建提交及 vendored Go 依赖源码、许可／构建元数据、SHA256SUMS，远端大小与 digest 均与本地一致。源码归档核验 vendor/modules.txt 和托盘许可证，元数据 ZIP CRC 通过。原始 BUILD.json 的 public_release=false、native_gui_verified=false、installation_verified=false、signed=false 保留，真实发布／下载证据另存 publication/PUBLISHED.json 与 ONLINE_DOWNLOAD.json。

定向 `go test -p=2 ./internal/logbook ./internal/protocol ./internal/core ./internal/client ./internal/cli ./internal/desktop` 在工作区及干净源码通过，覆盖真实隔离核心／CLI／Bridge、HTTP200校验失败、重试用量、流式取消、指标、身份校验、存储故障与保留。前端类型、lint及91项单元测试通过；浏览器分别完成13项桌面和3项日志交互，日志修正焦点后复验通过，包含列偏好、筛选、窄屏、深色、分页、系统关联、导出、保留和离线。浏览器使用模拟绑定，真实 Go Bridge 单独联调；不宣称单次16项全通过。10万条历史及800次／8客户端流式性能测试通过，无日志丢弃；实际测量与限制见日志文档。

打包包含 Go→TS 契约比对、TypeScript/Vite、Windows Go 与 Ubuntu24.04 Linux 编译及 NSIS/deb。隔离目录使用本轮源码 CLI `update download --channel dev --product desktop --current-version 0.1.0-dev.desktop.15 --json` 发现并真实下载 `.16` Linux 包，大小／SHA256校验通过；Windows GitHub digest 与本地一致。未调用 apply 或安装器，未修改真实 Agent 配置，未请求付费上游；双平台原生窗口和安装、全量／race测试未执行。

本轮一次完整打包，按 resource-guard 串行限内存运行；任务自有 worktree、容器、源导出与隔离下载目录清理结果见本次 release 记录。既有 scripts/__pycache__/ 保留。

### 历史在线 dev 版 `.15`

`0.1.0-dev.desktop.15` 于北京时间 2026-10-11 23:10:43 公开为 dev prerelease（非 latest），构建元数据渠道为 `desktop-dev`。源码为干净提交 `e9d12cab19d8c8a99d43efccc4a8bca4024253ce`，从独立 worktree 构建，后续文档提交不进入二进制，也未包含同时进行的供应商功能改动。

当前用户只运行一个 GUI 与托盘；再次启动还原最小化并显示／聚焦已有窗口，藏到托盘同样恢复。启动未就绪时合并恢复请求，已有宿主无法响应时最多等待5秒后退出而不多开；异常退出不留下持有的内核锁。GUI 锁独立于 `--data-dir`，第二次启动不切换已有窗口目录或写更新配置。后台核心与安装 helper 不参与 GUI 锁，原生命周期保留；本版没有数据库迁移。

| 平台 | 安装包 | 字节数 |
| --- | --- | ---: |
| Windows AMD64 | `Anchor-Desktop-0.1.0-dev.desktop.15-windows-amd64-setup.exe` | 7,634,694 |
| Ubuntu24.04 AMD64 | `anchor-desktop_0.1.0-dev.desktop.15_amd64.deb` | 5,449,636 |

```text
8cbf94bc497c5a7668130e5419c703ad6474ffea6862d312cd5bed5bc7657fdc  Anchor-Desktop-0.1.0-dev.desktop.15-windows-amd64-setup.exe
d92647567e672a4e2ea1e80d79c68b0b62e12ba9caf956248e1531a4515e9466  anchor-desktop_0.1.0-dev.desktop.15_amd64.deb
```

产物位于 `dist/desktop/0.1.0-dev.desktop.15/`。六个公开附件为两平台安装包、flat `update-desktop.json`、对应提交与 vendored Go 依赖源码、许可／构建元数据和 `SHA256SUMS`；发布后所有远端大小／digest 与本地一致，版本标签指向上述真实源码。源码归档含 `vendor/modules.txt`、托盘依赖许可证和新的宿主代码；元数据 ZIP CRC 通过。保留 `BUILD.json` 的 `public_release=false` 构建快照；实际发布、真实下载证据在 `publication/PUBLISHED.json` 和 `ONLINE_DOWNLOAD.json`。

定向 Go 测试通过（在工作区及独立干净源码分别执行），覆盖八个并发启动、跨进程重复启动、启动中恢复、退出拒绝、异常退出后重开、通知鉴权、核心复用和更新交接；打包包含 Go→TS 契约比对、TypeScript/Vite、Windows Go 与 Ubuntu24.04 Linux 编译、NSIS/deb。隔离目录用本轮源码 CLI `update download --channel dev --product desktop --current-version 0.1.0-dev.desktop.14 --json` 发现并真实下载 `.15` Linux 包，大小／SHA256 校验通过；Windows GitHub digest 与本地一致。未调用 apply、安装器、真实 Agent 配置或付费模型上游；全量／race／浏览器和双平台原生窗口、安装验收未运行，`native_gui_verified`、`installation_verified`、`signed` 仍为 false。

本轮只有一次完整打包。打包前后 MemAvailable 7487→7444 MiB，SwapUsed 4073 MiB、/tmp 42 MiB、PSI 0.00；真实下载及清理后 MemAvailable 7342 MiB，Swap／/tmp 无增长。独立 worktree、容器、源码导出及隔离下载目录均已清理，只保留产物和日志；其他任务的工作区改动与既有 `scripts/__pycache__/` 保留。

### 历史在线 dev 版 `.14`

`0.1.0-dev.desktop.14` 于北京时间 2026-10-11 21:44:37 公开为 dev prerelease（非 latest），构建元数据渠道为 `desktop-dev`。源码为干净提交 `fc9f1c2339e33f080779fad1be3dac652acecd6b`，从独立 worktree 构建，后续文档提交不进入二进制。

修正关闭行为：普通关闭隐藏到系统托盘／通知区域，单击托盘图标或「打开 Anchor」恢复；托盘菜单「退出界面（核心继续运行）」真正结束宿主。最小化按钮仍缩到任务栏。Linux 没有可用 StatusNotifierHost 或托盘尚未注册时允许界面退出，核心继续服务，避免留下不可恢复的隐藏窗口。显式退出和已批准的更新安装绕过关闭隐藏；本版没有数据库迁移。

| 平台 | 安装包 | 字节数 |
| --- | --- | ---: |
| Windows AMD64 | `Anchor-Desktop-0.1.0-dev.desktop.14-windows-amd64-setup.exe` | 7,625,489 |
| Ubuntu24.04 AMD64 | `anchor-desktop_0.1.0-dev.desktop.14_amd64.deb` | 5,445,600 |

```text
80060ee70b00e215adee830f0b436f034e08d193e9b02166005ceddbec068a0d  Anchor-Desktop-0.1.0-dev.desktop.14-windows-amd64-setup.exe
fda0c36f04c85913d7bef70524758018126690fac8c42652a028a19eca9b3e93  anchor-desktop_0.1.0-dev.desktop.14_amd64.deb
```

产物位于 `dist/desktop/0.1.0-dev.desktop.14/`。六个公开附件包含两安装包、flat `update-desktop.json`、对应构建提交及 vendored Go 依赖源码、许可／构建元数据和 `SHA256SUMS`；全部远端大小／digest 与本地一致，清单地址与公开附件一致。源码归档包含新增 `fyne.io/systray` 源码及许可证，元数据 ZIP CRC 通过。保留原始 `BUILD.json` 的 `public_release=false` 快照；实际发布与下载证据另存 `publication/PUBLISHED.json`、`ONLINE_DOWNLOAD.json`。

验证通过：关闭／退出／更新交接定向 Go 测试，Linux 和 Windows 托盘包编译，前端 lint，构建时 Go→TS 比对、TypeScript/Vite、Windows Go 和 Ubuntu24.04 Linux 编译、NSIS 及 `.deb` 打包。在隔离磁盘目录执行源码 CLI `update download --channel dev --product desktop --current-version 0.1.0-dev.desktop.13 --json`，从默认源发现 `.14` 并真实下载 Linux 包，大小／SHA256 通过；Windows 附件 digest 与本地一致。未调用 apply 或安装器，未使用真实配置或模型上游。全量／浏览器测试及原生托盘、安装验收未运行，`native_gui_verified`、`installation_verified`、`signed` 仍为 false。

首次打包在两平台编译成功后因隔离依赖链接在容器内不可见而停止，未生成安装器；已保留 `0.1.0-dev.desktop.14-failed-packaging/` 和 `0.1.0-dev.desktop.14-build-attempt1.log`。改为完整依赖副本后，使用同一提交、同一版本重新运行原打包流程成功，没有覆盖任何已发布附件。最终打包前后 MemAvailable 7538→7521 MiB，SwapUsed 4088→4087 MiB，/tmp 42 MiB，PSI 0.00；首次构建 swap 增加13 MiB 后稳定，无持续异常增长。任务临时 worktree、容器、源码归档和下载目录均已清理。

### 历史在线 dev 版 `.13`

`0.1.0-dev.desktop.13` 于 2026-10-11 构建并发布，更新渠道为 `dev`，构建元数据渠道为 `desktop-dev`。源码为干净提交 `fdad0ac8762219e86508a1f3c9df87ed928f0a47`，本次续接已完成的构建和草稿上传；后续发行文档提交不进入本版二进制。

新增概览页和模型映射聚合页；供应商列表直接显示健康、Key／模型数量及最近测试，并提供行内测试、编辑和更多操作。供应商详情改为「概况／Key／模型／用量」四页签整页视图，模型目录位于「模型」页签；「退出界面」「停止核心」收进顶栏「本机实例」弹层。本版没有新增数据库 schema 迁移。

已有构建完成 Go→TS 契约比对、TypeScript/Vite、Windows Go 交叉编译、Ubuntu24.04 Linux 编译、NSIS 与 `.deb` 打包。产物保存在 `dist/desktop/0.1.0-dev.desktop.13/`：

| 平台 | 安装包 | 字节数 |
| --- | --- | ---: |
| Windows AMD64 | `Anchor-Desktop-0.1.0-dev.desktop.13-windows-amd64-setup.exe` | 7,517,672 |
| Ubuntu24.04 AMD64 | `anchor-desktop_0.1.0-dev.desktop.13_amd64.deb` | 5,356,968 |

```text
e1f1e86f94a74505873967da90a8a6b4ca045d78be763481c93f04e869edfb37  Anchor-Desktop-0.1.0-dev.desktop.13-windows-amd64-setup.exe
39e13868430a571d9748c8fb190b9ddb1cc2289a9e735168e79e0f5c2a87e5e6  anchor-desktop_0.1.0-dev.desktop.13_amd64.deb
```

GitHub Release `v0.1.0-dev.desktop.13` 于北京时间 2026-10-11 21:08:33 公开为 prerelease（非 latest），tag 指向实际构建提交。六个附件包括两安装包、flat 布局 `update-desktop.json`、含 vendored Go 依赖的对应源码归档、构建／许可元数据包和 `SHA256SUMS`。全部远端附件大小／digest 与本地一致；清单 URL 与正式附件地址一致，源码归档含 `vendor/modules.txt`，元数据 ZIP 的 CRC 检查通过。`BUILD.json` 保留构建时 `public_release=false` 的事实快照，实际发布与下载凭据另存于本版 `publication/PUBLISHED.json` 和 `ONLINE_DOWNLOAD.json`。

使用本轮源码 CLI，在 guard 自动创建的隔离磁盘目录执行 `update download --channel dev --product desktop --current-version 0.1.0-dev.desktop.12 --json`，从默认源发现 `.13` 并真实下载 Linux 安装包，大小／SHA256 校验通过。命令经 guard 限制内存，`GOMAXPROCS=2`、`go run -p=2`；未调用 apply、安装器、真实数据目录或模型上游。Windows 通过远端附件 digest 核对。

复用源码提交的类型、lint、Vitest 81/81、Playwright 原型 16/16 与模拟宿主 12/12 结果，本轮未重复运行浏览器或全量测试。`native_gui_verified`、`installation_verified`、`signed` 仍为 false；真实核心弹层视觉及原生窗口／安装尚未验收。下载前后 MemAvailable 7630→7611 MiB、SwapUsed 4076 MiB、/tmp 42 MiB、PSI 0.00，无异常资源增长。下载临时目录和上个会话中断遗留的本版源码归档临时目录已清理。

### 历史在线 dev 版 `.12`

`0.1.0-dev.desktop.12` 于 2026-10-11 构建并发布，更新渠道为 `dev`，构建元数据渠道为 `desktop-dev`。源码为干净提交 `9b3d4fabd2fb5e5bfe536104a9dc86721cac0f2e`；文档和发行记录后续提交，不改已发布安装包。

普通关闭由 Wails 宿主收起到任务栏。新增 `ExitDesktop` 桥接供「退出界面」使用；显式退出和已批准的更新安装绕过关闭最小化，保证更新 helper 可以等待宿主真正退出。两种窗口操作都不自行停止核心。

```bash
env NODE_OPTIONS=--max-old-space-size=768 \
  ./scripts/package-desktop.sh --version 0.1.0-dev.desktop.12
```

命令返回0，完成 Go→TS 契约比对、TypeScript/Vite、Windows Go 交叉编译、Ubuntu24.04 Linux 编译、NSIS 与 `.deb` 打包。产物保存在 `dist/desktop/0.1.0-dev.desktop.12/`：

| 平台 | 安装包 | 字节数 |
| --- | --- | ---: |
| Windows AMD64 | `Anchor-Desktop-0.1.0-dev.desktop.12-windows-amd64-setup.exe` | 7,508,388 |
| Ubuntu24.04 AMD64 | `anchor-desktop_0.1.0-dev.desktop.12_amd64.deb` | 5,349,392 |

```text
6e0e3ad702dddb1d17b6f78173f08419a1b7430a7ea0471358ee4e06f1d06e4f  Anchor-Desktop-0.1.0-dev.desktop.12-windows-amd64-setup.exe
38d29bf3b905faaace3991e91288604dc4365d68a3a1ed0f1e431c32bebe6244  anchor-desktop_0.1.0-dev.desktop.12_amd64.deb
```

GitHub Release `v0.1.0-dev.desktop.12` 于北京时间 2026-10-11 08:09:22 公开为 prerelease（非 latest）。附件包含两安装包、flat 布局 `update-desktop.json`、含 vendored Go 依赖的对应源码归档、构建／许可元数据包和 `SHA256SUMS`。构建时 `BUILD.json` 保留 `public_release=false` 的原始快照；实际投放状态记录在 Release 和本次任务记录中，不改写已产出的文件。

实际检查：`TestTypeScriptContractDrift` 与前端 `typecheck` 通过；新包文件哈希与构建清单一致；GitHub 已上传附件 digest 与本地一致。在 guard 磁盘临时数据目录运行现有 CLI 的 `update download --channel dev --product desktop --current-version 0.1.0-dev.desktop.11`，成功从默认公开源发现 `.12`，真实下载 Linux 安装包并通过大小／SHA256 校验。未调用 apply 或安装器，未使用真实数据目录或模型上游。

`native_gui_verified`、`installation_verified`、`signed` 仍为 false。未运行全量测试、浏览器或原生验收，不把下载成功写成系统安装完成。构建日志的容器 man 链接与 `dpkg-shlibdeps` 暂存目录提示未阻止打包。构建结束 MemAvailable 8672 MiB、SwapUsed 4122 MiB、/tmp 30 MiB、PSI 0.00；在线下载后为 8522 / 4094 / 30 MiB、PSI 0.00，无异常增长，任务临时目录和构建容器已清理。

后续迭代应递增 `dev.desktop.N`，按[在线投放流程](../design/online-updates.md#3-快速测试投放)上传安装包和同版清单，再发布 dev Release；仅本地打包不会出现在应用的在线更新中。

### 历史供应商第一优先功能版 `.11`（本地产物保留）

候选 `0.1.0-dev.desktop.11`，`desktop-dev`，2026-10-11 本地构建。源码提交 `a35ecb6a584c0b01480c5011cf7dabd1c776ac3e`，构建开始时 `SOURCE.json` 的 `dirty=false`；后续发行文档提交不进入本版二进制。

```bash
env NODE_OPTIONS=--max-old-space-size=768 \
  ./scripts/package-desktop.sh --version 0.1.0-dev.desktop.11
```

命令返回0。Go → TS 契约生成比对、TypeScript / Vite desktop 生产编译、Windows Go 交叉编译、Ubuntu24.04 容器 Linux Go 编译、NSIS（`-WX`）与 `.deb` 打包完成。供应商详情顶部提供改名、复制、删除，工作台包含「密钥与状态」「模型与测试」「用量」三个页签，并复用 CLI / 核心管理能力。

产物位于 `dist/desktop/0.1.0-dev.desktop.11/`：

- Windows AMD64：`windows-amd64/Anchor-Desktop-0.1.0-dev.desktop.11-windows-amd64-setup.exe`，7,507,182 bytes。
- Ubuntu24.04 AMD64：`linux-amd64/anchor-desktop_0.1.0-dev.desktop.11_amd64.deb`，5,348,912 bytes；Debian 版本 `0.1.0~dev.desktop.11`，架构 `amd64`。
- 日志：`dist/desktop/0.1.0-dev.desktop.11-build.log`；根目录含 `SOURCE.json` / `BUILD_ENVIRONMENT.txt`，各目标目录含程序、许可／通知、`BUILD.json` / `SHA256SUMS`。

以下 SHA256 已读取最终文件并与新包清单核对：

```text
cccaf747dc4d146d71769070832a2c778fabcee127e74c592b716901b7d56bf4  Anchor-Desktop-0.1.0-dev.desktop.11-windows-amd64-setup.exe
8dca2974a2b6b02873023b1057eac1d8c4ed8f830a6de47bbb70262c8a577314  anchor-desktop_0.1.0-dev.desktop.11_amd64.deb
```

新包 Windows 6项、Linux 7项 `SHA256SUMS` 均匹配；已核对 `BUILD.json` 的版本／目标／源码和 Debian 包字段。元数据仍为 `native_gui_verified=false`、`installation_verified=false`、`public_release=false`、`signed=false`。未安装新包、启动程序或重验旧包；协议转换仅覆盖设计文档列出的文本／函数工具子集，不宣称完整兼容所有 Agent。

构建仍有容器内 man 手册链接提示及 `dpkg-shlibdeps` 暂存目录警告，命令返回0且两安装包生成；不宣称全链路零告警。资源保护串行执行：开始 MemAvailable 8518 MiB、SwapUsed 4122 MiB、/tmp 30 MiB；结束 MemAvailable 8617 MiB、SwapUsed 4129 MiB、/tmp 30 MiB、PSI 0.00，无异常增长。打包脚本完成任务容器和临时目录清理。

**安装前先停止旧核心并关闭全部窗口。** `.11` 将 schema 2 迁移到3，旧程序不能打开升级后的数据库；不提供数据库降级。该版本为本地交付，未生成 `update-desktop.json`、上传包、发布 Release 或部署更新源。

### 历史在线更新启动版 `.9`（本地产物保留）

候选 `0.1.0-dev.desktop.9`，`desktop-dev`，2026-10-11 本地构建。构建源码为已提交的 `d07799d6451f53e41ae13f1dce8d4e8563bea820`，`SOURCE.json` 的 `dirty=false`；没有临时修改源码或修改安装包内的版本信息。

```bash
env NODE_OPTIONS=--max-old-space-size=768 \
  ./scripts/package-desktop.sh --version 0.1.0-dev.desktop.9
```

命令返回0。Go → TS 契约生成比对、TypeScript / Vite desktop 生产编译、Windows Go 交叉编译、Ubuntu24.04 容器 Linux Go 编译、NSIS（`-WX`）与 `.deb` 打包完成。本版包含“设置 → 在线更新”入口及受限检查／下载／安装桥接；没有启动生成的应用或安装器。

产物位于 `dist/desktop/0.1.0-dev.desktop.9/`：

- Windows AMD64：`windows-amd64/Anchor-Desktop-0.1.0-dev.desktop.9-windows-amd64-setup.exe`，7,422,669 bytes。
- Ubuntu24.04 AMD64：`linux-amd64/anchor-desktop_0.1.0-dev.desktop.9_amd64.deb`，5,266,492 bytes；Debian 包版本为 `0.1.0~dev.desktop.9`，架构为 `amd64`。
- 构建日志：`dist/desktop/0.1.0-dev.desktop.9-build.log`；根目录含 `SOURCE.json` / `BUILD_ENVIRONMENT.txt`，各目标目录含程序、许可／通知、`BUILD.json` / `SHA256SUMS`。

以下 SHA256 已重新读取最终文件并与元数据核对：

```text
b1e97b4deff0f790bb5eafaa3ffd687906e96eb2f7f5fad06bcbe603aa4a0513  Anchor-Desktop-0.1.0-dev.desktop.9-windows-amd64-setup.exe
162131f4e56a544866c0504e05c3fc97eb78067fb943a8aff2aa4589f6d2e2c0  anchor-desktop_0.1.0-dev.desktop.9_amd64.deb
```

实际静态核验：两目标 `sha256sum -c SHA256SUMS` 全部通过，`BUILD.json` 的版本／渠道／源码标识及所有文件哈希一致；`file` 确认 PE32+ AMD64 GUI / NSIS 安装器和 ELF AMD64 / Debian 包格式；`go version -m` 确认 Go 1.26.0、目标平台及生产构建标签，构建日志与程序嵌入版本均为 `.9`。`dpkg-deb --field/--contents` 确认包版本、动态库依赖、主程序、应用入口、图标及许可／通知。前端产物含更新桥接方法及契约标记，不含构建脚本禁止的原型专用标记。这些核验不替代安装或原生行为测试。

构建日志仍包含容器依赖安装时缺少 man 手册链接的提示及 `dpkg-shlibdeps` 暂存目录提示；后者返回0，后续打包完成，不宣称全链路零告警。两平台元数据均为 `native_gui_verified=false`、`installation_verified=false`、`public_release=false`、`signed=false`。未安装、启动、原生验收、上传或发布；发行记录状态为 partial（本地打包成功，原生及公开发行门禁未执行）。

资源保护串行执行；构建前 MemAvailable 8749 MiB、SwapUsed 4311 MiB、/tmp 28 MiB、PSI 0.00，静态核验时 MemAvailable 8780 MiB、SwapUsed 4307 MiB、/tmp 28 MiB、PSI 0.00，无异常增长；本轮容器已删除。

**首次安装 `.9` 前，先在旧版明确停止核心并等待退出，再关闭全部窗口后覆盖安装。** `.9` 只是在线更新启动包，本轮没有传入 `--update-base-url`，没有生成 `update-desktop.json`、部署更新源或创建 Release；当时仅本地交付；现已有 `.12` dev Release，`.9` 用户可选择 dev 渠道在线升级。

### 历史标题栏修正版 `.8`（本地产物保留）

候选 `0.1.0-dev.desktop.8`，`desktop-dev`，2026-10-11 本地构建，未公开发布。`./scripts/package-desktop.sh --version 0.1.0-dev.desktop.8` 返回0；TypeScript / Vite desktop、Windows Go 交叉编译、Ubuntu24.04 容器 Linux Go 编译、NSIS（`-WX`）与 `.deb` 打包完成。

本版开启 Windows / Linux Frameless，窗口控制融入现有 shadcn Blocks header；接入拖动区域、双击最大化／还原及窗口控制 MVC。关闭按钮只退出 GUI，不调用核心停服。原生视觉、拖动、缩放及运行行为未验收。

产物位于 `dist/desktop/0.1.0-dev.desktop.8/`：

- Windows AMD64：`windows-amd64/Anchor-Desktop-0.1.0-dev.desktop.8-windows-amd64-setup.exe`，7,387,173 bytes。
- Ubuntu24.04 AMD64：`linux-amd64/anchor-desktop_0.1.0-dev.desktop.8_amd64.deb`，5,233,872 bytes。

以下 SHA256 来自生成的 `BUILD.json` / `SHA256SUMS`，不代表安装验收：

```text
0c0299c650d4bb0dda44a3650fec881e941046dfa3f5045f5f22eb11568e47d9  Anchor-Desktop-0.1.0-dev.desktop.8-windows-amd64-setup.exe
bceced55e5cfb2ff29ec755f5d2030c26b87af94c668177ec0c74a0aba69cf76  anchor-desktop_0.1.0-dev.desktop.8_amd64.deb
```

构建日志保留容器依赖安装时缺少 man 手册链接的提示及 `dpkg-shlibdeps` 暂存目录提示；后者返回0且后续打包完成，不宣称全链路零告警。两平台元数据均为 `native_gui_verified=false`、`installation_verified=false`、`public_release=false`。按用户要求，未安装、启动应用或核心，未运行测试或验收；发行记录状态为 partial（本地打包成功，原生及公开发行门禁未执行）。

资源保护结束状态：MemAvailable 9105 MiB、SwapUsed 4322 MiB、/tmp 28 MiB、PSI 0.00；相对构建前无异常增长，未残留本轮构建容器。旧 `.7` 不会自动更新；更新前先在旧版停止核心并关闭窗口，再使用新包覆盖安装。

### 上一安装包版本 `.7`（历史结果；本地产物已清理）

> 2026-10-11 此前工作区整理时：`dist/desktop/` 只保留了 `.8` 产物及其构建日志；`.1`～`.7` 本地目录和 `.4`～`.7` 构建日志已移入系统回收站，不再作为本地交付物。以下内容和各自任务记录仅作历史结果，路径不再存在。

候选：`0.1.0-dev.desktop.7`，`desktop-dev`，本地构建，未公开发布。

```text
dist/desktop/0.1.0-dev.desktop.7/
  SOURCE.json                         # 源码／工具链／容器镜像标识
  BUILD_ENVIRONMENT.txt               # Ubuntu 容器实际依赖版本
  windows-amd64/
    anchor-desktop.exe
    Anchor-Desktop-0.1.0-dev.desktop.7-windows-amd64-setup.exe
    LICENSE, README.txt, THIRD_PARTY_NOTICES.txt
    BUILD.json, SHA256SUMS
  linux-amd64/
    anchor-desktop
    anchor-desktop_0.1.0-dev.desktop.7_amd64.deb
    LICENSE, README.txt, THIRD_PARTY_NOTICES.txt, RUNTIME_DEPENDENCIES.txt
    BUILD.json, SHA256SUMS
```

当前构建结果（2026-10-11）：`./scripts/package-desktop.sh --version 0.1.0-dev.desktop.7` 返回0；TypeScript / Vite desktop 编译、Windows 交叉编译、Ubuntu 容器 Linux 编译、`.deb` 与 NSIS 安装器打包均完成。NSIS 使用 `-WX` 完成编译，无该编译器警告。没有安装、启动、测试或公开发布。

- Windows AMD64 安装器：`windows-amd64/Anchor-Desktop-0.1.0-dev.desktop.7-windows-amd64-setup.exe`，7,385,972 bytes。
- Ubuntu24.04 AMD64 安装包：`linux-amd64/anchor-desktop_0.1.0-dev.desktop.7_amd64.deb`，5,235,360 bytes。

以下 SHA256 抄自打包生成的 `BUILD.json` / `SHA256SUMS`，不是安装验收结果：

```text
76f1d73ee026a29903f033e582766f788f3375d541ae4d59ed88b81c891bfbb0  Anchor-Desktop-0.1.0-dev.desktop.7-windows-amd64-setup.exe
41733dbd0fbc6024d0c4e01d7fc6dd2ac2493e6448350ea5ffeb31f408877ed0  anchor-desktop_0.1.0-dev.desktop.7_amd64.deb
```

构建日志仍有一条 `dpkg-shlibdeps` 暂存目录提示：`binaries to analyze should already be installed in their package's directory`。该命令返回0，生成的 `RUNTIME_DEPENDENCIES.txt` 包含 GTK3、WebKitGTK4.1、libc 等依赖，后续 `.deb` 打包完成；不能因此将此轮描述为全链路零告警或依赖安装已验证。

`BUILD.json` 分别保留 `native_gui_verified=false`、`installation_verified=false`、`public_release=false` 和用户要求跳过验收的说明。开发安装器未签名，不做生产就绪承诺。`.1` / `.2` / `.3` Windows 开发 EXE 的历史结果见各自任务记录（本地产物已清理）。本轮 `.4` 仅产出 Windows 裸 EXE：Ubuntu 编译因发行版 Go 的跨目录符号链接未完整挂载而失败，没有安装包；修正只读挂载布局和 Go 通知文件收集后，`.5` 两平台裸程序均编译成功，但 npm 包缺少独立许可证文件导致通知收集中断，没有生成安装器。补充两份包通知并排除未安装的可选平台依赖后，`.6` 安装包生成成功，但 NSIS 报出卸载命令字符串的引号错误，标为非交付产物。`.7` 修正引号、启用 NSIS 编译警告视为错误，并按 Debian 标准暂存路径提取动态库依赖；`.4` / `.5` / `.6` 为非交付中间产物（本地产物已清理）。

## 4. 本轮验证边界与公开发行门禁

本轮 `.11` 先完成与新增能力直接相关的 Go 定向测试（隔离数据库、回环上游、桌面桥接和协议转换）及前端类型检查、55项单元测试，再编译并打包 Windows / Linux。CLI / client 在定向 Go 命令中完成编译，没有匹配到该筛选条件的测试，不将其计为 CLI 操作测试通过。未运行完整测试、race、浏览器或原生安装验收，未请求真实付费供应商或线上 models.dev，未改真实 Agent 配置。只核对新包的元数据与哈希，不重验历史包。此前在线更新实现轮的测试不算本轮重跑。

后续真正做原生验收时，必须使用 `anchor-desktop --data-dir <新的独立测试目录>`，无真实 Agent 绑定、无真实付费上游：

1. Windows WebView2 / Linux WebKitGTK 真实窗口、JS↔Go、中文输入、缩放、主题；Linux 分别记录 X11 / Wayland。
2. 首次安全初始化、再次复用、并发启动、身份／授权／端口／版本错误拒绝；关闭宿主后核心存活，重新打开复用。关窗存活不等于注销／重启存活。
3. 供应商写入、revision 冲突、密钥三态、未知结果、断开禁写；确认停服及正常条件恢复。
4. 两端首次安装、更新、卸载、保留数据、快捷方式；运行时缺失、离线或下载失败、Windows 文件占用拒绝与 Linux 旧进程处理。
5. 对应源码／依赖许可证、安全审计、签名与公开分发流程；不把本地未提交源码包直接上传为正式 Release。

本轮没有改变前端依赖，先前记录的安全审计遗留项未重跑、未宣称修复。公开发行前仍需处理。每次实际打包按任务记录规则生成 release 更新记录，按实际交付范围标明状态：本地打包成功不代表公开发行通过；产物生成后必需的上传、重启或验证失败时记为 partial。


## 5. 在线更新清单生成

`package-desktop.sh --version X.Y.Z-dev.desktop.N --update-base-url <HTTPS版本目录>` 可在成功打包后生成根目录 `update-desktop.json`，URL 对应 `版本目录/target/安装包名`。不传参数保持旧打包输出；参数会先验证，不联网上传。已有最终文件可用 `scripts/update_manifest.py` 生成清单；默认 GitHub Release 附件路径一般使用 flat 布局，自建站点可使用 target。先上传包与许可证／对应源码，校验后再切换渠道清单，保留旧版，不覆盖已发布安装包。首次必须人工安装含更新入口的新包；旧 desktop.8 不支持更新检查。当前已本地生成 `.11` 功能安装包（`.9` 为首次包含更新入口的历史启动版），但未提供真实更新源地址，未生成 `update-desktop.json`、部署测试更新源或发布 Release。
