# Anchor 最新 CLI 指令大全

- 最近核对：2026-10-10
- 适用实现：源码默认 `0.1.0-dev`，发行构建使用注入版本；均为当前开发里程碑，不是稳定发布。
- **本文件是唯一最新指令大全**，只列已实现入口。变更过程见 [指令变更历史](changes.md)，协议/安全细节见 [实施说明](../design/implementation.md)。
- 核对来源：`internal/cli/cli.go`、`cmd/anchor/main.go` 及其调用实现。

## 1. 调用与公共约定

下文 `anchor` 表示可执行文件。源码开发时使用仓库内 `./scripts/anchor-dev.sh` 替换 `anchor`；脚本会切换到仓库目录，并在当前机器使用 resource-guard。可通过 [发行流程](../release/README.md) 生成便携测试包；尚无安装器。源码脚本不包含在用户包中。

```text
anchor [--data-dir DIR] [--context NAME] COMMAND [--json]
```

| 全局选项 | 默认与行为 |
| --- | --- |
| `--data-dir DIR` | `os.UserConfigDir()/anchor`；Linux 通常为 `$XDG_CONFIG_HOME/anchor` 或 `~/.config/anchor`，Windows 通常为 `%AppData%/anchor`。存储本机实例、凭据和 Context；不同目录不共享默认 Context |
| `--context NAME` | 未提供时使用 `context use` 选择的当前 Context；只适用于 `status`、`provider`、`model`。本机操作显式提供非空 Context 会失败 |
| `--json` | 当前接受此兼容选项，但正常查询/写操作本来就输出 JSON，不切换格式；帮助仍为文本 |

推荐把全局选项放在命令之前。当前解析器也会从整条参数列表提取全局选项。值选项可写 `--name VALUE` 或 `--name=VALUE`；布尔选项关闭时用 `--name=false`，不要写 `--name false`。除表中指定的位置参数外，不添加额外位置参数。

- 正常操作 stdout 为一个 JSON 值；`help` 为文本，`serve` 没有正常 stdout 结果。诊断和服务日志在 stderr。`access grant` 的 JSON 是**明确含秘密的例外**。
- 应用退出码：成功 `0`，失败 `1`；帮助 `0`。开发包装脚本的资源保护/环境失败可能返回不同非零码。
- ID 和 Context 名称为 1..128 个字符，首字符为字母/数字，其余为字母/数字、`.`、`_`、`-`。
- 管理写请求不自动重试。断线后结果可能未知，先查询并核对 revision，不盲目重复写入。
- 密钥只从 stdin 输入；不要把真实密钥放进命令参数、历史或文档。供应商秘密目前依赖文件权限/ACL，**不是静态加密**。

## 2. 当前命令索引

共 19 个规范命令路径（别名单列于对应说明）。

| 指令 | 用途 | 作用域 / 前置条件 |
| --- | --- | --- |
| `help` | 查看总帮助 | 无需初始化 |
| `version` | 查看程序版本 | 无需初始化 |
| `init` | 初始化本机状态 | 本机；核心不应占用该状态目录 |
| `serve` | 前台运行核心 | 本机已初始化 |
| `context add` | 登记并验证本机实例 | 本机已初始化；核心在线 |
| `context use` | 选择默认 Context | 本机；名称已登记 |
| `context list` | 列出 Context | 本机已初始化 |
| `context inspect` | 查看已保存连接信息 | 本机；名称已登记 |
| `context test` | 验证连接与实例身份 | 本机；名称已登记，核心在线 |
| `status` | 查询核心状态 | 所选 Context 的核心在线 |
| `provider list` | 列出供应商 | 所选 Context 的核心在线 |
| `provider add` | 新增供应商 | 所选 Context 的核心在线 |
| `provider put` | 条件更新供应商 | 核心在线；当前 revision |
| `model list` | 列出公开模型映射 | 所选 Context 的核心在线 |
| `model add` | 新增公开模型 | 核心在线；候选供应商已配置 |
| `model put` | 条件更新公开模型 | 核心在线；当前 revision |
| `access grant` | 签发授权令牌 | 本机已初始化，无需核心在线 |
| `access list` | 查看授权元数据 | 本机已初始化，无需核心在线 |
| `access revoke` | 撤销授权 | 本机已初始化，无需核心在线 |

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

无专属参数；前台运行管理/网关两监听面，日志在 stderr。只有两端口均成功绑定才 ready；第二面失败释放第一面，同一实例已有核心或端口冲突则失败。管理 CLI 退出不会停止它。Linux Ctrl+C/SIGTERM、Windows Ctrl+C 触发最多 10 秒的优雅停服；关闭承载终端可能停服。没有已实现的 `stop`/后台化/系统服务命令。

## 4. Context 与状态

```text
anchor context add NAME --local
anchor context use NAME
anchor context list
anchor context inspect NAME
anchor context test NAME
anchor [--context NAME] status
```

- **`context add`**：`NAME` 必填，`--local` 必填。连接本机初始化的管理地址，核验实例身份/API 主版本后登记；重复名称失败，不覆盖既有 Context。返回连接对象（名称、endpoint、期望实例 ID、凭据引用），不输出拥有者令牌，不自动设为当前 Context。
- **`context use`**：`NAME` 必须存在；保存默认选择，返回 `current_context`。本步骤不检测在线状态。
- **`context list`**：无专属参数，返回连接对象数组；不检测可达性，当前实现不附带“当前选中”标记。
- **`context inspect`**：返回指定名称的已保存连接对象；不请求核心，不检测可达性。
- **`context test`**：连接指定实例并验证身份/API 主版本；返回实例信息，包括版本、平台、用户、监听地址和能力。失败不重新绑定或切换目标。
- **`status`**：无专属参数，查询显式或默认 Context，返回 `core`、`gateway`、`upstream_health`、`agent_management`；当前不会附带实例 ID、配置 revision 或网关地址；缺少 Context、不可达、认证失败或身份不一致均失败，不回退。

Context 目前仅支持本机 `--local`，没有 SSH、远程凭据或 Context 删除/重绑定入口。

## 5. 供应商

```text
anchor provider list
anchor provider add --id ID --base-url URL [OPTIONS]
anchor provider put --id ID --base-url URL --revision N [OPTIONS]
```

`list` 返回供应商数组（含 revision 和 `has_secret`，不含密钥）。`add` 创建不存在的 ID；`put` 更新既有 ID，返回保存后的供应商对象。

| 参数（add/put） | 必填 / 默认 | 约束 |
| --- | --- | --- |
| `--id` | 必填 | 供应商 ID |
| `--base-url` | 必填 | API 根地址，可含 `/v1` 前缀，不含 `/responses`；HTTPS，仅数值回环测试地址允许 HTTP；禁止 userinfo/query/fragment |
| `--protocol` | `responses` | 当前只接受 `responses` |
| `--timeout` | `120` | 单候选总超时，秒，1..3600；仍受上游响应头 30 秒等待上限限制 |
| `--max-concurrent` | `4` | 单供应商并发 1..64；网关全局另有限制 16 |
| `--disabled` | `false` | 设置后禁用此供应商 |
| `--revision` | `0` | add 必须为 0；put 必填当前值且 ≥1，通过 list 获取 |
| `--secret-stdin` | `false` | 从 stdin 读取密钥，最多 8192 字节，去除首尾空白；内部换行无效 |

更新为**完整配置替换**，不是字段合并：省略的普通字段回到表中默认值，例如未提供 `--disabled` 会启用供应商。密钥是例外：put 未提供 `--secret-stdin` 保留旧密钥，提供并读到空值会清空。成功保存 revision 加一；重复创建或旧 revision 冲突失败。失败不输出密钥。

安全输入示例（变量须由用户安全提供；URL 为占位值）：

```bash
printf '%s' "$PROVIDER_KEY" | anchor provider add \
  --id primary --base-url https://api.example.com/v1 --secret-stdin
```

没有供应商删除命令；可用完整 `put ... --disabled` 暂停路由，但这不会删除存储的秘密。

## 6. 公开模型与候选路由

```text
anchor model list
anchor model add --id ID --candidate PROVIDER:MODEL [OPTIONS]
anchor model put --id ID --candidate PROVIDER:MODEL --revision N [OPTIONS]
```

`list` 返回模型数组；add/put 返回保存后的模型对象。供应商/实际模型目录不自动探测。

| 参数（add/put） | 必填 / 默认 | 约束 |
| --- | --- | --- |
| `--id` | 必填 | 对外公开模型 ID |
| `--candidate` | 必填，可重复 | 1..16 个有序候选；格式 `供应商ID:上游模型ID`，按第一个冒号分隔；供应商须已配置，重复候选拒绝 |
| `--display-name` | 空字符串 | 最多 256 字节 |
| `--tools` | `false` | 声明函数工具能力，不等于上游验证通过 |
| `--streaming` | `false` | 声明 SSE 能力，不等于上游验证通过 |
| `--revision` | `0` | add 必须为 0；put 必填当前值且 ≥1，通过 list 获取 |

上游模型 ID 非空、最多 256 字节且不得含换行。put 完整替换候选、展示名及能力，省略能力开关会设为 false；revision 加一，旧值冲突失败。只按显式候选顺序路由，结果未知的超时/传输失败和已开始输出的请求不重放。详细重试和协议边界见实施说明。

```bash
anchor model add --id anchor-code --candidate primary:upstream-model --tools --streaming
```

没有模型删除命令；这里的示例不会证明真实供应商或 Codex 已兼容。

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

授权操作仅在本机执行，核心运行时亦可操作；不会随 grant/list/revoke 自动启动核心。拥有者引导令牌只授权管理面；新令牌目前未绑定 Agent Binding，也没有 CLI 导入/选择 manage 授权令牌的入口。

## 8. 最小本机流程

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

实际模型调用使用鉴权的 HTTP 数据面（`GET /v1/models`、`POST /v1/responses`），目前没有专用 CLI 推理命令；不要用拥有者管理令牌请求模型网关。

## 9. 功能的 CLI 覆盖与缺口

“大部分功能有指令”指用户可操作的管理能力，不要求每个内部函数、后台事件或 HTTP 推理请求各造一个子命令。新增用户操作时应优先补 CLI；例外在此表解释。

| 功能 | 当前入口 / 状态 | 说明 |
| --- | --- | --- |
| 初始化、启动、版本/帮助 | init / serve / version / help | 已提供 CLI；退出靠信号，尚无后台服务管理 |
| 本机连接、默认选择、检查 | context 五个子命令 | 已提供 CLI；删除、重绑定未实现 |
| 状态查询 | status | 已提供 CLI |
| 供应商、模型新增/更新/查询 | provider / model 的 add、put、list | 已提供 CLI；删除未实现 |
| 授权签发/列表/撤销 | access 的 grant、list、revoke | 已提供 CLI；管理令牌选择、Binding 级授权未实现 |
| 模型推理与模型目录数据面 | HTTP 网关 | 数据面供 Agent 调用，不强制新增 CLI 推理入口；无专用调用命令 |
| SQLite、限流、路由、日志脱敏 | 内部机制 | 无需一一变成命令；暂无独立日志订阅/用量查询 CLI |
| SSH 连接与远程凭据 | 未实现 | 用户管理能力，后续实现需同步提供 CLI |
| Agent 绑定/接管、条件恢复/recover | 未实现 | 用户管理能力，后续实现需同步提供 CLI |
| Chat/Anthropic 转换 | 未实现 | 后续协议能力及其用户配置需同步核对 CLI 覆盖 |
| 分发、系统服务、升级 | 本地发行脚本及手动升级说明；无产品 CLI 入口 | 便携预发行包构建已实现；公开发布、安装器、自启动、自动升级仍未实现，见发行流程 |

未实现项目不是可用指令；新增/删除命令及其他契约变化必须同步本文件与指令变更历史。
