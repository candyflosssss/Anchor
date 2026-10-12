# 供应商桌面接口语义

- 更新：2026-10-12；标识：`anchor-desktop-v1`（替代仅供设计的 `provider-ui-prototype-v1`）。
- 状态：真实 Go 桥接、桌面传输和共享契约已实现；隔离核心读写已测试，**不等于 Wails 原生窗口或安装验收通过**。
- 桌面 DTO 权威：`internal/desktop/contract.go`；TypeScript 由 `go run ./cmd/desktop-contract` 生成，漂移由 Go 测试检查。核心业务规则仍归 `internal/state` / `internal/core`，不复制到宿主。
- 受限方法、窗口／核心生命周期与构建边界见[桌面运行契约](desktop-runtime.md)。

## 1. 读写与数据

| 前端端口 / Wails 方法 | 真实客户端与行为 |
| --- | --- |
| `list(signal)` / `Bridge.Providers()` | `client.Providers` → GET 当前实例目录；只回脱敏 DTO，空目录固定为空数组 |
| `discover(input, signal)` / `Bridge.DiscoverProviderModels(input)` | `client.DiscoverProviderModels` → POST `/control/v1/providers/{id}/discover-models`；显式读取上游目录，不改配置 |
| `save(input, signal)` / `Bridge.SaveProvider(input)` | `client.PutProvider` → PUT 指定 ID；revision=0 创建，>0 条件更新；普通字段完整替换 |

Provider 字段固定为 `id`, `homepage_url`, `base_urls`, `enabled`, `timeout_seconds`, `max_concurrent`, `revision`, `has_secret`。`homepage_url` 可空，仅展示；`base_urls` 为 1..16 个不重复请求地址，网关按请求轮询，目录与测试使用第一个。供应商不再在 DTO 暴露 protocol，协议由 Key 与模型决定；核心内部保留供应商 protocol 仅用于新 default Key 初始化与匿名请求，桌面保存时空值保留已存值、新建按 `responses`。不添加健康、余额、最近测试时间、display_name 等没有来源的字段。协议接受 `responses`、`chat-completions`、`anthropic`，未知协议拒绝。Go 内部 Secret 不进入 DTO，前端传输再次挑选公开字段。

- 返回 revision 范围 1..Number.MAX_SAFE_INTEGER；请求 expected_revision 为 0..MAX_SAFE_INTEGER-1，且与输入 provider.revision 一致，防止更新后超出 JS 安全整数范围。
- 超时单位为秒，1..3600；并发为 1..64。前端即时校验只帮助填写，核心仍做最终业务校验。
- 新增供应商不填 Key，创建时固定发送 secret=null；Key 在详情中添加。编辑时 secret=null 保留 default Key、空字符串清除 default Key、非空替换 default Key；命名 Key 不受影响；清除有明确确认。秘密只入不出、不写前端持久存储、日志或 URL；提交开始清空表单，失败需重新输入。不能承诺 JS/Go 字符串的物理内存擦除。
- 保存成功返回新 revision，不表示供应商连通性已经验证。

## 2. 查询、写入与错误

所有桥接结果含 `contract`、`data`、`problem`。版本不符、无绑定或无有效 data 时失败，不回退 Mock。`problem.code` 仅白名单，前端不解析任意异常文本。

| 桌面错误 | 语义 |
| --- | --- |
| offline / read_error | 无法连接或无法读取；数据不可用不等于空列表 |
| unauthorized | 401/403；授权留在 Go 层，不要求页面填管理令牌 |
| referenced | 409；供应商仍被模型引用，或待删模型仍被 Binding 引用；先处理引用 |
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

- 输入为 `provider_id`、`expected_revision`（桌面 1..MAX_SAFE_INTEGER）和 `key_id`（空字符串选择首个启用 Key）。必须匹配路径及当前已存供应商 revision，使用管理授权和 `X-Anchor-Instance`。不接收临时 URL、密钥或任意路径；冲突在发起上游请求前返回409。
- 后端从已存快照取地址／密钥，使用 `<base_url 去尾斜杠>/models`，保留部署前缀；未带版本的根地址或自定义前缀，仅404/405或200 HTML网页时尝试同前缀 `/v1/models`，兼容 NewAPI 代理部署（例如 `/newapi/v1/models`）；已带版本路径或版本后的 `/openai` 不重复追加 `/v1`。官方 OpenAI、Anthropic、Kimi/Moonshot 和硅基流动根地址固定 `/v1/models`；DeepSeek 根地址固定 `/models`，OpenRouter 固定 `/api/v1/models`，Gemini 根地址及 `/v1beta/openai` 目录改用 `/v1beta/models`；`/v1beta` 前缀使用 `x-goog-api-key`，其他路径沿用 Key 默认协议鉴权。所有凭据只进请求头。停用供应商也允许显式查询。复用无环境代理、禁止重定向的客户端与全局16／供应商并发上限；全部路径/分页共用 `min(timeout_seconds, 8)` 秒。最多10次 HTTP请求、每页请求1000条，不重试失败请求、后台轮询或持久缓存；完整规则见[CLI 大全](../cli/commands.md#provider-modelsdesktop10-小步)。
- 输出固定七字段：`provider_id`, `key_id`（实际查询 Key；无 Key 为 anonymous；所有 Key 停用且未指定时为空）, `revision`（本次使用的快照版本）, `http_status`（未收到状态为0）, `elapsed_ms`（整数毫秒）, `models`（字符串数组，始终非null）, `failure`（成功为空字符串）。接受 OpenAI/Anthropic `data[].id` 和 Gemini `models[].name`（去除 `models/` 前缀，兼容裸 ID）；按 `has_more/last_id → after_id` 或 `nextPageToken → pageToken` 读取全部页。去重排序、丢弃元数据；累计最多512 KiB、1000条原始记录、每 ID 256字节，无控制／格式字符。分页游标仅编码为固定参数，不当作 URL；循环、混用格式、缺失游标、后续页失败及超限时整体失败，不返回部分列表。
- 请求受理后目录失败仍返回 HTTP200 与固定分类：`authentication`（401/403）、`unsupported`（404/405）、`rate_limited`（429）、`http_error`（其他非200）、`network`、`timeout`、`busy`、`too_large`、`invalid_response`。上游错误正文、原始网络异常、密钥不回显；包含已存密钥的 ID 也拒绝。管理层鉴权／冲突／实例错误仍走 `problem`，不混作上游结果。
- 不写配置、revision、模型映射或健康状态。一次目录成功仅说明此接口返回有效目录，不代表推理、工具、流式、余额或持续健康；404/405 不等于供应商推理不可用。
- UI 详情手动触发、禁重复点击，显示目录 HTTP／耗时／失败分类和限高模型列表；区分“目录列出”与“仅已存协议”，避免把旧覆盖当成本次 Key 目录；Key / 模型协议编辑见 §6。查询中阻止供应商切换、编辑和页面离开；选择、重读或保存后清空结果，取消／卸载丢弃晚到数据。Wails AbortSignal 不能保证中断后端请求，后端超时仍生效；查询错误不会显示“可能已保存”。新功能与旧核心不同步时失败关闭，不回退 Mock。
- CLI 等价入口为 `provider models --id ID --revision N [--key KEY]`；与 `model list`（Anchor 已存公开映射）及网关 `/v1/models` 区分。测试分层沿 §3，原生两平台仍未验收。

## 5. 第一优先工作台（desktop.11）

新增 `Bridge.ProviderWorkspace({provider_id})` 与 `Bridge.ProviderAction(Action)`，对应 GET `/control/v1/providers/{id}/workspace` 与 POST `/control/v1/providers/{id}/action`。动作白名单、默认值和 CLI 等价入口见[指令大全 §5](../cli/commands.md#5-供应商)及[实现契约](provider-priority-one.md)。`internal/catalog/workspace.go` 是核心 DTO 源，桌面规范以别名纳入生成器，共用 `desktop/contract/provider-workspace.json`。

Workspace 固定返回 provider_id/revision/strategy/keys/models/usage/prices/estimates，集合始终为数组；probe 和 imported_price 可空。KeyStatus 只含 ID、enabled、状态、连续失败次数、冷却结束时间和最近测试，以及 protocol 与 model_protocols（[{model,protocol}]，始终数组），绝不返回密钥。Probe 绑定 revision、Key ID、模型、执行时间、HTTP 状态、耗时与失败分类。Prices 表达 USD/百万 token，缓存单价与估算 USD 可以 null，不能用0替代未知。

每次动作携带当前供应商 revision，模型编辑/删除还携带 model_revision。删除、改名、删 Key、重置 Key、删除模型和推理测试须 approved；复制 Key 须 copy_keys。复制返回新供应商工作台，删除返回 deleted=true，其余返回操作后的工作台。模型映射修改只增加模型 revision；配置/Key/模型协议覆盖/策略/价格写入增加供应商 revision；test、key-reset、price-import 不修改配置 revision。冲突/结果未知先重读，不自动重放；取消前端消费不等于取消已发出的写入。

目录查询可指定已存 Key（允许停用），不存在的 Key 返回 not_found 且不请求上游；省略时使用第一个启用 Key。认证使用 Key 默认协议，模型覆盖不影响目录认证。存在 Key 但全部停用时返回 authentication，不退回匿名。推理测试可以明确指定停用/冷却 Key；未指定则使用首个启用 Key，无 Key 才匿名。目录查询不更新健康；真实网关结果和显式推理测试更新健康。状态接口 upstream_health=per_key 提示从供应商工作台查看，不能视作总健康结论。

## 6. Key / 模型协议（2026-10-11，desktop.16 dev 已发布）

协议归属 Key，Action.protocol 使用 responses / chat-completions / anthropic。key-put 省略或空 protocol 保留已有 Key 默认值，新 Key 默认 responses；secret 可空以保留凭据，因此支持纯协议编辑。key-model-protocol 使用 target=Key ID、model=实际模型名、protocol=独立覆盖；空值或 inherit 删除覆盖。两种写入均要求当前供应商 revision，成功加一，失败事务回滚；不改变公开模型映射。

转发先选 Key，然后按该 Key 和候选中的实际模型名解析覆盖 → Key 默认值，JSON、SSE、认证与推理测试均使用此结果。模型未设置覆盖时动态继承，以后修改 Key 默认值自动生效。供应商 protocol 仅保留为核心内部值，用于历史兼容、新 default Key 初始化与匿名请求，不再控制已有 Key，桌面不显示也不设置。schema3→4 把每个供应商旧协议复制到它的全部 Key；复制凭据时一并复制覆盖，改名/删除按外键级联。升级须同步客户端与核心；旧程序无法打开 schema4，不提供降级。

UI 的 Key 编辑支持默认协议与留空保留凭据；模型页按 Key 获取目录，为实际模型逐项选择继承/独立协议，已存覆盖在目录尚未查询或不再返回该模型时仍可查看和恢复继承。切换供应商/Key，修改 Key 凭据或删除 Key 时清理目录结果；模型协议写入保留当前目录，方便连续编辑。查询结果包含实际 Key，前端拒绝与显式请求不符的响应。验证层次和限制沿用 §3。已随 desktop.16 dev 打包发布；原生安装未验收，安装包能力以[发行记录](../release/desktop.md)为准。


## 7. 共享公开模型映射与调度（2026-10-12）

Action 新增 multiplier；model-put 使用 strategy=round-robin|priority 设置公开模型调度，和 key strategy 动作分属两个层级。ModelMapping 新增 multiplier（始终有效正数，旧值0回读为1）及 strategy（旧模型空值回读为priority）。Go/TS 和 `desktop/contract/provider-model-mapping.json` 为共享契约。

model_revision=0 表示当前供应商加入别名，不覆盖它已有的映射；别名不存在则创建并默认round-robin，已存在则追加并保留其他候选、能力、策略及Codex元数据。正 revision 编辑须匹配共享模型 revision，只改当前供应商上游/倍率；tools/streaming/strategy 为共享模型字段。追加写入仍以读取的最新模型 revision 进行CAS及供应商 revision 事务校验，不自动重放并发冲突。model-delete 事务内只删当前供应商候选与价格，剩余模型 revision+1；最后一个候选才删除公开模型、清理模型价格并检查 Binding 默认模型引用。

round-robin 在核心按权重1/multiplier执行平滑加权轮询，默认倍率1，范围0.01..1000；只在当前可调度候选中分配首选上游，不预占半开Key。真正转发时再次核对并发及Key状态；失败转移仍遵循原有禁止未知结果/流式重放规则。倍率不从外部站点拉取，目录成功不表示可推理。
