# Anchor CLI 连接与实例管理设计

- 日期：2026-10-10
- 状态：设计提案，尚未实现
- 当前交付范围：Linux / Windows 的无头核心服务、CLI 与实例连接能力。
- 不在当前范围：Web 页面、桌面窗口、安卓服务端、多设备配置同步。
- 长期方向：桌面端是主要交互入口之一，但连接能力与业务能力不依赖桌面进程。

> 本文所有 `anchor` 命令、端口、接口、数据结构均为拟定契约，不是现有可执行功能。

## 1. 核心决定

1. 每个运行 Agent 的开发环境可以独立运行一个 Anchor 实例，负责该环境的模型代理。
2. CLI 是管理客户端，不直接读写远程数据库，不通过任意远程 Shell 拼接业务命令。
3. 本机管理通过回环 HTTP；远程管理首先支持 SSH 隧道中的相同 HTTP 管理 API。
4. 管理客户端不要求本机运行模型代理。只安装 `anchor` 客户端能力也可以管理远程实例。
5. 连接档案称为 context；显示名面向用户，实例 ID 用于防止连接到错误的实例。
6. 管理连接与 Agent 模型连接分开。切换 context 不会重定向任何 Agent 的模型请求。
7. CLI、未来桌面端复用同一连接组件和类型化管理客户端；桌面端不抓取 CLI 的表格输出。
8. Agent 配置修改由目标实例托管层执行；用户选择长期托管后，核心启动自动接管、正常停止条件恢复。详见 [托管层设计](agent-management-layer.md)。

## 2. 两条互不混淆的链路

### 2.1 管理链路

```text
本机 anchor CLI
  -> context 解析 / 凭据读取 / 连接组件
     -> 本机回环管理 API
     或
     -> 本机回环临时端口 -> SSH -> 远端回环管理 API
```

作用：管理供应商、模型映射、日志和指定实例的 Agent 接管配置。

### 2.2 模型链路

```text
机器 A 上的 Codex
  -> 机器 A 上的 Anchor 模型接口
  -> 机器 A 上配置的供应商
```

机器 B 只负责管理机器 A 时，B 不进入这条模型链路。B 的 CLI 退出、SSH 管理隧道断开、未来桌面端关闭，不停止 A 的模型代理。

第一版不提供“服务器上的 Codex 依赖笔记本上的 Anchor”这种反向转发拓扑。可以后续显式增加，但不能与远程管理混为一谈。

## 3. 核心服务的两个监听面

拟定默认端口，实施前检查冲突并允许配置覆盖：

| 监听面 | 拟定地址 | 用途 |
| --- | --- | --- |
| 模型面 | `127.0.0.1:17860` | Responses 网关与模型目录，给 Agent 使用 |
| 管理面 | `127.0.0.1:17861` | `/control/v1/` 管理 API，给 CLI / 未来桌面客户端使用 |

- 两个面默认均不绑定 `0.0.0.0`，不自动开防火墙或修改 SSH 服务配置。
- IPv6 回环需要显式且一致的监听配置，不能使用会意外解析到公网的默认主机名。
- 两套凭据、权限、接口路由独立：Agent 的网关凭据不能调用管理 API。
- 管理面没有 Web 页面；提供 HTTP API 不等于当前要开发 Web 前端。
- 核心默认以目标用户身份运行，只管理其明确登记的 Agent 配置。首版不是整机多用户管理员。
- 用户级实例配置可记录实际监听地址；本机 CLI 从受权限保护的本机实例登记信息读取，SSH context 显式记录远端管理端口。

## 4. Context 数据模型

每个 context 是客户端本地的连接档案，不是目标实例的业务配置副本。

```json
{
  "schema_version": 1,
  "current_context": "dev-a",
  "contexts": [
    {
      "name": "local",
      "transport": "local",
      "control_endpoint": "http://127.0.0.1:17861",
      "expected_instance_id": "<local-instance-id>",
      "credential_ref": "<local-credential-reference>"
    },
    {
      "name": "dev-a",
      "transport": "ssh",
      "ssh_target": "dev-a",
      "remote_control_host": "127.0.0.1",
      "remote_control_port": 17861,
      "expected_instance_id": "<remote-instance-id>",
      "credential_ref": "<remote-management-credential-reference>"
    }
  ]
}
```

约束：

- `ssh_target` 可以是已有 OpenSSH 配置中的主机别名；可选 `ssh_config_file`，而不是复制 SSH 私钥。
- SSH context 的远端管理主机仅允许明确的回环地址，不做任意网络转发器。
- 管理令牌不放在普通 context JSON 中，文件只保存凭据引用。
- 本地临时隧道端口、在线状态是运行时数据，不写成永久远端地址。
- 实例 ID 首次安装生成并持久化；重装、更换实例、复制实例数据需要明确的重新绑定流程。实例 ID 是防误连标识，不代替 SSH / TLS 身份校验或令牌认证。
- 用户名、平台和版本来自目标实例返回的元数据，不由本地操作系统推断。
- 一个物理机的 Windows、WSL、不同用户或容器可以是不同实例。显示名与实例 ID 不能等同于 hostname。
- 并发更新本地 context 文件时加锁并原子替换；上下文选择在命令开始时冻结。

## 5. 拟定命令体验

### 5.1 在需要模型代理的机器启动核心

```text
anchor init
anchor serve
```

`serve` 默认前台运行，适合终端与外部服务管理器监管。CLI 退出管理命令不会关闭服务；关闭正在执行 `serve` 的终端则属于停止该前台服务。

Linux / Windows 的自启动和服务注册属于后续生命周期工作，不在连接设计阶段假定已经支持。不由远程管理客户端自动安装、启动或提权。

### 5.2 本机管理

```text
anchor context add local --local
anchor context use local
anchor status
anchor model list
anchor provider list
```

本机绑定也必须经过认证并登记实例 ID，不能仅因为来源是 loopback 就放行。CLI 以同一用户读取受保护的本机引导凭据；不同系统用户不自动获得管理权限。

### 5.3 首次连接远端

前置条件：目标用户已安装 Anchor、启动核心，已有可用的 SSH 登录与 TCP 转发权限。

1. 用户先正常 SSH 登录，确认服务器身份，并配置适合自动连接的认证方式。
2. 在目标机器上，通过本机拥有者权限签发一个可撤销、可设置期限的独立管理令牌：

```text
anchor access grant --label my-laptop --scope manage --show-once
```

3. 在控制电脑登记连接：

```text
anchor context add dev-a --ssh dev-a
```

交互流程：展示非敏感目标信息 -> 建立隧道 -> 隐藏输入管理令牌 -> 完成认证 -> 展示实例 ID / 目标用户 / 平台 -> 用户确认绑定 -> 保存凭据引用和 context。

也可以从标准输入导入令牌，但不提供携带明文令牌的命令行参数。令牌仅在明确签发时显示一次，不进入历史、调试日志或普通列表输出。该初始手动授权流程避免跨 Windows / Linux 拼接远程签发命令，自动 SSH 配对留待后续。

本机引导凭据只用于同用户本机授权，不能把它当成所有远端客户端共享的主令牌。每台控制电脑分别签发令牌；撤销一个客户端不影响其他客户端或 Agent 网关凭据。context 删除与远端授权撤销是不同操作，应明确提示。

### 5.4 查看与切换

```text
anchor context list
anchor context inspect dev-a
anchor context test dev-a
anchor context use dev-a
anchor --context dev-a status
anchor --context dev-a model list
anchor --context dev-a provider list
anchor --context dev-a log follow
```

- `context list` 默认列出档案和最近已知状态，不为列出十台设备立即建立十个隧道；需要主动探测时显式执行 `test`。
- `use` 只修改当前客户端默认目标，不要求目标在线，也不更新任何代理配置。
- 管理命令选择顺序：显式 `--context` > 当前 context；未配置目标时报错。
- 当前目标不可达时不自动回退到本机或另一台设备。
- `inspect` 默认只展示脱敏连接信息，不读取供应商密钥。
- `status --json` 等机器输出仅将 JSON 写到 stdout；提示、警告和诊断写 stderr，永不混入令牌。

### 5.5 修改与接管

```text
anchor --context dev-a provider add
anchor --context dev-a model add
anchor --context dev-a agent manage codex --preview
anchor --context dev-a agent manage codex
```

- `agent manage` 保存长期托管选择，目标核心正在运行时立即计划接管；未运行时登记为待接管。管理 CLI 的退出不解除托管，具体生命周期见托管层设计。
- 通过远端管理 API 修改时，目标核心必须运行；核心未运行时的登记或恢复由目标用户在本机 CLI 执行。
- 所有修改执行在目标实例；供应商凭据输入通过已认证连接发送，不在本机保存另一份供应商数据库。
- 远程修改首版要求显式 `--context`，并显示目标名、实例 ID 和目标用户后确认，避免默认 context 切换造成误写。
- 自动化可使用显式目标加 `--yes`；不能把 `--yes` 理解为允许跳过实例身份校验。
- 接管先生成带 revision 的变更计划和差异；应用时重新校验文件状态并备份。预览后目标文件发生变化时拒绝应用旧计划。
- 接管仅作用于目标 Anchor 登记的 Agent 路径，不接受一个任意文件写入 API。
- 本机初始化、前台服务启动等生命周期命令只作用于运行命令的本机，不读取当前远程 context；帮助和输出必须明确这一点。

## 6. SSH 隧道生命周期

### 6.1 首版策略：按命令建立，不做常驻本地连接守护进程

一次普通命令：

```text
解析 context
 -> 加载凭据
 -> 找到系统 ssh / ssh.exe
 -> 建立本机回环临时转发
 -> 检查管理 API / 认证 / 实例 ID / API 版本
 -> 执行业务调用
 -> 关闭本次隧道
```

本机 context 跳过 SSH。`log follow` 等长连接命令在其存续期间保持隧道，Ctrl+C 后关闭订阅和隧道。

未来桌面端用同一连接组件创建长期 Session；不需要把短命令 CLI 改成桌面应用的宿主。只有在实际测得 SSH 重连开销影响体验后，才考虑可选共享连接代理。

### 6.2 系统 OpenSSH，而非首版自建 SSH 实现

复用已有配置中的 Host、密钥、SSH agent 与跳板机能力。Windows 检测 `ssh.exe`；缺少客户端时返回安装指引，不下载不明程序或自动修改系统功能。

隧道启动示意，不是要求用户手工管理固定端口：

```text
ssh -N -T
  -o BatchMode=yes
  -o StrictHostKeyChecking=yes
  -o ExitOnForwardFailure=yes
  -o ServerAliveInterval=15
  -o ServerAliveCountMax=3
  -L 127.0.0.1:<临时本机端口>:127.0.0.1:17861
  dev-a
```

实施要求：

- 参数数组启动程序，校验目标字符串，避免本地 Shell 拼接与参数注入；不使用 `-f` 放任后台进程。
- 普通连接要求已完成主机信任与非交互认证，不能关闭主机密钥检查或无限等待密码。首次 setup 可以引导用户进行正常 SSH 登录；密码交互模式留作显式后续能力。
- 禁用不需要的 agent / X11 转发和用户配置中额外的 Forward 配置；只创建本任务要求的隧道，确保不是借用并关闭用户已有的 master 连接。
- 普通命令设置总连接超时和 API 超时；日志流使用独立的流生命周期 / 心跳策略，不沿用短请求超时。
- 临时本地端口由连接组件选择。端口探测与 ssh 绑定之间存在竞争窗口，绑定失败需有限次数重试；不能承诺“找到空闲端口就一定绑定成功”。
- `ExitOnForwardFailure` 只确认转发建立，不能证明远端 Anchor 已运行，必须另做受认证管理 API 探测。
- 只记录本次创建的 PID / 进程句柄；正常结束、超时和用户中断均回收。父进程异常退出也需平台相应的子进程生命周期约束，在实现测试中验证。
- 不杀掉全部 ssh 进程，不清理用户已有 SSH 会话；临时目录和状态文件只删除本次拥有的内容。
- 远端管理端口被 SSH 服务禁止转发时返回明确错误，不自动修改 sshd 配置，也不改用无认证公网监听。

## 7. 认证与凭据保护

| 凭据 | 保存位置 / 用途 |
| --- | --- |
| SSH 私钥 | 继续由用户现有 SSH 环境管理，Anchor 不导入副本 |
| 本机引导凭据 | 目标实例受保护的用户目录，用于本机拥有者授权 |
| 管理访问令牌 | 控制客户端凭据存储；目标保存校验摘要、权限、有效期、撤销状态 |
| Agent 网关令牌 | 目标实例及其接管配置，限制为模型调用 |
| 供应商凭据 | 目标实例凭据存储，管理 API 只返回脱敏信息 |

`CredentialStore` 是独立接口，不要求无头 Linux 存在图形会话或已解锁桌面钥匙串。实施时分别验证 Windows 和无头 Linux 的可用保护方案；首版可以明确采用当前用户受限文件权限 / ACL 的存储方式，但必须标注其不等于静态加密，不能静默从加密存储降级。

未来使用操作系统安全存储时须定义服务启动时的解锁能力，不能仅证明桌面登录后可以读取。禁止把加密钥匙和密文放在同一普通文件后宣称具备有效的隔离保护。

其他边界：

- API 不因请求来源为 loopback 或 SSH 转发而免认证。
- 不启用任意 CORS origin；验证允许的 Host，不为未来桌面方便提前放开浏览器访问。
- 不执行环境中的公网 HTTP 代理配置来转发 loopback 管理请求。
- 无头模式缺少可用凭据时快速失败，不弹出 GUI 或无限等待输入。
- 默认不提供供应商原始密钥导出接口，也不把认证请求头写入日志。

## 8. API 与未来桌面端契约

初步管理 API：

| 接口 | 用途 |
| --- | --- |
| `GET /control/v1/instance` | 认证后获取实例 ID、平台、目标用户、版本、能力与监听信息 |
| `GET /control/v1/status` | 核心 / 模型代理状态，不把连接在线误认为供应商可用 |
| `/control/v1/providers` | 供应商管理，密钥只写入 / 脱敏返回 |
| `/control/v1/models` | 公开模型与上游映射管理 |
| `/control/v1/agent-bindings` | 目标用户的 Agent 接管对象管理 |
| `/control/v1/agent-bindings/.../plan` | 生成接管 / 恢复计划 |
| `/control/v1/agent-bindings/.../apply` | 校验后应用计划 |
| `GET /control/v1/logs/stream` | 已认证的结构化日志流 |

连接组件职责：

```text
ContextStore + CredentialStore
           -> Connector (local / ssh)
           -> Session (实例身份、管理客户端、关闭方法)
           -> 类型化业务服务
           -> CLI / 未来桌面端
```

- 连接建立、认证、实例身份检查、重连和错误分类只在共享层实现一次。
- 业务逻辑在核心服务；CLI 只组织输入、确认和输出。
- 桌面应用复用 SDK / 类型化连接接口；如果桌面技术栈不同，可通过受控本地 sidecar 提供结构化协议，不解析自然语言 CLI 输出。
- 桌面目标切换应创建独立 session 并隔离缓存 / 日志订阅，旧请求的迟到结果不得覆盖新目标。
- 长期连接关闭不调用远端停服；选择设备不等于选择模型流量目的地。
- 首版接口采用版本前缀并返回能力信息。主版本不兼容时报错，不把未知字段当成服务正常。
- 不对远程写请求盲目重试。配置变更带 revision，应用动作带 idempotency key；断线后先查询操作状态再决定是否重试。
- 数据库提交和 Agent 文件修改不是天然跨资源原子事务；接管操作需要持久化计划、备份和恢复状态，崩溃后可核对并恢复，而不是简单声称事务成功。

## 9. 失败行为

| 情况 | 要求 |
| --- | --- |
| 本机 / 远端核心未启动 | 明确提示目标与管理端口，不自动启动或切换目标 |
| SSH 未安装 | 明确告知缺少系统客户端 |
| SSH 认证失败 / 需要交互 | 提示先完成正常 SSH 登录 / agent 配置，不采集并保存 SSH 密码 |
| 主机密钥未知或变化 | 停止连接，要求用户核实，不自动接受变化 |
| 转发失败 | 提示端口竞争或服务端限制；只对本机绑定竞争做有限重试 |
| 管理令牌无效 / 到期 / 撤销 | 要求重新授权，不退回无认证访问 |
| 实例 ID 不符 | 停止所有业务调用，要求显式重新绑定 |
| API 版本不兼容 | 显示两端版本和升级建议 |
| 配置 revision 已变化 | 拒绝覆盖，重新获取配置 / 生成计划 |
| 写操作中途断线 | 标记结果未确认，按操作 ID 查询，不能直接宣称失败后重放 |
| 日志流断开 | 可有限重连；显示可能存在缺口，不虚构完整日志 |

CLI 应提供稳定的结构化错误码，区分传输、身份、认证、版本与业务失败。未来桌面端据此显示不同状态。

## 10. 后续连接方式

保留 `https` connector 的扩展点，但第一版不实现公网 / 内网直接远程管理入口、Web 控制台或浏览器 SSH 功能。

直接 HTTPS 需要明确部署、可信证书或明确证书绑定、认证、凭据管理及目标限制。未来安卓控制端可能使用该入口，但不因此在首版开放公网监听。

## 11. 验收计划（尚未执行）

1. Linux / Windows 前台运行同一核心与 CLI 契约，无浏览器、无桌面会话也能使用。
2. 本机管理经过认证；两个监听面及其凭据不能互相越权。
3. 控制电脑不运行模型代理，也能通过 SSH 管理远端实例。
4. Linux -> Linux、Windows -> Linux、Linux -> Windows、Windows -> Windows 分别验证；目标缺少 SSH Server 时给出前置条件而非宣称默认可连接。
5. SSH 别名、自定义配置、跳板机、主机密钥变化、非交互认证失败有明确行为。
6. 两个不同 context 的模型 / 供应商配置互不影响；不可达时不自动回退。
7. 远程接管只修改远端目标用户登记的配置，本机文件保持不变。
8. CLI 退出或管理连接断开后，远端模型代理仍可供远端 Codex 使用。
9. 普通命令、日志流中断、连接超时、父进程异常退出都不遗留任务创建的 SSH 进程。
10. 临时端口竞争、转发已建立但服务未运行、令牌撤销和实例重装可以被正确识别。
11. 日志 / context 输出 / JSON 不包含密钥；管理修改重复提交、并发 revision 冲突及未知结果可恢复。
12. 未来桌面端不需要重新实现业务逻辑、SSH 策略或凭据规则。

## 12. 外部依据

仅以下 SSH 行为是外部工具能力，Anchor 的命令与 API 均为设计提案：

- OpenSSH `ssh(1)`：本地 `-L` 转发、`-N`、`-T`、主机别名与跳板机。[官方手册](https://man.openbsd.org/ssh)
- OpenSSH `ssh_config(5)`：`BatchMode`、`StrictHostKeyChecking`、`ExitOnForwardFailure`、ServerAlive，以及转发成功不等于目标服务可用。[官方手册](https://man.openbsd.org/ssh_config)
- Windows OpenSSH 概览：客户端 / 服务端能力与安装前置条件。[Microsoft Learn](https://learn.microsoft.com/en-us/windows-server/administration/openssh/openssh-overview)
- Windows OpenSSH 配置：客户端配置路径，以及远端默认 Shell 的平台差异。[Microsoft Learn](https://learn.microsoft.com/en-us/windows-server/administration/openssh/openssh-server-configuration)

本设计不要求为修改 Anchor 业务配置而发送任意远程 Shell 命令，避免依赖远端 PowerShell / cmd / bash 的不同转义规则。
