# Anchor 最新 CLI 指令大全

- 最近核对：2026-10-15
- 适用实现：源码默认 `0.1.0-dev`，发行构建使用注入版本；均为当前开发里程碑，不是稳定发布。
- 中文供应商 ID 适用于本次开发修订及 公开版 `0.1.0-alpha.6`（早期本地构建 `0.1.0-dev.provider-unicode`）；公开版 `0.1.0-alpha.3` 不支持，须同时更新 CLI 与运行中的核心服务。
- **本文件是唯一最新指令大全**，只列已实现入口。变更过程见 [指令变更历史](changes.md)，协议/安全细节见 [实施说明](../design/implementation.md)。
- 本轮新增 `stop --approve`、核心 starting/stopping 状态与结构化客户端错误仅在当前源码 / desktop-dev 中，**不包含于已公开 alpha.6**；旧核心需显式正常退出并升级，不会自动替换。
- 本机 `update` 命令在当前 CLI 源码中；桌面在线更新入口从 desktop.9 提供，**desktop.32 已发布到默认 GitHub 源的 dev 渠道**。alpha.6 CLI / desktop.8 不含入口；CLI 未配置时默认 alpha，desktop.18 首次未配置时按安装版本选择渠道，已有渠道保持不变。
- 新增 `provider models --id ID --revision N`：当前源码 / desktop.10 小步；旧 alpha.6 / desktop.9 核心不支持，需正常停服后升级。
- 第一优先工作台适用于当前源码 / desktop.11：多 Key、管理操作、显式测试、三协议、用量与价格；旧核心须正常停服后升级，schema3 数据库不兼容旧程序。当前源码新增 Key / 实际模型协议继承，自动迁移到 schema4，旧程序不能打开；已随 desktop.16 dev 打包发布；安装包能力以各发行记录为准，升级须同步客户端与核心。
- 跨供应商共享公开模型、候选倍率与调度策略已随 desktop.19 dev 交付；`provider model-put` 参数见下文。倍率为手动配置，不自动读取站点分组价格。
- Agent discover/enroll/sync 在源码 CLI 中，桌面托管入口与核心已随 desktop.21 dev 发布；alpha.6 CLI 不含，需使用兼容客户端与核心。
- 平台兼容查询（sub2api / NewAPI / 原版 OneAPI）的核心接口已随 desktop.21 dev 发布，命令由源码 CLI 提供，alpha.6 CLI 不含，已安装桌面程序不提供通用 CLI。新增 schema5，旧程序不能打开升级后的数据库，客户端与核心需一起升级；desktop.22 的桌面平台配置／账户余额／Key 额度页已发布，desktop.25新增独立分组倍率查询；请求账单桌面页仍未接入。
- desktop.23 已交付全部启用 Key 的模型目录合并与逐 Key 分组倍率显示，停用 Key 不查询；复用 `provider models`，命令契约见下文。desktop.25已交付来源Key范围、模型排序／调度设置与账户分组倍率查询；独立账单桌面页仍未接入。
- 核对来源：`internal/cli/cli.go`、`cmd/anchor/main.go` 及其调用实现。

## 1. 调用与公共约定

下文 `anchor` 表示可执行文件。源码开发时使用仓库内 `./scripts/anchor-dev.sh` 替换 `anchor`；脚本会切换到仓库目录，并在当前机器使用 resource-guard。可通过 [发行流程](../release/README.md) 生成便携测试包；另提供 [一键安装脚本](../release/install.md)，从公开 Release 下载并校验单文件，不是 `anchor install` 命令，也不初始化状态。公开 Release 是安装源；实际公开版本及平台附件见发行页。发行程序的核心、CLI 和认证 helper 是同一个可执行文件，单独的 `anchor` / `anchor.exe` 即可运行，完整文档归档不是运行依赖；状态/配置仍独立保存。源码脚本不包含在用户包中。

```text
anchor [--data-dir DIR] [--context NAME] [--ssh-password-stdin] COMMAND [--json]
```

| 全局选项 | 默认与行为 |
| --- | --- |
| `--data-dir DIR` | `os.UserConfigDir()/anchor`；Linux 通常为 `$XDG_CONFIG_HOME/anchor` 或 `~/.config/anchor`，Windows 通常为 `%AppData%/anchor`。存储本机实例、凭据和 Context；不同目录不共享默认 Context |
| `--context NAME` | 未提供时使用 `context use` 选择的当前 Context；只适用于 `status`、`stop`、`provider`、`model`、`usage`、`logs`、`catalog`、`agent` 管理动作（不含 token）。本机操作显式提供非空 Context 会失败 |
| `--ssh-password-stdin` | 从 stdin 读取本次 SSH 密码（最多 8192 bytes，去掉末尾 CR/LF，非空），仅本次使用，不保存；SSH add 时选择密码模式，已保存密码 Context 每次管理请求重新提供。不能与供应商 `--secret-stdin` 同用 |
| `--json` | 当前接受此兼容选项，但正常查询/写操作本来就输出 JSON，不切换格式；帮助仍为文本 |

推荐把全局选项放在命令之前。当前解析器也会从整条参数列表提取全局选项。值选项可写 `--name VALUE` 或 `--name=VALUE`；布尔选项关闭时用 `--name=false`，不要写 `--name false`。除表中指定的位置参数外，不添加额外位置参数。

- 正常操作 stdout 为一个 JSON 值；`logs export` 为逐条 JSONL，`logs follow` 为重复页 JSON；`agent token` 是原始秘密文本例外；`help` 为文本，`serve` 没有正常 stdout 结果。诊断和服务日志在 stderr。`access grant` 的 JSON 是**明确含秘密的例外**。
- 应用退出码：成功 `0`，失败 `1`；帮助 `0`。开发包装脚本的资源保护/环境失败可能返回不同非零码。
- 供应商 ID 为 1..128 个 Unicode 码点，支持中文等 Unicode 字母、十进制数字，以及 `.`、`_`、`-`；首字符必须为字母或十进制数字。拒绝空白、控制/格式字符、组合附加符号、emoji、冒号、路径分隔符与 URL 保留符号；不自动裁剪、大小写折叠或 Unicode 归一化，按原值精确匹配。
- 当前源码中 Key 名称、公开模型 ID／别名、Context／设备连接名称和 Agent Binding ID 使用相同中文命名规则。Agent ID 另有最多 64 个 UTF-8 字节的限制（中文通常每字 3 字节），用于保持托管日志与目录文件名长度安全；其余为最多 128 个 Unicode 码点。实例／授权 ID、令牌、错误码及 SSH 技术字段不采用用户名称规则。此增量已随 desktop.33 桌面 dev 安装包发布；独立 CLI 正式预发行仍为 alpha.6，源码 CLI 需重新构建。使用中文名称须同时升级客户端和运行中的核心。
- 管理写请求不自动重试。断线、5xx、无效／超大回包可能使写入结果未知，先查询并核对 revision，不盲目重复写入。客户端以 `RequestError` 保留 HTTP 状态、受限 code 与 Unknown 标记，不把原始远端 message 或秘密写到 stderr；网络错误文案使用 `offline`，响应异常使用 `invalid_response`。退出码规则不变。
- 密钥只从 stdin 输入；不要把真实密钥放进命令参数、历史或文档。供应商秘密目前依赖文件权限/ACL，**不是静态加密**。

## 2. 当前命令索引

以下列出当前已实现的命令路径；别名单列于对应说明。

| 指令 | 用途 | 作用域 / 前置条件 |
| --- | --- | --- |
| `help` | 查看总帮助 | 无需初始化 |
| `version` | 查看程序版本 | 无需初始化 |
| `update config` | 查看本机更新渠道与源 | 无需初始化 |
| `update configure` | 配置本机更新渠道与源 | 无需初始化；源不能含凭据 |
| `update check` | 查询兼容的新版本 | 无需核心在线；默认 CLI，桌面必须提供实际版本 |
| `update download` | 下载并校验更新包 | 无需核心在线；仅接受更高版本 |
| `update apply --approve` | 原位更新本机 CLI，保留上一版 | 已下载、核心已退出、用户可写安装路径；不自动重启 |
| `init` | 初始化本机状态 | 本机；核心不应占用该状态目录 |
| `serve` | 前台运行核心 | 本机已初始化 |
| `context add` | 登记本机或 SSH 实例 | 本机已初始化；SSH 目标已安装支持 bootstrap 的 Anchor |
| `context bootstrap` | 远端账户初始化／启动核心并签发管理授权 | 本机操作；明确 `--show-once`；输出含秘密 |
| `context use` | 选择默认 Context | 本机；名称已登记 |
| `context list` | 列出 Context | 本机已初始化 |
| `context inspect` | 查看已保存连接信息 | 本机；名称已登记 |
| `context test` | 验证连接与实例身份 | 本机；名称已登记，核心在线 |
| `status` | 查询核心状态 | 所选 Context 的核心在线 |
| `stop --approve` | 请求核心正常停服 | 所选 Context 在线、已核验身份及管理授权；需要本轮新版核心 |
| `provider list` | 列出供应商 | 所选 Context 的核心在线 |
| `provider platform` / `platform-put` | 读取／配置平台身份和独立用户授权 | 核心在线；写入需当前 revision，令牌仅 stdin |
| `provider balance` / `key-quota` | 分别读取远端账户余额、Key 额度 | 核心在线；平台已配置、当前 revision；按平台提供授权 |
| `provider usage-records` / `effective-rate` | 读取远端逐请求账单／当前分组倍率 | 核心在线；当前 revision；部分平台需用户授权和站点 Key ID |
| `provider models` | 手动读取已存供应商的上游模型目录 | 核心在线；供应商 ID 与当前 revision |
| `provider add` | 新增供应商 | 所选 Context 的核心在线 |
| `provider put` | 条件更新供应商 | 核心在线；当前 revision |
| `provider workspace` | 查询 Key 状态、映射、用量与价格 | 核心在线；已存供应商 |
| `provider delete` / `rename` / `copy` | 删除、改名、复制供应商 | 核心在线；当前 revision；删除/改名须 approve |
| `provider key-put` / `key-delete` / `key-reset` / `key-model-protocol` | 多 Key 管理、默认协议与模型覆盖、状态恢复 | 核心在线；当前 revision；删除/恢复须 approve |
| `provider strategy` | 配置轮流或优先使用 Key | 核心在线；当前 revision |
| `provider test` | 显式最小推理测试 | 核心在线；已存供应商/模型 ID；须 approve |
| `provider model-put` / `model-delete` | 工作台模型映射写入/删除 | 核心在线；供应商与模型 revision；删除须 approve |
| `provider price-put` / `price-import` | 保存单价 / models.dev 预览 | 核心在线；当前 revision；导入不自动保存 |
| `logs list` | 结构化日志分页/筛选 | 所选 Context 核心在线，支持日志接口 |
| `logs detail REQUEST_ID` | 完整安全请求详情 | 对应记录仍在保留期 |
| `logs follow` | 每 2 秒输出日志页快照 | 核心在线；可重复更新记录 |
| `logs export` | 流式 JSONL 导出 | 核心在线；最多 200,000 条/2 分钟 |
| `logs settings` | 查询日志保留设置 | 所选 Context 核心在线 |
| `logs configure` | 校验并持久化保留设置 | 所选 Context 管理授权 |
| `usage` | 持久用量汇总 | 所选 Context 的核心在线 |
| `model settings` | 读取或保存模型能力、限制、思考默认值与参考快照 | 所选 Context；模型已存在，保存需当前 revision |
| `model routing` | 原子保存已有候选的完整顺序与调度规则；GUI 松手／改规则自动保存并可撤销 | 核心在线、当前模型 revision；来源和元数据保留 |
| `model list` | 列出公开模型映射 | 所选 Context 的核心在线 |
| `model add` | 新增公开模型 | 核心在线；候选供应商已配置 |
| `model put` | 条件更新公开模型 | 核心在线；当前 revision |
| `access grant` | 签发授权令牌 | 本机已初始化，无需核心在线 |
| `access list` | 查看授权元数据 | 本机已初始化，无需核心在线 |
| `access revoke` | 撤销授权 | 本机已初始化，无需核心在线 |
| `agent discover` | 只读识别本地已知配置目录及托管状态 | 新版核心在线；不证明安装或运行 |
| `agent enroll` | 批准 Codex 托管并复用已有模型映射 | 新版核心在线；完整本地缓存；更新须当前 revision |
| `agent sync` | 条件恢复并同步当前模型目录 | 核心在线、Binding enabled、当前 revision |
| `agent add` | 明确批准并登记 Codex 基础配置目标 | 核心在线；配置根存在；默认 all-compatible 需版本匹配元数据 |
| `agent list` | 查询 Binding 元数据 | 所选 Context 的核心在线 |
| `agent inspect` | 查看配置层检查与托管状态 | 核心在线；Binding 已登记 |
| `agent plan` | 脱敏配置计划与文件 fingerprint | 核心在线；尚未接管或已恢复 |
| `agent apply` | 按审核计划接管 | 核心在线；enabled；revision 和 fingerprint |
| `agent restore` | 字段级条件恢复 | 核心在线；当前 Binding revision |
| `agent enable` | 批准下次启动自动接管 | 核心在线；当前 Binding revision |
| `agent disable` | 条件恢复并禁用绑定授权 | 核心在线；当前 Binding revision |
| `agent set-models` | 显式更新默认模型与模型授权范围 | 核心在线；当前 Binding revision；先条件恢复 |
| `agent token` | Codex 认证命令专用秘密 stdout | 本机；enabled Binding；无需核心在线 |
| `recover` | 核心离线时独立恢复 | 本机；必须取得实例锁 |

所有 `context` 子命令均编辑/使用本地 Context 库，不接受 `--context` 作为远程编辑目标。不存在静默回退到其他实例的行为。

## 3. 帮助、版本和本机生命周期

### `help`

```text
anchor help
anchor --help
anchor -h
anchor
```

以上均输出总帮助文本，不读取实例状态。参数解析型命令的 `--help` 输出其 flag 帮助到 stderr；业务命令先建立管理连接，所以不能把子命令帮助当作离线总帮助。并非每个子命令都有独立帮助入口。

### `version`

```text
anchor version
```

输出 `{"version":"版本号"}`，无状态修改。源码开发默认 `0.1.0-dev`；通过发行脚本构建的程序返回本次 `--version` 注入的预发行版本，JSON 字段和退出码不变。发行工具的参数不是 anchor 的全局选项，见 [发行流程](../release/README.md)。

### `init`

```text
anchor init [--control-address ADDRESS] [--gateway-address ADDRESS]
```

| 参数 | 默认 | 要求 |
| --- | --- | --- |
| `--control-address` | `127.0.0.1:17861` | 管理监听；数值回环 IP，端口 1..65535 |
| `--gateway-address` | `127.0.0.1:17860` | 模型监听；数值回环 IP，端口 1..65535 |

创建受保护的目录、SQLite 状态、实例身份和本机拥有者凭据；返回 `initialized`、`settings`、`credential_protection`。重复初始化保留既有实例和监听配置，不使用新参数覆盖。正在运行的核心持有实例锁时，初始化失败。初始化不启动服务，也不自动创建或选择 Context。

### `serve`

```text
anchor serve
```

无专属参数；前台运行管理/网关两监听面，日志在 stderr。只有两端口均成功绑定才 ready；第二面失败释放第一面，同一实例已有核心或端口冲突则失败。管理 CLI 退出不会停止它。Linux Ctrl+C/SIGTERM、Windows Ctrl+C 触发最多 10 秒的优雅停服；关闭承载终端可能停服。本轮新增 `stop --approve` 可在另一进程请求正常停服；`serve` 不自行后台化，也没有系统服务管理命令。桌面宿主的独立核心模式见[桌面运行契约](../design/desktop-runtime.md)，不改变 `serve` 的前台语义。

两面绑定后才对已批准且 enabled 的 Binding 自动接管；某个目标失败不阻断其他目标，`status.bindings` 分别报告 pending/applied/overridden/conflict 等状态。正常停止先冻结托管写入并条件恢复，然后最多 10 秒排空请求。崩溃不会自动执行恢复；重新启动会核对原日志，或核心离线后用 `recover`。已运行的 Codex 会话可能缓存旧地址，恢复后应重启 Codex。

### `stop`（当前开发版）

```text
anchor [--context NAME] stop --approve
```

- `--approve` 为必需确认开关，默认 false；不弹出交互提示。无位置参数。缺少确认、未知参数、目标不可达、授权／身份校验失败时不发送停服写请求。
- 作用域为所选 Context，不是无条件停止本机所有进程；须先登记 Context，与其他管理命令一致。请求携带 Bearer 管理授权与 `X-Anchor-Instance`，调用 `POST /control/v1/shutdown`，不按 PID 强杀或关闭其他服务。
- 成功 stdout：`{"accepted":true,"instance_id":"<目标实例>","shutdown_complete":false}`。退出 0 **只代表请求被接受**，不代表进程已退出、在途请求全部成功或托管配置已恢复；未知／失败退出 1，不自动重放。
- 接受后核心拒绝新管理写请求和网关请求，执行既有托管条件恢复与最长 10 秒请求排空，再关闭监听；恢复冲突仍按核心日志与恢复流程处理。外部服务管理器可能重新启动进程，本命令不修改服务策略或开机自启动。
- 旧核心无该接口（通常 404），正在停止／未运行生命周期会拒绝（503）；升级须用户明确操作，命令不会替换核心、绕过锁或回退强杀。

## 4. Context 与状态

```text
anchor context add NAME --local
anchor context add NAME --ssh TARGET [--ssh-port N] [--identity-file PATH]
  [--remote-anchor anchor] [--remote-control-port 17861] [--trust-new-host]
anchor [--ssh-password-stdin] context add NAME --ssh TARGET [SSH_OPTIONS]
anchor context bootstrap [--control-port 17861] --show-once
anchor context use NAME
anchor context list
anchor context inspect NAME
anchor context test NAME
anchor [--context NAME] status
```

- **`context add`**：`NAME` 必填（支持中文的安全标识），`--local` 与 `--ssh TARGET` 二选一。本机模式连接已初始化核心；SSH 模式登录目标账户、执行固定 bootstrap 命令、验证实例身份/API/授权后登记。重复名称失败，不覆盖，不自动设为当前 Context。JSON 仅含档案公开字段，不含密码或管理令牌。本机模式拒绝 SSH 专用选项。
- **`context use`**：`NAME` 必须存在；保存默认选择，返回 `current_context`。本步骤不检测在线状态。
- **`context list`**：无专属参数，返回连接对象数组；不检测可达性，当前实现不附带“当前选中”标记。
- **`context inspect`**：返回指定名称的已保存连接对象；不请求核心，不检测可达性。
- **`context test`**：连接指定实例并验证身份/API 主版本；返回实例信息，包括版本、平台、用户、监听地址和能力；新版核心另返回 process_id，旧核心没有该字段。不按 PID 判断授权或强杀进程。失败不重新绑定或切换目标。
- **`status`**：无专属参数，查询显式或默认 Context，返回 `core`、`gateway`、`upstream_health`、`agent_management`；`core` / `gateway` 在运行生命周期中为 `starting`、`ready` 或 `stopping`，就绪前不接受管理写入或网关调用（可请求正常停服）；不会附带实例 ID、配置 revision 或网关地址；缺少 Context、不可达、认证失败或身份不一致均失败，不回退。

SSH 参数：`--ssh-port` 默认 0（密钥模式使用 SSH 配置，密码模式为 22），范围 0..65535；`--identity-file` 默认为空（系统默认密钥／SSH Agent），`--remote-anchor` 默认 `anchor`（依次检查 SSH 的 PATH、`~/.local/bin/anchor`、`~/bin/anchor`、`/usr/local/bin/anchor`、`/usr/bin/anchor`；自定义路径只使用指定程序，支持 `~/`，只允许安全可执行路径，禁止 shell 语法），`--remote-control-port` 默认 17861、范围 1..65535，仅转发远端 IPv4 回环。`--trust-new-host` 默认 false；显式同意后记录首次主机密钥，变化时始终拒绝。密钥模式使用系统 OpenSSH，可用 Host 别名／SSH Agent；密码模式内置 SSH，使用 user@地址，不读取 SSH 配置／别名，密码不保存。失败分类包括 ssh_missing、ssh_host_key、ssh_auth、ssh_unreachable、remote_anchor_missing（当前 SSH 账户下找不到可执行程序）、remote_anchor_unsupported（引导帮助检查失败）、remote_control_port（已有管理地址不匹配）、remote_core_not_ready（核心启动／就绪失败）、remote_anchor（其余引导失败或响应无效）、tunnel_failed，不输出原始 SSH stderr。连接前通过 `context bootstrap --help` 检查能力，不签发令牌；旧远端 CLI 的未分类引导错误保留 remote_anchor。

**`context bootstrap`** 是执行所在机器的本机操作，不接受 `--context`。远端必须已安装支持此命令的 Anchor。安全初始化当前账户缺失状态；管理地址必须等于指定的 `127.0.0.1:PORT`，保留既有身份和配置；核心未运行时启动独立后台 `serve`，已运行则核验复用。默认新建网关为 127.0.0.1:17860；端口冲突／身份不符不会强杀或覆盖。成功 JSON 含 settings、access、token（manage，720 小时），必须显式 `--show-once`，只通过已认证 SSH 接收，不应打印或记录秘密。

已保存 SSH 管理授权过期／撤销时，经重新 SSH 登录可以重新签发，但仍核验原实例身份。桌面选择独立于 CLI `context use`；关闭 CLI／UI 只关闭本次管理连接，远端核心继续运行。Context 支持本机和 SSH，尚无删除／重绑定入口。schema7 迁移后旧版不能打开本机状态，升级前备份并同步客户端／核心。

## 5. 供应商

```text
anchor provider list
anchor provider models --id ID --revision N [--key KEY]
anchor provider add --id ID --base-url URL [--base-url URL ...] [--homepage-url URL] [OPTIONS]
anchor provider put --id ID --base-url URL [--base-url URL ...] [--homepage-url URL] --revision N [OPTIONS]
```

`list` 返回供应商数组（含 revision 和 `has_secret`，不含密钥）。`add` 创建不存在的 ID；`put` 更新既有 ID，返回保存后的供应商对象。

| 参数（add/put） | 必填 / 默认 | 约束 |
| --- | --- | --- |
| `--id` | 必填 | 供应商 ID，支持中文，例如 `摸鱼AI`；1..128 个 Unicode 码点，完整规则见公共约定 |
| `--base-url` | 必填，可重复 | 请求地址，1..16 个且不可重复（忽略末尾 `/`）；多个地址按请求轮询做负载均衡，模型目录与测试使用第一个。每个都是 API 根地址，可含 `/v1` 前缀，不含 `/responses`、`/chat/completions` 或 `/messages`；HTTPS，仅数值回环测试地址允许 HTTP；禁止 userinfo/query/fragment |
| `--homepage-url` | 空 | 供应商主页，仅用于展示，不参与请求；HTTP(S)，禁止 userinfo |
| `--protocol` | 空 | `responses` / `chat-completions` / `anthropic`；仅用于新建 default Key 的初始协议及无 Key 时的匿名协议，不批量改已有 Key；空值保留已存值，新供应商按 `responses` |
| `--timeout` | `120` | 单候选总超时，秒，1..3600；仍受上游响应头 30 秒等待上限限制 |
| `--max-concurrent` | `4` | 单供应商并发 1..64；网关全局另有限制 16 |
| `--disabled` | `false` | 设置后禁用此供应商 |
| `--revision` | `0` | add 必须为 0；put 必填当前值且 ≥1，通过 list 获取 |
| `--secret-stdin` | `false` | 从 stdin 读取密钥，最多 8192 字节，去除首尾空白；内部换行无效 |

更新为**完整配置替换**，不是字段合并：省略的普通字段回到表中默认值，例如未提供 `--disabled` 会启用供应商，省略 `--homepage-url` 会清除主页，`--base-url` 列表整体替换。密钥是例外：put 未提供 `--secret-stdin` 保留 default Key，提供并读到空值会清除 default Key；其他命名 Key 保留。has_secret 表示至少存在一个已存 Key，不等于全部启用。成功保存 revision 加一；重复创建或旧 revision 冲突失败。失败不输出密钥。

安全输入示例（变量须由用户安全提供；URL 为占位值）：

```bash
printf '%s' "$PROVIDER_KEY" | anchor provider add \
  --id primary --base-url https://api.example.com/v1 --secret-stdin
```

中文供应商示例（`PROVIDER_KEY` 须先安全读取；URL 仍为占位值，需替换）：

```bash
printf '%s' "$PROVIDER_KEY" | anchor provider add \
  --id '摸鱼AI' --base-url https://api.example.com/v1 --secret-stdin
anchor model add --id public-model --candidate '摸鱼AI:upstream-model'
```

中文 ID 可用于 `provider add/put/list` 以及 `model add/put --candidate` 的供应商部分。命令仍要求显式参数，不会进入交互式向导。已有 ASCII ID、JSON 字段、SQLite schema、revision 与密钥输入方式不变；无需重新初始化，但只替换 CLI、不重启旧核心仍不能创建中文供应商。当前源码进一步支持中文 Key、公开模型 ID／别名、Context 和 Agent Binding ID；desktop.33 已支持上述中文名称；独立 CLI alpha.6 发行包仅支持中文供应商名称。

### `provider models`（desktop.10 小步）

```bash
anchor provider models --id '摸鱼AI' --revision 3
# 仅查询一个已存 Key 时添加 --key default
```

`--id` 必填已存供应商 ID；`--revision` 必填正整数，来自 `provider list`。使用所选 Context 的在线核心、管理鉴权与实例身份校验；不允许临时地址、密钥、协议或生成参数。revision 不匹配、未授权、实例不符或参数错误时非零退出，不发起目录请求。停用供应商也可显式查询。可用 `--key KEY` 指定启用的已存 Key，不存在时在请求上游前失败；指定停用 Key 时返回 `no_enabled_keys` 并跳过目录和倍率请求。省略时自动查询全部启用的已存 Key（最多64个），按模型 ID 合并并保留全部可选 Key；仅无已存 Key 才匿名，全部 Key 停用时不回退匿名请求。目录认证由 Key 默认协议决定，不使用模型覆盖。

读取已存 Base URL 和 Key，优先 `GET <base_url>/models`；保留站点部署前缀。未带版本的根地址或自定义前缀，在 `/models` 返回404/405或200 HTML网页时，尝试同前缀的 `/v1/models`（例如 NewAPI 的 `/newapi` → `/newapi/v1/models`）；已带 `/v1`、`/v2`、`/v1beta1` 等版本路径或版本后的 `/openai` 时不再追加 `/v1`。认证失败、429、5xx、重定向及损坏 JSON 不触发路径切换。OpenAI、Anthropic、Kimi/Moonshot 和硅基流动官方根地址固定 `/v1/models`；DeepSeek 官方根地址固定 `/models`，OpenRouter 官方根地址固定 `/api/v1/models`。Gemini 官方根地址或 `/v1beta/openai` 使用原生 `/v1beta/models`；`/v1beta`（含站点自定义前缀）采用 Google 目录认证 `x-goog-api-key`，其他目录沿用 Key 默认协议的 Bearer 或 Anthropic 请求头。Key 只放请求头，不写 URL。

解析 OpenAI/Anthropic `data:[{id:...}]` 与 Gemini `models:[{name:...}]`；Gemini 名称去除 `models/` 前缀，也接受兼容站返回的裸 ID。Anthropic 使用 `has_more/last_id` 和 `after_id` 继续分页，Gemini 使用 `nextPageToken/pageToken`；每页请求1000条。每个 Key 最多10次目录 HTTP请求（含路径切换），全部启用 Key 的目录与倍率抓取共用供应商超时与8秒的较小值和并发限额；目录累计响应最多512 KiB，每个 Key 最多1000条原始记录，合并最多1000个模型。先查全部目录，再查分组倍率。禁止重定向和环境代理，不自动重试失败请求。每 ID 最多256字节；最终去重排序，丢弃元数据。分页循环、格式变化、后续页失败或超限时该 Key 的目录整体失败；其他 Key 的完整结果可以保留，聚合超限则不返回模型列表。

stdout 为 `{provider_id, key_id, revision, http_status, elapsed_ms, models, failure, partial, entries, keys}`。默认聚合查询的 key_id 为空，显式查询为指定 ID，无 Key 为 anonymous。models 为合并去重排序的字符串数组；entries 为 `{id, key_ids}`，列出该模型对应的全部启用且目录成功的可选 Key。keys 逐项包含 `{id, enabled, protocol, http_status, failure, group, group_name, multiplier, rate_basis, rate_failure}`；multiplier 为十进制字符串或 null（未知），不会假定为1。至少一个 Key 目录成功时顶层 failure 为空，partial 表示存在失败 Key，逐 Key failure 保留原因；所有查询 Key 失败时 models/entries 为空；无启用 Key 时 http_status=0，models/entries/keys 为空。目录层失败输出结构化结果并退出0，调用者必须检查 failure 和 partial；管理层失败仍非零退出。目录失败分类为 no_enabled_keys（无启用 Key，未请求上游）、authentication、unsupported、rate_limited、http_error、network、timeout、busy、too_large、invalid_response。

分组倍率从已存平台账户授权自动匹配推理 Key 的远端身份后抓取，支持 Sub2API 的个人覆盖／分组默认倍率、NewAPI 的确定分组倍率；OneAPI 暂不支持，auto 分组无单一倍率。NewAPI 掩码候选必须通过固定只读 `POST /api/token/{数字ID}/key` 取回完整 Key 核对，不能凭掩码判定身份；Sub2API 使用完整 Key 精确匹配。最多4页账号 Key（每页100条），每 Key 的倍率查询最多8次请求／2 MiB响应，并受上述共享期限限制。倍率缺授权、未识别平台、找不到 Key、超时等均通过 rate_failure 表示，不丢弃已查询的模型，不把抓取值写入 Anchor 映射倍率。响应不包含上游错误正文、密钥或原始网络异常。

一次显式目录查询（可能包含路径切换和分页），不修改供应商、revision、模型映射或缓存；**目录成功不证明推理、工具、流式、余额或持续健康**，404/405不代表推理不可用。`model list` 与网关 `GET /v1/models` 返回 Anchor 已存公开映射，不是此上游目录。显式推理测试和映射写入使用下列独立命令，目录查询本身不自动导入。

供应商可用下列 delete 删除；有模型引用时阻止。完整 `put ... --disabled` 仅暂停路由，不删除秘密。

协议解析顺序为：选出 Key → 查该 Key 下实际候选模型名的覆盖 → 没有覆盖则用 Key 默认协议。覆盖随供应商改名/复制（copy-keys）保留，删除 Key 时清理；改变 Key 默认协议不会改动已有独立覆盖。旧供应商 protocol 自动复制给其所有既存 Key，后续 provider put 的 protocol 不改已有 Key。请求路径、认证、JSON 和 SSE 转换都使用本次解析结果；协议转换能力边界保持现有文本/函数子集。

### 第一优先工作台（desktop.11）

```text
anchor provider workspace --id ID
anchor provider delete --id ID --revision N --approve
anchor provider rename --id ID --revision N --target NEW_ID --approve
anchor provider copy --id ID --revision N --target NEW_ID [--copy-keys]
anchor provider key-put --id ID --revision N --target KEY [--secret-stdin] [--disabled] [--protocol PROTOCOL]
anchor provider key-model-protocol --id ID --revision N --target KEY --model UPSTREAM [--protocol PROTOCOL|inherit]
anchor provider key-delete --id ID --revision N --target KEY --approve
anchor provider key-reset --id ID --revision N --target KEY --approve
anchor provider strategy --id ID --revision N --strategy round-robin|priority
anchor provider test --id ID --revision N --model UPSTREAM [--target KEY] --approve
anchor provider model-put --id ID --revision N --target ALIAS --model UPSTREAM
                          [--model-revision M] [--multiplier N] [--strategy round-robin|priority] [--tools] [--streaming]
anchor provider model-delete --id ID --revision N --target ALIAS --model-revision M --approve
anchor provider price-put --id ID --revision N --target ALIAS --input-price USD --output-price USD
                          [--cache-read-price USD] [--cache-write-price USD]
anchor provider price-import --id ID --revision N --target ALIAS --catalog-provider PROVIDER --model UPSTREAM
anchor usage
```

上述动作均使用所选 Context 的管理鉴权及实例校验。`workspace.models` 返回当前供应商的 `multiplier`（相对倍率）、`strategy`（公开模型策略）及 `candidate_count`（总候选数）。`workspace` 的 keys 额外返回 protocol 和 model_protocols（[{model,protocol}]，始终数组），不返回秘密。`workspace` 的 id 必填；其余供应商动作还须当前 `--revision`，从 list/workspace 获取。供应商、Key ID 和公开别名均沿用中文命名规则。不自动重试；未知结果先 workspace/list 重读。stdout 为 Workspace（provider_id/revision/strategy/keys/models/usage/prices/estimates，及可空 probe/imported_price）；删除返回 deleted=true。`usage` 单独返回全供应商/模型聚合数组，包括已删除或改名前 ID 的历史数据。

| 动作 | 参数、默认值和语义 |
| --- | --- |
| delete / rename | `--approve` 必填；删除阻止模型引用，成功删除已存 Key/策略/价格；改名以事务迁移模型候选、Key、价格，模型和供应商 revision 增加，在途请求保留旧快照 |
| copy | target 必填且不能已存在；默认不复制 Key，`--copy-keys` 才复制秘密、Key 默认协议和模型覆盖；新对象 revision=1、默认停用，复制配置/策略，不复制映射/价格/用量 |
| key-put | target 必填，最多64个 Key；新 Key 必须有非空 stdin 秘密，已有 Key 省略或空 stdin 保留秘密；默认 enabled，`--disabled` 停用；替换秘密只经 stdin，删除用 key-delete；protocol 省略/空值保留已有 Key 协议，新 Key 默认 responses；可显式选 responses / chat-completions / anthropic |
| key-model-protocol | target 必填已存 Key，model 为实际模型名（不是公开别名，1..256 UTF-8 字节，无首尾空白/控制/格式字符）；protocol 为上述三协议；省略、空值或 inherit 删除覆盖，恢复动态继承 Key 默认值；不修改模型映射 |
| key-delete / key-reset | target 与 approve 必填；delete 删除秘密，reset 只清空该配置版本的内存失败/测试状态，不改变 enabled 或 revision |
| strategy | 默认 round-robin，另可 priority；前者轮流用可用 Key，后者按 Key ID 排序先用首个可用 Key；都跳过停用/失效/冷却 Key |
| test | model 必填上游模型 ID，approve 确认少量推理用量；target 省略时选第一个启用 Key，无 Key 才匿名。显式 target 可测试停用/冷却 Key。固定短文本、最多16输出 token、最多10秒并受并发限制；结果看 probe.failure，失败结果仍退出0，管理错误非零 |
| model-put | target 为公开别名；model 为上游 ID；model-revision 默认0创建或加入已有别名（当前供应商不能已映射），编辑须提供当前模型 revision。倍率 multiplier 范围0.01..1000，省略/0保留旧值，新候选默认1。新模型 strategy 默认 round-robin（按倍率倒数加权轮询），可选 priority（候选顺序）、cache-priority（缓存亲和）、lowest-price（路由倍率升序）、lowest-latency（近期成功延迟）；编辑省略 strategy 保留原策略。加入已有别名保留它的策略、能力和元数据；编辑 tools/streaming 默认 false，能力与策略对该别名所有供应商生效。只更新当前供应商的候选，保留其他候选 |
| model-delete | target、model-revision、approve 必填；只移除当前供应商的候选及对应价格；存在其他候选时保留公开模型、Binding 和其他供应商价格，模型 revision 加一。移除最后一个候选才删除公开模型，Binding 默认模型引用时409 referenced，事务回滚 |
| price-put | target 必须是该供应商映射的公共模型；输入/输出价格必填，USD/百万 token、范围0..1e6。缓存价格省略表示未知、0表示免费；完整替换该模型价格，来源 custom |
| price-import | target 为待定价公共别名，catalog-provider/model 为 models.dev 精确 ID；共享 catalog 的1小时缓存与固定公开目录最多30秒/16MiB，无推理凭据或重定向，公开下载支持 HTTPS_PROXY/NO_PROXY，拒绝陈旧快照；更新时间为快照获取时间，仅基础价不含分档。只返回 imported_price **预览**，不保存；核对后另行 price-put |

推理测试校验上游响应，而不是要求输出必须能跨协议转换。Responses 的 `reasoning` 项不再被误判为 `invalid_response`；16 token 额度包含推理，达到额度时只有推理内容的 `incomplete` 响应也可通过最小连通测试。成功不保证完整可见答案、跨协议推理转换或持续健康。HTTP 200 仍可能因无效 JSON、缺少响应 ID、上游失败或其他不支持的输出而返回 `probe.failure="invalid_response"`；不回显原始正文。推理项修复已随 desktop.16 dev 发布。当前源码进一步修复文本元数据误判（未发布）：测试允许文本项携带 `logprobs`、非空 `annotations`，只检查文本和元数据容器，不保存其内容；跨协议转换边界保持原有要求。测试失败时，`logs detail REQUEST_ID` / `logs export` 的 `evidence.validation_reason` 提供固定原因：`invalid_json`（无效 JSON）、`missing_response_id`（缺少响应 ID）、`upstream_error`（上游错误或 failed）、`unsupported_content`（文本结构不支持）、`unsupported_output`（输出类型不支持）、`invalid_tool_call`（工具调用不完整）、`invalid_choices`（Chat Completions choices 不符合要求）、`refusal`（拒绝回复）；未分类错误使用 `invalid_structure`。此字段不含原始错误文本或未知字段名，历史日志不会补写。命令语法、`probe.failure` 和退出码不变。

`provider test` 与真实模型转发先使用配置地址；若未带版本号的 API 基址返回 HTML（HTTP 200／404／405），核心保留部署前缀并追加 `/v1` 重试一次。两个请求共用原有超时与并发名额，不修改配置；已带 `/v1` 等版本路径、鉴权／限流／JSON 错误和已开始的 SSE 不触发补路径。若最终 HTTP 200 仍为 HTML，`probe.failure` 返回 `unexpected_html`，表示收到网站页面，应检查 API 基址。诊断日志的 `endpoint_resolution`、`endpoint_requests`、`initial_http`、`response_kind` 只记录结构事实。命令参数、输出字段结构与退出码保持不变；`failure` 增加上述枚举值。

配置/Key/模型协议覆盖/策略/价格写入增加供应商 revision；model-put/delete 单独修改模型，供应商 revision 保持。test、reset、price-import 不改配置 revision。变更 revision 或重启核心后 Key 临时健康重置。401/403 暂停该 Key；429 使用 Retry-After（默认30秒、最小1秒、最大1小时）；连续3次失败冷却30秒，随后单次半开探测；成功恢复。所有存储 Key 均不可用时不退回匿名。全局 status 的 upstream_health=per_key 表示查看工作台，不宣称总体健康。

usage/workspace.usage 按 provider_id/model 累计 requests/failures/elapsed_ms/known/input_tokens/output_tokens/cache_read/cache_write；requests 是**上游尝试数**，包含换 Key 重试及 `test:<上游模型>` 测试行。unknown=requests-known，平均延迟=elapsed_ms/requests；缺失用量不当成0。缓存 token 包含于 input_tokens。只保存计数，不保存提示词、输出正文或秘密。workspace.estimates 为按当前价格估算的已知部分 USD；没价格、无已知用量、发生缓存用量但缺缓存单价时为 null。不是账单，不记录历史价格或包含未知请求费用。

GUI 在供应商详情的三个页签提供同等操作。数据库启动自动从 schema2 升至3并迁移旧 secret 为 default Key，再升至4将供应商旧协议复制给全部既存 Key；安装前正常停止旧核心，升级后旧二进制不能再打开该数据库，无数据库降级入口。



### 来源 Key 与模型调度


`provider model-put` 支持可重复 `--source-key ID` 限制候选Key；省略时编辑保留原范围、新建允许所有启用Key。`--all-keys` 显式清空限制，与 source-key互斥。导入的指定来源Key参与真实调度；健康失败只切换范围内Key或其他候选，仍被引用的Key不能删除。`workspace.models` 返回 key_ids／candidate_index。schema5→6自动迁移，旧核心不能打开，需同步升级CLI与核心。

`model routing` 的 candidate列表必须完整且成员不变，原子修改共享模型顺序／规则、递增模型revision，保留所有来源Key、路由倍率、tools／streaming及Codex元数据。策略语义见[供应商接口 §9](../design/provider-interface.md#9-来源-key与模型调度2026-10-13当前源码)：缓存亲和10分钟（不保证缓存命中），低价按路由倍率，低延迟成功EWMA观察30分钟。输出JSON含model_id／revision／strategy／order。旧revision返回conflict，不自动重试写入。

```bash
anchor model routing --id coding-main --revision 3 --strategy priority \
  --candidate backup:upstream-model --candidate primary:upstream-model
anchor provider model-put --id primary --revision 2 --target coding-main \
  --model upstream-model --model-revision 4 --source-key key-main --multiplier 0.8
```

### 平台兼容查询（源码 CLI；desktop.21 核心已发布）

```text
anchor provider platform --id ID
anchor provider platform-put --id ID --revision N [--platform auto|sub2api|newapi|oneapi]
    [--site-url URL] [--user-id N] [--user-token-stdin | --clear-user-token]
anchor provider detect-platform --id ID --revision N [--key KEY]
anchor provider balance-summary --id ID --revision N [--key KEY]
anchor provider balance --id ID --revision N [--key KEY] [--remote-key-id N]
anchor provider key-quota --id ID --revision N [--key KEY] [--remote-key-id N]
anchor provider usage-records --id ID --revision N [--key KEY] [--remote-key-id N]
    [--scope account|key] [--page N] [--page-size N]
anchor provider effective-rate --id ID --revision N [--key KEY] [--remote-key-id N]
```

这些命令使用所选 Context 的在线核心、管理授权和实例校验。`--id` 为已保存供应商，查询和配置的 `--revision` 为 `provider list` 当前版本；配置成功递增版本，后续查询须使用新版本。普通查询不改供应商、公开模型、路由倍率、本地用量或日志保留设置，不请求付费推理。可查询停用供应商；显式 `--key` 可选择停用 Key。

`platform` 返回 `{provider_id, revision, config}`，配置只有平台、可选 `site_url` / `user_id` 和 `has_user_token`，不返回令牌。`platform-put` 完整替换公开配置；平台与推理协议分开，不能用 `responses` 等协议推断平台。`--site-url` 默认为空，从供应商首个请求地址去掉末尾 `/v1` 后推导后台根地址，保留真实部署前缀；若后台路径不同，显式指定 HTTP(S) 根地址，不能带查询、片段或 URL 凭据。`--user-id` 默认0，仅 NewAPI 用户接口使用，需该站点要求 `New-Api-User` 时填写。

`--platform` 默认 `auto`；既有未配置平台同样自动探测。`detect-platform` 根据公开接口和必要的 Key 接口响应重新识别 sub2api／NewAPI／OneAPI，不凭域名或站点品牌猜测，不写配置。无法确认返回 `platform_unknown`，凭据被拒绝保留分类失败。人工指定平台可覆盖自动模式。

`balance-summary` 在 NewAPI 已保存系统访问令牌时优先查询 `/api/user/self` 的账户余额（scope=account），与 `balance` 一致；授权失败直接报告，不退回 Key 数值。其他情况下使用默认推理 Key：sub2api 明确钱包返回 scope=account，受限 Key 返回 scope=key；NewAPI 优先读旧 billing 的 subscription 与 usage，以总额度减去已用量得到剩余值，scope=billing 表示范围由站点设置决定。仅在 billing 缺失或格式不兼容时回退 Key 原始 quota（scope=key）；认证、权限、限流、网络和服务错误直接报告。原版 OneAPI 返回 auth_required。余额／额度字段为空或失败时不能当作0。查询结果的 platform 为本次识别值，key_id 为实际所选 Key。NewAPI `balance` 有已保存用户授权时仍查账户钱包，无用户授权时使用所选 API Key 查 billing；不会将站点范围直接标成账户钱包。

`--user-token-stdin` 读取独立的后台用户授权（sub2api JWT、NewAPI/OneAPI 用户 access token）；NewAPI 在该站后台「个人设置／安全设置 → 系统访问令牌」生成，与推理 API Key 分开配置。支持权限选择的新版本，余额需 profile:read，自动核对 Key 分组还需 api_key:read 和 api_key:reveal。不能使用 `--secret` 参数传入。空 stdin 拒绝；与 `--clear-user-token` 互斥。未指定令牌选项时，仅平台、后台根地址和用户 ID 都不变才保留原令牌；改变其中任一项会清除。后台地址由请求地址推导时，修改请求地址也清除令牌；显式独立后台地址不受此影响。复制供应商不复制用户授权，即使指定 `--copy-keys`；改名保留，删除级联清除。凭据存于本机受限数据库，不能理解为数据库列加密。

`--key` 默认优先已启用的 `default`，否则回退首个启用的本地 Key，显式指定不存在的 Key 则管理失败。`--remote-key-id` 默认0，指站点后台的数字 Key ID，**与本地 `--key` 名称及密钥字符串不同**；需要时必须提供正整数。显式指定时，调用者须确保该数字对应希望查询的站点 Key。`balance` 不使用该数字；sub2api/NewAPI 的 Key 额度直接用所选密钥，原版 OneAPI 使用该数字和用户授权；倍率查询在该数字为0（默认）时，sub2api 优先以所选推理 Key 读取 `/v1/sub2api/billing` 的 `effective_rate_multiplier`，无需 JWT，包含个人覆盖与当前高峰规则。仅404/405或200 HTML网页说明接口缺失时回退用户账户列表；认证、权限、限流、服务和有效 JSON 格式错误直接报告。NewAPI 通过用户账户列表核对所选保存的完整密钥后读取分组；旧 sub2api 回退也沿用该身份核对，最多4页各100条。NewAPI 的掩码需通过固定的 POST `/api/token/{id}/key` 读取完整密钥核对，掩码本身不能证明身份；找不到返回 `key_not_found`。正整数仍直接读取指定远端 Key 的绑定分组。

| 功能 | sub2api | NewAPI | 原版 OneAPI |
| --- | --- | --- | --- |
| 模型目录 | 沿用 `provider models`，当前 Key 过滤后的声明目录 | 同左 | 同左 |
| 账户余额 | 用户授权查钱包；仅 Key 时要求 `/v1/usage` 明确包含钱包 `balance` | 用户授权查账户 quota；仅 Key 时查 billing 站点余额 | 用户授权查账户 quota |
| Key 额度 | Key 查询总额度／滚动窗口，未限额不等于钱包无限 | Key 查询 remaining / used / limit / unlimited | 用户授权 + remote-key-id 查 Key，保留原始 quota |
| 请求账单 | 用户授权分页；Key 范围另需 remote-key-id | 用户授权分页，或 Key 查近期批次 | 用户授权分页；Key 范围返回 unsupported_key_filter |
| 实际扣费及 tokens | USD actual_cost、输入输出、缓存、费用分项 | quota、输入输出及已有缓存／音频等数值明细 | quota、基础字段；token 语义标记待核验 |
| 当前有效分组倍率 | 推理 Key 直读当前有效倍率（含个人／高峰）；旧站回退或显式 remote-key-id 需用户授权 | 用户授权，默认自动核对所选 Key；可显式 remote-key-id，用户身份定价；auto 返回 unknown | unsupported |

sub2api Key 直接倍率结果的 `basis=key_billing`，`group` 和 `group_name` 可为空（接口未披露组名／ID），不可推定为默认组或1×；`multiplier` 为平台的当前有效计费倍率。显式正整数 `--remote-key-id` 保持旧的用户授权查询语义，不使用当前推理 Key 的直读结果替代该远端 Key。`provider models` 未配置账户平台时可通过一个 Key 专用接口请求识别 sub2api；其他平台仍需账户平台配置。倍率读取失败不会丢弃模型目录，也不会修改 Anchor 路由倍率。

`usage-records` 的 `--scope` 默认 `account`，可选 `key`。`--page` 默认1，上限100000；`--page-size` 默认50，范围1..100。sub2api 和 NewAPI 用户日志正常分页；NewAPI Key 日志只支持第1页且批次大小由站点决定，`history=recent`、`page_size=0`、`total=null`、`has_more=false`，不表示完整历史。原版 OneAPI 使用站点固定分页大小，返回 `page_size=0`、`total=null`，非空页 `has_more=true`，需空末页确定结束。所有记录受上游默认时间范围、日志保留和站点开关限制；这不是 Anchor 本地 `usage show` 或 `logs list`。

查询 stdout 为 `{provider_id, key_id, remote_key_id?, revision, platform, operation, observed_at, source, http_status, failure?, balance?, key_quota?, records?, effective_rate?}`，按操作只返回对应 payload。金额为 `{value, unit}`，`unit=USD|CNY|quota|site_unit`，不猜汇率或默认换算；billing 仅按公开 `quota_display_type` 确认 USD/CNY/TOKENS，无法确认则 site_unit。额度以 `scope=account|key|billing` 区分账户、Key 和范围未确认的站点余额，字段未知为 null，不能当作0；`unlimited=true` 仅表示 Key 自身不设上限。账单含 `charged_amount`、可选 `base_amount`、`billing_source`（NewAPI 已报告 wallet/subscription 时）、历史 `rate_multiplier`、tokens、已知内建工具的 tool_charges（次数与站点原始价格）和数值明细白名单；原始提示词、IP、后台信息及原始日志正文不返回。NewAPI 的日志 id 不是稳定标识，`id_stable=false`，有 `request_id` 时可关联请求；不承诺所有平台存在该字段。原版 OneAPI 的 token 字段标记 `platform_fields_unverified`，部分非文本计费可能不是实际 token 数。当前分组倍率是查询时快照，不替代历史账单扣费，也不自动写入候选路由。

模型查询仍用前一节 `provider models` 的输出和边界；其他查询最多8次请求（NewAPI 掩码核对包含固定 POST 读接口）、累计2 MiB响应，共用供应商超时与8秒的较小值，遵守全局／供应商并发限制，不重试、不跟随重定向，沿用核心上游传输策略（不读环境代理）。后台兼容路径和完整数据契约见 [供应商接口语义](../design/provider-interface.md#8-平台账户与计费兼容层2026-10-13当前源码)。

管理错误（参数、实例、授权、revision、Key 不存在）非零退出且不请求上游。上游能力失败仍输出 JSON 并退出0，需检查 `failure`，包括 `platform_required`、`auth_required`、`remote_key_required`、`key_not_found`、`unsupported`、`unsupported_key_filter`、`unsupported_pagination`、`unknown`、`authentication`、`permission_denied`、`rate_limited`、`http_error`、`network`、`timeout`、`busy`、`too_large`、`invalid_response`、`upstream_rejected`。失败不返回可用额度或账单；自动核对分组倍率失败时可保留已知分组但 multiplier=null，上游错误正文不暴露。

## 6. 公开模型与候选路由

2026-10-15 源码行为修复（未发布）：`model add` 及 `provider model-put` 创建全新公开别名时，共享核心自动尝试 models.dev 精确匹配候选的**上游模型 ID**，最多等待 3 秒，复用缓存。无歧义且所有候选解析为同一目录记录时，将上下文、输入/输出上限、能力、原生思考范围与完整参考价格快照写入模型 `settings`，同次创建 revision 仍为 1。显式 provider/model 身份或一致且唯一的 canonical 所属记录可消歧；不按本地供应商名、公开别名或模型名前缀猜测。下载失败、目录陈旧、未匹配、歧义和多候选不一致时正常创建但不自动填值，可用 `model settings --file FILE` 的 set 来源显式导入。更新/追加已有别名不覆盖设置，tools/streaming 仍按显式映射标记。仅请求公开目录，不带推理凭据；参考价格不写入供应商实际计费单价。

```text
anchor model list
anchor model add --id ID --candidate PROVIDER:MODEL [OPTIONS]
anchor model put --id ID --candidate PROVIDER:MODEL --revision N [OPTIONS]
anchor model routing --id ID --revision N --strategy RULE --candidate PROVIDER:MODEL [--candidate ...]
```

`list` 返回模型数组；add/put 返回保存后的模型对象。供应商/实际模型目录不自动探测。

| 参数（add/put） | 必填 / 默认 | 约束 |
| --- | --- | --- |
| `--id` | 必填 | 对外公开模型 ID |
| `--candidate` | 必填，可重复 | 1..16 个有序候选；格式 `供应商ID:上游模型ID`，按第一个冒号分隔；供应商须已配置，供应商 ID 支持中文，重复候选拒绝 |
| `--candidate-key` | 可重复 | add/put：`候选序号:Key ID`（序号1开始），限制该候选来源 Key；未指定允许所有启用 Key，put为完整替换 |
| `--strategy` | add/put默认priority；routing必填 | priority / round-robin / cache-priority / lowest-price / lowest-latency；provider strategy 仍只用于 Key 的前两种规则 |
| `--display-name` | 空字符串 | 最多 256 字节 |
| `--tools` | `false` | 声明函数工具能力，不等于上游验证通过 |
| `--streaming` | `false` | 声明 SSE 能力，不等于上游验证通过 |
| `--revision` | `0` | add 必须为 0；put 必填当前值且 ≥1，通过 list 获取 |
| `--codex-catalog` | 未提供 | 本地完整 Codex models JSON 文件，最多 1 MiB；与下面两项同时提供 |
| `--codex-model` | 未提供 | 源目录中唯一精确匹配的 slug；复制其完整元数据 |
| `--codex-version` | 未提供 | 源目录/目标 Codex 的精确 `X.Y.Z` 版本；由操作者核对，不自动探测 |

上游模型 ID 非空、最多 256 字节且不得含换行。put 完整替换候选、展示名、能力和 Codex 元数据，省略能力开关会设为 false，省略元数据三项会清除旧元数据；revision 加一，旧值冲突失败。此入口完整替换时策略恢复 priority、倍率恢复1；可用 provider model-put 编辑当前候选倍率及共享策略。priority 按显式候选顺序路由，round-robin 按倍率倒数分配可调度请求；倍率1和2的上游约分配2:1请求。跳过停用、Key 不可用和并发已满的供应商。结果未知的超时/传输失败和已开始输出的请求不重放。详细重试和协议边界见实施说明。

```bash
anchor model add --id anchor-code --candidate primary:upstream-model --tools --streaming
```

元数据含真实模型指令、reasoning 选项、工具与上下文等。应从与远端 Codex 精确版本匹配的官方目录取出与实际上游能力相符的条目；不要凭模型名字虚构能力，也不要把不同供应商的模型随意冒充同一个模板。Anchor 不自动下载 Codex 的版本匹配完整目录、不读取供应商密钥、不执行目录中的指令。导入需 JSON models 数组、唯一源 slug 和完整必需字段；无效/缺失/重复/超限拒绝。`model list` 会返回保存的元数据，普通网关 `/v1/models` 不返回这些指令。

```bash
anchor model add --id anchor-code --candidate primary:upstream-model --tools --streaming \
  --codex-catalog /absolute/path/version-matched-models.json \
  --codex-model SOURCE_SLUG --codex-version X.Y.Z
```

删除使用 `provider model-delete`（见 §5），核对供应商和模型 revision，只移除该供应商候选；删除最后一个候选时阻止删除 Binding 默认模型。这里的示例不会证明真实供应商或 Codex 已兼容。

### 模型能力与默认值设置（2026-10-14，源码未发布）

```text
anchor model settings --id ID
anchor model settings --id ID --file FILE
```

前置条件：选定已初始化、在线核心的 Context，使用管理凭据和正确实例身份；目标模型已存在。未提供 `--file` 时只读，返回 model_id、revision、display_name、candidate_count、configuration、reference。提供文件时保存，文件为至多 1 MiB 的单个 `SaveModelSettings` JSON 对象，拒绝未知字段和多个 JSON 值，`model_id` 必须与 `--id` 完全相同。文件为本机 CLI 读取，保存作用于所选核心；不直接修改 Agent 配置。

写入字段：`model_id`、当前 `expected_revision`、`display_name`、完整 `configuration`、`reference_action`、`reference_provider_id`、`reference_model_id`。configuration 中的 null/空数组表示未设置：limits.context/input/output；capabilities 的模态/视觉/附件/tool_call/reasoning/structured_output/temperature；streaming、parallel_tool_calls；supported_reasoning、default_reasoning；default_output_tokens、default_temperature。思考范围保留原生 effort/toggle/budget_tokens；默认值对应 value/enabled/budget_tokens，其他项为 null，必须在声明范围内。显示名与 configuration 是显式替换，不支持部分 patch。

`reference_action` 必须是 `keep`（保留已保存快照）、`clear`（清除）或 `set`（核心按精确 provider/model ID 保存 models.dev 快照，同时补齐未声明的上下文、限制、能力及思考范围）。补值保留显式设置，目录输入/输出与手工上下文或默认输出预算冲突时跳过相应限制；未知默认思考值不猜测。keep/clear 的两个来源 ID 必须为空；set 时必须都给出，不能传入前端生成的报价。目录下载失败/陈旧不保存新来源；写入超时/断线结果未知，应先重新读取核对 revision，不自动重试。

从读取结果整理写入文件的示例（`jq` 是外部工具）：

```sh
anchor model settings --id public > current-settings.json
jq '{model_id, expected_revision: .revision, display_name, configuration,
     reference_action: "keep", reference_provider_id: "", reference_model_id: ""}
    | .configuration.limits.context = 272000' current-settings.json > model-settings.json
anchor model settings --id public --file model-settings.json
```

正整数限制不超过安全整数，上下文不小于已声明输入/输出，默认输出不超过最大输出；temperature 默认值须声明支持且在0..2。保存不改变候选、倍率、Key 范围、调度或版本匹配 Codex 模板。普通 `model put` 保留本节 settings/reference（但其他旧字段仍按其完整替换语义）；供应商编辑同步 tools/streaming。返回的新模型 revision 也用于后续路由/供应商映射编辑。

Codex 同步导出可应用上下文、text/image 输入、并行调用及可表示的 effort/default；其他能力/默认调用值仅保存元数据，当前不自动改写请求。版本匹配完整模板仍必需，不支持的 effort（如 max/default/null）会使同步失败。`agent sync` 后重启 Agent 才读取新目录，现有会话不热更新。详见[模型参考目录契约](../design/model-catalog.md)。

## 7. 本机授权

```text
anchor access grant --label LABEL --scope manage|gateway [--ttl DURATION] --show-once
anchor access list
anchor access revoke ID
```

| grant 参数 | 必填 / 默认 | 约束 |
| --- | --- | --- |
| `--label` | 必填 | 非空、最多 128 字节，不含换行 |
| `--scope` | 必填 | `manage` 或 `gateway`，权限不互通 |
| `--ttl` | `0` | 非负 Go duration，如 `24h`、`30m`；0 永不过期，推荐显式有限期限；不支持 `1d` |
| `--show-once` | 必填，默认 `false` | 显式同意把新令牌输出到 stdout；未提供不签发 |

- **`grant`**：生成新授权，返回 `{access, token}`；**此输出含秘密，不应进入日志或公开记录**。持久化只保存令牌摘要；没有重新查看令牌的命令。元数据 `expires_at` 是 Unix 秒，0 表示永不过期。
- **`list`**：无专属参数，返回授权元数据数组，含 ID/label/scope/期限/revoked，不返回令牌。
- **`revoke`**：`ID` 必填，为授权记录 ID 而非令牌；持久撤销，返回 `{"revoked":true}`，不存在的 ID 失败。

授权操作仅在本机执行，核心运行时亦可操作；不会随 grant/list/revoke 自动启动核心。拥有者引导令牌只授权管理面；`access grant` 签发的普通授权不绑定 Agent；`agent add` 另行自动建立独立 Binding 网关凭据，限定该 Binding 的公开模型和模型目录。尚无 CLI 导入/选择 manage 授权令牌入口。

## 8. Agent 托管与独立恢复

### `agent discover` / `agent enroll` / `agent sync`

```text
anchor agent discover
anchor agent enroll --target ID --model MODEL --approve [--revision N]
                    [--model-scope all-mapped|all-compatible]
                    [--template PUBLIC_ALIAS:LOCAL_SOURCE ...]
anchor agent sync ID --revision N
```

作用域为所选 Context 的核心所在机器，需要兼容的新版核心在线。`discover` 无参数，只读返回 `{targets, bindings, models}`。目标包含路径、来源、适配状态、可选缓存版本和模板摘要；绑定包含 `adapter_id`、revision、默认模型、授权模型及恢复状态。models 的 `available_targets` 直接依据全部模型映射和至少一个启用供应商候选计算，不要求 tools、streaming 或 Codex 元数据，不做上游健康探测。接口不返回令牌或完整模型指令。

发现范围为绝对路径 `CODEX_HOME`、`CLAUDE_CONFIG_DIR`、`DSH_HOME`，以及 `~/.codex`、`~/.claude`、`~/.dsh`、`~/.gemini`、`~/.config/opencode`；Windows 另查用户 `AppData/Roaming/opencode`。同一目录去重，不扫描全盘、执行 Agent 或请求上游。目录痕迹不证明已安装或运行。当前 Codex、Claude Code、DSH 可托管；Gemini/OpenCode 只识别。

`enroll` 的 target/model/approve 必填；`--model-scope all-mapped|all-compatible` 可选。省略时 Codex CLI 保留 all-compatible，Claude/DSH 默认 all-mapped；桌面显式提交 all-mapped。revision 默认0，仅新目标使用；已登记目标须传当前值。默认模型必须在相应授权范围内。all-mapped 采用所有具有启用供应商的公开映射，清除旧目标模板覆盖，不读本地模型缓存、不生成 Codex 专有模型目录。模型名称来自映射，不能证明客户端或上游支持全部能力。桌面默认选择器显示全部映射，无启用供应商的项禁用并标明原因。

仅 Codex all-compatible 要求 tools/streaming、精确版本匹配的完整 Codex 元数据与本地 `models_cache.json`（最多4 MiB、128条，有效 `client_version=X.Y.Z`）。`--template ALIAS:SOURCE` 可重复，空源排除别名；未指定时依次使用共享元数据、精确别名或唯一上游 slug 匹配。其他范围不接受非空模板覆盖。发现时缓存缺失/无效不阻止 all-mapped 托管，兼容目录登记失败则需要核对缓存和版本，不伪造元数据。

登记为新绑定创建凭据；更新保留凭据。在线核心保存后立即应用；更新先条件恢复，冲突拒绝更新。可能出现配置应用失败但登记已保存，需要重新 discover 检查。成功返回 workspace。默认开关关闭，初次点击展开平台设置；点「开启托管」才提交批准，成功后开关打开。设置、目录和同步全部收进对应平台折叠区域。

`sync` 必填 Binding ID 和当前 revision≥1，仅 enabled 且网关就绪时可操作，不增加 revision、不旋转凭据。先校验新配置，再条件恢复和重新应用；第三方冲突保留并报告，失败不重放。网关授权按当前映射动态计算，客户端配置中的模型列表为应用时快照，模型变更后用 sync 更新。Codex/Claude Code 需要重启，DSH 默认模型在新会话生效。配置写入成功不证明真实请求已通过 Anchor。

### `agent add`

```text
anchor agent add --id ID [--adapter codex|claude|dsh] --config-root ABS_DIR --model MODEL --approve
                 [--anchor-executable ABS_PATH] [--model-scope all-compatible|all-mapped|single]
```

- 作用域：所选 Context 的目标实例；路径按目标机器解释。支持本机及已连接 SSH Context。
- `--id` 必填，符合中文命名规则且最多 64 个 UTF-8 字节；`--adapter` 默认 `codex`，支持 `codex|claude|dsh`，适配器修订固定为 `1`。
- `--config-root` 必填绝对目录，必须已存在；`--codex-home` 为仅限 Codex 的旧参数别名，二者不可同时传。Codex 修改基础 `config.toml` 和可选目录；Claude 修改 `settings.json`；DSH 修改已有 `profiles/web` 下的 `cordis.patch.yml`，不创建 profile 目录。
- `--model` 必填，指定**默认公开模型**，不是 all-mapped/all-compatible 的权限边界；仅 Codex single/all-compatible 要求 tools/streaming，声明不是上游能力探测。
- `--model-scope` Codex 默认 `all-compatible`，Claude/DSH 默认 `all-mapped`。all-mapped 授权所有具备启用供应商候选的映射，不要求 Codex 元数据或能力标记；all-compatible 仅支持 Codex：凭据可使用所有 tools/streaming、具有与绑定精确版本匹配的有效 Codex 元数据、且至少一个候选供应商 enabled 的公开模型；默认模型也必须在其中。版本从默认模型元数据固定到绑定。不是供应商健康探测，也不是任意/未知模型放行。
- `single` 只授权默认模型；有元数据时生成单模型目录，无元数据时保留旧的仅配置方式，Codex 需自行识别 slug 或已有目录。旧 Binding 未填 scope 时仍为单模型，升级不会静默扩权。
- `--anchor-executable` 默认当前 CLI 可执行文件的绝对路径；必须是目标机器现存、稳定的 Anchor 二进制。**不要把 `go run` 临时二进制作为长期 helper**，源码入口使用此命令时必须显式传稳定路径。搬动/删除二进制会破坏认证；目前没有修改 helper 路径的命令。
- `--approve` 必填显式为 true：批准长期配置管理、下次 `serve` 自动接管与正常停服恢复。只登记 SQLite/凭据，不立即改配置；当前服务中的接管需 `agent plan` + `agent apply`，或重启服务。
- 输出 Binding 元数据，`enabled:true`、`revision:1`，无秘密；同一规范化配置根只能登记一个 Binding，禁用亦保留所有权。Linux 配置根/文件须当前用户所有且不可被组/其他用户写入；符号链接、硬链接配置文件和 Windows reparse point 拒绝。
- Codex 支持基础配置，已选择 profile、WebSocket 和不支持的 TOML 结构拒绝。Claude 仅管理用户 settings 的默认模型、availableModels、Anthropic 网关/auth/default/subagent 环境字段及 apiKeyHelper；项目/策略/CLI 覆盖不探测。DSH 仅管理 web profile 的 llm-pi-ai 命名路由与 agent-default-model，使用 openai-completions 和公开映射；现有其他路由及 YAML 注释/自定义标量标签保留，insert、别名/合并和重复键拒绝。不会执行 Agent、自定义脚本或安装流程。

### `agent list` / `agent inspect` / `agent plan`

```text
anchor agent list
anchor agent inspect ID
anchor agent plan ID
```

- `list` 返回 Binding 数组。`inspect` 返回 `{binding, inspection, management}`：仅配置层支持、需重启、实际连接 `unknown` 及恢复状态；不读回原始配置或 token，不执行 Agent、不探测上游。
- `plan` 返回 binding ID/revision、目标路径、默认公开模型、model_scope、available_models/catalog_path/catalog_version（有目录时）、网关 base URL、credential method、受管理字段路径、配置与目录计划组合 fingerprint、重启/验证边界；不返回原值、备份或秘密。不写 Agent 配置。
- 当前 `plan` 用于新目标或已恢复目标；已接管时用 `inspect`，先 `restore` 再生成新的接管计划。配置已变化则旧计划不能应用。

### `agent apply` / `agent restore` / `agent enable` / `agent disable`

```text
anchor agent apply ID --revision N --fingerprint SHA256
anchor agent restore ID --revision N
anchor agent enable ID --revision N
anchor agent disable ID --revision N
```

- `ID` 与 `--revision` 必填；revision >= 1，从 list/inspect 获取。`apply` 另须已审核的 plan fingerprint，文件、目录计划、模型元数据或 revision 变化会拒绝，不自动重试。
- `apply` 只对 enabled Binding 且网关就绪时执行；先持久恢复日志，再原子写目录（有目录时）、原子替换配置指针并配置层校验。返回 `{binding_id, operation_id, status}`。再次应用已接管目标不重置原值或重放文件写入。托管字段为根级 model/model_provider/web_search、可选 model_catalog_json 和独立带标记 provider（含命令认证）；不嵌入 token。
- `restore` 仅恢复当前值仍等于 Anchor 写入值的字段；第三方值保留并报告 `conflict`。原值已恢复视为完成；被用户删除的配置不盲目重建；被基础配置或独立 `*.config.toml` profile 引用的 provider/目录不删除；目录只有仍等于 Anchor 写入内容且无引用时才删除，用户改过的目录保留并报告冲突。返回状态和冲突字段路径，冲突时 CLI 输出 JSON 后以 1 退出。
- `restore` 不修改 enabled 或 Binding revision，因此下次启动仍会接管。配置本来不存在时恢复可留下空文件，不删除可能已被其他工具引用的配置文件。
- `enable` / `disable` 返回 `{binding, management}` 并递增 revision；enable 只批准后续接管，不立即改配置。disable 先条件恢复再禁用凭据，即使出现已报告的第三方冲突也禁用授权，CLI 非零退出；真正的文件/日志 I/O 失败会中止禁用。已接受请求不承诺即时取消。
- `model put` 不会自动刷新已写入的 Codex 目录；网关 all-compatible 授权按每次请求的当前模型/供应商配置判断，而 Codex 目录是接管时快照。新增模型/改元数据后可用 `agent sync ID --revision N`，或 `restore` → `plan` → `apply`（或正常停服再启动），再重启远端 Codex app-server。更新目录不等于让已有会话立即切换。
- 当前没有 Binding 删除、改路径或改适配器版本入口；默认模型与授权范围用下面的 set-models 显式修改。

### `agent set-models`

```text
anchor agent set-models ID --revision N --model DEFAULT --model-scope all-compatible|all-mapped|single
```

所选 Context 核心在线；ID、当前 revision≥1、默认公开模型和明确 scope 全部必填。先校验新策略及目录，再条件恢复旧配置/目录；恢复冲突或 I/O 失败会中止策略变更，旧 revision/授权不变（无冲突的配置字段可能已恢复）。成功保存新 default/scope/精确元数据版本、revision 加一，不旋转 token、不自动接管，输出 `{binding, management, apply_required:true}`。重新审核 plan 并 apply，或下次正常启动才写新配置。旧绑定扩权需显式执行此命令；缩到 single 后非默认模型立即失去授权，已接受请求不承诺取消。

### 远端 Codex app-server 主路径

桌面端 SSH 运行远端 Codex 时，Anchor 应配置在**远端**，CLI 操作目标也是远端已批准 CODEX_HOME。基础配置提供统一 Anchor provider、默认模型、模型目录与认证 helper；无需额外 profile。Codex 客户端可以从目录选其他公开模型，或者显式 `--model`/app-server model 参数选择；所有这些模型请求仍经 Anchor 路由到各自候选供应商。模型目录不是授权凭据，网关仍单独校验模型范围。

本实现不生成/选择 profile、SSH Connector 仅负责目标 Anchor 的管理连接，不接管 Codex 所有网络出口、工具外网或登录状态。更高优先级配置/策略/命令参数可能改变供应商，inspect 不把配置写入当成运行连接证明。基础 provider/目录改动后重启远端 app-server；本轮隔离验证覆盖后台协议，不代表桌面模型选择器 UI 已验收。

### `agent token`（认证 helper）

```text
anchor agent token ID
```

本机操作，拒绝 `--context`；无需运行核心。成功 stdout 是**仅网关用途的原始秘密 token 加换行，不是 JSON**，供 Codex 命令认证直接读取；不要人工打印、管道到日志或分享。ID 必须是 enabled Binding。Binding 凭据在受保护 SQLite 中以秘密和鉴权摘要独立存储，非加密；普通查询、plan 和 Anchor 日志不含它。Codex 配置使用 helper，不内嵌 token；Claude/DSH 的受保护配置含仅网关用途的 Binding token（Linux 0600，Windows 当前用户权限），不写供应商密钥。DSH 自定义 Authorization header 不保证客户端日志脱敏，请勿分享原始配置/客户端请求日志。只能请求 Binding 授权范围内的公开模型（single 或动态 all-mapped/all-compatible），不能访问管理面。disable 后鉴权与 helper 输出均拒绝；重新 enable 会恢复同一绑定授权。

### `recover`

```text
anchor recover [--id ID]
```

本机操作，拒绝 `--context`。默认处理全部已登记 Binding（包括 disabled）；`--id` 可限定一个目标。必须核心停止并能取得实例锁；不读取默认 Context，不依赖 HTTP、Agent 或上游。使用接管时记录的原操作而非新规则重新推断，输出恢复结果数组；有冲突/失败时非零退出，保留第三方改动。恢复不撤销长期 enabled 批准，若不想下次重新接管需另行 disable。恢复材料在 `DATA_DIR/recovery/`，可能含原有秘密，严禁公开分享。

## 9. 最小本机流程

在仓库根目录执行；此流程会创建本机实例状态，不是只读检查。

```bash
./scripts/anchor-dev.sh init
./scripts/anchor-dev.sh serve
```

保持 serve 终端运行，在另一个终端：

```bash
./scripts/anchor-dev.sh context add local --local
./scripts/anchor-dev.sh context use local
./scripts/anchor-dev.sh status --json
./scripts/anchor-dev.sh provider list
./scripts/anchor-dev.sh model list
```

实际模型调用使用鉴权的 HTTP 数据面（`GET /v1/models`、`POST /v1/responses`、`POST /v1/chat/completions`、`POST /v1/messages`）。CLI 的 `provider test` 仅执行固定最小诊断请求，不是任意提示词调用命令；不要用拥有者管理令牌请求模型网关。

## 10. 功能的 CLI 覆盖与缺口

“大部分功能有指令”指用户可操作的管理能力，不要求每个内部函数、后台事件或 HTTP 推理请求各造一个子命令。新增用户操作时应优先补 CLI；例外在此表解释。

| 功能 | 当前入口 / 状态 | 说明 |
| --- | --- | --- |
| 初始化、启动、停服、版本/帮助 | init / serve / stop --approve / version / help | CLI 已提供；serve 仍为前台；无后台服务管理 |
| 桌面自动后台启动／复用 | 开发宿主已实现；原生两平台待验收 | 自动启动协调属于宿主机制，不要求单独命令；详见[桌面运行契约](../design/desktop-runtime.md) |
| 桌面窗口最小化／最大化／还原／关闭／托盘／退出界面 | 桌面自定义顶栏、系统托盘及「本机实例」弹层中的「退出界面」；原生行为未验收 | 本机窗口展示行为，不新增 CLI 命令；desktop.14 起普通关闭隐藏到托盘，单击图标恢复，菜单可退出界面；最小化按钮保留任务栏行为，无可用托盘时关闭直接退出宿主，均不停止核心 |
| 桌面单实例／重复启动恢复 | 桌面宿主已实现；原生两平台待验收 | 当前用户只运行一个 GUI 与托盘；重复打开恢复已有窗口，不新增管理命令。宿主锁独立于 `--data-dir`；后台核心和安装 helper 沿用原生命周期 |
| 开机自启动设置 | 未实现；无 GUI 开关或 CLI | 独立、默认不启用；未来实现需同步提供 CLI，安装和启动宿主不自动注册服务 |
| 本机连接、默认选择、检查 | context 五个子命令 | 已提供 CLI；删除、重绑定未实现 |
| 状态查询 | status | 已提供 CLI |
| 供应商、模型管理 | provider / model 的 add、put、list；provider delete/rename/copy/model-put/model-delete | GUI 和 CLI 共用引用保护与 revision |
| 平台账户／Key额度、请求账单和当前分组倍率 | provider platform/platform-put、balance/key-quota、usage-records/effective-rate | desktop.21 已发布核心接口；类型化客户端与源码 CLI 可用，alpha.6 CLI 不含；desktop.22 桌面已接入平台配置／余额／Key 额度，当前源码账户／模型映射页已接分组倍率，账单页面未接入；原版 OneAPI 能力缺口如上表 |
| 上游模型目录查询 | provider models --id ID --revision N；桌面供应商详情 | 目录本身只读；provider model-put 保存映射，provider test 显式推理 |
| 目录快速／全部导入（桌面） | 复用 provider model-put --id ID --revision N --target ALIAS --model UPSTREAM --multiplier 1 --tools --streaming | 桌面新建默认开启两项能力；CLI 默认不变。暂无专用批量命令，可读取 provider models/workspace 后逐项 model-put；跳过已有上游、非法或占用别名，首错停止并重读，禁止盲目重放 |
| 授权签发/列表/撤销 | access 的 grant、list、revoke | 已提供 CLI；管理令牌选择未实现；Binding 级凭据随 agent 自动管理 |
| 桌面实时快速测试 | provider models --id ID --revision N --key KEY；provider test --id ID --revision N --target KEY --model MODEL --approve | 弹窗先按 Key 查可用模型，在当前页显示临时流式文本；开始／轮询／停止为桌面展示生命周期，无新增 CLI 子命令。CLI test 继续同步返回原有 Probe，语法／输出／退出码不变 |
| 模型推理与模型目录数据面 | HTTP 三协议网关；provider test | 数据面供 Agent 调用，CLI test 仅最小诊断请求 |
| 多 Key 与恢复 | provider workspace/key-put/key-delete/key-reset/strategy | GUI 同等操作；冷却/熔断为内部机制 |
| 平台自动识别与列表余额（desktop.27） | provider detect-platform/balance-summary；platform-put 默认 auto | 供应商列表自动查询；NewAPI 已配置系统令牌时优先读账户余额，未配置时查默认 Key billing／额度；已随 desktop.27 投放。当前源码 GUI 切页先显示同实例／同配置的会话缓存并刷新，失败保留旧值标注「余额未刷新」，支持单行刷新按钮；仅桌面展示行为，沿用既有 balance-summary，无新增 CLI 契约 |
| 平台配置、账户余额与 Key 额度 | provider platform/platform-put/balance/key-quota | desktop.22 GUI「供应商 → 账户与余额」同等操作，复用核心兼容层；账单与倍率仍仅源码 CLI；额度保留原始单位 |
| 模型参考能力与估算 | catalog providers/models/model/estimate | 核心和桌面方法已提供；映射设置及显式默认值导入已接入源码，日志逐请求比价待接入，详见§6、§13 |
| 用量与价格 | usage；provider workspace/price-put/price-import | GUI 累计统计、单价设置与导入预览；未知明确区分 |
| 真实日志查询、详情、导出、保留管理 | logs list/detail/follow/export/settings/configure | GUI 与 CLI 共用核心日志；27 项列选择、顺序与预设是本机外观偏好，无需 CLI，不影响后台采集保存；见[日志设计](../design/logging.md) |
| SQLite、限流、路由、日志脱敏 | 内部机制 | 无需一一变成命令；暂无独立日志订阅；用量使用 usage |
| SSH 连接与远程凭据 | CLI／桌面已实现 | context add --ssh、--ssh-password-stdin、context test；远端须已安装新版 Anchor，自动安装待实现 |
| Agent 绑定/接管、条件恢复/recover | agent 管理子命令及 recover | 已提供 Codex 基础配置、Claude 用户 settings、DSH web profile、本地发现/登记/同步及桌面入口；全部映射默认选择和范围更新已提供；Gemini/OpenCode、其他 profile、绑定删除/路径更新仍缺失 |
| Chat/Anthropic 转换 | provider key-put / key-model-protocol；桌面 Key 与模型协议 | 无状态文本、函数工具、SSE、用量；不支持内容明确报错 |
| 在线升级 | update config/configure/check/download/apply；桌面设置页 | CLI 原位更新保留 .previous；桌面在线下载并确认正常停服后交接系统安装器。不是静默升级，无系统服务／数据库回滚；desktop.16 dev 已投放且真实下载通过，原生安装未验收，见 [在线更新](../design/online-updates.md) |
| 分发、系统服务 | 发行脚本与外部安装脚本 | 单文件打包及公开 Release 下载/校验安装脚本已实现（见 [安装说明](../release/install.md)）；公开下载仍为 CLI，见仓库 README。脚本替换需显式授权，不停服/迁移；Windows 当前用户安装器与 Ubuntu24.04 AMD64 `.deb` 打包入口已实现（[桌面说明](../release/desktop.md)，desktop.16 已发布 dev，原生安装未验收）；系统服务、自启动与静默自动更新未实现 |

未实现项目不是可用指令；新增/删除命令及其他契约变化必须同步本文件与指令变更历史。

发行许可为 [AGPL-3.0-only](../license.md)，允许商用并保留相应源码义务；已公开的 `0.1.0-alpha.3` 只含 Linux AMD64，远程安装已实际验证。当前公开版本和 Windows / Linux 附件以仓库 README 与[安装说明](../release/install.md)为准；stop、update 与第一优先供应商命令已在当前 CLI 源码中；对应桌面能力已随 desktop.16 dev 发布，不反写 alpha.6 CLI 的能力。


## 11. 本机在线更新

桌面首次未配置更新渠道时按安装版本选择（dev 包为 dev），已有配置保持不变；CLI 的未配置默认渠道仍为 alpha。通过 `anchor update configure --channel dev` 可显式选择桌面开发更新。

```text
anchor [--data-dir DIR] update config
anchor [--data-dir DIR] update configure [--channel stable|alpha|beta|rc|dev]
                                      [--manifest HTTPS_URL | --default-source]
anchor [--data-dir DIR] update check|download [--channel CHANNEL] [--manifest HTTPS_URL]
                                      [--product cli|desktop] [--current-version VERSION]
anchor [--data-dir DIR] update apply --approve
```

- 作用域：本机程序／更新缓存，不是 selected Context，所有子命令拒绝 `--context`；无需 init 或数据库。检查／下载不停止核心，不修改 Agent 配置。`--json` 沿用正常 JSON 输出。
- config：无参数，读取 `DIR/updates/config.json`；不存在时 `{channel:"alpha",manifest_url:""}`，无副作用。
- configure：只修改更新配置；省略参数保留旧值，`--default-source` 清空源，不能同时指定 `--manifest`。合法渠道为上述五项。源为空时为 GitHub Releases；URL 必须 HTTPS，无凭据／query／fragment，字面量回环 IP 的 HTTP 仅用于本机测试。配置影响同一数据目录的桌面宿主，用户确保源可信。成功返回配置 JSON；无模型密钥、无 GitHub token。
- check/download：参数只覆盖本次，不写配置。默认 product=cli、当前版本为当前 CLI 的注入版本；product=desktop 必须提供实际 `--current-version`，CLI 模式禁止该参数。默认先从API查询所选渠道的最高已发布 SemVer，不取 draft，最多500个 Release；成功查不到返回 available=false。API阶段最多10秒，网络/HTTP错误时尝试同仓库官方Atom近期发布，按同渠道SemVer最多读取5个候选清单；回退无兼容清单时报告失败，不误报无更新。自定义清单版本／渠道／平台必须一致。跨渠道更新同样必须严格高于当前版本，不支持在线降级。
- check 输出 `{current_version,version,channel,product,source,available,artifact}`，artifact 可为空；附件含 file/target/kind/url/sha256/bytes；可用性依据版本，不仅依据附件存在。成功查询无需更新时退出0；网络、限流、非法元数据失败退出1。
- download：先检查，再将更高版本的当前平台文件流式下载到 `DIR/updates/`，大小／SHA256 双重校验后写入 `<product>-download.json`；返回 `{check,path}`，path 是绝对路径。有效完整缓存可复用，不续传 partial。失败不覆盖现有程序或旧的完整缓存；无新版本、无兼容文件、大小或 SHA 不符均退出1。
- apply：仅处理已下载的 CLI receipt，不联网；必须 `--approve`、能取得本机实例锁（用户先显式停止核心）、文件与目录属于当前用户；Linux 还要求不对组／其他用户开放写入。重新检查平台、版本和实际哈希，再在可执行文件目录进行备份／替换，`.previous` 保留一份上一版，下次替换会覆盖该备份；普通替换失败尝试恢复。安装权限不足、符号链接／reparse 路径、Linux 硬链接、文件被修改、核心未退出等失败拒绝；Windows 缓存有受保护 ACL，但不声称已全面审计任意安装路径的 ACL／硬链接。不自动提权／重启／停远程服务。目标是 os.Executable 解析后的实际 CLI 路径，不允许任意目标路径。
- apply 成功输出 `{installed:true,backup:ABS_PATH,restart_required:true,core_restarted:false}`；installed 仅表示本次 CLI 文件替换完成，当前命令仍是旧进程，用户下一次运行／重新启动核心才使用新版本。需要回退时，正常停服后人工恢复备份；不承诺数据库回滚或所有数据目录中的进程都已退出。请先停止使用同一程序的其他核心。
- 网络代理：check/download 每次请求和重定向实时读取对应协议的 `HTTPS_PROXY`／`HTTP_PROXY`（含小写），再用 `ALL_PROXY`／`all_proxy`；未指定对应环境代理时，Windows 通过 WinHTTP 读取当前用户手动代理/PAC，PAC按URL解析并有界等待，失败可回退手动代理，仅自动发现失败可直连。HTTP-only设置不抑制HTTPS系统代理。`NO_PROXY`／`no_proxy` 同时适用于环境和系统代理，回环始终直连。支持 HTTP/HTTPS/SOCKS5 代理，非法明确配置失败，不静默直连。不使用第三方更新镜像。
- 当前平台包最多256 MiB、清单2 MiB；check/download 整个操作最多5分钟。更新清单格式、签名限制及 dev 投放步骤见 [在线更新](../design/online-updates.md)。SHA256 仅是完整性校验，不能替代独立发布签名。

桌面宿主配置入口为 `anchor-desktop [--data-dir DIR] [--update-channel CHANNEL] [--update-manifest HTTPS_URL]`，首次 GUI 启动时只在指定参数后保存配置；当前用户已有 GUI 时再次启动只恢复已有窗口，不切换其 `--data-dir` 或保存新更新参数。已有宿主未响应时有界等待后退出1，不再开第二个 GUI。核心 `--core` 和内部安装 helper 不受 GUI 锁限制。窗口连接和供应商功能仍走原会话规则。桌面 GUI 只接收受控渠道和已检查版本，不提供任意 URL/路径/命令执行。Linux 安装权限交由系统安装器，不能等同于 CLI apply 原位替换 `/usr/bin`。


## 12. 请求与系统日志（当前源码；基础能力自 desktop.16 dev）

需要升级并正常重启到支持日志的核心；旧版不会补采历史。使用所选 Context、owner 管理授权及实例身份核验。日志独立于累计 usage，配置日志保留不删除累计用量。

```text
anchor logs list|follow|export [--kind requests|system] [--since 1h] [--limit 50]
  [--cursor N] [--until N] [--status VALUE] [--source VALUE]
  [--provider ID] [--model NAME] [--key ID] [--http N]
  [--code VALUE] [--stage VALUE] [--level VALUE] [--search PREFIX]
anchor logs detail REQUEST_ID
anchor logs settings
anchor logs configure [--days N] [--system-days N] [--max-requests N]
  [--request-mib N] [--system-mib N]
```

| 选项 | 默认 / 范围 / 行为 |
| --- | --- |
| kind | requests；system 查询系统事件 |
| since | 1h，Go duration，>0 且 ≤2160h（90 天）；24h、168h 可用，不支持 `7d` |
| limit | 50，1–200；导出内部固定每页 100 条 |
| cursor / until | 0；列表 next_cursor 读取更旧记录，latest 固定序号上界 |
| status / source / provider / model / key / code / stage / level | 空表示不筛选；非空按原值精确匹配；requests 与 system 适用字段不同 |
| http | 0 表示不筛选；显式值 100–599，代表实际收到的上游状态 |
| search | 空；区分大小写的标识前缀（请求 ID / 模型 / 供应商 / Key / 错误码；系统事件名），不搜索正文 |
| days / system-days | 查询当前值再修改；初始均 7，范围 1–90 |
| max-requests | 初始 50,000，100–200,000 |
| request-mib / system-mib | 初始 256 / 32，范围 1–512 / 1–64；逻辑预算，非文件精确大小 |

`list` 输出 `{requests,events,next_cursor,latest,has_more,health}`，列表省略 attempts/evidence 大字段，详情输出完整安全 Request。HTTP/用量/指标缺失为 null，明确报告零仍为0。根请求任一尝试用量未知时总量未知。尝试最多64项，attempt_count 保存实际总数。

2026-10-15 当前源码（未发布）：请求 `metrics` 新增可空的 `time_to_first_output_ms`，表示从根请求开始到首个非空正文或工具调用参数增量（含 Responses 自定义工具输入）；角色、心跳、空增量、工具空壳与单独思考不计入。`time_to_first_text_ms` 继续只表示正文首字，原有正文指标继续保留，当前界面采用下述新增输出指标计算输出 TPS。旧历史缺少新字段时不补算；升级正在处理请求的核心后仅对新请求采集，旧桌面可忽略新增字段。

2026-10-15 当前源码（未发布）：请求 `metrics` 新增可空的 `output_generation_ms`、`non_reasoning_output_tokens` 和布尔 `output_interval_complete`。输出耗时从首次有效正文或工具参数增量到协议完成，不含首字前等待；非思考输出 Token = 本次成功尝试的确认输出 Token − 思考 Token，包含正文和工具参数，无思考事件的 Anthropic 沿用既有规则。桌面输出 TPS = 非思考输出 Token / 输出耗时秒数，只有流式、成功完整区间、可分离且为正的 Token 和耗时时显示；纯思考、中断、缺用量或无法分离思考时未知。区间可能包含交错思考或等待，并非纯解码速度。`generation_ms/output_text_tokens/text_interval_complete` 仍为原有正文指标；旧日志不补写，桌面只对已确认的完整正文统计作兼容回退。`logs list/detail/follow/export` 沿用既有入口输出新增字段，无新命令或参数；需要升级处理请求的核心才对新记录生效。

2026-10-13 当前源码（未发布）：`list/detail/follow/export` 的请求结果在协议正常结束且响应写入成功后保留 `succeeded` / `complete`，客户端在日志或用量收尾期间断开不再改记为 `client_cancelled`；未完成或写入失败时的取消仍为 `cancelled`。历史记录不自动重判，须升级正在处理请求的核心后才能影响新记录。原有正文指标继续保留；当前输出 TPS 口径见上文。命令、参数不变。

`follow` 每 2 秒输出一页完整 JSON（不是逐条去重订阅，可重复且 pending 后续会更新）；窗口起点在启动时固定，cursor/until 按提供值使用。Ctrl+C 终止，正常退出规则沿用应用。`export` 忽略 cursor/limit，从固定 latest 上界按页流式写 JSONL 到 stdout，最多 200,000 条、2分钟；每条为完整请求或系统事件，适合重定向到文件。记录到期/更新仍可能改变导出内容，不承诺事务级快照。中途失败返回非零，CLI 输出文件可能不完整，应按退出码处理。

`settings` 输出 `{days,max_requests,request_bytes,system_bytes,system_days}`；`configure` 沿用未提供项并校验、持久化配置，返回最终设置。收紧限制后每次清理有限删除，逐步收敛；没有 clear/delete 命令。未知记录 ID 返回错误；无授权、实例不匹配、存储不可用或超时均非零，禁止 Mock 回退。隐藏 UI 字段不影响采集、详情或导出。

```bash
anchor logs list --status failed --http 200 --since 24h
anchor logs detail REQUEST_ID
anchor logs export --kind requests --since 168h > anchor-logs.jsonl
anchor logs configure --days 14 --max-requests 100000
```

## 13. models.dev 参考目录与本地费用估算（desktop.29 dev）

```bash
anchor catalog providers [--refresh]
anchor catalog models --provider ID [--search TEXT] [--limit 50] [--refresh]
anchor catalog model --provider ID --model MODEL [--refresh]
anchor catalog estimate --provider ID --model MODEL \
  [--input-tokens N] [--output-tokens N] \
  [--cache-read-tokens N] [--cache-write-tokens N] [--reasoning-tokens N] [--refresh]
```

前置条件：连接已初始化、已运行的核心，使用管理凭据和所选 Context 的实例身份；本机可先建立 local Context（§9）。远端 Context 沿用既有安全连接边界。所有命令只读、输出 JSON，不写模型映射、价格或日志，不调用推理。公开目录由所选核心获取和缓存，而不是 CLI 自行抓取。

- `providers` 列出 models.dev 的供应商 ID、名称与模型数量。ID 不等于 Anchor 中配置的供应商名称。
- `models` 列出指定供应商模型，`--search` 在模型 ID／名称中做不区分大小写的子串搜索；`--limit` 默认50，范围1..100；返回 `total/truncated`。
- `model` 精确匹配 `--provider/--model`，返回输入／输出模态、视觉／工具／思考等能力、上下文／输入／输出限制、原始思考选项、USD 单价和上下文价格分档。没有模糊映射或跨供应商兜底；未声明字段返回 null，不把0上下文当可用限制，不生成猜测的思考等级。
- `estimate` 返回当前目录与本次用量的文本 token 估算。计数省略或 `-1` 表示未知，显式0表示观测为零，其他值须为0..9007199254740991。输入总量包含缓存读写，输出总量包含思考 token；估算分别扣除以避免重复计价。缓存计数缺失返回 `usd:null/reason:missing_cache_usage`，而非擅自按0计算。思考价格与普通输出价不同但计数缺失时同样返回未知。单位为 USD／百万 token，单价与结果使用十进制字符串；结果最多保留18位小数。分档阈值按输入总 token 严格大于 `above_tokens` 生效。调用方必须先将供应商原始用量转换为此口径。

共享固定 `https://models.dev/api.json`，最多30秒／16MiB，无推理凭据或重定向，公开下载支持 HTTPS_PROXY／NO_PROXY。缓存1小时，并发刷新合并；`--refresh` 强制尝试更新。自动刷新失败1分钟后重试，有旧快照时返回 `stale:true/failure:refresh_failed`，没有快照则报 `catalog_unavailable`；显式刷新仍可立即重试。结果包含 `fetched_at/expires_at`（Unix秒）、`version`（内容 SHA256）、模型 `source_updated_at`，没有实时账单保证。缺失价格或用量返回未知原因；非法参数、未知供应商／模型、无可用目录失败退出，读请求不自动重放。

示例（先从 `providers/models` 选出精确 ID）：

```bash
anchor catalog model --provider PROVIDER_ID --model MODEL_ID
anchor catalog estimate --provider PROVIDER_ID --model MODEL_ID \
  --input-tokens 1000 --output-tokens 100 \
  --cache-read-tokens 0 --cache-write-tokens 0 --reasoning-tokens 0
```

这是能力默认值和日志参考价的共享基础。目前已提供核心、CLI、桌面桥接与前端 Model 方法；映射表单显式导入与模型参考快照已接入当前源码（§6）；日志逐请求比价及逐请求价格快照尚未接入。仅估算文本 token，不推断音视频、图像专用费、请求费、路由倍率、折扣或平台账单。详见[模型参考目录契约](../design/model-catalog.md)。
