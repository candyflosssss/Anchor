# Anchor CLI 指令变更历史

当前可用指令以 [最新指令大全](commands.md) 为准。本文件记录实际契约变化，按时间追加，不把未来计划写成已发生变更，不改写旧条目。

命令、别名、参数、默认值、作用域、输出和行为变化均应记录。每条包含日期、适用版本/未发布状态、旧→新差异、兼容与迁移方式及实际验证。纯内部重构或仅修正文档措辞且不改变 CLI 契约时无需新增虚假变更。

## 2026-10-10 — 建立当前 CLI 文档基线（0.1.0-dev，未发布）

- 类型：基线收录；本轮仅登记既有实现，没有新增、删除或改变可执行指令。
- 指令：help、version、init、serve、context add/use/list/inspect/test、status、provider add/put/list、model add/put/list、access grant/list/revoke，共 19 个规范路径。
- 参数：登记全局 data-dir/context/json；初始化监听地址；Context 的 local；供应商 id/base-url/protocol/timeout/max-concurrent/disabled/revision/secret-stdin；模型 id/candidate/display-name/tools/streaming/revision；授权 label/scope/ttl/show-once。
- 旧 → 新：原先指令分散在 help 和实施示例中；现在以 commands.md 汇总当前完整契约，明确参数默认值、完整替换更新、本机/Context 作用域、秘密输出与未实现能力。实现行为不变。
- 兼容与迁移：没有运行时破坏性变化，不需要数据或配置迁移；README/实施说明转而指向唯一最新大全。
- 验证：按 CLI 路由/参数解析、state/client 实现及 main 退出逻辑核对；本轮不启动核心、不调用真实上游、不重跑 Go 功能测试。skill 元数据、文档链接及命令覆盖检查结果见本轮任务更新记录。

## 2026-10-10 — 发行构建返回实际版本（源码 0.1.0-dev；预发行包使用注入版本）

- 类型：修改行为；没有新增或删除 anchor 命令/参数。
- 指令与参数：`anchor version`；发行工具独立的 `scripts/release.sh --version` 不属于产品 CLI 参数。
- 旧 → 新：原先版本为源码固定的 0.1.0-dev；现在保留源码默认值，但发行编译通过 ldflags 注入预发行版本，version 和管理实例接口使用一致的实际版本。
- 兼容与迁移：JSON 仍只有 version 字段，退出码和无需初始化的行为不变；依赖固定字面量 0.1.0-dev 的脚本应改为读取实际值，无数据迁移。
- 验证：新增无初始化 version 测试，包内程序烟测要求版本与发行参数一致；本轮实际执行结果见对应任务 release 记录。

## 2026-10-10 — 托管层与 Codex 全局配置 v1（0.1.0-dev，未发布）

- 类型：新增 / 修改。
- 指令与参数：新增 agent add/list/inspect/plan/apply/restore/enable/disable/token 和 recover；add 使用 id/adapter/codex-home/model/approve/anchor-executable，变更动作要求 revision，apply 另要求 fingerprint，recover 可按 id 限定；最新大全共 29 路径。
- 旧 → 新：之前不托管 Agent；现在显式批准 Binding 后，下次 serve 自动接管全局 Codex 配置，正常停止条件恢复，崩溃后可独立 recover。status 新增逐 Binding 状态，实际连接仍为 unknown。普通输出仍为 JSON，但 agent token 专供认证 helper，stdout 是原始秘密文本；不会进入普通查询、计划或日志。Responses 允许客户端执行的 custom 工具同协议透传（不支持服务端工具）。
- 兼容与迁移：schema 1 → 2 自动保留原有数据/身份，但旧二进制拒绝 schema 2，回退需先停止核心并使用升级前受保护的完整状态备份。已发行旧测试包不包含新命令；本轮未打包/发布。Codex 只支持全局配置，需工具/流式公开模型和稳定 helper 路径；不能把 go run 临时程序用于长期托管，未生成模型目录或验证真实付费供应商。普通 access grant 行为不变。
- 验证：新增隔离字段恢复/冲突/崩溃/脱敏、迁移/Binding 凭据、管理 API 和 CLI 测试；受 resource-guard 保护的真实 CLI 生命周期烟测通过，隔离 codex-cli 0.159.3 对回环模拟上游的 Responses 文本请求通过。最终测试命令与平台边界见本任务更新记录；不宣称 Windows 原生运行或真实工具执行验收。

## 2026-10-10 — Codex 基础配置模型目录与多模型授权（0.1.0-dev，未发布）

- 类型：新增 / 修改。
- 指令与参数：`model add/put` 增加成组参数 `--codex-catalog FILE --codex-model SOURCE --codex-version X.Y.Z`，从本地、版本匹配的完整目录导入指定模型元数据；`agent add` 增加 `--model-scope all-compatible|single`，默认 `all-compatible`；新增 `agent set-models ID --revision N --model DEFAULT --model-scope all-compatible|single`。最新大全共 30 个规范路径。
- 旧 → 新：原先 Binding 的 model 同时充当默认值和唯一授权模型；现在 default 与授权范围分离。all-compatible 动态授权工具/流式、同精确 Codex 版本有效元数据、至少一个 enabled 候选供应商的公开模型；默认模型只决定初始选择，不代表供应商健康或任意模型兼容。基础配置接管按同一规则生成绑定专有模型目录，公开 plan/fingerprint 包含目录与策略；恢复条件清理未改动且不再被基础配置/独立 profile 引用的自有目录。Responses 同协议透传补充仅含 function/custom 的 namespace 和 `tool_search execution=client`，仍拒绝服务端工具及嵌套 namespace。
- 兼容与迁移：旧 scope 省略的 Binding 仍为 single，不自动扩权。显式 `set-models` 先安全恢复旧接管，遇冲突不变更策略，成功后递增 revision、保留 token 并等待重新 plan/apply。`model put` 仍为完整替换，省略元数据参数将清除原元数据。目录是接管时快照，网关权限动态判断；模型配置更新后须 restore → plan → apply（或正常停服/启动）刷新目录，并重启远端 Codex app-server。只托管批准的 CODEX_HOME 基础配置，不生成/强制启用额外 profile；不自动获取目录、探测安装版本或推断真实上游能力。未打包/发布，不修改正在使用的 Agent 配置。
- 验证：最终全包 `go test -race -p 1 -count=1 -timeout=240s ./...` 通过；隔离 Codex CLI 0.159.3 和实际 app-server 的 model/list、默认/手动切换模型、set-models、停止/崩溃恢复与冲突场景使用回环模拟上游验收，具体最终结果见本任务更新记录。Windows 仅交叉编译；不宣称桌面 UI、真实 SSH 传输、真实付费供应商或完整工具执行已验收。

## 2026-10-10 — 一键安装入口（未推送、未公开发布）

- 类型：新增外部分发脚本；核心 CLI 仍为 30 个规范路径，没有新增 `anchor install/update`，不改变现有命令语义。
- 入口与参数：Linux `scripts/install.sh [--version VERSION] [--install-dir DIR] [--replace]`（支持 `-h/--help`）；Windows `scripts/install.ps1 [-Version VERSION] [-InstallDir DIR] [-Replace] [-Help]`。完整语法、默认值、失败路径与安全边界见 [安装说明](../release/install.md)。
- 旧 → 新：原来需手动下载/校验/放置程序；现在可自动识别 AMD64 平台、从公开 Release 下载一个程序、核对 SHA256 后安装到用户目录。默认选择 GitHub API 返回的最新创建公开 Release（含预发行），可固定版本。默认不覆盖已有程序，替换必须显式确认；非支持平台、非法参数、下载或校验失败均拒绝。
- 兼容与迁移：不修改 PATH 持久设置、Codex 或 Anchor 状态，不启动/停止服务；升级须先停服并备份状态，程序替换不保证 schema 回退。当前没有公开 Release，Actions/草稿不能匿名安装；远程命令需脚本推送及附件人工公开后才可用，发行许可门禁未改变。
- 验证：Linux 隔离离线安装回归通过；Windows 原生安装器回归配置为 Actions 门禁，本轮没有 Windows 环境，未执行原生 Windows 用例；临时 PowerShell 7（Linux）的原始语法/帮助及测试副本模拟安装回归通过（不等于 Windows 验收）。具体数量及结果见本轮任务记录，不宣称线上下载或真实平台核心运行通过。

## 2026-10-10 — AGPL 许可与 Linux-only 公开发行（0.1.0-alpha.3）

- 类型：发行范围及许可说明更新；核心 30 个命令路径和安装器参数/默认值未改变。
- 旧 → 新：项目发行许可待定 → AGPL-3.0-only；预发行 tag 的两平台未验收草稿 → 只包含已验证 Linux AMD64 的候选，并附对应源码（含 vendored 依赖）、许可和依赖声明。手动 CI 仍提供两平台内部评估包。
- 兼容与迁移：Linux 安装方式不变；Windows 首发不提供附件，安装会明确下载失败，不借用内部包。商业使用须遵守 AGPL；没有自动配置 Agent、启动服务或授权真实供应商。
- 验证：托管运行 38065332482 全部通过；正式公开后原始远程管道安装、校验、程序版本和隔离模拟上游烟测通过；对应源码包离线重建和烟测通过，详见本轮 release 任务记录。


## 2026-10-11 — 中文供应商 ID（开发修订，本地构建 0.1.0-dev.provider-unicode，未公开发布）

- 类型：修改。
- 指令与参数：`provider add/put --id` 支持中文等 Unicode 字母和十进制数字；`provider list` 原样输出；`model add/put --candidate PROVIDER:MODEL` 的供应商部分采用同一规则。总帮助、参数帮助和非法 ID 错误补充规则。
- 旧 → 新：供应商 ID 仅 ASCII 字母/数字及 `._-` → 1..128 个 Unicode 码点、首字符为 Unicode 字母或十进制数字，后续允许字母、十进制数字和 `._-`。保留对空白、控制/格式字符、组合附加符号、emoji、冒号、斜杠、反斜杠及 URL 保留字符的拒绝；精确匹配，不自动归一化。其他模型、Context、Agent 标识校验不变。
- 兼容与迁移：ASCII ID 无需迁移；数据库和输出结构不变，无需重新 init。`0.1.0-alpha.3` 不支持此能力，必须更新 CLI 并正常重启到新版核心；旧核心不会因为客户端升级自动获得支持。未替换本机已安装程序或停止现有服务，未修改真实供应商或 Agent 配置。没有新增交互式向导、删除、重命名命令。
- 验证：全包 `go test -p 1 -count=1 -timeout=180s ./...` 及同参数 `go test -race` 均通过；覆盖校验边界、SQLite 关闭/重开、中文 CLI 新增/更新/列表与模型映射、管理客户端及回环模拟 Responses 工具往返。新构建二进制的隔离启动、中文增改查、模型引用、非法 ID/重复 ID 拒绝、密钥脱敏和优雅停止烟测通过；没有调用真实付费上游。


## 2026-10-11 — 正常停服与桌面会话支持（源码 / desktop-dev，未公开发布）

- 类型：新增、行为修改。
- 指令与参数：新增 Context 作用域 `stop --approve`（确认默认 false）；共 31 个规范命令路径。更新 help；既有供应商指令及完整替换语义不变。
- 旧 → 新：原先仅凭信号停服；现在可经鉴权与实例校验请求正常退出，返回 accepted、instance_id 与 shutdown_complete=false，不把接受当成恢复完成。`serve` 仍前台，桌面宿主独立后台模式不改变它的行为。status 的 core/gateway 从固定 ready 改为真实 starting/ready/stopping，就绪前及停服中拒绝新写入／网关请求。
- 旧 → 新：管理客户端新增结构化 RequestError，保留状态／受限 code／Unknown；网络／无效响应错误文案归一化为 offline / invalid_response，写入网络异常、无效回包或 5xx 不重试。CLI 退出码及不回退目标不变。
- 旧 → 新：新版 instance API / context test 增加 process_id。桌面将已认证响应与本次启动子进程 PID 对照，防止 CLI 并发抢先启动后误标来源；不使用 PID 替代认证或强杀。旧核心缺字段时保守视为复用。
- 兼容与迁移：alpha.6 不含 stop 接口，旧核心会拒绝；须明确正常退出并升级核心，不会自动替换或强杀。status 消费方不能假定始终 ready；不得解析任意 stderr 文本为稳定错误 API。无需重建数据库、旋转凭据或修改 Agent 授权；本轮未实现登录自启动设置及系统服务命令。
- 验证：全包 `go test -race -p 1 -count=1 -timeout=120s ./...` 与 `go vet -p 2 ./...` 通过；覆盖停止确认、目标身份、认证／Origin、无生命周期／空 body／null 拒绝、starting/stopping 门禁、结果未知与禁止重放，以及隔离核心正常退出。Go 桌面集成另验证并发启动／复用、供应商保存和宿主进程退出后核心存活；不将其当作 Windows/Linux 原生 GUI 验收。

## 2026-10-11 — 本机在线更新与桌面安装交接（当前源码，未打包／发布）

- 类型：新增指令、参数与本机更新能力；没有删除或重命名既有指令。
- 指令与参数：新增 `update config`、`update configure [--channel stable|alpha|beta|rc|dev] [--manifest URL | --default-source]`、`update check`、`update download`、`update apply --approve`，最新大全共 36 个规范路径。check/download 接受本次 `--channel`／`--manifest` 覆盖和 `--product cli|desktop`；desktop 必须提供实际 `--current-version`，cli 不接受伪造当前版本。桌面宿主另外增加 `--update-manifest`／`--update-channel` 配置入口，不是 anchor CLI 子命令。
- 旧 → 新：之前依靠外部下载安装脚本、没有内置更新入口；现在无需 init 或核心在线即可读取／配置更新源、检查并下载，默认 alpha 渠道与公开 GitHub Releases，支持自建 HTTPS dev 清单。configure 持久化，check/download 覆盖不持久化；更新为本机 data-dir 作用域，拒绝显式非空 `--context`，不跟随默认远程 Context。所有操作仍输出 JSON；apply 返回 backup 路径。
- 旧 → 新：CLI apply 只更新当前真实可执行文件，必须显式批准且本机核心已停止，校验版本、平台、大小与 SHA256 后原位替换并保留一份 `.previous`；替换失败尝试恢复，不自动停服／启动。桌面设置页提供检查、下载与确认安装，确认后认证本机核心并正常停服、等待退出，独立助手再次核对确认版本和 SHA256 后交接 Windows NSIS／Linux 系统 `.deb` 安装器，不强杀或静默提权；installing 表示交接而非安装成功。
- 兼容与迁移：既有业务命令、数据库和 Agent 授权契约不变，无需重新 init。仅向更新目录写配置／缓存；不允许降级或同版本安装。默认源兼容已有 CLI release.json，但现有发行没有桌面 feed。旧 desktop.8 不含更新入口，第一次须人工安装包含本功能的新启动包；本轮未构建该包、上传清单、部署更新源或发布 Release。安装包尚未签名，SHA256 不是独立发布者认证；系统安装权限与数据／数据库回退不由本功能保证。
- 验证：隔离全包 `go test -race -p 1 -count=1 -timeout=120s ./...`、`go vet -p 1 ./...`、Windows 交叉 vet 通过，包含 TS 契约漂移、正常停服、CLI 替换与备份、损坏／未批准拒绝，以及窗口退出期间换版本／同版本换包拒绝。前端 typecheck、lint、61 项单元测试、10 项桌面模拟宿主和 15 项原型浏览器回归通过；浏览器使用已安装的 Chromium headless shell。Windows／Linux 原生 GUI 和安装器尚未验收，不将模拟验证当作线上升级成功。

## 2026-10-11 — 上游模型目录查询（desktop.10 / 未公开发布）
- 类型：新增。
- 指令与参数：`provider models --id ID --revision N`，两项必填；支持已有中文供应商 ID，使用所选管理 Context。
- 旧 → 新：此前只能列供应商和 Anchor 模型映射；现在可手动读取已存地址的 `/models`，返回模型 ID、快照 revision、HTTP 状态、毫秒耗时和固定失败类型。无推理调用／写配置／导入映射；目录失败退出0并以 failure 表达，管理层错误非零退出。
- 兼容与迁移：增量新增，不改既有 list/add/put 或 model 命令；旧核心不支持，须明确正常停服后升级。不要把目录成功当推理能力或健康证明。
- 验证：隔离核心 + 回环模拟上游的 CLI 成功／中文 ID／revision 冲突／缺参数／未知参数与只读检查；核心目录格式、错误脱敏、重定向、并发与超时测试，Go 桥接真实调用链测试。具体最终运行结果见本轮打包记录。


## 2026-10-11 — 供应商第一优先工作台（desktop.11 / 未公开发布）

- 类型：新增管理指令与外部行为，未删除/重命名既有命令。新增 provider workspace/delete/rename/copy/key-put/key-delete/key-reset/strategy/test/model-put/model-delete/price-put/price-import 与 usage；完整参数、默认值、输出见最新大全 §5。
- 旧 → 新：单 Key 变为最多64个命名 Key，旧 secret 自动迁移 default；基础 put 的 secret 只影响 default。新增删除/改名/复制与引用保护、模型删除/工作台映射、固定最小推理测试、多 Key 状态与策略、持久用量、单价估算和 models.dev 预览。写入核对 revision，破坏性动作/测试须 approve；秘密仍只经 stdin，不回显。
- 旧 → 新：provider --protocol 接受 responses/chat-completions/anthropic；网关有三入口，转换无状态文本、函数工具和 SSE，用量缺失标未知。401/403 跳过失效 Key，429 Retry-After 冷却，连续失败熔断/单次半开；明确未建连才允许网络失败切换，已发送结果未知和输出后不重放。status.upstream_health 从 not_probed 变为 per_key，详情看工作台。
- 兼容：旧核心不支持新增动作，需正常停服并升级；schema2 自动升级3，旧二进制不能打开新 schema，无数据库降级。历史用量保留旧供应商 ID；价格估算按当前价格重算已知部分，不是账单。没有 Claude Code 配置托管适配器或全量协议兼容承诺。
- 验证：状态迁移/引用保护、多 Key/冷却/半开、三协议 JSON/SSE、并行工具及不支持事件、真实隔离桌面桥接与生成契约的定向 Go 检查通过；前端类型检查与55项单元测试通过。未运行全量/race/浏览器/原生安装，未调用付费上游或线上 models.dev。打包结果见本轮 release 记录。

## 2026-10-11 — 桌面启动入口单实例（源码已实现，desktop.15 候选）

- 类型：行为修改；无新增、删除或重命名命令。
- 指令与参数：`anchor-desktop [--data-dir DIR] [--update-channel CHANNEL] [--update-manifest HTTPS_URL]`。
- 旧 → 新：原来重复启动可创建多个 GUI／托盘，且先写启动参数指定的更新配置；现在当前用户只有一个 GUI，重复启动只恢复已有窗口并退出0，不切换其数据目录或修改更新源／渠道，已有宿主未响应时最多等待5秒后退出1。核心模式和安装 helper 不参与 GUI 锁。
- 兼容与迁移：参数语法不变；需要切换 GUI 目录或通过启动参数设置更新源时先「退出界面」再打开。CLI `anchor serve` 的数据目录／端口和核心实例锁行为保持原语义。
- 验证：定向 Go 隔离测试通过，覆盖并发与跨进程重复启动、启动中恢复、退出拒绝、崩溃后重开及通知鉴权；双平台原生窗口验收未执行。

## 2026-10-11 — 修复推理测试误拒绝 Responses 推理项（源码未发布）

- 类型：兼容性修复；无命令、参数或退出码变更。
- 指令与参数：`anchor provider test --id ID --revision N --model UPSTREAM [--target KEY] --approve`，桌面测试共享同一核心逻辑。
- 旧 → 新：HTTP 200 的 Responses 输出包含 `reasoning` 时，旧测试因复用跨协议转换校验而返回 `invalid_response`；现在测试接受此推理项，并保留用量和 Key 健康记录。固定16 token可能在生成可见答案前耗尽，只有推理内容的 `incomplete` 响应也可通过最小连通测试。
- 兼容与迁移：无需修改配置，需升级核心后重新测试；请求额度和次数保持不变。成功不保证完整答案或跨协议推理兼容；网关跨协议转换仍拒绝推理项，无效 JSON、缺 ID、上游失败及其他不支持输出仍失败。
- 验证：修复前模拟上游复现 completed / token-limited 两种 HTTP 200 误判；修复后 protocol/core 定向测试通过，覆盖 Key/用量记录、失败保留及转换限制。未调用真实付费上游，未打包或发布桌面版本。
