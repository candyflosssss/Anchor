# Anchor 首个实施里程碑

- 日期：2026-10-10；桌面与正常停服补充：2026-10-11
- 版本标识：`0.1.0-dev`，不是稳定发布。
- 范围：本机核心 / CLI、供应商第一优先工作台、三协议无状态文本/函数转换及 Codex 全局配置托管/恢复，不等于整体初版完成。
- 总体产品边界继续以 `architecture.md` 为准；本文记录本轮已落地的决策与实际接口。

2026-10-21 源码协议分层（未发布）：请求级 Session／候选级 Attempt 已接入网关；请求校验、工具映射、响应与流式语义归属独立协议层。网关保留调度、网络、重试、HTTP 交付与持久化。原生字段保留、额外工具检查、终止校验和上限变化见[协议翻译层契约](protocol-translation.md)。无新增命令或配置，不更新运行版本。

## 1. 已落地的决策

| 项目 | 决策 | 理由与限制 |
| --- | --- | --- |
| 核心与 CLI | Go；模块最低 Go 1.25.0 | 单一无头进程和类型化管理客户端；当前机器使用 Go 1.26.0 验证 |
| HTTP | 标准库 `net/http` | 暂不引入 Web 框架；无 Web 页面、无任意 CORS |
| 存储 | SQLite + `modernc.org/sqlite v1.53.0` | 无 CGO 驱动，依赖锁在 go.mod/go.sum；后续 schema 必须显式迁移 |
| 默认目录 | `os.UserConfigDir()/anchor`，可用 `--data-dir` 覆盖 | 持久状态、当前用户引导凭据与本机 Context 存在这个目录；后续远程控制端仍不复制远端供应商数据 |
| 默认端口 | 模型 `127.0.0.1:17860`，管理 `127.0.0.1:17861` | 初始化可覆盖，仅允许数值回环地址与 1..65535 端口；已初始化实例不会被再次 init 改写 |
| 凭据保护 | Linux 当前用户目录 0700 / 文件 0600；Windows 当前用户受保护 DACL | **不是静态加密**。供应商秘密暂存在受保护的 SQLite 中；本机拥有者令牌在 owner.token 中；普通 access 授权只保存 SHA-256 摘要；Binding helper 凭据另在受保护 SQLite 中保存秘密与摘要 |
| 运行方式 | CLI 前台 `serve`；新增开发桌面宿主协调独立核心 | serve 仍不自动后台化；GUI 关窗不停止其独立核心；不注册系统服务／自启动；见[桌面运行契约](desktop-runtime.md) |
| CLI 输出 | 正常输出 JSON（agent token 除外）；日志 / 诊断 stderr | `access grant --show-once` 和本机 `agent token` 是含秘密 stdout 例外；供应商秘密只从 stdin 输入 |
| 协议支持 | Responses / Chat Completions / Anthropic Messages | 三入口及文本/函数/SSE/用量转换；上游协议按 Key 的实际模型覆盖 → Key 默认值解析；不支持的内容明确报错，不宣称 CC / DSH 或所有 Codex 能力已兼容 |

Linux / Windows 分别使用内核文件锁；进程退出释放锁，不根据残留锁文件猜测是否运行。
数据库使用 WAL、FULL 同步、busy timeout 和 IMMEDIATE 写事务；写配置要求当前 revision，旧版本拒绝覆盖。
目录和受管理状态文件拒绝已知符号链接，Linux 拒绝非当前用户所有的状态目录 / 文件。该保护不承诺防住具有同等用户权限的恶意进程。

## 2. 实际 CLI

完整、最新的命令与参数以 [CLI 指令大全](../cli/commands.md) 为准；变化见 [指令变更历史](../cli/changes.md)。本节只保留开发流程示例，不作为第二份完整目录。

开发入口是仓库根目录下的 `scripts/anchor-dev.sh`。当前机器自动使用 resource-guard 限制编译 / 运行资源，并使用磁盘临时目录。`go run` 的临时可执行文件不是分发产物；便携预发行包使用独立 [发行流程](../release/README.md)。

```bash
./scripts/anchor-dev.sh init
./scripts/anchor-dev.sh serve
```

在另一个终端执行：

```bash
./scripts/anchor-dev.sh context add local --local
./scripts/anchor-dev.sh context use local
./scripts/anchor-dev.sh status --json

# PROVIDER_KEY 为用户在 shell 中安全提供的变量，不把密钥填入命令参数。
printf '%s' "$PROVIDER_KEY" | ./scripts/anchor-dev.sh provider add \
  --id primary --base-url https://api.example.com/v1 --secret-stdin

./scripts/anchor-dev.sh model add --id anchor-code \
  --candidate primary:upstream-model --tools --streaming
./scripts/anchor-dev.sh provider list
./scripts/anchor-dev.sh model list
```

`api.example.com` 和 `upstream-model` 是占位示例，必须替换为供应商的实际 API 根地址与模型 ID。Base URL 不包含推理端点；代理保留该 URL 的路径前缀，按协议追加 `/responses`、`/chat/completions` 或 `/messages`。拒绝 URL 中的 userinfo、查询参数和 fragment；上游要求 HTTPS，本机服务允许 HTTP（精确主机名 `localhost`，大小写不敏感，或数值回环 IP）。直接连接 `localhost` 时不查询 DNS／hosts，依次尝试 `127.0.0.1` 和 `::1`，保留 URL 路径与 Host；显式代理仍按原配置路由，失败不回退直连。此处 localhost 指核心所在机器，不一定是桌面窗口所在机器。

更新供应商 / 模型使用 `put --revision N`，通过 list 获取当前 revision；其余字段是完整替换，不是隐式合并。更新供应商时不提供 `--secret-stdin` 则保留旧秘密，显式输入空值则清空。供应商删除/改名/复制、模型删除及多 Key 工作台已提供，见最新 CLI 大全 §5；删除供应商／Key（包括清除 default 凭据）自动清理对应模型来源，保留其他 Key／供应商的同名模型，最后一个来源的已启用 Binding 默认模型保护仍有效。

可通过下面的命令显式签发测试网关令牌；stdout 此时含秘密，不应进入日志或公共记录：

```bash
./scripts/anchor-dev.sh access grant \
  --label gateway-test --scope gateway --ttl 24h --show-once
./scripts/anchor-dev.sh access list
./scripts/anchor-dev.sh access revoke ACCESS_ID
```

`manage` 与 `gateway` scope 不互通，拥有者引导令牌只授权管理面。普通 access grant 授权无 Binding；托管阶段已另加 Binding 级凭据，按 Binding 的 single / 动态 all-compatible 模型范围访问，网关列表与配置模型目录使用同一范围规则。
Context 当前只支持 `--local`，创建时必须连通并验证实例身份，后续每次管理连接再次验证身份与 API 主版本。缺少 / 不可达 Context 不回退到其他目标。`init` / `serve` / `access` / Context 编辑只作用于本机；业务操作通过类型化客户端调用管理 API。

## 3. 实际 HTTP 契约

全部路由要求对应 scope 的认证；管理面使用 Bearer，网关也接受 x-api-key；原管理与 Agent 网关仅接受数值回环 Host，不接受浏览器 Origin；不开放公开管理或任意 CORS。当前源码另提供默认关闭的独立[应用 API 服务](api-service.md)：可设置端口及私有 IPv4 LAN 准入，完整鉴权和无 Origin 规则继续生效，管理面不对 LAN 开放。

| 接口 | 状态 |
| --- | --- |
| `GET /control/v1/instance` | 实例 ID、API 主版本、程序版本、平台、目标用户、地址、进程 ID（process_id）与能力 |
| `GET /control/v1/status` | 核心与网关 starting / ready / stopping；供应商健康为 per_key（详见工作台，不是总健康结论），托管为 ready/partial，并含逐 Binding 状态与实际连接 unknown |
| `POST /control/v1/shutdown` | 管理鉴权与实例身份校验，严格对象 `{}` 为普通条件恢复；`{"for_restart":true}` 为保留托管配置交接，需 `restart_handoff` 能力，返回 `{accepted:true,bindings_preserved:true}`；未知字段／非布尔值拒绝；接受不代表已退出或新核心 ready，无生命周期503 |
| `GET /control/v1/providers` | 脱敏供应商目录与 revision |
| `PUT /control/v1/providers/{id}` | `{provider, expected_revision, secret?}`；secret 缺失 / null 保留旧值 |
| `GET /control/v1/models` | 公开模型、能力与有序候选映射 |
| `PUT /control/v1/models/{id}` | `{model, expected_revision}`，拒绝不存在的候选供应商 |
| `GET /v1/models` | 已配置的公开模型目录；不代表真实供应商可用性 |
| `POST /v1/responses`、`POST /v1/chat/completions`、`POST /v1/messages` | 三协议入口，无状态文本、函数工具、SSE 与用量；三协议同协议保持受控原生透传，跨协议受能力校验 |
| `GET /control/v1/providers/{id}/workspace` | Key 状态、模型映射、用量、价格和当前价估算 |
| `POST /control/v1/providers/{id}/action` | 固定动作 DTO，管理授权/实例/revision/必要确认；不开放任意 URL |
| `GET /control/v1/usage` | 持久化的供应商/模型累计用量，包含已删除对象历史 |

写管理 API 要求 `X-Anchor-Instance` 与目标一致。revision=0 只创建，N>0 只更新 revision=N 的资源；保存后 revision 加一。CLI 不自动重试写请求，断线后应查询目录、revision 和脱敏字段核对结果；配置托管已有稳定操作 ID 和恢复日志；通用持久操作查询、按幂等键查找及 SSH 写操作仍未实现。

响应错误采用 `{error:{code,message}}`，常见状态：400 无效 / 未支持字段，401 鉴权失败，403 浏览器请求，404 未配置资源，409 实例或 revision 冲突，413 网关请求过大，429 网关总并发超限，502 上游拒绝 / 传输错误，503 无可用候选或核心 starting/stopping/not_running，504 总超时。

2026-10-20 当前源码（未发布）：核心网关与管理 API 的显式错误统一由 `internal/apierror` 目录管理状态码、错误码与中英文模板；`error.message` 使用 `[Anchor] ` 前缀，例如 `[Anchor] model is outside the binding route scope`。每次请求按 `Accept-Language` 选择英文或简体中文，缺省／不支持的语言回退英文，JSON 错误带 `Content-Language` 与 `Vary: Accept-Language`；HTTP 状态、`error.code` 及原响应结构保持原样。Anchor 生成的上游拒绝／传输失败说明和跨协议流式失败事件也接入目录；透传的上游 SSE 错误保留原文。具体目录、参数、回退、日志和客户端边界见[核心错误管理与多语言](errors.md)。

实例能力包含 `graceful_stop`；2026-10-15 当前未发布源码增加 `restart_handoff`。客户端缺少后者时拒绝维护交接，不发送停服写请求或回退普通停服。真实 Run 未 ready 时拒绝新管理写入（正常停服请求除外）和网关调用；正在 stopping 时拒绝新管理写入／网关调用，读取状态仍可进行。管理客户端用结构化 RequestError 返回受限分类；写入网络错误、5xx、无效／超大回包标记 Unknown，不重放。

## 4. 网关的真实支持边界

- 改写公共模型 ID 为候选上游 ID。Responses 同协议受限透传；Chat/Anthropic 入口和跨协议转换支持无状态文本、函数调用/结果、SSE 与用量。2026-10-20 当前源码新增 Responses → Chat 的思考配置、输出格式、明文思考历史及工具身份映射，以及 Chat → Responses 的思考／自定义工具返回；精确范围见[Chat 转换兼容说明](protocol-chat-bridge.md)。图片、音频、未覆盖的推理块、引用注解、服务端状态或无法表达的字段仍明确拒绝。
- `function`、`custom`、仅含这两类工具的 namespace、`tool_search execution=client` 可同协议透传；工具由客户端执行。Responses → Chat 另支持 function/custom/namespace：custom 转为单个 input 字符串参数的函数，原生 grammar 约束降为提示词指令，返回恢复身份；不支持跨协议的 tool_search。未知 / 服务端工具明确拒绝，不能借此宣称所有 Agent 工具已兼容。
- 默认设置 `store:false`。拒绝 `store:true`、非 null 的 `previous_response_id` / `conversation`、`background:true`；客户端需发送完整 input 历史。未来服务器状态需要供应商亲和与相应 API，不可直接沿备用路由重放。
- `tools`、`streaming` 能力由用户在公开模型上显式声明；声明不等于真实上游探测通过。
- 全局并发上限 16；单供应商并发可配置 1..64；网关请求体上限 32 MiB（包含内嵌图片与完整历史），管理写入上限仍为 128 KiB；超限返回 HTTP 413 `request_too_large` 并提示 32 MiB 上限。此上限按字节计，不是模型 token 上下文限制；Chat/Anthropic 转换后的 Responses 请求也受同一上限保护。
- 单次候选的总超时默认 120 秒，可配置 1..3600 秒；上游响应头等待上限 30 秒，下游每次写入等待上限 15 秒。
- priority 策略（旧模型默认）按用户声明顺序调用；供应商页面新模型默认 round-robin，按候选相对倍率倒数平滑加权轮询；禁用、Key 不可用或并发已满的候选跳过；所有候选不可用时失败，绝不选择未配置默认模型。
- 每个供应商 + Key + 候选上游模型在单个网关请求内通常仅尝试一次；取消同渠道普通额外重试与退避，旧 settings.channel_retries 读取时忽略。2026-10-20 当前源码增加明确密文拒绝后的单次清理重试例外（未打包／未发布），见下文。Codex 原生请求／流重试由核心统一设置控制，默认5，见 [Codex统一重试](codex-retries.md)。
- 可安全转移的连接前失败及明确401/403/408/429/5xx响应尝试其他可用Key或候选；全部渠道不可用返回503 no_available_candidate。Key错误分数按配置窗口累计，401/403加100、429加50、408/网络失败/5xx加25；达到阈值后该Key默认冷却300秒，429 Retry-After 可延长冷却。其他 Key 不受影响，到期直接恢复。显式会话成功后保持渠道；该渠道不可用才 fallback。建立连接后结果不确定、已输出流式数据时不重放。
- DNS/连接拒绝/TLS 等未取得连接的失败允许切换；建立连接后结果未知的超时/传输错误、重定向、无效成功响应不自动切换。显式 HTTP 失败也不能保证上游没有执行或计费。
- 下游已提交任何响应后不重放；断开 / 取消传播到上游。流式失败可能只留下不完整事件序列，客户端应核对终止事件，不以 HTTP 200 判断模型任务完成。
- 不跟随上游重定向；不透传管理 / 网关令牌；上游仅收到供应商密钥。不把任意上游错误正文、认证头或 Location 返回给客户端。
- 管理客户端不使用环境 HTTP 代理，避免回环管理泄漏。供应商上游默认 `direct` 不使用应用代理；显式 `system` 按核心主机环境／Windows 系统设置解析，`proxy` 使用绑定节点，Key 覆盖供应商默认。代理不可用或绕过规则命中时不降级直连；应用直连不能绕过系统 TUN／路由器透明代理。契约、兼容与验证边界见[出站代理](proxy-settings.md)。
- JSON 元数据日志记录请求 ID、公开 / 实际模型、供应商、协议、状态、耗时与传输结果；默认不记录请求 / 响应正文或认证秘密。用量按供应商/公共模型累计持久化，缺失用量标未知；请求数按上游尝试统计。价格与 models.dev 导入用于当前价估算已知部分，不是账单。

<a id="历史加密推理恢复2026-10-20当前源码未打包未发布"></a>

### 历史加密推理恢复（desktop.60已发布）

**发送前兼容修复（2026-10-21，desktop.61 dev 已发布）**：原生 Responses 派发时先移除 input 中 reasoning 条目显式为 null、非字符串或空字符串的 encrypted_content 及该条目的 id，保留摘要、其他字段、非空密文和全部其他历史条目。缺少 encrypted_content 的条目保持原样；不判断非空密文是否有效。该步骤只修改本次派发副本，不修改客户端历史，无需等待上游报错、无额外重试、不消耗恢复预算，也不记录为报错后的清理恢复。此前仅识别明确密文拒绝，不能覆盖 `encrypted reasoning content must be a string` 这类类型错误。详细边界见[协议翻译层](protocol-translation.md)。

**后续诊断与针对性修复（desktop.60已发布并配套更新本机）**：用户确认.59仍报同错。临时探针识别首个未知事件为`codex.response.metadata`，类型SHA256为`4928dcffeb7ec6134a0662ac1a91cbdbe737af8d60c407fc34e0245b540c3c28`，并核对[官方Codex SSE处理](https://github.com/openai/codex/blob/main/codex-rs/codex-api/src/sse/responses.rs)，其视为附加元信息而不产生输出。现仅将该确切类型加入可暂存列表，未知事件仍退出，不采用“所有未知都跳过”。新增类型哈希与固定字段名存在性诊断，不保存未知原名／字段值；临时探针2曾仅采集80字符以内小写字母／点／下划线格式的协议名用于确认，最终源码移除此临时原名采集。明确错误中的传输包装前缀不再妨碍识别。用户在修正探针.3安装后确认原失败会话已恢复正常。当前源码将检查前缀上限由64 KiB提升至2 MiB（与既有SSE事件上限一致），覆盖较大instructions/tools初始化信息；仍共享原请求期限与一次清理预算。

**后续修复（desktop.59 已发布并更新本机）**：`.58` 仍把空 reasoning/message 容器当作已输出而退出检查；23:01及23:25失败日志均为 non_lifecycle_event、单次尝试，有思考结构观察但无正文首字，旧日志不能区分空壳与实际思考内容。本轮允许严格验证的空 reasoning/message 条目、空 summary/text part 继续等待明确拒绝，保持64 KiB及原请求期限；未知字段、工具项、真实思考／正文／密文输出仍退出。`response.failed` 内仅含空壳 output 也可识别；SSE仅event行提供类型时同样识别。新增安全事件类别日志用于补齐诊断，不把本地模拟视作真实失败会话已恢复。

仅入站和实际 Key 协议均为 Responses、请求 input 带历史 reasoning.encrypted_content 时启用。原请求照常发送；HTTP 400/422 的 JSON 错误、HTTP 200 JSON 错误或流开头的 `error` / `response.failed` 明确包含 `invalid_encrypted_content`、`The encrypted content … could not be verified` 或推理密文 `was not issued to this caller` 时，移除本次请求 input 中全部 reasoning 条目，在同一供应商、Key、上游模型与选定基址上重试一次。所有候选合计只允许一次清理；共享原候选超时与并发名额，不增加退避。普通消息、函数调用及结果、compaction 与 agent_message（含加密任务）保持原样；不修改客户端历史、Agent 配置或请求文件。本轮不恢复压缩密文和加密工具结果；若错误来自这些内容，清理推理也可能无法恢复。

流式恢复只在提交下游响应前进行：最多检查 2 MiB，沿用原候选请求超时，暂存心跳、codex.response.metadata附加元信息、created/in_progress/queued生命周期事件，以及严格检查无生成内容的reasoning/message空壳和空summary/text part；真实推理／正文／密文输出、任何工具项、含实际输出的生命周期事件、未知字段／无效事件或超限即原样交付并退出恢复窗口。超时／取消按原请求生命周期结束，不清理重试；取消 desktop.57 的独立 2 秒检查截止，防止在慢返回的明确拒绝到达前过早提交下游。检查退出原因写入安全枚举 `encrypted_context_stream_inspection`，最后检查的事件类别写入 `encrypted_context_stream_event`，即使没有清理也可诊断。未匹配错误、EOF、网络／超时、鉴权／限流及已提交的流不触发清理。两次实际派发各保存独立 attempt、用量；根 Token／参考费用沿用缺失任一次用量即未知的规则。Key 健康取清理重试后的最终渠道结果，避免一次已修复的上下文拒绝额外惩罚 Key。日志标记和诊断边界见[日志设计](logging.md#历史加密推理恢复2026-10-20当前源码未发布)。这是一种有损容错，可能失去隐藏推理连续性，不保证所有上游接受剩余上下文。

## 5. 生命周期与验证

两监听面全部绑定成功后才发布 ready；第二个端口失败会关闭第一个监听器。CLI 管理请求或桌面窗口结束不会停止核心；Linux Ctrl+C / SIGTERM、Windows Ctrl+C 或带批准的 `stop --approve` 可触发停服流程。停服同时排空两面，最多等待 10 秒，然后取消剩余请求并关闭连接。

**2026-10-15 未发布修复**：用户普通停服仍先条件恢复。维护 `stop --approve --for-restart`／桌面安装更新则显式选择保留交接，不恢复 Agent 文件、目录、原始日志或轮换 Binding token；新核心使用相同数据目录和地址，核验原日志后继续托管，最终普通停服仍能恢复真正的接管前配置。首次接受的停服意图固定，信号／监听错误与交接请求竞争时不得事后切换模式。只要替代核心未 ready，仍存在不可用窗口；不保证在途请求无损、原生安装成功或已有 Codex 会话自动重载。

本轮自动测试使用临时 SQLite 与回环模拟上游，没有读取 / 修改现有 Agent 配置，没有真实付费供应商请求。

执行后的实际检查结果记录在本次 `docs/updates/` 记录中。Windows 编译检查不等于 Windows 真机的 ACL、锁和生命周期验收。

Linux 源码入口烟测由 `scripts/smoke_test.py` 提供：使用独立临时状态、真实 CLI 子进程和本机模拟上游，验证初始化、配置、模型请求、管理客户端退出不停止核心、SIGTERM 停服及端口释放。它不会留下常驻服务。当前机器通过资源保护脚本运行：

```bash
/home/mosaki/.codex/skills/resource-guard/scripts/resource_guard.sh run \
  --expected-mib 2048 --max-mib 3072 --swap-max-mib 256 \
  --label anchor-smoke -- python3 scripts/smoke_test.py
/home/mosaki/.codex/skills/resource-guard/scripts/resource_guard.sh status
```

Go 测试与静态检查也通过同一保护入口执行，设置 `GOMAXPROCS=2`、`-p 1`，不并发运行多套测试。Windows 编译检查使用 `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -exec=/usr/bin/true`，其中 true 只代替执行，不代表 Windows 测试运行通过。

## 6. 明确尚未完成

1. SSH Connector、远程独立凭据、远端实例绑定与隧道回收。
2. Codex profile、升级后的 helper 路径更新、全工具实测与 Windows 原生接入；Binding 级鉴权与全局自动接管已提供。
3. 更多适配器、JSON/YAML 声明式描述、通用多文件补偿；Codex 配置+自有 JSON 目录的补偿、字段所有权、持久恢复日志、条件恢复与 recover 已提供。
4. 全量跨协议能力、CC / DSH 配置托管与声明式适配器。
5. 桌面双平台原生窗口／安装验收、GUI 公开发行、系统服务、自启动和自动升级迁移。CLI 的公开版本与实际验收以仓库 README 为准；桌面内部构建见[桌面开发版说明](../release/desktop.md)，两者不能互相替代。
6. 管理日志订阅、日志保留、用量时间筛选和历史计费；累计用量与当前价估算已实现。

下一步继续补 Codex 覆盖与跨平台验收、其他适配器及核心连接契约（SSH / Context）。不能把当前里程碑对外称为完成 A01–A14。


## 7. 托管层与 Codex 配置闭环

### 已落地

- `internal/managed`：Adapter 的 Discover/Inspect/Plan/Verify 契约；Editor 字段读写契约；引擎独占配置写入与恢复。当前仅注册 Codex/TOML 基础配置与专有 JSON 模型目录能力，不是动态插件/脚本执行运行时。
- SQLite schema 1 → 2 显式增加 bindings，不旋转实例/拥有者/供应商凭据；旧二进制会拒绝新版 schema，不能直接回退旧程序。升级前应在核心停止时自行保存完整受保护状态，勿公开备份。
- Binding 固定已批准 config root、适配器版本、默认模型、模型授权范围、Codex 元数据精确版本、helper 路径、enabled 与 revision。v1 用整个规范化根保守拒绝重叠，禁用不释放目标所有权；Windows 比较根时忽略大小写。
- 网关只认识 Binding 授权与模型范围，不读 Agent 配置。独立随机凭据存在受保护 SQLite 中，管理查询/配置计划/日志不返回秘密；Codex 配置只写稳定二进制 helper 命令，运行时取 token。禁用后 helper 与网关鉴权均失效。
- 引擎先将原文件（可能含原有秘密）、字段前/后值、接管适配器版本和操作 ID 同步持久化到 `DATA_DIR/recovery/ID.json`，有目录时同时记录其精确内容/路径，先原子创建目录，再比较配置 fingerprint 并原子替换配置指针；公开计划 fingerprint 还覆盖目录和模型策略。Linux 保留原文件模式，Windows 保留原 DACL 并使用 MoveFileEx；新文件限制到当前用户。目录/配置拒绝链接与 reparse，Linux 另拒绝组/其他用户可写或硬链接文件。
- 原值/第三方值的处理按字段比较：等于写入值才恢复，等于原值视为完成，第三值保留并报告冲突。恢复可先完成无冲突字段；provider/模型目录被基础配置或独立 `*.config.toml` 引用时保留并报告冲突；专有目录已被用户修改也保留，无法安全检查引用时中止删除。备份不用于无条件整文件覆盖。由 Anchor 创建的空配置可保留，而非冒险删除。
- 根级字符串行和带标记的专有 provider 表局部编辑，保留无关字节；编辑后校验整个 TOML 语义。受管理键的行内注释/排版可能规范化，复杂/引用/多行等不可安全编辑的受管理表达形式拒绝，不重新序列化用户整文件。
- 正常停止冻结托管写入并条件恢复，再排空两面。重新启动沿用原已接管日志，不把 Anchor 值重新当成原值；prepared 记录先按条件恢复再接管。applied 记录先核验当前配置支持范围及期望目录，过期且没有外部覆盖时条件恢复并重建；不支持配置和外部覆盖不自动重接管。独立 recover 取得实例锁，不依赖运行核心、Agent 或 HTTP。

### Codex v1 边界与证据

当前写入根级 model/model_provider/web_search=disabled、可选 model_catalog_json 和独立 model_providers.anchor_<binding> 表，采用 Responses HTTP、关闭 provider WebSocket；请求与流重试采用统一次数（默认5）。认证配置依官方 command-backed auth 契约：command/args、timeout_ms、refresh_interval_ms，helper stdout 是原始 token。官方依据：<https://developers.openai.com/codex/config-advanced>。

仅全局 CODEX_HOME；用户选择的 profile 与显式 WebSocket 配置拒绝，未探测安装版本、系统策略、环境/命令行/项目覆盖，inspect 始终区分 configuration_only 与实际连接 unknown。Anchor 用操作者导入的、与精确 Codex 版本匹配的完整元数据生成 `anchor_ID.models.json`：复制指令/上下文/工具/reasoning，slug 改为公开别名、展示名可覆盖，visibility=list/supported_in_api=true、upgrade=null、prefer_websockets/use_responses_lite=false。不猜能力、不自动下载、不自动探测版本；操作者必须核对模板和真实供应商能力。自定义目录替代而非合并 Codex 自带目录。single 且无元数据的旧方式仍要求 Codex 自行识别别名；不更改登录状态、审批或沙箱策略。

新 CLI 绑定默认 all-compatible；旧省略 scope 的记录仍为单模型。兼容范围动态要求 tools/streaming、精确版本有效元数据和至少一个 enabled 候选供应商；默认模型仅初始选择。网关 `/v1/models` 和请求授权复用规则，未知/非兼容模型拒绝，Binding token 不可访问管理 API。`agent set-models` 显式更新 default/scope/版本，先安全恢复、后递增 revision，凭据不旋转，留待重新 plan/apply。目录是接管快照，模型/供应商变更影响实时授权但不偷偷改目录；运行中刷新可用 agent sync，或 restore → plan → apply、正常停服/启动，然后重启 Codex app-server。

桌面 SSH 主路径在远端 CODEX_HOME 的**基础配置**设置 Anchor provider + 默认模型 + 目录，无需 profile。实际 Codex 的模型选择/显式 model 参数可切换目录内公开模型，经 Anchor 路由各供应商。它不等于 Anchor 自己实现了 SSH Connector，也不是拦截全部网络出口；独立 profile/更高优先级覆盖不会被冒充成受控。

`scripts/agent_smoke_test.py` 在受限磁盘临时目录构建一次性测试二进制（非发行包），验证真实 CLI、认证 helper、流式 custom 工具载荷透传、正常停止、重启、SIGKILL 后独立恢复与第三值冲突。可选 `--codex-binary` 使用全新 HOME/CODEX_HOME、无继承密钥环境与纯回环模拟上游；2026-10-10 已在 **codex-cli 0.159.3** 验证版本匹配的官方 gpt-5.5 元数据映射为两个模拟别名：CLI 默认/`--model` 请求成功，实际 app-server JSON-RPC `model/list` 返回两个别名，`thread/start` + `turn/start` 的默认/显式模型均完成回环模拟 Responses 请求。`set-models`、认证 helper、正常停止、重启、崩溃恢复和冲突均使用隔离状态。为此支持客户端 function/custom、其 namespace 和 `tool_search execution=client` 透传，namespace 中的 hosted/nested 非兼容工具以及 server tool_search 仍拒绝。它不等于真实付费上游、Codex 全工具调用闭环、桌面模型选择器 UI 或 Windows 原生验收通过。

```bash
/home/mosaki/.codex/skills/resource-guard/scripts/resource_guard.sh run \
  --expected-mib 2048 --max-mib 3072 --swap-max-mib 256 \
  --label anchor-agent-smoke -- python3 scripts/agent_smoke_test.py
/home/mosaki/.codex/skills/resource-guard/scripts/resource_guard.sh status
```

没有 Binding 删除/路径更新、代码插件、声明式 JSON/YAML、多文件原子提交、通用操作幂等键或独立日志订阅。内核实例/引擎锁只协调 Anchor；外部编辑器的最后检查与 rename 之间仍有竞争窗口，不宣称防御具有同等用户权限的恶意进程。旧会话可能保留网关地址，停服恢复后需要重启 Agent。


## 8. 桌面供应商开发切片（2026-10-11）

已新增 Wails v2.15.0 宿主、受限桥接、Go DTO→TypeScript 生成与真实管理客户端接入。宿主自动安全初始化／启动／复用本机核心，独立子进程不随窗口 context 退出；供应商界面只在身份／API／授权／ready 通过后开放。状态轮询不重启核心，断开后清除供应商缓存与草稿。供应商密钥只入不出；写入仍使用核心完整替换与 revision 规则。

接口细节见[桌面运行契约](desktop-runtime.md)和[供应商接口](provider-interface.md)，构建与平台缺口见[桌面开发版说明](../release/desktop.md)。真实 Go 隔离核心测试不等于原生 Wails 窗口已验证；后续安装包轮补齐 Windows NSIS 与 Ubuntu24.04 AMD64 `.deb` 的隔离构建入口，仅执行编译／打包，按用户要求不做本轮安装或原生验收，未对外发布。

### desktop.10：只读上游目录

管理面新增 `POST /control/v1/providers/{id}/discover-models`，复用身份／管理鉴权、已存 revision、禁止代理与重定向的 HTTP 客户端，以及网关／供应商并发边界。CLI 和受限桌面方法复用同一后端，最多8秒获取 OpenAI 风格 `/models`，限制响应大小和模型 ID，并固定错误脱敏；不会生成内容、导入映射或更新健康。DTO／失败语义见[供应商接口 §4](provider-interface.md)，实际指令见[最新大全](../cli/commands.md)。


### desktop.11：第一优先六项

供应商工作台已接入删除/复制/改名、模型映射与删除、显式最小推理测试、独立 Key 健康与轮询/优先策略、三协议转换、持久用量及价格估算。详细支持边界见[实现契约](provider-priority-one.md)，入口见[CLI 大全](../cli/commands.md)。schema2 自动升级3，旧单 secret 迁移 default Key，命名 Key、策略、价格与用量独立存储。改名迁移配置引用但历史用量保持旧 ID；删除不抹掉统计。

本轮仅定向检查和打包，不运行全量/race/浏览器或原生安装验收。当前价估算不包含未知用量；models.dev 只做解析样例验证，未请求线上目录；协议仅回环模拟上游，未调用真实付费服务。具体产物与提交见 release 任务记录。

### 本地 Agent 发现与桌面托管（2026-10-12 实现，desktop.21 dev 已发布）

`internal/managed` 提供有界只读发现和 workspace/enroll/sync 应用服务，`internal/core` 提供已鉴权的 `/control/v1/agents/workspace`、`/enroll` 和 action=sync。已知用户目录识别产品痕迹，仅 Codex 可托管；不执行客户端或自动推理。Codex 从本地完整 models_cache.json 提取 client_version 和 ModelInfo，操作者核对版本及能力后批准，模板及排除选择存 Binding，而非修改共享供应商模型。目标本地策略也用于网关鉴权，single 与 all-compatible 共用元数据解析。限制和完整操作见 CLI 大全 §8。

桌面受限 Bridge 复用同一 client/core；前端 feature agents 按 Model、hook Controller、View 分层，不拥有文件写入或业务路由。唯一写入者仍是托管 Engine；GUI 支持识别、批准登记、同步、启停及失败后重读。宿主在 GUI 初始化前只放行精确的 `--data-dir DIR agent token ID` 内部 helper，复用已有 CLI stdout 凭据契约，不暴露任意命令执行。

## 平台识别持久缓存（2026-10-21，当前源码未发布）

2026-10-21 平台识别增量（未发布）：供应商首次创建自动识别并将成功／失败写入独立观察字段，旧供应商首次账户／目录倍率查询补齐；后续查询复用缓存，手动 detect-platform／桌面「重新探测」才更新。复用既有平台表 JSON，无 schema 迁移；不改 provider revision，不因自动识别清理用户授权。后台倍率维护只消费已保存平台，不负责探测重试。详情、并发与升级边界见[供应商接口](provider-interface.md#自动探测与供应商列表余额2026-10-13当前源码未发布)。

## models.dev 参考目录基础（2026-10-13，当前源码）

新增 `internal/modeldev`，共享1小时只读目录缓存、能力／限制／原始思考选项及文本 token 费用纯函数；管理面 `POST /control/v1/model-catalog/query` 与 `/estimate` 沿用管理鉴权和实例身份，CLI／桌面复用类型化客户端。旧基础价导入改用同一缓存。没有数据库迁移，不自动修改配置，也未接入映射默认填充或日志逐请求比价。契约、价格口径与缺口见[模型参考目录](model-catalog.md)。

## 日志区间浏览与错误汇总（2026-10-21，当前源码未发布）

既有日志查询／导出支持可选 `errors_only`，只匹配失败及中断请求；`LogStats` 通过可选 `include_errors` 在同一只读快照中统计按错误码、阶段及 HTTP 分组的错误次数，最多返回100类，总数不截断。沿用 owner 与实例鉴权，不新增上游调用、数据库迁移或原始错误正文采集。桌面图表选区、请求列表、候选和导出共享固定半开区间，CLI 提供 `--from/--to`、`--errors-only`、`--include-errors`；契约与兼容边界见[日志设计](logging.md#区间浏览与错误汇总2026-10-21当前源码未打包未发布)。

## 日志筛选候选（2026-10-16，desktop.43 dev 已发布）

`internal/logbook` 提供基于保留日志标识列的去重、计数与字面前缀候选查询，管理面 `POST /control/v1/logs/suggestions` 验证 owner 与目标实例身份，核心3秒超时；CLI `logs suggestions` 与 Wails `LogSuggestions` 复用类型化客户端。查询覆盖所选时间范围全部保留记录，不依赖前端页码／过滤；默认20、最多100个候选，范围最多90天，不扫描正文或上游。固定字段白名单和参数化值保护数据库边界，无 schema 迁移或上游调用；旧核心不支持时桌面只降级为手动筛选。字段、排序与计数语义见[日志设计](logging.md#10-筛选与搜索候选2026-10-16desktop43-dev-已发布)。


## 日志归属与分类（2026-10-16，desktop.44 dev 已发布）

网关在鉴权通过后从请求上下文 Binding 采集 Agent 类型与绑定 ID，仅保存安全标识，不读取 Agent 配置。项目／会话只接受显式 `X-Anchor-Project` / `X-Anchor-Session`，严格限制单值、字符与长度，排除当前鉴权凭据；没有自动项目／会话识别或历史归属回填。归属字段保存在既有日志 JSON，四个幂等表达式索引支持过滤／候选，不改变 user_version。日志查询与统计共用精确过滤，缺省表示所有、空字符串表示未识别；统计每组共用原始累加口径，最多返回100组但总量覆盖完整范围。管理鉴权、实例保护和受限查询时长沿用，不把这些标签用于授权或转发上游。CLI、桌面桥接、类型生成与共享样例同步；完整隐私、接口和兼容边界见[日志设计](logging.md#11-归属与分类统计2026-10-16desktop44-dev-已发布)。

### 2026-10-19 请求日志参考费用（当前源码，未打包）

请求日志复用 `modeldev.Estimate`，优先取映射保存的参考快照、否则仅精确匹配已有有效目录缓存，不在代理／推理测试中发起价格网络查询。逐次尝试按缓存、思考及输入阶梯估算并合计，未知尝试使总价未知，无价格省略费用快照。`logs list/detail/follow/export` 与桌面共享可选 `reference_cost`；详情和导出保留每次尝试的 `reference_usd`。价格快照持久保存，不回填旧日志、不代表真实账单；具体字段与边界见[日志设计](logging.md)。

### 2026-10-19 日志费用分区与倍率（后续源码，未打包）

在原始参考费用上新增请求时供应商/Key 分组倍率快照和最终参考费用，逐次计算再汇总；新增共享 CostAdjustment/CostPart DTO、列表可用的乘区汇总及详情逐次快照。原始 usd 语义不变，未知与零分开，旧记录不补算，超预算时截断尝试明细而不丢汇总。前端最初拆为四列，后续展示修正将原始费用、供应商倍率、分组倍率和最终费用合为一个“参考费用”块，并保留数据型悬浮/聚焦明细；列表、请求详情和尝试时间线复用摘要。列偏好 v3 合并 v1/v2 旧费用列并保留隐藏选择，不改变 JSON、采集与计费口径。未新增配置或命令，logs JSON 同步新增字段；口径见[日志设计](logging.md)。


### Key 错误分数与冷却（2026-10-21，当前源码未发布）

核心以带 revision 的实例级设置统一 CLI、管理 API 和桌面设置。按不同错误分数与有效时间窗累计，达到阈值后只冷却对应 Key；默认阈值100、冷却300秒、错误窗口300秒。429 Retry-After 可延长冷却；冷却到期直接恢复。手动停用和并发门禁保留。用户拖动保存渠道优先级；显式会话成功后保持渠道直到渠道不可用。连接前失败及明确上游失败可 fallback，连接后不确定结果和已开始输出的流不重放；全部不可用返回503。接口及生命周期见[供应商接口](provider-interface.md#key-错误分数与冷却设置)。

## 2026-10-22 Claude CLI 与 Desktop 支持（当前源码，未打包）

CLI 的 claude 适配器增加完整映射 modelPicker（需要 Claude Code 2.1.242+）；新增独立 claude-desktop，本地配置库已应用 UUID 固定到 Binding，接入/同步/停用恢复共用现有引擎，策略及远程配置拒绝。桌面七平台列表与首次范围确认已接入；不修改真实客户端配置。原生 Messages 保留版本和重复 beta 请求头，跨协议不携带，供应商凭据仍独立替换。支持范围、官方资料与未实现项目见[接入契约](agent-management-layer.md#2026-10-22-claude-clidesktop-独立接入当前源码未打包)。实际测试结果见本轮 docs/updates 记录；本轮不生成发行包，不声明真实 Claude/Windows 原生窗口/WSL/SSH/付费上游验收。
