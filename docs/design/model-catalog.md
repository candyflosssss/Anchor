# models.dev 模型参考目录契约

状态：2026-10-13 实现，随 desktop.29 dev 已交付。已实现共享目录、核心管理 API、Go 客户端、CLI、桌面桥接及前端 Model 方法。2026-10-14 源码新增映射设置、显式目录默认值导入与参考价格快照保存，未打包。日志逐请求参考价／实际费用对比仍待接入。

## 数据与身份

固定来源 `https://models.dev/api.json`。这是社区维护的参考目录；官方供应商文档及实际请求账单仍需单独核对。模型按 `(provider_id, model_id)` 精确选择；供应商 ID 属于 models.dev，与 Anchor 本机供应商 ID 无自动对应关系。`canonical_model_id` 保留原始声明，不按模型名前缀猜测供应商。2026-10-15 源码新增创建映射的精确自动解析：显式 `provider_id/model_id` 优先，其次全目录唯一的精确上游 ID；同名多渠道仅在全部声明同一 canonical 身份且唯一 canonical 所属记录存在时消歧，否则保留未设置。多个候选必须解析为同一目录记录，不使用模糊搜索、手工别名或第一个匹配。

`ModelCatalogQuery`：`provider_id/model_id/search` 默认空，`limit` 默认0（模型列表实际取50，最大100），`refresh` 默认false。空 provider 返回供应商列表；只给 provider 返回模型列表，search 做 ID／名称子串过滤；给 provider+model 精确查询一条模型，不能同时 search。provider/model/search 最多128/256/128字节，无控制字符或首尾空白。未知供应商／模型返回 not_found；目录默认排序并给出 `total/truncated`。

返回 `ModelCatalog`：`source=models.dev`、`fetched_at/expires_at`（Unix秒）、`version`（响应内容SHA256）、`stale/failure`，以及非空或空数组 `providers/models`。模型 `source_updated_at` 保留源目录 `last_updated`（可能缺失，不等同本次下载时间）。展示名称去除首尾空白；ID 不做修改。每次返回深拷贝，编辑默认值不会改变共享缓存。

模型字段：

- `capabilities`：输入／输出模态数组，视觉从输入 image 推导，attachment、tool_call、reasoning、structured_output、temperature 保留源布尔。未声明布尔为 null；模态未声明时视觉为 null。streaming 不在此目录契约中，不擅自补全。
- `limits`：context、input、output 都可空。源目录的0限制归为未知，正整数保留，不根据 context 反推 input 或 output。
- `reasoning_options`：保留 toggle、effort 和 budget_tokens。effort 可含 null 或 none/minimal/low/medium/high/xhigh/max/default，只保留实际值，不生成统一等级；budget min 可为-1（供应商自动预算）。未声明选项为空数组，即使 reasoning=true 也不猜选项。
- `pricing` 可空；币种 USD、单位 per_million_tokens。input/output/cache_read/cache_write/reasoning/input_audio/output_audio 为可空十进制字符串，null未知、"0"免费。tiers 保留按上下文输入长度分档的精确阈值；存在精确 tiers 时优先于旧 context_over_200k，否则兼容旧200000阈值。未知源字段忽略，已支持字段类型／值无效则拒绝整个快照，保留已有缓存。

models.dev 元数据是用户可核对的默认参考，不能证明某个中转商或 Key 的真实能力、实际可用性和收费倍率；不替代 Codex 的版本匹配完整 ModelInfo 模板。后续映射消费者应让用户明确选择来源和模型，保留人工覆盖，不用新参考值覆盖已保存配置。

## 获取与缓存

`internal/modeldev.Client` 使用独立 HTTP 客户端：GET 固定公开地址、30秒总超时、16MiB响应上限、最多一个该主机连接，不携带 Authorization，不允许重定向或推理探测；公开下载支持标准 HTTPS_PROXY／NO_PROXY 环境配置。解析上限1024供应商／50000模型。数值长度与指数在精确解析前限制，避免小型异常数字导致大整数无限分配。

每个核心维护一个内存快照，TTL为1小时，首次需要时加载，不后台轮询、不写磁盘。并发刷新合并；`refresh=true` 显式刷新。失败有旧快照则返回旧值并标 `stale=true/failure=refresh_failed`，自动调用1分钟后重试；无快照返回 catalog_unavailable，1分钟内自动失败退避。显式刷新可以立即重试。调用取消只停止等待共享抓取；核心退出取消并等待抓取结束，关闭闲置连接。

旧 `provider price-import` 也复用此缓存：只返回基础输入／输出／缓存单价预览，价格更新时间为快照获取时间；不能表示分档及陈旧状态，因此拒绝陈旧快照。用户核对后仍需 price-put，查询和刷新不写配置。

## 文本 token 估算

`CatalogEstimateInput={query,usage}`，query 必须精确指定 provider+model，limit=0。usage 的 input_tokens、output_tokens、cache_read_tokens、cache_write_tokens、reasoning_tokens 均可空；有值须为0..9007199254740991。输入总量**包含**缓存读写，输出总量**包含**思考 token。缓存之和不得超过输入，思考不得超过输出。供应商适配器须先规范化原始用量，尤其 Anthropic 等可能单独提供缓存计数；不能直接将各协议原始字段一一复制。

纯函数 `modeldev.Estimate(model,usage)` 不发网络请求，后续日志可以对捕获的价格快照使用此函数。客户端 `Estimate` 方法先查询目录再调用纯函数，不应将它放入代理请求关键路径。

输入估算=(总输入−缓存读−缓存写)×输入价；输出估算=(总输出−已知思考)×输出价；缓存读／写和思考各计一次。思考未单独定价时使用输出价；单独思考价与输出价相同则无需思考计数即可估总额。各分项除以1000000。选择输入总量严格超过阈值的最高分档，不对缓存进行阈值前扣除；分档率是完整档价，缺失字段保持未知，不擅自继承基础价。

金额采用有界有理数计算，结果十进制字符串最多18位小数，舍入极小值可能为0。目录价以最多64字符、指数±308和不大于1000000 USD／百万 token 限制异常数据。

结果 `CatalogEstimate` 携带目录来源／版本／陈旧状态、本次usage、`basis=text_tokens/currency=USD`、`usd` 及分项、`above_tokens` 和 `reason`。价格缺失、输入／输出未观测、缓存计数未观测、不同思考价但计数未观测，以及正数类别缺少单价，均 `usd=null` 并说明原因；已完成的分项可以存在，不代表完整总价。观测为0的类别计0，即使该类别价格缺失。目录已公布的音频单价仍保留供未来消费者；当前估算不纳入音视频／图像专用费、按请求费、平台倍率／折扣／税费或其他币种。

后续日志应保留本次用量口径和捕获的目录版本／价格、估算状态，以及独立的真实账单来源／币种／倍率，不能拿后来刷新价格当历史账单。

## 调用边界

| 层 | 方法／路由 | 行为 |
| --- | --- | --- |
| 核心 | POST /control/v1/model-catalog/query | ModelCatalogQuery → ModelCatalog |
| 核心 | POST /control/v1/model-catalog/estimate | CatalogEstimateInput → CatalogEstimate |
| Go client | ModelCatalog / EstimateModelCost | 类型化只读POST，失败 Unknown=false，不自动重放 |
| 桌面 Bridge | ModelCatalog / EstimateModelCost | 标准 anchor-desktop-v1 Result，复用当前本机实例和管理凭据，45秒方法超时 |
| 前端 Model | modelCatalogService.query / estimate | 校验 DTO／精确模型身份／用量，拷贝并筛选字段；可用 AbortSignal 在调用前／返回后丢弃响应 |
| CLI | catalog providers/models/model/estimate | JSON 只读入口，完整语法与未知计数见[指令大全 §13](../cli/commands.md#13-modelsdev-参考目录与本地费用估算desktop29-dev) |

管理路由仍要求 management scope、实例 X-Anchor-Instance 和受限 Host／Origin，不能由浏览器直接跨源调用。invalid=400、not_found=404、catalog_unavailable=502；对外不泄露远端错误内容。桌面将目录不可用映射 read_error、传输断开 offline；纯读失败无需写入结果未知确认。前端取消只停止消费，不能取消已发到 Wails 的调用；调用取消只停止等待共享下载；核心退出和30秒抓取超时负责终止下载。

DTO 由 Go 生成到 `desktop/contract.generated.ts`，共享离线样例 `desktop/contract/model-catalog.json` 在 Go 与前端验证。默认测试均离线；公开目录兼容检查显式 opt-in，不涉及凭据和付费推理。原生窗口／安装包和后续消费界面不在本轮验收范围。

## 模型映射设置（2026-10-14，源码未发布）

每个公开模型新增可空 `settings`，沿用模型 JSON 存储，不改变数据库 schema。`configuration` 是人工确认后的模型声明，`reference` 是核心从 models.dev 获取的单模型快照，包含版本、抓取/过期时间、模型更新时间、完整 nullable 报价和分档。

设置支持：`limits.context/input/output`；`capabilities` 中的模态、视觉、附件、工具调用、思考、结构化输出与 temperature；`streaming`、`parallel_tool_calls`；`supported_reasoning`；`default_reasoning`；`default_output_tokens` 与 `default_temperature`。未知保持 null/空数组，正整数上限为 JavaScript safe integer。上下文不能小于已声明的输入/输出上限，默认输出不能超过模型最大输出。Temperature 默认值需声明支持，范围 0..2 仅作配置验证，不代表每个上游都接受整个范围。

思考支持范围保留原生 effort/toggle/budget_tokens。默认值分别使用 `value`（可为 JSON null）、`enabled`、`budget_tokens`；其他两项必须为 null。默认值须位于对应已声明范围；预算 -1 只有范围允许时有效。reasoning=false 时不能留下选项或默认值。模态已声明时视觉必须与 image 输入一致。旧映射的 tools/streaming 布尔继续权威，设置读取始终返回现有声明；未知写入保留旧值。已有供应商编辑与 `model put` 同步这两项，同时保留其他设置及参考快照。

受管理凭据、Origin/Host 和实例身份保护的接口：

- `GET /control/v1/models/{id}/settings` 返回 `ModelSettingsState`：model_id、revision、display_name、candidate_count、configuration、reference、defaults_status。
- `POST /control/v1/models/{id}/settings` 接收 `SaveModelSettings`：model_id、expected_revision、display_name、configuration、reference_action、reference_provider_id、reference_model_id。只替换显示名/设置，保留候选顺序、来源 Key、倍率、调度策略和 Codex 指令模板；乐观版本校验阻止覆盖其他操作。
- `reference_action=keep` 保留旧快照，`clear` 清除；两者来源 ID 必须为空。`set` 需精确目录 provider/model ID，由核心抓取当前缓存/刷新后的单模型快照，拒绝陈旧失败快照，前端不能传入报价或伪造来源。客户端与桥接保存方法45秒超时，断线/异常回包仍按写入结果未知处理，无自动重放。

桌面通过 Model → Controller → View 使用这组方法。目录读取和显式刷新仅改变表单草稿：补充未设置值保留人工字段/默认值；替换草稿替换目录包含的声明并清除可能不再适用的默认思考值。保存才持久化。多个候选使用同一份设置，页面提示核对所有候选；单目录导入不能证明所有渠道真实能力。参考价格不是候选实际费用，目录刷新不后台覆盖已保存设置。

Codex 的完整版本匹配目录模板仍是必要前提。生成目录时应用显式 context、text/image 输入模态/视觉、并行工具调用；effort 可以导出 none/minimal/low/medium/high/xhigh，其他或 null effort 会阻止同步，避免伪造/静默降级。非思考模型清空目录中的思考等级和默认值。模板指令与截断策略保留。toggle/budget 思考、输出预算、temperature、结构化输出等保存为能力/默认值元数据，目前不自动改写推理请求。更新目录需 `agent sync`，并重启 Agent；保存映射不热更新正在运行的会话。Agent 自动压缩策略没有添加到通用模型设置。

CLI 使用 `model settings --id ID [--file FILE]`，详见[指令大全](../cli/commands.md)。本轮验证使用隔离状态和模拟目录，没有修改真实 Agent 配置或调用真实付费上游。


### 2026-10-15 创建自动补值与手动应用修复（源码未发布）

共享核心在两条创建入口（model add、provider model-put 创建新别名，含桌面批量导入）尝试精确解析；每个创建操作最多等待 3 秒，共享目录抓取仍由既有 30 秒超时和 Close 生命周期约束。无目录、陈旧、未匹配、歧义、多候选不一致或导入配置无效时保留未设置，不阻止创建；编辑/追加已有映射不重填。候选使用上游 ID，不拿 Anchor 供应商名称当 models.dev provider。手工 tools/streaming 标记仍权威，目录没有声明的默认值不猜测。

手动 `reference_action=set` 在核心捕获参考快照时同时补齐未声明的配置，保留显式值；目录输入/输出超过手工上下文、或输出低于已有默认预算时跳过，避免生成无法保存的配置。前端“补充”采用同一冲突规则；“替换”清除超限输出预算和不受支持的 Temperature 默认值。参考快照包含完整价格、来源与版本；未知和零价区别保留。参考价格**不覆盖**供应商 `prices` 计费表，也不自动改写 Agent 配置。已有映射不会在只读查询时悄悄迁移；自动补全草稿仍须显式保存（见下节）。

### 2026-10-15 已有映射读取自动补全（源码未发布）

`GET /control/v1/models/{id}/settings` 现在返回只读设置草稿。优先使用已保存参考快照（不触网、不更新来源版本），否则共享目录匹配所有上游候选，最多等待 8 秒；空字段以 FillCatalog 补充，手工值、tools/streaming 保留。未匹配、下载失败、目录陈旧或超时不阻止读取，也不修改状态；GET 不落库、不递增 revision、不触发 Agent 同步。参考快照与配置仍需显式 POST 保存。

`defaults_status` 是新增的只读字符串：`saved` 为 POST 保存后的实际设置；`reference` 为按已存快照补值；`matched` 为新匹配；`unmatched` 为没有唯一一致来源（含多个候选冲突）；`unavailable` 为目录失败、陈旧或超时；`incompatible` 为合并结果无法通过现有配置校验。旧桌面数据缺字段时前端按 saved 兼容。新匹配且 reference 非空时，前端保存使用 set + provider/model ID；已存来源保持 keep。clear 仅清除已保存来源，不关闭后续读取的自动建议；新建议仍需确认保存。速度映射保留基础能力补值，但不自动附加标准价格来源。

精确裸 ID 消歧允许使用目录中自声明的唯一 canonical 所属记录；其他同名第三方条目缺少 canonical 不再使匹配失败。任何非空冲突 canonical 仍拒绝匹配，没有所属记录时不根据模型名前缀或本地供应商名猜测。显式 provider/model 仍优先。CLI 查询、输出字段、示例和错误行为同步见[指令大全](../cli/commands.md)。
