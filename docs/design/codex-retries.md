# Codex 统一设置：提示词目录授权、重试与自动压缩（2026-11-06 当前源码）

本轮取消模型映射的同渠道额外重试，将重试交给 Codex 原生机制。统一设置保存在当前 Anchor 核心的状态目录，不是设备外观偏好；新模型与之后接管的 Codex 自动使用同一值。默认5次，范围0–10整数，不含首次请求，0关闭。生成的 Anchor provider 同时设置 `request_max_retries` 与 `stream_max_retries`，两项各自使用该上限；并非整轮操作最多请求6次的总预算。客户端实际重试仍取决于 Codex 的错误分类、版本与运行参数。

## 接口与状态

- Go DTO为规范源：`state.CodexSettings`、`state.SaveCodexSettings`、`managed.CodexSettingsState`，桌面契约从 `internal/desktop/contract.go` 生成TS；共享样例为 `desktop/contract/codex-settings.json`。
- `GET /control/v1/settings/codex` 只读，返回 `{settings:{override_model_catalog,retries,auto_compact,compact_percent,revision},needs_sync:[],compaction:[]}`。未保存时返回 override_model_catalog=false、retries=5、auto_compact=true、compact_percent=90、revision=1，不写入默认值。
- `POST` 同路径，输入 `{override_model_catalog?,retries?,auto_compact?,compact_percent?,expected_revision}`，至少一个设置非 null，expected_revision 必填。override_model_catalog 为布尔值；retries 取0–10整数、auto_compact 为布尔值、compact_percent 取1–99整数；省略或 null 的项保持原值。校验通过后事务保存并递增revision；旧revision返回409 conflict，无效输入返回400 invalid。两端需要同时升级。
- 接口沿用管理授权与 `X-Anchor-Instance`。CLI为 `agent codex-settings [--override-model-catalog=BOOL] [--retries N] [--auto-compact=BOOL] [--compact-percent N] [--revision R]`，桌面为 `CodexSettings`／`SaveCodexSettings` 薄桥接；前端按Model／hook Controller／View组织。
- 保存成功后仅安全同步已启用且已接管的Codex目标，不修改其他Agent、不扩大模型授权、不接管未登记目录、不恢复已停用目标。外部编辑引起冲突时保留外部配置，偏好仍保存，`needs_sync`列出待处理Binding；前往Agent页确认更新或使用既有 `agent sync` 显式重新接管。
- 配置同步后须重启Codex（包括app-server）生效，不重启或终止用户进程。未启用／未接管的目标在之后接管时使用当前值。
- 前端在读取前禁写，输入校验范围；保存超时／断线返回unknown，revision冲突返回conflict，锁定编辑直到重新读取，不自动重试管理写入。实例切换卸载旧控制器并丢弃迟到结果；旧宿主缺少方法显示unsupported，不回退Mock。

## 提示词与模型目录授权（2026-11-06，当前源码未发布）

`override_model_catalog` 默认 false，已有 settings 缺少该字段也按 false 读取，旧版托管／导入元数据不视为授权。GUI「设置 → Codex → 允许 Anchor 覆盖模型提示词与目录」明确展示影响，只有用户开启并保存或 CLI 显式 `--override-model-catalog=true --revision N` 才允许生成和加载 Anchor 模型目录。设置作用于当前实例的所有 Codex 绑定；同一 revision 保护、管理鉴权、保存后同步、冲突与 unknown 规则继续适用。新前端遇到缺少此字段的旧核心返回 unsupported，不能显示为“安全关闭”；旧客户端省略／null 保留已存授权，不能隐式开启。

关闭时不生成、不读取作为模板也不加载 Anchor 目录，保留托管前 `model_catalog_json`（包括用户自己的目录）及全局 `model_context_window`／`model_auto_compact_token_limit`；`model_instructions_file`、developer instructions、personality、AGENTS.md 均不由本开关修改。只管理连接与既有路由字段，Codex 自行选择原生／用户模板；公开别名、速度分身不保证都出现在原生选择器中，不能宣称原生一定识别任意名称。此前的网页搜索／协议限制不因本开关解除。

开启后才使用既有显式元数据 → 用户目录 → 精确本地缓存 → 通用编程助手兜底规则，目录同时承载基础指令、personality、工具、模型列表和上下文能力，不能描述为仅同步名称。用户指令文件仍由 Codex 按自身优先级解释。已撤回上一轮对通用指令额外注入“跟随用户语言”的修改；关闭状态完全不使用该兜底。自动压缩偏好保留，但只有目录覆盖和自动配置同时开启时才导出各模型阈值；关闭覆盖时文件核对通过的状态为 disabled，CLI 不将该状态误报为压缩未完成。

升级后的核心启动、保存设置或同步会把已应用的旧目录指针按 journal 原恢复基线还原，保留凭据与授权范围；不是整份重写配置。外部修改不覆盖，设置可以保存为 false 但 `needs_sync` 必须明确列出未完成的目标，不能声称已恢复。已退役的自有目录暂留受保护 journal，以便开关往返及崩溃恢复；正常恢复时只在内容仍匹配且没有引用时清理，不能删用户文件。清洁重建同样遵循当前开关，关闭时不重新引入通用提示词。

配置成功后需完全重启使用该配置的 Codex／app-server；本功能不终止用户进程、不重写历史会话。已有会话可能保留旧基础指令，运行验收须用新会话，不能以文件校验冒充客户端已重载。该修复未打包、未安装、未发布，不改变现有运行进程。

## 自动压缩配置（2026-11-03 desktop.82 dev 已发布，2026-11-06 授权边界修订未发布）

自动压缩偏好默认开启，但仅在用户明确开启模型目录覆盖时执行。按每个模型最终导出的 `context_window` 计算 `floor(context_window × compact_percent / 100)`，默认90%，至少1 token，使用整数运算避免浮点误差和 int64 溢出。优先应用显式模型上下文设置，再使用已登记元数据或接管前本地模板，不按模型名称猜测；价格策略已选择的工作窗口仍是有效导出窗口，不改成参考资料中的更大原始容量。窗口未知时不新增阈值，保留原模板／Codex 默认行为。只导出到 Anchor 自有目录，不修改保存的模型设置和模板。

目录里的每个模型设置 `auto_compact_token_limit`，使 Codex 切换模型后按各自窗口读取阈值。开启自动配置时，根级 `model_context_window` 和 `model_auto_compact_token_limit` 会覆盖这些模型值，故一起纳入 journal，保存原值后移除。关闭自动配置则还原根级恢复值，模型目录回到源模板／显式模型设置；Codex 原生自动压缩仍可运行。没有生成目录的绑定保留这两个字段原值，不推测其窗口。

旧 settings JSON 缺少新字段时继承开启与90%，重试单项保存保持当前压缩偏好。启动会检测旧目录并进行条件同步，新增 journal 字段保存真实原值；切换开关不丢弃字段所有权或吸收 Anchor 写入值。后台遇外部修改只报告待同步，显式 `agent sync` 可重新接管并保存外部值作为恢复基线；停用／正常停服仍条件恢复。设置更改只同步已启用且已接管的 Codex，不自动登记其他目标。

桌面设置沿用 Model／hook Controller／View，显示自动配置开关和阈值百分比；读取、保存、revision 冲突、unknown 写入与设备切换遵循原设置边界。配置落盘并不保证压缩请求成功；压缩由 Codex 触发和执行，Anchor 不增加压缩端点、后台摘要请求或恢复重试。

## 模型压缩配置检查（2026-11-05 当前源码，未发布）

统一设置 GET 与 POST 响应新增 `compaction` 数组。核心遍历已启用 Codex Binding 及当前授权模型，复用实际目录导出器取得窗口和阈值，并只读核对 journal、目标配置、根级覆盖和目录内容；不依赖内存中的上一次同步结果来判断文件健康。读取不补写模型草稿、不创建托管目标、不修复或覆盖外部文件，不请求上游。

每项包含 `binding_id`、`model_id`、`is_default`、`context_window`（整数或 null）、`token_limit`（整数或 null）及 `status`：

| status | 含义 |
| --- | --- |
| `written` | 当前目标文件与导出计划一致，该模型已写入自动计算的窗口和阈值；不表示客户端已加载。 |
| `missing_context` | 文件同步完成，但模型没有声明窗口，无法配置阈值；窗口和阈值为 null。 |
| `needs_sync` | 文件缺失、外部改动、计划已变化、检查失败或既有后台失败；不返回计划值冒充已写入值。 |
| `unmanaged` | 目标尚未应用，或已恢复，未写入托管配置。 |
| `unsupported` | 已应用的旧 single 绑定没有模型目录，不能自动导出阈值。 |
| `disabled` | 自动配置已关闭且文件检查通过；token_limit 为 null，不表示关闭 Codex 自身压缩。 |

禁用 Binding 不列入数组，无已启用目标时返回空数组。`needs_sync` 保持文件同步失败的 Binding 列表，不把缺失模型窗口伪装成文件同步失败；`missing_context` 可以与空 `needs_sync` 同时出现。任一检查失败仍返回其他目标的状态，不泄露配置正文、恢复原值或凭据。未写入的项不返回推测的窗口或阈值。

CLI 沿用 `agent codex-settings`，读取输出上述检查结果。保存仍先持久化并返回 JSON；存在 needs_sync，或目录覆盖与自动配置同时开启时有非 written 项，输出后以非零退出，明确“已保存但配置未完成”，不能按失败退出就自动重放写请求。没有目标时仍保存偏好，数组为空。桌面逐模型显示窗口、阈值和默认模型标记，缺失窗口／未应用／无目录／待同步保持可见；保存提示按配置完整性给出，不统一显示待重启。已写入值也始终注明需 Codex 重载、实际加载与触发未验证。无需自动修改数据库中的模型窗口、扩大供应商授权或终止客户端进程。

DTO 规范源是 managed／desktop 的 Go 类型，经既有生成器生成 TypeScript；共享样例为 `desktop/contract/codex-settings.json`（无目标）及 `codex-settings-incomplete.json`（已写入与缺失窗口混合）。响应新增字段，旧 CLI 消费者须允许新增 JSON 字段；新桌面遇旧核心缺少 compaction 返回 unsupported，不能把能力缺失当作空列表正常状态。写入后响应结构不完整沿用结果未知并要求重读。宿主与核心需配套升级，无数据库迁移；未打包或原生窗口验收。

## 自动配置已开启但当前模型没有阈值的排查（2026-11-05）

`auto_compact=true` 和 `needs_sync=[]` 只说明偏好已保存且文件同步检查未发现失败，不保证每个模型都有可导出的窗口。已有映射缺少 `configuration.limits.context`、元数据和本地模板也未声明窗口时，生成目录会保留 `context_window: null`，不会产生 `auto_compact_token_limit`。`model settings` 读取时显示的 models.dev 补充值只是草稿；未显式保存就不会进入 Agent 目录，不能把页面中的建议窗口当作已生效配置。

排查时先读取 `agent codex-settings` 和 `model list`，确认实际保存的窗口；再核对 Codex 配置所指向的 Anchor 目录中当前 slug 的 `context_window` 与 `auto_compact_token_limit`。若窗口缺失，通过 `model settings --id ID` 取得建议并审核，按[指令大全](../cli/commands.md#模型能力与默认值设置2026-10-17源码未发布)的 revision 检查保存入口持久化；只修窗口时，从原已保存 configuration 整理写入对象，保留其他字段，避免无意接受整份能力草稿。随后验证目录中的阈值等于已保存窗口的设定百分比，根级覆盖不会遮蔽它，并核对同步状态。

完成文件核验后仍须完全退出并重新启动 Codex（包括使用该目录的 app-server），再恢复原聊天。已运行会话不会因 Anchor 保存成功而自动重载；文件核验不能冒充实际压缩成功。缺少上游请求与客户端触发证据时，只能报告配置已修复、运行时触发待验证。速度变体是独立映射，须分别核对保存值及是否进入当前启用供应商对应的目录，不假定继承基础模型的后续修改。

## 旧配置迁移与网关边界

旧模型JSON中的 `channel_retries` 读取时忽略，不合并为统一值，之后模型设置保存不再输出此字段；模型管理API与文件写入拒绝该字段，旧导出文件须删除它。已托管Codex原先写入的0由正常启动同步或保存统一设置迁移，保留既有条件恢复基线。

网关单个入站请求对每个供应商／Key／候选模型通常仅尝试一次。唯一例外是原生 Responses 明确拒绝历史加密 reasoning，且尚未输出时，删除转发副本的 reasoning 并在同一渠道补发一次；整个入站请求最多一次清理重试，共享原超时并记录结果，详见 [加密上下文恢复](logging.md#历史加密推理恢复2026-10-20当前源码未发布)。保留安全的其他Key／候选切换、供应商超时、健康状态与日志；已开始输出的流及执行状态不明的失败不由网关重放。Codex负责判断并重试客户端请求／中断流，避免在两个层面反复请求同一渠道。

实现验证使用隔离状态目录、合成Codex配置与本地假上游，不读取或改动真实Agent配置，不调用付费上游。具体执行证据见本轮任务记录；原生桌面安装与真实Codex端到端运行须另行验收。
