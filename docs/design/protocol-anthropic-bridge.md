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
| Messages metadata.user_id | 映射到 Responses／Chat 的 user；未知 metadata 字段拒绝 |

工具结果通过下一次请求携带的声明、历史、namespace／name／call_id 重新建立映射；没有跨 HTTP 请求的工具注册表。未知字段、非法角色和工具输入类型仍拒绝，不用宽松过滤来掩盖丢失。

## 思考与响应

Messages 上游的明文 thinking 在 Responses 客户端返回为独立 reasoning summary，在 Chat 客户端返回为 reasoning_content；普通与流式路径一致。每次出现此转换，诊断记录 `anthropic_thinking_plaintext` 和 degraded。签名不会被标为 OpenAI encrypted_content，也不会混进正文或诊断。

这是明文展示转换，无法保留 Anthropic 的签名验证和推理历史连续性。带签名的 Messages 历史、redacted_thinking、不透明历史密文、Responses／Chat 非空明文思考回放到 Messages 继续拒绝；不能把本轮输出支持理解为签名历史的跨协议往返。thinking／budget_tokens／output_config 等专有请求参数继续仅原生保留，不按模型名猜测预算或改写强度。引用注解、图片、音频、文件和托管工具也不在本轮跨协议支持范围。

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
| Responses text.format.type=text／Chat response_format.type=text | 规范化为 Messages 默认文本输出；空对象／null 不改变行为。结构化输出、verbosity 或未知格式约束以 text_format 拒绝 |
| reasoning.effort=none／Chat reasoning_effort=none | 明确发送 thinking.type=disabled；其他强度以 reasoning_options 拒绝，不按模型名选择预算或假装已映射 |
| reasoning.summary=auto／concise／detailed | Messages 没有同等样式选择器，省略并记录 reasoning_summary_style；未知值拒绝 |
| Responses client_metadata | 仅接受字符串映射，省略非空值并记录 client_metadata_omitted；不转发、改装为 metadata 或写入日志 |
| stream_options.reasoning_summary_delivery=sequential_cutoff | 省略并记录 reasoning_summary_delivery；未知选项／值拒绝，不转发 Responses SSE 选项 |
| include=[reasoning.encrypted_content] | 可选返回投影不发送，记录 encrypted_reasoning_projection；不生成或丢弃已有密文，未知 include 拒绝 |
| truncation=disabled | 等价于不自动截断；auto／未知值以 truncation 拒绝 |
| 空 reasoning 历史项 | 校验既有字段及 summary/content part 结构后，不生成 Messages 内容块；非空明文以 reasoning_history 拒绝，非空／非法密文以 encrypted_reasoning_history 拒绝 |

空／null 传输选项不产生降级；原生请求仍保留上述字段。每个 Attempt 独立处理私有副本，失败候选不污染下一次准备；不清理客户端历史、不触发额外请求或重试。此修复不扩展多模态、签名思考、结构化输出或任意推理强度支持，已随 desktop.76 投放；已有安装仍需通过在线更新安装新版核心。

验证包括协议选项／拒绝边界、原生保留与候选隔离，以及本地模拟 HTTP 上游的 Responses／Chat × 普通／流式派发、降级日志脱敏和发送前拒绝；执行结果以任务记录为准。没有请求真实付费上游或修改 Agent 配置。

## 缓存亲和提示兼容修复（2026-11-01，当前源码未发布）

Codex 0.163 的 [ResponsesApiRequest](https://github.com/openai/codex/blob/rust-v0.163.0/codex-rs/codex-api/src/common.rs) 含有 `prompt_cache_key`，其[请求构造器](https://github.com/openai/codex/blob/rust-v0.163.0/codex-rs/core/src/client.rs)默认填入该值。前一轮 OpenAI 选项修复没有将它纳入 Messages 目标白名单；只增加此字段就能在派发前复现 `candidate protocol anthropic cannot represent request (request_fields)`。这是合成回归与固定版本客户端源码确认的缺口，尚未取得用户原始失败请求，不能据此断定唯一原因。

Responses／Chat → Messages 现在仅接受字符串或 null 的 `prompt_cache_key`，校验后从当前 Attempt 的派发副本中省略；非空字符串记录 `prompt_cache_key_omitted` 和 degraded，缺失、空串或 null 不产生降级。Messages 没有对应的缓存亲和键，不把它改装成 `cache_control`、metadata 或网关会话身份，也不将键值写入错误或日志。这会失去该提示的缓存亲和，不承诺上游缓存命中。

原生 Responses／Chat、Responses → Chat 继续保留原字段；准备 Messages 候选不会污染后续候选。非法类型仍以 `request_fields` 拒绝，其他未知字段、显式服务档位、真实推理强度、格式约束和历史密文的拒绝边界保持。回归覆盖两种客户端、普通／流式模拟 HTTP、降级诊断、日志脱敏、非法类型预派发拒绝及候选保留；实际执行情况见任务记录。修复没有自动替换运行核心，也未请求真实付费上游。
