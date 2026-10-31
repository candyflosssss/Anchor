# Claude / Anthropic Messages 桥接

2026-10-31，已随 desktop.76 dev 发布。沿用 `/v1/messages`、`/v1/responses` 和 `/v1/chat/completions`，不新增协议、管理开关或 CLI。总边界见[请求级协议翻译层](protocol-translation.md)；供应商协议仍通过[现有 CLI](../cli/commands.md)选择。

## 请求和工具

| 方向／字段 | 当前行为 |
| --- | --- |
| Responses → Messages namespace／custom／additional_tools | 复用 Chat 桥的请求局部工具索引，展平名称并处理 64 字节限制和碰撞；历史调用、结果、强制选择使用同一映射，普通和流式返回恢复原身份 |
| Responses custom 工具 | 使用 `{"input":"原始自由文本"}` 函数包装；返回先完整校验再解包，保留 Unicode 和转义；grammar 仅作为描述约束，记录 `custom_grammar_description` |
| parallel_tool_calls ↔ disable_parallel_tool_use | 在 auto／required(any)／指定工具之间反向映射布尔值；none 不附带并行属性，未声明工具时不单独生成 tool_choice |
| Messages 普通自定义函数 | 接受省略 type 或 `type=custom` 的客户端执行工具；托管工具仍由能力检查拒绝 |
| Messages system／文本块／工具声明／tool_result 中的 cache_control | 仅接受 ephemeral，ttl 可省略或为 5m／1h；跨协议不发送缓存断点，记录 `prompt_cache_control`；原生请求保留 |
| Messages tool_result.is_error | 必须为布尔值；true 时将结果文本加上 `[Tool execution failed]\n`，记录 `tool_result_error_text`；false 不加前缀 |
| Responses／Chat 后置 system／developer | 按原输入相对顺序合并到顶层 system，instructions 在前；非空指令晚于会话内容时记录 instruction_scope_hoisted，明确作用域扩大；前缀或空后置指令不记降级 |
| Messages metadata.user_id | 映射到 Responses／Chat 的 user；未知 metadata 字段拒绝 |

工具结果通过下一次请求携带的声明、历史、namespace／name／call_id 重新建立映射；没有跨 HTTP 请求的工具注册表。未知字段、非法角色和工具输入类型仍拒绝，不用宽松过滤来掩盖丢失。

## 思考与响应

Messages 上游的明文 thinking 在 Responses 客户端返回为独立 reasoning summary，在 Chat 客户端返回为 reasoning_content；普通与流式路径一致。每次出现此转换，诊断记录 `anthropic_thinking_plaintext` 和 degraded。签名不会被标为 OpenAI encrypted_content，也不会混进正文或诊断。

这是明文展示转换，无法保留 Anthropic 的签名验证和推理历史连续性。带签名的 Messages 历史、redacted_thinking、不透明历史密文、Responses／Chat 的历史思考在 Messages 派发副本中按下方显式投影策略省略并记录连续性损失；不能把本轮输出支持理解为签名历史的跨协议往返。thinking／budget_tokens 等专有请求参数继续仅原生保留；output_config.effort 的公开强度选择器按下方规则跨协议映射，不按模型名猜测预算或改写强度。非空引用注解、音频、文件和托管工具继续不在已验证的跨协议支持范围；user／工具结果图片的限定支持见下节。

## 流式与用量

Messages → 其他协议分别跟踪文本、thinking 和工具块，校验索引、块类型和开始／增量／关闭边界。必须有 message_start、各块 content_block_stop、有效 stop_reason 的 message_delta 和 message_stop；重复块、关闭后的增量、类型错配、缺失终止、refusal／pause_turn、截断参数均失败，不合成成功完成事件。只含 SSE event 名且 JSON 缺失 type 的兼容路径保持。

工具参数优先使用 input_json_delta；若没有参数增量，则在块关闭时使用 content_block_start.input，包含非空对象，空对象保留为 `{}`。custom 包装继续受完整 JSON 校验约束，转换内容 16 MiB、输出项 256、单事件 2 MiB 和累计 SSE 输入 64 MiB 上限保持。正文和明文思考增量即时输出，不缓冲整份回答。

原生与转换的 Messages 流都逐字段合并 message_start 和 message_delta 的 usage，避免最终帧只更新 input/output 时清空前帧 cache_read_input_tokens／cache_creation_input_tokens。明确返回的零覆盖旧值，未知不伪造成已知。原生 SSE 载荷和事件仍原样转发，不执行跨协议块改写。

## 参考与验证边界

参考以下固定版本的转换思路，以 Anchor 现有适配器和私有映射独立实现，没有复制源码或引入依赖：

- [CLIProxyAPI 90c447d：Messages 请求编码](https://github.com/router-for-me/CLIProxyAPI/blob/90c447d3a8727d115aa7552ec93ce023736ed492/internal/translator/claude/openai/responses/claude_openai-responses_request.go)：工具选择和并行约束、工具结果和名称映射。
- [CLIProxyAPI 90c447d：Messages 返回转换](https://github.com/router-for-me/CLIProxyAPI/blob/90c447d3a8727d115aa7552ec93ce023736ed492/internal/translator/claude/openai/responses/claude_openai-responses_response.go)：thinking 增量与正文分离。
- [cc-switch ddcb111：Messages 流状态](https://github.com/farion1231/cc-switch/blob/ddcb111765be66ed9e7e43e240497fd2c9576688/src-tauri/src/proxy/providers/streaming_codex_anthropic.rs)：块生命周期、开始帧工具参数回退与用量合并。

本轮没有采用参考项目的思考签名载体、模型名称预算推断、托管工具扩展或恢复机制。验收使用协议测试和隔离 HTTP 模拟上游，覆盖普通／流式两轮工具闭环、安全诊断和拒绝边界；实际命令和结果见任务记录。未调用真实付费上游、未修改 Agent 配置，也未声称 Claude 官方服务或任一中转站已通过实测。

<a id="openai-请求选项规范化2026-10-31当前源码未发布"></a>

## OpenAI 请求选项规范化（2026-10-31，desktop.76 dev 已发布）

此前 Messages 目标仅准备工具映射，Responses 的顶层选项随后直接进入共用消息编码器；普通文本声明、传输元数据等也可能被笼统归为 `request_content`。现在由 `anthropic_egress.go` 先执行目标专属校验，再复用消息编码和工具映射；错误码仍为既有 candidate_conversion_unsupported，固定类别区分真实内容和选项问题，不输出用户字段或原文。

| 选项／历史 | 当前行为 |
| --- | --- |
| Responses text.format.type=text／Chat response_format.type=text | 规范化为 Messages 默认文本输出；空对象／null 不改变行为。verbosity 的 low／medium／high 校验后省略并记录 text_verbosity_omitted；结构化输出、非法 verbosity 或未知格式约束以 text_format 拒绝 |
| reasoning.effort=none／Chat reasoning_effort=none | 明确发送 thinking.type=disabled；low／medium／high／xhigh／max 同名映射 output_config.effort，不隐式开启 thinking；minimal／default／未知强度以 reasoning_options 拒绝，不按模型名选择预算或改档 |
| reasoning.summary=auto／concise／detailed | Messages 没有同等样式选择器，省略并记录 reasoning_summary_style；未知值拒绝 |
| Responses client_metadata | 仅接受字符串映射，省略非空值并记录 client_metadata_omitted；不转发、改装为 metadata 或写入日志 |
| stream_options.reasoning_summary_delivery=sequential_cutoff | 省略并记录 reasoning_summary_delivery；未知选项／值拒绝，不转发 Responses SSE 选项 |
| include=[reasoning.encrypted_content] | 可选返回投影不发送，记录 encrypted_reasoning_projection；不生成或丢弃已有密文，未知 include 拒绝 |
| truncation=disabled | 等价于不自动截断；auto／未知值以 truncation 拒绝 |
| reasoning 历史项 | 校验字段及 summary/content part 后，Messages 派发副本省略历史思考块；非空明文摘要／思考或字符串密文记录 reasoning_history_omitted，空项不记损失。非法密文类型／未知字段仍拒绝；原生及 Chat 方向不变 |

空／null 传输选项不产生降级；原生请求仍保留上述字段。每个 Attempt 独立处理私有副本，失败候选不污染下一次准备；不清理客户端历史、不触发额外请求或重试。此修复不扩展多模态、签名思考、结构化输出或任意推理强度支持，已随 desktop.76 投放；已有安装仍需通过在线更新安装新版核心。

验证包括协议选项／拒绝边界、原生保留与候选隔离，以及本地模拟 HTTP 上游的 Responses／Chat × 普通／流式派发、降级日志脱敏和发送前拒绝；执行结果以任务记录为准。没有请求真实付费上游或修改 Agent 配置。

<a id="缓存亲和提示兼容修复2026-11-01当前源码未发布"></a>

## 缓存亲和提示兼容修复（2026-11-01，desktop.77 dev 已发布）

Codex 0.163 的 [ResponsesApiRequest](https://github.com/openai/codex/blob/rust-v0.163.0/codex-rs/codex-api/src/common.rs) 含有 `prompt_cache_key`，其[请求构造器](https://github.com/openai/codex/blob/rust-v0.163.0/codex-rs/core/src/client.rs)默认填入该值。前一轮 OpenAI 选项修复没有将它纳入 Messages 目标白名单；只增加此字段就能在派发前复现 `candidate protocol anthropic cannot represent request (request_fields)`。这是合成回归与固定版本客户端源码确认的缺口，尚未取得用户原始失败请求，不能据此断定唯一原因。

Responses／Chat → Messages 现在仅接受字符串或 null 的 `prompt_cache_key`，校验后从当前 Attempt 的派发副本中省略；非空字符串记录 `prompt_cache_key_omitted` 和 degraded，缺失、空串或 null 不产生降级。Messages 没有对应的缓存亲和键，不把它改装成 `cache_control`、metadata 或网关会话身份，也不将键值写入错误或日志。这会失去该提示的缓存亲和，不承诺上游缓存命中。

原生 Responses／Chat、Responses → Chat 继续保留原字段；准备 Messages 候选不会污染后续候选。非法类型仍以 `request_fields` 拒绝，其他未知字段、显式服务档位、真实推理强度、格式约束和历史密文的拒绝边界保持。回归覆盖两种客户端、普通／流式模拟 HTTP、降级诊断、日志脱敏、非法类型预派发拒绝及候选保留；实际执行情况见任务记录。已随 desktop.77 dev 投放；已有安装须通过在线更新安装新版核心。本轮未自动替换真实运行实例，也未请求真实付费上游。


<a id="正常-codex-强度与文本样式2026-11-01当前源码未发布"></a>

## 正常 Codex 强度与文本样式（2026-11-01，desktop.78 dev 已发布）

本机运行核心确认为 desktop.77。2026-11-01 02:44 的真实失败日志已从 request_fields 变为 text_format 与 reasoning_options，所记录 effort 分别为 low 和 xhigh；日志未保存正文，因此只确认了实际类别与强度，未声称重放原始载荷。Codex 0.163 的正常请求可将 text.verbosity 与 reasoning.effort、缓存键等一起发送，原先只用 effort=none 的回归没有覆盖这组输入。实际请求无法通过后续校验，说明 desktop.77 的缓存键修复没有覆盖整个调用路径。

Messages 的公开 [effort 文档](https://platform.claude.com/docs/en/build-with-claude/effort) 和 [SDK 固定版本](https://github.com/anthropics/anthropic-sdk-go/blob/d38e929d2d031c5052818af00c0a276a2d7e498c/message.go) 已定义 output_config.effort=low／medium／high／xhigh／max；Anchor 的供应商对话测试入口已有相同映射。网关现在同名发送这些值，保留请求的强度选择，不把 xhigh 降为 high，不臆造 token 预算，也不额外开启 thinking。公开协议映射不表示每个旧模型或中转站都支持这些值；上游明确拒绝仍按既有错误返回，不自动改档或重试。

Messages 没有同等的 verbosity 字段；仅 low／medium／high 在校验后省略，记录 text_verbosity_omitted 降级。不改写提示词，不宣称等价保留了回答详略要求。空／null 不产生该降级，原生 Responses／Chat 保留原字段。未知 verbosity、结构化格式约束、minimal／default 等未映射强度，及非法思考历史仍明确拒绝；Messages 目标已校验历史思考的显式投影见下方已有会话支持表。

回归将两种入口、六种强度、三种 verbosity 与缓存／传输选项组合验证，并通过隔离 HTTP 上游检查普通／流式发送、日志降级和脱敏、非法类型拒绝、候选保留，以及 xhigh＋verbosity 的两轮 custom 工具调用。没有调用真实付费上游，也没有把模拟上游结果称为用户原会话恢复。

已随 desktop.78 dev 同版投放；已有安装需更新实际处理请求的核心。两平台编译／打包及默认源下载通过，用户原会话与真实付费上游未在本轮验收。


<a id="会话中的后置指令2026-11-01当前源码未发布"></a>

## 会话中的后置指令（2026-11-01，desktop.79 dev 已发布）

运行核心为 desktop.78 的 03:58 真实请求仍以 request_content 在派发前失败。对应 Codex 0.163 本机会话保存了 developer → user → developer → user 四条输入；移除客户端对第三方接口本就不发送的 internal_chat_message_metadata_passthrough 后，在隔离任务目录对这些实际输入执行无网络转换，修复前重现相同类别。原转换器在已有 user／assistant／工具消息后遇到 system／developer 就直接拒绝，前几轮仅选项与单 user 测试没有覆盖这条分支。

Messages 只有请求顶层 system，没有逐消息 system／developer 角色。现在按输入相对顺序收集这些文本，接在已有 instructions 后；保留全部指令和会话文本，继续合并相邻相同 Messages 角色。非空后置指令被提升为请求全局指令，作用范围扩大，必须记录 instruction_scope_hoisted 和 degraded；不声称保留了逐位置权限／时序语义。前缀指令及空后置内容不产生此降级。原生候选不改输入位置，Chat 目标不受影响；未知字段、非文本内容、历史推理／密文及格式约束仍按现有边界处理。

修复后的本地实际输入离线转换通过，并逐条核对指令／用户文本未丢失；私有输入不打印、不提交、不随包发布，也未发送给真实上游。另以无私有内容的样例覆盖两种入口、前缀／后置／空指令、原生候选隔离，以及含后置 developer 的24组普通／流式网关请求和两轮 custom 工具闭环。此结果证明请求准备分支通过，不代表实际付费上游响应或用户会话已验收。

已随 desktop.79 dev 投放；安装后实际核心需升至 .79。本轮已验证真实本地输入准备分支和默认源下载，没有自动替换运行实例或验收真实上游响应。


<a id="已有会话的历史投影与图片2026-11-01当前源码未发布"></a>

## 已有会话的历史投影与图片（2026-11-01，desktop.80 dev 已发布）

05:37 的真实失败由 desktop.79 核心处理，类别为 encrypted_reasoning_history，尚未派发。对应 Codex 0.163 会话在失败前保存162项 input：22条消息、38条 reasoning、51个函数调用及51个结果，含7份图片结果和7条带 phase 的历史回答。仅修顶层选项或单 user 输入不足以证明旧会话切换可用。

Messages 无法验证 OpenAI 推理密文，也不能直接接收无 Anthropic 签名的思考历史。现在默认在目标为 Messages 的 Attempt 私有副本中，校验后省略整个 reasoning 块，包含其中的明文摘要／思考和密文；非空内容记录 reasoning_history_omitted，明确隐藏思考及其摘要连续性丢失，不解密、不伪造签名、不把它当作普通回答。客户端持久历史、Session 基线和后续原生候选保留原值；用户消息、可见回答、工具调用／结果和其他不透明载荷不因此移除。原生 Responses 密文拒绝后的单次恢复、Responses → Chat 历史密文拒绝继续独立保留。非法 reasoning 容器／摘要／密文类型／未知字段仍拒绝，不能借投影抹去非法内容。

历史 assistant message 的 phase=commentary／final_answer 校验后省略，以 message_phase_omitted 记录阶段信息损失；未知值和非法角色继续拒绝。output_text 的 annotations／logprobs 仅为空数组或 null 时可规范化，非空或错误类型不得丢弃。

user 消息与 function/custom 工具结果中的 input_image 按标准 Messages image.source 转换：PNG／JPEG／GIF／WebP data URL 校验 base64 后保留原数据与媒介类型，HTTP(S) URL 原样转发且本地不获取；保持与周围文本的顺序及 tool_use_id。file_id、未知媒体、音频或任意内容仍拒绝。detail 的 low／high／original 以 image_detail_omitted 记录精度选择器损失，auto／缺失／null 不记此项；不删除图片、不替换占位或捏造描述。参照 [Anthropic SDK 固定版本 image/tool_result 类型](https://github.com/anthropics/anthropic-sdk-go/blob/d38e929d2d031c5052818af00c0a276a2d7e498c/message.go)，上游模型是否支持仍由实际响应决定。

对实际会话 input 做本地无网络验证：修复前复现 encrypted_reasoning_history；完整投影后逐项核对22条消息、51个调用、51个结果和7份图片，文本／参数／ID与图片解码字节均保持，后续原生 input 仍与原值一致。顶层请求控制项为合成设置，完整线上 HTTP 原请求及当前全部工具声明不可得，因此不称为完整线上请求重放；没有发送私有历史到付费上游。私有副本与临时测试不提交或打包，验证后删除。另以合成样例覆盖 malformed／unknown 拒绝、候选隔离、Chat 明文思考往返、图片顺序与非法媒体、普通／流式和工具闭环；实际执行结果见任务记录。

已随 desktop.80 dev 同版投放；需升级实际处理请求的核心。真实 input 离线准备与文本／工具／图片保留、两平台打包和默认源下载通过；真实上游生成和用户会话最终结果未在本轮验收。
