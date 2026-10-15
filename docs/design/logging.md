# Anchor 日志设计与实现

2026-10-12：已接入核心真实代理请求、推理测试与允许记录的系统事件。页面使用受控桌面接口，不在连接失败时回退虚拟数据。安装包和线上验收情况以[桌面发行说明](../release/desktop.md)及本次发布记录为准。

## 1. 产品基调与当前范围

日志围绕一次请求的排障组织：请求表格、可选字段、详情与尝试时间线、系统页签。HTTP、Anchor 结果、生成状态与客户端交付分别记录，因此 HTTP 200 可以同时显示 `invalid_response`、`response_validate` 和失败结果。该状态证明传输成功但 Anchor 校验未通过；不能仅凭原始报错推断具体缺失字段。

页面只保留标题、字段名、数据、操作控件、必要状态及错误；释义和指标口径写在文档，不放说明段落或解释性 Tooltip。27 项字段归为 12 个显示块，默认布局及预设见[可选显示内容](log-columns.md)。列选择和顺序只属于展示偏好；隐藏字段不改变采集、计算、持久化、详情或导出。

当前提供请求/系统查询、结果/来源/供应商/模型/Key/HTTP/错误码/阶段筛选、标识前缀搜索、暂停轮询、刷新、历史翻页、请求详情、系统请求关联、复制安全摘要、JSONL 导出及保留设置。详情保存最后一次上游快照和最多 64 次尝试，另保留实际尝试总数及截断标识。未收到上游 HTTP、缺失用量或不能可靠计算的指标保持 null，界面显示“未知”或“未收到”。

只记录获授权的三个推理 POST 入口及实际推理测试。鉴权拒绝、任意 HTTP 路由、模型目录请求、桌面宿主自己的文件诊断不在本次请求日志范围内。没有新增自动重放、清空历史、正文搜索、日志推送订阅、管理审计、成本曲线或供应商测试页到日志页的快捷跳转。

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

代理请求总耗时覆盖解析、路由、等待、重试、上游输出及现有用量统计写入。首字时间取首次上游正文事件的本机观测时刻，包含首字前的重试/等待，不把角色、网络首包或单独的思考事件算正文；它不是客户端接收确认。完成流的正文区间与明确可分离的正文 Token 支持 TPS；非流式、纯思考、工具/非文本输出、缺失 reasoning 用量或中断区间保持未知。具体字段口径见[列文档](log-columns.md)。

一次尝试缺失 usage 时，根请求输入/输出/总 Token 均未知，详情仍显示各次确认值；不会把已知小计当完整总量。明确报告的零与未报告分开。现有累计用量继续独立写入 state.db，日志到期不删除累计用量。

日志不记录请求/响应正文、提示词、思考原文、工具参数、认证头、密钥或任意上游错误文本。probe 只记录固定错误码及 `body_bytes`、`json_valid`、校验通过等有限结构事实。2026-10-13 当前源码（未发布）在失败详情的 `evidence` 中增加 `validation_reason`：`invalid_json`、`missing_response_id`、`upstream_error`、`unsupported_content`、`unsupported_output`、`invalid_tool_call`、`invalid_choices`、`refusal`，未知错误固定为 `invalid_structure`；不直接存储解析错误字符串，其中可能含任意上游字段名。已有 evidence 字典契约、查询/复制/导出沿用，列表仍省略 evidence，历史记录不补写。文本项的 `logprobs` 和 `annotations` 只用于测试结构校验，不进入日志。系统事件使用代码消息与属性白名单。复制和导出来自这些安全结构化字段，不触发新的上游请求。未知 reasoning effort 不作为任意字符串写入日志。

2026-10-13 当前源码（未发布）修正完成响应的取消误判：协议正常结束且响应写入成功后，即使客户端在日志／用量收尾前断开，也保留 succeeded/complete；尚未完成或写入失败时的客户端取消仍保存 cancelled。已经写出 HTTP 200 数据的失败请求保存 partial；没有写出数据则 none。success/complete 只表示本机写入没有报错，不代表最终用户确认收到。历史记录不自动重判；TPS 是否可计算仍由正文区间及可确认的正文 Token 决定，与结果标签独立。旧 pending 在核心重新打开数据库时改为 interrupted/core_interrupted，交付结果 unknown；NORMAL WAL 在突然断电时可能丢失最近批次，不承诺断电零损失。

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
