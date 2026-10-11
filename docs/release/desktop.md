# 桌面开发版构建与安装

- 更新：2026-10-11。
- 当前桌面 dev 版：`0.1.0-dev.desktop.13`，2026-10-11 已发布到 GitHub Releases。Windows / Linux 安装包与 `update-desktop.json` 同版投放，新增概览、供应商健康列表、四页签整页详情和模型映射聚合页。
- 范围：供应商删除／复制／改名、多 Key 与独立冷却熔断、模型目录／映射／显式测试、三种协议转换，以及用量／价格工作台；沿用 shadcn Blocks sidebar-07 / neutral 界面。详见[第一优先设计与实现](../design/provider-priority-one.md)。
- 已有 `.9`～`.12`：**设置 → 在线更新 → dev · 快速测试 → 检查更新 → 下载更新 → 安装更新**。默认 GitHub 源现已提供 `.13`；未配置渠道时仍为 alpha，需要手动选择 dev。旧 `.8` 无更新入口，首次须手动安装；历史包及日志保留。见[在线更新与测试渠道投放](../design/online-updates.md)。
- 本轮复用 UI 提交已通过的类型、lint、81 个单元测试及 16 个原型／12 个模拟宿主浏览器用例，完成两平台编译打包、发布附件哈希核对及真实更新下载；**未安装或启动真实应用，未做原生窗口／安装验收**。这是 dev 预发行，未签名；此前 alpha.6 CLI 下载未被替换。

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
sudo apt install ./anchor-desktop_0.1.0-dev.desktop.13_amd64.deb
```

文件安装到 `/usr/bin/anchor-desktop`，应用菜单入口位于 `/usr/share/applications/anchor-desktop.desktop`；包含 SVG / PNG 图标。包的 GTK3 / WebKitGTK4.1 / libc 等依赖从实际 ELF 用 `dpkg-shlibdeps` 提取。没有安装／卸载脚本自动启服务，没有 systemd unit 或自启动项。安装后从应用菜单打开 GUI，无需手动 `init` / `serve`。没有图形会话的服务器继续使用 CLI。

### 两平台共同行为与数据保留

打开窗口后，Go 宿主安全初始化缺失状态、启动独立后台核心或核验复用已运行实例，连接就绪后进入供应商页。供应商列表、新增／编辑、删除／复制／改名、模型映射、多 Key、显式测试和用量工作台均复用核心管理能力。从 `.12` 起，点击关闭或系统关闭会收起到任务栏，点击任务栏可恢复；独立「退出界面」才退出当前宿主。两者都保留后台核心；停止核心必须在界面单独确认（或用兼容 CLI `stop --approve`）。从 `.13` 起，「退出界面」「停止核心」位于顶栏「本机实例」弹层。停服请求被接受不等于恢复完成。

**手动安装或卸载前，先明确停止核心并等待退出，再退出全部窗口（`.12` 起使用「退出界面」）。** 应用内确认安装会正常停止核心并真正退出当前宿主，其余窗口需要先手动退出。 Windows 安装器／卸载器拒绝替换或删除正在使用的程序，不强杀、不安排重启强删。Linux 由包管理器替换文件，不会自动升级或停止旧运行进程；跳过停服可能留下旧核心。卸载不是停服操作。

**从旧版升级到 `.11` 或更新版本，首次启动核心会将 SQLite schema 2 自动迁移到3，旧密钥保留为 default Key。旧二进制无法重新打开升级后的数据库，本版不提供数据库降级。**

卸载仅移除程序和快捷入口，保留用户的状态目录、数据库、秘密和恢复材料。安装不新增 Agent 授权；打开应用后仍可能执行此前已批准的托管策略。开机自启动为独立默认关闭的需求，当前没有可用设置开关，本轮不实现。

## 2. 构建入口（不执行验收）

打包与已授权的在线投放统一按 [anchor-desktop-release skill](../../.agents/skills/anchor-desktop-release/SKILL.md) 执行；本节脚本只负责生成安装包，线上交付还需发布安装包及同版清单。

### 同时生成两平台安装包

构建机要求：Linux AMD64、可用的 Linux AMD64 Docker daemon、Go ≥1.25、Python ≥3.11、Node22.22.1、npm9.2.0，以及按锁文件安装的前端依赖。图标已纳入源码；只有重生成图标时需要 Pillow。宿主无需 GTK / WebKit 开发头文件或 NSIS。

```bash
# 示例使用下一版，避免覆盖已存在的 .12：
env NODE_OPTIONS=--max-old-space-size=768 \
  ./scripts/package-desktop.sh --version 0.1.0-dev.desktop.14
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
./scripts/build-desktop.sh --target windows-amd64 --version 0.1.0-dev.desktop.14
```

Linux 直接构建仍要求宿主 `pkg-config gtk+-3.0 webkit2gtk-4.1`；宿主高版本发行版编译的文件不能声称具有 Ubuntu24.04 最低兼容性。双平台安装包入口用容器解决开发库缺失与 glibc 基线问题，不绕过资源保护。

## 3. 产物、在线投放与实际构建状态

### 当前在线 dev 版 `.13`

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
