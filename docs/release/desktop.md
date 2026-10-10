# 桌面开发版构建与安装

- 更新：2026-10-11。
- 本轮源码新增：手动在线更新（尚未打包／发布），见[在线更新与测试渠道投放](../design/online-updates.md)。下面 `.8` 的“不测试”说明是此前打包任务范围，不约束本轮隔离测试。
- 范围：供应商第一版、本机自动启动／核验复用核心、关窗不停服；保持现有 shadcn Blocks sidebar-07 / neutral 界面。
- 本轮：移除 Windows / Linux 原生系统标题栏，改用融入现有 header 的窗口控制；候选 `0.1.0-dev.desktop.8` 已完成双平台编译与安装包打包，未做原生验收。此前 `.7` 安装包保留，不覆盖。
- **按用户要求，本轮不安装、不启动程序、不做测试或原生验收。编译和打包成功不代表已验证可用。** 后续公开发行门禁不因此取消；公开 alpha.6 下载仍为 CLI，不被替换。

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
sudo apt install ./anchor-desktop_0.1.0-dev.desktop.8_amd64.deb
```

文件安装到 `/usr/bin/anchor-desktop`，应用菜单入口位于 `/usr/share/applications/anchor-desktop.desktop`；包含 SVG / PNG 图标。包的 GTK3 / WebKitGTK4.1 / libc 等依赖从实际 ELF 用 `dpkg-shlibdeps` 提取。没有安装／卸载脚本自动启服务，没有 systemd unit 或自启动项。安装后从应用菜单打开 GUI，无需手动 `init` / `serve`。没有图形会话的服务器继续使用 CLI。

### 两平台共同行为与数据保留

打开窗口后，Go 宿主安全初始化缺失状态、启动独立后台核心或核验复用已运行实例，连接就绪后进入供应商页。供应商列表、新增／编辑、启用／停用与密钥保留／替换／清除复用既有核心。关窗只退出界面，核心继续运行；停止核心必须在界面单独确认（或用兼容 CLI `stop --approve`）。停服请求被接受不等于恢复完成。

**更新或卸载前，先明确停止核心，等待退出，再关闭全部窗口。** Windows 安装器／卸载器拒绝替换或删除正在使用的程序，不强杀、不安排重启强删。Linux 由包管理器替换文件，不会自动升级或停止旧运行进程；跳过停服可能留下旧核心。卸载不是停服操作。

卸载仅移除程序和快捷入口，保留用户的状态目录、数据库、秘密和恢复材料。安装不新增 Agent 授权；打开应用后仍可能执行此前已批准的托管策略。开机自启动为独立默认关闭的需求，当前没有可用设置开关，本轮不实现。

## 2. 构建入口（不执行验收）

### 同时生成两平台安装包

构建机要求：Linux AMD64、可用的 Linux AMD64 Docker daemon、Go ≥1.25、Python ≥3.11、Node22.22.1、npm9.2.0，以及按锁文件安装的前端依赖。图标已纳入源码；只有重生成图标时需要 Pillow。宿主无需 GTK / WebKit 开发头文件或 NSIS。

```bash
./scripts/package-desktop.sh --version 0.1.0-dev.desktop.8
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
./scripts/build-desktop.sh --target windows-amd64 --version 0.1.0-dev.desktop.9
```

Linux 直接构建仍要求宿主 `pkg-config gtk+-3.0 webkit2gtk-4.1`；宿主高版本发行版编译的文件不能声称具有 Ubuntu24.04 最低兼容性。双平台安装包入口用容器解决开发库缺失与 glibc 基线问题，不绕过资源保护。

## 3. 本地产物与实际构建状态

### 当前标题栏修正版 `.8`

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

> 2026-10-11 工作区整理：`dist/desktop/` 只保留当前 `.8` 产物及其构建日志；`.1`～`.7` 本地目录和 `.4`～`.7` 构建日志已移入系统回收站，不再作为本地交付物。以下内容和各自任务记录仅作历史结果，路径不再存在。

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

## 4. 本轮不执行的验收与公开发行门禁

用户已明确本轮不用验收，因此不运行单元／集成／浏览器测试，不安装新包、不拉起真实 GUI 或核心。此前宿主实现轮的 Go／模拟浏览器测试只证明当时范围，不能作为新安装包已验收的证据。

后续真正做原生验收时，必须使用 `anchor-desktop --data-dir <新的独立测试目录>`，无真实 Agent 绑定、无真实付费上游：

1. Windows WebView2 / Linux WebKitGTK 真实窗口、JS↔Go、中文输入、缩放、主题；Linux 分别记录 X11 / Wayland。
2. 首次安全初始化、再次复用、并发启动、身份／授权／端口／版本错误拒绝；关闭宿主后核心存活，重新打开复用。关窗存活不等于注销／重启存活。
3. 供应商写入、revision 冲突、密钥三态、未知结果、断开禁写；确认停服及正常条件恢复。
4. 两端首次安装、更新、卸载、保留数据、快捷方式；运行时缺失、离线或下载失败、Windows 文件占用拒绝与 Linux 旧进程处理。
5. 对应源码／依赖许可证、安全审计、签名与公开分发流程；不把本地未提交源码包直接上传为正式 Release。

本轮没有改变前端依赖，先前记录的安全审计遗留项未重跑、未宣称修复。公开发行前仍需处理。每次实际打包按任务记录规则生成 release 更新记录，未完成公开门禁标为 partial。


## 5. 在线更新清单生成

`package-desktop.sh --version X.Y.Z-dev.desktop.N --update-base-url <HTTPS版本目录>` 可在成功打包后生成根目录 `update-desktop.json`，URL 对应 `版本目录/target/安装包名`。不传参数保持旧打包输出；参数会先验证，不联网上传。已有最终文件可用 `scripts/update_manifest.py` 生成清单；默认 GitHub Release 附件路径一般使用 flat 布局，自建站点可使用 target。先上传包与许可证／对应源码，校验后再切换渠道清单，保留旧版，不覆盖已发布安装包。首次必须人工安装含更新入口的新包；旧 desktop.8 不支持更新检查。本任务未生成新安装包、部署测试更新源或发布 Release。
