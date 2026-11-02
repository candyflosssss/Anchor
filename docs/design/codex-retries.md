# Codex 统一设置：重试与自动压缩（2026-11-03 当前源码）

本轮取消模型映射的同渠道额外重试，将重试交给 Codex 原生机制。统一设置保存在当前 Anchor 核心的状态目录，不是设备外观偏好；新模型与之后接管的 Codex 自动使用同一值。默认5次，范围0–10整数，不含首次请求，0关闭。生成的 Anchor provider 同时设置 `request_max_retries` 与 `stream_max_retries`，两项各自使用该上限；并非整轮操作最多请求6次的总预算。客户端实际重试仍取决于 Codex 的错误分类、版本与运行参数。

## 接口与状态

- Go DTO为规范源：`state.CodexSettings`、`state.SaveCodexSettings`、`managed.CodexSettingsState`，桌面契约从 `internal/desktop/contract.go` 生成TS；共享样例为 `desktop/contract/codex-settings.json`。
- `GET /control/v1/settings/codex` 只读，返回 `{settings:{retries,auto_compact,compact_percent,revision},needs_sync:[]}`。未保存时返回 retries=5、auto_compact=true、compact_percent=90、revision=1，不写入默认值。
- `POST` 同路径，输入 `{retries?,auto_compact?,compact_percent?,expected_revision}`，至少一个设置非 null，expected_revision 必填。retries 取0–10整数、auto_compact 为布尔值、compact_percent 取1–99整数；省略或 null 的项保持原值。校验通过后事务保存并递增revision；旧revision返回409 conflict，无效输入返回400 invalid。两端需要同时升级。
- 接口沿用管理授权与 `X-Anchor-Instance`。CLI为 `agent codex-settings [--retries N] [--auto-compact=BOOL] [--compact-percent N] [--revision R]`，桌面为 `CodexSettings`／`SaveCodexSettings` 薄桥接；前端按Model／hook Controller／View组织。
- 保存成功后仅安全同步已启用且已接管的Codex目标，不修改其他Agent、不扩大模型授权、不接管未登记目录、不恢复已停用目标。外部编辑引起冲突时保留外部配置，偏好仍保存，`needs_sync`列出待处理Binding；前往Agent页确认更新或使用既有 `agent sync` 显式重新接管。
- 配置同步后须重启Codex（包括app-server）生效，不重启或终止用户进程。未启用／未接管的目标在之后接管时使用当前值。
- 前端在读取前禁写，输入校验范围；保存超时／断线返回unknown，revision冲突返回conflict，锁定编辑直到重新读取，不自动重试管理写入。实例切换卸载旧控制器并丢弃迟到结果；旧宿主缺少方法显示unsupported，不回退Mock。

## 自动压缩配置（2026-11-03 desktop.82 dev 已发布）

默认开启 Anchor 自动配置，按每个模型最终导出的 `context_window` 计算 `floor(context_window × compact_percent / 100)`，默认90%，至少1 token，使用整数运算避免浮点误差和 int64 溢出。优先应用显式模型上下文设置，再使用已登记元数据或接管前本地模板，不按模型名称猜测；价格策略已选择的工作窗口仍是有效导出窗口，不改成参考资料中的更大原始容量。窗口未知时不新增阈值，保留原模板／Codex 默认行为。只导出到 Anchor 自有目录，不修改保存的模型设置和模板。

目录里的每个模型设置 `auto_compact_token_limit`，使 Codex 切换模型后按各自窗口读取阈值。开启自动配置时，根级 `model_context_window` 和 `model_auto_compact_token_limit` 会覆盖这些模型值，故一起纳入 journal，保存原值后移除。关闭自动配置则还原根级恢复值，模型目录回到源模板／显式模型设置；Codex 原生自动压缩仍可运行。没有生成目录的旧 single 配置不新增这两个托管字段，不推测其窗口。

旧 settings JSON 缺少新字段时继承开启与90%，重试单项保存保持当前压缩偏好。启动会检测旧目录并进行条件同步，新增 journal 字段保存真实原值；切换开关不丢弃字段所有权或吸收 Anchor 写入值。后台遇外部修改只报告待同步，显式 `agent sync` 可重新接管并保存外部值作为恢复基线；停用／正常停服仍条件恢复。设置更改只同步已启用且已接管的 Codex，不自动登记其他目标。

桌面设置沿用 Model／hook Controller／View，显示自动配置开关和阈值百分比；读取、保存、revision 冲突、unknown 写入与设备切换遵循原设置边界。配置落盘并不保证压缩请求成功；压缩由 Codex 触发和执行，Anchor 不增加压缩端点、后台摘要请求或恢复重试。

## 旧配置迁移与网关边界

旧模型JSON中的 `channel_retries` 读取时忽略，不合并为统一值，之后模型设置保存不再输出此字段；模型管理API与文件写入拒绝该字段，旧导出文件须删除它。已托管Codex原先写入的0由正常启动同步或保存统一设置迁移，保留既有条件恢复基线。

网关单个入站请求对每个供应商／Key／候选模型通常仅尝试一次。唯一例外是原生 Responses 明确拒绝历史加密 reasoning，且尚未输出时，删除转发副本的 reasoning 并在同一渠道补发一次；整个入站请求最多一次清理重试，共享原超时并记录结果，详见 [加密上下文恢复](logging.md#历史加密推理恢复2026-10-20当前源码未发布)。保留安全的其他Key／候选切换、供应商超时、健康状态与日志；已开始输出的流及执行状态不明的失败不由网关重放。Codex负责判断并重试客户端请求／中断流，避免在两个层面反复请求同一渠道。

实现验证使用隔离状态目录、合成Codex配置与本地假上游，不读取或改动真实Agent配置，不调用付费上游。具体执行证据见本轮任务记录；原生桌面安装与真实Codex端到端运行须另行验收。
