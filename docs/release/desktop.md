# 桌面开发版构建与安装

- 更新：2026-10-17。
- 当前线上桌面 dev 版：[0.1.0-dev.desktop.50](https://github.com/candyflosssss/Anchor/releases/tag/v0.1.0-dev.desktop.50)，北京时间 2026-10-17 23:52:30 发布；非 latest 的 prerelease，两平台包与 flat 清单同版投放。
- 本版为最近功能统一集成包，包含内置 mihomo／代理设置、供应商与Key代理继承／直连／覆盖、映射重试、本机与远程核心重启、Agent统一更新、Skill库、日志折叠及对话推理；范围与构建轮验证见[下节](#最近功能统一集成包-50已发布-dev未安装)。不含旧工作区余额缓存／刷新改动。
- 构建：沿用干净隔离提交 `b456835e41a8595e96577d07827cfc10f3345603` 的既有同版安装包，不重构建。**核心将 state.db schema8 迁移到 schema9；升级前备份，核心与桌面须配套，旧核心不能打开新库，不支持数据库降级。**
- 更新入口：设置 → 在线更新 → dev · 快速测试 → 检查更新 → 下载更新 → 安装更新。沿用 restart_handoff；desktop.34 及更早版首次仍须备份、停止旧核心并退出界面，再人工安装新版。升级有停机窗口，不承诺会话自动重连，SSH 远端另行升级。
- 本次在线验证：六附件大小／SHA256 digest、tag revision、对应提交源码逐文件内容／构建输入摘要、Anchor与mihomo的Go vendor、元数据ZIP CRC均通过。隔离构建源码CLI从默认GitHub dev源以 `.47` 为当前版真实发现并下载 `.50` Linux包，大小／SHA256通过；Windows远端附件digest一致。保持非latest，原有无stable/latest状态不变。安装器未签名，未运行安装／apply、原生窗口、真实SSH或付费上游验收。
- 当前本机安装：本机 CLI／核心／GUI 已于2026-10-17 22:39统一升级为 `.47`；备份 `/home/mosaki/.config/anchor/backups/pre-desktop47-20261017-223946`（目录0700、文件0600），正常保留交接后恢复原有受限 transient user units，无新增持久服务或自启动。core／gateway／agent_management ready，运行 ELF 与 .47 构建哈希一致。实例身份、配置表、绑定凭据、托管文件及其缺失状态和原始恢复基线保持，Codex／DSH 均维持 applied；不借安装重新接管。此为用户目录升级，不是 apt／NSIS／应用内 apply 或原生窗口验收，SSH 远端未更新。

## 最近功能统一集成包 .50（已发布 dev，未安装）

`0.1.0-dev.desktop.50` 从干净基线 `b456835e41a8595e96577d07827cfc10f3345603` 构建，`SOURCE.json` dirty=false。这里的“统一集成”指最近各会话已提交的功能，不把旧工作区改动一并采纳；旧余额缓存／单行刷新源码仍作为原有未提交工作保留，不在本包中。

| 最近功能 | 已包含的来源提交 |
| --- | --- |
| 内置 mihomo、侧栏代理设置、供应商代理和 Key 继承／直连／覆盖 | `090a729` |
| 瞬态失败同渠道重试及模型映射重试次数设置 | `25bd34c`、`6d3d414` |
| 托盘完整退出、本机安全重启和离线恢复 | `4817256`、`cd2c675` |
| 按已绑定实例身份重启远程核心 | `b456835`（在 .48 本地包之后补入） |
| Agent 统一“更新配置”、外部漂移接管 | `ee172a6`、`0d9f847` |
| Skill 中心库、版本管理及固定版本挂载 | `26dd199` |
| 日志运行趋势折叠 | `b283c83` |
| 供应商多轮对话测试及推理强度选择 | `9e830a9`、`5fac9d1` |

- 本地产物在 `dist/desktop/0.1.0-dev.desktop.50/`：Windows AMD64 `Anchor-Desktop-0.1.0-dev.desktop.50-windows-amd64-setup.exe`，Ubuntu24.04 AMD64 `anchor-desktop_0.1.0-dev.desktop.50_amd64.deb`。两平台内嵌固定 mihomo v1.19.32。
- 当前正确基线重新执行 TypeScript、ESLint、34文件315项 Vitest，均通过；两平台元数据／SHA256SUMS、精确生产 JS 与对应 mihomo archive 的二进制内嵌、Linux deb 版本／架构／实际 GUI 哈希及许可检查通过。
- 对同一份生产 dist 串行执行19项 Chromium隔离模拟桥接回归，覆盖代理／Key覆盖、本机及远程重启、映射重试、Agent统一更新、Skill版本挂载、日志折叠、多轮对话及推理强度，全部通过。证据保存在产物 `verification/`；不等于原生 Wails／Windows窗口或实际 SSH、安装器／apply、真实付费上游验收。
- 本轮此前执行的7个Go包测试通过，Python64项中55通过、9项Windows原生条件跳过；相关后端／契约／脚本输入与正确基线逐字节一致，复用该结果，不把旧余额前端测试计入本包验收。
- 误含旧余额的 .49 尝试已取消，仅留下一个未完成包装的Windows可执行文件和取消记录，隔离在 `dist/desktop/desktop49-attempt1-cancelled/`；不是整合安装包，未发布／安装，不复用该版本号。
- 已于北京时间 2026-10-17 23:52:30 [发布到默认 dev 更新源](https://github.com/candyflosssss/Anchor/releases/tag/v0.1.0-dev.desktop.50)；六个附件与更新清单齐全。本机运行仍为 .47；本次未安装或替换CLI／核心／窗口。安装整合包前须备份并同步升级核心与桌面：代理迁移schema8→9后旧核心不能打开数据库，SSH远端仍需独立升级。仅重启 .47 不会出现代理页面。

- 本次发布复用上述已验证安装包，未重新构建或重跑构建轮测试。发布证据在 `dist/desktop/0.1.0-dev.desktop.50/publication/`：同版flat清单、六附件、默认源真实下载、tag与非latest核验。
- 对应源码包含Anchor的Go vendor、两个固定mihomo archive，以及未修改的mihomo v1.19.32完整对应源码与Go vendor（`bundled-sources/mihomo-v1.19.32/`，上游commit `88dcbf7f1614a67c3b36b848ee3592dfa92ada36`）。原始BUILD字段保留构建时未发布／未安装事实，发布状态单独记录。

## 本地代理集成包 .48（尚未公开发布／安装）

`0.1.0-dev.desktop.47` 明确不含代理／mihomo；当前已安装的 `.47` 不会通过重启自动出现代理页面。此前代理实现停留在未提交工作区，被干净源码打包排除；现已独立提交为 `090a729b7f96b252ab9538b75d417741fec99fa7`，未混入余额缓存改动。

- 本轮从该干净提交构建 `dist/desktop/0.1.0-dev.desktop.48/` 的 Windows AMD64 NSIS 和 Ubuntu24.04 AMD64 `.deb`，`SOURCE.json` dirty=false。两平台均嵌入固定 mihomo v1.19.32；新增构建回归门禁检查生产 JS 的代理页面及桥接标识。
- 核验两平台构建元数据与 SHA256SUMS、可执行文件内的精确生产 JS 和对应 mihomo archive、Linux deb 的版本／架构／实际 GUI 哈希与许可；均通过。对**同一份生产 dist** 用隔离模拟桥接运行两项 Chromium 页面交互测试，通过实际侧栏点击进入“代理设置”，覆盖秘密保留、Key 覆盖与未知写入后显式重读。截图及日志在产物 `verification/` 中，不代表原生 Wails 或 Windows 验收。
- 仍未上传 GitHub Release、生成线上更新投放或替换本机 `.47` 的 CLI／核心／窗口，不影响当前请求。下次安装须备份并同步升级核心与桌面：schema8 将迁移到 schema9，旧核心不能打开新库；SSH 远端仍需独立升级。未执行原生 Windows、安装器／apply 或真实付费上游验收。

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
sudo apt install ./anchor-desktop_0.1.0-dev.desktop.47_amd64.deb
```

文件安装到 `/usr/bin/anchor-desktop`，应用菜单入口位于 `/usr/share/applications/anchor-desktop.desktop`；包含 SVG / PNG 图标。包的 GTK3 / WebKitGTK4.1 / libc 等依赖从实际 ELF 用 `dpkg-shlibdeps` 提取。没有安装／卸载脚本自动启服务，没有 systemd unit 或自启动项。安装后从应用菜单打开 GUI，无需手动 `init` / `serve`。没有图形会话的服务器继续使用 CLI。

### 两平台共同行为与数据保留

打开窗口后，Go 宿主安全初始化缺失状态、启动独立后台核心或核验复用已运行实例，连接就绪后进入供应商页。供应商列表、新增／编辑、删除／复制／改名、模型映射、多 Key、显式测试和用量工作台均复用核心管理能力。从 `.14` 起，点击关闭或系统关闭会隐藏到系统托盘，点击托盘图标可恢复；无可用托盘时退出宿主。从 `.15` 起当前用户只保留一个 GUI／托盘，重复启动恢复已有窗口；「退出界面」真正退出宿主，以上操作均保留后台核心；停止核心必须在界面单独确认（或用兼容 CLI `stop --approve`）。从 `.13` 起，「退出界面」「停止核心」位于顶栏「本机实例」弹层。停服请求被接受不等于恢复完成。

**手动安装或卸载前，先明确停止核心并等待退出，再退出全部窗口（`.12` 起使用「退出界面」）。** 应用内确认安装会正常停止核心并真正退出当前宿主，其余窗口需要先手动退出。 Windows 安装器／卸载器拒绝替换或删除正在使用的程序，不强杀、不安排重启强删。Linux 由包管理器替换文件，不会自动升级或停止旧运行进程；跳过停服可能留下旧核心。卸载不是停服操作。

**从旧版升级，首次启动核心将 SQLite schema2 自动迁移到3，旧密钥保留为 default Key；`.16` 再从3迁移到4，保留 Key 的协议继承设置；`.21` 从4迁移到5，新增独立的平台身份与用户授权，保留供应商和 Key。旧二进制无法打开升级后的数据库，本版不提供数据库降级。** `.25` 在此基础上继续 schema5→6，旧二进制不能打开已迁移数据库。

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

### 历史在线 dev 版 `.47`

本版于北京时间 2026-10-17 22:37:49 发布为[非 latest 的 dev prerelease](https://github.com/candyflosssss/Anchor/releases/tag/v0.1.0-dev.desktop.47)，源码 `6d3d4146b2b02000d2aa8830318791d21a713e1e`，SOURCE.json dirty=false。安装包、flat 清单、含 Go vendor 的对应源码、许可／构建元数据与 SHA256SUMS 共六附件同版投放，原 BUILD.json 保留打包时事实，实际发布及本机状态单独记录。

模型映射新增「同一渠道重试次数」，默认额外3次，可设0–10；502/503/504与取得连接前失败按共享超时预算重试，已输出流式响应或执行结果未知时不重放。另交付托盘完整退出、本机安全重启及离线恢复。 设置路径为「模型映射 → 模型设置 → 重试、上下文与能力」；0关闭同渠道额外重试但仍可换 Key／候选，3表示含首次最多4次尝试。CLI复用 `model settings` 的 `channel_retries`，省略／null保留既有值；保存只影响后续请求，活动请求保留原快照。401/403/429直接换 Key／候选，额外尝试可能增加等待或上游计费，5xx不保证上游未执行。

产物：`dist/desktop/0.1.0-dev.desktop.47/`；生产契约比对、TypeScript／Vite、Windows 与 Ubuntu 24.04 AMD64 编译、NSIS／deb 打包通过。Linux deb 版本 `0.1.0~dev.desktop.47`、架构 amd64、GUI payload 哈希与本地 ELF 一致。复用功能提交已通过的7包Go测试、31项前端单测、4项模拟桌面浏览器用例及typecheck／定向lint，不重复全量／race或原生窗口验收。

| 平台 | 安装包 | 字节数 | SHA256 |
| --- | --- | ---: | --- |
| windows-amd64 | `Anchor-Desktop-0.1.0-dev.desktop.47-windows-amd64-setup.exe` | 8,374,370 | `ab81433de79469c1fbca24304c842e64bbba3f46c6da41a7e51e047e5bec4cc4` |
| linux-amd64 | `anchor-desktop_0.1.0-dev.desktop.47_amd64.deb` | 6,172,856 | `0fe277880651638f1b622e6608bd662889d1e4efd8193749eaca4250f3b32677` |

六附件大小／SHA256 digest、对应提交源码逐文件内容／构建输入摘要、Go vendor、元数据 ZIP CRC 和 tag revision 核验通过。隔离源码 CLI 从默认 GitHub dev 源以 `.46` 为当前版发现并真实下载 `.47` Linux 包，大小／SHA256 校验通过；Windows 远端附件 digest 与本地一致。安装器未签名，未执行原生窗口、系统安装器／apply 或真实付费上游验收。

**本机更新**：本机 CLI／核心／GUI 已于2026-10-17 22:39统一升级为 `.47`；备份 `/home/mosaki/.config/anchor/backups/pre-desktop47-20261017-223946`（目录0700、文件0600），正常保留交接后恢复原有受限 transient user units，无新增持久服务或自启动。core／gateway／agent_management ready，运行 ELF 与本版构建哈希一致。实例身份、配置表、绑定凭据、托管文件及其缺失状态和原始恢复基线保持，Codex／DSH 均维持 applied；不借安装重新接管。此为用户目录升级，不是 apt／NSIS／应用内 apply 或原生窗口验收，SSH 远端未更新。

发布初次创建草稿后，GitHub 列表尚未立即可见，预检安全停止，未上传或公开；核对同版空草稿的 target revision 后复用并完成六附件校验与发布，没有覆盖旧附件或移动标签。

<a id="当前在线-dev-版-46"></a>

### 历史在线 dev 版 `.46`

本版于北京时间 2026-10-17 01:04:25 发布为非 latest 的 dev prerelease，源码 `0d9f8479cba2c7c6ca0b58af1c3c9b313154d79f`，SOURCE.json dirty=false。 安装包、flat 清单、含 Go vendor 的对应源码、许可／构建元数据与 SHA256SUMS 共六附件同版投放，原构建元数据保留打包时事实。

产物：`dist/desktop/0.1.0-dev.desktop.46/`；生产契约比对、TypeScript／Vite、Windows 与 Ubuntu 24.04 AMD64 编译、NSIS／deb 打包通过。Linux deb 版本 `0.1.0~dev.desktop.46`、架构 amd64、GUI payload 哈希与本地 ELF 一致。复用功能提交的四包 Go 测试、typecheck、16项 Agent Model 单测、定向 lint 与22项单 worker 桥接测试，本轮不重复全量／race／浏览器测试。

| 平台 | 安装包 | 字节数 | SHA256 |
| --- | --- | ---: | --- |
| windows-amd64 | `Anchor-Desktop-0.1.0-dev.desktop.46-windows-amd64-setup.exe` | 8,360,723 | `488623f18b799c981571f111564adc15a3f0981689368f6986f42ef3c4e3b255` |
| linux-amd64 | `anchor-desktop_0.1.0-dev.desktop.46_amd64.deb` | 6,161,752 | `bd49bd265bb438b9ca4eb0b0d075f5e063a402a95704981ec0100421fe7a752f` |

六附件大小／SHA256 digest、对应提交源码逐文件内容／构建输入摘要、Go vendor、元数据 ZIP CRC 和 tag revision 核验通过。隔离源码 CLI 从默认 GitHub dev 源以 `.45` 为当前版发现并真实下载 `.46` Linux 包，大小／SHA256 校验通过；Windows 远端附件 digest 与本地一致。安装器未签名，未执行原生窗口、系统安装器／apply 或真实付费上游验收。

**本机更新**：本机 CLI／核心／GUI 已于2026-10-17 01:06统一升级为 `.46`；备份 `/home/mosaki/.config/anchor/backups/pre-desktop46-20261017-010601`（目录0700、文件0600），正常保留交接后恢复原有受限 transient user units，无新增持久服务或自启动。core／gateway ready，运行 ELF 与本版构建哈希一致。实例身份、配置表、绑定凭据、托管文件及其缺失状态和原始恢复基线保持；Codex conflict／DSH applied 与升级前一致，不借安装自动重新接管。此为用户目录升级，不是 apt／NSIS／应用内 apply 或原生窗口验收，SSH 远端未更新。 首次本机备份预检发现 Codex 生成目录已被外部删除，在停服前安全退出；改为保留该缺失状态后完成更新，没有补写或接管。

<a id="local-desktop45"></a>

<a id="当前在线-dev-版-45"></a>

### 历史在线 dev 版 `.45`

`0.1.0-dev.desktop.45` 从干净隔离源码 `78d06f0cc5a89bccb1c490d3ac899fe190e8f291` 构建（SOURCE.json dirty=false），先于2026-10-16 19:09完成本地打包／本机升级，再于北京时间 2026-10-16 20:03:09 将同一版本发布为 [dev prerelease](https://github.com/candyflosssss/Anchor/releases/tag/v0.1.0-dev.desktop.45)（非 latest）。六附件已同版投放，tag 指向实际构建提交；本次不重新构建或重启本机，SSH 远端不在范围。

日志趋势只跨无请求空桶以淡虚线连接真实采样，不补零／插值，不连接有请求但指标未知的桶。已认证 Codex Binding 可读取原生线程 UUID，并按该线程只读关联同机 Codex 项目元数据；显式 Anchor 标签优先，不保存完整路径、会话标题或正文。旧日志不回填，缺失／跨机元数据不猜项目；候选、分类、筛选和导出复用现有日志接口。源码及包不包含工作区未提交的余额缓存／刷新增量。

产物位于 `dist/desktop/0.1.0-dev.desktop.45/`，含两平台安装包、SOURCE／BUILD 元数据与平台 SHA256SUMS；本地升级另保存 `local-install/BUILD-local.json`、`verification-local.json` 和 `verification-final.json`。已补齐 publication 六附件：同版两平台安装包、flat 更新清单、含 Go vendor 的对应源码、许可／构建元数据 ZIP 和 SHA256SUMS；构建元数据中的未安装／未发布／未原生验收状态保留构建时事实，实际发布与下载验收另存 publication/，不事后改写 BUILD.json。

| 平台 | 安装包 | 字节数 | SHA256 |
| --- | --- | ---: | --- |
| windows-amd64 | `Anchor-Desktop-0.1.0-dev.desktop.45-windows-amd64-setup.exe` | 8,293,546 | `090e11d04fec70f504db1afa94b1b5fc3fc343dbc1f0ec0d928f9e74b2ffea95` |
| linux-amd64 | `anchor-desktop_0.1.0-dev.desktop.45_amd64.deb` | 6,093,764 | `60c8f2ca447216eb094ae9fbb16d50261e346649539ebf91555a1dd294ad5d17` |

生产构建的 Go／TypeScript 契约比对、TypeScript／Vite、Windows／Ubuntu 24.04 AMD64 编译及 NSIS／deb 打包通过；两平台 SHA256SUMS、源码元数据与 deb 版本／架构／GUI payload 核验通过。首次打包因隔离源码缺少 npm 依赖在 preflight 退出，未产出包；受限执行 `npm ci` 后从同一干净提交重跑成功。另从同版锁定源码离线受限编译本机 CLI。复用功能提交的六包 Go 回归、49项日志前端单测、typecheck／定向 ESLint、20项单 worker 日志浏览器测试及隔离 Codex 0.162.1 对本地模拟 SSE 上游的原生元数据验证；本轮不重复全量、race或浏览器测试。

**在线投放补记（2026-10-16 20:03:09）**：本次复用原安装包，仅生成发布附件并上传公开；未改写安装包或移动旧标签。六附件大小／SHA256 digest、对应源码逐文件内容与构建输入摘要、Go vendor、元数据 ZIP CRC 和 tag revision 核验通过；默认 GitHub dev 源从 `.44` 发现并真实下载 `.45` Linux 安装包，大小／SHA256 校验通过，Windows GitHub 附件 digest 与本地一致。未执行系统安装器／应用内 apply、Linux／Windows 原生窗口或真实付费上游验收；安装器未签名。 GitHub latest 保持不变。首次按 tag 查询新草稿得到404，已按唯一 release ID确认并续传同一草稿，无重复创建或覆盖资产。隔离验证不修改用户更新配置，不调用 apply／安装器，不请求模型上游。

**本机升级（2026-10-16 19:09）**：备份 `/home/mosaki/.config/anchor/backups/pre-desktop45-20261016-190847`（目录0700、文件0600）；正常 `stop --approve --for-restart` 收到 bindings_preserved 确认，等待旧核心与监听面退出后，原子替换用户目录 CLI／GUI，恢复同名受限 transient user units（Restart=no，无新增持久 unit 或登录自启动）。观察期未获得空闲窗口，仍按正常停服流程最多10秒排空，不强杀；升级有停机窗口，不承诺既有请求零中断或会话自动重连。

当前本机安装：2026-10-16 19:09，CLI／核心／GUI 由 `.44` 统一更新为 `.45`。先备份后正常保留交接，恢复原有受限 transient user units；core／gateway ready、桌面进程运行，运行 ELF 哈希与同版构建一致。原有 Codex 模型覆盖（overridden，Agent 汇总 partial）与 DSH applied 保持，配置、凭据、实例身份和原始恢复基线保持。六档日志统计、四维分类及未识别过滤、请求十一字段与系统候选、真实系统日志 JSONL 导出通过。此为用户目录安装，不是系统安装器／应用内 apply 或原生窗口验收；同版 `.45` 已投放 dev 在线更新，SSH 远端未更新。 state.db 保持 schema8，quick_check／foreign_key_check 通过；本地 `/v1/models` HTTP 200、受管目录与 token helper 校验通过，系统日志 JSONL 共57条仅内存解析、不保存正文。19:38 只读复核 CLI／核心版本、两 unit、运行 ELF 与备份权限通过，原有管理状态保持。未执行系统安装器、应用内 apply、Windows／Linux 原生窗口交互、既有 Codex 会话重载或真实付费上游推理验收；原生归属验证复用隔离测试，不冒充真实生产新请求验收。

<a id="当前在线-dev-版-44"></a>

### 历史 dev 版 `.44`

`0.1.0-dev.desktop.44` 从干净隔离 worktree 的 `08daad1c618bebc3b6a7fbc08bc5c19039888a53` 构建，于北京时间 2026-10-16 18:24:42 发布为 [dev prerelease](https://github.com/candyflosssss/Anchor/releases/tag/v0.1.0-dev.desktop.44)（非 latest）。tag 指向实际构建提交，SOURCE.json dirty=false，不指向后续发行文档提交。

日志支持按 Agent 类型、Agent 绑定、项目、会话分类汇总；四维精确筛选、真实历史候选、分类下钻联动列表／多尺度趋势／JSONL 导出，详情与复制显示归属。默认不分类，按请求量展示前100组并标明总组数，总体统计仍覆盖完整范围。Agent／绑定仅从已认证托管 Binding 自动归属；**项目／会话尚未自动识别**，需调用端显式提供 `X-Anchor-Project`／`X-Anchor-Session` 安全标识，未上报与旧日志保持“未识别”，不猜测或回填。

保留既有日志快速候选、图表悬停时间点、用量／TPS／错误率／缓存命中率多尺度曲线与按条件导出。四维只读 CLI 过滤和 `logs stats --group-by` 由兼容源码 CLI 提供，本机同步安装该版 CLI；独立公开 CLI 仍为 alpha.6，桌面程序不提供通用 CLI。宿主与核心须一起更新，SSH 远端核心不会随本机更新自动升级。

产物位于 `dist/desktop/0.1.0-dev.desktop.44/`；publication 六附件包括两平台安装包、flat 更新清单、含 vendor 的对应源码、许可／构建元数据 ZIP 与 SHA256SUMS。

| 平台 | 安装包 | 字节数 | SHA256 |
| --- | --- | ---: | --- |
| windows-amd64 | `Anchor-Desktop-0.1.0-dev.desktop.44-windows-amd64-setup.exe` | 8,287,474 | `9c7b3a6d77045981d0a8c2f797df0bb8899ec70c3956a961e00b754d7b35286a` |
| linux-amd64 | `anchor-desktop_0.1.0-dev.desktop.44_amd64.deb` | 6,091,992 | `c0777b1ceb8aebb155b2687e864ad95e779ad59e2596bef3523baeebe46b859d` |

Go／TypeScript 契约比对、TypeScript／Vite 与双平台生产编译／打包通过；六附件大小／SHA256 digest、vendored 源码、元数据 ZIP CRC 与 tag revision 校验通过；默认源从 `.43` 发现并真实下载 Linux 安装包，大小／SHA256 校验通过，Windows 附件 digest 与本地一致。 复用该功能提交已通过的 logbook／core／desktop／cli／client 五包回归、TypeScript、ESLint、48项前端单测和19项单 worker 日志浏览器测试，本轮不重复全量、race或浏览器测试。 首次构建在许可收集阶段因容器不可见的依赖软链接失败，未产出安装包或上传；改为复制真实依赖后从同一干净提交重新打包成功。保留原 BUILD.json 的未原生验收／构建时未发布状态，实际投放与本机验证另存 `publication/`、`local-install/`。

**本机升级记录（2026-10-16 18:26）**：从同版 vendored 源码离线受限构建 CLI，GUI ELF 与发布 deb 内容逐字节一致。备份位于 `/home/mosaki/.config/anchor/backups/pre-desktop44-20261016-182553`，目录0700、文件0600；以 `stop --approve --for-restart` 正常保留交接后原子替换用户目录程序，恢复原有受限 transient user units，不新增持久服务或登录自启动。升级前观察到空闲窗口，再执行认证保留交接。

该次本机升级：2026-10-16 18:26，CLI／核心／GUI 由 `.43` 统一更新为 `.44`。先备份后正常保留交接，恢复受限 transient user units；core／gateway ready、桌面进程运行。原有 Codex 模型覆盖（overridden，Agent 汇总 partial）与 DSH applied 保持，配置、凭据、实例身份和原始恢复基线保持。六档日志统计、四维分类及未识别过滤、请求十一字段与系统候选、真实系统日志 JSONL 导出通过。此为用户目录安装，不是系统安装器／应用内 apply 或原生窗口验收。 state.db schema8、quick_check／foreign_key_check 通过；本地模型目录 HTTP 200，与受管目录一致；系统日志导出逐行 JSON 解析通过，共 58 条，仅保存计数不落盘正文。保留原始用户模型目录、恢复基线与用户模型覆盖，没有重新接管或请求付费推理。未执行系统安装器／应用内 apply、Linux／Windows 原生窗口或真实付费上游验收；安装器未签名，下载成功不等于原生安装成功。 未验收既有 Codex 会话自动重载。

<a id="当前在线-dev-版-43"></a>

### 历史 dev 版 `.43`

`0.1.0-dev.desktop.43` 从干净隔离 worktree 的 `ff229b6e2786f128b4d9542b006a59a9f74a5ec8` 构建，于北京时间 2026-10-16 17:41:46 发布为 [dev prerelease](https://github.com/candyflosssss/Anchor/releases/tag/v0.1.0-dev.desktop.43)（非 latest）。tag 指向实际构建提交，SOURCE.json dirty=false，不指向后续发行文档提交。

日志搜索／供应商／模型／Key／HTTP／错误码／阶段提供基于当前时间范围保留历史的快速候选及出现次数；修复趋势图悬停时间点，保留用量、TPS、错误率、缓存命中率等多尺度曲线；新增 Agent 托管失败安全诊断，并包含网关 32 MiB 请求体修复。已有「导出」可按当前时间及筛选条件保存请求／系统日志 JSONL，包含新诊断事件，不限当前页。

候选查询覆盖全部匹配时间范围而非当前分页，不受其他字段过滤限制；默认20项，按次数降序／值升序，支持手动输入、键盘选择和中文组合输入。对应只读 CLI `logs suggestions` 与 Wails 桥接同版交付；公开独立 CLI 仍为 alpha.6，桌面程序不提供通用 CLI。宿主与核心需一起更新；SSH 远端核心不随本机更新自动升级。Agent 诊断只记录阶段、原因等白名单字段，不记录配置内容或秘密，不修改现有 API 错误码或配置保护策略。

产物位于 `dist/desktop/0.1.0-dev.desktop.43/`；publication 六附件包括两平台安装包、flat 更新清单、含 vendor 对应源码、许可／构建元数据 ZIP 与 SHA256SUMS。

| 平台 | 安装包 | 字节数 | SHA256 |
| --- | --- | ---: | --- |
| windows-amd64 | `Anchor-Desktop-0.1.0-dev.desktop.43-windows-amd64-setup.exe` | 8,281,237 | `fd7c22df6b643a522b16bdcd544f093379ccd169e7b19529c3ae40e59b7dd8ad` |
| linux-amd64 | `anchor-desktop_0.1.0-dev.desktop.43_amd64.deb` | 6,086,664 | `b6c8d799e60eab81cf3c86d12f06bbf902c6d1345da2add3f97b68e54f7ca8cf` |

Go／TypeScript 契约比对、TypeScript／Vite 与双平台生产编译／打包通过；六附件大小／SHA256 digest、vendored 源码、元数据 ZIP CRC 与 tag revision 校验通过；默认源从 `.42` 发现并真实下载 Linux 安装包，大小／SHA256 校验通过，Windows 附件 digest 与本地一致。复用本轮功能已通过的 Go 定向回归、managed／logbook／core 三包完整回归、TypeScript、ESLint、63项前端单测和18项单 worker 日志浏览器测试。构建时保留原 BUILD.json 的未原生验收／未发布状态，发布与实际安装验证分别另存 `publication/`、`local-install/`。

**本机升级记录（2026-10-16 17:43）**：从同版 vendored 源码离线受限构建 CLI，GUI ELF 与发布 deb 内容逐字节一致。备份位于 `/home/mosaki/.config/anchor/backups/pre-desktop43-20261016-174222`，目录0700、文件0600；以 `stop --approve --for-restart` 正常保留交接后原子替换用户目录程序，恢复原有受限 transient user units，不新增持久服务或登录自启动。30秒观察未捕获空闲窗口，随后按授权执行正常10秒排空停服，未强杀进程；不承诺更新期间请求无中断。

当前本机安装：2026-10-16 17:43，经用户明确要求，CLI／核心由 `.42.gateway32.1`、GUI 由 `.42` 统一更新为 `.43`。先备份后正常保留交接，恢复受限 transient user units；core／gateway ready、桌面进程运行。原有 Codex 模型覆盖（overridden，Agent 汇总 partial）与 DSH applied 保持，配置、凭据、实例身份和原始恢复基线保持。六档日志统计、请求七字段及系统事件候选、真实系统日志 JSONL 导出通过。此为用户目录安装，不是系统安装器／应用内 apply 或原生窗口验收。state.db schema8、quick_check／foreign_key_check 通过，本地模型目录 HTTP 200；系统日志导出逐行 JSON 解析通过，共 52 条，验证只保存计数而不落盘日志正文。保留原始用户模型目录与恢复基线，没有强制重新托管或请求付费上游。未执行系统安装器／应用内 apply、Linux／Windows 原生窗口或真实付费上游验收；安装器未签名，下载成功不等于原生安装成功。未验收既有 Codex 会话自动重载。

<a id="当前在线-dev-版-42"></a>

### 历史 dev 版 `.42`

`0.1.0-dev.desktop.42` 从干净隔离 worktree 的 `43f2c41d378c4d4c38f382a5512e64a8f1c417e8` 构建，于北京时间 2026-10-16 16:35:25 发布为 [dev prerelease](https://github.com/candyflosssss/Anchor/releases/tag/v0.1.0-dev.desktop.42)（非 latest）。tag 指向实际构建提交，不指向后续发行文档提交；SOURCE.json dirty=false。

请求日志页新增请求量、Token 用量、总 TPS、错误率、缓存命中率、平均耗时与首字延迟趋势，复用全部匹配保留历史而非当前分页；范围15分钟至90天，自动或10秒至1天粒度、最多600桶，支持15秒刷新／暂停／手动刷新、鼠标和键盘查点、浅／深主题与窄屏。未知观测保持 null／断线，已知0保留0；沿用 schema8，无新增数据库迁移或上游调用。

TPS 按成功请求输出 Token／总耗时加权，错误率分母排除 pending／cancelled，缓存命中率只纳入单次尝试且输入／缓存已知的有效样本、按 Token 加权。图表复用既有筛选但不受列表分页影响；保留样本数和未知语义，统计受日志保留与 health 丢弃／写入失败限制，不是永久消费账本。同步交付只读 stats 管理接口、Wails 桥接与源码 CLI `logs stats`。宿主与核心须一起升级；独立公开 CLI 仍为 alpha.6，已安装桌面程序不提供通用 CLI。

另包含此前已提交的界面规范复核改进；工作区尚未提交的余额缓存／工作台改动未包含在包中。

输出 `dist/desktop/0.1.0-dev.desktop.42/`；publication 六附件包括 Windows NSIS、Ubuntu24.04 AMD64 deb、flat 清单、含 vendor 对应源码、许可／构建元数据 ZIP 与 SHA256SUMS。

| 平台 | 安装包 | 字节数 | SHA256 |
| --- | --- | ---: | --- |
| linux-amd64 | `anchor-desktop_0.1.0-dev.desktop.42_amd64.deb` | 6,074,256 | `43fb15c9d611962f33bbe2cb0da619c2011ceb5f837e71a629d8bf41221c0a92` |
| windows-amd64 | `Anchor-Desktop-0.1.0-dev.desktop.42-windows-amd64-setup.exe` | 8,273,322 | `5a192975047cc51af1028848b59f92946da678150e6a88c403fcccaf84ed518c` |

Go／TypeScript 契约比对、TypeScript／Vite 生产编译、双平台生产编译／打包通过；六附件大小／SHA256 digest、vendored 源码、元数据 ZIP CRC 与 tag revision 校验通过；默认源从 `.41` 发现并真实下载 Linux 安装包，大小／SHA256 校验通过，Windows 附件 digest 与本地一致。复用功能提交已通过的 logbook／core／desktop／cli／client 五包 Go 测试、TypeScript、275项完整前端单测、ESLint 和11项单 worker 日志 Playwright；本轮不重复全量或浏览器测试。

打包发布轮次未自动安装或替换正在运行的程序；未执行系统安装器／应用内 apply、原生 GUI／Windows 安装或真实付费上游验收。安装器未签名，下载成功不等于安装成功。 未改真实 Agent 配置。BUILD.json 保留构建时 public_release=false 与未原生安装验收的原始事实；发布与默认源下载证据另存 publication。

**本机升级记录（2026-10-16 16:46）**：按用户明确要求，复用已校验的 `.42` Linux ELF（与发布 deb 内容一致），从同版 vendored 源码离线受限构建 CLI，将 `~/.local/bin/anchor` 与 `anchor-desktop` 从 `.40` 原子替换为 `.42`。先备份数据库、程序、凭据和恢复材料，关闭旧 GUI，再通过 `stop --approve --for-restart` 保留交接；正常退出后恢复受限 transient user units，不新增持久服务或开机自启动。备份位于 `~/.config/anchor/backups/pre-desktop42-20261016-164634/`，目录0700、文件0600。

实际运行 CLI／核心版本与两进程 ELF 哈希匹配 `.42`，core／gateway ready；Codex 原有模型覆盖保留、DSH 仍 applied，Agent 汇总维持升级前的 partial，未强行改写为正常托管。配置表、settings、实例身份、凭据、用户目录与原始恢复基线保持；state.db schema8、数据库 quick_check／foreign_key_check、备份完整性和权限通过。真实只读日志统计在15分钟、1小时、24小时、7天、30天、90天及自动粒度通过；本地模型目录返回200，未请求付费上游。安装与发布证据分开保存于 `local-install/` 和 `publication/`；没有重新发布线上版本或改写原 BUILD.json。未执行系统安装器、应用内 apply、原生窗口可见性或现有 Codex 会话自动重载验收。

### 历史 dev 版 `.41`

`0.1.0-dev.desktop.41` 从干净隔离 worktree 的 `bf3097f8ffd2742c09416a00572d41ba0fbd9484` 构建，于北京时间 2026-10-16 11:39:20 发布为 [dev prerelease](https://github.com/candyflosssss/Anchor/releases/tag/v0.1.0-dev.desktop.41)（非 latest）。tag 指向实际构建提交，不指向后续发行文档提交；SOURCE.json dirty=false。

日志安全保留入站、实际发送与上游报告的原始 service_tier 标量，只标注可解析值；新增“发送档位”“加速状态”，Fast 用青色闪电、Ultrafast 用紫色火箭。加速确认仅依据上游报告，Ultrafast → Fast 显示“仅确认 Fast”，未知／未报告不误判；不保证实际速度或计费。默认／性能布局展示两块，既有自定义布局保持；无数据库迁移、无历史回填、不改转发策略。

原始值受长度、类型与凭据过滤限制，不存任意请求／响应内容。`fast_triggered` 为字符串 true／false／unknown，表示上游报告而非测速结果。列表、详情、复制与 CLI 日志证据共用解析；JSON／SSE、探测和重试候选均覆盖。旧自定义布局可在日志字段设置中添加新显示块。

输出 `dist/desktop/0.1.0-dev.desktop.41/`；publication 六附件包括 Windows NSIS、Ubuntu24.04 AMD64 deb、flat 清单、含 vendor 源码、许可／构建元数据 ZIP 与 SHA256SUMS。

| 平台 | 安装包 | 字节数 | SHA256 |
| --- | --- | ---: | --- |
| linux-amd64 | `anchor-desktop_0.1.0-dev.desktop.41_amd64.deb` | 6,062,508 | `6d552517f83bd3f0ec7276e786bc6199bb2fccc26323b8fd3735a108d3aafafb` |
| windows-amd64 | `Anchor-Desktop-0.1.0-dev.desktop.41-windows-amd64-setup.exe` | 8,262,139 | `8186b56e02455d3aee9ad673b76a0fd39d588426a6dbbd2dc721925948a91b98` |

Go／TypeScript 契约比对、TypeScript／Vite 生产编译、双平台编译和打包通过；六附件大小／SHA256 digest、vendored 源码、元数据 ZIP CRC、tag revision 校验通过；默认源从 `.40` 发现并真实下载 Linux 安装包，大小／SHA256 校验通过，Windows 附件 digest 与本地一致。复用功能提交已通过的 protocol／core／desktop／cli 四包 Go 测试、TypeScript／定向 ESLint、37 项日志单测与 9 项单 worker Playwright；本轮不重复全量或浏览器测试。

本轮未自动安装或替换正在运行的程序；未执行系统安装器／应用内 apply、原生 GUI／Windows 安装或真实付费上游验收。安装器未签名，下载成功不等于安装成功。 未改真实 Agent 配置；独立公开 CLI 仍为 alpha.6，读取新增日志证据须兼容源码 CLI／新版核心。BUILD.json 保留构建时 public_release=false 与未原生安装验收的原始事实，发布与下载证据另存 publication。工作区未提交的余额缓存／工作台改动未包含在包中。

### 历史 dev 版 `.40`

`0.1.0-dev.desktop.40` 从干净隔离 worktree 的 `2da6c20910b54ece98be4f7d713045f711e15d6b` 构建，于北京时间 2026-10-16 01:06:46 发布为 [dev prerelease](https://github.com/candyflosssss/Anchor/releases/tag/v0.1.0-dev.desktop.40)（非 latest）。tag 指向实际构建提交，不指向后续发行文档提交。

供应商模型映射新增多选／全选与批量测试、编辑、删除；串行执行、显式确认、固定 revision、保留逐项结果与部分完成状态，不自动重试。另包含已有模型默认值补全、上下文分档计费提示、DSH Desktop 独立托管与模型能力同步，以及托管失败分类反馈。

在「供应商 → 选择供应商 → 模型 → 模型映射」勾选条目：批量测试提示用量并按已保存上游与 Key 绑定执行，无可用 Key 跳过；批量编辑未修改字段保持各自原值，并提示共享模型属性影响；批量删除只移除当前供应商映射，核心保留启用 Binding 的引用保护。停止在当前请求完成后生效，批次不是事务，管理／传输错误后保留部分结果并要求重读。

输出 `dist/desktop/0.1.0-dev.desktop.40/`，publication 六附件包括 Windows NSIS、Ubuntu24.04 AMD64 deb、flat 清单、含 vendor 源码、许可／构建元数据 ZIP 与校验清单。Go／TypeScript 契约比对、TypeScript／Vite 生产编译、双平台编译和打包通过；六附件大小／SHA256 digest、vendored 源码、元数据 ZIP CRC、tag revision 校验通过；默认源从 `.39` 发现并真实下载 Linux 安装包，大小／SHA256 校验通过，Windows 附件 digest 与本地一致。

复用功能提交已通过的 TypeScript、定向 ESLint、29 项 Vitest 与 9 项单 worker Playwright；本轮打包不重复运行全量或原生测试。BUILD.json 中 `public_release=false` 是构建时的原始状态，后续发布另行记录。上述打包发布轮次未自动安装或替换正在运行的程序；未执行系统安装器／应用内 apply、原生 GUI／Windows 安装或真实付费上游验收。安装器未签名，下载成功不等于安装成功。 `.39` 的既有本机安装／原生 Codex 名单验收保留为下方历史事实；`.40` 的后续本机升级另记如下。


**后续本机升级（2026-10-16 01:18，用户目录安装）**：应用户要求，将 `/home/mosaki/.local/bin/anchor` 和 `/home/mosaki/.local/bin/anchor-desktop` 从 `.39` 升级至 `.40`。CLI 从本版已发布的 vendored 源码（revision `2da6c20910b54ece98be4f7d713045f711e15d6b`）受限离线编译；GUI 复用本版 Linux ELF，已核对与发布 deb 解压内容逐字节一致。没有重建桌面安装包、执行 apt 或重复公开发行，应用菜单仍指向用户目录中的桌面入口。

升级前用 SQLite backup API 保存数据库，并备份 owner token、恢复日志、受管文件和旧二进制至 `/home/mosaki/.config/anchor/backups/pre-desktop40-20261016-011752/`（目录0700／文件0600，不提交内容）。关闭旧 GUI 后，核验本机核心身份并执行 `stop --approve --for-restart`，收到保留确认且旧核心／监听退出后才原子替换二进制。交接期间受管文件与恢复日志不变；新版正常启动后配置表、settings、实例身份、owner token 和绑定凭据保持，用户自有模型目录、首次恢复基线与已有 edits.before 均保留。未走普通停服／恢复／重新托管或强杀。

CLI 和受认证核心均报告 `.40`，核心与 GUI 的运行 ELF 哈希匹配本版产物；core／gateway／agent management 均 ready，Codex／DSH 均 applied。state.db 仍为 schema8，quick_check／foreign_key_check 通过；Codex 默认模型不变，token-helper 验证通过，生成目录与本地 `GET /v1/models` 的12个模型完全一致（HTTP200）。核心／GUI 使用受内存上限约束的临时用户服务，Restart=no，未创建持久 unit 或登录自启动；安装 helper 已退出。证据保留于 `dist/desktop/0.1.0-dev.desktop.40/local-install/`，任务解压目录已清理。未执行系统安装器、应用内 apply、原生窗口可见性、Windows 安装、现有 Codex 会话自动重载或真实上游推理验收；进程运行不等于已验证窗口可见性。

### 历史 dev 版 `.39`

`0.1.0-dev.desktop.39` 从干净隔离 worktree 的 `7e4a721347ae0c53b01457114589131ba328f785` 构建，于北京时间 2026-10-15 17:39:27 发布为 [dev prerelease](https://github.com/candyflosssss/Anchor/releases/tag/v0.1.0-dev.desktop.39)（非 latest）。tag 指向构建提交，不混入未提交余额界面改动。

修复 Codex 目录被上游 `max/default/null` effort 或无指令条目整份拒绝的问题：仅投影客户端可表示的档位，无模板条目使用明示的 Anchor 通用编程助手指令；不修改已有模板、不删除模型／速度分身、不把 max 改成 xhigh。

输出 `dist/desktop/0.1.0-dev.desktop.39/`，publication 六附件包括 Windows NSIS、Ubuntu24.04 AMD64 deb、flat 清单、含 vendor 源码、许可／构建元数据 ZIP 与校验清单。六包 Go 回归通过，新增用例后 managed／CLI 再次通过；Go／TypeScript 契约比对、TypeScript／Vite 生产构建、双平台编译／打包通过。六附件大小／SHA256 digest、vendored 源码、元数据 ZIP CRC、tag revision、默认源从 `.37` 发现并真实下载 Linux 安装包通过；Windows 附件 digest 与本地一致。

**本机修复已验收**：本机 CLI／核心／GUI 已升级为 `.39`，core／gateway／agent management 均 ready，Codex／DSH 均 applied。Codex 自有目录与网关均为 12 个模型，默认仍为 `gpt-6.1-sol(fast)`；手动 `agent sync` applied，配置、目录与 journal 无变化。独立 CODEX_HOME 的原生 codex-cli 0.162.1 app-server `initialize + model/list` 已解析实际生成目录，12 个模型与网关完全一致，含普通／Fast／Ultrafast。仅本地名单 RPC，未创建线程、未发推理请求，未重启当前用户 Codex。state.db schema8、quick_check／foreign_key_check、配置行、settings、owner token 与绑定凭据均保持；用户原目录哈希、首次恢复基线和已有 edits.before 保留。受限临时核心／GUI 服务均 active，运行 ELF 哈希匹配本版产物；未设置持久服务或登录自启动。保护备份：`/home/mosaki/.config/anchor/backups/pre-desktop39-20261015-173838/`（不提交秘密内容）。沿用 restart_handoff，仍有停机窗口，不承诺现有会话自动重连。

**中间构建 `.38` 未上线**：本轮先构建并安装 `.38`，已修复 effort 导出、托管达到 ready，但隔离原生 Codex 检查发现无模板模型缺少指令导致整份目录被拒。未将该包发布；补齐明示通用指令并回归后构建、发布与安装 `.39`。保留 `.38` 产物和失败证据，不能把 `.38` 的程序替换当成完整名单修复。

未执行 Windows 原生安装、原生 GUI 窗口可见性、系统安装器／应用内 apply、真实付费上游或当前 Codex 会话自动重载验收；不保证上游加速权限、速度或费用。 当前 Codex 进程仍可能缓存旧名单，需用户完全退出并重开相应客户端／app-server；本轮不终止当前会话。


### 历史在线 dev 版 `.37`

`0.1.0-dev.desktop.37` 从干净隔离 worktree 的 `45ac60087a4a046cbcc47c46f22dae19d880fd2b` 构建，于北京时间 2026-10-15 17:00:56 发布为 [dev prerelease](https://github.com/candyflosssss/Anchor/releases/tag/v0.1.0-dev.desktop.37)（非 latest），tag 指向实际构建提交，未混入工作区未提交余额改动。

模型映射设置新增「速度与分身」入口；映射新增／修改／删除后自动同步已托管 Agent，Codex all-mapped 生成 Anchor 自有模型目录。保留原用户目录、恢复基线及凭据；不强开 Codex 原生速度菜单，不保证上游支持、权限或费用。

本地输出 `dist/desktop/0.1.0-dev.desktop.37/`，`publication/` 六附件含 Windows NSIS、Ubuntu24.04 AMD64 deb、flat 更新清单、对应源码（含 Go vendor）、许可／构建元数据 ZIP 和总校验清单。BUILD.json 保留构建时验证事实，在线投放结果另行记录。

Go／TypeScript 契约比对、TypeScript／Vite 生产构建、双平台编译／打包通过；六附件大小／SHA256 digest、vendored 源码、元数据 ZIP CRC、标签指向及默认源从 `.36` 发现／实际下载 Linux 包通过。复用功能任务六包 Go、27 项单元测试、8 项浏览器测试与类型／定向 lint 结果；本轮未重跑完整套件，未安装本机、未执行原生 Codex／Claude／DSH 加载、原生窗口／安装器／apply 或真实付费上游验收。 本次只发布，没有替换本机程序、重启真实核心或改动真实 Agent 配置；本机仍为 `.36`。

**后续本机升级（2026-10-15 17:13，部分完成）**：应用户要求，沿用用户目录安装，CLI 从本版已发布的 vendored 源码受限离线编译，GUI 使用已核验与发布 deb payload 一致的 ELF，替换 `~/.local/bin/anchor` 与 `~/.local/bin/anchor-desktop`。没有重建桌面安装包、重复线上发布或启用登录自启动。安全备份保留在仓库外 `~/.config/anchor/backups/pre-desktop37-20261015-170754/`（权限保护，不提交内容）。先退出旧 GUI，经 `stop --approve --for-restart` 收到保留确认、等待旧核心退出后替换；旧核心到达 drain deadline 后取消了剩余请求，不能声称零停机。当前核心和 GUI 的受限临时用户服务均 active，运行 ELF 哈希匹配本版程序；core／gateway ready，GUI 原生窗口可见性未验收。

**Codex 名单迁移失败，不能把程序替换视为映射问题已修复。** 在独立数据副本中只读诊断确认：`gpt-6.1-sol` 及其 Fast／Ultrafast 分身的 portable reasoning effort 包含 `max`、`default`，本版 Codex 目录导出仅接受已列出的 Codex effort，报 `reasoning effort is not supported by Codex export`，整份目录未生成。Codex 绑定为 `apply_failed`，Agent management 为 partial；原 `config.toml`、用户目录、Codex 恢复日志未变，默认模型仍为 `gpt-6.1-sol(fast)`。没有删改用户推理档位、重新绑定、手动覆盖目录或重启当前 Codex 会话；仅重启 Codex 不能解决这次导出失败。

DSH 绑定 applied，启动时更新其受管 provider 字段；核对当前配置与 journal 的 after 一致，所有首次恢复基线及已有 edits.before 保留。state.db schema8、quick_check／foreign_key_check 通过，配置表行、settings、owner token 和绑定凭据保持不变。Codex token-helper 核验通过，绑定凭据访问本机 `GET /v1/models` 返回 200／12 个映射，含 `gpt-6.1-sol(fast)`／`(ultrafast)`；这不代表 Codex 原生模型选择器已加载，也不证明上游速度档位权限或效果。未请求真实付费推理、未执行系统安装器／应用内 apply。实际部署证据位于 `dist/desktop/0.1.0-dev.desktop.37/local-install/verification-local.json`；本次本机升级按 partial 记录，目录兼容性缺陷待修复。

### 历史在线 dev 版 `.36`

`0.1.0-dev.desktop.36` 从干净隔离 worktree 的 `f7e961f405a1515f89b412051937fe9d0ecd202c` 构建，于北京时间 2026-10-15 16:03:45 发布为 [dev prerelease](https://github.com/candyflosssss/Anchor/releases/tag/v0.1.0-dev.desktop.36)（非 latest），tag 指向实际构建提交，未混入工作区未提交的余额界面改动。

本地输出 `dist/desktop/0.1.0-dev.desktop.36/`，`publication/` 六附件含两平台安装包、flat 更新清单、对应源码（含 Go vendor）、许可／构建元数据 ZIP 和总校验清单。构建元数据保留构建时未发布及未安装的原始事实，投放结果另行记录。

Fast／Ultrafast 分身与限制见[设计契约](../design/model-speed-variants.md)；安全交接沿用 `.35`，旧版首次迁移限制见页首。Linux 默认源实际下载与大小／SHA256校验通过，Windows 及其他附件 GitHub digest 与本地一致；未安装本机或重启真实核心，未进行原生窗口／安装器或真实付费上游验收。

**后续本机升级（2026-10-15 16:12，用户目录安装）**：应用户要求，CLI 与桌面程序从 `.35` 升至 `.36`。CLI 从本版已发布的 vendored 源码（revision `f7e961f405a1515f89b412051937fe9d0ecd202c`）受限离线编译，GUI 复用本版已校验的 Linux ELF；原子替换 `~/.local/bin/anchor` 和 `~/.local/bin/anchor-desktop`，没有重建安装包、执行 apt、重新投放线上或启用登录自启动。

升级前将数据库、owner token、绑定恢复日志、现有受管配置／目录和旧程序备份至仓库外权限保护目录 `~/.config/anchor/backups/pre-desktop36-20261015-161000/`。先关闭本机旧 GUI，再对已核对身份的 local 核心执行 `stop --approve --for-restart`；收到保留确认并等待旧核心退出后才替换文件。交接前后与新版启动后，受管配置、目录、owner token、原始恢复日志和绑定凭据均未变化，未走普通停服／restore／sync 或强杀。

**升级中发现既有启动状态问题**：保留交接后，两个 `all-mapped` 绑定的恢复日志仍是 `applied`，配置也未变，但启动流程对空目录版本进行元数据校验，将内存管理状态标成 `apply_failed`。核对旧日志和现有配置后，使用原始日志对应的计划 fingerprint 走已有 `agent apply` 的已应用验证分支，清除了状态缓存；未恢复或重新写入 Agent 文件。此轮未修改产品代码修复根因，后续相同重启场景可能再次提示，不应通过普通恢复／重接管掩盖问题。首次本机升级检查也曾因用户自有的相对模型目录路径而提前拒绝；当时尚未停服，核对真实路径后继续。

最终核心、网关、Agent 管理均 ready，两个绑定 applied；核心与 GUI 的 transient user services active，运行 ELF 哈希与本轮 CLI／已发布桌面产物一致。实例身份和配置计数保持，state.db 仍为 schema8，quick_check／foreign_key_check 通过；Codex token-helper 与原绑定凭据匹配，本机 `GET /v1/models` 返回 HTTP200／8模型。未自动创建速度分身、未请求真实付费推理、未验证既有 Codex 会话重载；GUI 进程运行不等于已验收原生窗口可见性。本机证据保留在 `dist/desktop/0.1.0-dev.desktop.36/local-install/`，临时 vendored 源码已清理。

### 历史 dev 版 `.35`

`0.1.0-dev.desktop.35` 从干净隔离 worktree 的 `2bb386fc5a679107f4e659efb09e23fcfc669565` 构建，于北京时间 2026-10-15 14:57:18 发布为 [dev prerelease](https://github.com/candyflosssss/Anchor/releases/tag/v0.1.0-dev.desktop.35)（非 latest），tag 指向实际构建提交，未混入余额界面等未提交改动。

本地输出 `dist/desktop/0.1.0-dev.desktop.35/` 包含两平台安装包、`SOURCE.json`、各平台 `BUILD.json`／许可通知／`SHA256SUMS`；`publication/` 的六个线上附件为 Linux `.deb`、Windows NSIS 安装器、`update-desktop.json`、含 Go vendor 的对应源码、许可／构建元数据 ZIP 和总校验清单。构建元数据保留未安装、未原生验收与构建时未发布的原始事实，线上发布／下载结果另行保存。

修复与首次迁移限制见页首及[安全重启交接](../design/desktop-runtime.md#2026-10-15-安全重启交接修复desktop35-已发布)。默认源 Linux 实际下载与大小／SHA256校验通过，Windows 及其他附件 GitHub digest 与本地一致；该发布轮次未执行本机安装或重启真实服务。

**后续本机升级（2026-10-15 15:06，用户目录安装）**：应用户要求，从同一已发布的 vendored 源码（revision `2bb386fc5a679107f4e659efb09e23fcfc669565`）受资源限制编译 CLI，并将 `~/.local/bin/anchor` 和 `~/.local/bin/anchor-desktop` 从 `.34` 原子替换为 `.35`；桌面 ELF 直接复用已校验的发布产物，没有重建安装包、执行 apt 或再次线上投放。升级前核心与 GUI 均未运行，旧用户服务单元不存在；本轮创建分离且限内存的 transient user units，`Restart=no`，没有启用登录自启动，也未改动其他代理服务。

升级前数据库、owner token、恢复数据、受管 Agent 配置及旧二进制已备份至仓库外受保护目录 `~/.config/anchor/backups/pre-desktop35-20261015-150633/`。启动后验证运行核心与 CLI 哈希一致、GUI 进程与发布 ELF 哈希一致、两个服务 active；核心／网关／Agent 管理 ready，两个绑定 applied，`restart_handoff` 能力存在。state.db 仍为 schema8，quick_check／foreign_key_check 通过，实例身份及供应商／Key／模型／绑定／Context 数量保持。Codex 受管配置指向 `http://127.0.0.1:17860/v1`，新版 token-helper 与绑定凭据匹配，使用绑定凭据请求本机 `GET /v1/models` 返回 200／8 个模型，未请求付费推理。CCS 在操作前已关闭，操作后仍无相关进程且 15722 未监听，无需终止进程。

本轮证据在 `dist/desktop/0.1.0-dev.desktop.35/local-install/`，不含秘密。GUI 进程已启动，但未验证 Wayland 原生窗口可见性；未执行安装器／应用内 apply／浏览器测试／真实上游推理／现有 Codex 会话重载验收，不能将配置接管或本机模型列表通过视为旧会话已经重连。

### 历史 dev 版 `.34`

`0.1.0-dev.desktop.34` 于北京时间 2026-10-15 13:52 从干净隔离 worktree 构建，revision 为 `cae11dc63ed42c56e4b1c134fa8469b6fea0c496`，`SOURCE.json` 的 dirty=false；没有混入原工作区未提交的余额界面／缓存改动。本轮复用这批安装包，于北京时间 2026-10-15 14:02:58 发布为 [dev prerelease](https://github.com/candyflosssss/Anchor/releases/tag/v0.1.0-dev.desktop.34)（非 latest），标签指向上述实际构建提交，未重新编译。

本包包含 `.33` 之后已提交的顶栏设备快速切换、日志字段分组与首文本／工具输出计时、非推理输出 TPS、供应商倍率 × Key 缓存倍率成本路由、模型映射默认配置及默认流式、请求与上游返回服务档位展示。服务档位日志用于观察请求和响应，并不代表修复了 Codex 的 Fast 入口显示或确认上游提供加速。

本地输出目录为 `dist/desktop/0.1.0-dev.desktop.34/`：

| 平台 | 安装包 | 字节数 | SHA256 |
| --- | --- | ---: | --- |
| Windows AMD64 | `windows-amd64/Anchor-Desktop-0.1.0-dev.desktop.34-windows-amd64-setup.exe` | 8223883 | `0ecec0319b1e3462216f34704fb8545ecf2c9bb5c6b05734d3a4538a0fafeab0` |
| Ubuntu 24.04 AMD64 | `linux-amd64/anchor-desktop_0.1.0-dev.desktop.34_amd64.deb` | 6027592 | `ed1ac02faa1b433301e94c9638554182389479e29fb0dfeb9f43a05ba0347060` |

同目录保留 `SOURCE.json`、容器环境记录、两平台 `BUILD.json`／`SHA256SUMS`、许可证与第三方通知。构建使用 Go 1.26.0、Node 22.22.1、npm 9.2.0 和记录 digest 的 Ubuntu 24.04 容器；通过现有受资源限制的 `scripts/package-desktop.sh --version 0.1.0-dev.desktop.34` 串行完成。

实际验证：Go／TypeScript 契约比对、TypeScript／Vite desktop 生产构建、双平台编译及 `.deb`／NSIS 打包通过；两平台 `SHA256SUMS` 共 13 个文件与 `BUILD.json` 哈希交叉核对通过。Debian 控制版本按脚本映射为 `0.1.0~dev.desktop.34`，包名、amd64 架构与可执行文件条目核对通过；Linux ELF／Windows AMD64 GUI 二进制及 NSIS 安装器类型符合预期。Vite 保留大于 500 kB 的 chunk 提示，不是构建失败。

本版核心会将 state.db 从旧 schema 迁移到 schema8；需先备份，旧核心不能打开已升级数据库，不支持数据库降级。未操作真实数据或 Agent 配置，未运行测试套件、浏览器、原生 GUI、安装器或 apply 验收。原始 `BUILD.json` 的 `public_release`／`native_gui_verified`／`installation_verified`／`signed` 均保留构建时的 false，不用发布结果改写构建事实。

六附件已完整投放：两平台安装包、`update-desktop.json`、`anchor_0.1.0-dev.desktop.34_source.tar.gz`（含 vendored Go 依赖）、`anchor_0.1.0-dev.desktop.34_metadata.zip`、`SHA256SUMS`。草稿附件上传状态、大小与 SHA256 digest 通过后再公开发布；公开后的正式附件 URL 与 flat 清单一致，标签和源码归档对应实际构建 revision。归档内 go.mod／go.sum、前端锁文件和生成 TS 与该提交一致，元数据 ZIP CRC 通过。发布证据保留在 `dist/desktop/0.1.0-dev.desktop.34/publication/`。

在隔离临时数据目录从该版源码执行 `update download --channel dev --product desktop --current-version 0.1.0-dev.desktop.33 --json`，默认 GitHub 源发现 `.34` 并实际下载 Linux 安装包，6,027,592 字节及 SHA256 校验通过；Windows 附件 digest 与本地安装包一致。未调用 apply 或安装器，下载验证不等于安装验收。


**后续本机升级（2026-10-15 14:08）**：应用户要求，沿用原有用户目录安装，将 `~/.local/bin/anchor-desktop` 从 `.32` 替换为上述已发布 `.34` 的 Linux ELF；CLI 从同一发布源码归档（含 vendor，revision `cae11dc63ed42c56e4b1c134fa8469b6fea0c496`）受资源限制编译为 `.34` 并更新 `~/.local/bin/anchor`，没有混入工作区未提交代码，也未重建桌面包或改动线上 Release。正常重启现有用户 `anchor-core.service` 后，核心／网关 ready、Agent 管理 ready、两个绑定 applied；运行核心 ELF 与新 CLI 哈希一致，本机桌面 ELF 与发布包哈希一致，桌面运行库无缺失。

迁移前 state.db／logs.db、owner token、恢复数据及旧二进制已在仓库外受保护备份，目录 `~/.config/anchor/backups/pre-desktop34-20261015-140854/`；备份 state.db 为 schema7，quick_check=ok。升级后 schema8，quick_check 和 foreign_key_check 均通过，实例身份与供应商／Key／模型／绑定／Context 数量保持。当前证据位于 `dist/desktop/0.1.0-dev.desktop.34/local-install/`，不含秘密。更新前没有桌面宿主运行，因此没有退出或启动 GUI；未执行 apt 安装、应用内 apply 或原生窗口验收，不能把用户目录替换等同于系统包安装测试。

### 历史在线 dev 版 `.30`

`0.1.0-dev.desktop.30` 于北京时间 2026-10-14 14:18:14 发布为 dev prerelease（非 latest）。构建提交 `010bc0a44805995967fc0868c82d60ac816f3ea7`，干净隔离 worktree 的 SOURCE.json 为 dirty=false。

推理请求先使用配置地址，仅未带版本路径收到明确 HTML 200／404／405 时同源追加 `/v1` 重试一次，不改已保存地址；仍返回网页时测试卡片、快速测试与日志显示 `unexpected_html` 和中文 API 地址提示。包含 `.29` 后已提交的 Agent 管理简化与 DSH 发现，沿用 schema6，无新增数据库迁移。 JSON、鉴权失败、限流、网络错误、重定向与 SSE 不触发补路径；已带版本的地址不再次追加版本。HTTP 200 不能单独代表模型响应有效，受限端点证据不保存 URL、Key 或网页正文。

| 平台 | 安装包 | bytes |
| --- | --- | ---: |
| Ubuntu24.04 AMD64 | `anchor-desktop_0.1.0-dev.desktop.30_amd64.deb` | 5,681,216 |
| Windows AMD64 | `Anchor-Desktop-0.1.0-dev.desktop.30-windows-amd64-setup.exe` | 7,879,308 |

```text
50d1b7571df94c3f710c139324f2161f2319b0f19ecaf0384d13cde47a8c82c6  anchor-desktop_0.1.0-dev.desktop.30_amd64.deb
3de876fd5c14f7c11bf354a0b769ec933afca6e304ac643f842e0916de0f9819  Anchor-Desktop-0.1.0-dev.desktop.30-windows-amd64-setup.exe
```

六附件完整投放，GitHub 附件大小／digest 及正式 URL 与本地清单一致，标签指向实际构建提交。源码归档含 vendored Go 依赖，go.mod／go.sum及本轮推理端点与测试提示源码与构建提交一致；元数据 ZIP CRC 通过，原始 BUILD.json 的原生／安装／签名状态保留。发布及下载证据位于 `dist/desktop/0.1.0-dev.desktop.30/publication/`。

隔离数据目录从本轮源码运行 `update download --channel dev --product desktop --current-version 0.1.0-dev.desktop.29 --json`，默认源发现 `.30` 并实际下载 Linux 包，大小与 SHA256 通过；Windows 附件 digest 与本地一致。双平台生产编译及安装包构建通过；定向 Go 测试、前端类型检查、15项定向测试及受影响文件 lint 结果复用本轮构建提交的已有验证。未执行原生窗口、安装器或 apply 验收。

### 历史在线 dev 版 `.29`

`0.1.0-dev.desktop.29` 于北京时间 2026-10-13 23:51:11 发布为 dev prerelease（非 latest）。构建提交 `ea3196bc45b3b30b45d17d8c7ce8b7b9af53f832`，干净隔离工作区的 SOURCE.json 为 dirty=false。

模型映射恢复原有上下分区，仅常驻供应商、完整模型名、Key 名称和倍率；卡片收紧为三／二／一列，增加六点拖动把手、虚线边框与抓取提示。包含窄窗口滚动修复，以及 models.dev 参考目录和文本 token 费用估算基础接口／CLI；尚无模型参考目录新页面，沿用 .28 的 sub2api Key 计费倍率修复，无新增数据库迁移。

| 平台 | 安装包 | 字节数 |
| --- | --- | ---: |
| Ubuntu24.04 AMD64 | `anchor-desktop_0.1.0-dev.desktop.29_amd64.deb` | 5,676,204 |
| Windows AMD64 | `Anchor-Desktop-0.1.0-dev.desktop.29-windows-amd64-setup.exe` | 7,868,769 |

```text
b0e3fb2fd71969e5767e7ea5cd2ce857287ea328e7da73711e5e98cfc1c71db5  anchor-desktop_0.1.0-dev.desktop.29_amd64.deb
9f6cf53010ae59eb19a81b91502ed605128ae71caec0c4804ff0a900fe182079  Anchor-Desktop-0.1.0-dev.desktop.29-windows-amd64-setup.exe
```

六附件完整投放，GitHub附件大小／digest及正式URL与本地清单一致，标签指向实际构建提交。源码归档含 vendored Go 依赖，go.sum和映射样式与构建提交一致；元数据ZIP CRC通过，原始BUILD.json的原生／安装／签名状态保留。发布及下载证据位于 `dist/desktop/0.1.0-dev.desktop.29/publication/`。

隔离数据目录从本轮源码运行 `update download --channel dev --product desktop --current-version 0.1.0-dev.desktop.28 --json`，默认源发现`.29`并实际下载Linux包，大小与SHA256通过；Windows附件digest与本地一致。模型映射前端 typecheck、Lint 及倍率查询、实际拖动排序／保存、冲突刷新三项模拟宿主浏览器测试通过；双平台生产编译与安装包构建、六附件大小／digest核对、默认源 Linux 实际更新下载及大小／SHA256通过。未执行原生窗口、安装器或 apply 验收。

### 历史在线 dev 版 `.28`

`0.1.0-dev.desktop.28` 于北京时间 2026-10-13 23:15:54 发布为 dev prerelease（非 latest）。构建提交 `af7359642bcb340c3f723b19ee2b294b496bc3a8`，独立工作区的 SOURCE.json 为 dirty=false。

sub2api 默认以所选推理 Key 调用 `/v1/sub2api/billing`，读取服务端 `effective_rate_multiplier`，包含个人覆盖及当前高峰规则，无需 JWT。账户页、逐 Key 目录和模型来源倍率接受 `key_billing`；接口未披露分组 ID／名称时留空并显示「当前计费倍率」。只读快照不改路由倍率。旧站仅在404/405或200 HTML页面时回退用户授权，认证、权限、限流、服务及格式错误保留；显式后台 Key ID 仍按旧用户授权查询。

| 平台 | 安装包 | 字节数 |
| --- | --- | ---: |
| Windows AMD64 | `Anchor-Desktop-0.1.0-dev.desktop.28-windows-amd64-setup.exe` | 7,842,288 |
| Ubuntu24.04 AMD64 | `anchor-desktop_0.1.0-dev.desktop.28_amd64.deb` | 5,645,700 |

```text
301b1affe1c5d46d8451a12513d3e145bdaaf3e465c0722b3f36cbb1af5da1aa  Anchor-Desktop-0.1.0-dev.desktop.28-windows-amd64-setup.exe
381f40d2b6f233cdef6caaf18e4237f90bbcc086e9c7c893f22aed8c36ffe0dc  anchor-desktop_0.1.0-dev.desktop.28_amd64.deb
```

六附件完整投放，GitHub附件大小／digest及正式URL与本地清单一致，标签指向实际构建提交。源码归档含 vendored Go 依赖及本轮修复，go.sum与构建提交一致；元数据ZIP CRC通过，原始BUILD.json的原生／安装／签名状态保留。发布及下载证据位于 `dist/desktop/0.1.0-dev.desktop.28/publication/`。

隔离数据目录从本轮源码运行 `update download --channel dev --product desktop --current-version 0.1.0-dev.desktop.27 --json`，默认源发现`.28`并实际下载Linux包，大小与SHA256通过；Windows附件digest与本地一致。五包Go定向测试、前端typecheck/lint和44项账户／目录／展示测试通过。未使用真实站点凭据、真实Agent配置或付费推理；未执行原生窗口、安装器或apply验收。

### 历史在线 dev 版 `.27`

`0.1.0-dev.desktop.27` 于北京时间 2026-10-13 21:40:01 发布为 dev prerelease（非 latest）。构建提交 `5b1e676c5fbbe66c8802fe8962bfecea8b723664`，干净独立 worktree 的 SOURCE.json 为 dirty=false。

NewAPI 保存后台系统访问令牌后，供应商列表的余额摘要现在与账户详情一致，优先调用账户接口，返回 scope=account。此前列表忽略已保存用户授权；本版修复该缺口。系统令牌无效、权限不足或账户查询失败时保留错误，不退回 Key 限额。明确配置 NewAPI 时无需推理 Key 即可查询；未配置系统令牌时沿用 `.26` 的 billing / Key 额度兼容查询。

「供应商 → 账户与余额 → 平台账户配置」明确标注“NewAPI 系统访问令牌”，说明后台生成路径、输入／替换与保存方式、按站点要求填写的用户 ID，以及支持权限选择的站点所需的查询权限：余额读取需 profile:read，自动核对 Key 分组需 api_key:read、api_key:reveal。没有新增参数、公开 DTO 字段或数据库迁移。

| 平台 | 安装包 | 字节数 |
| --- | --- | ---: |
| Windows AMD64 | `Anchor-Desktop-0.1.0-dev.desktop.27-windows-amd64-setup.exe` | 7,836,542 |
| Ubuntu24.04 AMD64 | `anchor-desktop_0.1.0-dev.desktop.27_amd64.deb` | 5,644,688 |

```text
cdfbace6b16b52292518d12b4487cdcbe5a81e16186743c49cbb7452666f147d  Anchor-Desktop-0.1.0-dev.desktop.27-windows-amd64-setup.exe
337861a9c6c8ee473a57ec2bf5d9e11f92d89f0d8af6aff62fdd10fce2af4205  anchor-desktop_0.1.0-dev.desktop.27_amd64.deb
```

两安装包、flat 更新清单、对应提交的源码（含 vendored Go 依赖）、许可／构建元数据和 SHA256SUMS 共六附件上传状态、大小及 GitHub digest 与本地一致；标签指向实际构建提交。原始 BUILD.json 的编译／安装验收状态保留，发布证据位于 `dist/desktop/0.1.0-dev.desktop.27/publication/`。隔离数据目录用本轮源码执行 `update download --channel dev --product desktop --current-version 0.1.0-dev.desktop.26 --json`，默认源发现 `.27` 并实际下载 Linux 包，通过大小和 SHA256 校验；未运行安装器或 apply。

本轮五包定向 Go 测试、前端 typecheck/lint、15项模型／展示测试和两平台构建通过。首次打包因隔离目录的 node_modules 外部软链接在容器中不可达而失败，复制依赖到隔离目录后重新构建成功。未使用真实站点凭据、真实 Agent 配置或付费推理请求；未执行浏览器、双平台原生窗口及安装验收。

### 上一在线 dev 版 `.26`

`0.1.0-dev.desktop.26` 于北京时间 2026-10-13 19:27:47 发布为 dev prerelease（非 latest）。构建提交 `f9b40872e50fcdd5c692d934334e9d55cbfdd993`，干净独立 worktree 的 SOURCE.json 为 dirty=false。

NewAPI 无用户授权时，供应商列表和账户详情的「余额」使用所选推理 API Key 查询 billing subscription/usage，剩余值按 `hard_limit_usd - total_usage/100` 精确计算。账户详情使用用户授权时仍优先查账户钱包；billing 范围由站点设置决定，显示「站点余额」，不能直接视为账户钱包。公开显示单位保留 USD/CNY/quota，无法确认则标注 site_unit。无限额 Key 的占位数不当资金；列表仅在接口缺失或不兼容时回退 Key 额度，认证／权限／限流／服务失败保留原因。CLI 同步修复，本轮不新增数据库迁移。

| 平台 | 安装包 | 字节数 |
| --- | --- | ---: |
| Windows AMD64 | `Anchor-Desktop-0.1.0-dev.desktop.26-windows-amd64-setup.exe` | 7,840,670 |
| Ubuntu24.04 AMD64 | `anchor-desktop_0.1.0-dev.desktop.26_amd64.deb` | 5,643,800 |

```text
acc42c5f014c3a7092c1d6e15b3b7e29e53b3e8b65546f826c1aaf48090a3aa9  Anchor-Desktop-0.1.0-dev.desktop.26-windows-amd64-setup.exe
a296db47f12ed0ed2f9ec501f8d5886f16a021174e563ad553f43ec830a9c1c7  anchor-desktop_0.1.0-dev.desktop.26_amd64.deb
```

六附件上传状态、大小及 GitHub digest 与本地一致；标签指向实际构建提交。原始 BUILD.json 的编译／安装验收状态保留，发布证据位于 `dist/desktop/0.1.0-dev.desktop.26/publication/`。隔离数据目录用本轮源码执行 `update download --channel dev --product desktop --current-version 0.1.0-dev.desktop.25 --json`，默认源发现 `.26` 并实际下载 Linux 包，通过大小和 SHA256 校验；未运行安装器或 apply。

本轮五包定向 Go 测试、前端 typecheck/lint、14项模型／展示测试和两平台构建通过。未使用真实站点凭据、真实 Agent 配置或付费推理请求；未执行浏览器、双平台原生窗口及安装验收。

### 历史在线 dev 版 `.25`

`0.1.0-dev.desktop.25` 于北京时间 2026-10-13 18:37:11 发布为 dev prerelease（非 latest）。构建提交 `f37d11fc45ffa4e6ada05896164916528e0169e0`，干净独立 worktree 的 SOURCE.json 为 dirty=false。

模型映射显示真实来源 Key ID、启用／健康状态、已保存路由倍率及查询的平台分组倍率。目录导入保留来源范围，请求和重试不会越过该范围；旧模型空范围允许供应商全部启用 Key。候选可以拖动或上移／下移，每个模型有「调度设置」，「保存顺序与规则」以模型 revision 原子提交；冲突重读，不重放写入。引用中的来源 Key 不能被删除或清空。

固定顺序按保存顺序；加权轮询按路由倍率倒数分配；缓存优先将同一调用方、模型及稳定请求前缀交给最近成功的供应商／模型／Key，记录10分钟有效，无记录时加权轮询，不保证上游缓存命中。低价优先按保存路由倍率排序，不是实时货币价格；平台分组倍率不自动覆盖路由倍率。低延迟先学习未知来源，再按成功首段文本延迟平滑均值排序，无首段时使用总耗时，记录30分钟有效。两类观察仅在内存中保存，重启后重新学习。所有策略继续受健康和并发限制并保留故障回退。

「刷新分组倍率」及候选「查询分组倍率」按来源 Key 串行去重；「供应商 → 账户与余额 → 分组倍率」支持选择保存的 Key。sub2api优先个人覆盖倍率，再用默认分组；NewAPI核对完整 Key 后读取已认证分组倍率。需独立平台用户授权，NewAPI可能需用户ID；原版OneAPI不支持，未知不补默认值。CLI提供 model routing、model add/put --strategy / --candidate-key、provider model-put --source-key / --all-keys。首次启动 schema5→6，旧程序不能打开迁移后数据库。逐请求账单桌面页尚未接入。

| 平台 | 安装包 | 字节数 |
| --- | --- | ---: |
| Windows AMD64 | `Anchor-Desktop-0.1.0-dev.desktop.25-windows-amd64-setup.exe` | 7,829,725 |
| Ubuntu24.04 AMD64 | `anchor-desktop_0.1.0-dev.desktop.25_amd64.deb` | 5,634,024 |

```text
dc0adecd1c4514bb47807ba65c2f4ce045227568fa522770c396644979342d5f  Anchor-Desktop-0.1.0-dev.desktop.25-windows-amd64-setup.exe
ae00061cf5f2448b98ad84ab039e21441076337655136d764c39c8b8099d084e  anchor-desktop_0.1.0-dev.desktop.25_amd64.deb
```

六附件包含两平台安装包、flat更新清单、含vendored Go依赖的对应源码、许可／构建元数据和SHA256SUMS。全部上传状态、大小、GitHub digest、清单URL与版本已核对；标签指向构建提交。原始BUILD.json验证状态保留，发布证据位于 `dist/desktop/0.1.0-dev.desktop.25/publication/` 的 PUBLISHED.json、TAG_VERIFIED.json和ONLINE_DOWNLOAD.json。

隔离状态使用本轮源码 `update download --channel dev --product desktop --current-version 0.1.0-dev.desktop.23 --json`，从默认GitHub源实际下载Linux包并通过大小／SHA256校验，Windows远端digest与本地一致。定向Go测试、76项前端单测、8项单worker模拟宿主浏览器检查、typecheck/lint和双平台生产构建通过；未执行全量、race、原生窗口、PAC、安装器或apply验收，未修改真实Agent配置或请求真实付费上游。

### 保留本地包 `.24`（未在线发布）

同轮较早已从干净提交 `3f5fe7d9578fc698232a4d666d4237043b3a8bf3` 构建 `.24`，包含分组倍率显示；Windows安装包7,805,948字节，Linux包5,613,392字节，SOURCE.json dirty=false。该包保留在 `dist/desktop/0.1.0-dev.desktop.24/`；没有更新清单、附件上传或公开Release。后续来源Key与调度规则整合进 `.25` 交付，`.24` 不作为线上更新目标。

### 历史在线 dev 版 `.23`

`0.1.0-dev.desktop.23` 于北京时间 2026-10-13 15:28:01 发布为 dev prerelease（非 latest）。构建提交 `2ffc82b97fdcdf36a682bccd204184e4faec6070`，干净独立 worktree 的 SOURCE.json 为 dirty=false。

模型目录默认遍历供应商全部启用 Key，同名模型合并一行并保留所有成功目录返回的 Key 关联，展示完整可选 Key 名单、逐 Key 所属分组和平台返回的倍率。停用 Key 不参与目录或倍率抓取；全部 Key 停用时返回 no_enabled_keys，不发上游请求或匿名回退。只有未保存任何 Key 时允许匿名目录。已存协议覆盖仍可编辑，未经本次目录确认的覆盖不作为可选 Key。未知倍率保持未知，原版 OneAPI 不支持分组倍率，NewAPI 自动分组倍率仍未知。

沿用 `.22` 的账户与余额页面、schema5 和此前更新代理／官方订阅回退，不支持数据库降级。账单及独立倍率查询页尚未接入。

| 平台 | 安装包 | 字节数 |
| --- | --- | ---: |
| Windows AMD64 | `Anchor-Desktop-0.1.0-dev.desktop.23-windows-amd64-setup.exe` | 7,802,594 |
| Ubuntu24.04 AMD64 | `anchor-desktop_0.1.0-dev.desktop.23_amd64.deb` | 5,610,200 |

```text
fd6383bb7f93f4731ebb3e7736b32192bc5dd0b4c6a0069ff548c63eaaed79f2  Anchor-Desktop-0.1.0-dev.desktop.23-windows-amd64-setup.exe
1736440552833982ef1bb0d1ae8c344e36ab206071048e73b5eb125ce6cab579  anchor-desktop_0.1.0-dev.desktop.23_amd64.deb
```

产物位于 `dist/desktop/0.1.0-dev.desktop.23/`。六个附件包括两平台安装包、flat 更新清单、对应构建提交的源码及 vendored Go 依赖、许可／构建元数据、SHA256SUMS。源码归档核对 vendor/modules.txt、项目许可证与本轮实现文件，元数据 ZIP CRC 检查通过；全部远端大小和 GitHub digest 与本地一致，正式清单 URL 与同版附件匹配，标签指向构建提交。原始 BUILD.json 的未验收／未签名／未发布标记保留，真实发布证据为 `publication/PUBLISHED.json`、`TAG_VERIFIED.json`、`ONLINE_DOWNLOAD.json`。

隔离目录用本轮源码 `update download --channel dev --product desktop --current-version 0.1.0-dev.desktop.22 --json` 从默认 GitHub 源发现并真实下载 `.23` Linux 包，大小／SHA256 校验通过；Windows 附件远端 digest 与本地一致。复用功能提交已通过的 Go core/CLI/desktop 定向测试、前端 typecheck/lint、模型目录30项单测和1项浏览器交互；本轮契约比较及双平台生产编译、安装器生成通过。未执行原生窗口、PAC、系统安装、apply、全量或 race 验收，未修改真实 Agent 配置或请求付费模型。开发安装包未签名。重操作串行经 resource-guard，/tmp 保持42 MiB，无异常资源增长。

### 历史在线 dev 版 `.22`

`0.1.0-dev.desktop.22` 于北京时间 2026-10-13 11:07:28 发布为 dev prerelease（非 latest）。构建提交 `fbf2f2606b2b18d5d7223842602a533fbf1f418a`，干净独立 worktree 的 SOURCE.json 为 dirty=false。

供应商详情新增「账户与余额」，概况页提供快捷入口。可配置 sub2api、NewAPI、OneAPI 的平台类型、独立用户授权、可选站点根地址和 NewAPI 用户 ID；手动分别刷新钱包与 Key 额度。金额以十进制字符串保留原始精度，USD 与 quota 分开展示，未返回字段为未知。失败刷新清除对应旧结果，冲突或保存结果未知需明确重新读取配置，不自动重放用户令牌。复用 Wails → 类型化客户端 → 核心兼容层，无付费推理请求。

NewAPI/OneAPI 钱包需用户授权；NewAPI 另需站点用户 ID。OneAPI Key 额度需明确后台令牌数字 ID；sub2api 特定无限额 Key 可直接查钱包，受限 Key 额度不作为账户余额。账单与倍率桌面页尚未接入。沿用 schema5、更新代理和官方订阅回退，不支持数据库降级。

| 平台 | 安装包 | 字节数 |
| --- | --- | ---: |
| Windows AMD64 | `Anchor-Desktop-0.1.0-dev.desktop.22-windows-amd64-setup.exe` | 7,793,243 |
| Ubuntu24.04 AMD64 | `anchor-desktop_0.1.0-dev.desktop.22_amd64.deb` | 5,598,068 |

```text
20e480c773e069977beb175403711c24c15849af9dea2819b2b55540e487e9d8  Anchor-Desktop-0.1.0-dev.desktop.22-windows-amd64-setup.exe
757f488aea67ec091e9f6e6ce5a21273154365f83f13348018ba416ede55b179  anchor-desktop_0.1.0-dev.desktop.22_amd64.deb
```

产物位于 `dist/desktop/0.1.0-dev.desktop.22/`。六附件包括两平台安装包、flat 更新清单、对应构建提交的源码及 vendored Go 依赖、许可／构建元数据、SHA256SUMS。上传后所有附件大小与 GitHub digest 均匹配本地；标签指向实际构建提交，原始 BUILD.json 未验收标记保持不变。证据为 `publication/DRAFT_VERIFIED.json`、`PUBLISHED.json`、`TAG_VERIFIED.json`、`ONLINE_DOWNLOAD.json`。

默认源在隔离目录通过 `update download --channel dev --product desktop --current-version 0.1.0-dev.desktop.21 --json` 发现 `.22`，真实下载 Linux 包并校验大小／SHA256。Go 桥接隔离核心与模拟三平台、契约漂移检查、前端 typecheck/lint、78项定向单测、2项浏览器交互测试通过。浏览器最初因系统 Chrome 与普通 Chromium 环境失败，改用缓存 headless shell 后通过；未下载浏览器。双平台编译与安装器生成通过；未做原生窗口、PAC、系统安装、apply、全量或 race 验收，未改真实配置或调用付费模型。开发安装包未签名。

### 历史在线 dev 版 `.21`

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


## 历史在线 dev 版 .31

- 发布：北京时间 2026-10-14 22:34:21，GitHub `v0.1.0-dev.desktop.31`，prerelease／非 latest，六附件完整；标签与源码 revision 为 `1c5c9104e0d232749f1caf122d3b5fd2f4f133de`，SOURCE.json dirty=false。
- 新增“设备连接”：在 UI 内使用 SSH 密码／密钥添加、保存、重连与切换远端实例，自动核验／启动远端核心并建立管理连接，无需手动隧道。密码不保存，首次主机信任明确确认，主机密钥变化拒绝；桌面当前连接独立于 CLI。远端须预装支持 context bootstrap 的新版 CLI 且启用 SSH，自动安装尚未实现。包含已提交的模型设置与 Claude Code／DSH 托管更新。
- 使用：侧栏“设备连接” → “添加远程设备”，输入名称、user@主机、密码或密钥，确认首次信任，点击“添加并连接”；保存后可切换、重连或返回本机。密码连接重开应用后需重输，密钥模式支持 SSH 配置／Agent。
- 定向 Go 测试、前端 typecheck／lint、22 项单元测试、2 项连接浏览器测试、隔离核心 detached bootstrap 验证通过。双平台生产编译与安装包构建、六附件大小／SHA256 digest 核对、默认源 Linux 实际下载通过；Windows 远端 digest 与本地一致。本机 CLI／桌面二进制已升级，核心 ready，状态库已备份并迁移到 schema7。本轮执行环境未提供图形显示，未运行原生窗口、安装器或应用内 apply 验收。
- 本机沿用用户目录安装方式，以包中 ELF 替换 `~/.local/bin/anchor-desktop`，同步替换 CLI 并正常重启 `anchor-core.service`；没有执行 apt／NSIS 安装或打开窗口。

## 历史在线 dev 版 .32

- 发布：北京时间 2026-10-14 23:12:40，GitHub `v0.1.0-dev.desktop.32`，prerelease／非 latest；六附件齐全且大小、SHA256 digest 与本地一致。标签与实际构建 revision 均为 `b69d8b8f38006703ff700a2e8b673d99778ef168`，SOURCE.json dirty=false。
- 修复：SSH 非交互 PATH 缺少 ~/.local/bin 时，默认远端路径自动发现用户目录 CLI；密钥与密码模式共享发现命令，自定义路径不回退。bootstrap --help 先检查能力；固定脱敏错误区分未安装、版本不支持、管理端口不匹配及核心未就绪。用户须更新发起连接的桌面端，远端自动安装尚未实现。
- 验证：定向 Go 测试、前端 typecheck／lint、22 项 Vitest 单测通过；隔离真实 SSH／密码连接及 shell 路径、显式路径、诊断脱敏、能力检查不创建状态等用例通过。双平台安装包构建成功；对应源码、vendored 依赖与元数据 ZIP 核对通过；默认 GitHub 源实际下载 Linux .deb 并验证大小与 SHA256。
- 本机：CLI 与桌面 ELF 均更新为 .32，正常重启 anchor-core.service，实际运行核心与新二进制一致，core/gateway ready；状态备份位于 ~/.config/anchor/backups/pre-desktop32-20261014-230950，schema7 保持不变。无图形显示，桌面窗口未启动，未执行浏览器、原生 GUI、apt／NSIS 或 apply 验收。
- 过程：初次打包因隔离源码依赖软链接超出容器挂载而在许可收集失败；改为实际复制依赖后复用原打包脚本重试成功，失败产物未安装或发布。上传附件成功后草稿按 tag 查询返回404，改用 release ID 核验草稿并发布；发布后的 tag revision 已核验，没有重复上传或替换资产。

## 当前在线 dev 版 .33

- 发布：北京时间 2026-10-15 01:18:46，GitHub `v0.1.0-dev.desktop.33`，prerelease／非 latest；两平台安装包、更新清单、对应源码、许可／构建元数据及 SHA256SUMS 六附件完整。全部附件大小及远端 digest 与本地一致，标签指向实际构建 revision `72ed6e9c429bec7d1c246a51c0e3062720ff1668`，SOURCE.json dirty=false。
- 中文：Key 名称、公开模型 ID／别名、Context／设备连接名称和 Agent Binding ID 支持中文。用户名称允许 1–128 个 Unicode 字母／十进制数字或 `._-`，首字符须为字母／数字；Binding ID 另限 64 个 UTF-8 字节。令牌、实例／授权 ID、错误码和 SSH 技术字段继续使用各自技术约束；Codex TOML 中文供应商表名正确编码与恢复。
- 验证：复用已提交功能的八个 Go 包测试、49 项前端定向测试、typecheck／lint 结果；本次执行双平台生产构建、安装包校验、源码及 vendored 依赖／元数据核对。隔离源码 CLI 从默认 GitHub dev 源以 .32 为当前版本发现 .33，实际下载 Linux .deb（6,009,304 字节）并验证 SHA256；Windows 远端 digest 与本地一致。
- 边界：独立 CLI 正式预发行仍为 alpha.6；使用中文名称须更新桌面／源码 CLI 及核心。本次仅打包并在线投放，没有替换本机程序、停止真实核心或改动 Agent 配置；未执行原生窗口、apt／NSIS 或 apply 验收，开发安装器未签名。工作区未提交的余额相关改动未包含在构建中。

## 内置 mihomo 构建输入

Windows/Linux amd64 桌面构建入口会先执行 `scripts/prepare-mihomo.py --target TARGET`，下载固定 v1.19.32 compatible archive 并核对 SHA256；生成物位于被 Git 忽略的 `internal/egress/payload/`。失败必须中止，不构建声称内置但缺少内核的包。CLI release 同样准备对应 payload。直接 Go 开发构建可不带 payload，代理页会准确显示缺失。独立可执行文件内嵌 GPLv3 及来源通知，安装包第三方声明也包括它们；分发须继续提供对应版本源码下载，见 `third_party/mihomo/NOTICE.md`。此改动不是已经发布的安装包。
