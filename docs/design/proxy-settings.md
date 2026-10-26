# 出站代理（proxy-v1）

独立于供应商协议适配。管理 API `/control/v1/proxies` 返回脱敏配置、供应商及 Key 的绑定和 mihomo 状态；`POST /control/v1/proxies/action` 为唯一写入入口，要求实例身份、明确批准、workspace revision。CLI 与桌面使用同一类型与校验。

## 选择规则

Key 覆盖 > 供应商默认；`direct` 不使用应用代理，`system` 显式使用核心所在主机的系统／环境代理，`proxy` 使用指定节点，Key `inherit` 继承供应商。供应商未绑定仍默认 `direct`，不自动读取环境代理。代理失效、节点无效或内核缺失报错，绝不降级直连。适用于推理、流式对话、测试、模型发现和供应商账户查询；公共模型元数据下载、连接 Anchor 的 SSH 隧道不使用供应商代理。


## 系统代理与真实直连的边界（2026-10-19，proxy-v1-system，当前源码未发布）

- 管理 API、CLI 和桌面共同接受 `system`；没有新增秘密字段、桥接方法或数据库 schema。`system` 不引用 profile，传 `proxy_id` 为非空会拒绝；仍要求 approve + workspace revision。共享样例为 `desktop/contract/proxies.json`。
- `system` 按请求读取核心进程的 `HTTPS_PROXY` / `HTTP_PROXY`，再取 `ALL_PROXY`，大写优先于小写；环境缺失时 Windows 读取当前用户系统代理／PAC。Linux 等其他平台当前只读核心进程环境，不自动发现 GNOME/KDE 桌面设置；可配置环境变量或添加显式 HTTP/SOCKS5 profile。桌面进程的环境不是远端核心环境，修改启动环境变量需重启核心。
- 环境配置不合法、没有代理、NO_PROXY／系统绕过列表／PAC DIRECT／loopback 绕过以及代理连接失败均返回 `proxy_unavailable`（推理 HTTP502），不发出直连请求、不切换直连候选。公共升级器复用解析器但保留原来的 bypass/DIRECT 规则，不受供应商 fail-closed 语义影响。
- `system`、`direct`、指定节点有独立连接池；切换后新请求按新设置分流，已经建立的 SSE 不因修改绑定而被主动切断。系统／环境代理每次请求重新解析，不使用 net/http 首次环境快照缓存；这是应用代理选择，不是修改外部代理软件的“全局／规则”模式。
- **`direct` 仅保证 Anchor 不使用应用代理，不能绕过系统 TUN、透明重定向、网关 SNAT 或路由器分流，更不能保证公网 IP。** 要实现物理直连，须在实际接管流量的系统／路由器配置针对 Anchor／目标的绕过规则；本轮不自动更改路由、防火墙、TUN 或全局代理模式。界面不再声称“强制直连”。
- 老核心不认识新 `system` 模式，需要配套升级；旧桌面严格校验枚举会拒绝读取含 `system` 的 workspace，须同步更新桌面。状态库 schema 未变；切回旧核心前先在新核心把 `system` 绑定改回旧模式。不把源码实现等同于已安装版本已生效。

验证已覆盖隔离 HTTPS 上游与 CONNECT 代理、环境变更、Key 直连覆盖及继承、NO_PROXY 命中和代理断开时返回 HTTP502 且不尝试直连候选。关闭测试代理监听后先清理空闲 CONNECT 隧道，避免将既有隧道上的成功请求误判为断开后回退。升级器保留共享代理错误哨兵，非法代理配置返回脱敏错误。定向测试、默认 Go 编译检查和 Windows 代理相关包交叉编译通过；全量桌面测试另有后台倍率查询与旧测试桩不匹配，内置代理生命周期测试出现过一次失败后独立三次通过，不将这些检查记为全量通过。未验收 Windows 系统代理／PAC 原生运行、桌面原生窗口或真实网络出口，未打包安装。

## 配置与秘密

HTTP、HTTPS 和 SOCKS5 以 URL 导入，支持认证；mihomo 导入单节点 YAML，或从 Clash YAML 的 `proxies` 列表按 `node_name` 选取一个节点。API 只返回 ID、名称、类型、revision；认证 URL/节点 YAML 单独存储，列表不回显。编辑时 secret 留空保持。只提取节点，不执行用户提供的监听器、规则、DNS、TUN、控制器或脚本。暂不包含订阅定时刷新、URI/Base64 订阅、测速或修改全局系统代理；`system` 只读取现有设置，不更改系统。

## 生命周期与分发

固定版本 mihomo v1.19.32，发布前下载并校验 Windows/Linux amd64 compatible 构件，嵌入对应平台二进制。源码 checkout 未准备 payload 时外部代理仍可用，mihomo 显示未内置。核心按需为使用中的节点启动独立子进程（最多 16 个活动/退役实例），仅监听 loopback 混合端口（默认随机，可指定）并启用随机认证；没有外部控制 API、不写系统代理。配置代际隔离连接池，旧代在响应 Body 关闭后退出，保证修改不会切断活动 SSE；核心退出统一回收。进程日志不转发（避免秘密泄漏）。配置和解压内核在实例私有目录。

删除在用的代理返回 referenced；供应商/Key 重命名、删除遵循数据库外键，复制供应商同时复制默认代理，复制 Key 时复制覆盖。列表和更改均不访问真实上游。Windows 本机进程/窗口和安装包仍需 Windows 原生验收；交叉编译不等价于原生验证。

## DTO

规范 Go DTO 位于 `internal/state/proxies.go`，桌面 `internal/desktop/contract.go` 使用类型别名，TypeScript 通过现有生成器生成。绑定模式 `direct|system|proxy`（供应商）和 `inherit|direct|system|proxy`（Key）。`ProxyAction.action` 为 `put|delete|bind`。`put` 的 `secret` 可选，`bind` 空 `key_id` 表示供应商默认。`ProxyWorkspace` 包含 `revision/profiles/bindings/targets/engine`。远端旧核心返回 unsupported，不用 Mock 代替。

## 升级与构建

初次代理实现将状态库从 schema8 自动迁移至 schema9（当前端口增量升级至 schema10）；请先备份并同步升级客户端与核心，旧核心不能再打开升级后的库。代理实现已独立提交；本地 `.48` 双平台安装包已构建并验证，尚未公开发布或替换本机程序。线上及本机 `.47` 均不包含此功能；仅重启 `.47` 不会出现入口。独立源码构建先运行 `python3 scripts/prepare-mihomo.py --target linux-amd64`（Windows 改为 `windows-amd64`）；已有桌面和 CLI 发行脚本会自动准备对应平台并校验固定 SHA256。

## 初次源码验证边界（2026-10-17 凌晨，未打包）

- Linux 固定 mihomo 构件已实际启动：回环 HTTP 节点转发、拒绝未认证请求、异常退出后重建和核心关闭回收均通过；另外覆盖外部 HTTP/SOCKS5、HTTPS CONNECT 和旧代连接池排空。
- 六个受影响 Go 包测试、代理相关 race 测试、Go/TypeScript 契约漂移检查、前端类型检查及全量 ESLint 通过；前端单元测试 298 项通过。
- 两项 Chromium 隔离桥接交互测试覆盖节点秘密不回显、保留秘密、Key 覆盖和结果未知后显式重读；这是浏览器中的真实前端与模拟桥接，不是原生 Wails 或真实远程核心验收。
- Windows AMD64 egress / desktop 测试程序仅交叉编译通过，未执行 Windows 原生进程与窗口测试；构建脚本回归 63 项，9 项原生 Windows 条件测试跳过。
- 没有构建发行安装包，没有调用真实付费供应商、改动真实 Agent 配置或设置系统代理。

## 桌面集成补交（本地 .48，未公开发布）

- 修复此前功能未提交、被干净源码打包排除的交付缺口；提交 `090a729` 排除同工作区的余额缓存改动。生产构建新增代理页面／桥接标识缺失即失败的回归门禁。
- 六个受影响 Go 包、Go/TS 契约、前端 typecheck／lint、310 项 Vitest 通过；Python 构建回归64项，9项原生 Windows 条件测试跳过。
- 从干净提交构建双平台 .48 包。精确生产 JS、固定 mihomo archive 已核实存在于两平台可执行文件中，Linux deb 实际载荷／许可及两平台包哈希通过核对。
- 同一份生产 dist 经单 worker Chromium 的2项隔离桥接交互测试通过，确认侧栏入口可点击且真实渲染代理页；不是原生窗口验收。详情与本地交付边界见[桌面发行说明](../release/desktop.md#本地代理集成包-48尚未公开发布安装)。
- 没有公开发布、替换本机二进制、停真实核心或迁移真实状态；Windows 原生、安装器及付费上游仍未验收。

## mihomo 监听端口（proxy-v1-ports，2026-10-27 已随 desktop.75 dev 发布）

- 复用 `put` 及既有管理鉴权／实例核验／workspace revision。`ProxyProfile.listen_port` 为可选整数：新建默认 0；同类型编辑省略／null 保留，显式 0 恢复自动分配，1..65535 固定端口。切换类型省略时归零；HTTP/SOCKS5 不接受非零值。同一核心内不同 profile 固定端口不得重复，非法值不写入、不增加 revision。CLI `proxy put --listen-port PORT` 和 GUI 编辑器共用此校验。
- schema9→10 事务新增非空 `listen_port`，默认 0，不改变既有节点秘密／绑定；新库同样为 schema10，旧二进制拒绝打开。列表始终返回配置端口；旧客户端省略字段仍保留已有端口。旧核心严格拒绝新增输入字段，需配套升级，不把旧版本的缺失字段显示为已知自动／未启动。
- `ProxyEngine.listeners` 为可选运行快照（没有存活节点时省略），每项只有 `profile_id/revision/port/retiring`。按 profile ID、revision 排序，包含仍在服务旧响应的退役代，不含认证、节点配置或 URL。读取不启动进程，手动重新读取刷新；运行端口不写库，自动分配在下次启动可能变化。子进程仅允许 `127.0.0.1`、随机认证，显示端口不赋予其他客户端免认证使用权。远程 Context 的端口位于远端核心主机。
- 启动前检查目标端口能否绑定；端口占用或启动失败仍返回既有 `proxy_unavailable`，不改随机端口、不强杀其他进程、不降级直连。配置写入不启动节点，因此保存成功不能当作端口可用或上游连通。
- 保留代际排空：同固定端口编辑时旧代有在途响应则保持监听，新请求在旧代关闭前失败；旧响应关闭后下次请求在固定端口启动新代。改成另一空闲端口／自动分配可允许新旧代并存。没有隐式重试，不中断旧 SSE。
- 前端仍由 Model 白名单校验和摘取公开字段，Controller 管理读写与冲突／unknown 锁定，View 仅管理草稿、显示配置值与运行地址。监听端口与节点 YAML 中的远端服务器端口分开；此设置不开放 LAN、系统代理或 mihomo 控制器。
- 规范源 `internal/state/proxies.go` 经桌面别名和现有生成器派生 TS；共享样例 `desktop/contract/proxies.json` 覆盖固定配置／运行端口及端口编辑。测试使用隔离状态与本机合成 HTTP 节点；原生窗口、安装和真实网络出口仍需单独验收。端口能力已随 desktop.75 dev 同版打包／发布，未安装或做原生窗口／安装器验收。
