# Anchor

本地 Agent 模型中转与管理工具。管理用户选择的 Codex、CC、DSH 等平台，
提供供应商配置、模型映射、自动路由和协议转换，不提供自有 Agent 执行引擎。

## 公开下载

当前公开 CLI 预发行版 **0.1.0-alpha.6**，包含中文供应商名称支持；桌面 dev 版为 **0.1.0-dev.desktop.42**，可在已有更新入口中选择 dev 检查／下载；从 `.34` 及更早版首次迁移须安排维护窗口并手动安装，详见下方升级注意：

- [Windows x64 单文件 EXE](https://github.com/candyflosssss/Anchor/releases/download/v0.1.0-alpha.6/anchor_0.1.0-alpha.6_windows_amd64.exe)
- [Linux x64 单文件](https://github.com/candyflosssss/Anchor/releases/download/v0.1.0-alpha.6/anchor_0.1.0-alpha.6_linux_amd64)
- [发行页、SHA256 和对应源码](https://github.com/candyflosssss/Anchor/releases/tag/v0.1.0-alpha.6)

桌面 dev 安装包（旧版代理接入失败时可手动升级一次）：

- [Windows x64 图形界面安装器](https://github.com/candyflosssss/Anchor/releases/download/v0.1.0-dev.desktop.42/Anchor-Desktop-0.1.0-dev.desktop.42-windows-amd64-setup.exe)
- [Ubuntu24.04 x64 安装包](https://github.com/candyflosssss/Anchor/releases/download/v0.1.0-dev.desktop.42/anchor-desktop_0.1.0-dev.desktop.42_amd64.deb)
- [桌面发行页、SHA256 和对应源码](https://github.com/candyflosssss/Anchor/releases/tag/v0.1.0-dev.desktop.42)

CLI 下载无需登录 GitHub。也可按 [一键安装说明](docs/release/install.md) 下载并自动校验。Windows 原生核心烟测及 PowerShell 5.1/7 安装器回归已通过；这是命令行预发行程序，不是图形界面安装器，尚无代码签名或自启动。

## 当前阶段

已开始实现首个开发里程碑：Go 无头核心与 CLI、本机 Context、独立管理 / 网关鉴权、
SQLite 供应商与模型配置、Responses / Chat Completions / Anthropic Messages 无状态文本及函数工具转换，以及托管层/条件恢复、Codex 基础配置与模型目录、多模型绑定授权。当前版本为 `0.1.0-dev`，
不等于整体初版已完成；desktop.31 已实现 UI／CLI SSH 设备连接，并包含 Claude Code／DSH 基础托管。远端自动安装、其他 Agent 适配器、Codex profile 和完整协议能力仍待实现。

支持 Linux / Windows AMD64；alpha.6 已通过两平台原生核心运行门禁，是 CLI 预发行程序。desktop.31 dev 提供 Wails 桌面宿主、供应商真实接入、独立核心生命周期和 SSH 设备连接，同时保留明确标记的 Mock 预览，不开发独立 Web 产品。Windows 当前用户 NSIS 与 Ubuntu24.04 AMD64 `.deb` 已构建并在线投放；本轮定向 Go 测试、前端 typecheck／lint、22 项单元测试与 2 项设备连接浏览器测试通过。原生窗口和安装器未实机验收。构建和边界见[桌面开发版说明](docs/release/desktop.md)。

供应商[第一优先六项](docs/design/provider-roadmap.md)已在当前源码完成：删除/复制/改名、目录与模型映射/推理测试、多 Key、冷却/熔断、三协议、用量及单价估算。桌面详情采用「密钥与状态」「模型与测试」「用量」「账户与余额」等页签，CLI 同步提供。实现边界和 schema3→4 迁移见[工作台契约](docs/design/provider-priority-one.md)，操作见[最新指令大全](docs/cli/commands.md)。这些能力已包含在 desktop.12 dev 桌面发行中；公开 alpha.6 CLI 不含这些增量。当前源码进一步把协议设置移至 Key，并支持按实际模型覆盖、默认动态继承 Key 协议；该协议继承已随 desktop.16 发布。desktop.21 加入三平台账户与计费查询核心接口，并升级到 schema5；desktop.22 接入「供应商 → 选择供应商 → 账户与余额」，可保存独立用户授权并分别刷新账户余额／Key 额度；desktop.23 默认查询全部启用 Key 的模型目录，同名模型合并并显示全部可选 Key、所属分组与抓取倍率，停用 Key 不抓取；desktop.25新增映射来源Key、逐Key分组倍率、拖动排序与实际调度菜单，并接入账户页分组倍率查询，升级到schema6。desktop.26修复NewAPI只配API Key时的billing余额查询，显示站点范围并保留明确单位，账户详情使用已有用户授权时仍优先查账户钱包。desktop.27修复列表忽略系统访问令牌的问题，并增加后台令牌的获取与配置说明。账单桌面页尚未接入，命令由源码CLI提供，公开alpha.6 CLI不含。

**已确认的正式桌面目标（Windows 与 Linux 一致）**：安装后双击应用直接打开 GUI，由宿主自动启动或核验连接本机后台核心，无需手动开终端；desktop.14 关闭窗口隐藏到系统托盘，单击托盘图标恢复；无可用托盘时直接退出界面。「退出界面」退出宿主，均不停服，显式停服单独确认，开机自启动为默认关闭的独立设置。详见[桌面启动与生命周期规范](docs/design/frontend.md#11-已确认的桌面启动与生命周期目标2026-10-11)。桌面 dev 包已包含启动／复用与窗口生命周期实现；开机自启动设置未实现，原生安装与窗口行为未验收。

## 开发运行

以下为 CLI 开发流程。GUI 开发／构建见[前端说明](desktop/frontend/README.md)及[桌面开发版构建](docs/release/desktop.md)，不是打开公开 CLI 包就出现图形窗口。

需要 Go 1.25.0 或更新版本。当前机器的开发入口会自动使用资源保护脚本。

```bash
./scripts/anchor-dev.sh init
./scripts/anchor-dev.sh serve
```

在另一个终端执行：

```bash
./scripts/anchor-dev.sh context add local --local
./scripts/anchor-dev.sh context use local
./scripts/anchor-dev.sh status --json
```

服务前台运行，不由管理命令退出触发停服；关闭 serve 终端可能停止服务。本轮源码新增 Context 作用域 `stop --approve` 请求正常退出（alpha.6 CLI 尚不包含），只报告接受、不保证恢复完成；桌面关窗不会调用它。
Codex 托管使用稳定 Anchor 二进制及明确批准的配置根，登记后先审核计划再接管。操作契约见 [最新 CLI 指令大全](docs/cli/commands.md#8-agent-托管与独立恢复)。已公开的 Linux `0.1.0-alpha.3` 包含上述能力；验收结果见任务记录，旧测试包不包含它。

桌面端通过 SSH 使用远端 Codex 的主路径：在远端批准 CODEX_HOME 基础配置 → Anchor provider + 默认模型 + 版本匹配模型目录 → 用户手动选择其他授权模型 → Anchor 路由实际供应商。新 CLI 绑定默认 `all-compatible`，旧绑定保留单模型权限；无需额外 profile。目录元数据需操作者显式导入并核对真实上游能力，修改目录后重启远端 app-server。Anchor 自己的 SSH Connector 和桌面 UI 验收不在本轮范围。

完整配置命令、实际 API、安全限制和未实现能力见
[首个实施里程碑](docs/design/implementation.md)。
凭据当前依赖用户文件权限 / Windows ACL，**不是静态加密**。

## 打包与发行

以下一键安装入口用于 CLI alpha.6。Windows GUI 安装程序和 Ubuntu24.04 AMD64 `.deb` 已随 desktop.42 发布到 dev 在线更新渠道，新增日志用量／TPS／错误率／缓存命中率等多尺度趋势图，使用方式与构建结果见[桌面安装说明](docs/release/desktop.md)；本轮已完成默认源 Linux 安装包真实下载及完整性检查，Windows 附件 digest 与本地一致；未自动安装、替换运行程序或进行原生窗口验收。

已提供 [一键安装脚本](docs/release/install.md)：识别 Linux/Windows AMD64、下载公开 Release 的单文件并校验 SHA256，默认安装到用户目录，不需要 sudo/管理员权限、不改 Codex 配置。可固定版本，已有程序必须显式允许替换；未设置 PATH 时只提示，不编辑 shell 配置。**Linux / Windows AMD64 `0.1.0-alpha.6` 已公开发布；两平台附件匿名下载与校验通过，Linux 远程一键安装已验证**。不从 Actions artifact 或草稿偷偷安装。

**运行只需要一个文件**：Linux 的 `anchor` 或 Windows 的 `anchor.exe`，核心、CLI 和认证 helper 都在其中；不依赖旁边的文档或资源文件。配置/数据库仍独立存放于用户状态目录，首次使用仍需 init 和供应商/模型配置。

已提供 [CLI 发行流程](docs/release/README.md)：输出两个平台各自的单文件可执行程序，并保留含文档/依赖声明的可选归档；进行校验与程序验收。入口为 `scripts/release.sh`，不会自动提交、打标签或上传。
项目采用 **AGPL-3.0-only**：允许商用，分发及修改版网络服务的源码义务见 [许可说明](docs/license.md) 和 [LICENSE](LICENSE)。公开候选必须来自干净版本提交，并附对应源码（含 vendored 依赖）及声明；双平台候选必须通过 Windows 原生 ACL/锁、初始化、中文供应商、模拟网关和正常退出检查，不能把打包成功视为公开发行完成。

已配置 [GitHub Actions 自动打包与草稿发行](docs/release/github-actions.md)：手动运行生成内部测试程序，分别提供每个平台仅含一个程序的 artifact（GitHub 下载外层仍为 ZIP）；推送预发行标签后构建两平台公开候选（含对应源码），经 Windows 原生验收与安装器矩阵通过后创建草稿 Release。配置已纳入项目源码；实际推送和托管运行结果见任务记录，不自动公开发布。

## CLI 指令与维护

- [最新 CLI 指令大全](docs/cli/commands.md)：唯一当前指令目录，含参数、默认值、作用域、示例与功能覆盖缺口。
- [CLI 指令变更历史](docs/cli/changes.md)：记录指令增删改、参数与行为变化，以及兼容/迁移方式。
- [项目 CLI 文档 skill](.agents/skills/anchor-cli-docs/SKILL.md)：开发用户可操作功能和修改 CLI 时使用；项目约束见 [AGENTS.md](AGENTS.md)。

大部分用户可操作功能应有 CLI。每次实际 CLI 契约变化必须同时更新帮助、相关测试、最新大全和变更历史；设计计划不算已实现命令。

## 设计文档

- [供应商能力路线图](docs/design/provider-roadmap.md)：完整优先级、当前进度与本轮第一优先六项范围。
- [初版技术设计（总览）](docs/design/architecture.md)：会话确认的范围、总体架构、
  代理与托管职责、生命周期、远程管理、验收目标和待定技术决策。
- [CLI 连接与实例管理](docs/design/cli-connections.md)：本机连接、SSH 远程管理、
  context 切换、认证、安全边界，以及未来桌面端复用契约。

- [托管层与 Agent 接入契约](docs/design/agent-management-layer.md)：内置适配器、
  声明式接入、绑定对象，以及常驻服务的自动接管和条件恢复。

- [桌面前端技术设计](docs/design/frontend.md)：UI 基调、React / TypeScript / shadcn/ui 选型、
  Wails 桌面容器、前端功能模块 MVC、前后端职责边界与 Linux / Windows 跨平台目标；本机真实接入契约已实现。
- [供应商第一版界面设计](docs/design/providers-v1.md)：现有 CLI 能力对应的列表、详情、编辑与风险确认；支持明确标注的[本地 Mock 原型与真实 Wails 桌面模式](desktop/frontend/README.md)，真实接口已接入；原生安装与窗口验收单独记录。
- [前后端分离与前端 MVC skill](.agents/skills/anchor-frontend-architecture/SKILL.md)：前端及相关接口开发时遵循的分层、文档同步与验证约束。
- [多 agent 契约协作 skill](.agents/skills/anchor-contract-collaboration/SKILL.md)：前后端并行开发的共同契约、文件归属、变更协商、共享样例和真实联调验收。


### 在线更新（desktop.42 已投放 dev 渠道）

**设置 → 在线更新 → dev · 快速测试 → 检查更新 → 下载更新 → 安装更新**。默认 GitHub 源已发布 `0.1.0-dev.desktop.42` 的 Windows / Ubuntu24.04 安装包与同版清单。日志页新增请求量、Token 用量、总 TPS、错误率、缓存命中率、平均耗时和首字延迟趋势；范围15分钟至90天，粒度自动或10秒至1天，复用当前筛选与全部匹配的保留历史，支持暂停、刷新和键盘查点。未知数据不补0，沿用 schema8、无新增数据库迁移。本轮默认源发现、Linux 真实下载和大小／SHA256 校验通过，Windows 附件 digest 与本地一致；未自动安装或替换运行中的程序，原生窗口／安装器／真实付费上游未验收。

**desktop.34 及更早版的旧升级器不含修复，首次迁移请备份状态、安排维护窗口、普通停止旧核心并退出全部界面，再手动安装 `.42` 并重新打开 Anchor，核对核心和 Agent 真实连接。** 新宿主遇到不支持交接的旧核心会拒绝在线安装，不降级为普通停服或强杀。主机与核心均升级后，后续更新才使用保留交接；仍有停机窗口，安装器不自动启动核心，既有 Codex 会话缓存地址不会自动迁移。远端核心须另行升级。沿用 state.db schema8，从更早版升级须备份，旧程序不能打开迁移后的数据库。Linux 默认源实际下载及大小／SHA256 检查通过，Windows 远端 digest 与本地一致；此前已完成 desktop.40 的本机 Linux 用户目录升级及相关隔离检查；本轮 desktop.42 未升级本机程序；未执行系统安装器、原生窗口可见性或当前 Codex 会话重载验收。公开 alpha.6 CLI 不含本轮修复。旧 desktop.8 无更新入口。

desktop.35 的桌面安装确认使用保留交接停服、真正退出当前宿主，再交给系统安装器；安装完成须重新打开程序。desktop.14 起普通关闭隐藏到系统托盘，托盘菜单可打开或退出界面；其他窗口需要用「退出界面」退出。「退出界面」「停止核心」也位于顶栏「本机实例」弹层。CLI 源码同时提供本机更新命令（alpha.6 不含），原位替换保留上一版。语法见[最新指令大全](docs/cli/commands.md)，渠道投放、数据库迁移与验证边界见[在线更新](docs/design/online-updates.md)及[桌面发行说明](docs/release/desktop.md)。
