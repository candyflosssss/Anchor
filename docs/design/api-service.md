# 应用 API 服务（2026-10-22，当前源码，未打包／未发布）

契约标识 `anchor-api-service-v1`；唯一 Go DTO 源为 `internal/state/api_service.go`、`internal/desktop/contract.go`，TypeScript 由 `go run ./cmd/desktop-contract` 生成。桌面共享初始样例为 `desktop/contract/api-service.json`。

## 监听与接入

新增独立、默认关闭的应用模型监听，默认端口 17862。设置 → API 服务可启停、配置端口和允许局域网；CLI 见[最新指令大全](../cli/commands.md#应用-api-服务与调用-key2026-10-22当前源码未发布)。原本机模型网关 17860、管理面 17861 及已有 Agent 配置保持原用途，初始化的两个回环地址不由此表单修改。

关闭 LAN 时监听 `127.0.0.1:PORT`；开启时监听 IPv4 `0.0.0.0:PORT`。LAN 请求的实际 TCP 来源和数值 Host 必须为回环／私有 IPv4，不信任 Forwarded / X-Forwarded-For；不支持 DNS Host、公网来源或直接浏览器 Origin，不开放任意 CORS。认证和三种模型协议复用核心网关，管理面不挂到此监听。HTTP 不提供传输加密，适合可信局域网；系统防火墙由用户按需放行。

监听设置完整替换，携带 `expected_revision`，保存只持久化，核心重启生效；运行中的鉴权策略始终采用启动快照，不能因保存草稿提前放开访问。核心正常启动时三个监听全部绑定成功才 ready；应用端口冲突会清理已绑定端口并失败，不降级成局部成功。普通停止、保留配置重启均共同排空所有监听，最长沿用10秒。端口为1..65535，禁止与初始化的管理或 Agent 网关端口重复。没有自动修改用户真实运行设置或防火墙。

核心返回 `settings`（enabled/port/allow_lan/revision）、`active_address`、`restart_required`、`base_urls`、`keys`。地址显示当前实际启动快照；待重启时不把已保存值显示为已生效。Base URL 包含 `/v1`，仅枚举实际监听的回环地址和核心设备的私有 IPv4 网卡地址；不把 `0.0.0.0` 当接入地址，也不保证跨设备网络已连通。

提供 `GET /v1/models`、`POST /v1/responses`、`POST /v1/chat/completions`、`POST /v1/messages`。模型名使用公开映射；不增加私有请求协议或改变协议转换规则。新入口同样验证 Bearer / x-api-key，不把调用者凭据转发给上游。

## Key 与管理契约

仅已认证、实例身份匹配的管理连接能调用：

| HTTP | 用途 |
| --- | --- |
| `GET /control/v1/api-service` | 监听状态与普通 gateway 授权元数据；不包含 Binding 秘密或 manage 授权 |
| `POST /control/v1/api-service/settings` | 保存完整监听配置；enabled / allow_lan 必须显式提供 |
| `POST /control/v1/api-service/keys` | label、ttl_seconds → `{access, token}`；仅成功签发时返回完整秘密 |
| `DELETE /control/v1/api-service/keys/{id}` | 仅撤销普通 gateway 授权，返回 `{revoked:true}` |

能力标记 `api_service_v1`，旧核心不支持时客户端在请求前拒绝，桌面提示升级，不回退 Mock 或本机实例。CLI / 桌面通过现有类型化管理客户端和 Context / 连接机制访问；支持选定的本机或 SSH 核心，不以 LAN 接口提供远程管理。

Key 复用 access 表：随机256位秘密，仅保存 SHA-256 摘要；独立 ID、名称、scope=gateway、Unix秒 expires_at、revoked。名称非空、最多128字节且禁止控制字符；ttl_seconds 为0..315360000，0永不过期。桌面默认30天，明确输入0可永久；不持久化完整秘密，不自动写剪贴板。可撤销、过期立即拒绝新请求；不承诺中断已接受请求。Key 可访问全部已配置公开模型，暂不提供每 Key 的模型范围或额度限制。原 `access grant --scope gateway` 签发的 Key 也列入此列表，且可调用两种模型入口。

桌面桥接为 APIService / SaveAPIService / IssueAPIKey / RevokeAPIKey。前端按 Model → hook Controller → View 分层；Key 仅存当前页面内存，关闭显示、离开页面、切换实例或开始下一操作时清除。结果未知不自动重放，阻止继续写入直到重新读取；丢失签发响应时，可从列表找出并撤销后重新签发，无法重新查看秘密。重新读取不能恢复丢失的 Key。

## 调用日志

`access_key` 为验证通过的普通 gateway Access ID，`access_label` 是当时名称快照；不从客户端自报头读取。列表、详情、JSONL导出保留，支持 `--access-key` 精确过滤、`--group-by access_key` 汇总、`suggestions --field access_key`。既有 `key` 是上游供应商 Key，语义不变；Binding 仍用 agent / binding 识别，不混成普通 Access。未知／历史记录空值不补写。

日志页面新增「调用 Key」列、筛选及分类，详情和安全摘要包含名称和ID；上游 Key 独立显示。失败解析请求同样归属已经验证的调用 Key；鉴权失败不猜归属。名称意外包含本次认证秘密时抑制该名称。完整令牌、认证头及正文不写日志。日志 JSON 上新字段建立表达式索引，不升级 state schema。

## 验证边界

使用隔离状态、共享样例与本机模拟 HTTP 上游；验证配置 revision / 持久化、权限隔离、Host／来源／Origin、签发撤销、真实桥接读写与 CLI、监听失败清理及停止释放、日志归属和统计。模拟 LAN 地址测试验证服务器准入逻辑；本机回环监听测试不代表第二台设备和系统防火墙已经验收。原生 Windows / Linux 窗口、真实跨设备 LAN 及付费上游未在本任务验证。
