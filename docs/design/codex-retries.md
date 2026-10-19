# Codex 统一重试（2026-10-19 当前源码）

本轮取消模型映射的同渠道额外重试，将重试交给 Codex 原生机制。统一设置保存在当前 Anchor 核心的状态目录，不是设备外观偏好；新模型与之后接管的 Codex 自动使用同一值。默认5次，范围0–10整数，不含首次请求，0关闭。生成的 Anchor provider 同时设置 `request_max_retries` 与 `stream_max_retries`，两项各自使用该上限；并非整轮操作最多请求6次的总预算。客户端实际重试仍取决于 Codex 的错误分类、版本与运行参数。

## 接口与状态

- Go DTO为规范源：`state.CodexSettings`、`state.SaveCodexSettings`、`managed.CodexSettingsState`，桌面契约从 `internal/desktop/contract.go` 生成TS；共享样例为 `desktop/contract/codex-settings.json`。
- `GET /control/v1/settings/codex` 只读，返回 `{settings:{retries,revision},needs_sync:[]}`。未保存时返回5、revision=1，不写入默认值。
- `POST` 同路径，输入 `{retries,expected_revision}`，两项必填，retries不可为null。校验通过后事务保存并递增revision；旧revision返回409 conflict，无效输入返回400 invalid。两端需要同时升级。
- 接口沿用管理授权与 `X-Anchor-Instance`。CLI为 `agent codex-settings [--retries N --revision R]`，桌面为 `CodexSettings`／`SaveCodexSettings` 薄桥接；前端按Model／hook Controller／View组织。
- 保存成功后仅安全同步已启用且已接管的Codex目标，不修改其他Agent、不扩大模型授权、不接管未登记目录、不恢复已停用目标。外部编辑引起冲突时保留外部配置，偏好仍保存，`needs_sync`列出待处理Binding；前往Agent页确认更新或使用既有 `agent sync` 显式重新接管。
- 配置同步后须重启Codex（包括app-server）生效，不重启或终止用户进程。未启用／未接管的目标在之后接管时使用当前值。
- 前端在读取前禁写，输入校验范围；保存超时／断线返回unknown，revision冲突返回conflict，锁定编辑直到重新读取，不自动重试管理写入。实例切换卸载旧控制器并丢弃迟到结果；旧宿主缺少方法显示unsupported，不回退Mock。

## 旧配置迁移与网关边界

旧模型JSON中的 `channel_retries` 读取时忽略，不合并为统一值，之后模型设置保存不再输出此字段；模型管理API与文件写入拒绝该字段，旧导出文件须删除它。已托管Codex原先写入的0由正常启动同步或保存统一设置迁移，保留既有条件恢复基线。

网关单个入站请求对每个供应商／Key／候选模型仅尝试一次。保留安全的其他Key／候选切换、供应商超时、健康状态与日志；已开始输出的流及执行状态不明的失败不由网关重放。Codex负责判断并重试客户端请求／中断流，避免在两个层面反复请求同一渠道。

实现验证使用隔离状态目录、合成Codex配置与本地假上游，不读取或改动真实Agent配置，不调用付费上游。具体执行证据见本轮任务记录；原生桌面安装与真实Codex端到端运行须另行验收。
