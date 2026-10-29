# Responses → Chat Completions 兼容转换

2026-10-21 当前源码，未打包／未发布。此次修复针对 Codex Responses 请求选择 Chat Completions 候选后在 `request_convert` 返回 `unsupported_conversion`；不修改用户配置、安装版本或真实聊天记录，也不承诺特定供应商已经通过实测。

当前转换规则由[请求级协议翻译层](protocol-translation.md)承载：Session 保留客户端基线，每个候选创建独立 Attempt，请求与返回共用私有映射；网关不再持有 ChatMapping。原生透传、生命周期、结构化诊断与本轮兼容差异见该文档。

## 已实现范围

- `reasoning.effort` → `reasoning_effort`，保留明确档位，不把 high 猜测为供应商的 thinking budget。`reasoning.summary` 接受 auto/concise/detailed，但 Chat 没有摘要风格选择器，因此不发送该选择器；上游 `reasoning_content` 单独返回 Responses reasoning summary，不混入正文。
- `text.format` 的 text/json_object/json_schema → `response_format`；JSON Schema 的 name/schema/strict/description 保留。`text.verbosity` → `verbosity`。Chat 入口的对应参数也可归一化为 Responses 格式。
- `metadata`、`user`、`safety_identifier`、`prompt_cache_key`、`prompt_cache_retention` 按同名 Chat 字段保留；上游是否支持仍由其实际接口决定。只接受 `truncation=disabled`，不伪造自动截断。
- `include=["reasoning.encrypted_content"]` 作为可选返回投影接受；Chat 不能生成密文，不转发该投影、不编造 encrypted_content。**已有非空 encrypted_content 的历史 reasoning 明确拒绝**，不因改用另一协议而默默丢弃历史推理。
- 明文 reasoning summary/content → assistant.reasoning_content，与其后的相邻 assistant 文本／函数调用合并。Chat 入口也保留 assistant.reasoning_content；仅明文思考可转换，不把它解释为 Anthropic 的已签名 thinking。
- namespace 展平为 Chat 函数名，支持 64 字节名称限制及碰撞隔离，逐请求／逐候选保存映射，返回恢复 name/namespace；未指定 namespace 时仅解析唯一的局部工具名，不猜测歧义身份。
- custom 工具转为函数，参数为 `{"input":"原始自由文本"}`；历史调用／结果和强制 tool_choice 同步转换，响应恢复为 custom_tool_call。grammar 的 syntax/definition 保留在函数描述中，**降为提示词约束，不等于原生语法受限生成**。支持顶层 tools 与 Responses Lite 的 additional_tools 声明。
- 流式保留 reasoning、正文、并行工具的独立 output_index；普通函数参数仍增量发送。custom 包装参数在已有 16 MiB 转换上限内有界缓冲，待完整 JSON 字符串校验成功后发送原始 input，避免分片转义／Unicode 被破坏或泄露 JSON 包装。缺失终止事件、非法／截断参数不发送 response.completed。
- 非流式同样恢复思考／工具身份；usage 转换保留已知 reasoning_tokens，未知不伪造零；probe 不把明文思考当作最终可见回复。

## 仍会明确拒绝

跨协议图片／音频／文件、引用注解、托管工具、客户端 tool_search、未覆盖的 Responses reasoning → Chat 返回方向、自动截断、未知字段，以及不能无损解释的历史密文。Anthropic 签名历史和供应商专有 thinking 参数继续拒绝；2026-10-29 起，Anthropic 上游明文 thinking → Chat reasoning_content 的展示转换及显式降级见 [Messages 桥接支持表](protocol-anthropic-bridge.md)。切换为原生 Responses 是否可用必须由该供应商验证，不能据转换成功推断。

## 错误与日志

保留 HTTP 400 / `unsupported_conversion`，错误消息追加目标协议和代码自有类别。失败详情的 evidence 增加 `conversion_target`、`conversion_reason`；根记录保留选中 provider/key/upstream_model，但没有派发时 HTTP 仍为 null、attempt_count=0、sent_service_tier=not_sent。详情和导出可查询，不新增管理 DTO、命令或配置开关。

类别仅来自固定集合：request_fields、request_content、reasoning_options、reasoning_history、encrypted_reasoning_history、text_format、include_options、truncation、tools、custom_tool_grammar。未知错误归为 request_content，不输出原始字段名、字段值、密钥、思考原文或工具参数。历史日志不补写。

## 参考与验收边界

参考 [CLIProxyAPI ae3ee3b](https://github.com/router-for-me/CLIProxyAPI/tree/ae3ee3be303fb07fb8173279923413005bf32177/internal/translator/openai/openai/responses) 的请求局部工具索引、reasoning.effort 映射、custom 的 input 包装以及返回身份还原思路；独立实现，没有复制源码或引入其依赖。上游 MIT 许可不改变 Anchor 自身许可。

API 形状核对 [OpenAI Responses 迁移文档](https://developers.openai.com/api/docs/guides/migrate-to-responses) 和 [自定义工具／grammar 文档](https://developers.openai.com/api/docs/guides/function-calling)。本机合成测试覆盖请求格式、工具身份碰撞／歧义、历史回放、流式转义、截断、非法输入与日志脱敏；具体执行结果见本轮任务记录，不以模拟上游通过冒充石头或 DeepSeek 的真实验收。
