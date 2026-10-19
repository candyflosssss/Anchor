# 供应商桌面接口语义

- 更新：2026-10-19；标识：`anchor-desktop-v1`（替代仅供设计的 `provider-ui-prototype-v1`）。
- 状态：真实 Go 桥接、桌面传输和共享契约已实现；隔离核心读写已测试，**不等于 Wails 原生窗口或安装验收通过**。
- 桌面 DTO 权威：`internal/desktop/contract.go`；TypeScript 由 `go run ./cmd/desktop-contract` 生成，漂移由 Go 测试检查。核心业务规则仍归 `internal/state` / `internal/core`，不复制到宿主。
- 受限方法、窗口／核心生命周期与构建边界见[桌面运行契约](desktop-runtime.md)。

## 1. 读写与数据

| 前端端口 / Wails 方法 | 真实客户端与行为 |
| --- | --- |
| `list(signal)` / `Bridge.Providers()` | `client.Providers` → GET 当前实例目录；只回脱敏 DTO，空目录固定为空数组 |
| `discover(input, signal)` / `Bridge.DiscoverProviderModels(input)` | `client.DiscoverProviderModels` → POST `/control/v1/providers/{id}/discover-models`；显式读取上游目录，不改配置 |
| `save(input, signal)` / `Bridge.SaveProvider(input)` | `client.PutProviderWithKeys` → PUT 指定 ID；revision=0 创建，>0 条件更新；普通字段完整替换 |

Provider 公开字段为 `id`, `homepage_url`, `base_urls`, `protocol`, `enabled`, `timeout_seconds`, `max_concurrent`, `cost_multiplier`, `revision`, `has_secret`。`homepage_url` 可空，仅展示；`base_urls` 为 1..16 个不重复请求地址，网关按请求轮询，目录与测试使用第一个。`cost_multiplier` 为供应商充值成本倍率。`protocol` 接受 `responses`、`chat-completions`、`anthropic`，未知协议拒绝；当前宿主返回已存协议，作为 `anchor-desktop-v1` 的可选字段增量，旧宿主未提供时前端保留未知状态，不将它默认为 Responses。不添加健康、余额、最近测试时间、display_name 等没有来源的字段。Go 内部 Secret 不进入 DTO，前端传输再次挑选公开字段。

- 新增／编辑弹窗的「请求渠道」选择对应 `protocol`，详情概况回显。新建默认 `responses`；编辑仅在选择变化时显式发送该字段，旧宿主缺字段时显示「保留现有渠道」。
- 保存显式非空 `protocol` 时，核心在同一 revision 条件事务中更新供应商匿名请求协议与已有 default Key 的默认协议，无需重新输入密钥，不影响其他命名 Key、default Key 启用／健康状态或模型独立协议覆盖。省略／空字符串保留供应商和已有 Key 各自的值；新建 default Key 仍按供应商协议初始化。普通编辑不重复发送未改变的渠道，避免覆盖单独设置过的 default Key。
- 本行为为 2026-10-19 当前源码增量，未打包发布；旧核心不会同步修改已有 default Key，须同步升级宿主／CLI 与核心。数据库 schema 不变，不自动重写现存 Key；CLI 复用既有 `provider add|put --protocol`，见[指令大全](../cli/commands.md)。

- 返回 revision 范围 1..Number.MAX_SAFE_INTEGER；请求 expected_revision 为 0..MAX_SAFE_INTEGER-1，且与输入 provider.revision 一致，防止更新后超出 JS 安全整数范围。
- 超时单位为秒，1..3600；并发为 1..64。前端即时校验只帮助填写，核心仍做最终业务校验。
- 新增供应商可选填写多个 Key：首个默认 Key 留空发送 secret=null，非空保存为 default Key；可选 `initial_keys` 数组包含其他 Key 的 `id`、`protocol`、`secret`，只在 expected_revision=0 创建时接受。名称唯一且不可为 default，协议逐 Key 独立、秘密非空且不超过 8192 字节，总计至多 64 个 Key（表单保留一个默认槽位）；这些 Key 默认启用，与供应商及全部地址在同一事务保存，只产生 revision=1，任何校验或写入失败全部回滚。更新不得携带非空 initial_keys，仍用 key-put 管理命名 Key；旧核心严格拒绝新增字段，不会静默丢 Key，需配套更新宿主和核心；无数据库迁移。管理请求仍受 128 KiB 总体积上限约束。只填命名 Key 也可保存；has_secret 在保存／列表／单项读取统一表示存在任意 Key，而不是仅 default。填写任意 Key 后保存成功使用返回的 revision 显式查询目录，并在同一弹窗快速／全部导入模型。目录失败保留已保存供应商，可重试查询或修改配置／Key；不重放创建请求。自定义别名仍在详情中处理。编辑时 secret=null 保留 default Key、空字符串清除 default Key、非空替换 default Key；命名 Key 不受影响；清除有明确确认。秘密只入不出、不写前端持久存储、日志或 URL；提交开始清空所有 Key 输入，完成或失败后清理请求快照，取消／卸载丢弃整个草稿；失败需重新输入。不能承诺 JS/Go 字符串的物理内存擦除。
- 添加／编辑均可增删请求地址，支持 HTTPS 域名或 IP（含端口）；HTTP 仍仅限数值回环测试地址，不放宽 TLS 规则。修改地址时保留全部 Key、各 Key 协议和既有 revision 冲突保护。
- 保存成功返回新 revision，不表示供应商连通性已经验证。

## 2. 查询、写入与错误

所有桥接结果含 `contract`、`data`、`problem`。版本不符、无绑定或无有效 data 时失败，不回退 Mock。`problem.code` 仅白名单，前端不解析任意异常文本。

| 桌面错误 | 语义 |
| --- | --- |
| offline / read_error | 无法连接或无法读取；数据不可用不等于空列表 |
| unauthorized | 401/403；授权留在 Go 层，不要求页面填管理令牌 |
| referenced | 409；删除供应商／Key／模型将移除已启用 Binding 默认模型的最后一个候选；先更换默认模型或停用 Binding |
| conflict | 409；不覆盖新配置，丢弃旧草稿重读后重新编辑 |
| invalid | 本地参数／核心校验拒绝，不以旧草稿静默覆盖 |
| unknown | 写入网络中断、5xx 或无法验证回包；可能已生效，须先查询，不自动重放 |
| identity / unsupported / closed / startup_failed | 宿主状态错误，统一由会话页解释；不开放供应商写入 |

`internal/client.RequestError` 提供 HTTP 状态、受限错误码与 Unknown 标记；桥接按结构映射，保留实例身份校验。查询／保存最多 15 秒（包含身份握手），Status 为 3 秒，启动最多 20 秒。Wails 调用不能通过浏览器 AbortSignal 保证取消 Go 操作：AbortSignal 与 generation 只阻止发起已取消的请求或消费过期响应，**取消不等于回滚**。

供应商 Controller 的同步 latch 防双击重复写。新建成功后自动读取权威列表、选中新供应商并读取详情；工作台动作发生版本冲突时自动同步列表和详情，完成后提示原操作未保存，用户核对后重新操作。配置编辑冲突与写入结果未知仍需读取核对。每次成功重读供应商列表都触发当前工作台刷新，不以 revision 变化作为唯一条件：模型冲突、Key 调度状态重置或部分未知结果不会改变供应商 revision。工作台读取期间暂停提交；读取成功后清除错误和待重读锁定，失败时保留锁定并可再次读取。重读只查询，不自动重放之前的动作。核心断开后供应商页面卸载、缓存与秘密草稿丢弃，Status 不自动重启；用户明确重新连接才再次 Start。当前只连接本机目录，不提供远程／多实例切换 UI。

## 3. 共享样例与两种模式

`desktop/contract/providers.json` 集中维护完全合成的供应商、空列表和错误样例，Mock 与 Go DTO/错误映射测试读取同一份内容；不放真实地址或秘密。

- `prototype`：显式内存 Mock，提供原型场景入口；密钥丢弃，只改 has_secret；刷新重置。
- `desktop`：仅真实桥接适配器，无场景入口；缺 Wails 绑定就报错，不能在普通浏览器中绕过管理鉴权。

管理 API 的 Origin/Host 防护不放宽，不增加 CORS 或通用 URL／shell／文件接口。测试分为共享 DTO/Mock、浏览器模拟 Wails 边界、真实 Go 宿主→client→隔离核心、原生桌面四层；前三层通过不能替代最后一层。

## 4. 上游模型目录小步（desktop.10）

规范 DTO `DiscoverProviderModels` / `ProviderModels` 定义于 `internal/catalog/models.go`，经 `internal/desktop/contract.go` 别名纳入 Go → TS 单一生成源；桌面 envelope 为 `ProviderModelsResult`，继续使用 `anchor-desktop-v1`。本次 Key 协议增量扩展查询字段及工作台动作，客户端与核心须同步升级。共享合成样例为 `desktop/contract/provider-models.json`，Mock 与 Go / TS 测试共用。

- 输入为 `provider_id`、`expected_revision`（桌面 1..MAX_SAFE_INTEGER）和 `key_id`（空字符串自动查询全部启用的已存 Key，跳过停用 Key）。必须匹配路径及当前已存供应商 revision，使用管理授权和 `X-Anchor-Instance`。不接收临时 URL、密钥或任意路径；冲突在发起上游请求前返回409。
- 后端从已存快照取地址／密钥，使用 `<base_url 去尾斜杠>/models`，保留部署前缀；未带版本的根地址或自定义前缀，仅404/405或200 HTML网页时尝试同前缀 `/v1/models`，兼容 NewAPI 代理部署（例如 `/newapi/v1/models`）；已带版本路径或版本后的 `/openai` 不重复追加 `/v1`。官方 OpenAI、Anthropic、Kimi/Moonshot 和硅基流动根地址固定 `/v1/models`；DeepSeek 根地址固定 `/models`，OpenRouter 固定 `/api/v1/models`，Gemini 根地址及 `/v1beta/openai` 目录改用 `/v1beta/models`；`/v1beta` 前缀使用 `x-goog-api-key`，其他路径沿用 Key 默认协议鉴权。所有凭据只进请求头。停用供应商也允许显式查询。复用无环境代理、禁止重定向的客户端与全局16／供应商并发上限；全部路径/分页共用 `min(timeout_seconds, 8)` 秒。每个 Key 最多10次目录 HTTP请求、每页请求1000条；全部 Key 的目录与倍率抓取共用上述期限，先读取全部目录再读取倍率，不重试失败请求或后台轮询，目录本身不持久缓存；逐 Key 倍率持久缓存见[倍率设计](provider-multipliers.md)；完整规则见[CLI 大全](../cli/commands.md#provider-modelsdesktop10-小步)。
- 输出十字段：原有 provider_id、key_id、revision、http_status、elapsed_ms、models、failure，新增 partial、entries、keys。默认聚合 key_id 为空，无已存 Key 为 anonymous，显式指定时保留该 ID。全部 Key 停用或指定停用 Key 时返回 no_enabled_keys、http_status=0 和空 models/entries/keys，不回退匿名请求。models 为始终非null的模型 ID 去重排序数组；entries 为 `{id,key_ids}`，合并同一模型对应的全部启用且目录成功的可选 Key；keys 为 `{id,enabled,protocol,http_status,failure,group,group_name,multiplier,rate_basis,rate_failure}`，倍率为十进制字符串或 null。至少一个 Key 目录成功则顶层 failure 为空，partial 标记失败 Key；逐 Key 目录必须完整分页才关联模型，全部失败或聚合超限时不返回模型。接受 OpenAI/Anthropic data[].id、Gemini models[].name（去除 models/ 前缀）；全部 Key 累计目录512 KiB，最多64个 Key、每 Key 1000条原始记录、合并1000个模型、每 ID 256字节；分页循环、格式变化、后续页失败只使该 Key 失败。
- 倍率复用 providerapi，与 Key 默认推理协议独立。sub2api 优先用每个推理 Key 读取 `/v1/sub2api/billing` 的当前有效倍率（含个人覆盖与高峰规则），无需用户 JWT；未配置平台时以一个 Key 专用请求识别 sub2api，缺接口才要求配置平台。该接口不返回组名／ID，`group` 与 `group_name` 留空，`rate_basis=key_billing`。仅404/405或200 HTML网页回退旧站用户授权查询，其他错误直接报告。旧 sub2api 与 NewAPI 自动读取账号 Key 列表并精确核对身份；NewAPI 掩码候选经固定只读 POST /api/token/{数字ID}/key 验证完整凭据后取分组与 pricing.group_ratio，Sub2API 取个人覆盖或可用组倍率和组名；OneAPI、auto 分组或缺授权不会假定1×。每 Key 最多4页账号列表（100条/页），倍率适配器最多8次请求/2 MiB响应。未知和失败用 rate_failure 标明，不写入 Anchor 映射倍率，不使目录结果丢失；多 Key 的组名也检查凭据回显。
- 请求受理后目录失败仍返回 HTTP200 与固定分类：`no_enabled_keys`（无启用 Key，跳过目录和倍率请求）、`authentication`（401/403）、`unsupported`（404/405）、`rate_limited`（429）、`http_error`（其他非200）、`network`、`timeout`、`busy`、`too_large`、`invalid_response`。上游错误正文、原始网络异常、密钥不回显；包含已存密钥的 ID 也拒绝。管理层鉴权／冲突／实例错误仍走 `problem`，不混作上游结果。
- 不写配置、revision、模型映射或健康状态。一次目录成功仅说明此接口返回有效目录，不代表推理、工具、流式、余额或持续健康；404/405 不等于供应商推理不可用。
- UI 详情手动触发、禁重复点击，显示目录 HTTP／耗时／失败分类和限高模型列表；区分“目录列出”与“仅已存协议”，避免把旧覆盖当成本次 Key 目录；Key / 模型协议编辑见 §6。查询中阻止供应商切换、编辑和页面离开；选择、重读或修改供应商／Key 凭据后清空结果；模型协议保存保留当前目录，取消／卸载丢弃晚到数据。Wails AbortSignal 不能保证中断后端请求，后端超时仍生效；查询错误不会显示“可能已保存”。新功能与旧核心不同步时失败关闭，不回退 Mock。
- CLI 等价入口为 `provider models --id ID --revision N [--key KEY]`；与 `model list`（Anchor 已存公开映射）及网关 `/v1/models` 区分。测试分层沿 §3，原生两平台仍未验收。

## 5. 第一优先工作台（desktop.11）

新增 `Bridge.ProviderWorkspace({provider_id})` 与 `Bridge.ProviderAction(Action)`，对应 GET `/control/v1/providers/{id}/workspace` 与 POST `/control/v1/providers/{id}/action`。动作白名单、默认值和 CLI 等价入口见[指令大全 §5](../cli/commands.md#5-供应商)及[实现契约](provider-priority-one.md)。`internal/catalog/workspace.go` 是核心 DTO 源，桌面规范以别名纳入生成器，共用 `desktop/contract/provider-workspace.json`。

Workspace 固定返回 provider_id/revision/strategy/keys/models/usage/prices/estimates，集合始终为数组；probe 和 imported_price 可空。KeyStatus 只含 ID、enabled、状态、连续失败次数、降权结束时间和最近测试，以及 protocol 与 model_protocols（[{model,protocol}]，始终数组），绝不返回密钥。Probe 绑定 revision、Key ID、模型、执行时间、HTTP 状态、耗时与失败分类。Prices 表达 USD/百万 token，缓存单价与估算 USD 可以 null，不能用0替代未知。

每次动作携带当前供应商 revision，模型编辑/删除还携带 model_revision。删除、改名、删 Key、重置 Key、删除模型和推理测试须 approved；复制 Key 须 copy_keys。复制返回新供应商工作台，删除返回 deleted=true，其余返回操作后的工作台。删除供应商在同一事务清理其全部模型候选及价格，保留其他供应商的同名模型、顺序和元数据，存续模型 revision 加一；失去全部候选才删除公开模型，已启用 Binding 默认模型引用仍返回 referenced 并整体回滚。停用只保留配置并暂停调度，不删除映射。模型映射修改只增加模型 revision；配置/Key/模型协议覆盖/策略/价格写入增加供应商 revision；test、key-reset、price-import 不修改配置 revision。冲突/结果未知先重读，不自动重放；取消前端消费不等于取消已发出的写入。

目录查询可指定启用的已存 Key，不存在的 Key 返回 not_found 且不请求上游；指定停用 Key 返回 no_enabled_keys，不抓取目录或分组倍率。省略时自动查询全部启用的已存 Key 并合并模型，列出同名模型的全部可选 Key。认证使用 Key 默认协议，模型覆盖不影响目录认证。仅无已存 Key 时使用匿名目录，全部 Key 停用时不回退匿名。推理测试可以明确指定停用/降权 Key；未指定则使用首个启用 Key，无 Key 才匿名。目录查询不更新健康；真实网关结果和显式推理测试更新健康。状态接口 upstream_health=per_key 提示从供应商工作台查看，不能视作总健康结论。

## 6. Key / 模型协议（2026-10-11，desktop.16 dev 已发布）

协议归属 Key，Action.protocol 使用 responses / chat-completions / anthropic。key-put 省略或空 protocol 保留已有 Key 默认值，新 Key 默认 responses；secret 可空以保留凭据，因此支持纯协议编辑。key-model-protocol 使用 target=Key ID、model=实际模型名、protocol=独立覆盖；空值或 inherit 删除覆盖。两种写入均要求当前供应商 revision，成功加一，失败事务回滚；不改变公开模型映射。

转发先选 Key，然后按该 Key 和候选中的实际模型名解析覆盖 → Key 默认值，JSON、SSE、认证与推理测试均使用此结果。模型未设置覆盖时动态继承，以后修改 Key 默认值自动生效。供应商 protocol 仅保留为核心内部值，用于历史兼容、新 default Key 初始化与匿名请求，不再控制已有 Key，桌面不显示也不设置。schema3→4 把每个供应商旧协议复制到它的全部 Key；复制凭据时一并复制覆盖，改名/删除按外键级联。升级须同步客户端与核心；旧程序无法打开 schema4，不提供降级。

UI 的 Key 编辑支持默认协议与留空保留凭据；当前源码模型页默认查询全部启用 Key，跳过停用 Key；同一模型一行展示全部可选 Key、分组倍率和各自独立协议。全部停用时提示已跳过查询，已存协议覆盖不作为目录确认的可选 Key。已存覆盖在目录尚未查询或不再返回该模型时仍可查看和恢复继承。切换供应商、修改 Key 凭据或删除 Key 时清理目录结果；模型协议写入保留当前目录，方便连续编辑。前端校验模型与 Key 关联、部分失败及倍率；显式 CLI 查询仍须匹配指定 Key。2026-10-13 此聚合与倍率增量尚未打包，desktop.16 原有 Key 协议功能已发布；原生安装未验收，安装包能力以[发行记录](../release/desktop.md)为准。


## 7. 共享公开模型映射与调度（2026-10-12）

Action 新增 multiplier；model-put 使用 strategy=round-robin|priority 设置公开模型调度，和 key strategy 动作分属两个层级。ModelMapping 新增 multiplier（始终有效正数，旧值0回读为1）及 strategy（旧模型空值回读为priority）。Go/TS 和 `desktop/contract/provider-model-mapping.json` 为共享契约。

model_revision=0 表示当前供应商加入别名，不覆盖它已有的映射；别名不存在则创建并默认round-robin，已存在则追加并保留其他候选、能力、策略及Codex元数据。正 revision 编辑须匹配共享模型 revision，只改当前供应商上游/倍率；tools/streaming/strategy 为共享模型字段。追加写入仍以读取的最新模型 revision 进行CAS及供应商 revision 事务校验，不自动重放并发冲突。model-delete 事务内只删当前供应商候选与价格，剩余模型 revision+1；最后一个候选才删除公开模型、清理模型价格并检查 Binding 默认模型引用。

round-robin 在核心按权重1/multiplier执行平滑加权轮询，默认倍率1，范围0.01..1000；只在当前可调度候选中分配首选上游，不预占Key。真正转发时再次核对并发及Key状态；失败转移仍遵循原有禁止未知结果/流式重放规则。倍率不从外部站点拉取，目录成功不表示可推理。

## 8. 平台账户与计费兼容层（2026-10-13，当前源码）

本节是核心／客户端／源码 CLI 已实现的兼容契约，核心已随 desktop.21 dev 发布；desktop.22 已将平台配置、账户余额与 Key 额度纳入桌面 DTO、Wails 方法和供应商详情「账户与余额」页签；逐请求账单及独立倍率查询仍仅核心／源码 CLI，alpha.6 CLI 不含这些命令。当前源码模型目录入口已复用本兼容层抓取并展示 Key 分组倍率。CLI 完整语法见[指令大全](../cli/commands.md#平台兼容查询源码-clidesktop21-核心已发布)。

### 身份、入口和返回

平台类型 `auto|sub2api|newapi|oneapi` 独立于 Key 的推理协议。`provider_platforms` 表按 provider_id 保存公开配置及独立 user_token，schema4→5 自动迁移；既有供应商缺少平台配置或显式 `auto` 时自动探测；具体平台值仍可作为人工覆盖。旧程序不能打开 schema5，升级需同步核心与客户端。

- `GET /control/v1/providers/{id}/platform`：返回 `providerapi.Settings`，仅 `has_user_token`，不返回用户授权。
- `PUT /control/v1/providers/{id}/platform`：`providerapi.Configure`，包含 provider_id、expected_revision、完整公开 config 及可选 user_token。nil 保留同目标授权，空字符串清除；平台、site_url、user_id 或推导后台根地址改变时，不隐式迁移旧令牌。写成功递增供应商 revision。
- `POST /control/v1/providers/{id}/query`：`providerapi.Query`，operation 为 `models|detect-platform|balance-summary|balance|key-quota|usage-records|effective-rate`；provider_id 必须与路径一致，expected_revision 必填。records 的 scope 为 account（默认）或 key；page 从1开始，默认1，page_size 默认50且最大100。remote_key_id 是用户提供的站点数字 ID；正整数不会证明与本地 key_id 对应。effective-rate 在 remote_key_id=0 时会使用账户列表核对所选保存的完整密钥，NewAPI 掩码须经固定 POST /api/token/{id}/key 读取完整密钥后才能匹配；其余操作的数字 ID 语义不变。
- 统一核心方法 `Server.QueryProvider` 负责读取供应商、授权与 Key，校验 revision、限制并发／超时和选取平台。`providerapi.Adapter` 提供 `GetBalance`、`GetKeyQuota`、`ListUsageRecords`、`GetEffectiveRate`；models 复用核心现有目录读取器。Go 客户端 `ProviderPlatform`、`ConfigureProviderPlatform`、`QueryProvider` 与 CLI 复用这些管理入口。

所有管理路径沿用管理鉴权、Host／Origin 规则；PUT／POST 校验实例，不能让浏览器绕过宿主直接请求。query 虽用 POST，业务上只读；不写本地消费估算、不调整路由倍率、不自动导入目录。查询未指定 key_id 时优先选择已启用的 `default`，否则回退到第一个已启用 Key；用户授权查询不要求推理 Key 存在。模型仍受所选 Key 默认推理协议认证约束，不通过平台身份强行改变协议。

`Result` 包含 provider_id、实际选用的本地 key_id（没有时 anonymous）、用户提供的 remote_key_id、revision、platform、operation、observed_at、source 路径、http_status，以及操作对应的 payload 或 failure。models 的 source 为 `model_catalog`（表示复用目录读取器），其他为真实请求的端点路径；不含站点 URL、查询秘密、原始错误正文。管理错误使用原管理错误响应；平台能力或上游失败作为结构化 Result，不返回部分数据。

### 端点选择与认证

后台根地址取显式 site_url，否则将供应商首个 Base URL 去掉末尾 `/v1`，保留自定义部署前缀。不会凭空添加 `/newapi`；示例中的 `/newapi` 只是用户 URL 自带的代理部署前缀。模型目录沿 §4 的版本路径／回退规则，不使用后台根地址规则替代。

| 内部操作 | sub2api | NewAPI | 原版 OneAPI |
| --- | --- | --- | --- |
| models | 既有目录读取器，所选推理 Key | 同左 | 同左 |
| balance | 用户 `GET /api/v1/user/profile`；无用户授权时 Key `GET /v1/usage`，仅明确钱包 balance 可用 | 优先用户 `GET /api/user/self`；无用户授权时 Key `GET /api/usage/token/` 的可选 user_quota，其次旧 billing，最后可用的只读 Key 额度 | 用户 `GET /api/user/self` |
| key-quota | Key `GET /v1/usage` | Key `GET /api/usage/token/`，兼容其布尔 `code:true` 成功封装 | 用户 `GET /api/token/{remote_key_id}` |
| usage-records account | 用户 `GET /api/v1/usage?page=&page_size=` | 用户 `GET /api/log/self?type=2&p=&page_size=` | 用户 `GET /api/log/self?type=2&p=`，内部把页号转换成从0开始 |
| usage-records key | 同上，加 api_key_id 筛选 | Key `GET /api/log/token`，只提供近期批次 | unsupported_key_filter，不使用可能重名的 token_name 筛选 |
| effective-rate | Key 直接读取 `/v1/sub2api/billing`；旧站回退或显式 remote_key_id 时用户读取 `/api/v1/keys/{id}`、`/api/v1/groups/rates`、必要时 `/api/v1/groups/available` | 用户读取 `/api/token/{id}`、必要时 `/api/user/self`、`/api/pricing` | unsupported |

推理 Key 与用户令牌均放 Bearer 请求头；NewAPI 用户授权若配置 user_id 则加 `New-Api-User`。sub2api 用户令牌为登录 JWT；NewAPI/OneAPI 用户接口需用户级 access token，不把 `sk-` 推理令牌当后台授权。不要请求管理员接口或将上游原始日志转发到桌面。

### 数据口径与能力缺口

`Amount` 保留高精度 JSON 数字与明确单位。sub2api 为 USD；OneAPI 的账户与 Key 原始额度为 quota。NewAPI 账户与 Key 的原始 quota 按公开 `/api/status` 的 quota_per_unit 及 quota_display_type 换算：USD 为 quota / quota_per_unit，CNY 再乘站点 usd_exchange_rate；TOKENS、未知显示类型或换算参数缺失／无效时保留 quota，不猜测倍率或汇率。NewAPI billing 已由站点换算，仅确认 USD/CNY/quota 单位，无法确认则 site_unit，不二次换算。GUI 展示统一四舍五入两位小数，用十进制字符串运算避免大额度及1.005等边界的浮点误差；底层与CLI保持精度。`Allowance` 显式区分 account/key/billing（站点决定账户或 Key 范围），remaining、used、limit、unlimited 和窗口缺失均可为 null。sub2api 订阅 remaining 或 Key cap 不能当账户钱包余额；unrestricted 表示 Key 本身不受额度上限限制，不表示账户余额无限。原版 OneAPI 旧 billing 接口受站点开关影响其范围／单位，兼容层不依赖它。

账单只保留消费记录：sub2api `actual_cost` 为 charged_amount，`total_cost` 为 base_amount；NewAPI/OneAPI `quota` 为此次请求结算的内部额度。NewAPI 若返回 billing_source 则保留 wallet/subscription，并保留 wallet_quota_deducted、subscription_consumed 等数值，不能把订阅消耗当作钱包扣款。token 输入、输出、缓存读／写、5分钟／1小时写入独立返回，缺失保留 null。原版 OneAPI 的 prompt_tokens 在部分非文本计费路径可能承载其他含义，统一标记 `platform_fields_unverified`，不保证实际模型 token。NewAPI 日志 id 可被重新编号，`id_stable=false`，request_id 仅在站点返回时提供。

`tool_charges` 保留 NewAPI 已知内建工具的名称、次数和站点原始 price（不转换币种）；未知工具名或附带正文不复制。`details` 仅含已知数值白名单：sub2api 费用分项、图像 token／费用／数量、billing_type；NewAPI 缓存分项及倍率、音频／文本 token 与倍率、图像、工具计费字段、订阅计费数值，以及 `billing_tokens.p/c/len/cr/cc/cc1h/img/img_cr/img_o/ai/ao` 等阶梯计费实际计费 token 字段（仅原站返回时）。兼容常见分支的缓存字段别名。不返回原始 other、content、提示词、IP、admin_info、表达式或 request_rules，未知字段不复制。

records 的 history 区分近期批次 `recent` 和可分页保留记录 `paginated_retained`；不是永久全历史。NewAPI Key 日志只支持第1页，批次大小由站点决定，total=null / has_more=false / page_size=0。原版 OneAPI 的服务器固定 page_size 不可从响应确认，返回0，需请求空末页确认结束。其他分页返回站点 total；默认日期范围／日志保留由上游决定。

当前倍率按 remote_key_id 的绑定分组查询：sub2api 个人覆盖优先，再读可用分组默认倍率；NewAPI 使用用户身份下 pricing 的 group_ratio（覆盖后的倍率），空 Key 分组继承用户分组，auto 可能按请求变化因此返回 unknown。历史账单 rate_multiplier 是上游报告值，NewAPI 同时保留 group_ratio / user_group_ratio，不能据当前倍率重算历史实际扣费；OneAPI 普通用户没有可靠的当前有效倍率接口。

账户查询单次最多8个 GET、累计2 MiB响应、每页最多1000条原始记录，统一供应商超时与8秒的较小值，全局与供应商并发共用现有核心限额。禁止重定向、沿用核心不使用环境代理的上游策略，不重试；登录过期、权限不足、限流、超限、格式变化或分页失败返回分类失败。模型目录沿 §4 自身更小的响应限制。未提供本轮未勾选的用量汇总、完整分组列表、模型价格目录和订阅列表；不将站点目录声明当作推理健康或自动负载均衡依据。

sub2api 所选推理 Key 的倍率查询（remote_key_id=0）优先使用 `/v1/sub2api/billing`，无需 JWT；`basis=key_billing` 的 multiplier 取服务端 `effective_rate_multiplier`，包含个人覆盖和当前高峰规则，不在客户端重算。组名与 ID 未披露则为空。正整数远端 ID 查询仍需用户授权，避免将保存 Key 的倍率误当作另一远端 Key 的倍率。仅缺接口（404/405或200 HTML）回退旧查询；格式错误、拒绝授权、限流、网络和服务错误不回退。auto 探测增加 object/schema_version/billing_scope 强识别并复用本次响应，避免重复计费接口请求。

### 来源与验证边界

实现对照固定修订的原始项目路由／处理器，分支或站点定制可能与其不同：

- [sub2api ab80abc](https://github.com/Wei-Shaw/sub2api/tree/ab80abc6d1f929a6864c8804fd077b509d21fda2)：gateway usage、用户 routes、usage handler、DTO、API Key handler。
- [NewAPI 633b4c5](https://github.com/QuantumNous/new-api/tree/633b4c5b190dc1af99a2814391354d86948e7943)：token/log/pricing controller、API router、公开日志计费信息及 text_quota 的工具附加费／缓存字段。
- 2026-10-19 再核对 [NewAPI 官方认证文档](https://docs2.newapi.pro/en/docs/api/management/auth)、[用户信息](https://docs2.newapi.pro/en/docs/api/management/user-management/user-self-get)、[令牌用量](https://docs2.newapi.pro/en/docs/api/management/token-management/usage-token-get)，以及当日最新 [NewAPI 058c823](https://github.com/QuantumNous/new-api/tree/058c8231b68c51ee02abee15befcd720fee9f4a2) 的 controller/token.go（可选 user_quota）、billing.go、misc.go（站点换算参数）和 middleware/auth.go（只读认证）。分支／旧站未提供 user_quota 时沿用上述兼容路径。
- [原版 OneAPI 8df4a26](https://github.com/songquanpeng/one-api/tree/8df4a2670b98266bd287c698243fff327d9748cf)：user/token/log/billing controller 与 log model；不将各分叉能力当成原版保证。

使用隔离 SQLite 与模拟 HTTP 站点验证认证、版本、请求路径、分页、金额精度、字段缺失、上游失败、体积限制和脱敏，以及 CLI／客户端完整调用链。没有使用真实站点令牌，没有调用付费推理，没有完成原生桌面接入或发行验证。

### 桌面账户查询契约（2026-10-13）

Go `internal/desktop/contract.go` 是唯一 DTO 源，生成 TS；共享样例为 `desktop/contract/provider-account.json`。

- `ProviderAccountSettings(WorkspaceQuery)` 读取公有平台配置；`SaveProviderAccount(SaveAccountSettings)` 完整写入 platform/site_url/user_id，user_token 为 null 保留同目标授权、空字符串清除、非空替换。只返回 has_user_token，提交后清空表单令牌；清除须勾选确认，改变授权目标保留模式会明确提示旧令牌被清除。
- `ProviderAllowance(AccountQuery)` 接受 balance / key-quota / balance-summary，通过现有 client.QueryProvider 复用核心端点、鉴权、并发和超时。校验当前 provider revision；Key 留空时优先已启用 default，再回退首个启用 Key，OneAPI Key 额度需用户核对并输入远端整数令牌 ID。
- 标准桌面 envelope 的 Problem 表示管理／连接失败；AccountObservation.failure 表示平台查询失败。账户 scope=account，Key scope=key，NewAPI 旧 billing scope=billing（站点决定查询范围）；amount.value 为十进制字符串，unit 保留 USD/CNY/quota，币种不明确时为 site_unit，不进行价格换算；null 为未知，unlimited=true 仅说明 Key 自身无限额，不能推导账户钱包。
- 读取、写入、额度查询共享页面操作互斥；配置或供应商变化取消／丢弃旧查询，改变 Key 或远端 ID 清空旧观察值。余额与 Key 查询各自更新对应结果，刷新失败不把旧数值当当前结果。写入结果 unknown 或 conflict 需显式重新读取，不自动重放写入。
- 后台根地址为空时沿用核心推导；可选地址仅由用户按真实部署填写，不自动加 `/newapi` 等平台名路径。查询不发推理请求，不将余额写入路由配置；普通本地 Key 倍率查询会缓存有效分组倍率。

### 自动探测与供应商列表余额（2026-10-13，当前源码未发布）

`detect-platform` 强制重新探测，只返回观察结果，不持久化平台或递增 revision。未配置／auto 的其他查询先探测：公开 `/api/status` 以版本、quota_per_unit 和 NewAPI／OneAPI 特有字段区分；公开 `/api/v1/settings/public` 以 sub2api settings 字段识别。公开探测不发送凭据；必要时用所选推理 Key 检查 `/v1/usage` 与 `/api/usage/token/` 的结构。域名、站点名称或推理协议不作为识别依据，无法确认则 platform_unknown；网络、超时、限流和响应预算失败立即停止。沿用部署前缀、禁止重定向，全部探测与查询共享8次请求／2 MiB／8秒上限。

`balance-summary` 供列表自动读取；NewAPI 已保存用户授权时优先使用系统访问令牌查 `/api/user/self`，返回 scope=account，与详情 `balance` 一致。账户认证、权限或接口失败直接报告，不退回 Key 数值；没有启用推理 Key 也可用明确配置的平台和系统访问令牌查询账户。其余路径使用推理 Key。sub2api unrestricted 的明确 balance 返回 account 钱包；受限 Key 返回 key 额度及周期窗口。NewAPI 先读取只读 `/api/usage/token/`：仅当站点在 Key 允许查询用户额度时返回有效 user_quota，才标为 account，其账户字段不会继承 Key 的 unlimited_quota。若未返回账户字段，尝试 `/v1/dashboard/billing/subscription` 与 `/v1/dashboard/billing/usage`，剩余值为 `hard_limit_usd - total_usage/100`，精确十进制运算；范围由私有站点开关决定，因此 scope=billing。公开显示类型确认单位，未知为 site_unit；无限额100000000占位值仍须核对 Key，不当作钱包。billing 缺失、格式不兼容或拒绝认证／权限时，可回退本次已成功读取的 Key 额度（scope=key）；已耗尽／过期 Key 在只读接口仍获准查询时可显示额度，禁用／IP限制仍由站点执行。限流、网络及服务错误不降级吞掉；显式系统访问令牌失败不回退 Key。NewAPI balance 与 balance-summary 共用以上策略，key-quota 只返回 Key 范围。auto 探测复用 token 响应，所有请求仍共享8次／2 MiB预算。其他平台 balance 的账户钱包语义不变。用户授权可在详情配置。NewAPI 在该站后台「个人设置／安全设置 → 系统访问令牌」生成独立用户授权，与推理 API Key 分开；GUI 显示获取步骤。支持 scoped token 的上游版本，账户余额读取需 profile:read，自动核对 Key 分组还需 api_key:read、api_key:reveal；用户 ID 按站点要求填写。不同部署版本名称和权限配置以该站为准。缺失值保持 null，失败不伪造0。

桌面沿用 anchor-desktop-v1、既有 ProviderAllowance 和 AccountObservation；新增 operation 允许 account 或 key scope，不增加任意 HTTP 桥接。共享样例包含钱包与 Key 两种 summary。列表 Controller 每轮最多并发2个查询，无轮询，列表刷新触发新一轮；供应商、revision、传输实例或导航变化取消／丢弃旧观察。View 显示平台、scope、原单位与查询状态，悬浮提示含所选 Key 和时间；周期额度标注实际周期。账户页默认自动探测，可选具体平台覆盖。

2026-10-13 当前源码的普通 effective-rate 查询已复用模型目录的有界 Key 核对逻辑：最多4页、每页100条，所有列表、核对和倍率查询共用8次请求／2 MiB／核心超时预算。平台认证仍为用户授权；当前源码进一步将本地 Key 自动核对查询持久缓存，按供应商倍率×分组倍率调度；显式远端数字ID不写缓存，旧模型倍率字段仅预留。桌面 ProviderRate 在账户页可选 Key 查询，模型映射页按来源 Key 逐项查询并标明 Key 与来源。原版 OneAPI 返回 unsupported，NewAPI auto 为 unknown，未知和失败不会填成1。


## 9. 来源 Key与模型调度（2026-10-13，当前源码）

模型 candidates 新增可选 key_ids：空／缺失表示所有启用 Key，非空仅允许列出的本机 Key ID。写入检查 Key 属于候选供应商，去重且最多64项；anonymous 仅能用于没有 Key 的供应商。目录导入保留实际 entries.key_ids，禁止调度到目录未报告的其他 Key。手动停用与并发门禁照常生效，故障只降权（见下方自动降权设置），指定 Key 都不可用时切换其他候选，不越过范围。删除 Key（包括清除 default 凭据）在同一事务移除该供应商候选中的对应 key_ids；仍有其他来源 Key 时保留候选，列表耗尽则移除候选，禁止将空列表写回并扩大为全部 Key。不限 Key 的候选在仍有已存 Key 时保留，删除最后一个已存 Key 时移除，避免隐式变为匿名来源。其他供应商及其他 Key 提供的同名模型、候选元数据与顺序均保留；受影响的存续模型 revision 加一，仅当前供应商已无候选时清理其价格。公开模型失去全部候选才删除；已启用 Binding 默认模型引用仍返回 referenced 并整体回滚，已禁用 Binding 不阻止删除。该行为于 2026-10-19 在当前源码实现，尚未打包发布。schema5→6 顺序迁移保留模型、供应商与凭据，无新表；schema版本保护防止旧核心忽略 Key 限制。

Workspace.models 新增 key_ids（始终数组）和 candidate_index（0开始，核心真实顺序）；revision／strategy／candidate_count 仍描述共享公开模型。多个平台可以映射同一别名；一个候选可包含多个 Key，具体 Key 沿用该供应商的 priority／round-robin 策略。

新增管理 `POST /control/v1/models/{id}/routing`，复用管理鉴权与实例校验；输入为 model_id、expected_revision、strategy 和完整 order（provider_id／model）。只能排列已有候选，拒绝遗漏、重复、增加或替换成员；保留每个候选的 Key、倍率及全部模型元数据。原子提交递增模型 revision，不递增供应商 revision。响应为 ModelRoutingState；旧 revision 为 conflict，不自动重放。客户端 ConfigureModelRouting、CLI model routing 和桌面 Bridge.ConfigureModelRouting 共用此实现；共享样例 desktop/contract/model-routing.json，TS 由 Go DTO 生成。宿主15秒超时，前端取消仅丢弃结果，未知写结果必须重新读取。

| strategy | 行为 | 无观察或失败时 |
| --- | --- | --- |
| priority | 固定保存顺序 | 健康／并发门禁与失败切换照常 |
| round-robin | 按供应商／模型／Key展开，权重为供应商倍率×Key缓存分组倍率的倒数，平滑加权轮询 | 跳过不可用候选，revision变化重置权重债务 |
| cache-priority | 同一认证调用者／模型／instructions／首个input项复用最近成功供应商、上游模型和Key，TTL10分钟 | 没有有效亲和时加权轮询；降权降低亲和首选频率，配置变化或失败时切换 |
| lowest-price | 按供应商倍率×Key缓存分组倍率升序，相同倍率按保存顺序 | 不推断美元单价；未知作回退，全部未知等权，零倍率优先 |
| lowest-latency | 先按保存顺序学习无观察来源；有观察后按成功请求首字耗时的EWMA升序（旧值80%／新值20%），无首字时总耗时 | 观察TTL30分钟，过期再学习；相同耗时按保存顺序 |

观测只在内存保存，重启后清空；亲和最多1024项，延迟最多4096项，按模型和供应商revision隔离，删除过期后仍满则淘汰旧项。亲和只保留SHA256摘要，包含授权调用者与最大64KiB的稳定请求片段，不保存原始令牌或正文、不额外发探测推理。只使用成功请求观测；不是上游缓存命中承诺。成功采样、不可用过滤和实际转发前的Key健康／并发复查由核心完成。
2026-10-13 当前源码的普通 effective-rate 查询已复用模型目录的有界 Key 核对逻辑：最多4页、每页100条，所有列表、核对和倍率查询共用8次请求／2 MiB／核心超时预算。平台认证仍为用户授权；当前源码进一步将本地 Key 自动核对查询持久缓存，按供应商倍率×分组倍率调度；显式远端数字ID不写缓存，旧模型倍率字段仅预留。桌面 ProviderRate 在账户页可选 Key 查询，模型映射页按默认启用 Key 查询并标明 Key 与来源。原版 OneAPI 返回 unsupported，NewAPI auto 为 unknown，未知和失败不会填成1。

## 实时推理测试生命周期（2026-10-13，当前源码）

`POST /control/v1/providers/{id}/probe` 复用管理鉴权、实例身份与 revision 校验。`catalog.ProbeRequest` / `ProbeProgress` 是核心规范源，桌面 `contract.go` 引用并由 Go 生成 TS；共享样例为 `desktop/contract/provider-probe.json`。Wails `ProviderProbe` → 类型化 client → 管理 API；没有任意 URL、临时秘密或 shell 入口。原有 `provider test` / `ProviderAction(test)` 同步契约保持兼容，异步生命周期服务于弹窗显示。

- 输入固定字段为 provider_id、expected_revision、action、key_id、model、run_id、approved。`start` 要求 approved=true、非空模型和明确的启用 Key；无已存 Key 可指定 anonymous，已存 Key 全停用时禁止匿名回退。`read` / `cancel` 使用开始返回的不可猜测 run_id；仅允许同供应商及 revision 读取，冲突在操作前失败。重复 start 是另一请求，前端同步锁禁止连点，不自动重放未知开始结果。
- 输出固定字段为 provider_id、revision、run_id、key_id、model、status、phase、text、elapsed_ms、result。status 为 running / completed / cancelled；phase 为 connecting / receiving / complete；运行中 result=null，终态 result 为原有 Probe。耗时是毫秒；输出文本累计快照至多 16 KiB。前端运行中每150ms串行读取一次，终态／关闭／卸载停止轮询；迟到的开始结果取得 run_id 后仍发取消，取消不确认时由核心硬超时兜底。
- 最小请求仍是「Reply OK.」、max_output_tokens=16、store=false。异步测试增加 stream=true；使用所选 Key 的实际模型协议、已有协议流转换器，以及与同步探针相同的并发、最多10秒期限、健康和累计用量。真实 SSE 文本逐段可见；上游忽略流式并返回 JSON 时，校验后显示一次完整文本。EOF 缺终止事件、非法流和 incomplete 返回固定 invalid_response，错误正文不显示。
- 核心最多保留64次瞬态运行，开始一分钟后删除；停止取消上游 context，核心关闭取消并等待任务退出。文本只在运行内存／当前弹窗保留，不进入日志、数据库或浏览器存储。仅提取 output_text，不展示原始事件、思考内容、请求头或上游错误；所选 Key 凭据完整回显及跨片段前缀均被过滤。保留现有 Probe DTO 字段；新增的网页响应失败分类见下方推理地址兼容说明。
- 目录只说明 Key 可列出该模型，不保证推理或流式成功。前端目录错误／空目录禁用发起按钮；Key 切换清空选择后重新查询，不从供应商全量映射猜测 Key 权限。关闭快速测试保持列表、关闭详情测试保持详情。

Key 专用倍率接口另对照 sub2api 固定修订 `4d9ac3e482106d2da3a39fe3e546d8d6801f1cc1` 的 `backend/internal/handler/gateway_key_billing.go` 和 `backend/internal/server/routes/gateway.go`。隔离模拟验证不代表所有历史部署版本已提供该端点。

## models.dev 模型参考方法（2026-10-13，当前源码）

`ModelCatalog(ModelCatalogQuery)`／`EstimateModelCost(CatalogEstimateInput)` 是与供应商工作台独立的只读桥接方法，不要求 Anchor 供应商 ID 或 revision。使用精确 models.dev provider/model；不能将通用声明视为所选 Key 的已验证能力。基础价导入复用同一目录缓存，仍仅预览和手动保存。完整 DTO、未知／陈旧语义与计价口径见[模型参考目录](model-catalog.md)。


## 推理地址兼容与网页响应诊断（2026-10-14，当前源码）

同步 `provider test`、桌面实时 Probe 和真实三协议转发共用端点解析：优先按配置基址追加协议端点，仅在无版本后缀的基址返回 HTML 且 HTTP 为 200／404／405 时，在原部署前缀后追加 `/v1` 重试一次。结合 Content-Type 与最多512字节正文前缀识别网页；即使响应头为 HTML，也须确认正文前缀，避免重放标错类型的 JSON 响应；正常 JSON 遇到首个可判定字符即停止嗅探，所有已读字节保留给后续解析。不会无条件追加版本、跨域探测、修改数据库或缓存猜测结果；每次调用独立判断。

重试沿用原方法、正文、Key、请求头、context 截止时间和并发名额。已带版本后缀、JSON 错误、鉴权失败、限流、重定向、网络失败、超时和已标识的 SSE 不自动重放。补路径仍是同一次逻辑推理，累计用量和日志 attempt 计数保持1，`endpoint_requests` 单独记录实际端点请求数1或2。

若最终 HTTP 200 正文仍为 HTML，Probe `failure` 与网关错误码新增 `unexpected_html`；Probe 保留 `http_status=200`，网关向调用方返回502，并提示检查 API 基址（通常以 `/v1` 结尾）。其他 HTTP 错误保留原分类。日志 Evidence 增加 `endpoint_resolution=configured|v1_fallback`、`endpoint_requests`、`response_kind=html|api_or_unknown`、受限的 `upstream_content_type`；执行补路径时增加 `initial_response_kind=html`、`initial_http`。这些字段不保存 URL、凭据、页面或错误正文。共享样例 `desktop/contract/provider-probe.json` 覆盖 HTTP 200 的 HTML 失败；DTO 字段结构未变，Go／TS 需共同接受新错误枚举。

## 供应商与 Key 缓存倍率（2026-10-15，当前源码）

供应商新增 `cost_multiplier` 默认1，桌面编辑与 CLI 可设置；旧模型倍率输入撤下，字段保留但不参与调度。完整公式、DTO、schema8、缓存失败保留与凭据失效规则见[倍率设计](provider-multipliers.md)。本文早期未缓存观察的行为由此替代。


## Anchor 固定速度档位与分身（2026-10-15，源码未发布）

`model-put` 新增可选 `service_tier`／`speed_variants`，Workspace 映射新增可选固定档位。普通映射与所选速度分身事务保存，核心负责校验、隔离供应商渠道及请求参数注入；前端只管理草稿并调用原有类型化 action。结构、空值、revision、原子冲突与共享样例见[速度分身契约](model-speed-variants.md)。模型专用 ID 校验允许保留速度后缀，不扩大其他用户 ID 的字符范围。GUI 和 CLI 共用同一核心路径，不调用真实付费上游来验证。


## 多轮对话测试（2026-10-16，当前源码未发布）

新增受控 `ProviderChat` 桥接与核心 `/control/v1/providers/{id}/chat`；start/read/cancel 使用 `ChatRequest` 与既有 `ProbeProgressResult`，只允许所选已存供应商、准确 Key 和纯文本多轮消息。固定目标、不自动重试、不保存聊天正文；可选 `ChatRequest.reasoning_effort`（空／省略为默认），按所选 Key 的实际模型协议发送原生参数，UI 修改深度不清空上下文。沿用管理鉴权、实例核验和协议转换。既有 Probe/CLI test 不变，旧宿主／核心明确不支持，不回退 Mock。限制、取消、脱敏和共享样例见[对话测试契约](chat-testing.md)。

## 出站选择（proxy-v1）

供应商协议、认证、模型名不包含代理实现；`internal/egress` 根据 provider ID / 实际 Key ID 解析独立绑定。Key 可以继承、直连、指定 profile。核心将路由上下文传给推理、catalog 和 providerapi 账户客户端；适配器不关心 mihomo。代理连接错误保留 fail-closed 行为，不因既有候选故障切换逻辑绕到直连。管理及桌面契约见 [代理设置](proxy-settings.md)。


## Key 自动降权设置

2026-10-19 当前源码未发布，契约沿用 `anchor-desktop-v1`。Go 规范源 `internal/desktop/contract.go`（类型别名来自 state）；共享样例 `desktop/contract/key-cooldown.json`，TS 由生成器派生。

- `Bridge.KeyCooldownSettings()` → client → `GET /control/v1/settings/key-cooldown`；`Bridge.SaveKeyCooldownSettings(input)` → `POST` 同一路径。保留方法及路径名；沿用所选本机／SSH 核心、管理鉴权、实例核验和 15 秒桌面调用上限，不访问真实上游。
- 返回 `KeyCooldownSettings`：`enabled` 布尔、`weight_percent` 整数 1..100、`failure_threshold` 整数 1..100、`cooldown_seconds` 整数 1..3600、`rate_limit_seconds` 整数 1..3600、`revision` 正 JS 安全整数。默认 true／10／3／30／30／1，持久化于当前核心 meta，不包含 Key 或管理令牌。旧持久设置缺少比例时补 10。
- `SaveKeyCooldownSettings` 要求原四个非空字段及 `expected_revision`（1..MAX_SAFE_INTEGER-1）；新增可选 `weight_percent`，省略／null 保留已存比例。条件写入成功 revision +1，沿用 `{contract,data,problem}`；冲突不改设置或健康，读失败不回退 Mock，写入不自动重试。新前端总是发送比例并验证回包全部字段及 revision；无法确认时返回 unknown，先读取再操作。旧核心缺字段时显示连接异常，需同步升级。
- 全局策略作用于所有供应商（含匿名来源）。连续失败达到阈值，或 401/403/429 后立即降至指定正权重；429 优先有效 `Retry-After`，否则用配置时长，限制 1 秒..1 小时，仅影响降权期限。成功或到期恢复；不再永久认证封禁、硬冷却或独占探测。只有一个／全部 Key 降权时仍可调度，手动停用与并发门禁保留。
- `KeyStatus.state` 新增 `degraded`（可调度），`retry_at` 表示恢复正常权重的 Unix 秒时间。100% 不降低权重，状态为 ready。新核心不再产生 invalid/cooling/half-open；前端仍能展示旧核心的旧状态，但不假设旧核心具有新调度行为。
- 轮询将健康因子乘入调度权重，不改变计费倍率；有序规则（优先级、最低价格、最低延迟、缓存亲和）降低故障来源的首选频率并保留全部失败转移候选，相同健康因子保留原偏好。轮询的已知／免费来源优先以最高健康因子的来源为准。降权不重复请求同一 provider/key/model，不改变流式结果和未知结果的重放约束。
- 保存与请求结果归约串行，成功保存清除失败计数／临时降权／调度债务，保留最近测试，不修改供应商 revision。关闭时所有自动降权停止；核心启动读取策略，内存保留最近成功策略，结果归约不再因读取失败启用默认策略。启动读取失败时不自动降权，设置读取仍返回真实错误。重启保留策略，清空临时健康。
- CLI 同能力为 `settings key-cooldown`；详见[指令大全](../cli/commands.md#16-key-自动降权设置当前源码未发布)。
