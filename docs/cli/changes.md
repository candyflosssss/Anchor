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

## 2026-10-11 — 桌面启动入口单实例（desktop.15 dev 已发布）

- 类型：行为修改；无新增、删除或重命名命令。
- 指令与参数：`anchor-desktop [--data-dir DIR] [--update-channel CHANNEL] [--update-manifest HTTPS_URL]`。
- 旧 → 新：原来重复启动可创建多个 GUI／托盘，且先写启动参数指定的更新配置；现在当前用户只有一个 GUI，重复启动只恢复已有窗口并退出0，不切换其数据目录或修改更新源／渠道，已有宿主未响应时最多等待5秒后退出1。核心模式和安装 helper 不参与 GUI 锁。
- 兼容与迁移：参数语法不变；需要切换 GUI 目录或通过启动参数设置更新源时先「退出界面」再打开。CLI `anchor serve` 的数据目录／端口和核心实例锁行为保持原语义。
- 验证：定向 Go 隔离测试通过，覆盖并发与跨进程重复启动、启动中恢复、退出拒绝、崩溃后重开及通知鉴权；双平台编译打包与默认源真实更新下载通过，原生窗口验收未执行。

## 2026-10-11 — 修复推理测试误拒绝 Responses 推理项（desktop.16 dev 已发布）

- 类型：兼容性修复；无命令、参数或退出码变更。
- 指令与参数：`anchor provider test --id ID --revision N --model UPSTREAM [--target KEY] --approve`，桌面测试共享同一核心逻辑。
- 旧 → 新：HTTP 200 的 Responses 输出包含 `reasoning` 时，旧测试因复用跨协议转换校验而返回 `invalid_response`；现在测试接受此推理项，并保留用量和 Key 健康记录。固定16 token可能在生成可见答案前耗尽，只有推理内容的 `incomplete` 响应也可通过最小连通测试。
- 兼容与迁移：无需修改配置，需升级核心后重新测试；请求额度和次数保持不变。成功不保证完整答案或跨协议推理兼容；网关跨协议转换仍拒绝推理项，无效 JSON、缺 ID、上游失败及其他不支持输出仍失败。
- 验证：修复前模拟上游复现 completed / token-limited 两种 HTTP 200 误判；修复后 protocol/core 定向测试通过，覆盖 Key/用量记录、失败保留及转换限制。未调用真实付费上游；修复时未打包，后随 desktop.16 dev 发布，附件与下载验证见桌面发行记录。


## 2026-10-11 — Key 默认协议与实际模型覆盖（desktop.16 dev 已发布）

- 类型：新增 / 修改。
- 指令与参数：`provider key-put` 新增 `--protocol responses|chat-completions|anthropic`；新增 `provider key-model-protocol --id ID --revision N --target KEY --model UPSTREAM [--protocol PROTOCOL|inherit]`；`provider models` 新增 `--key KEY`，查询结果增加 `key_id`；`provider workspace` 的 Key 状态增加 `protocol`、`model_protocols`。
- 旧 → 新：协议原先由供应商统一设置；现在先选 Key，再以该 Key 下实际模型名的覆盖为准，无覆盖时动态继承 Key 默认值。已有 Key 的 key-put 省略/空协议保留默认值，新命名 Key 默认 responses；已有 Key 可省略秘密仅改协议。key-model-protocol 省略/空协议或 inherit 删除覆盖，恢复继承。默认协议与模型覆盖写入均增加供应商 revision，不改变公开模型映射。目录查询可明确指定停用 Key，认证采用该 Key 默认协议；省略 Key 仍选首个启用 Key，无 Key 才匿名，不影响健康状态。
- 兼容与迁移：schema3→4 自动将供应商旧协议复制给全部既存 Key，保留原配置与凭据。provider add/put 的 protocol 只初始化新 default Key 或控制匿名请求，不改变已有 Key；复制凭据一并复制协议及覆盖，改名/删除按外键级联。客户端与核心须同步升级；旧二进制不能打开 schema4，无数据库降级入口。已随 desktop.16 dev 打包发布，安装包能力以发行记录为准。
- 验证：隔离状态与回环模拟上游的 state/core/client/cli/desktop/protocol 定向 Go 测试通过，覆盖三协议 3×3 转换、换 Key 重试、目录认证、SSE、迁移与 revision 冲突；前端85项单元测试通过，类型检查与 lint 通过；缓存 Chromium 下原型4项及模拟桌面目录2项交互测试通过。系统 Chrome 首次启动 SIGTRAP，未进入页面；目录旧文案断言同步后复验通过。未请求真实付费上游，未改真实 Agent 配置，未验收原生安装与窗口行为。


## 2026-10-12 — 真实请求与系统日志（desktop.16 dev 已发布）

- 类型：新增管理入口和受控日志行为。
- 指令与参数：新增 logs list/detail/follow/export/settings/configure；完整过滤/游标/保留参数见最新大全 §12。help 同步 JSONL 导出与页快照输出例外。
- 旧 → 新：之前只有虚拟 UI；现在核心非阻塞采集、安全独立持久化，GUI/CLI 共用查询、详情、JSONL 导出和保留配置。列偏好只改变显示。follow 每2秒输出重复页快照，不是事件订阅；无清空命令。
- 兼容与迁移：日志本身独立 logs.db，不改变 state schema；同版供应商协议配置会迁移 state schema3→4，旧二进制不兼容；旧核心需正常停服并升级，不补采升级前历史。缺失 HTTP/usage/指标保持 null，未知不补零；任一尝试缺失 usage 时根总量未知。默认7天/50,000条/请求256MiB/系统32MiB，清理渐进收敛。alpha.6 和 desktop.15 没有日志接口。
- 验证：隔离 Go 核心/client/CLI/desktop 定向测试通过；真实桥接到隔离子核心及共享日志样例通过。10万条持久历史/索引查询/收紧保留验证通过；浏览器日志3项及桌面13项分别通过，800次／8客户端流式无日志丢弃；两平台打包、六项附件校验与真实 dev 更新下载通过；原生安装未验收，完整结果见本次 release 记录。

## 2026-10-12 — 供应商主页与多请求地址（0.1.0-dev.desktop.17 已发布）

- 类型：新增 / 修改。
- 指令与参数：`provider add/put` 的 `--base-url` 改为可重复（1..16 个，不可重复）；新增可选 `--homepage-url`；`--protocol` 默认值由 `responses` 改为空。
- 旧 → 新：供应商原先只有单个 API 地址；现在区分主页（仅展示）与请求地址，多个请求地址按请求轮询，模型目录和测试使用第一个。`--protocol` 留空时保留已存值，新供应商按 `responses`，仍只初始化新 default Key 与匿名请求。put 为完整替换，省略主页会清除，地址列表整体替换。桌面供应商页不再显示或设置协议，新增供应商不填 Key，取消／关闭／Escape 直接丢弃草稿。
- 兼容与迁移：无 schema 变更；只有 `base_url` 的旧数据照常读取，`base_url` 继续镜像第一个地址。桌面 DTO 移除 `protocol`、`base_url`，新增 `homepage_url`、`base_urls`，属于不兼容变更，桌面与核心须同步升级。
- 验证：state/core/desktop/client/cli 定向 Go 测试与 go vet 通过；前端 tsc、eslint、vitest 通过；缓存 headless Chromium 下供应商／恢复23项与桌面／日志16项 Playwright 通过。未打包，未验收原生安装，未请求真实付费上游。

## 2026-10-12 — 更新检查与下载支持 Windows 系统代理（0.1.0-dev.desktop.17 已发布）

- 类型：兼容性修复；命令、参数、输出和退出码不变。
- 指令与参数：`anchor update check|download`，以及桌面「在线更新」共享的同一更新器。
- 旧 → 新：原先只认代理环境变量，Windows 上只开了系统代理（如代理软件的系统代理模式）时，浏览器能访问 GitHub，更新却会失败。现在环境变量仍优先；都没设置时读取当前用户的 Windows 系统代理及绕过列表。
- 兼容与迁移：无配置变更；不执行 PAC 脚本，只用 PAC 时仍需设置 `HTTPS_PROXY`。已安装的旧版本不受影响，需要先手动安装含此修复的新包。
- 验证：updater/desktop 定向 Go 测试通过，覆盖系统代理解析、绕过规则、回环直连及环境变量优先；Windows 交叉 `go vet` 通过。未在原生 Windows 上验证。

## 2026-10-12 — 模型目录兼容 NewAPI 部署前缀与官方根地址（desktop.18 dev 已发布，CLI 为当前源码）

- 类型：修改。
- 指令与参数：`provider models --id ID --revision N [--key KEY]`；参数、七字段输出、管理鉴权、revision 与只读语义不变。
- 旧 → 新：未带版本的自定义部署前缀原先只读取 `<base>/models`；现在仅404/405或200 HTML网页时尝试同前缀 `<base>/v1/models`，兼容 NewAPI 反向代理子路径。`/v1`、`/v2`、`/v1beta1` 等显式版本及版本后的 `/openai` 不再追加 `/v1`。OpenAI、Anthropic、Kimi/Moonshot 与硅基流动的官方根地址直接读取 `/v1/models`，不先试 `/models`；保留 DeepSeek、OpenRouter、Gemini 既有目录规则。
- 兼容与迁移：无需修改已存 Key 或映射；站点令牌只放请求头，目录认证继续采用所选 Key 默认协议。认证失败、429、5xx、重定向和损坏 JSON 均不触发路径切换。目录列表只代表站点对当前 Key 返回的模型，不证明推理可用。
- 验证：先用隔离 NewAPI 模拟站复现部署前缀失败，再修复并通过 `go test -p 1 ./internal/catalog ./internal/core ./internal/client ./internal/desktop ./internal/cli`；供应商前端三个单元测试文件共57项与 `npm run typecheck` 通过。没有使用真实站点凭据或请求真实推理；原生桌面未验收。


## 2026-10-12 — 同名公开模型跨供应商映射与倍率调度（已随 desktop.19 dev 交付）

- 类型：新增参数 / 修改行为。
- 指令与参数：`provider model-put` 新增 `--multiplier`（0.01..1000，省略/0保留旧值或新候选取1），支持 `--strategy round-robin|priority` 设置公开模型策略；`--model-revision 0` 从只创建改为创建或将新供应商加入已有别名。`workspace.models` 新增 multiplier/strategy。
- 旧 → 新：跨供应商同名新增不再因模型已存在冲突；追加保留已有候选、能力和元数据。model-delete 从删除整个公开模型改为只移除当前供应商候选及价格；只有最后一个候选才触发 Binding 引用保护与公开模型删除。
- 调度：新公开模型默认按倍率倒数加权轮询，旧模型未设 strategy 时仍按候选顺序。编辑时省略 strategy 保留原值；加入已有模型保留它的策略和能力。调度跳过停用、认证失效、冷却、半开已占用或并发已满的上游，失败切换及禁止重放边界沿用原规则。倍率是手动相对配置，不自动读取站点分组价格。
- 兼容与迁移：模型 JSON 增量字段，无 schema 迁移。重复添加当前供应商或过期编辑仍409 conflict。`model put` 仍完整替换并恢复默认 priority/倍率1。旧核心不支持新 Workspace 字段，桌面与核心应一起升级。
- 验证：state/core/desktop/CLI/client 定向 Go 测试通过；CLI 与真实管理接口、桌面 Bridge 隔离读写通过；前端类型检查、78项定向单元测试及 lint 通过。原生安装未验收。

## 2026-10-12 — 更新网络代理与默认源回退（desktop.20 dev 已交付；CLI 源码）

- 类型：修改对外网络行为；命令、参数和输出字段不变。
- 指令：update check/download，共享桌面检查和下载实现。
- 旧 → 新：每请求实时读取对应协议的环境代理和 ALL_PROXY，NO_PROXY 同时适用于系统代理；HTTP-only 环境不抑制 HTTPS 系统代理。Windows 用 WinHTTP 支持手动/PAC，自动发现等待有界。默认源 API 错误或10秒超时后尝试同仓库官方 Atom feed，同渠道按SemVer选清单，不使用第三方源；无效元数据不回退。
- 兼容：自定义源、渠道、TLS与字节/SHA256校验不变，旧配置不需要迁移。feed缺少当前平台或兼容候选时报告失败，不误报无更新。
- 验证：隔离真实TLS/CONNECT覆盖版本API、清单、安装包及跨域签名重定向；环境即时修改、优先级、绕过、官方feed限流/网络失败与非法元数据定向测试；最终结果见本轮记录，Windows PAC原生运行未验收。


## 2026-10-12 — 本机 Agent 发现与 Codex 目录托管（当前源码，未发布安装包）

- 类型：新增命令 / 扩展托管行为与输出。
- 指令与参数：新增 `agent discover`、`agent enroll --target ID --model MODEL [--revision N] [--template ALIAS:SOURCE ...] --approve`、`agent sync ID --revision N`。enroll 的 revision 默认0表示新目标，已有目标须提供当前 revision；重复 template 选择按公开别名关联本地目录模板，`ALIAS:` 明确排除该别名，重复别名被拒绝。
- 旧 → 新：过去须手工指定目录，并在共享模型上配置 Codex 元数据；现在核心只读识别当前用户的已知 Agent 配置目录，Codex 可从本地 models_cache.json 导入完整且有版本的模板，由用户确认后登记并应用。模板快照和选择属于托管目标，不改供应商模型映射。启动核心时应用已授权目标并刷新过期目录；运行中映射变化用 sync 条件恢复并重新生成目录，外部配置修改会保留并报告冲突。
- 输出：discover/enroll 返回目标、可选模型模板、公开映射及托管状态；目录存在只表示配置证据，不证明程序已安装或运行。已有 list/add/inspect/set-models/enable/disable 的 Binding 输出不返回完整模板快照，只保留选择策略；其他字段语义保留。
- 兼容与迁移：保留原 agent add 及单模型托管，其他已知 Agent 目录只识别、不托管。桌面、CLI 和核心需升级到本轮源码；desktop.20 安装包不含新页面。Codex 缓存缺失、版本无效或元数据不完整时不猜测能力、不自动请求上游；先由 Codex 生成有效缓存。缓存版本或模板选择变化请重新 enroll，应用配置后完全退出并重启 Codex。
- 验证：全量 `GOMAXPROCS=2 go test -p 1 ./...` 通过，覆盖隔离真实核心/CLI/桌面桥接、凭据助手、启动恢复和冲突保护；前端 typecheck、lint、103项单元测试通过；2项托管页面浏览器 Mock 边界测试通过。Windows desktop/managed/platform/desktop 包交叉编译通过（未执行 Windows 二进制）；未修改个人 Agent 配置、未请求真实付费上游、未生成安装包。

## 2026-10-13 — 平台账户与请求计费兼容查询（当前源码，未发布）

- 类型：新增。
- 指令与参数：`provider platform --id`、`platform-put --id --revision --platform [--site-url] [--user-id] [--user-token-stdin | --clear-user-token]`；`balance`、`key-quota`、`usage-records`、`effective-rate` 均接受供应商 ID、revision、可选本地 Key／站点数字 Key ID；账单另有 scope、page、page-size。模型目录复用现有 `provider models`，核心通用 query 也支持 models。
- 旧 → 新：本地用量／手动价格之外，新增 sub2api、NewAPI、原版 OneAPI 的远端只读查询；显式区分账户钱包、Key 额度、USD／quota、近期／分页账单，保留扣费、token、缓存及已知数值计费明细；不提供不可靠的倍率或 Key 过滤结果，不自动写入路由。NewAPI Key额度正确兼容 `code:true`，用户接口和推理 Key 分别认证。
- 兼容与迁移：SQLite schema4 自动升级到5，旧程序不能打开，须同步升级核心和客户端；无须重建供应商或 Key。新增平台配置独立于推理协议，用户授权仅 stdin 输入、输出脱敏；目标改变清除旧授权，复制不复制用户授权。查询成功或分类失败退出0且需检查 failure，管理错误仍非零。当前仅核心／Go客户端／CLI，未接入桌面查询页、未发布安装包。
- 验证：经 resource-guard 运行 `GOMAXPROCS=2 go test -p 1 -timeout 120s ./...` 全部通过；随后补充工具附加费数组、计费来源和订阅数值字段并复测 providerapi/state/core/client/cli。覆盖模拟站点真实响应封装、部署前缀、认证头、金额精度、分页差异、token缺失／缓存别名／阶梯计费、敏感字段过滤、请求边界、revision／实例／授权、CLI及客户端完整链路、schema4→5及凭据清除／复制／改名／删除。未使用真实站点授权或请求付费上游，未验收原生桌面。


## 2026-10-13 — 修复推理测试文本元数据误判并补充失败诊断（当前源码，未发布）

- 类型：修改测试响应校验 / 扩展日志结构诊断。
- 指令与参数：`provider test`、`logs detail REQUEST_ID`、`logs export`；语法、测试请求额度、`probe.failure` 和退出码不变。
- 旧 → 新：测试复用转换用的文本解析，Responses 文本项带 `logprobs`（即使空数组）或非空 `annotations` 时会将 HTTP 200 / 有效 JSON 判为 `invalid_response`；现在测试接受这些文本元数据，仍检查文本类型和元数据容器。跨协议转换规则保持不变。失败详情/导出新增 `evidence.validation_reason` 固定码，区分缺少 ID、无效 JSON、上游错误、内容/输出不支持、工具调用不完整、choices 不符合要求或拒绝回复；不保存原始错误文本或上游未知字段名。
- 兼容与迁移：兼容新增 evidence 项，历史记录不补写；需要升级运行中的核心才能使用，当前安装包尚未包含此修复。成功仍只证明本次最小请求可执行。
- 验证：基于 `bc9e2bb` 加本任务改动的隔离检出，经 resource-guard 运行 `GOMAXPROCS=2 go test -p 1 -timeout 120s ./internal/protocol ./internal/core ./internal/client ./internal/cli` 全部通过；覆盖文本元数据、16 token 推理项、固定失败原因、用量/Key 状态与日志隐私。共享工作区存在其他任务进行中的供应商接口改动，首次核心编译失败，因此隔离验证；未请求真实付费上游、未生成安装包。


## 2026-10-13 — Codex 缓存可选字段兼容与发现诊断（当前源码，未发布）

- 类型：兼容性修复 / 扩展状态值；命令、参数和退出码不变。
- 指令与参数：`agent discover`、`agent enroll`，以及 `model add/put --codex-catalog FILE --codex-model SOURCE --codex-version X.Y.Z` 共用元数据校验。
- 旧 → 新：旧校验要求已不在当前 Codex ModelInfo 中序列化的 supports_parallel_tool_calls，且拒绝可选 context_window 缺省/null，导致有效缓存不能登记。现在允许这些合法缺省，原样保留，不猜测能力；非法类型/数值、无指令模板及其他不完整必需字段仍拒绝。发现不再把所有缓存问题归为 catalog_unavailable，新增 missing/unreadable/unsafe/invalid/version_invalid/empty/too_large/metadata_invalid/duplicate 分类，前端不再统一要求重复启动。
- 兼容与迁移：target.status 仍为 string，新增状态值，旧 UI 可显示原状态；客户端和核心一起升级可获得中文诊断。旧元数据无需迁移。确认新版安装后重新识别；已存在有效缓存时无需重复重开来修正校验。未取得用户 Windows 缓存，未宣称该机问题已实测消失。
- 验证：执行结果见本轮任务记录。隔离回归先复现合法条目被拒绝，再验证修复；未改用户 Agent 配置或请求付费上游；本轮未生成安装包。

## 2026-10-13：供应商平台自动识别与默认 Key 余额（源码未发布）

- 新增 `provider detect-platform --id ID --revision N [--key KEY]` 和 `provider balance-summary --id ID --revision N [--key KEY]`；复用只读管理查询，支持平台分类失败。
- `provider platform-put --platform` 改为可选，默认 auto；未配置平台的查询也自动识别，可继续指定 sub2api/newapi/oneapi 覆盖。公开探测无凭据，不依赖站点名称或域名。
- 未指定 Key 的平台查询优先已启用 default，再回退首个启用 Key；显式 Key 保持原选择行为。summary 只用推理 Key，区分钱包和 Key 原始额度，不使用用户授权、不改 revision。
- 桌面供应商列表复用 ProviderAllowance balance-summary；既有 balance/key-quota/账单/倍率语义不变。安装包未构建或发布。


## 2026-10-13 — 全 Key 模型目录合并与分组倍率（当前源码，未打包）

- 类型：修改默认选择行为，扩展目录 JSON；命令路径和退出码不变。
- 指令与参数：`provider models --id ID --revision N [--key KEY]`；省略 `--key` 从首个启用 Key 改为查询全部已保存 Key（含停用 Key），显式指定仍只查询该 Key；没有 Key 时仍使用匿名请求。
- 输出与行为：相同模型 ID 合并，保留 `models` 并新增 `entries` 的模型／Key 关联、`keys` 的逐 Key 目录状态与分组倍率、`partial` 部分失败标记；聚合 `key_id` 为空，有成功 Key 时保留目录结果，失败 Key 单独说明。倍率为十进制字符串或 null，不假定1×，不修改映射倍率、配置或 revision。
- 倍率来源：通过已存账户授权精确匹配推理 Key，NewAPI 掩码候选须通过固定只读接口验证；Sub2API 展示组名及个人覆盖或组默认倍率。缺授权、不支持、未知分组或读取失败单独返回 `rate_failure`，保留已取得的分组信息。所有 Key 共用目录字节预算、并发槽及超时，限制模型和 Key 数量，过滤凭据回显。
- 桌面：取消目录查询 Key 选择器；默认全部查询、每个模型一行展示对应 Key 与抓取倍率，每个模型／Key 的协议覆盖可独立编辑；同步 Go／TypeScript 契约、共享样例、CLI 帮助及最新指令大全。
- 兼容与迁移：无数据库迁移；核心与客户端需同步升级才能使用扩展契约及默认聚合语义。当前只交付源码，尚未生成安装包或投放在线更新；目录成功不代表推理可用。
- 验证：resource-guard 下 `GOMAXPROCS=2 go test -p 2 ./internal/core ./internal/providerapi ./internal/desktop ./internal/cli ./internal/catalog` 五包通过；前端 typecheck、lint 和28项目录单测通过；单 worker 的原型及模拟桌面浏览器共4项通过，覆盖合并、逐 Key 协议、成功及失败展示。未请求真实付费上游，未执行原生安装验收。

## 2026-10-13 — 模型目录跳过停用 Key（当前源码，未打包／发布）

- 类型：修改查询行为与固定失败分类，没有新增或删除命令／参数。
- 指令与参数：`provider models --id ID --revision N [--key KEY]`，桌面模型目录复用相同核心规则。
- 旧 → 新：省略 `--key` 从遍历全部已存 Key 改为仅遍历启用 Key；同名模型仍合并，并列出全部启用且目录成功的可选 Key 及其抓取分组倍率。指定停用 Key 或全部 Key 停用时返回 `failure=no_enabled_keys`、`http_status=0` 和空 models/entries/keys，不发起目录或倍率请求，不回退匿名；只有没有任何已存 Key 才匿名。
- 兼容与迁移：输出字段、管理错误和退出码保持原契约，新增固定分类 `no_enabled_keys`，目录层仍输出 JSON 并退出0。需要查询某个停用 Key 的目录时先明确启用该 Key；脚本须处理新增分类。无需数据库迁移，客户端与核心应同步升级。本轮未打包／发布安装包。
- 验证：资源保护下 `go test -p 1 -count=1 -timeout=120s ./internal/core ./internal/cli ./internal/desktop` 通过，覆盖默认跳过停用 Key、显式停用、全部停用、匿名边界、同名合并及分组倍率；前端 typecheck、lint、30 项目录单元测试和1项单 worker 浏览器定向回归通过。使用隔离状态与模拟上游，未请求真实供应商；浏览器回归不代替原生窗口验收。


## 2026-10-13 — Agent 托管复用已有模型映射（当前源码，未发布）

- 类型：修改（只读输出追加、桌面交互简化）；CLI 命令与参数保持兼容。
- 指令与参数：`agent discover` / `agent enroll` 返回的 workspace.models 增加 `available_targets: string[]`，总帮助补充映射可用性说明。`enroll` 的 `--template` 仍是可选高级覆盖，省略时采用原有自动规则。
- 旧 → 新：桌面重复要求逐模型选目录模板、可能排除已有共享元数据的模型 → 核心按已有映射计算各目标可用模型，桌面只读展示、选择默认模型并批准；不再提交模板覆盖。无兼容映射时明确提示并禁用保存。
- 兼容与迁移：`available_targets` 是追加字段，无可用目标时为 `[]`；客户端与核心需一同升级。既有绑定在用户重新保存前保持原策略；桌面更新提交空模板覆盖，移除旧目标级选择/排除并采用用户批准的自动模型列表，凭据及供应商映射保持不变。缺失或歧义元数据仍不伪造，请在共享模型配置补齐；CLI 可继续显式覆盖。
- 验证：结果见本轮任务更新记录，区分隔离核心/CLI/桥接测试与浏览器模拟桌面；未构建或发布安装包，未操作真实 Agent 配置或付费上游。

## 2026-10-13 — 当前分组倍率默认核对保存的 Key（源码 CLI；desktop.25 dev 已投放）

- 类型：修改行为；无新增或删除命令／参数。
- 指令：`provider effective-rate --id ID --revision N [--key KEY] [--remote-key-id N]`。
- 旧 → 新：remote-key-id=0 原先返回 remote_key_required；现在基于用户账户列表验证所选保存的 Key 后读取它的分组倍率。sub2api 使用完整密钥匹配；NewAPI 掩码候选必须由固定 POST 读完整密钥确认。显式正整数保持原有远端 Key ID 语义；原版 OneAPI 仍 unsupported。
- 边界：最多4页各100条，并共用8次请求／2 MiB预算和核心超时；找不到返回 key_not_found。NewAPI auto 返回 unknown，可保留组名但倍率为 null；不默认1、不更改保存的路由倍率、不回显凭据。
- 兼容与迁移：无需数据迁移；已有显式 ID 调用不变。依赖 remote_key_required 的脚本应改为检查实际 failure 或显式传入后台 ID。
- 验证：隔离 Go CLI 测试覆盖自动核对与显式 ID、凭据不回显；实际执行结果见本轮任务记录。


## 2026-10-13 — 来源Key范围与模型顺序／调度规则（源码 CLI；desktop.25 dev 已投放）

- 新增 model routing --id/--revision/--strategy/重复--candidate，已有完整候选成员原子排序，保留Key、倍率和模型元数据。
- model add/put新增--strategy（默认priority，保持旧行为）、重复--candidate-key INDEX:KEY；provider model-put新增重复--source-key及--all-keys（编辑省略保留，新建空范围）。
- model/workspace公开DTO增加key_ids、candidate_index；模型strategy新增cache-priority、lowest-price、lowest-latency。供应商Key策略仍为priority/round-robin。
- 缓存亲和与低延迟观察只留内存，低价按保存路由倍率；未知金额不填0或1。指定来源参与真实路由，失效时不调度到范围外Key，删除来源Key返回referenced。
- schema5→6保留既有数据，旧模型空范围允许全部启用Key；旧核心拒绝schema6，升级须同步核心/CLI/桌面。CLI与桌面共用同一管理实现；定向验证结果见本轮更新记录。

## 2026-10-13 — NewAPI 推理 Key 查询 billing 余额（desktop.26 dev）

- 类型：修改。
- 指令与参数：provider balance、provider balance-summary，参数不变；帮助补充 NewAPI 授权优先顺序。
- 旧 → 新：无用户授权的 balance 原来返回 auth_required，现在使用推理 Key 调用 subscription/usage；balance-summary 原来只读 Key quota，现在优先 billing，接口缺失或格式不兼容时回退 Key quota。balance 在配置用户授权时仍优先查询账户钱包。
- 兼容与迁移：输出 scope 新增 billing，金额 unit 新增 CNY/site_unit；站点未公开查询范围时明确保留 billing，不冒充账户钱包。剩余值用精确十进制相减，usage 按上游定义除以100；无限额占位值不当资金显示。旧的严格校验客户端需与核心同步升级；数据库不新增迁移。
- 验证：模拟 HTTP NewAPI 下的适配器、核心、客户端、CLI 和桌面桥接定向 Go 测试通过，包含无用户授权查询、已有授权优先、默认／指定 Key、部署前缀、回退及分类失败；未调用真实付费上游。

## 2026-10-13 — NewAPI 系统访问令牌用于列表账户余额（desktop.27 dev）

- 类型：修改行为和帮助，参数不变。
- 指令与参数：provider balance-summary；provider platform-put --user-token-stdin，provider balance 沿用已有授权优先顺序。
- 旧 → 新：NewAPI 列表余额原先忽略已保存用户令牌，现在与 balance 一样优先用系统访问令牌查账户余额（scope=account）；账户授权失败保留错误，不退回 Key 数值。未配置用户授权时沿用 billing / Key 额度兼容查询。
- 兼容与迁移：无新增参数和数据库迁移。通过该站后台个人／安全设置生成系统访问令牌，用 --user-token-stdin 保存；用户 ID 按站点要求填写。scoped token 的账户读取需要 profile:read，自动核对 Key 分组需要 api_key:read、api_key:reveal。
- 验证：providerapi/core/client/desktop/CLI 五包定向测试通过，覆盖已保存系统令牌、默认或缺失推理 Key、自动平台识别、用户 ID 请求头、零余额和授权失败不回退；前端类型/lint及15项定向单测通过。不使用真实凭据或付费请求。

## 2026-10-13 — sub2api 推理 Key 直接查询计费倍率（desktop.28 dev 已发布）

- 类型：修改行为和帮助，参数不变。
- 指令与参数：provider effective-rate、provider models；--remote-key-id 默认0沿用所选 Key，正整数仍查询该站远端 Key。
- 旧 → 新：原先缺少用户 JWT 就结束倍率查询，现在 sub2api 优先使用推理 Key 调用 /v1/sub2api/billing，读取含个人覆盖与高峰规则的 effective_rate_multiplier；旧站缺接口（404/405或200 HTML）才回退用户授权。认证、权限、限流、服务或格式错误直接报告。未配置账户平台的模型目录以一个 Key 专用请求识别 sub2api；auto 探测复用本次响应，不重复查询。
- 兼容与迁移：无新增命令或数据库迁移。effective_rate.basis / keys[].rate_basis 新增 key_billing，此来源允许 group/group_name 为空；旧客户端需同步升级。NewAPI、OneAPI 与显式后台 Key ID 查询的授权规则不变；只显示快照，不自动修改路由倍率。
- 验证：providerapi/core/client/desktop/CLI 定向 Go 测试通过，覆盖 Key-only、当前有效倍率、零倍率、精度、部署前缀、旧站回退、分类失败、多 Key 目录和桌面桥接；隔离源码的前端 typecheck/lint 和账户／目录／倍率展示44项定向单测通过；双平台打包、六附件digest与标签核对、默认源Linux实际更新下载及大小／SHA256验证通过。未使用真实站点凭据、付费推理请求或原生安装验收。

## 2026-10-13 — 完成请求不再误记为客户端取消（当前源码，未发布）

- 类型：修改。
- 指令与参数：`logs list/detail/follow/export`；命令、参数和 JSON 字段不变。
- 旧 → 新：协议正常结束、响应完整写出后，客户端在用量／日志收尾期间断开曾覆盖成功结果为 `cancelled/client_cancelled/partial`；现在保留 `succeeded` 和 `complete`。结束事件写入失败、完成前取消仍按取消记录。
- 兼容与迁移：升级实际代理请求的核心后影响新记录，历史记录不自动修改；不更改数据库或桌面 DTO。TPS 的正文区间与正文 Token 口径保持不变，无法确认时仍为未知。
- 验证：修复前合成回归复现流式及非流式完成后的误判；修复后这两条路径、结束事件写入失败、真实提前取消及 TPS 已知／未知条件的定向 Go 测试通过。仅使用隔离状态和合成上游。

## 2026-10-13 — models.dev 参考目录与费用方法（desktop.29 dev 已发布）

- 类型：新增；修改既有价格导入的数据获取行为。
- 指令与参数：新增 `catalog providers/models/model/estimate`；支持精确 provider/model、search、limit、refresh，以及输入／输出／缓存读写／思考 token 的观测计数。
- 旧 → 新：原先只导入基础单价、每次重新下载目录，现在共享1小时缓存，提供能力／上下文／原始思考选项／分档价与十进制文本 token 估算；旧 `provider price-import` 仍只返回基础价格预览、另行 price-put 保存，更新时间改为共享快照下载时间，刷新失败的陈旧快照拒绝导入。
- 兼容与迁移：无数据库迁移或映射默认值变化；现有 import 参数和 DTO 不变。新查询区分未知／0、目录陈旧状态和获取时间；计价调用方须规范化缓存包含于输入、思考包含于输出的总量口径。新旧核心需同步升级使用新命令。
- 验证：隔离核心／客户端／桌面／CLI 调用链，解析、缓存与刷新失败重试、并发合并、精确价格／阈值／未知值、鉴权与实例校验测试通过；前端方法单测、类型检查和定向 lint 通过；公开 models.dev 目录兼容检查通过。未调用付费推理，未验收原生窗口或打包。

- 发布状态：2026-10-13 随 desktop.29 dev 双平台安装包和在线更新投放；生产编译、安装包及默认源下载验证通过，未执行原生窗口或安装验收。原有功能测试记录保持不变，本轮无 CLI 契约变化。


## 2026-10-14 — DSH 本机目录发现（当前源码，未发布）
- 类型：修改。
- 指令与参数：`agent discover` 参数不变；发现结果可新增 agent=`dsh` 的目标。
- 旧 → 新：已知根新增核心宿主环境中的绝对路径 `DSH_HOME` 与默认 `~/.dsh`，同一目录去重；目录不存在不返回目标，路径不安全报告 unsafe。只读检查，不读取 DSH 凭据、不运行客户端、不访问上游。DSH 状态仍为 unsupported，不提供配置托管适配器。
- 兼容与迁移：新增发现项，不改已有目标 ID 和批准参数；无需迁移。GUI 使用平台开关与折叠设置，明确按钮提交仍传 approved=true；CLI 的 `agent enroll --approve` 保留。
- 验证：隔离目录测试覆盖 DSH 默认根、环境覆盖、去重和相对路径忽略；前端验证结果见本轮任务记录。


## 2026-10-14 — 模型映射能力与默认值设置（当前源码，未发布）

- 类型：新增。
- 指令与参数：新增 `model settings --id ID` 读取当前设置；`model settings --id ID --file FILE` 从本机 JSON 文件保存完整设置，文件至多 1 MiB。
- 旧 → 新：模型映射现在可保存上下文／输入／输出限制、能力声明、原生 effort／toggle／budget_tokens 思考范围与默认值、默认输出预算、Temperature 和并行工具调用；`reference_action=keep|set|clear` 独立控制 models.dev 参考快照，set 由核心按精确 provider/model 身份获取。
- 兼容与迁移：目标模型必须已存在，写入携带当前 expected_revision；版本冲突拒绝保存，结果未知需先重新读取。旧 `model put` 保留已有 settings/reference，候选顺序、倍率、Key 范围和 Codex 模板不变。需同步升级 CLI 与核心；当前源码尚未打包发布。Codex 只导出可表示的配置，保存后需同步并重启 Agent；日志逐请求比价尚未接入。
- 验证：`go test -p 2 ./internal/modelconfig ./internal/state ./internal/catalog ./internal/core ./internal/client ./internal/desktop ./internal/cli ./internal/managed` 通过；覆盖 CLI 读写、文件／身份校验、revision 冲突、目录快照、桌面桥接和 Codex 导出。前端 typecheck、相关 lint 和模型设置／目录／供应商汇总／传输共 37 个定向测试通过；29 项 desktop 浏览器回归通过（模拟宿主）。验证使用隔离状态与模拟目录，未修改真实 Agent 配置或请求付费推理。


## 2026-10-14 — 推理基址自动补路径与 HTML 错误提示（当前源码，未发布）

- 类型：修改 `provider test` 与真实转发行为，无新增命令或参数。
- 旧 → 新：缺少 `/v1` 时可能收到 HTTP 200 的网站 HTML 并报笼统的 `invalid_response`；现在优先原地址，仅明确收到 HTML 且状态为200／404／405时补 `/v1` 重试一次，保留部署前缀与原有超时，不修改已存地址。已带版本、JSON 错误、鉴权失败、限流、网络失败及 SSE 不补路径。
- 兼容与迁移：Probe 输出字段与退出码不变，`failure` 新增 `unexpected_html`；真实网关最终收到 HTTP 200 HTML 时返回502及同名错误码，日志保留实际上游200并记录端点解析证据。桌面新增中文地址提示。无需数据库迁移，新错误枚举需前后端同时升级。
- 验证：本次改动在独立 worktree 中通过 core／protocol／desktop／client／cli 测试，前端15个定向测试、typecheck 与定向 lint 通过；覆盖原地址成功、三协议与部署前缀、HTML 200／404／405 补路径、已带版本不追加、JSON／鉴权／限流不重放、SSE 与超时共享、错误日志脱敏。使用隔离模拟上游，尚未打包发布或验收原生窗口。


## 2026-10-14 — 全部映射默认选择与 Claude Code / DSH 托管（当前源码，未发布）

- 新增 `agent enroll --model-scope all-mapped|all-compatible`，`agent add --adapter claude|dsh --config-root ABS_DIR`，保留 Codex `--codex-home` 别名，两个路径参数不可并用。
- 新增 all-mapped 授权及 set-models 范围：读取全部已存模型映射，具备启用供应商即可，不以 Codex 元数据或 tools/streaming 筛掉模型。桌面登记显式使用 all-mapped；旧 Codex CLI 缺省 all-compatible、旧 single 绑定不静默扩权。发现预览对应全部映射，缺少 Codex 缓存不再阻止此范围。
- Claude 用户 settings.json 与 DSH web cordis.patch.yml 接入统一计划、指纹、日志、同步、条件恢复及禁用凭据生命周期。供应商密钥不写入 Agent 文件；两个新适配器使用受保护配置中的网关 Binding token，管理返回与 Anchor 日志不公开凭据。
- 客户端重载边界：Codex/Claude Code 重启，DSH 默认模型在新会话生效；项目/命令参数覆盖与真实客户端运行尚未验收。Gemini/OpenCode 仍只识别。

## 2026-10-14 — UI 与 CLI 共用 SSH 设备连接（desktop.31 dev）

- 类型：新增／修改。
- 指令与参数：context add --ssh、--ssh-port、--identity-file、--remote-anchor、--remote-control-port、--trust-new-host；context bootstrap --control-port --show-once；全局 --ssh-password-stdin。
- 旧 → 新：仅本机 Context → SSH 密钥／Agent 与本次密码连接；管理动作复用连接层，不需手动建立隧道。bootstrap 在目标账户核验／启动核心并签发 720 小时 manage 授权。列表不含管理秘密；桌面选择不改 CLI 默认 Context。
- 兼容与迁移：远端须已安装支持 bootstrap 的新 CLI 且允许 SSH 回环转发；密码不保存，重开应用需重新输入；自动安装尚未实现。schema7 自动迁移，旧程序不可打开，升级前备份。
- 验证：隔离 state/client/connection/cli/desktop 定向 Go 测试通过，真实 OpenSSH 与内置密码 SSH 用本地隔离 SSH 服务／核心验证；前端类型检查、lint、22 项单测通过。发行／浏览器结果见本轮 release 记录。
- 发布状态：北京时间 2026-10-14 22:34:21 随 desktop.31 dev 两平台安装包和同版更新清单公开；本机 CLI／桌面已更新到 0.1.0-dev.desktop.31。CLI 正式预发行仍为 alpha.6，远程连接使用本轮新版源码 CLI；本轮未投放 CLI 在线发行。

## 2026-10-14 — SSH 非交互环境发现 Anchor 与错误细分（desktop.32 dev 已发布）

- 类型：修改。
- 指令与参数：context add --ssh、--remote-anchor；context bootstrap。
- 旧 → 新：默认仅执行 PATH 中的 anchor → 按 PATH、~/.local/bin、~/bin 与系统常见路径发现可执行 CLI；自定义路径不回退，支持 ~/。密钥与密码模式共享受控命令；先用 bootstrap --help 检查能力。不存在、版本不支持、管理端口不匹配、核心未就绪分别返回固定错误码，原始 stderr 不回显。
- 兼容与迁移：无需调整已有默认档案；不安装缺失的远端 CLI，不修改远端 PATH／shell 配置。细分核心错误需要远端新版 CLI，旧版错误继续保留 remote_anchor；返回数据结构与凭据策略保持兼容。
- 验证：隔离非交互 shell、用户目录含空格／shell 字符、显式路径不回退、脱敏诊断、真实 OpenSSH／内置密码 SSH 连接用例；实际执行结果见本轮任务记录。
- 发布状态：北京时间 2026-10-14 23:12:40 随 desktop.32 dev 双平台安装包公开；本机 CLI／桌面二进制已更新，核心版本 .32 且 ready。发起连接的桌面端需更新以使用路径发现；CLI 正式预发行仍为 alpha.6，本轮未投放独立 CLI 在线发行。

## 2026-10-15 — 用户名称支持中文（当前源码，未发布）

- 类型：修改。
- 指令与参数：`provider key-put/key-delete/key-reset/key-model-protocol/model-put` 的 `--target`，`model add/put/settings/routing` 的 `--id`，`context add` 的 `NAME`，`agent add` 的 `--id`，以及 `agent add/enroll/set-models` 的公开模型名和关联查询、修改指令；命令及参数名称不变。
- 旧 → 新：原本仅供应商 ID 接受中文；现在 Key ID、公开模型 ID／别名、Context 名称、Agent Binding ID 也接受 1–128 个 Unicode 字母／十进制数字或 `._-`，首字符须为字母／数字；Binding ID 另外限制在 64 个 UTF-8 字节。拒绝空白、控制／格式字符、emoji、路径分隔符及 URL 保留符号；按原值精确匹配，不裁剪或归一化。机器错误码仍为 ASCII。
- 兼容与迁移：既有 ASCII 名称、命令、参数、JSON 字段与 revision 不变，无需迁移。创建中文名称须同步更新 CLI／桌面与运行中的核心，旧版客户端或核心不能处理新增名称；Agent 托管仍需显式批准、按原有条件恢复。本次未构建或公开发布安装包。
- 验证：隔离状态的 naming/state/catalog/client/core/managed/cli/desktop 八个受影响 Go 包测试通过，涵盖命名边界、中文 Key 与公开模型转发、模型设置、CLI、桌面桥接、Agent 托管恢复与 ASCII 错误码回归；前端 49 项定向测试、类型检查和 lint 通过。未验收原生窗口或真实 Agent 推理。
- 发布状态：北京时间 2026-10-15 01:18:46 随 desktop.33 dev 双平台安装包及同版更新清单公开，默认源 Linux 实际下载与六附件大小／SHA256 digest 核对通过。本次未安装本机；独立 CLI 正式预发行仍为 alpha.6，CLI 中文名称使用新版源码构建及核心。

## 2026-10-15 — 日志区分首次有效输出与正文首字（当前源码，未发布）

- 类型：新增兼容字段；命令、参数和鉴权不变。
- 指令与参数：`logs list/detail/follow/export` 的请求 `metrics`。
- 旧 → 新：仅有正文首字 `time_to_first_text_ms`，纯工具流式请求没有首字；新增可空 `time_to_first_output_ms`，正文或工具调用参数（含 Responses 自定义工具输入）的首次非空增量均可触发。角色、心跳、工具空壳、空增量和单独思考不触发。桌面“首字”优先使用新值，正文耗时与 TPS 仍用独立正文区间。
- 兼容与迁移：旧字段语义不变，无数据库迁移；旧日志与旧核心缺少新字段时可回退正文首字，不能补算历史纯工具请求。须升级正在处理请求的核心才能采集新值，未打包或部署。
- 验证：定向 Go 协议、核心日志与桌面契约测试，以及前端日志单测、类型和 lint 检查，实际执行结果见本任务记录；测试仅用隔离状态与合成上游。

## 2026-10-15 — 模型创建自动应用 models.dev 与手动补值（源码未发布）

- 类型：修改行为；无新增命令、参数、JSON 字段或数据库迁移。
- 指令与参数：`model add --candidate`、`provider model-put --model` 创建新别名，以及 `model settings --file FILE` 的 `reference_action=set`；创建相关参数帮助同步说明自动参考数据。
- 旧 → 新：创建仅保存映射 → 核心最多等待 3 秒，按上游精确 ID 无歧义解析目录，补入上下文/限制/能力/原生思考范围与参考价格快照；未匹配、目录不可用或候选不一致时正常创建、不猜测。set 来源原本只保存快照 → 同时补齐缺失配置，跳过与手工限制/预算冲突的目录字段。参考价格不覆盖供应商实际计费表。
- 兼容与迁移：创建 revision 仍为 1；更新/追加映射不重填，tools/streaming 继续按显式值，目录缺失的思考默认值不猜测。已有映射不会读取即迁移，可显式 set 来源并保存；运行中的核心需更新后才有新行为。本轮未打包、部署或发布。
- 验证：隔离核心创建两条入口、追加/更新保护、手动来源补值、价格快照、凭据边界与前端导入冲突/默认值测试；实际执行结果见本轮任务记录。


## 2026-10-15 — 供应商充值倍率与逐Key缓存调度（当前源码，未发布）

- 类型：新增／修改。
- 指令与参数：`provider add/put --cost-multiplier N`；`provider models/effective-rate/workspace`；`provider model-put --multiplier`。
- 旧 → 新：旧候选手填倍率停止参与调度，改为供应商倍率×自动缓存的Key有效分组倍率；新供应商默认1，编辑省略保留。workspace逐Key返回rate及effective_multiplier。默认自动身份核对查询缓存，显式远端数字ID不缓存；失败保留成功观察，无缓存未知，不递增配置revision。
- 兼容与迁移：schema8自动迁移，旧程序不可再打开；客户端和核心一起升级。旧multiplier字段及参数兼容保留但仅作未来模型倍率预留；现有自定义值不再决定路由，需设置供应商倍率并刷新Key分组缓存。固定优先／低延迟规则不改，已知／零／未知调度见当前大全。
- 验证：隔离Go管理／缓存／调度测试与前端类型、lint及单测；具体命令和最终结果见本轮任务记录，不代表原生安装或真实付费平台验收。

## 2026-10-15 — 增加含工具参数的输出 TPS 指标（源码，未发布）

- 类型：新增输出字段；修改桌面 TPS 展示口径。
- 指令与参数：`logs list/detail/follow/export`；沿用既有命令、鉴权与参数。
- 旧 → 新：原正文 TPS 遇工具/混合输出因正文 Token 无法分离而未知；新增 `metrics.output_generation_ms`、`non_reasoning_output_tokens`、`output_interval_complete`，计算非思考输出 Token / 首次正文或工具参数输出到完成的秒数。输出耗时可包含交错思考和等待，不等于纯解码速度；旧正文指标不改语义。
- 兼容与迁移：新增字段可被旧消费者忽略，无数据库迁移；未升级核心和历史工具请求不补算。旧完整正文统计可在没有新输出测量时作为输出子集兼容展示；缺少用量、思考不可分离、非流式、中断、纯思考保持未知。不记录工具参数，不调用付费上游。
- 验证：隔离合成上游验证工具输出指标持久化和导出；Go 核心、日志、桌面共享契约与 CLI 测试，日志前端单元测试、类型检查与 lint 的实际结果见本轮任务记录。未运行浏览器或原生窗口验收，未打包发布或重启本机核心。

## 2026-10-15 — CLI 模型映射默认开启流式（源码，未发布）

- 类型：修改默认值。
- 指令与参数：`model add`、`model put`、`provider model-put` 的 `--streaming`。
- 旧 → 新：默认 false → true，与桌面新建映射一致；帮助明确使用 `--streaming=false` 关闭。`--tools` 默认 false 不变。
- 兼容与迁移：不批量迁移已有映射，不改变管理 API 的布尔字段语义。上述命令写入时省略参数会声明流式支持；`model put` 和编辑用的 `provider model-put` 若要维持关闭状态，须显式传 `--streaming=false`。追加已有公开别名仍保留共享能力。默认开启不代表上游能力已验证。
- 验证：CLI 模拟管理接口测试覆盖上述创建／编辑入口的省略、显式开启和显式关闭，并断言工具默认值不变；帮助回归检查默认 true 与显式关闭说明。
