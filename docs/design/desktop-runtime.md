# 桌面运行契约 v1

- 日期：2026-10-11；标识：`anchor-desktop-v1`。
- 本轮实施基线：现有 `internal/client`、核心实例锁、Provider revision 与 secret 三态；不改已有供应商业务规则。
- 状态：源码已实现，Go 隔离真实核心及浏览器模拟边界测试通过；不是发行或双平台验收证明。共享 Go DTO、TypeScript 生成文件及样例由本轮唯一实现者维护。

## 1. 宿主接口

规范源为 `internal/desktop/contract.go`，通过 `go run ./cmd/desktop-contract` 生成前端类型。Wails 只绑定 `Bridge` 的受限方法：Start、Status、Connections、ConnectSSH、UseConnection、Providers、SaveProvider、StopCore、ExitDesktop，以及在线更新增量 UpdateStatus、CheckUpdate、DownloadUpdate、InstallUpdate（详见[在线更新](online-updates.md)）。前端仅通过桌面传输适配器访问绑定，不能指定文件、命令、凭据或任意目标地址。

每个结果带 `contract=anchor-desktop-v1`、data 与 problem；problem 仅含白名单 code，不返回原始错误、路径、管理令牌或供应商秘密。列表始终为数组。返回 Provider revision 为 1..Number.MAX_SAFE_INTEGER；写入 expected_revision 为 0..MAX_SAFE_INTEGER-1，且等于输入 provider.revision，超范围拒绝。SaveProvider 的 secret=null 保留、空字符串清除、非空替换；只入不出。写入发生网络/响应异常时返回 unknown，不自动重试；409 为 conflict、认证失败为 unauthorized。

## 2. 本机生命周期

GUI 在当前用户的默认 Anchor 配置目录取得独立的 `desktop-host.lock`；该锁不随 `--data-dir` 改变，同一用户只保留一个界面和托盘。锁在窗口与配置写入之前获取，重复启动只向已有宿主发送恢复请求并正常退出，不改变其数据目录、更新源或渠道。恢复请求在窗口尚未就绪时合并排队，就绪后还原最小化并显示／聚焦窗口，隐藏到托盘也使用相同路径。显式退出或更新交接期间拒绝恢复；若已有宿主暂未响应，最多等待5秒后报错，不回退多开。进程崩溃后内核释放锁，残留元数据不阻止下一次启动。

实例通知仅监听动态分配的 IPv4 回环端口，使用当前用户权限保护的随机凭据，不接受启动参数、浏览器 Origin 或业务操作，不占用核心／其他代理的固定端口。锁和通知属于 GUI 宿主机制，`--core` 与 `--finish-update` 安装 helper 在进入该机制前分支返回；独立 CLI 核心仍按数据目录使用原有实例锁。源码隔离测试覆盖并发与跨进程启动、早期恢复、退出、异常退出后重开和通知鉴权；原生两平台窗口恢复尚未验收。

桌面二进制以默认用户状态目录或显式 `--data-dir` 运行；JS 不接受目录参数。Start 在有界超时内取得桌面启动协调锁：安全初始化缺失状态，核验本机身份/API/授权，就绪则复用；否则依据实例锁启动同一二进制的受限 `--core` 模式。两个监听面与托管协调就绪后才开放供应商页；核心 starting/stopping 时拒绝新写入／网关请求（starting 时仍可正常请求停服）。重复打开不重复启动核心；未知端口占用、身份不符与无效状态不得覆盖、强杀或回退 Mock。

子进程不使用窗口 context、标准输入输出管道或父进程死亡信号；Windows 隐藏控制台并创建独立进程组，Linux 使用独立 session。从 desktop.14 起，普通关闭请求（自定义关闭按钮或系统关闭）隐藏到系统托盘，点击托盘图标可还原；无可用托盘时直接退出宿主。「退出界面」经 ExitDesktop 真正退出宿主，只取消本次查询并释放客户端，不停止核心。已有正常核心可以复用，不接管其外部服务管理器。连接来源对照已认证 instance 响应的 process_id 与本次子进程 PID，CLI 并发赢得实例锁时标为复用；旧核心无该字段同样保守复用。PID 仅用于来源标记，绝不替代身份／授权或用于强杀；Status 轮询刷新来源与停服能力。

Status 只查询，不启动／重启；核心退出后 UI 禁写，用户明确重新连接时才再次 Start。StopCore 须明确确认当前实例身份，通过带管理鉴权和 X-Anchor-Instance 的 POST /control/v1/shutdown 请求核心正常停服。返回 accepted 仅代表请求接受，不等于退出或恢复成功；UI 随后转为断开，不显示“恢复成功”。CLI 对应 `stop --approve`，无强杀和自动重试。

首次启动仅复用已批准的托管策略，不新增 Agent 授权。安装器沿用本契约，不在安装期间启动核心或登记服务；用户级开机自启动未实现，也不显示假开关，保持默认关闭。无图形环境继续使用 CLI。

## 3. 前端模式

Vite `prototype` 模式明确使用内存 Mock，仍可测试已有设计场景；正式桌面构建不包含 Mock 场景入口，不在绑定缺失／失败时回退模拟数据。真实界面移除“演示保存”等文案，展示连接状态和实例来源。2026-10-13 文案清理后，不再在侧栏和本机实例弹层常驻展示关窗不停服说明；关闭和核心生命周期行为保持既有实现。

### 3.1 自定义桌面顶栏（2026-10-11）

Windows / Linux 统一启用 Wails `Frameless`，以现有工作区 header 承担拖动和窗口控制，不改变核心业务规则；desktop.12 增加窗口生命周期方法 ExitDesktop，复用 StopResult envelope（accepted 仅指已接受退出请求）。窗口 View → hook Controller → Model → `desktop/window-runtime.ts`，适配器只允许 `WindowMinimise`、`WindowToggleMaximise`、`WindowIsMaximised`、`Quit`；接口依据仓库锁定的 Wails v2.15.0 runtime 源码，不向页面暴露通用 runtime 或系统执行入口。

拖动使用 `--wails-draggable: drag`；交互元素及窗口按钮明确为 `no-drag`。双击仅处理拖动区域。状态查询有超时并在 resize / focus / 恢复可见时更新，组件卸载移除监听和计时器；缺失 runtime 显示失败而不回退模拟。浏览器 prototype 隐藏原生控件。

desktop.14 起，`Quit` 普通关闭请求由 OnBeforeClose 拦截：只有系统托盘已就绪时才调用 `WindowHide`，窗口从任务栏消失、Anchor 图标保留在通知区域；不调用 StopCore。单击托盘图标或菜单「打开 Anchor」恢复窗口；菜单「退出界面（核心继续运行）」复用 Bridge.ExitDesktop。最小化按钮仍调用 WindowMinimise，保留标准任务栏最小化行为。

托盘由 Go 宿主的 `internal/desktoptray` 管理：Windows 使用原生通知图标和独立锁定线程的消息循环；Linux 使用 D-Bus StatusNotifierItem，并在隐藏前确认当前进程已注册到可用的 StatusNotifierHost。没有可用托盘时允许宿主真正退出，后台核心继续服务，用户从应用入口重新打开，不留下无法恢复的隐藏窗口。托盘只在 GUI 模式启动，`--core` 和安装 helper 不创建托盘。

ExitDesktop 和已经批准的安装更新共用宿主退出回调：先设置原子退出标记，再调用 Quit，绕过隐藏，确保更新 helper 不会卡在等待窗口退出。ExitDesktop 不要求核心在线，更新交接中返回 conflict；未绑定宿主退出回调时返回 unsupported。宿主退出会清理托盘。最小化 / 最大化 / 还原 / 托盘是本机窗口展示行为，不增加 CLI 管理指令。定向 Go 测试覆盖关闭隐藏、无托盘退出、显式退出及更新交接保护；Windows / Linux 原生托盘、拖动、缩放及按钮行为仍需原生验收。

## 4. 验收

- Go DTO/生成漂移、错误分类、秘密不回传、冲突、空列表及安全整数。
- 隔离目录真实核心：初始化、首次启动、并发复用、供应商增改、秘密清除、宿主关闭后仍服务、显式停服、端口冲突及错误凭据。
- Windows 源码编译不等于 Windows 原生窗口／后台进程验收；Linux 编译缺 GTK/WebKitGTK 时如实记录。
- 原型浏览器测试不替代真实 Wails 绑定及两端安装验收；不请求付费上游、不读取真实 Agent 配置。


## 5. 分轮验证与交付边界

此前宿主实现轮的 Go 全包 race/vet、Go→TS 漂移、共享样例以及真实隔离核心生命周期／供应商读写已通过；包括真正退出宿主进程后核心仍可服务、并发复用、丢失 owner.token 不重新创建凭据、认证与端口占用拒绝。前端 50 项单元、15 项原型浏览器和 6 项模拟 Wails 边界浏览器测试通过。

此前 Windows `.3` 内部 EXE 已交叉编译；当时 Linux 因宿主缺 GTK3/WebKitGTK4.1 开发依赖未产出。后续安装包轮新增受限 Ubuntu24.04 构建容器、Windows NSIS／Linux `.deb`，不改上述接口或核心生命周期；Linux 图标与应用入口在宿主中对应同一 ProgramName。该轮按用户要求不运行测试、安装、真实 GUI／核心或业务验收，构建结果单独记录，不能沿用旧测试作为新包验收。当前产物与剩余门禁见[桌面开发版构建](../release/desktop.md)。公开 alpha.6 分发不变。


## 6. 在线更新增量（2026-10-11）

新增 UpdateInfo/ConfirmUpdate/UpdateResult DTO 和 integrity/permission 错误码，保留既有 anchor-desktop-v1 供应商与生命周期方法语义。更新读取／下载与核心业务分别用锁，长下载不占用供应商／Status 的会话锁。安装必须明确批准版本，重新校验磁盘文件，认证本机核心并等待正常退出，再交给独立 helper 等待窗口退出；核心抢占、身份变化、文件被修改或不兼容均拒绝，不强杀或自行提权。普通关窗仍不停服。最初增量仅实现源码和隔离验证；desktop.16 已发布到默认 dev 源且完成真实下载校验，安装器交接及双平台原生更新验收未执行。

### 2026-10-15 安全重启交接修复（desktop.35 已发布）

安装更新在原有版本批准、文件校验、更新锁／桌面启动锁和本机身份检查后，要求本机核心声明 `restart_handoff`，通过共享 client 的 `StopForRestart` 发送 `{"for_restart":true}`，且必须收到保留确认；不再调用会恢复原配置的普通 Stop。缺少能力（包括已安装 desktop.34）时 fail closed，返回 unsupported，核心继续运行，不降级为普通停服或强杀。远程连接选中时依然只交接本机核心。

交接保留 Agent 配置、目录、凭据和恢复日志，等待本机实例锁释放后才启动安装 helper。helper 启动失败不退出 GUI、不伪报安装成功；核心可能已经停止，当前托管地址仍保留，可显式重新启动兼容核心后恢复服务。新核心按原日志核验复用，普通 StopCore 与系统信号仍条件恢复，不改变关闭窗口不停核心的约定。安装器本身不负责自动启动核心，安装后重新打开桌面；交接不承诺无停机或自动迁移已缓存旧地址的 Codex 会话。

该修复已随 desktop.35 的宿主与核心发布到 dev 渠道；旧版首次迁移须按[在线更新说明](online-updates.md)安排人工停服与安装，不能视为旧升级器已安全迁移。发布轮次通过双平台构建与默认源 Linux 实际下载检查，未进行原生安装验收。

隔离回归覆盖真实核心退出／重启期间配置、目录、日志和 token 不变，同一地址／凭据在重启前后均真实代理到模拟上游；也覆盖普通停服恢复原配置、旧核心零写拒绝及 helper 失败。未执行原生安装或修改本机真实 Agent 状态。

## 上游模型目录增量边界（desktop.10）

新增 `Bridge.DiscoverProviderModels`，输入仅已存 provider ID / revision / 可选 key_id（默认遍历所有启用 Key），返回独立只读目录 DTO：models 合并模型，entries 关联 Key，keys 包含目录状态与抓取的分组倍率，partial 标明部分失败，不扩展通用网络／文件能力。调用复用管理连接、实例身份和核心目录规则；POST 网络错误按只读处理，不标记写入结果未知。单次上游请求总超时最多8秒，宿主包含握手仍最多15秒，Wails 前端取消仅丢弃过期结果。细节、共享样例与无推理保证见[供应商接口 §4](provider-interface.md)。


## 第一优先工作台增量（desktop.11）

受控新增 ProviderWorkspace/ProviderAction；DTO、引用保护、确认和 revision 规则见[供应商接口 §5](provider-interface.md#5-第一优先工作台desktop11)。模型目录仍单独只读，推理测试是用户显式批准的一次最小请求。密钥仅输入不返回，价格导入只预填。前端保留 feature Model/View/hook Controller 边界。

本轮真实 Go 桥接→client→隔离核心的读取/写入/冲突及契约漂移已通过；前端类型检查及55项定向单元通过。不运行浏览器和原生安装验收，不将这些结果代替系统 WebView 实测。升级须先显式正常停止旧核心并退出窗口，安装新包后再启动；本轮数据库自动升级至 schema3，旧二进制不能打开升级后的数据库。


## 日志桥接（2026-10-12）

新增 Logs / LogDetail / LogSettings / ConfigureLogs / ExportLogs，DTO 规范源仍是 contract.go；共享样例为 desktop/contract/logs.json。宿主连接复用现有 client 管理 API，保留鉴权和实例身份保护。列表/详情超时5秒；导出2分钟，连接核验后释放 session mutex，不占住其他桌面操作。

ExportLogs 的路径只来自原生 SaveFileDialog，不接受 JS 文件路径；创建目标目录的600权限 partial，流式写入、Sync/Close/Rename，失败清理任务自己的部分文件；取消返回 accepted=false。列偏好在前端保存，不送核心；缺桥接或读取失败不使用 mock。核心日志生命周期独立于窗口，宿主退出不停止日志采集。具体语义和边界见[日志实现](logging.md)。

## Agent 托管增量（2026-10-12 实现，desktop.21 dev 已发布）

受控新增 AgentWorkspace / EnrollAgent / ManageAgent，DTO 来源为 managed 应用服务，在 contract.go 中声明并生成 TS；共享样例为 desktop/contract/agents.json。2026-10-13 当前源码的 AgentModel 增加 available_targets（始终为数组）：核心使用与登记/网关一致的 BindingModels 自动策略计算每个映射可用的目标 ID，包含已有共享元数据、精确缓存匹配、tools/streaming 与启用供应商检查；预览不读取旧目标级覆盖，表示本次按已有映射重新登记后的策略。桌面只消费该结果，不自行匹配或让用户重复选模板。登记仅收集发现目标 ID、默认模型、期望 revision 和明确批准，提交空 model_templates；核心/CLI 保留高级模板覆盖兼容。操作只允许 sync/enable/disable。前端不能指定配置路径、可执行程序或认证令牌，宿主不扫描目录、不写 Agent 文件，均复用 client 到核心的管理鉴权和实例身份校验。超时15秒；写失败按既有 problem 规则，结果未知不重放，Controller 重读状态。

2026-10-15 当前源码（未发布）诊断补充：HTTP 409 不再全部投影为 conflict；ProblemCodes 白名单新增 agent_config_conflict、agent_config_unsafe、agent_model_unavailable、agent_catalog_invalid、agent_operation_failed，分别表示外部修改、安全编辑受限、默认模型不可用、目录生成失败和未分类失败，普通 revision 冲突仍为 conflict。核心仅发送固定安全消息，原始错误 cause 留在本地错误链；未知写结果仍优先为 unknown。Go DTO 规范与生成 TS 同步，不透传任意后台错误码或原文。Controller 失败后保持／展开对应目标并重读，只有重读成功才宣称已重新读取；重读失败清空旧工作区并提示结果未确认。enabled 只表示托管策略，不代表 applied；未 applied 的 enabled 绑定在折叠行仍显示真实状态。desktop.39 不含本次修复，Windows 截图的具体根因不能从旧通用红字确认。

启动/停止仍由 Session 管理；进入页面要求核心 ready，核心启动只应用已批准 enabled 目标，不自动批准发现结果。GUI enable 先更新 enabled/revision，再 sync，新状态可能部分成功，因此失败后读取核对。正常停服条件恢复；关闭窗口保持核心运行。桌面可执行程序支持适配器精确 token helper 入口，在创建窗口前分派 CLI 凭据读取。

前端 Mock 交互、共享 DTO 检查和真实 Bridge/core 隔离测试分别验证不同层次，不代表 Windows 原生模型选择器或真实上游通过。

## 平台账户与额度桌面接入（2026-10-13，当前源码）

供应商详情新增「账户与余额」。受控方法 `ProviderAccountSettings` / `SaveProviderAccount` / `ProviderAllowance` 复用类型化管理客户端与已有 providerapi 核心。连接与实例身份在会话锁内校验，平台网络查询在解锁后执行，使用会话上下文和有界超时；不持有宿主锁等待上游。JS 不获得任意 upstream HTTP 方法或用户令牌读取能力。

公开 DTO、令牌三态、十进制字符串、额度 scope 和冲突语义详见[供应商契约 §8](provider-interface.md#8-平台账户与计费兼容层2026-10-13当前源码)。共享样例同时用于 Go round-trip、前端 Model 测试和 Mock；真实桌面模式无 Mock 回退。隔离 Go 桥接→核心→模拟三平台测试覆盖读取、写入、认证、精度和冲突；浏览器模拟 Wails 只验证交互，不代替 Windows/Linux 原生窗口、安装或真实站点授权验收。没有新增数据库迁移或修改 CLI 契约。

### 列表余额与自动平台（2026-10-13，当前源码未发布）

ProviderAllowance 增量接受 balance-summary，返回 account 或 key scope 与本次识别的平台。缺少／auto 配置由核心探测；列表只使用默认推理 Key，保存的独立用户授权只供显式账户查询等既有用途。沿用 anchor-desktop-v1、现有管理认证、revision、宿主有界上下文和共享样例；DTO 字段不变，生成 TS 无结构变化。无新增任意网络桥接或数据迁移。

### 2026-10-13 账户分组倍率查询（当前源码）

新增受限 `Bridge.ProviderRate(AccountQuery)`，只接受 `operation=effective-rate`，复用 client → core 查询及 revision、超时和安全整数校验。返回 `AccountRateObservationResult`，包含查询身份、Key、平台、时间、失败及 `rate={group,group_name,multiplier,basis}`；multiplier 为十进制字符串或 null，用户令牌与推理密钥不返回。remote_key_id=0 时核心验证所选保存的 Key 与平台账户对应关系；正整数保持指定远端 Key 的原有语义。未知分组可保留组名但倍率为 null；前端不推断 1。

账户页和模型映射页通过 AccountTransport.rate 调用，前端不访问平台 URL。共享样例 desktop/contract/provider-account.json 的 rate 同时由 Go DTO 漂移检查、前端 Model 与 Mock 使用；观察值不会写入模型映射或改变路由策略。


2026-10-13 模型调度新增受控 `ConfigureModelRouting(ModelRouting)`，返回 ModelRoutingResult 标准 envelope，复用客户端与核心管理鉴权、实例校验和 revision。输入仅已有模型的完整候选引用及规则；不提供任意请求／配置修改入口。保存纳入前端活动锁，阻止退出／安装更新期间并发写入。Go DTO和共享样例见供应商接口 §9；旧核心不支持此方法，需同步升级，schema6不向旧程序开放。

2026-10-13 sub2api Key 倍率兼容：`ProviderRate` 的 `remote_key_id=0` 优先使用推理 Key 的 `/v1/sub2api/billing`，无需 JWT；`basis=key_billing` 表示含个人覆盖和高峰规则的当前有效倍率，group/group_name 可为空。AccountRate DTO 字段不变；共享样例增加 key_billing_rate，目录 keys 同步接受 rate_basis=key_billing。旧站缺接口或显式远端 ID 沿用用户授权查询；当前源码本地 Key 自动核对结果会持久缓存，不写旧模型倍率字段；有效相对倍率为供应商倍率×Key分组倍率，见[倍率设计](provider-multipliers.md)。

2026-10-13 当前源码增加受限 `ModelCatalog`／`EstimateModelCost` 方法（见[参考目录契约](model-catalog.md)），沿用当前本机实例、管理权限和标准 Result 包装。方法是只读请求，失败不标记写入结果未知，不自动重放；核心负责有界公开抓取、缓存和估算，桌面不传推理凭据。新方法尚无界面入口，不影响核心独立运行生命周期。

## SSH 会话与设备切换（2026-10-14）

Connections 仅返回公开档案（name、transport、authentication、target、instance_id、active），包括内置 @local。ConnectSSH 输入安全连接字段和可选瞬时 password；UseConnection 输入名称与可选本次密码。SessionInfo 增加 profile_name、transport、target。规范 DTO 由 Go 生成 TS，共享样例为 desktop/contract/connections.json。

宿主复用 internal/connection 与 CLI 相同的身份／授权／连接策略。密钥模式使用受限 OpenSSH 的 stdio 转发（ssh -W），不开放临时本机管理端口；密码模式使用内置 SSH 通道，首次明确同意后保存公开主机密钥，变化即拒绝。固定 bootstrap 命令只允许已验证 executable／端口；默认在 SSH 非交互 PATH 中查找，缺失时依次检查该账户的 ~/.local/bin/anchor、~/bin/anchor 和系统常见路径，指定路径不回退并支持 ~/。在不加载登录脚本的 POSIX shell 内执行，先检查 bootstrap --help，密钥和密码模式共用此策略；管理令牌保存在受权限保护 SQLite 凭据表，密码不保存。认证错误只返回白名单 code，不暴露 stderr／令牌。程序缺失（remote_anchor_missing）、引导不支持（remote_anchor_unsupported）、管理端口不匹配（remote_control_port）、核心未就绪（remote_core_not_ready）分别展示；其余引导失败／响应无效保留 remote_anchor。旧远端未提供细分诊断时保留通用错误。

切换先核验目标 ready 并保存独立 desktop_context，再替换当前连接；失败不破坏已有连接，不改 CLI current_context。保存的密码连接在下次启动要求重新输入。关闭宿主只关闭本次 SSH，不停止目标核心；StopCore 明确停止当前选中目标，升级安装则只停止本机核心，即使当前 UI 连接远端。旧授权失效后重新 SSH 签发仍必须匹配已绑定实例。远端必须预先安装支持 context bootstrap 的 Anchor，自动安装尚未实现。


## 中文输入法桥接（2026-10-15，当前源码未打包）

Linux GUI 宿主在 Wails／GTK 初始化前检测 GNOME Wayland 会话。仅当 GTK_IM_MODULE 未显式配置、已安装 Fcitx5 GTK3 模块且当前 D-Bus 会话已有 org.fcitx.Fcitx5 所有者时，为当前宿主进程选择 GTK_IM_MODULE=fcitx，避免默认 GTK Wayland 模块无法连接独立 Fcitx5 的情况。检测使用现有 DBUS_SESSION_BUS_ADDRESS，限制查询时间，不自动拉起 D-Bus／输入法服务。模块缺失、探测失败、KDE／X11 或显式配置 ibus／simple／wayland 等情况保持原状；Windows WebView2 继续使用系统输入法。

此设置只作用于 GUI 启动，不用于核心或安装辅助入口，不修改用户配置、桌面会话环境、Agent 或系统输入法配置。隔离单元测试覆盖环境选择与保留；本机无窗口 GTK 探针确认默认 context 为 wayland、选择桥接后为 fcitx。前端组合态回归另见[前端规范](frontend.md#中文输入法组合态2026-10-15当前源码未打包)；尚未打包本轮修复，未替换当前运行程序，也未完成真实 WebKitGTK 候选输入或 Windows 原生输入法验收。

### DSH Desktop 独立托管增量（2026-10-15，当前源码未发布）

AgentWorkspace/EnrollAgent/ManageAgent DTO 形状不变，agent/adapter_id 字符串新增 `dsh-desktop`；旧 `dsh` 始终为 Web。核心返回相同 DSH home 下两个不同 target ID，前端按 adapter_id + 规范化配置根匹配绑定，不把两端合并。桌面固定展示 DSH Web、DSH Desktop 独立行；未发现也保留已登记端的恢复入口。生命周期、鉴权和写入继续复用核心，不新增任意路径/命令桥接。细节见 [Agent 管理层增量](agent-management-layer.md#2026-10-15-dsh-desktop-独立-agent当前源码未发布) 与 [CLI §8](../cli/commands.md#8-agent-托管与独立恢复)。模拟绑定验证不代表原生 DSH Desktop 已加载配置。

## 请求趋势只读桥接（2026-10-16，当前源码未打包）

**发行补记（2026-10-16）**：本节标题的“当前源码未打包”记录功能提交时状态；本功能现已随 [desktop.42 dev](../release/desktop.md#当前在线-dev-版-42) 发布，两平台安装包与清单已投放。未自动安装或替换运行程序，Linux WebKitGTK／Windows 原生窗口及安装验收仍未执行。

新增 `LogStats(LogStatsQuery) LogStatsResult`，复用当前已授权连接与标准 Result；DTO 别名由 logbook 规范源经 desktop/contract.go 纳入生成，TS 不手改，共享样例 `desktop/contract/log-stats.json`。方法只读、最多5秒，查询全部匹配保留请求，不采用列表游标或快照。核心 stats 查询最多3秒／90天／600桶；错误与旧核心无接口均返回 problem，不回退模拟数据、不新增上游请求或持久累计表。页面 Controller 负责15秒轮询、暂停、刷新、条件变化和卸载后的旧结果丢弃；切换实例沿用页面卸载和连接隔离。详细字段、指标分母和空值语义见[日志趋势设计](logging.md#9-请求趋势统计2026-10-16当前源码未打包)。本轮隔离 Go 调用链与模拟浏览器验收不等于 Linux／Windows 原生窗口验收，未更新已运行的安装程序或核心。


## 日志候选桥接（2026-10-16，当前源码未发布）

新增 `LogSuggestions(LogSuggestionsQuery) LogSuggestionsResult`，Go DTO 与生成 TS 同源，共享样例为 `desktop/contract/log-suggestions.json`。宿主校验输入并复用现有连接、owner 凭据与实例身份，调用 client 的只读 `/control/v1/logs/suggestions`，宿主5秒／核心3秒超时。仅暴露固定日志标识列、去重值与出现次数，不能从 JS 传 SQL、正文列或任意 URL。候选按时间范围查询所有保留行，不受页面游标及其他字段过滤限制。

这是 additive 方法，未改 `anchor-desktop-v1` 既有调用；旧宿主缺方法／旧核心不支持时前端明确显示候选不可用并保留手动筛选，不回退 Mock。前端按实例键卸载清理旧请求，中文输入及过期结果由局部 Controller 处理。接口、上限与排序见[日志设计](logging.md#10-筛选与搜索候选2026-10-16当前源码未发布)。未构建／安装／发布本轮桌面包，原生两平台未验收。
