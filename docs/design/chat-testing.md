# 对话测试（2026-10-17，当前源码，未发布）

## 范围与界面

侧栏新增“对话测试”，仅用于快速验证所选供应商、已存 Key 和上游模型的常规文本对话。支持多轮上下文、150ms 轮询的流式文本、停止生成、复制回复和确认清空；Enter 发送、Shift+Enter 换行，中文输入法组合阶段不发送。沿用工作台灰阶组件、浅深主题和可折叠侧栏，不替代 Agent 聊天产品。

供应商只列启用项，Key 只列启用标识和默认协议，不读取或显示真实密钥；没有已存 Key 时明确选择 `anonymous`。模型候选合并所选 Key 的已存映射和手动可刷新的远端目录，不把其他 Key 的模型混入；目录权限不足时仍可手工填写上游模型 ID。目录列出不代表推理已验证。目标更改或刷新配置会开始新对话；生成中锁定目标与刷新，仍可停止或离开页面。

对话只保留在当前页面 React 内存，切页、切实例、关闭或刷新即丢弃，不写 localStorage、数据库或聊天文件。失败／停止的轮次保留可见但不作为下一轮上下文；不提供附件、图片、工具、Markdown/HTML 执行、历史会话或批量测试。模型文本按原文展示，不插入 HTML。复制失败有明确提示，不把剪贴板当作自动操作。

## 思考深度

会话头部提供“思考深度”，默认“不指定”，省略上游参数并保留模型自身默认行为。选择只在下一次发送时生效，不清空消息上下文；生成中禁用选择，切换目标则随新会话重置为默认。档位为 `none / minimal / low / medium / high / xhigh / max`；它们是请求选项，不承诺每个模型都支持。较高档位可能增加耗时、思考 token 与费用；不按模型名称猜测支持范围，不自动降级重发。

- Responses：发送原生 `reasoning: {effort: LEVEL}`。
- Chat Completions：发送原生 `reasoning_effort: LEVEL`。
- Anthropic：按所选 Key 的模型协议覆盖，仅展示／接受 `low / medium / high / xhigh / max`，发送原生 `output_config: {effort: LEVEL}`。不强制开启 `thinking`，也不把档位换算成猜测的 `budget_tokens`；effort 与 thinking 是否启用是不同维度。`none/minimal` 在核心启动前拒绝。

普通文本先按既有转换器转换，再加入目标协议的原生字段，避免跨协议转换静默丢弃思考设置；全局严格转换器和原有 Probe 请求不变。不支持档位的具体模型由上游安全失败回报；不提供思考过程显示。默认不指定不等于关闭思考，请求的输出 token / 超时上限仍然生效。原生参数依据 [OpenAI reasoning 指南](https://developers.openai.com/api/docs/guides/reasoning)与 [Anthropic effort 指南](https://platform.claude.com/docs/en/build-with-claude/effort)，支持档位仍以所选模型实际响应为准。

## 同源类型与接口

- 规范源：`internal/catalog/chat.go`；桌面别名与方法由 `internal/desktop/contract.go`、`Bridge.ProviderChat` 纳入 `go run ./cmd/desktop-contract` 生成，不手改 TS。
- 共享样例：`desktop/contract/chat.json`，Go 契约／核心／桥接测试、前端 Model 测试和显式 prototype Mock 共同读取。
- 新增 `POST /control/v1/providers/{id}/chat` → `Client.ProviderChat` → `Bridge.ProviderChat(ChatRequest) ProbeProgressResult`。沿用 `anchor-desktop-v1` 标准结果封套和 `ProbeProgress`，属于新增方法，既有 Probe 的输入、16 token／10 秒限制及 CLI `provider test` 输出不变。
- 每个 start 包含 `provider_id`、当前 `expected_revision`、准确 `key_id`、上游 `model`、`approved=true`、空 `run_id` 和完整 `messages`；可选 `reasoning_effort` 取上述档位，空／省略为默认。消息只有非空 `role/content`，从 user 开始严格交替，以 user 结束，1..80 条、内容累计最多64 KiB UTF-8。没有系统提示、工具、附件或外部 URL 输入。
- read/cancel 包含同一供应商、启动时 revision、run_id；不需重传消息，仍核验管理授权与实例身份。它们按 run 快照校验，不要求当前供应商仍存在／revision 未变化，以免运行中编辑或删除配置使取消不可达。start 遇到旧 revision 返回 conflict，不覆盖、不自动重发。
- 首次响应只确认创建临时 run，不表示推理成功。progress 包含 run/provider/revision/key/model、running/completed/cancelled、阶段、累计文本和耗时（毫秒）；终态 result 包含安全 HTTP 状态和 failure。completed 且 failure 为空才算成功；空文本在界面视作没有完成可用回复。

## 安全、成本与生命周期

请求只在用户发送且界面已提示上游可能计费后执行。UI → Model → 类型化 Wails 薄桥接 → 已授权连接的 Client → 核心；浏览器不接触供应商 Key、数据库、命令或任意上游 URL，不改变 Origin、CORS、SSRF 和实例校验规则。desktop 缺方法／旧核心返回不支持或拒绝新增字段时明确报错，不回退 Mock。

核心固定选定供应商与 Key，按 Key 的实际模型协议覆盖，复用 Responses / Chat Completions / Anthropic 转换、并发名额、鉴权、状态归约、用量记录与安全日志。不会改写映射、切到其他供应商/Key或自动重试；既有无版本基址 HTML → `/v1` 的单次受控兼容请求保持不变（非候选故障切换）。按 `source=probe` 记录诊断元数据（含显式选择的 `reasoning_effort`，默认空值不臆测模型实际深度）、用量及 `test:模型` 用量汇总，不记录提示词或回复。用户自行输入的内容会发给其选择的上游；不保证上游不保存。

每次生成最多120秒，且受供应商较短超时限制；固定请求4096输出 token。输出文本最多64 KiB，读取上游最多8 MiB，超限或不完整响应以安全 failure 结束，保留已收到部分文本但不纳入后续上下文。沿用流式跨 delta 的密钥回显脱敏，不暴露上游原始错误正文。输出 token 限制对未完整生成不承诺成功。

chat 与 probe 共用最多64个临时 run 名额，chat 文本至多保留3分钟，超时或核心正常退出会取消工作并清理。取消是尽力中断，已发生的上游费用或用量不会回滚。页面卸载在已知 run_id 时发送 cancel；start 仍在途时收到 run_id 后再取消，不盲目丢弃结果。如果实例/网络已不可达，无法保证取消送达，旧任务由核心截止时间兜底；不将“点击停止”立即当作已确认取消。

错误统一使用既有 Problem 码；invalid/conflict/unauthorized/unsupported 分别说明输入或配置变化、管理授权与升级需要。传输结果未知时明确提示核对日志，不自动重试，避免重复计费。不会显示未知异常、上游正文或密钥片段。

## CLI 与验证边界

同一能力提供 `provider chat --id ID --revision N --key KEY --model MODEL [--reasoning-effort LEVEL] --approve`，stdin 提供消息数组，输出最终安全 progress JSON。具体语法、退出与费用见[最新指令大全](../cli/commands.md)。CLI 单次提交完整上下文，不额外提供交互式终端聊天；桌面实时轮询／复制／清空属于 UI 生命周期。

验证分层记录于本任务更新：合成样例与 Mock、真实 Go 核心／Client／桥接连接到 httptest 上游、Chromium 模拟 Wails 绑定互不替代。未打包、未发布，Linux WebKitGTK / Windows WebView2 原生窗口验收另行执行，不能用浏览器通过冒充原生桌面已验收。
