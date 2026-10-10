# 桌面运行契约 v1

- 日期：2026-10-11；标识：`anchor-desktop-v1`。
- 本轮实施基线：现有 `internal/client`、核心实例锁、Provider revision 与 secret 三态；不改已有供应商业务规则。
- 状态：源码已实现，Go 隔离真实核心及浏览器模拟边界测试通过；不是发行或双平台验收证明。共享 Go DTO、TypeScript 生成文件及样例由本轮唯一实现者维护。

## 1. 宿主接口

规范源为 `internal/desktop/contract.go`，通过 `go run ./cmd/desktop-contract` 生成前端类型。Wails 只绑定 `Bridge` 的受限方法：Start、Status、Providers、SaveProvider、StopCore，以及在线更新增量 UpdateStatus、CheckUpdate、DownloadUpdate、InstallUpdate（详见[在线更新](online-updates.md)）。前端仅通过桌面传输适配器访问绑定，不能指定文件、命令、凭据或任意目标地址。

每个结果带 `contract=anchor-desktop-v1`、data 与 problem；problem 仅含白名单 code，不返回原始错误、路径、管理令牌或供应商秘密。列表始终为数组。返回 Provider revision 为 1..Number.MAX_SAFE_INTEGER；写入 expected_revision 为 0..MAX_SAFE_INTEGER-1，且等于输入 provider.revision，超范围拒绝。SaveProvider 的 secret=null 保留、空字符串清除、非空替换；只入不出。写入发生网络/响应异常时返回 unknown，不自动重试；409 为 conflict、认证失败为 unauthorized。

## 2. 本机生命周期

桌面二进制以默认用户状态目录或显式 `--data-dir` 运行；JS 不接受目录参数。Start 在有界超时内取得桌面启动协调锁：安全初始化缺失状态，核验本机身份/API/授权，就绪则复用；否则依据实例锁启动同一二进制的受限 `--core` 模式。两个监听面与托管协调就绪后才开放供应商页；核心 starting/stopping 时拒绝新写入／网关请求（starting 时仍可正常请求停服）。重复打开不重复启动核心；未知端口占用、身份不符与无效状态不得覆盖、强杀或回退 Mock。

子进程不使用窗口 context、标准输入输出管道或父进程死亡信号；Windows 隐藏控制台并创建独立进程组，Linux 使用独立 session。宿主关闭只取消本次查询并释放客户端，不停止核心。已有正常核心可以复用，不接管其外部服务管理器。连接来源对照已认证 instance 响应的 process_id 与本次子进程 PID，CLI 并发赢得实例锁时标为复用；旧核心无该字段同样保守复用。PID 仅用于来源标记，绝不替代身份／授权或用于强杀；Status 轮询刷新来源与停服能力。

Status 只查询，不启动／重启；核心退出后 UI 禁写，用户明确重新连接时才再次 Start。StopCore 须明确确认当前实例身份，通过带管理鉴权和 X-Anchor-Instance 的 POST /control/v1/shutdown 请求核心正常停服。返回 accepted 仅代表请求接受，不等于退出或恢复成功；UI 随后转为断开，不显示“恢复成功”。CLI 对应 `stop --approve`，无强杀和自动重试。

首次启动仅复用已批准的托管策略，不新增 Agent 授权。安装器沿用本契约，不在安装期间启动核心或登记服务；用户级开机自启动未实现，也不显示假开关，保持默认关闭。无图形环境继续使用 CLI。

## 3. 前端模式

Vite `prototype` 模式明确使用内存 Mock，仍可测试已有设计场景；正式桌面构建不包含 Mock 场景入口，不在绑定缺失／失败时回退模拟数据。真实界面移除“演示保存”等文案，展示连接状态、实例来源及关窗不停服提示。

### 3.1 自定义桌面顶栏（2026-10-11）

Windows / Linux 统一启用 Wails `Frameless`，以现有工作区 header 承担拖动和窗口控制，不改变五个业务 Bridge 方法或 DTO 契约。窗口 View → hook Controller → Model → `desktop/window-runtime.ts`，适配器只允许 `WindowMinimise`、`WindowToggleMaximise`、`WindowIsMaximised`、`Quit`；接口依据仓库锁定的 Wails v2.15.0 runtime 源码，不向页面暴露通用 runtime 或系统执行入口。

拖动使用 `--wails-draggable: drag`；交互元素及窗口按钮明确为 `no-drag`。双击仅处理拖动区域。状态查询有超时并在 resize / focus / 恢复可见时更新，组件卸载移除监听和计时器；缺失 runtime 显示失败而不回退模拟。浏览器 prototype 隐藏原生控件。

`Quit` 仅请求桌面宿主退出，仍遵守 §2 关窗不停服，不调用 StopCore 或强杀进程。最小化 / 最大化 / 还原是本机窗口展示行为，不增加 CLI 管理指令。本轮只修改源码与编译打包，原生 Windows / Linux 拖动、缩放及按钮行为不声明已验收。

## 4. 验收

- Go DTO/生成漂移、错误分类、秘密不回传、冲突、空列表及安全整数。
- 隔离目录真实核心：初始化、首次启动、并发复用、供应商增改、秘密清除、宿主关闭后仍服务、显式停服、端口冲突及错误凭据。
- Windows 源码编译不等于 Windows 原生窗口／后台进程验收；Linux 编译缺 GTK/WebKitGTK 时如实记录。
- 原型浏览器测试不替代真实 Wails 绑定及两端安装验收；不请求付费上游、不读取真实 Agent 配置。


## 5. 分轮验证与交付边界

此前宿主实现轮的 Go 全包 race/vet、Go→TS 漂移、共享样例以及真实隔离核心生命周期／供应商读写已通过；包括真正退出宿主进程后核心仍可服务、并发复用、丢失 owner.token 不重新创建凭据、认证与端口占用拒绝。前端 50 项单元、15 项原型浏览器和 6 项模拟 Wails 边界浏览器测试通过。

此前 Windows `.3` 内部 EXE 已交叉编译；当时 Linux 因宿主缺 GTK3/WebKitGTK4.1 开发依赖未产出。后续安装包轮新增受限 Ubuntu24.04 构建容器、Windows NSIS／Linux `.deb`，不改上述接口或核心生命周期；Linux 图标与应用入口在宿主中对应同一 ProgramName。该轮按用户要求不运行测试、安装、真实 GUI／核心或业务验收，构建结果单独记录，不能沿用旧测试作为新包验收。当前产物与剩余门禁见[桌面开发版构建](../release/desktop.md)。公开 alpha.6 分发不变。


## 5. 在线更新增量（2026-10-11）

新增 UpdateInfo/ConfirmUpdate/UpdateResult DTO 和 integrity/permission 错误码，保留既有 anchor-desktop-v1 供应商与生命周期方法语义。更新读取／下载与核心业务分别用锁，长下载不占用供应商／Status 的会话锁。安装必须明确批准版本，重新校验磁盘文件，认证本机核心并等待正常退出，再交给独立 helper 等待窗口退出；核心抢占、身份变化、文件被修改或不兼容均拒绝，不强杀或自行提权。普通关窗仍不停服。本轮仅实现源码和隔离验证，安装器交接及双平台原生更新验收不是单元／模拟浏览器测试可替代的结果。
