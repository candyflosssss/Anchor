# Anchor CLI 指令变更历史

当前可用指令以 [最新指令大全](commands.md) 为准。本文件记录实际契约变化，按时间追加，不把未来计划写成已发生变更，不改写旧条目。

命令、别名、参数、默认值、作用域、输出和行为变化均应记录。每条包含日期、适用版本/未发布状态、旧→新差异、兼容与迁移方式及实际验证。纯内部重构或仅修正文档措辞且不改变 CLI 契约时无需新增虚假变更。

## 2026-10-10 — 建立当前 CLI 文档基线（0.1.0-dev，未发布）

- 类型：基线收录；本轮仅登记既有实现，没有新增、删除或改变可执行指令。
- 指令：help、version、init、serve、context add/use/list/inspect/test、status、provider add/put/list、model add/put/list、access grant/list/revoke，共 19 个规范路径。
- 参数：登记全局 data-dir/context/json；初始化监听地址；Context 的 local；供应商 id/base-url/protocol/timeout/max-concurrent/disabled/revision/secret-stdin；模型 id/candidate/display-name/tools/streaming/revision；授权 label/scope/ttl/show-once。
- 旧 → 新：原先指令分散在 help 和实施示例中；现在以 commands.md 汇总当前完整契约，明确参数默认值、完整替换更新、本机/Context 作用域、秘密输出与未实现能力。实现行为不变。
- 兼容与迁移：没有运行时破坏性变化，不需要数据或配置迁移；README/实施说明转而指向唯一最新大全。
- 验证：按 CLI 路由/参数解析、state/client 实现及 main 退出逻辑核对；本轮不启动核心、不调用真实上游、不重跑 Go 功能测试。skill 元数据、文档链接及命令覆盖检查结果见本轮任务更新记录。

## 2026-10-10 — 发行构建返回实际版本（源码 0.1.0-dev；预发行包使用注入版本）

- 类型：修改行为；没有新增或删除 anchor 命令/参数。
- 指令与参数：`anchor version`；发行工具独立的 `scripts/release.sh --version` 不属于产品 CLI 参数。
- 旧 → 新：原先版本为源码固定的 0.1.0-dev；现在保留源码默认值，但发行编译通过 ldflags 注入预发行版本，version 和管理实例接口使用一致的实际版本。
- 兼容与迁移：JSON 仍只有 version 字段，退出码和无需初始化的行为不变；依赖固定字面量 0.1.0-dev 的脚本应改为读取实际值，无数据迁移。
- 验证：新增无初始化 version 测试，包内程序烟测要求版本与发行参数一致；本轮实际执行结果见对应任务 release 记录。

## 2026-10-10 — 托管层与 Codex 全局配置 v1（0.1.0-dev，未发布）

- 类型：新增 / 修改。
- 指令与参数：新增 agent add/list/inspect/plan/apply/restore/enable/disable/token 和 recover；add 使用 id/adapter/codex-home/model/approve/anchor-executable，变更动作要求 revision，apply 另要求 fingerprint，recover 可按 id 限定；最新大全共 29 路径。
- 旧 → 新：之前不托管 Agent；现在显式批准 Binding 后，下次 serve 自动接管全局 Codex 配置，正常停止条件恢复，崩溃后可独立 recover。status 新增逐 Binding 状态，实际连接仍为 unknown。普通输出仍为 JSON，但 agent token 专供认证 helper，stdout 是原始秘密文本；不会进入普通查询、计划或日志。Responses 允许客户端执行的 custom 工具同协议透传（不支持服务端工具）。
- 兼容与迁移：schema 1 → 2 自动保留原有数据/身份，但旧二进制拒绝 schema 2，回退需先停止核心并使用升级前受保护的完整状态备份。已发行旧测试包不包含新命令；本轮未打包/发布。Codex 只支持全局配置，需工具/流式公开模型和稳定 helper 路径；不能把 go run 临时程序用于长期托管，未生成模型目录或验证真实付费供应商。普通 access grant 行为不变。
- 验证：新增隔离字段恢复/冲突/崩溃/脱敏、迁移/Binding 凭据、管理 API 和 CLI 测试；受 resource-guard 保护的真实 CLI 生命周期烟测通过，隔离 codex-cli 0.159.3 对回环模拟上游的 Responses 文本请求通过。最终测试命令与平台边界见本任务更新记录；不宣称 Windows 原生运行或真实工具执行验收。

## 2026-10-10 — Codex 基础配置模型目录与多模型授权（0.1.0-dev，未发布）

- 类型：新增 / 修改。
- 指令与参数：`model add/put` 增加成组参数 `--codex-catalog FILE --codex-model SOURCE --codex-version X.Y.Z`，从本地、版本匹配的完整目录导入指定模型元数据；`agent add` 增加 `--model-scope all-compatible|single`，默认 `all-compatible`；新增 `agent set-models ID --revision N --model DEFAULT --model-scope all-compatible|single`。最新大全共 30 个规范路径。
- 旧 → 新：原先 Binding 的 model 同时充当默认值和唯一授权模型；现在 default 与授权范围分离。all-compatible 动态授权工具/流式、同精确 Codex 版本有效元数据、至少一个 enabled 候选供应商的公开模型；默认模型只决定初始选择，不代表供应商健康或任意模型兼容。基础配置接管按同一规则生成绑定专有模型目录，公开 plan/fingerprint 包含目录与策略；恢复条件清理未改动且不再被基础配置/独立 profile 引用的自有目录。Responses 同协议透传补充仅含 function/custom 的 namespace 和 `tool_search execution=client`，仍拒绝服务端工具及嵌套 namespace。
- 兼容与迁移：旧 scope 省略的 Binding 仍为 single，不自动扩权。显式 `set-models` 先安全恢复旧接管，遇冲突不变更策略，成功后递增 revision、保留 token 并等待重新 plan/apply。`model put` 仍为完整替换，省略元数据参数将清除原元数据。目录是接管时快照，网关权限动态判断；模型配置更新后须 restore → plan → apply（或正常停服/启动）刷新目录，并重启远端 Codex app-server。只托管批准的 CODEX_HOME 基础配置，不生成/强制启用额外 profile；不自动获取目录、探测安装版本或推断真实上游能力。未打包/发布，不修改正在使用的 Agent 配置。
- 验证：最终全包 `go test -race -p 1 -count=1 -timeout=240s ./...` 通过；隔离 Codex CLI 0.159.3 和实际 app-server 的 model/list、默认/手动切换模型、set-models、停止/崩溃恢复与冲突场景使用回环模拟上游验收，具体最终结果见本任务更新记录。Windows 仅交叉编译；不宣称桌面 UI、真实 SSH 传输、真实付费供应商或完整工具执行已验收。
