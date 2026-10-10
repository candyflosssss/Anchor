# 供应商桌面接口语义

- 更新：2026-10-11；标识：`anchor-desktop-v1`（替代仅供设计的 `provider-ui-prototype-v1`）。
- 状态：真实 Go 桥接、桌面传输和共享契约已实现；隔离核心读写已测试，**不等于 Wails 原生窗口或安装验收通过**。
- 桌面 DTO 权威：`internal/desktop/contract.go`；TypeScript 由 `go run ./cmd/desktop-contract` 生成，漂移由 Go 测试检查。核心业务规则仍归 `internal/state` / `internal/core`，不复制到宿主。
- 受限方法、窗口／核心生命周期与构建边界见[桌面运行契约](desktop-runtime.md)。

## 1. 读写与数据

| 前端端口 / Wails 方法 | 真实客户端与行为 |
| --- | --- |
| `list(signal)` / `Bridge.Providers()` | `client.Providers` → GET 当前实例目录；只回脱敏 DTO，空目录固定为空数组 |
| `discover(input, signal)` / `Bridge.DiscoverProviderModels(input)` | `client.DiscoverProviderModels` → POST `/control/v1/providers/{id}/discover-models`；显式读取上游目录，不改配置 |
| `save(input, signal)` / `Bridge.SaveProvider(input)` | `client.PutProvider` → PUT 指定 ID；revision=0 创建，>0 条件更新；普通字段完整替换 |

Provider 字段固定为 `id`, `base_url`, `protocol`, `enabled`, `timeout_seconds`, `max_concurrent`, `revision`, `has_secret`。不添加健康、余额、最近测试时间、display_name 等没有来源的字段。协议当前仅 `responses`，未知协议拒绝。Go 内部 Secret 不进入 DTO，前端传输再次挑选公开字段。

- 返回 revision 范围 1..Number.MAX_SAFE_INTEGER；请求 expected_revision 为 0..MAX_SAFE_INTEGER-1，且与输入 provider.revision 一致，防止更新后超出 JS 安全整数范围。
- 超时单位为秒，1..3600；并发为 1..64。前端即时校验只帮助填写，核心仍做最终业务校验。
- secret=null 保留、空字符串清除、非空替换；清除有明确确认。秘密只入不出、不写前端持久存储、日志或 URL；提交开始清空表单，失败需重新输入。不能承诺 JS/Go 字符串的物理内存擦除。
- 保存成功返回新 revision，不表示供应商连通性已经验证。

## 2. 查询、写入与错误

所有桥接结果含 `contract`、`data`、`problem`。版本不符、无绑定或无有效 data 时失败，不回退 Mock。`problem.code` 仅白名单，前端不解析任意异常文本。

| 桌面错误 | 语义 |
| --- | --- |
| offline / read_error | 无法连接或无法读取；数据不可用不等于空列表 |
| unauthorized | 401/403；授权留在 Go 层，不要求页面填管理令牌 |
| conflict | 409；不覆盖新配置，丢弃旧草稿重读后重新编辑 |
| invalid | 本地参数／核心校验拒绝，不以旧草稿静默覆盖 |
| unknown | 写入网络中断、5xx 或无法验证回包；可能已生效，须先查询，不自动重放 |
| identity / unsupported / closed / startup_failed | 宿主状态错误，统一由会话页解释；不开放供应商写入 |

`internal/client.RequestError` 提供 HTTP 状态、受限错误码与 Unknown 标记；桥接按结构映射，保留实例身份校验。查询／保存最多 15 秒（包含身份握手），Status 为 3 秒，启动最多 20 秒。Wails 调用不能通过浏览器 AbortSignal 保证取消 Go 操作：AbortSignal 与 generation 只阻止发起已取消的请求或消费过期响应，**取消不等于回滚**。

供应商 Controller 的同步 latch 防双击重复写；冲突与未知结果先重读。核心断开后供应商页面卸载、缓存与秘密草稿丢弃，Status 不自动重启；用户明确重新连接才再次 Start。当前只连接本机目录，不提供远程／多实例切换 UI。

## 3. 共享样例与两种模式

`desktop/contract/providers.json` 集中维护完全合成的供应商、空列表和错误样例，Mock 与 Go DTO/错误映射测试读取同一份内容；不放真实地址或秘密。

- `prototype`：显式内存 Mock，提供原型场景入口；密钥丢弃，只改 has_secret；刷新重置。
- `desktop`：仅真实桥接适配器，无场景入口；缺 Wails 绑定就报错，不能在普通浏览器中绕过管理鉴权。

管理 API 的 Origin/Host 防护不放宽，不增加 CORS 或通用 URL／shell／文件接口。测试分为共享 DTO/Mock、浏览器模拟 Wails 边界、真实 Go 宿主→client→隔离核心、原生桌面四层；前三层通过不能替代最后一层。

## 4. 上游模型目录小步（desktop.10）

规范 DTO `DiscoverProviderModels` / `ProviderModels` 定义于 `internal/catalog/models.go`，经 `internal/desktop/contract.go` 别名纳入 Go → TS 单一生成源；桌面 envelope 为 `ProviderModelsResult`，继续使用 `anchor-desktop-v1`，增量新增方法，不改变既有方法。共享合成样例为 `desktop/contract/provider-models.json`，Mock 与 Go / TS 测试共用。

- 输入仅 `provider_id` 与 `expected_revision`（桌面 1..MAX_SAFE_INTEGER）。必须匹配路径及当前已存供应商 revision，使用管理授权和 `X-Anchor-Instance`。不接收临时 URL、密钥或任意路径；冲突在发起上游请求前返回409。
- 后端从已存快照取地址／密钥，仅发一次 `GET <base_url 去尾斜杠>/models`，不自动追加 `/v1`。停用供应商也允许显式查询。复用无环境代理、禁止重定向的上游客户端与全局16／供应商并发上限；总耗时上限 `min(timeout_seconds, 8)` 秒，无重试、后台轮询或缓存持久化。
- 输出固定六字段：`provider_id`, `revision`（本次使用的快照版本）, `http_status`（未收到状态为0）, `elapsed_ms`（整数毫秒）, `models`（字符串数组，始终非null）, `failure`（成功为空字符串）。只接受 OpenAI 风格 `data` 数组中的非空 `id`，去重排序、丢弃元数据；最多512 KiB、1000条原始记录、每 ID 256字节，无控制／格式字符。异常目录整体失败，不返回部分列表。
- 请求受理后目录失败仍返回 HTTP200 与固定分类：`authentication`（401/403）、`unsupported`（404/405）、`rate_limited`（429）、`http_error`（其他非200）、`network`、`timeout`、`busy`、`too_large`、`invalid_response`。上游错误正文、原始网络异常、密钥不回显；包含已存密钥的 ID 也拒绝。管理层鉴权／冲突／实例错误仍走 `problem`，不混作上游结果。
- 不写配置、revision、模型映射或健康状态。一次目录成功仅说明此接口返回有效目录，不代表推理、工具、流式、余额或持续健康；404/405 不等于供应商推理不可用。
- UI 详情手动触发、禁重复点击，显示目录 HTTP／耗时／失败分类和只读限高列表。查询中阻止供应商切换、编辑和页面离开；选择、重读或保存后清空结果，取消／卸载丢弃晚到数据。Wails AbortSignal 不能保证中断后端请求，后端超时仍生效；查询错误不会显示“可能已保存”。新功能与旧核心不同步时失败关闭，不回退 Mock。
- CLI 等价入口为 `provider models --id ID --revision N`；与 `model list`（Anchor 已存公开映射）及网关 `/v1/models` 区分。测试分层沿 §3，原生两平台仍未验收。
