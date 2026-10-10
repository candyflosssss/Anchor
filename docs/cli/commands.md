# Anchor 最新 CLI 指令大全

- 最近核对：2026-10-10
- 适用实现：源码默认 `0.1.0-dev`，发行构建使用注入版本；均为当前开发里程碑，不是稳定发布。
- **本文件是唯一最新指令大全**，只列已实现入口。变更过程见 [指令变更历史](changes.md)，协议/安全细节见 [实施说明](../design/implementation.md)。
- 核对来源：`internal/cli/cli.go`、`cmd/anchor/main.go` 及其调用实现。

## 1. 调用与公共约定

下文 `anchor` 表示可执行文件。源码开发时使用仓库内 `./scripts/anchor-dev.sh` 替换 `anchor`；脚本会切换到仓库目录，并在当前机器使用 resource-guard。可通过 [发行流程](../release/README.md) 生成便携测试包；另提供 [一键安装脚本](../release/install.md)，从公开 Release 下载并校验单文件，不是 `anchor install` 命令，也不初始化状态。当前尚无公开 Release，远程安装入口未上线。发行程序的核心、CLI 和认证 helper 是同一个可执行文件，单独的 `anchor` / `anchor.exe` 即可运行，完整文档归档不是运行依赖；状态/配置仍独立保存。源码脚本不包含在用户包中。

```text
anchor [--data-dir DIR] [--context NAME] COMMAND [--json]
```

| 全局选项 | 默认与行为 |
| --- | --- |
| `--data-dir DIR` | `os.UserConfigDir()/anchor`；Linux 通常为 `$XDG_CONFIG_HOME/anchor` 或 `~/.config/anchor`，Windows 通常为 `%AppData%/anchor`。存储本机实例、凭据和 Context；不同目录不共享默认 Context |
| `--context NAME` | 未提供时使用 `context use` 选择的当前 Context；只适用于 `status`、`provider`、`model`、`agent` 管理动作（不含 token）。本机操作显式提供非空 Context 会失败 |
| `--json` | 当前接受此兼容选项，但正常查询/写操作本来就输出 JSON，不切换格式；帮助仍为文本 |

推荐把全局选项放在命令之前。当前解析器也会从整条参数列表提取全局选项。值选项可写 `--name VALUE` 或 `--name=VALUE`；布尔选项关闭时用 `--name=false`，不要写 `--name false`。除表中指定的位置参数外，不添加额外位置参数。

- 正常操作 stdout 为一个 JSON 值，`agent token` 是原始秘密文本例外；`help` 为文本，`serve` 没有正常 stdout 结果。诊断和服务日志在 stderr。`access grant` 的 JSON 是**明确含秘密的例外**。
- 应用退出码：成功 `0`，失败 `1`；帮助 `0`。开发包装脚本的资源保护/环境失败可能返回不同非零码。
- ID 和 Context 名称为 1..128 个字符，首字符为字母/数字，其余为字母/数字、`.`、`_`、`-`。
- 管理写请求不自动重试。断线后结果可能未知，先查询并核对 revision，不盲目重复写入。
- 密钥只从 stdin 输入；不要把真实密钥放进命令参数、历史或文档。供应商秘密目前依赖文件权限/ACL，**不是静态加密**。

## 2. 当前命令索引

共 30 个规范命令路径（别名单列于对应说明）。

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

无专属参数；前台运行管理/网关两监听面，日志在 stderr。只有两端口均成功绑定才 ready；第二面失败释放第一面，同一实例已有核心或端口冲突则失败。管理 CLI 退出不会停止它。Linux Ctrl+C/SIGTERM、Windows Ctrl+C 触发最多 10 秒的优雅停服；关闭承载终端可能停服。没有已实现的 `stop`/后台化/系统服务命令。

两面绑定后才对已批准且 enabled 的 Binding 自动接管；某个目标失败不阻断其他目标，`status.bindings` 分别报告 pending/applied/overridden/conflict 等状态。正常停止先冻结托管写入并条件恢复，然后最多 10 秒排空请求。崩溃不会自动执行恢复；重新启动会核对原日志，或核心离线后用 `recover`。已运行的 Codex 会话可能缓存旧地址，恢复后应重启 Codex。

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
| `--codex-catalog` | 未提供 | 本地完整 Codex models JSON 文件，最多 1 MiB；与下面两项同时提供 |
| `--codex-model` | 未提供 | 源目录中唯一精确匹配的 slug；复制其完整元数据 |
| `--codex-version` | 未提供 | 源目录/目标 Codex 的精确 `X.Y.Z` 版本；由操作者核对，不自动探测 |

上游模型 ID 非空、最多 256 字节且不得含换行。put 完整替换候选、展示名、能力和 Codex 元数据，省略能力开关会设为 false，省略元数据三项会清除旧元数据；revision 加一，旧值冲突失败。只按显式候选顺序路由，结果未知的超时/传输失败和已开始输出的请求不重放。详细重试和协议边界见实施说明。

```bash
anchor model add --id anchor-code --candidate primary:upstream-model --tools --streaming
```

元数据含真实模型指令、reasoning 选项、工具与上下文等。应从与远端 Codex 精确版本匹配的官方目录取出与实际上游能力相符的条目；不要凭模型名字虚构能力，也不要把不同供应商的模型随意冒充同一个模板。Anchor 不下载目录、不读取供应商密钥、不执行目录中的指令。导入需 JSON models 数组、唯一源 slug 和完整必需字段；无效/缺失/重复/超限拒绝。`model list` 会返回保存的元数据，普通网关 `/v1/models` 不返回这些指令。

```bash
anchor model add --id anchor-code --candidate primary:upstream-model --tools --streaming \
  --codex-catalog /absolute/path/version-matched-models.json \
  --codex-model SOURCE_SLUG --codex-version X.Y.Z
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

授权操作仅在本机执行，核心运行时亦可操作；不会随 grant/list/revoke 自动启动核心。拥有者引导令牌只授权管理面；`access grant` 签发的普通授权不绑定 Agent；`agent add` 另行自动建立独立 Binding 网关凭据，限定该 Binding 的公开模型和模型目录。尚无 CLI 导入/选择 manage 授权令牌入口。

## 8. Agent 托管与独立恢复

### `agent add`

```text
anchor agent add --id ID [--adapter codex] --codex-home ABS_DIR --model MODEL --approve
                 [--anchor-executable ABS_PATH] [--model-scope all-compatible|single]
```

- 作用域：所选 Context 的目标实例；路径按目标机器解释。当前 Context 仍仅本机。
- `--id` 必填，符合普通 ID 规则且最多 64 字符；`--adapter` 默认/仅支持 `codex`，适配器修订固定为 `1`。
- `--codex-home` 必填绝对目录，必须已存在；批准修改其中的基础 `config.toml` 与本绑定专有的 `anchor_ID.models.json`（生成目录时），不依赖控制端环境变量猜测目标。
- `--model` 必填，指定**默认公开模型**，不是 all-compatible 的权限边界；模型需显式声明 `--tools --streaming`，声明不是上游能力探测。
- `--model-scope` 默认 `all-compatible`：凭据可使用所有 tools/streaming、具有与绑定精确版本匹配的有效 Codex 元数据、且至少一个候选供应商 enabled 的公开模型；默认模型也必须在其中。版本从默认模型元数据固定到绑定。不是供应商健康探测，也不是任意/未知模型放行。
- `single` 只授权默认模型；有元数据时生成单模型目录，无元数据时保留旧的仅配置方式，Codex 需自行识别 slug 或已有目录。旧 Binding 未填 scope 时仍为单模型，升级不会静默扩权。
- `--anchor-executable` 默认当前 CLI 可执行文件的绝对路径；必须是目标机器现存、稳定的 Anchor 二进制。**不要把 `go run` 临时二进制作为长期 helper**，源码入口使用此命令时必须显式传稳定路径。搬动/删除二进制会破坏认证；目前没有修改 helper 路径的命令。
- `--approve` 必填显式为 true：批准长期配置管理、下次 `serve` 自动接管与正常停服恢复。只登记 SQLite/凭据，不立即改配置；当前服务中的接管需 `agent plan` + `agent apply`，或重启服务。
- 输出 Binding 元数据，`enabled:true`、`revision:1`，无秘密；同一规范化配置根只能登记一个 Binding，禁用亦保留所有权。Linux 配置根/文件须当前用户所有且不可被组/其他用户写入；符号链接、硬链接配置文件和 Windows reparse point 拒绝。
- 支持全局配置；已选择的 profile、WebSocket、未知/复杂的受管理 TOML 字段拒绝。不会执行 Agent、自定义脚本、修改登录凭据或启动安装流程。

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
- `model put` 不会自动刷新已写入的 Codex 目录；网关 all-compatible 授权按每次请求的当前模型/供应商配置判断，而 Codex 目录是接管时快照。新增模型/改元数据后须 `restore` → `plan` → `apply`（或正常停服再启动），再重启远端 Codex app-server。更新目录不等于让已有会话立即切换。
- 当前没有 Binding 删除、改路径或改适配器版本入口；默认模型与授权范围用下面的 set-models 显式修改。

### `agent set-models`

```text
anchor agent set-models ID --revision N --model DEFAULT --model-scope all-compatible|single
```

所选 Context 核心在线；ID、当前 revision≥1、默认公开模型和明确 scope 全部必填。先校验新策略及目录，再条件恢复旧配置/目录；恢复冲突或 I/O 失败会中止策略变更，旧 revision/授权不变（无冲突的配置字段可能已恢复）。成功保存新 default/scope/精确元数据版本、revision 加一，不旋转 token、不自动接管，输出 `{binding, management, apply_required:true}`。重新审核 plan 并 apply，或下次正常启动才写新配置。旧绑定扩权需显式执行此命令；缩到 single 后非默认模型立即失去授权，已接受请求不承诺取消。

### 远端 Codex app-server 主路径

桌面端 SSH 运行远端 Codex 时，Anchor 应配置在**远端**，CLI 操作目标也是远端已批准 CODEX_HOME。基础配置提供统一 Anchor provider、默认模型、模型目录与认证 helper；无需额外 profile。Codex 客户端可以从目录选其他公开模型，或者显式 `--model`/app-server model 参数选择；所有这些模型请求仍经 Anchor 路由到各自候选供应商。模型目录不是授权凭据，网关仍单独校验模型范围。

本实现不生成/选择 profile、不实现 Anchor 自己的 SSH Connector、不接管 Codex 所有网络出口、工具外网或登录状态。更高优先级配置/策略/命令参数可能改变供应商，inspect 不把配置写入当成运行连接证明。基础 provider/目录改动后重启远端 app-server；本轮隔离验证覆盖后台协议，不代表桌面模型选择器 UI 已验收。

### `agent token`（认证 helper）

```text
anchor agent token ID
```

本机操作，拒绝 `--context`；无需运行核心。成功 stdout 是**仅网关用途的原始秘密 token 加换行，不是 JSON**，供 Codex 命令认证直接读取；不要人工打印、管道到日志或分享。ID 必须是 enabled Binding。Binding 凭据在受保护 SQLite 中以秘密和鉴权摘要独立存储，非加密；普通查询、plan、日志和配置文件均不含它。只能请求 Binding 授权范围内的公开模型（single 或动态 all-compatible），不能访问管理面。disable 后鉴权与 helper 输出均拒绝；重新 enable 会恢复同一绑定授权。

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

实际模型调用使用鉴权的 HTTP 数据面（`GET /v1/models`、`POST /v1/responses`），目前没有专用 CLI 推理命令；不要用拥有者管理令牌请求模型网关。

## 10. 功能的 CLI 覆盖与缺口

“大部分功能有指令”指用户可操作的管理能力，不要求每个内部函数、后台事件或 HTTP 推理请求各造一个子命令。新增用户操作时应优先补 CLI；例外在此表解释。

| 功能 | 当前入口 / 状态 | 说明 |
| --- | --- | --- |
| 初始化、启动、版本/帮助 | init / serve / version / help | 已提供 CLI；退出靠信号，尚无后台服务管理 |
| 本机连接、默认选择、检查 | context 五个子命令 | 已提供 CLI；删除、重绑定未实现 |
| 状态查询 | status | 已提供 CLI |
| 供应商、模型新增/更新/查询 | provider / model 的 add、put、list | 已提供 CLI；删除未实现 |
| 授权签发/列表/撤销 | access 的 grant、list、revoke | 已提供 CLI；管理令牌选择未实现；Binding 级凭据随 agent 自动管理 |
| 模型推理与模型目录数据面 | HTTP 网关 | 数据面供 Agent 调用，不强制新增 CLI 推理入口；无专用调用命令 |
| SQLite、限流、路由、日志脱敏 | 内部机制 | 无需一一变成命令；暂无独立日志订阅/用量查询 CLI |
| SSH 连接与远程凭据 | 未实现 | 用户管理能力，后续实现需同步提供 CLI |
| Agent 绑定/接管、条件恢复/recover | agent 管理子命令及 recover | 已提供 Codex 全局配置；模型元数据导入/目录生成与默认模型/范围更新已提供；其他适配器、profile、绑定删除/路径更新仍缺失 |
| Chat/Anthropic 转换 | 未实现 | 后续协议能力及其用户配置需同步核对 CLI 覆盖 |
| 分发、系统服务、升级 | 发行脚本与外部安装脚本；无产品 install/update 子命令 | 单文件打包及公开 Release 下载/校验安装脚本已实现（见 [安装说明](../release/install.md)）；当前无公开 Release。脚本替换需显式授权，不停服/迁移；桌面安装器、自启动、自动更新未实现 |

未实现项目不是可用指令；新增/删除命令及其他契约变化必须同步本文件与指令变更历史。
