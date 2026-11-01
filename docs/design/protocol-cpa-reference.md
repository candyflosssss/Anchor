# CPA 的 Responses → Claude 实现对照

- 核对日期：2026-11-01。
- 性质：固定版本源码调研、现状差异与后续实施依据；本文件不宣称已实现下方待补能力。
- Anchor 基线：desktop.81，修复提交 `475ee335fe752ee7d091c17171cd0fddb5cf85b6`。当前能力以[翻译层契约](protocol-translation.md)与[Messages 支持表](protocol-anthropic-bridge.md)为准。
- 主参考：[router-for-me/CLIProxyAPI `250d598175db61a4e99ac38b9164ed04745754e5`](https://github.com/router-for-me/CLIProxyAPI/tree/250d598175db61a4e99ac38b9164ed04745754e5)，提交时间 2026-10-31T13:09:56Z。
- 补充对照：[kaitranntt/CLIProxyAPIPlus `b95819f3757ff2bc226cfecf3c8d5fc53ddebd1f`](https://github.com/kaitranntt/CLIProxyAPIPlus/tree/b95819f3757ff2bc226cfecf3c8d5fc53ddebd1f)，提交时间 2026-10-28T15:04:49Z；仅核对其 Responses 请求／返回与 Claude thinking applier，不能将两个仓库或两个版本的行为混写。

## 结论与依据范围

此前 Anchor 已引用 CPA 的旧版工具映射与部分流式测试，但没有完整对照「请求翻译 → 模型能力／thinking → 最终请求校验 → 返回与下一轮历史」的链路。导致顶层选项、新会话、旧会话及长回答分别通过局部检查，却仍在后续分支失败。完整链路与回归样例是本轮最值得借鉴的部分。

本轮读取固定提交的请求／返回转换、executor 的普通／流式请求准备、thinking applier、签名兼容与有限范围 replay、相关测试源码；没有运行 CPA 程序或测试，没有复制其实现到 Anchor，没有引入依赖、修改真实 Agent 配置或调用推理上游。下文引用测试表示“已阅读其断言”，不表示已在本机执行通过。

CPA 按目标协议构造新请求，随后还有模型能力、凭据与配置相关的 executor 处理。不能用一个转换函数代表整个运行行为；例如某条原生 Messages 的独立 `output_config.effort` 会保持原 thinking 默认，而 Responses 翻译和显式 thinking 处理走另一条路径。

## 逐项对照

| 范围 | CPA 固定版本的行为 | Anchor desktop.81 的状态与判断 |
| --- | --- | --- |
| 目标请求与传输选项 | 构造 Claude 目标字段，源请求的未映射字段不会自动透传；系统指令、工具、内容等分别适配。 | 仍有 Responses 形状中间桥与目标白名单；已补已知 Codex 传输选项，但仅放宽白名单不能代替完整映射。保留未知语义拒绝，已知元数据需显式规范化或降级。 |
| 缺省输出预算 | Responses 转换优先用注册模型的 `MaxCompletionTokens`；未知模型回退32000。Fable 特例回退64000；显式预算超过注册最大值会被压低。 | .81 已用保存的默认预算→最大输出→4096回退，且保留显式客户端预算。借鉴能力驱动优先级，不直接照搬名称特例、未知预算或静默压低显式值。 |
| thinking／effort | 注册能力有离散等级时映射 adaptive；旧模型可映射手动预算并保证 `budget_tokens < max_tokens`。不支持的通用档位可映射到可接受值；关闭类型也来自模型能力。 | 目前只把 low／medium／high／xhigh／max 同名放入 `output_config.effort`，none 发disabled；没有完整 adaptive／manual／disable type 能力策略。这是现存功能差异，不是所有模型都必须额外开启 adaptive。 |
| 思考历史与签名 | 返回路径携带 Claude 签名；下一请求识别兼容签名后还原 thinking，外来或不可识别密文默认不回放为无签名 thinking。 | .80 起省略所有跨到 Messages 的 reasoning；Claude 经过 Responses 客户端再回 Claude 也丢连续性。应区分来源和可回放载体，不能把任意 `encrypted_content` 当 Claude 签名。 |
| system／developer | 所有位置的指令先按源顺序成为独立顶层 system 块，保留块边界和缓存提示；executor 后续按配置／目标策略决定最终位置。 | .79 起提升并拼接成单个 system 字符串，记录作用域降级。可借鉴块边界保留；不能不经能力／权限契约就引入中途system角色或提示词包装。 |
| 消息与图片结果 | 消息和工具结果按文本／图片／文件等内容块转换，相邻角色归并；图片结果保留为 image 块；空块有单独处理。 | .80 已保留限定图片及空输出元数据；尚不支持其所有文件／搜索方向。应借鉴按内容块处理和完整场景验证，不通过删除未知内容制造成功。 |
| 工具历史约束 | 显式检查紧邻调用／结果，移动真实结果到下一user内容最前；补中断错误结果、孤立结果转文本、重复结果取末次。 | 目前主要逐项编码和相邻角色合并，缺少完整的配对／紧邻验证。可先增加协议不变量校验；合成结果、去重或孤立结果文本化改变历史，不能直接照搬。 |
| custom／namespace／schema | 长名与命名空间有映射；自由文本custom包装；工具schema规范化；函数转换不原样保留Responses strict保证。 | 已有请求局部工具映射与custom包装，strict=true仍拒绝。不能把“忽略strict后发得出去”称为完整兼容；应分开评估语法／严格性损失。 |
| 流式终止 | max_tokens和model_context_window_exceeded返回response.incomplete；refusal映射content_filter。pause_turn有托管工具继续语义及测试。 | max_tokens→incomplete已经保留。model_context_window_exceeded／refusal／pause_turn尚未等量支持；pause_turn不能脱离相应工具状态直接标completed。 |
| replay状态与资源 | 某些兼容API Key、原生Claude入口才启用思考回放缓存；要求会话身份并隔离凭据／模型。 | Session／Attempt当前无跨请求思考缓存。CPA这项有限缓存不是所有Responses→Claude请求通用的恢复，也不是自动重发生成。若引入必须先定义归属、上限、过期和失败清理。 |

### 输出预算：主仓与 Plus 的实际差别

主仓的[缺省预算选择器](https://github.com/router-for-me/CLIProxyAPI/blob/250d598175db61a4e99ac38b9164ed04745754e5/internal/translator/claude/openai/responses/claude_openai-responses_request.go#L795)对非 Fable 注册模型直接取输出上限，其[测试](https://github.com/router-for-me/CLIProxyAPI/blob/250d598175db61a4e99ac38b9164ed04745754e5/internal/translator/claude/openai/responses/claude_openai-responses_request_test.go#L103)明确覆盖128000与64000，及null预算。入口同时有显式预算按注册最大值压低的逻辑。

Plus 的[同名函数](https://github.com/kaitranntt/CLIProxyAPIPlus/blob/b95819f3757ff2bc226cfecf3c8d5fc53ddebd1f/internal/translator/claude/openai/responses/claude_openai-responses_request.go#L620)只在注册上限比默认32000／Fable64000更小时取它，因此不能把当前主仓的128000缺省策略归到该 Plus 提交。

executor 的 [`stripTranslatedDefaultMaxTokens`](https://github.com/router-for-me/CLIProxyAPI/blob/250d598175db61a4e99ac38b9164ed04745754e5/internal/runtime/executor/claude_executor.go#L153)还会在原请求没显式预算时，从用于payload配置匹配的“原始译文”移除转换器默认字段，以允许配置default规则生效。这个操作不是无条件删除最终请求max_tokens；最终准备流程还调用ensureModelMaxTokens和用户payload配置。

Anchor不应把未知模型的4096直接改成一个更大的任意常量就结束处理。应沿用保存能力／预算的明确来源、客户端显式上限保护，并以实际准备日志和目标模型支持验证；上限变大允许更多实际用量，不保证生成完整。

### thinking：能力分支与原生保留必须分开

[Responses请求中的effort适配](https://github.com/router-for-me/CLIProxyAPI/blob/250d598175db61a4e99ac38b9164ed04745754e5/internal/translator/claude/openai/responses/claude_openai-responses_request.go#L78)按注册能力选adaptive或manual。[Claude applier](https://github.com/router-for-me/CLIProxyAPI/blob/250d598175db61a4e99ac38b9164ed04745754e5/internal/thinking/provider/claude/apply.go#L72)再处理最终thinking配置，关闭类型来自模型元数据；[预算规范化](https://github.com/router-for-me/CLIProxyAPI/blob/250d598175db61a4e99ac38b9164ed04745754e5/internal/thinking/provider/claude/apply.go#L176)必要时降低思考预算以符合max_tokens，而不是无条件扩大调用者上限。

其通用等级→预算表及不支持档位的映射只是CPA自己的兼容策略，不是供应商承诺的等价公式。Anchor当前契约禁止按名称猜预算或静默改档，不能照抄1024／8192／24576等常数。应先建显式且验证过的模型／渠道能力档案，再决定哪些映射是完整或降级；未知能力给准确诊断。

[原生独立effort测试](https://github.com/router-for-me/CLIProxyAPI/blob/250d598175db61a4e99ac38b9164ed04745754e5/internal/thinking/claude_standalone_effort_test.go#L47)明确要求仅output_config.effort的原生Claude请求保留原样；不能从adaptive路径推导“所有Claude请求都要加thinking”。另外executor遇到强制工具选择会[移除thinking与effort](https://github.com/router-for-me/CLIProxyAPI/blob/250d598175db61a4e99ac38b9164ed04745754e5/internal/runtime/executor/claude_executor_request.go#L907)，这会改变强度意图，Anchor需要自己的显式冲突策略和测试。

### 签名闭环：外来密文与同源回放不是同一件事

[历史转换](https://github.com/router-for-me/CLIProxyAPI/blob/250d598175db61a4e99ac38b9164ed04745754e5/internal/translator/claude/openai/responses/claude_openai-responses_request.go#L981)调用[供应商签名兼容判断](https://github.com/router-for-me/CLIProxyAPI/blob/250d598175db61a4e99ac38b9164ed04745754e5/internal/signature/provider_compatibility.go#L444)，兼容时还原thinking，外来密文默认丢弃；redacted_thinking使用专有前缀。此处是结构／来源兼容识别，不能宣称CPA能解密GPT推理或替Anthropic验证签名。

[流式返回](https://github.com/router-for-me/CLIProxyAPI/blob/250d598175db61a4e99ac38b9164ed04745754e5/internal/translator/claude/openai/responses/claude_openai-responses_response.go#L786)收集signature_delta并放入Responses reasoning载体，下一轮才有恢复依据。Anchor的Item目前只保留Text/Reasoning，普通与流式均不保存签名；因此仅改输入端“识别Claude签名”不能补齐闭环。

后续适合独立实现带明确供应商来源的载体与双向还原，覆盖普通／流式、多轮工具、换渠道／模型和原生回退。不能原样套用CPA将签名放入encrypted_content的外部形状而不修改Anchor现有“不得冒充OpenAI密文”的契约、来源验证和文档。CPA的[compat模式思考缓存](https://github.com/router-for-me/CLIProxyAPI/blob/250d598175db61a4e99ac38b9164ed04745754e5/internal/runtime/executor/claude_thinking_replay.go#L22)还仅对特定原生Claude兼容入口启用，不应误写为自动恢复所有跨协议历史。

### 工具历史：优先学不变量，单独评估历史修复

[repairClaudeToolPairing](https://github.com/router-for-me/CLIProxyAPI/blob/250d598175db61a4e99ac38b9164ed04745754e5/internal/translator/claude/openai/responses/claude_openai-responses_request.go#L1195)检查每个tool_use在紧邻下一user消息得到结果，并让真实tool_result排在该消息其他内容之前；它还会补造is_error的中断结果、把孤立结果变为文本。后两种策略改变输入语义，Anchor当前禁止伪造结果，必须另立边界。

近期应借鉴其合法性检查及合成历史测试：并行调用、重复ID、结果晚到、用户文本先于结果、丢失结果、跨消息迟到结果、custom包装与长名碰撞。对真实存在但顺序不合法的结果是否可重排，需要明确允许范围；不可恢复的工具链应精确拒绝，不合成“工具成功”。图片／文件块与文本顺序应在结果内部保留，不能因孤立结果文本化而把非文本内容丢掉。

### 返回结束：不是隐藏incomplete就能修好

[claudeResponsesTerminalState](https://github.com/router-for-me/CLIProxyAPI/blob/250d598175db61a4e99ac38b9164ed04745754e5/internal/translator/claude/openai/responses/claude_openai-responses_response.go#L172)将max_tokens和model_context_window_exceeded映射为response.incomplete/max_output_tokens；CPA也不会把这两个结束当作completed。Anchor .81对max_tokens的处理应保留，真正要核对的是发出的预算和模型能力。

[pause_turn与连续工具测试](https://github.com/router-for-me/CLIProxyAPI/blob/250d598175db61a4e99ac38b9164ed04745754e5/internal/translator/claude/openai/responses/claude_openai-responses_stop_reasons_test.go#L136)包含停止、历史回传、下一轮结果及工具ID去重；这个方向依赖托管工具等状态，不能只把未知stop_reason加入成功白名单。

## 后续实施顺序与验收

下列是本轮源码对照得出的待补项，不是已交付能力；不因本文件新增模型名推断、恢复预算或运行服务。

1. **完整场景回归基线与最终请求不变量。** 按CPA测试场景独立写Anchor fixture，串起客户端请求、最终目标请求、普通／流式返回、下一轮历史、工具结果与再次生成。保留有效输出顺序、预算来源和未改的候选；不能只测单user或单个字段。先让缺口在离线测试中暴露，再分解实现。
2. **thinking能力与参数冲突。** 明确adaptive／manual／关闭模式和上限的类型化能力来源；未知能力不猜。验证none／auto／各级effort、显式预算、强制工具、温度／top_p冲突和供应商明确拒绝，不能无声降低用户要求。
3. **Claude签名往返。** 先确定来源隔离和载体契约，再实现普通／流式保存和下一轮恢复；用GPT外来密文、可识别Claude签名、redacted块、非法／未知签名及候选切换验证。不得把同源和外来reasoning全部清空作为完整方案。
4. **工具链与其他结束原因。** 明确配对、结果位置和合法重排；缺失结果给稳定诊断。model_context_window_exceeded／refusal可以按目标表达能力独立验证，pause_turn／托管工具需完整生命周期。schema／structured-output的提示词降级不得写成强约束支持。

CPA构造新请求也不等于保留所有语义：未知item有告警后省略路径，unsupported attachment只有在整条user内容被清空等条件下才返回错误，structured output可以退化成提示词，工具strict不等价保留。Anchor应借鉴可复现的场景与目标合法性，不照搬每个宽松丢弃、指纹伪装、凭据策略或假定所有中转站行为相同。

## 本轮验证与许可

- 使用GitHub API固定两个完整提交SHA；按固定raw URL取得目标源文件，读取上述函数及测试断言；没有clone／编译整个CPA仓库或执行下载代码。
- 对照主仓与Plus的请求／返回／Claude applier文件，确认文件不同且缺省预算策略存在上述实质差别。
- 核对Anchor对应的Session、output_budget、anthropic_history、request、response及stream代码，区分已发布 .81 规则与待补能力；未修改运行时代码，未重新打包发布。
- 上游[MIT许可](https://github.com/router-for-me/CLIProxyAPI/blob/250d598175db61a4e99ac38b9164ed04745754e5/LICENSE)已读取。本轮仅记录行为、链接和独立实施依据，没有复制上游实现；若后续复制实质源码，应保留其版权及许可声明，Anchor本身许可不因此改变。
