# Anchor 首个实施里程碑

- 日期：2026-10-10
- 版本标识：`0.1.0-dev`，不是稳定发布。
- 范围：本机核心 / CLI 和 Responses 同协议代理，不等于整体初版完成。
- 总体产品边界继续以 `architecture.md` 为准；本文记录本轮已落地的决策与实际接口。

## 1. 已落地的决策

| 项目 | 决策 | 理由与限制 |
| --- | --- | --- |
| 核心与 CLI | Go；模块最低 Go 1.25.0 | 单一无头进程和类型化管理客户端；当前机器使用 Go 1.26.0 验证 |
| HTTP | 标准库 `net/http` | 暂不引入 Web 框架；无 Web 页面、无任意 CORS |
| 存储 | SQLite + `modernc.org/sqlite v1.53.0` | 无 CGO 驱动，依赖锁在 go.mod/go.sum；后续 schema 必须显式迁移 |
| 默认目录 | `os.UserConfigDir()/anchor`，可用 `--data-dir` 覆盖 | 持久状态、当前用户引导凭据与本机 Context 存在这个目录；后续远程控制端仍不复制远端供应商数据 |
| 默认端口 | 模型 `127.0.0.1:17860`，管理 `127.0.0.1:17861` | 初始化可覆盖，仅允许数值回环地址与 1..65535 端口；已初始化实例不会被再次 init 改写 |
| 凭据保护 | Linux 当前用户目录 0700 / 文件 0600；Windows 当前用户受保护 DACL | **不是静态加密**。供应商秘密暂存在受保护的 SQLite 中；本机拥有者令牌在 owner.token 中；授权令牌只保存 SHA-256 摘要 |
| 运行方式 | 前台 `serve`，独立于管理 CLI 的生命周期 | 不自动注册系统服务、不自动后台化；关闭 serve 或承载它的终端可能停服 |
| CLI 输出 | 正常输出 JSON；日志 / 诊断 stderr | 仅明确 `access grant --show-once` 输出新授权令牌；供应商秘密只从 stdin 输入 |
| 协议顺序 | 先 Responses → Responses | 暂不支持 Chat / Anthropic 转换，不宣称 Codex / CC / DSH 已兼容 |

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

`api.example.com` 和 `upstream-model` 是占位示例，必须替换为供应商的实际 API 根地址与模型 ID。Base URL 不包含 `/responses`；代理保留该 URL 的路径前缀后追加 `/responses`。拒绝 URL 中的 userinfo、查询参数和 fragment；上游要求 HTTPS，仅数值回环测试端点允许 HTTP。

更新供应商 / 模型使用 `put --revision N`，通过 list 获取当前 revision；其余字段是完整替换，不是隐式合并。更新供应商时不提供 `--secret-stdin` 则保留旧秘密，显式输入空值则清空。删除操作尚未实现。

可通过下面的命令显式签发测试网关令牌；stdout 此时含秘密，不应进入日志或公共记录：

```bash
./scripts/anchor-dev.sh access grant \
  --label gateway-test --scope gateway --ttl 24h --show-once
./scripts/anchor-dev.sh access list
./scripts/anchor-dev.sh access revoke ACCESS_ID
```

`manage` 与 `gateway` scope 不互通，拥有者引导令牌只授权管理面。当前测试网关授权还没有绑定到 Agent Binding；绑定级授权将在托管阶段补齐。
Context 当前只支持 `--local`，创建时必须连通并验证实例身份，后续每次管理连接再次验证身份与 API 主版本。缺少 / 不可达 Context 不回退到其他目标。`init` / `serve` / `access` / Context 编辑只作用于本机；业务操作通过类型化客户端调用管理 API。

## 3. 实际 HTTP 契约

全部路由要求对应 scope 的 Bearer 认证；仅接受数值回环 Host，不接受浏览器 Origin；不开放公开管理或任意 CORS。

| 接口 | 状态 |
| --- | --- |
| `GET /control/v1/instance` | 实例 ID、API 主版本、程序版本、平台、目标用户、地址与能力 |
| `GET /control/v1/status` | 核心与网关就绪；供应商健康为 not_probed，托管为 not_implemented |
| `GET /control/v1/providers` | 脱敏供应商目录与 revision |
| `PUT /control/v1/providers/{id}` | `{provider, expected_revision, secret?}`；secret 缺失 / null 保留旧值 |
| `GET /control/v1/models` | 公开模型、能力与有序候选映射 |
| `PUT /control/v1/models/{id}` | `{model, expected_revision}`，拒绝不存在的候选供应商 |
| `GET /v1/models` | 已配置的公开模型目录；不代表真实供应商可用性 |
| `POST /v1/responses` | 同协议、非后台、无服务器会话状态的 Responses 转发 |

写管理 API 要求 `X-Anchor-Instance` 与目标一致。revision=0 只创建，N>0 只更新 revision=N 的资源；保存后 revision 加一。CLI 不自动重试写请求，断线后应查询目录、revision 和脱敏字段核对结果；完整持久操作查询与幂等键将在托管 / SSH 写操作阶段实现。

响应错误采用 `{error:{code,message}}`，常见状态：400 无效 / 未支持字段，401 鉴权失败，403 浏览器请求，404 未配置资源，409 实例或 revision 冲突，413 网关请求过大，429 网关总并发超限，502 上游拒绝 / 传输错误，503 无可用候选，504 总超时。

## 4. 网关的真实支持边界

- 只支持同协议透传，改写请求中的模型 ID 为候选实际模型，保留内容块、函数定义 / 调用 / 结果及上游响应事件。
- 只有 `function` 工具得到支持；工具由客户端执行。未知 / 服务端工具明确拒绝，不能借此宣称实际 Agent 工具兼容。
- 默认设置 `store:false`。拒绝 `store:true`、非 null 的 `previous_response_id` / `conversation`、`background:true`；客户端需发送完整 input 历史。未来服务器状态需要供应商亲和与相应 API，不可直接沿备用路由重放。
- `tools`、`streaming` 能力由用户在公开模型上显式声明；声明不等于真实上游探测通过。
- 全局并发上限 16；单供应商并发可配置 1..64；请求体上限 8 MiB，管理写入上限 128 KiB。
- 单次候选的总超时默认 120 秒，可配置 1..3600 秒；上游响应头等待上限 30 秒，下游每次写入等待上限 15 秒。
- 候选按用户声明顺序调用；禁用、不支持或并发已满的候选跳过；所有候选不可用时失败，绝不选择未配置默认模型。
- 只在明确收到上游 HTTP 429 / 502 / 503 / 504，且尚未提交下游响应时尝试下一个候选。不对同一个候选循环重试。
- DNS / TLS / 网络错误、结果未知的超时、认证错误、重定向、无效成功响应不自动切换。显式 HTTP 失败也不能保证上游没有执行或计费。
- 下游已提交任何响应后不重放；断开 / 取消传播到上游。流式失败可能只留下不完整事件序列，客户端应核对终止事件，不以 HTTP 200 判断模型任务完成。
- 不跟随上游重定向；不透传管理 / 网关令牌；上游仅收到供应商密钥。不把任意上游错误正文、认证头或 Location 返回给客户端。
- 管理与上游客户端均不使用环境 HTTP 代理，避免回环管理泄漏和绕过显式供应商路由；显式代理支持是后续独立能力。
- JSON 元数据日志记录请求 ID、公开 / 实际模型、供应商、协议、状态、耗时与传输结果；默认不记录请求 / 响应正文或认证秘密。用量目前标记 not_collected；上游用量在响应中原样保留，不伪造统计。

## 5. 生命周期与验证

两监听面全部绑定成功后才发布 ready；第二个端口失败会关闭第一个监听器。CLI 管理请求结束不会停止核心；Linux Ctrl+C / SIGTERM 与 Windows Ctrl+C 可触发停服流程。停服同时排空两面，最多等待 10 秒，然后取消剩余请求并关闭连接。

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
2. Codex 实际版本兼容、模型选择器、Binding 级鉴权与自动接管。
3. 配置字段所有权、持久恢复日志、条件恢复与独立 recover。
4. Chat / Anthropic 转换、CC / DSH 与声明式适配器。
5. Windows 真机测试、公开发行、安装器、系统服务、自启动和升级迁移。本地便携预发行包构建已提供，见发行流程。
6. 管理日志订阅、日志保留与用量汇总。

下一步优先补核心连接契约（SSH / Context），然后完成 Codex 的真实接入和安全托管闭环。不能把当前里程碑对外称为完成 A01–A14。
