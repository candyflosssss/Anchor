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
