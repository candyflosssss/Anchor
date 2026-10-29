# Claude / Anthropic Messages 桥接

2026-10-29 当前源码，未打包／未发布。沿用 `/v1/messages`、`/v1/responses` 和 `/v1/chat/completions`，不新增协议、管理开关或 CLI。总边界见[请求级协议翻译层](protocol-translation.md)；供应商协议仍通过[现有 CLI](../cli/commands.md)选择。

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

这是明文展示转换，无法保留 Anthropic 的签名验证和推理历史连续性。带签名的 Messages 历史、redacted_thinking、不透明历史密文、Responses／Chat 明文思考回放到 Messages 继续拒绝；不能把本轮输出支持理解为签名历史的跨协议往返。thinking／budget_tokens／output_config 等专有请求参数继续仅原生保留，不按模型名猜测预算或改写强度。引用注解、图片、音频、文件和托管工具也不在本轮跨协议支持范围。

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
