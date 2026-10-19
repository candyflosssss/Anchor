# Anchor 日志设计与实现

2026-10-12：已接入核心真实代理请求、推理测试与允许记录的系统事件。页面使用受控桌面接口，不在连接失败时回退虚拟数据。安装包和线上验收情况以[桌面发行说明](../release/desktop.md)及本次发布记录为准。

## 1. 产品基调与当前范围

日志围绕一次请求的排障组织：请求表格、可选字段、详情与尝试时间线、系统页签。HTTP、Anchor 结果、生成状态与客户端交付分别记录，因此 HTTP 200 可以同时显示 `invalid_response`、`response_validate` 和失败结果。该状态证明传输成功但 Anchor 校验未通过；不能仅凭原始报错推断具体缺失字段。

页面只保留标题、字段名、数据、操作控件、必要状态及错误；释义和指标口径写在文档，不放说明段落或解释性 Tooltip。费用单元格按用户要求提供数据型 Tooltip，悬浮或键盘聚焦查看捕获的乘区明细，Escape 关闭。原有 12 个显示块加上发送档位、加速状态及一个参考费用块，共 15 个显示块，默认布局及预设见[可选显示内容](log-columns.md)。列选择和顺序只属于展示偏好；隐藏字段不改变采集、计算、持久化、详情或导出。

当前提供请求/系统查询、结果/来源/供应商/模型/Key/HTTP/错误码/阶段筛选、标识前缀搜索、暂停轮询、刷新、历史翻页、请求详情、系统请求关联、复制安全摘要、JSONL 导出及保留设置。详情保存最后一次上游快照和最多 64 次尝试，另保留实际尝试总数及截断标识。未收到上游 HTTP、缺失用量或不能可靠计算的指标保持 null，界面显示“未知”或“未收到”。

只记录获授权的三个推理 POST 入口及实际推理测试。鉴权拒绝、任意 HTTP 路由、模型目录请求、桌面宿主自己的文件诊断不在本次请求日志范围内。没有新增自动重放、清空历史、正文搜索、日志推送订阅、管理审计、成本曲线或供应商测试页到日志页的快捷跳转。

### 逐请求参考费用（2026-10-19 当前源码，未打包）

代理和推理测试记录可选 `reference_cost`：`usd` 为可空十进制字符串，`currency=USD/basis=text_tokens`；同时保存 `source/provider_id/model_id/catalog_version/fetched_at/stale/pricing`，其中 pricing 保留基础价和全部上下文分档。无参考价格省略整个字段；有价格但缺用量或所需单价时 `usd=null`、`reason` 说明原因。明确免费保留零。列表、详情、复制和 JSONL 导出共享费用，详情／导出的尝试另保存可选 `reference_usd`。历史日志不回填，价格或映射修改不重算已保存费用。

核心优先取本次模型配置的参考快照；缺省仅取有效内存目录中所有候选精确匹配同一模型的快照。推理测试使用实际上游模型；不按公开别名或供应商名称猜测。计价不发起／等待网络目录刷新，冷缓存／过期缓存和不明确的模型不估算。未显式设置参考的加速映射不自动套标准价格；显式选择时仍仅作参考，不推算加速倍率。

复用 `modeldev.Estimate`：每次尝试分别选取输入阶梯，缓存读写从输入扣除后单独计价，思考包含在输出且不重复收费。根请求只在每次尝试均可完整估算时合计；未知尝试不会作为零费，尝试明细截断不影响已计算总额。OpenAI 两种协议没有独立缓存写入类别，确认总用量后该项为零；Anthropic 仅在实际报告字段时确认缓存写入。未报告缓存读取不补零。仅估算文本 Token，不纳入图像／音频专用费、请求费、服务档位附加费、未记录的平台附加系数、税费和其他折扣。统计图暂不汇总费用。

### 供应商/分组折算与乘区快照（2026-10-19 后续源码，未打包）

- 保持 `reference_cost.usd` 和尝试 `reference_usd` 为**原始模型费用**，不改变已有 JSON 语义。新增可选 `reference_cost.adjustment` 与尝试 `cost_adjustment`；其中 `usd` 是最终参考费用，`reason` 为未知原因。
- 每次派发前从选定供应商/Key 保存 `supplier_multiplier`、`group_multiplier`（十进制字符串或 null），以及分组 `group/group_name/basis/observed_at/failure`。供应商旧配置 0 沿用默认 1；Key 分组缺失不补 1。分组倍率已含平台有效覆盖，不再叠乘个人/高峰倍率；最近刷新失败时保留上次成功观测及失败标识，不声称是实时账单倍率。不进行价格/倍率网络查询。
- **最终参考费用 = Σ(每次原始模型费用 × 当次供应商成本倍率 × 当次 Key 分组有效倍率)**。各次原始费用与倍率都必须已知；零倍率保留零，但不让未知用量变成免费。部分未知使最终总额 null，不抹掉可算的原始费用。
- `parts` 包含互斥的 `input/cache_read/cache_write/output/reasoning`：`tokens`、`rate`（USD/百万 Token）、`rate_multiplier`（相对同次非缓存输入，12 位小数；分母零则 null）、原始 `usd` 和 `final_usd`。同价且思考未单报时输出包含未拆分思考（`includes_reasoning=true`），思考 Token 仍为 null，不推断零。上下文阶梯逐次选择，乘区金额由核心计算，前端只格式化。
- 根记录保留完整乘区汇总，列表即使省略 attempts 也能悬浮查看。重试系数或分组快照不同时 `supplier_mixed/group_mixed` 标记逐次，相关根倍率为 null；单价不同则 `mixed_rate=true`、根单价/单价倍率为 null，不用最后一次覆盖。详情可核对每次快照。若丰富快照导致超过 32 KiB 上限，截断尾部尝试明细并设置 `attempts_truncated`，保留根次数、原始/最终总额与乘区汇总；超大根本身仍按存储预算丢弃。
- 当前后续源码将界面原四列合为一个“参考费用”块，按原始费用、供应商倍率、分组倍率、最终费用四行同时显示；最终金额加重，悬浮明细保留，请求详情与尝试时间线使用同一摘要。无模型参考价整个块显示“—”；旧日志有原始费用但没有倍率快照则新项“未记录”，不按当前配置补算。未计算金额“未知”，明确零显示 $0。当前配置后续修改不重算历史。CLI `logs list/detail/follow/export` 共享新增字段，无新增命令。

## 2. 参考项目与取舍

按 2026-10-11 查阅到的以下源码快照研究；这里只描述选定文件能证明的行为，不据此宣称对方所有页面、协议或平台均已验收。CPA 指 CLIProxyAPI，CCS 指 Claude Codex Switch（`kaitranntt/ccs`），与 CC Switch 分开。

| 项目 | 可核对的日志设计 | Anchor 采用 | Anchor 的取舍 |
| --- | --- | --- | --- |
| sub2api | UsageLog 包含 request ID、请求/实际/响应模型、Token、缓存、成本、流式、总耗时与首 Token 耗时；另有请求明细保留策略测试与错误趋势查询 | 调用维度、模型映射快照、已知用量、明细与汇总生命周期分离 | 不引入用户充值、订阅和计费账本；缺失用量保持未知，不直接照搬字段的零默认值 |
| new-api | Log 区分消费、错误、系统等类型，具有 request ID / upstream request ID、模型、渠道、耗时、Token 等字段；失败详情支持展开和复制 | 可筛选列表、错误详情、请求关联和安全复制 | 不把所有事件塞进一个任意 `other` 字段；复制内容由 Anchor 生成安全摘要，不直接复制上游错误正文 |
| CPA | 配置区分应用文件日志、代理请求日志与用量统计，并提供总大小/错误文件数约束；请求日志格式包含请求、响应、上下游段及敏感头值掩码 | 上下游分层、独立开关、容量约束和失败取证思路 | 首版不保存其完整正文取证格式；只保留协议结构事实与固定错误解释 |
| CCS | 独立结构化 JSONL、稳定事件、request ID、阶段、process/run ID、脱敏及轮转；明确禁止原始正文、提示词与凭据进入日志 | 系统日志契约、请求阶段关联、有限字段、脱敏作为第二道防线 | 不依靠字符串替换保证安全，也不把终端提示当成历史日志 |

固定版本源码入口见[参考来源](#8-参考来源)。以上是借鉴后的 Anchor 设计决策，保留天数与容量是本文选择的默认值，并非对方项目默认值。

## 3. 采集、用量与隐私

根请求保存开始/最终记录、公开模型、请求协议、可识别的 reasoning effort 和流式设置。每次真正派发的上游尝试保存供应商、Key 安全标识、实际模型、协议、HTTP、耗时、结果、阶段及确认的输入输出用量。路由未派发时尝试数为零。provider revision 是最后一次上游配置版本；模型映射 revision 另保存在安全诊断中。已保存的历史不依赖当前供应商对象仍然存在。

代理请求总耗时覆盖解析、路由、等待、重试、上游输出及现有用量统计写入。界面首字时间取首次上游有效输出的本机观测时刻：非空正文增量或工具调用参数增量（含 Responses 自定义工具输入），保存在新增的 `metrics.time_to_first_output_ms`；包含之前的重试/等待，不计角色、心跳、网络首包、工具空壳、空增量及单独的思考事件。`metrics.time_to_first_text_ms` 独立保留正文首字供正文区间计算；两者均不是客户端接收确认。旧日志无新字段时回退正文首字，旧的纯工具请求不能可靠补算；无需数据库迁移，不存储工具参数。桌面总 TPS 使用根请求 `output / (elapsed_ms / 1000)`：总输出已含思考，不重复累加或扣除；总耗时包含等待、思考、重试与传输，衡量端到端效率，不是纯解码速度。成功结果、有限非负总输出与正总耗时即可计算，不依赖流式、首字或独立 reasoning 用量；纯思考和非流式亦可计算。零输出显示0，失败/中断/处理中或缺数据未知。多次尝试不使用已知小计代替完整总量。已有历史满足条件即可按新口径展示，无需补写；`output_generation_ms/output_interval_complete/non_reasoning_output_tokens` 及正文指标独立保留，不再作为 TPS 依据。具体字段口径见[列文档](log-columns.md)。

一次尝试缺失 usage 时，根请求输入/输出/总 Token 均未知，详情仍显示各次确认值；不会把已知小计当完整总量。明确报告的零与未报告分开。现有累计用量继续独立写入 state.db，日志到期不删除累计用量。

日志不记录请求/响应正文、提示词、思考原文、工具参数、认证头、密钥或任意上游错误文本。probe 只记录固定错误码及 `body_bytes`、`json_valid`、校验通过等有限结构事实。2026-10-13 当前源码（未发布）在失败详情的 `evidence` 中增加 `validation_reason`：`invalid_json`、`missing_response_id`、`upstream_error`、`unsupported_content`、`unsupported_output`、`invalid_tool_call`、`invalid_choices`、`refusal`，未知错误固定为 `invalid_structure`；不直接存储解析错误字符串，其中可能含任意上游字段名。已有 evidence 字典契约、查询/复制/导出沿用，列表仅保留下述档位证据白名单，历史记录不补写。文本项的 `logprobs` 和 `annotations` 只用于测试结构校验，不进入日志。系统事件使用代码消息与属性白名单。复制和导出来自这些安全结构化字段，不触发新的上游请求。未知 reasoning effort 不作为任意字符串写入日志。

2026-10-13 当前源码（未发布）修正完成响应的取消误判：协议正常结束且响应写入成功后，即使客户端在日志／用量收尾前断开，也保留 succeeded/complete；尚未完成或写入失败时的客户端取消仍保存 cancelled。已经写出 HTTP 200 数据的失败请求保存 partial；没有写出数据则 none。success/complete 只表示本机写入没有报错，不代表最终用户确认收到。历史记录不自动重判；总 TPS 仅对成功且总输出与总耗时已确认的记录计算，不以首字后的区间作为依据。旧 pending 在核心重新打开数据库时改为 interrupted/core_interrupted，交付结果 unknown；NORMAL WAL 在突然断电时可能丢失最近批次，不承诺断电零损失。

2026-10-16 当前源码（未发布）：服务档位采用原值优先、可识别值附加标注的方式，代理及推理测试共用以下 `evidence` 口径：

- `requested_service_tier` 是入站 JSON 值的规范化标签，未传为 `unspecified`；固定映射策略仍单独记录在 `mapping_service_tier`。`sent_service_tier` 取固定策略及协议转换后的实际派发请求体，未派发为 `not_sent`、未带参数为 `unspecified`。派发表示进入网络发送步骤，不证明上游收到。
- `upstream_service_tier` 取最后候选的 OpenAI 兼容 JSON 或 SSE 返回值；缺少报告则缺省。上述三个观测各自可带同前缀的 `*_service_tier_raw`，值为 **JSON 标量的文本**，如 `"ultrafast"`、`"turbo-v2"`、`""`、`null`、`false`、`42`。保留未知名称与标量类型，不把原始字符串 `"unknown"`、`"unspecified"` 混同缺失哨兵；JSON 字符串转义会规范化，不承诺保留线上字节排版。
- 安全边界：原始字符串最多 128 UTF-8 字节、序列化后最多 256 字节；对象、数组、控制字符、可识别凭据片段及当前可用的认证秘密不进入原值字段。被抑制的值保留 `unknown` 标签而不保存原值，不保存请求／响应正文。规范化标签只允许 `auto/default/priority/fast/ultrafast/flex/scale`；其余为 `unknown`。
- `fast_triggered` 为字符串 `true/false/unknown`：上游明确报告 `priority/fast/ultrafast` 为 true，`default/flex/scale` 为 false，缺失、auto 或其他值为 unknown。它只代表**上游报告的档位**，不证明实际速度、计费或完全符合请求。界面另外比较派发与返回档位：Ultrafast → Fast 标“仅确认 Fast”，请求加速但返回非 Fast 标“未按请求加速”；未知不误判成失败或未加速。
- SSE 后续报告覆盖先前报告（含未知或被抑制值）；切换候选清空前一候选的派发／返回快照。失败／中断时保留已观测报告，不与请求成功混为一谈；非 200、错误内容类型或无法解码等提前退出路径不额外读取正文取证，仍为未知。字段描述最后候选，不新增逐尝试档位历史。
- 列表与 `follow` 仅保留五项紧凑证据：`sent_service_tier`、`sent_service_tier_raw`、`upstream_service_tier`、`upstream_service_tier_raw`、`fast_triggered`，仍省略 attempts 及其他 evidence。详情、复制与 JSONL 导出保留完整安全证据；历史不补写，旧日志已知标签可继续标注，但不从模型名、固定策略或速度反推原值。

桌面增加“发送档位”“加速状态”显示块：原始 JSON 值为主，青色闪电标 Fast、紫色火箭标 Ultrafast，辅以文字和确认／不匹配状态。`auto`、`flex`、`scale` 单独显示，不算已确认 Fast。核心运行日志 `model request` 只输出安全规范化的请求／派发／返回档位与 fast_triggered，不输出任意原值。本改动仅观测，不改写请求、不启用服务档位、不改变跨协议转换支持范围。


### Agent 托管失败诊断（2026-10-16，desktop.43 dev 已发布）

Agent 管理接口在处理 workspace、enroll、add、inspect、plan、sync、set-models、apply、restore、enable/disable 时返回的操作错误，现在通过系统事件 `agent.operation_failed` 记录，级别 `warn`、来源 `core`。inspect 读取托管状态失败但仍返回降级状态时也记录，动作标为 `inspect-status`。正常成功操作不新增失败事件；请求格式、鉴权、实例身份等前置拒绝不在本次诊断范围内。启动和自动同步仍沿用现有状态/汇总事件，本次不声称完整覆盖后台生命周期。

事件 `fields` 包含 `action`、`stage`、`reason`；能从已保存绑定解析时，附带 `binding_id`、`adapter_id`；已知冲突可附带固定 `field` 标签。主要诊断示例：

| stage / reason | 事实含义 |
| --- | --- |
| `apply_precondition / unfinished_journal` | 托管记录尚未完成，阻止开始新的覆盖写入；不代表配置一定已被写入 |
| `apply_precondition / active_journal` | 引擎已有生效中的托管记录，拒绝重复创建 |
| `read_configuration / configuration_changed_since_plan` | 写入前读取的配置与计划时的文件指纹不符 |
| `write_configuration / configuration_changed_before_write` | 原子替换前的最后一次比较发现文件变化 |
| `plan_check / plan_changed` | 用户确认的计划指纹与重新计算的计划不符 |
| `verify_written / managed_field_mismatch` | 本次写入后立即校验未通过；不能据此认定用户/其他程序改过配置 |
| `verify_existing`、`apply_precondition`、`sync_verify` 下的 `managed_field_mismatch` | 已有托管字段或同步前校验与预期不符 |
| `recovery / recovery_conflict`、`catalog_mismatch`、`catalog_exists` | 恢复冲突、目录内容不符或目录文件已存在 |

不带专门阶段的错误使用 `operation` 和安全类别，例如 `revision_conflict`、`model_unavailable`、`catalog_invalid`、`unsafe_configuration`、`permission_denied`；未知原因固定为 `operation_failed`。`field` 只从代码白名单映射，例如 `dsh.default_reasoning`、`dsh.provider`，不序列化原始字段路径或动态 provider ID。事件不包含底层 `error.Error()`、文件绝对路径、配置内容、比较前后值、指纹、Token 或认证头；终端日志和持久系统日志遵循同一数据边界，事件属性白名单不扩展到其他系统事件。

在日志页的**系统**页签查看，或使用既有 `anchor logs list --kind system --since 1h`、`anchor logs export --kind system --since 1h`。沿用当前上下文的 owner 管理权限、保留配置和 JSONL 导出，不增加命令或改变 API 错误码。只对升级后新发生的失败采集，历史错误不会补录；日志丢弃/写入失败仍受既有日志健康状态约束。本改动不放宽任何配置保护检查，也不自动重试覆盖写入。

## 4. 存储、性能与保留

独立 `logs.db` 与 state.db 位于同一受保护 data-dir。SQLite WAL、单写连接、两个只读连接、2048 项非阻塞队列、128 项批量、100 ms 刷新。提交时形成不可变的有界 JSON 快照；单请求上限 32 KiB、系统事件 4 KiB，队列满/超限立即丢弃并计数，转发不等待日志数据库。生产者仍承担字段整理及有界 JSON 编码成本。

默认请求 7 天、50,000 条、256 MiB 逻辑预算；系统 7 天、32 MiB，系统条数另限制 10,000。可配置请求/系统 1–90 天、请求 100–200,000 条、请求 1–512 MiB、系统 1–64 MiB。逻辑预算含每条预留开销，不等于实际文件大小。每批及每秒清理最多两轮 128 条，缩小设置后逐步收敛，不阻塞一次性清空；每分钟有限增量 vacuum 和 checkpoint。计数通过触发器维护，不用每次 COUNT 全表。

索引覆盖时间、状态、来源、模型、供应商、Key、HTTP、错误码、阶段，系统索引覆盖时间/级别/来源/事件。游标使用单调序号，分页 1–200 条；桌面每页 50 条，不加载全量历史。数据库页上限 262,144（新库 4 KiB 页，即 1 GiB），写缓存 8 MiB；WAL 自动 checkpoint 256 页，完成 checkpoint 后保留文件目标 4 MiB。活动读事务期间 WAL 可暂时超过该目标，它不是整个数据库目录的硬配额。

页面显示 dropped、write_failures 与存储错误；队列故障不得被伪装成“没有日志”。缺口计数与保留清理不同；退出/断电前未刷盘的计数可能丢失。这里没有事件订阅或精确丢失区间协议。

## 5. 管理接口、CLI 与桌面边界

以下接口继承 owner 鉴权、Origin 防护和实例身份核验：

| 接口 | 行为 |
| --- | --- |
| POST `/control/v1/logs/query` | 过滤及游标分页，3 秒查询上下文；列表省略 attempts/evidence 大字段 |
| GET `/control/v1/logs/requests/{id}` | 完整安全详情，3 秒上下文 |
| GET / PUT `/control/v1/logs/settings` | 查询/校验/持久保留配置 |
| POST `/control/v1/logs/export` | 每页 100 条，最多 200,000 条，2 分钟，逐条 JSONL 写出 |

导出固定序号上界并保留完整详情，记录随后更新或到期仍可能影响内容；不承诺事务级全库快照。中途查询或传输错误令下载失败，桌面删除自己的 partial 文件。宿主原生保存对话框选择路径，JS 不能提交任意路径；只在成功写入、Sync、Close 后 Rename。UI 列选择不影响导出字段。

CLI 提供 `anchor logs list|detail|follow|export|settings|configure`，参数、默认值与示例以[最新指令大全](../cli/commands.md)为准。follow 每 2 秒输出有界页快照，可重复/更新 pending 记录，不是逐条去重事件流。远程调用沿用已选 Context，保留设置属于对应核心实例。

DTO 规范源为 `internal/desktop/contract.go` 的 logbook 类型别名；`cmd/desktop-contract` 生成 TS，Go 与 TS 共用 `desktop/contract/logs.json`。前端 MVC：log.service.ts 调用类型化传输；hook Controller 管查询/分页/轮询/详情/保存；View 渲染。切换实例卸载旧控制器，轮询与异步结果受生命周期代号保护。首屏每 3 秒读取当前页，历史页停止自动轮询；新请求不会主动滚动页面。页面退出不停止独立核心采集。

## 6. 已执行验证

隔离合成上游验证真实查询/详情/导出/配置、身份保护、HTTP 200 probe 校验失败、429→成功、路由零派发、流式正文指标、取消与部分交付。存储验证未知/零、不可变提交、超限隔离、队列溢出、恢复、保留与安全白名单。共享契约和真实桌面桥接到隔离核心另有测试。

10 万条持久历史实测写入 12.90 秒；100 次索引查询 p50 0.298 ms、p95 0.550 ms；数据库 107 MiB、配置物理页上限 1024 MiB；查询前后 Go heap 约 2.35/2.49 MB，队列最终为零，无丢弃；EXPLAIN 使用 request_model。收紧到 1000 条后有界清理收敛。该结果来自本机合成负载，不代表所有磁盘或上游性能。8 客户端并发、800 次请求 × 16 个流式增量的隔离合成测试：关闭日志持久化时首包 p95 9.39 ms，开启时 11.20 ms；总运行 0.758/0.814 秒，无丢弃。该对比包含既有用量写入，测的是本机首个字节，不等同于正文首字。浏览器与发布检查的最终结果见本次任务记录。

未运行真实付费上游、未修改真实 Agent 配置；Windows/Linux 原生安装和窗口行为尚未验收。浏览器模拟绑定、Go 真实桥接联调、生产编译、安装包下载和实际系统安装分别记录，不互相替代。

## 7. 后续范围

清空历史、离线文件查看、宿主诊断接入、推送订阅、精确缺口区间、跨进程关联和管理审计可以另行扩展。内容诊断需要单独的授权、短时失效、存储与导出契约，不把 debug 解释为允许保存提示词或 reasoning 原文。

## 8. 参考来源

源码快照固定 SHA，便于复核，不把移动的默认分支当稳定契约：

- sub2api `3a6fd1c9db07203ca308aaba69e502bc1f35b307`：[UsageLog](https://github.com/Wei-Shaw/sub2api/blob/3a6fd1c9db07203ca308aaba69e502bc1f35b307/backend/ent/schema/usage_log.go)、[请求保留策略测试](https://github.com/Wei-Shaw/sub2api/blob/3a6fd1c9db07203ca308aaba69e502bc1f35b307/backend/internal/service/request_log_retention_test.go)、[错误趋势查询](https://github.com/Wei-Shaw/sub2api/blob/3a6fd1c9db07203ca308aaba69e502bc1f35b307/backend/internal/service/ops_errors.go)。
- new-api `42e694eb60fe3191795c00d416571ced716eb9c4`：[日志模型](https://github.com/QuantumNous/new-api/blob/42e694eb60fe3191795c00d416571ced716eb9c4/model/log.go)、[失败详情](https://github.com/QuantumNous/new-api/blob/42e694eb60fe3191795c00d416571ced716eb9c4/web/src/features/usage-logs/components/dialogs/fail-reason-dialog.tsx)。
- CPA `f1d4a94ef4398349716f2db5bbc3a27624c174a7`：[配置](https://github.com/router-for-me/CLIProxyAPI/blob/f1d4a94ef4398349716f2db5bbc3a27624c174a7/config.example.yaml)、[请求日志格式](https://github.com/router-for-me/CLIProxyAPI/blob/f1d4a94ef4398349716f2db5bbc3a27624c174a7/internal/logging/request_logger_format.go)。
- CCS `f45fa923f231ecb95b782d3a9edde3cad3476d48`：[Logging Contract](https://github.com/kaitranntt/ccs/blob/f45fa923f231ecb95b782d3a9edde3cad3476d48/docs/logging-contract.md)、[日志类型](https://github.com/kaitranntt/ccs/blob/f45fa923f231ecb95b782d3a9edde3cad3476d48/src/services/logging/log-types.ts)、[轮转存储](https://github.com/kaitranntt/ccs/blob/f45fa923f231ecb95b782d3a9edde3cad3476d48/src/services/logging/log-storage.ts)。

## 9. 请求趋势统计（2026-10-16，当前源码未打包）

**发行补记（2026-10-16）**：本节标题的“当前源码未打包”记录功能提交时状态；本功能现已随 [desktop.42 dev](../release/desktop.md#当前在线-dev-版-42) 发布，两平台安装包与清单已投放。未自动安装或替换运行程序，Linux WebKitGTK／Windows 原生窗口及安装验收仍未执行。

请求日志页在筛选区与列表之间增加运行趋势。可切换请求量、已知 Token 用量（输入／输出／合计）、总 TPS、错误率、缓存命中率、平均耗时及首字延迟；不同单位不混在同一坐标轴。使用现有日志语义色，支持浅／深主题、窄窗口、鼠标时间点和键盘左右／Home／End 读数，不增加图表依赖。稀疏展示在 desktop.45 dev 曾以淡虚线连接无请求空档；当前源码（2026-10-19，未打包／未发布）改为缺失值刻度按0绘制，连续空桶保持在0基线，首尾空桶同样回到0。鼠标与键盘读数使用相同的0值，提示保留原时间桶并标注「缺失值按 0 显示」及无请求／无可计算指标原因。

时间范围为最近 15 分钟、1 小时、24 小时、7 天、30 天、90 天；统计粒度为自动、10 秒、30 秒、1 分钟、5 分钟、15 分钟、1 小时、6 小时、1 天。自动选择不超过120个桶的最细可用粒度；手选最多600个桶，超限选项禁用，扩大范围导致粒度不适用时回到自动。统计按请求开始时间归桶，范围为 `[since_ms,before_ms)`，首桶从 since_ms 开始，末桶可以不足完整间隔；不是请求完成时刻或 UTC 整点对齐。

### 接口与生命周期

- 核心：`POST /control/v1/logs/stats`，管理鉴权、实例身份和 Origin 防护沿用核心管理边界。输入 `StatsQuery {filter: Query, interval_ms}`，只允许请求日志；时间跨度最多90天，interval_ms=0 表示自动，其余只能是上述粒度。无写入、不访问上游。
- `filter` 复用列表的结果、来源、供应商、模型、Key、HTTP、错误码、阶段和标识前缀规则；忽略 limit/cursor/until，level 不适用。不从当前分页、前端累计缓存或 JSONL 导出计算。后端在日志只读连接中扫描所选保留数据，仅提取数值字段，内存随桶数而非历史条数增长，核心超时3秒。
- 输出 `Stats {since_ms,before_ms,interval_ms,buckets,summary,health}`；bucket 为 `{at_ms,values}`。Go `internal/logbook/stats.go` 是规范源，经 `internal/desktop/contract.go` 引入并生成 TS；共享合成样例为 `desktop/contract/log-stats.json`。
- 桌面 `LogStats` 使用标准 Result，超时5秒，复用 client→管理接口。Controller 独立每15秒刷新，避免每3秒列表轮询都重扫历史；全局暂停停止后续自动刷新，手动刷新、条件／粒度变化仍读取。翻页不改变趋势范围。切换条件清空旧统计并丢弃过期响应，离开页签／切换实例卸载后不再消费旧结果。宿主缺绑定、旧核心或读取失败明确显示不可用，不回退 Mock。
- CLI 为 `anchor logs stats`，参数与 JSON 结构见[最新指令大全](../cli/commands.md)。统计属于保留日志，不是永久账本；时间窗超过保留天数、容量淘汰或 health 丢弃记录会使历史不完整，90天选项不代表有90天数据。

### 聚合口径

| 指标 | 口径 |
| --- | --- |
| requests | 全部匹配根请求（含 pending/cancelled），重试不另算请求 |
| input/output/total_tokens | 仅累加输入和输出都已确认且非负的请求；total=input+output，输出已含思考，不再加缓存或思考。usage_samples 为贡献请求数；没有样本为 null，部分已知小计明确标「已知用量」，不声称是完整消费 |
| tps | 仅成功且输出非负、总耗时为正的请求，`sum(output)*1000/sum(elapsed_ms)`，按总耗时加权，非逐请求 TPS 的简单平均；含思考、等待、重试和传输，与列表总 TPS 同源。tps_samples 给出样本数 |
| error_rate | `errors/completed*100`；errors=failed+interrupted，completed=succeeded+failed+interrupted；不将 HTTP200 直接判成功，pending 和客户端 cancelled 均不计分母；分母为0时 null |
| cache_hit_rate | 仅单次尝试、输入>0、缓存读数已确认且 `0≤cache≤input` 的请求，`sum(cache_read_tokens)/sum(input)*100`；按 Token 加权，不是「有缓存请求占比」，不将缺失读数当0；cache_samples 给出样本数 |
| latency_ms / first_output_ms | 对相应已知非负观测求均值；总耗时排除 pending，首字优先有效输出首字，旧日志回退正文首字；各带样本数 |

空桶请求数为0；无用量、速度或率的样本在接口中仍为 null。当前图表仅在绘制与时间点读数时将 null 按0展示，已观测到的0保持0；汇总卡片、日志详情和 CLI 统计保留未知值语义。整个范围没有请求和读取失败分别展示，后者不伪装成空历史。总览直接使用相同查询的累加器，不平均各桶比率。

本轮验证与平台边界记于当前任务记录：Go 隔离管理链路与浏览器模拟 Wails 绑定分别验收，不代表 Linux WebKitGTK／Windows 原生窗口已验收。两平台安装包和在线更新已随 desktop.42 首次发布；desktop.43 已按用户要求完成本机宿主与核心同步升级，原生窗口验收仍未执行。


## 10. 筛选与搜索候选（2026-10-16，desktop.43 dev 已发布）

`POST /control/v1/logs/suggestions` 是只读 owner 管理接口，校验当前实例身份，最多3秒；不调用供应商，不读取请求正文、证据 JSON、真实密钥或当前供应商配置，只查询保留日志的标识列。Wails `LogSuggestions` 复用 client，最多5秒，DTO 由 Go 生成 TS，共享样例 `desktop/contract/log-suggestions.json`。

输入为 `{kind,field,prefix,since_ms,before_ms,limit}`：kind 默认 requests；field 必填，允许 search/provider/model/key/http/code/stage。system 仅支持 search（事件名）。前缀区分大小写、最长512字节，百分号／下划线不是通配符。默认最近1小时至现在，包含起止时间，最多90天；limit 默认20，范围1..100。不接受页码、游标、状态或其他字段条件，故在选择某一值后仍可清空文本查看同一时间范围的其他值。

输出 `{items:[{value,count}],has_more}`；去重、跳过空值／未知 HTTP，按出现次数降序再按值升序稳定排序。HTTP 返回十进制字符串，Key 是安全日志 ID；请求 search 合并请求 ID、模型、供应商、Key 与错误码，一条请求中重复出现相同值只计一次。候选仅来自根请求，与列表筛选列一致，不混入未用于列表筛选的尝试字段。总数超 limit 时 has_more=true，可输入更长前缀；没有匹配返回空数组而非错误。保留策略、丢弃与写入失败也限制候选范围，不代表永久历史。

前端仅焦点框请求，180ms 防抖；值、页签、时间范围变化或卸载后丢弃旧响应，连接切换由实例键卸载。选择候选立即走既有精确过滤／前缀搜索；手动输入一直可用。候选失败／旧核心无接口不让整个日志页失败；不持久化候选，不伪造值。中文组合输入期间不查询或用 Enter 误选。无新时间／结果／来源枚举。

CLI `logs suggestions` 通过同一客户端查询，详见[最新指令大全](../cli/commands.md#12-请求与系统日志当前源码基础能力自-desktop16-dev)。测试采用隔离核心与浏览器受控桥接；desktop.43 已发布并完成本机用户目录升级，Linux WebKitGTK／Windows 原生窗口未验收。


## 11. 归属与分类统计（2026-10-16，desktop.44 dev 已发布）

请求记录新增可选 `agent`、`binding`、`project`、`session`。只读归属信息与模型／供应商路由无关，不改变请求内容、鉴权或重试规则。

- `agent` 是已验证 Binding 的 `adapter_id`（如 codex），`binding` 是登记目标 ID。Binding 是配置目标而不是进程；同一类型可按绑定进一步区分。共用全局网关凭据的调用、推理测试及旧记录没有可靠 Agent 归属，不根据 User-Agent、IP、模型、提示词或时间猜测。
- `project` / `session` 优先接受调用端显式设置的 `X-Anchor-Project` / `X-Anchor-Session`。它们是客户端自报标签，不是经 Anchor 验证的本地项目或会话身份。每个字段只接受单一值、1..128 字节、首字符 ASCII 字母或数字，其余还允许 `-_.:@`；重复、空、过长、路径／任意文本等不合规值忽略，不中断推理。与当前请求鉴权凭据完全相同的值也会丢弃。不要把密钥、正文、完整工作目录放到标签中。这两个头只供本地归属采集，不转发上游。
- **Codex 原生归属（desktop.45 dev 已发布）**：仅对认证的 codex Binding，读取单值、最多8 KiB 的 `x-codex-turn-metadata`，优先取 UUID `thread_id`、其次 `session_id`，兼容旧单值 `session_id` 请求头；实际采集入口以隔离 Codex 0.162.1 对本地模拟上游验证。子线程独立分类，不取 turn/window/installation/parent/cache ID，不根据请求正文、User-Agent、IP或时间猜测。合法的显式 Anchor 标签优先，其他 Agent／全局凭据不采集 Codex 头。
- 项目仅以原生线程 ID 精确查询该 Binding 配置根下 Codex 最新 `state_N.sqlite` 的结构化元数据，遵循配置中的 `sqlite_home`（相对路径以配置根解析）。优先取已登记项目集合的名称／ID，否则用线程 `cwd` 的目录名／路径摘要。日志只保存「可读名称@12位摘要」，同名目录不会合并，不保存完整路径、项目原ID、会话标题、消息或整个 metadata 头。目录枚举最多256项；只读单连接、20ms忙等待、100ms查询上下文，查询后关闭；不迁移／修改 Codex DB，不扫描 rollout 或回填历史。符号链接／不安全权限、缺DB／不支持的结构／查无线程时项目保持未识别，原生会话ID仍可采集。元数据位于别的SSH设备／其他用户时不会跨机猜测。
- 归属是请求自报与本地元数据关联，不是安全身份。Codex 未提供原生字段时，仍须显式标签；其他客户端自动接入尚未实现。旧日志没有字段时显示“未识别”；候选只基于当前范围保留的请求，不导入所有客户端历史会话。

列表、趋势、导出复用 `Query.agent/binding/project/session` 精确筛选：缺省／null 不筛选，显式空字符串只选缺失或空字段。系统事件不支持这四个筛选，提供则拒绝；请求前缀搜索和 `LogSuggestions` 的字段白名单也增加这些维度，候选不包含空值。存储沿用 JSON 文档，新增幂等表达式索引，不重写历史记录或升级 SQLite user_version。

`LogStats(StatsQuery)` 增加可选 `group_by=agent|binding|project|session`。沿既有授权、实例校验、3秒查询限制和90天／600桶限制，仍对筛选范围的所有保留请求统计，不受页大小／游标影响。回复增加 `group_by`、`groups:[{value,values}]`、`group_count`；每组复用 StatsValues 的原始累加器，Token／TPS／错误率／缓存命中率遵循同一分母和 null 规则，不能平均各请求的百分比或 TPS。空 `value` 单独代表未识别。组按请求数降序、同数按值升序，至多返回100组；group_count 表示完整组数，summary 与 buckets 始终覆盖所有匹配请求，**不是**前100组的合计。没有分组时响应省略分组字段。

桌面 Go DTO 与生成的 TypeScript 类型同源；共享样例 `desktop/contract/log-dimensions.json` 同时用于存储与前端验证。分类选择属于 hook Controller，Model 构造与验证类型化查询，View 仅展示分类表和发出筛选动作；新维度在全局过滤状态中保存，点击组后列表／曲线／导出保持一致。默认不分类；旧核心缺少字段时不伪装成有效分组，显式选择新分类失败提示核心版本，不回退 Mock。CLI 同步支持 `logs stats --group-by` 和相关筛选，具体命令见[最新指令大全](../cli/commands.md)。

归属筛选／分组基础能力已随 desktop.44 宿主／核心发布到默认 dev 在线源，并完成本机 CLI／核心／GUI 同版用户目录升级；desktop.44 仍仅接受显式项目／会话标签。上述 Codex 原生归属与稀疏曲线增量已从干净提交打包为 desktop.45，并于2026-10-16 19:09完成本机 CLI／核心／GUI 升级；两平台包及哈希、本机核心／网关就绪、六档统计／四维分类／候选／导出验证通过。同版现已于北京时间 2026-10-16 20:03:09 发布到默认 GitHub dev 在线源，两平台安装包、清单及源码／元数据／校验附件齐全；Linux 默认源真实下载与大小／SHA256校验通过，Windows 附件 digest 与本地一致。在线投放未重启本机，SSH 远端未更新；旧日志不补归属。原生归属复用隔离 Codex 与本地模拟上游验证，未请求真实付费推理；原生窗口／系统安装器未验收。详见[desktop.45 dev 发行记录](../release/desktop.md#当前在线-dev-版-45)。
