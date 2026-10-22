# 请求级协议翻译层

2026-10-21 已随 desktop.61 dev 打包发布。仅处理既有 Responses、Chat Completions、Anthropic Messages，不增加第四种外部协议或运行服务。

相关设计、开发、审查和重构遵循[协议翻译架构 skill](../../.agents/skills/anchor-protocol-architecture/SKILL.md)。本文维护当前实现契约与支持范围；skill 维护职责、生命周期与实施约束。涉及架构原则、模块边界、公共接口或兼容／恢复策略的变更，必须在同一任务同步更新 skill、适用设计契约、实现和测试；纯内部实现变化须核对一致性。

## 调用边界与文件归属

```text
HTTP 入口 → protocol.NewSession → RequestInfo / CheckCapabilities
        → 网关选择模型、候选和 Key
        → Session.Prepare(AttemptOptions) → Attempt.RequestBody / RequestHeaders / Endpoint
        → 网关认证、网络发送、期限、响应类型检查
        → Attempt.TranslateResponse / TranslateStream
        → 网关 HTTP 交付、重试决策、日志和用量持久化
```

| 模块 | 职责 |
| --- | --- |
| `internal/protocol/session.go` | 不可变的客户端原生请求与转换基线；每个候选独立 Prepare；请求编码与私有返回映射绑定 |
| `ingress.go`、`client_tools.go` | 类型化 RequestInfo / Capabilities；无状态与工具执行边界校验；不访问模型库、不执行工具 |
| `request.go`、`chat_request.go` | 现有协议结构适配与白名单校验；参数、输出格式、历史思考、工具身份和 custom 包装规则 |
| `attempt_response.go`、`response.go` | 有界读取、普通响应语义校验、工具还原和客户端编码；返回 Usage、Observation、Diagnostics |
| `stream.go` | 同步增量 SSE 解析／编码，只依赖 reader/writer；工具分片、索引、终止、内容和数量上限 |
| `stream_http.go` | 兼容旧 Stream 入口；HTTP 写期限和刷新适配到通用 writer，不参与协议语义 |
| `recovery.go` | 已有密文拒绝识别、恢复窗口、清理派生会话；没有自动重试或渠道选择 |
| `native_reasoning.go` | 原生 Responses 派发副本中无效加密推理字段的预清理；不修改客户端基线 |
| `responses_tool_ids.go` | 目标 Responses 派发副本中工具项 ID 前缀、长度和碰撞规范化；保留 call_id 与客户端基线 |
| `diagnostics.go` | 固定阶段、错误类别、完整／降级／不支持结果，安全错误包装 |
| `internal/core/gateway*.go`、`translation_logging.go` | 鉴权／访问范围、调度、上游认证、连接与取消、候选及恢复预算、HTTP 交付、诊断与账务持久化 |

2026-10-22 当前源码的逐 Key 来源排序由核心 model_routing_control／model_routing 管理，身份为供应商／上游模型／Key；不改变协议层 Session／Attempt、同渠道恢复预算或转换规则。

网关不再持有 `ChatMapping`，不判断何时展平或恢复工具，也不解析普通输出、思考、结束原因、用量或流式事件。协议库不依赖 core、state 或数据库；HTTP 专用兼容入口单独隔离。已有低层 ConvertRequest/Stream 入口继续共用相同规则，不建立第二套转换器。

## 生命周期与内部表示

Session 复制入口字节，保留客户端协议。`PolicyFields` 是供旧日志／亲和性算法使用的独立 Responses 形状投影，**不是通用语义模型或可用于原生发送的请求**；改写它不能影响 Session。公共接口使用 RequestInfo、Capabilities、AttemptOptions、Response、Usage、Observation、Diagnostics 等类型；Result/Item 继续明确区分正文、明文思考、函数调用、custom 调用及其身份，不为架构形式重写既有适配器。

每次 Prepare 都重新解码私有基线，设置本次上游模型及明确的模型档位策略，创建新的工具映射。请求体返回副本，映射不暴露给网关。Session 可以并发 Prepare；一个 Attempt 由一次上游尝试独占。失败或取消后，调用栈退出即可回收请求状态，无注册表或全局映射。

跨协议继续使用现有 Responses 形状的内部请求桥及类型化输出；不能表达的原生字段在 Prepare 时拒绝，不把最小公共子集当成静默过滤器。下一轮工具结果通过客户端回传的声明、name/namespace、call_id 和历史调用重建映射，不依赖上次 HTTP 请求的进程内状态。碰撞、长名称与歧义身份继续由既有 Chat 适配器处理。

同协议发送保留原生字段，保留原生响应字节；修改路由选定的 model、明确的 service_tier，以及 Responses 的 store=false，并应用下述原生 Responses 无效密文字段预清理与工具项 ID 规范化；原生 Anthropic 省略 max_tokens 时保留既有 4096 默认值。原生 Chat 的 stream_options、原生 Anthropic 的签名思考／多模态等可以在相同协议中保留，**不意味着新增了这些能力的跨协议转换**。入口仍检查消息基本结构、鉴权、无状态限制、工具执行范围以及映射的 tools/streaming 能力。客户端 tool_search 仅沿用现有原生 Responses 受限透传，跨协议仍拒绝。

不按供应商或模型名推测 thinking、预算或 Token 字段。AttemptOptions 中的档位来自已保存的明确路由策略；若以后增加供应商能力配置，应在 Prepare 的显式选项边界扩展，经验证后由适配器消费，而不是向调度分支添加名称猜测。

## 兼容策略与本轮实际行为变化

完整转换表示本次所需语义在已有支持范围内可表达，不表示目标实现必然支持所有标准字段。原生透传不承诺上游实现本身可用。

既有 Responses → Chat 降级保持默认允许，并返回固定 Degradation：

- `reasoning_summary_style`：Chat 不提供摘要样式选择器。
- `encrypted_reasoning_projection`：可选密文返回投影无法生成；不伪造密文，不丢弃非空密文历史。
- `custom_grammar_description`：grammar 仅保留为函数描述，不保留原生强制语法约束。

明确的历史密文恢复仍是原有有损例外：仅原生 Responses、明确拒绝、未交付、同渠道至多一次；清理历史 reasoning 后创建**派生 Session**，其他候选仍使用未污染的原请求。清理丢失隐藏推理连续性，沿用原恢复 evidence，不扩大触发条件。检查中已有的单次待决 Read 协程沿用原实现；取消由网关关闭响应体／取消 HTTP 请求以解除阻塞。

2026-10-21 后续修复（desktop.61 dev 已发布）：原生 Responses 在每次 Prepare 时，删除 input 顶层 reasoning 条目中显式为 null、非字符串或空字符串的 encrypted_content，并删除该条目的 id，避免 store=false 下无密文的条目触发历史存储查找。保留 reasoning 条目、summary、其他字段、非空字符串密文及全部非 reasoning 条目；缺少 encrypted_content 的条目不改写，不解析或猜测非空密文格式。只改派发副本，不修改 Session 基线、客户端历史或跨协议规则；无额外请求、不消耗恢复预算，也不生成“已清理加密上下文”重试标记。已有明确密文拒绝后的恢复机制保留。

本轮除分层外实际改变：

1. Chat／Anthropic 原生请求不再强制往返 Responses；只能原生表达的字段不再在入口被无条件拒绝，选择其他协议时仍拒绝。原生工具限制不放宽。
2. `additional_tools` 声明现在与顶层 tools 一起执行能力和托管工具检查，堵住原生透传绕过工具限制的缺口。
3. 普通响应显式 failed、queued、in_progress 或无效结束原因不能因 HTTP 200 当作完成；跨协议不能表达的过滤／拒绝结束会失败，不合成为 completed。流式转换要求目标成功事件所需的上游终止及结束原因。
4. 新增 64 MiB 累计 SSE 输入字节上限，连原生流也有界。原有单事件 2 MiB、转换内容 16 MiB、最多 256 转换输出项和普通响应 16 MiB 上限保持。
5. SSE 一帧合并为一次 writer 调用，防止写出完成帧后立即取消覆盖已完成的日志状态；写入失败仍不记成功。
6. Anthropic 输出按上游内容项结束边界关闭当前块并释放后续块，顺序文本不再等到整体回答完成。

原生 Responses 历史兼容路径仍接受没有 status 的 JSON，状态记为未知；不会因 HTTP 200 记录 generation_status=completed。原生 Chat 的 `[DONE]` 与 Anthropic `message_stop` 继续是透传终止边界；缺少可解析结束信息时不编造生成状态。传输成功与完整生成语义分开。

## 工具项 ID 兼容修复（2026-10-22，已随 desktop.69 dev 发布）

每次 Prepare 在目标协议为 Responses 时，对派发副本 input 顶层的 function_call、custom_tool_call、custom_tool_call_output 的非空字符串 id 分别检查 fc、ctc、ctco 前缀；缺少对应前缀则补上。例如 custom_tool_call 的 id=call_fixture 改为 ctc_call_fixture，call_id=call_fixture 保持不变。由协议层在首次发送前完成，不等待上游拒绝、不新增重试，也不依赖前一请求的进程状态。普通与流式请求共用同一规则。

已有正确前缀且不超过64个Unicode字符的ID保留。修复后的ID如果超过64字符，或与输入中已有ID／本次生成的ID碰撞，则使用保留前缀的确定性SHA-256短后缀缩短／消歧；相同输入重复准备以及对已修复内容再次处理保持稳定。既有重复ID不属于本次修复范围。

只修改上述工具项的 id；保留 call_id、name、namespace、参数／自由文本／执行结果和未知扩展字段，不生成缺失／空ID，不改非字符串ID、不扫描嵌套内容、不改 function_call_output、message、reasoning 或未知类型。原有无效密文字段预清理独立保留，不引入 CLIProxyAPI 的超长加密推理条目删除策略。客户端 Session 基线、其他目标协议的尝试及原生返回JSON／SSE保持原有内容，不重写真实会话文件或Agent配置。

参考 [CLIProxyAPIPlus 工具项ID规范化](https://github.com/kaitranntt/CLIProxyAPIPlus/blob/8f22abcbc089051073167882157556a90a753c0a/internal/runtime/executor/helps/codex_input_ids.go) 的兼容边界，使用 Anchor 现有 RawMessage 派发副本机制独立实现。此项属于内部协议数据面修复，不新增管理开关、CLI命令或日志正文。回归测试覆盖保留边界、候选隔离、前缀与长度／碰撞，以及模拟HTTP上游的普通／流式工具调用→历史与结果回传→最终答案；不代表真实付费上游或运行版本验收。

## 流式与取消边界

普通文本和函数参数按事件增量交付，不能整段读完再伪装为流。custom 包装 JSON 必须完整校验后才解包 input，以既有 16 MiB 总转换内容限制缓冲，避免破坏转义、Unicode 或泄露包装字段。Anthropic 顺序内容块在上游发出内容项结束事件后立即推进，后续文本继续增量交付；无法提前结束前块的并行交错工具才保留有界缓冲，不发出不合法的交错 Anthropic 块。

转换器无新增后台协程或队列。通用 Stream 接口检查 context；阻塞网络 Read 的解除由拥有连接的网关负责取消和 Close。协议层不持有连接、不关闭调用者的 reader/writer。网关在开始 SSE 交付后无条件结束该尝试，不重放；缺失终止、非法／截断参数、超限、客户端写失败不会合成成功完成事件。

## 安全诊断与验收

Diagnostics 只含客户端／上游协议、request/response/stream 阶段、固定类别、兼容性及固定降级列表；Error() 不展开底层错误。网关补充实际派发与正文交付事实，保存至既有 evidence：`translation_client`、`conversion_target`、`translation_stage`、`translation_category`、`translation_compatibility`、`translation_dispatched`、`translation_delivered`、`translation_degradations`。预派发转换失败继续保留 `conversion_reason`、路由身份、HTTP=null、attempt_count=0。

成功的 category 为空；请求类别包括 protocol、invalid_json、model、stream、stateful_request、tools、streaming_disabled、tools_disabled、hosted_tool，以及[既有请求转换类别](protocol-chat-bridge.md#错误与日志)。普通响应使用 incomplete_body、invalid_json、upstream_error、unsupported_finish、unsupported_content、missing_usage 及既有 responseValidation 固定类别（如 invalid_choices、refusal、invalid_tool_call、missing_response_id）；流式使用 invalid_stream、missing_terminal、cancelled。未知底层错误不会把原始文本当作类别。

不记录正文、提示词、思考、工具参数、凭据、未知字段名或原始上游错误。API 状态／错误码继续使用既有目录；日志详情和导出包含新增 evidence，列表白名单保持原样。旧日志不补写。

新增 Session 测试覆盖三协议九种组合、同协议保留、普通／流式、明确零与未知用量、并发 Prepare／候选污染、请求拒绝／降级、顺序多块的提前交付、增量／取消／写失败、工具及事件／累计上限；既有适配器测试继续覆盖思考和输出格式、长名称、碰撞、歧义、custom 转义及密文恢复。网关用隔离 SQLite 与 httptest 上游跑普通和流式两轮并行 function/custom 工具闭环，包含候选切换、结果回传、最终答案和日志脱敏。实际执行命令与结果见本轮任务记录；没有真实供应商、Agent 配置、桌面安装或运行版本验收。

跨协议的多模态、tool_search、签名思考、Responses 输出 reasoning/custom 的未实现方向，以及无法无损表示的未知字段仍不支持。详细 Responses → Chat 支持表见[现有桥接说明](protocol-chat-bridge.md)。


### 网关调度会话与重试边界（2026-10-21，当前源码未发布）

显式会话由 `X-Anchor-Session` 或 Codex thread/session 元数据确定；不再以提示词相似度推测会话。仅在会话请求成功后记住渠道，后续优先复用；该渠道没有可用 Key／供应商不可用时按当前顺序 fallback，新会话按当前映射优先级开始。粘性仅影响网关调度，不表示上游必然缓存命中；进程重启或有界会话表淘汰后重新选择。连接前传输失败及明确 HTTP 失败可尝试下一候选；连接后的超时／传输结果不明不重放。任何客户端流式输出开始后不重放。

## Claude 请求头（2026-10-22，当前源码未发布）

AttemptOptions.ClientMetadata 只接受类型化的 AnthropicVersion/AnthropicBeta，不含认证；网关从请求头提取后交给 Prepare，协议层仅在 Anthropic → Anthropic 原生路径生成 anthropic-version/anthropic-beta。重复 beta 值和顺序保留，缺少客户端 version 时保持供应商认证层既有默认值；RequestHeaders 返回副本，每次 Attempt 独立。Anthropic 与其他协议之间转换不复制这些客户端头，避免对目标协议虚报能力。此处仅保留能力声明，不据此新增 tool search 或其他未验证语义。

网关在应用供应商认证后机械写入协议层返回的固定头，不转发客户端 Authorization/x-api-key/cookie/任意 custom headers，不记录原始头值，不扩大重试预算。签名思考/cache_control 在原生请求体上继续沿用既有保留契约；跨协议仍按已有能力表拒绝或转换。新增测试覆盖原生声明/认证替换、重复 beta、跨协议隔离及请求副本，不代表真实 Claude 或付费上游验收。

## 期限与自动端点修复（2026-10-22，当前源码未发布）

网关总期限由供应商 timeout_seconds 统一约束，包括等待响应头，不再叠加固定 30 秒响应头上限；另有显式可配置 first_token_timeout_seconds 首字期限，见下节。连接后超时属于响应头阶段，返回 upstream_header_timeout，未确认结果不重放；连接前失败沿用既有候选切换规则。网关只持久保存固定传输类别/阶段、配置秒数和 Anchor 错误身份；非 200 错误响应前缀最多 16 KiB，仅提取已识别 code/type 的固定类别，不保存原文，读取后完整还原给既有密文恢复识别器。

自动端点修复由网关 inference_endpoint 管理。没有版本路径的基址明确返回 HTML 且 HTTP 为 200/404/405 时，只允许同源补 /v1 一次，共享原期限和并发槽；保留部署前缀。补路径返回 2xx、非 HTML 且声明 JSON/SSE 后，在进程内按基址/协议/凭据摘要缓存 10 分钟，最多 128 项；后续请求直接使用修复地址，不改配置或 revision。该验证只证明响应类型，不保证生成完整；协议层继续验证内容。缓存返回 HTML/404/405 时失效，当前请求不重放，下一独立请求重新识别。重启、地址或凭据变化后重新识别，不从 JSON 错误、鉴权失败、限流、重定向或传输失败推测路径。

## 显式首字期限（2026-10-22，当前源码未发布）

供应商 first_token_timeout_seconds 默认30秒、可设1..3600秒；timeout_seconds 新建默认500秒，max_concurrent 新建默认20。已存总期限/并发不覆盖；缺首字字段的旧配置读取时规范化为30秒，旧客户端省略字段更新保留既有首字值，不写库迁移。CLI add/put 的普通字段仍为完整替换，显式使用默认值，见指令大全。网关共享并发保护由16提高至64，供应商并发1..64继续受控，避免默认20被全局16截断。

网关在每个候选派发前启动总期限与首字期限，同候选补/v1及历史密文恢复共享原期限，不重新计时。协议层 StreamOptions.OnFirstOutput 只报告第一次实际生成：文本、思考、工具参数或音频；响应头、角色声明、心跳及空事件不计。该通知在下游写入之前发出，不改变既有TTFT指标或缓存/内容。网络计时/取消和连接释放仍归网关。首字后只取消首字计时，剩余生成继续受总期限；非流式无法从普通JSON增量确认生成，等待完整响应。

首字超时为504/upstream_first_token_timeout，当前请求不重放。已提交SSE时保留HTTP200，由协议层按客户端协议/语言编码一个固定错误事件；ErrorWriter 由网关提供独立15秒写期限，避免上游取消后连错误帧也被请求context拦截。不合成成功终止，仍记录真实上游HTTP与失败日志。路径类型检查、恢复和内容限制保持。probe/chat 复用同一首字限制，仍保留诊断各自10/120秒总上限；目录/余额查询不应用生成首字计时。
