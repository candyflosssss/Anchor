# Anchor 托管层与 Agent 接入契约

- 日期：2026-10-10
- 状态：部分落地；托管/恢复引擎与 Codex 全局配置 v1 已实现，其余仍是设计
- 适用范围：无头常驻服务与 CLI；Linux / Windows；优先 Codex、CC（Claude Code）、DSH。
- 不提供：Agent 执行引擎、任意脚本执行平台、系统全局代理、多设备同步。

> 本文描述整体目标，声明式描述示例仍未实现。当前已支持的 CLI 以 [指令大全](../cli/commands.md) 为准，已落地边界见 [实施说明](implementation.md#7-托管层与-codex-配置闭环)。Agent 配置字段须按安装版本验证，不以示例代替真实兼容性测试。

## 1. 总体分层

```text
CLI / 未来桌面端
        |
        v
管理 API + 核心服务生命周期协调器
        |
        +-- 托管层
        |     +-- 内置平台适配器：Codex / CC / DSH
        |     +-- 声明式配置适配器：JSON / YAML / TOML
        |     +-- 统一变更、恢复与凭据落地引擎
        |
        +-- 代理层
              +-- 客户端协议入口
              +-- 公开模型目录 / 每个绑定的鉴权与路由策略
              +-- 供应商路由
              +-- 上游协议转换、流式事件与工具调用
```

代理层不识别 `config.toml`、`settings.json` 或 Cordis 配置文件；托管层不实现请求协议转换。二者经绑定信息连接，不相互读写内部数据库表。

托管层运行在 Agent 所在实例，拥有目标用户批准的配置写入权限。控制电脑通过管理 API 发起操作，路径与环境变量按目标机器解析，而非控制电脑解析。

## 2. 两种“协议”不能混淆

1. **模型请求协议**：Responses、Chat Completions、Anthropic Messages 等，属于代理层。
2. **托管接入契约**：怎样发现、检查、修改、验证和恢复某个 Agent 配置，属于托管层。

支持未知 Agent 的前提是它允许配置 API 地址 / 认证，且其模型请求协议已被 Anchor 支持。新增字段描述不会自动让代理支持未知私有网络协议。

对没有可配置地址、只能使用官方认证、或依赖无法转换的特殊接口的 Agent，应报告不支持 / 部分支持，不承诺通过配置文件万能接管。

## 3. Binding：连接两层的稳定对象

每次用户选择托管一个具体配置目标时建立 binding，而不是每个产品名称只能有一个条目。

```text
Binding
  id                         稳定绑定 ID
  adapter_id / version       适配器身份与版本
  target                     已批准的目标用户、配置根、profile / 项目范围
  enabled                    用户是否选择长期托管
  gateway_binding_id         对应代理授权与路由范围
  selected_public_model      默认公开模型或模型选择策略
  lifecycle_policy           启动接管、正常停止恢复
  revision                   并发修改保护版本
  status                     等待 / 已接管 / 覆盖 / 冲突 / 恢复待处理
```

例如同一用户的 Codex 全局配置和一个明确批准的独立 profile 可以是不同目标。多个 binding 若修改同一文件同一字段，必须检测重叠并拒绝冲突，不能依赖“最后写入者获胜”。

代理向托管层提供类型化的 `GatewayBinding`：

- 绑定 ID、代理是否就绪、支持的客户端协议与能力；
- 按协议定义的 base URL，而不是由描述文件自行拼接任意地址；
- 公开模型 ID、默认模型与目录描述；
- 绑定级网关凭据的秘密引用，以及适配器获批时可使用的安全落地方法；
- 配置 generation / revision。

GatewayBinding 不包含供应商原始密钥或管理令牌。不同绑定可有独立网关凭据，便于撤销、识别来源和应用路由策略。其地址永远来自目标实例，不来自控制电脑。

每种客户端可能自行追加不同版本路径；适配器负责声明 URL 约定并测试，不能对所有 Agent 统一硬拼 `/v1`。

## 4. 两级扩展，而不是第一版插件运行时

### 4.1 内置适配器

用于主流平台的复杂情况：多层配置优先级、版本迁移、模型目录文件、特定认证方式、不同 profile，以及需要语义判断的恢复。

首批平台：Codex、CC、DSH。共同实现拟定的生命周期接口：

```text
Discover(target)                 -> 检测版本 / 目标配置候选
Inspect(target)                  -> 有效连接配置、覆盖来源和支持状态
Plan(target, gatewayBinding)     -> 只生成结构化修改计划
Verify(target, appliedPlan)      -> 配置层验证；可选请求层诊断
```

Adapter 不直接任意改盘。计划、应用、撤销、恢复由统一引擎执行。确实需要额外逻辑时，也必须归约成受管理操作 / 工件，不能绕过日志与备份。

`Inspect` 无法可靠确定有效配置时返回 unknown / partially supported；不能把“文件里有 Anchor 地址”直接当成“实际流量一定被接管”。

### 4.2 声明式适配器

供非主流 Agent 接入，描述受限事实：

- schema 版本、适配器 ID、描述与适用平台；
- 必须由用户批准的目标配置根和文件；
- 配置格式；
- 有限的字段操作：`set`、明确的 `remove`、带前置条件的字段级 `merge`；
- 值的类型化绑定：网关地址、公开模型 ID、绑定级认证值；
- 恢复与加载语义；
- 要求的客户端协议 / 能力。

不允许：Shell、eval、任意脚本、自动下载安装、任意网络请求、系统服务修改、全盘路径匹配、导出供应商密钥、未批准配置根以外的文件写入。

后续确实需要代码插件时应另设计信任、权限、隔离和版本契约，初版不预留一个默认可以执行不可信代码的“通用钩子”。

## 5. 声明式描述示例

以下为虚构 Agent 示例，不是 Codex / CC / DSH 的真实配置：

```yaml
schema_version: 1
id: custom-example
name: Example Agent
platforms: [linux, windows]
requires:
  client_protocol: chat_completions
  capabilities: [tool_calling]
reload_policy: restart_required
files:
  - target_ref: approved_main_config
    format: json
    operations:
      - op: set
        path: [api, base_url]
        value_from: binding.gateway.chat_base_url
      - op: set
        path: [api, token]
        value_from: binding.auth.gateway_token
      - op: set
        path: [model]
        value_from: binding.models.default_id
```

解释：

- `approved_main_config` 在实例本机登记时由用户批准，不是描述任意指定的绝对路径。
- `path` 是字段段数组，不是执行表达式；含点的键与嵌套路径不得混淆。
- `value_from` 只解析白名单里的类型化数据，不是自由模板语言。
- `gateway_token` 是仅网关调用的秘密；只在批准的写入阶段取出，不进入普通计划预览 / 日志。
- 静态模型映射、默认模型与平台模型选择能力分开；不支持动态模型目录的 Agent 不伪装成支持。
- 声明式首版不覆盖任意数组查询、YAML 特殊标签、跨文件引用或所有 TOML 语义；遇到复杂文件返回需内置适配器 / 人工配置，而不是破坏性重新序列化。

路径审批要校验目标用户、规范化路径、符号链接 / Windows reparse point 和父目录权限。批准 descriptor 不等于批准其中所有目标路径。

## 6. 统一变更与恢复引擎

### 6.1 变更计划

每个受管理字段 / 工件记录：

- 原始值及是否存在；
- 计划写入值；
- 当前观察值与文件 fingerprint；
- binding ID、适配器版本、generation、操作 ID；
- 脱敏差异、前置条件、备份和恢复记录引用；
- 实际平台的权限、ACL 等保留要求。

原文件的恢复材料可能含用户原有秘密，应以目标用户受保护权限存储，不放入日志或普通配置导出。

### 6.2 应用

```text
验证用户选择与目标授权
 -> 取得实例 / 路径锁
 -> 读取并校验当前 revision / 文件状态
 -> 持久化恢复日志与安全备份
 -> 应用操作和必要的模型目录等工件
 -> 校验结果
 -> 记录已应用状态
```

同一字段只能有一个托管所有者；多文件变更使用日志记录分步进度和补偿恢复，不宣称文件和数据库之间天然原子提交。

文件更新尽量使用保留权限的原子替换，并在发布前再次检查 fingerprint。Anchor 锁只能协调 Anchor 自身；不能保证阻止未遵循锁协议的外部编辑器竞争。检测到已知并发变更则拒绝发布，无法安全处理的复杂文件不强行接管。

### 6.3 条件恢复

- 当前值仍等于 Anchor 写入值：恢复原值 / 原先不存在状态。
- 当前值已经等于原值：视为恢复完成。
- 当前值被用户或其他工具改成第三个值：保留并报告冲突。
- 仅删除确认由该 binding 创建、且未被用户修改 / 引用的工件；不能删除用户自有 provider 或目录。
- 恢复时使用接管时记录的适配器版本 / 原操作计划，不能由一个升级后不同规则猜测过去改了什么。

完整备份用于人工兜底，不作为日常无条件整文件覆盖方法。默认不常驻强制纠正配置；检测到手动修改时显示 overridden / conflict，不与编辑器争抢。

## 7. 核心服务生命周期

### 7.1 启动

```text
取得实例锁
 -> 读取并核对未完成记录
 -> 启动代理，确认本机监听与绑定能力就绪
 -> 为 enabled binding 生成 / 校验计划
 -> 自动接管
 -> 发布逐 binding 状态
```

失败时不把 Agent 指向一个未启动的代理。重启或崩溃后重新启动时，不把上次 Anchor 写入值当成新原值；沿用并核对原恢复记录。

某 binding 接管失败时保留其他已就绪 binding，但服务状态必须显式反映部分接管失败，CLI 不显示全部成功。管理服务可用不等于所有 Agent 已接管。

### 7.2 正常停止

```text
冻结新的托管修改
 -> 对托管配置执行条件恢复并报告冲突
 -> 提示已有 Agent 会话可能仍使用旧连接
 -> 完成 / 有限等待已接受请求
 -> 关闭代理与管理服务
```

只恢复文件不能保证已经运行的进程改用原供应商；不能无缝迁移会话。初版按适配器标注 reload / restart required，必要时要求用户结束或重启 Agent 会话。

恢复失败或有冲突时，不静默将状态标记为 fully restored；保留恢复材料并明确提示仍可能指向 Anchor。无人值守的停服超时与强制停止要记录未完成状态，而非无限等待。

### 7.3 异常退出与服务重启

- 强杀 / 断电不保证立即恢复，不能依赖退出回调作为唯一保护。
- 首版用持久恢复日志和独立本机恢复命令，不加第二个常驻看门狗。
- 自动重启沿用同一托管 generation；实际停止与重启不混淆。
- 管理 CLI 退出、SSH 隧道断开或未来桌面窗口关闭，不触发配置恢复；只有目标核心服务的停服 / 取消 binding 托管才触发。

## 8. 三个内置适配器的关注点

| 平台 | 配置关注点 | 不应假定 |
| --- | --- | --- |
| Codex | 自定义 provider、连接认证、模型目录、profile、有效配置覆盖来源 | 用户级配置能覆盖 CLI / 所有项目配置；修改文件能立即切换已有会话 |
| CC | settings 中环境配置、网关 URL、认证与模型映射、用户/项目/命令行层级 | 修改 settings 能改变父 shell 的环境；所有认证模式都能随意替换 |
| DSH | 安装版本、LLM 适配器、profile / Cordis patch、凭据来源与模型目录 | 所有版本都使用同一 settings.yaml；只有一种请求协议 |

持久环境配置与进程环境注入不是同一能力。声明式初版支持“配置文件里的 env 字段”，不向用户 shell profile 全局写入代理变量，也不声称能修改已运行进程的环境。

若某 Agent 只能通过启动参数 / 环境变量接入：报告 manual / launch-scoped 支持；可后续提供可选启动包装，但不得为了“自动接管”偷偷替换二进制、alias 或全局 PATH。主要平台的常规使用目标仍是直接启动其原命令。

版本未知、字段不支持、有效配置被覆盖时应返回结构化诊断，要求用户确认目标范围；不扩大到全部项目或全部系统用户。

## 9. CLI 与 API 体验（拟定）

```text
anchor agent discover
anchor agent manage codex --preview
anchor agent manage codex
anchor agent manage dsh
anchor agent list
anchor agent inspect codex
anchor agent unmanage codex
anchor adapter import ./custom-agent.yaml --preview
anchor adapter import ./custom-agent.yaml
anchor recover
```

- `manage` 保存 enabled 选择；核心正在运行时可立即生成计划并接管，未运行时为等待状态，下一次核心启动自动接管。
- `unmanage` 恢复该 binding 管理字段，不停止代理或其他 binding。
- 实际命令应支持具体 binding / target，产品名称快捷写法仅在目标唯一时有效；不隐式修改所有 profile。
- 远程命令遵循 `--context` 和写入确认规则；导入 descriptor 后在目标实例审批路径。导入本机文件不代表使用本机路径作为远端配置路径。
- `recover` 是核心未运行时可用的本机恢复入口；远端服务未运行时不能假装通过失效管理 API 进行恢复，需目标用户本机 / SSH 登录执行。
- 描述导入只登记规则并审批，不自动启用所有匹配 Agent。
- 预览 / 列表 / inspect 全部脱敏，不在命令行参数中接收秘密。
- 未来桌面使用相同的 discover / plan / approve / apply / inspect API，不自己编辑平台配置。

## 10. 能力与状态报告

分别报告：

1. descriptor / adapter 是否兼容；
2. 用户是否已选择长期托管；
3. 配置是否已写入并验证；
4. 是否存在配置覆盖 / 冲突；
5. Agent 是否需要重新加载 / 重启；
6. 是否观察到该 binding 的真实网关请求。

“配置已接管”与“实际请求已经过 Anchor”是不同证据。没有请求时不能凭空显示链路验证成功；真实请求验证优先由用户正常启动 Agent 触发，不自动启动高权限任务。

## 11. 验收计划（尚未执行）

- Codex 首先完成配置接管、模型选择、请求、工具调用与恢复闭环；CC / DSH 各按实际安装版本独立验证。
- 同一 adapter 可以注册多个非重叠目标，重叠字段被拒绝。
- 虚构 JSON Agent 不改代理核心即可用受限 descriptor 接入已支持协议。
- Linux / Windows 的路径、权限、原子文件更新与无头凭据读取分别验证。
- 恶意路径、符号链接跳出、执行钩子、未知模板和秘密导出被拒绝。
- 用户运行中改配置、服务重启、强杀后再次启动、部分文件写入失败都保留可恢复状态。
- 代理初始化失败不修改配置，管理连接断开不触发恢复，已有 Agent 会话不被宣称无缝切换。
- 供应商 / 管理密钥不进入 Agent 配置；绑定网关凭据不会越权调用管理 API。
- descriptor 版本升级不会破坏旧恢复记录；动态模型目录和认证落地按 adapter 能力声明。

## 12. 外部依据

以下用于说明需要平台差异适配，不代表 Anchor 已完成兼容：

- Codex 自定义 provider、连接与认证：[OpenAI 官方高级配置](https://developers.openai.com/codex/config-advanced/)。
- Codex 配置层级：[OpenAI 官方基础配置](https://developers.openai.com/codex/config-basic/)。
- CC 网关 URL、认证、协议要求：[Claude Code 官方 LLM gateway 文档](https://code.claude.com/docs/en/llm-gateway)。
- CC settings 与环境变量层级：[Claude Code 官方 settings 文档](https://code.claude.com/docs/en/settings)。
- DSH profile / Cordis patch：[上游 settings README](https://github.com/deepseek-ai/deepseek-harness/blob/master/packages/settings/settings/README.md)。
- DSH DeepSeek 适配器、baseURL 与模型能力：[上游 llm-deepseek README](https://github.com/deepseek-ai/deepseek-harness/blob/master/packages/llm/llm-deepseek/README.zh.md)。

具体实现必须绑定实际安装版本和测试样本；上游 master 和在线文档会继续变化。


## 13. 当前落地补充：Codex 基础配置、多模型与目录（2026-10-10）

本节是实现状态，不把上文通用适配器/profile 等设计全算成已完成。操作入口以 [唯一最新 CLI 大全](../cli/commands.md#8-agent-托管与独立恢复) 为准。

- 桌面端 SSH 使用远端 app-server 时，托管远端已批准 CODEX_HOME 基础配置；Anchor 是统一 provider，profile 不是本路径必需的“消费者”或模型组，不生成额外 profile。
- Binding 分开记录 default/model_scope/CodexVersion：新 CLI add 默认 all-compatible，旧记录保持 single；模型目录表示选择能力，Binding 凭据和网关检查才表示权限。可选模型动态限定到工具/流式、同精确版本元数据与 enabled 供应商候选。
- `model add/put` 显式导入本地、精确版本目录的完整模型条目；操作者负责与实际模型能力对应。目录生成保留真实能力和指令，只改公开 slug/可选展示名、选取与 HTTP 传输字段，禁用 upgrade 自动越到未授权名称。
- 接管生成绑定专有 JSON 目录，先持久恢复材料再写目录/配置；保留原目录指针，不覆盖已有同名文件。停止/recover 只移除内容仍相同且基础配置/独立 profile 均不再引用的自有目录；用户修改和引用均保留。
- 目录是接管快照，网关权限是当前配置动态判断；模型变动后显式恢复/重建目录，再重启远端 app-server。`set-models` 先恢复再更新 revision，恢复冲突不改授权，token 不旋转。
- 版本 0.159.3 的隔离真实 CLI 与 app-server 验证覆盖目录列表及默认/显式模型请求，只有回环模拟上游。未验收桌面 UI、真实供应商、所有工具闭环、Windows 原生或全部网络出口接管。

## 14. 本机发现与桌面登记落地（2026-10-12，源码未发布）

发现只检查已知用户配置根（含核心环境 CODEX_HOME），去重并报告安全/适配/缓存状态，不遍历磁盘，不执行客户端，不据目录宣称程序已安装。其他产品仅显示无适配器。Codex 读取有界的完整本地 models_cache.json，保留 client_version 和能力条目，批准前要求版本及实际能力核对。

Catalog 快照和 public alias → 源 slug 选择归 Binding；空源为排除。供应商 Model 不承担每个 Agent 目录/安装版本信息。核心应用服务同时供 CLI 和薄 Wails Bridge 使用，文件修改仅走既有 Engine。在线登记立即应用；运行中映射变更可显式 sync，网关授权始终按当前模型策略；重启核验过期目录后条件恢复重建。外部覆盖、不支持 profile、目录引用或修改冲突均保留并报告。停止恢复，退出窗口不恢复，更新配置后重启 Agent。

只读发现、明确批准登记、显式同步已提供 CLI 和 GUI；详见指令大全 §8。真实核心/Bridge 测试使用隔离合成目录，Mock UI 检查不等于 Windows 原生或真实供应商验收。
