# Anchor

本地 Agent 模型中转与管理工具。管理用户选择的 Codex、CC、DSH 等平台，
提供供应商配置、模型映射、自动路由和协议转换，不提供自有 Agent 执行引擎。

## 当前阶段

已开始实现首个开发里程碑：Go 无头核心与 CLI、本机 Context、独立管理 / 网关鉴权、
SQLite 供应商与模型配置，以及 Responses 同协议代理。当前版本为 `0.1.0-dev`，
不等于整体初版已完成；SSH、Agent 托管 / 恢复和跨协议转换仍待实现。

必须支持 Linux / Windows；Windows 运行验收尚未完成。未来注重桌面端，
不在本阶段开发 Web 或桌面界面。

## 开发运行

需要 Go 1.25.0 或更新版本。当前机器的开发入口会自动使用资源保护脚本。

```bash
./scripts/anchor-dev.sh init
./scripts/anchor-dev.sh serve
```

在另一个终端执行：

```bash
./scripts/anchor-dev.sh context add local --local
./scripts/anchor-dev.sh context use local
./scripts/anchor-dev.sh status --json
```

服务前台运行，不由管理命令退出触发停服；关闭 serve 终端可能停止服务。
完整配置命令、实际 API、安全限制和未实现能力见
[首个实施里程碑](docs/design/implementation.md)。
凭据当前依赖用户文件权限 / Windows ACL，**不是静态加密**。

## 打包与发行

已提供 [CLI 发行流程](docs/release/README.md)：检查、Linux/Windows amd64 便携预发行包、校验及包内程序验收。入口为 `scripts/release.sh`，不会自动提交、打标签或上传。
当前缺少项目发行许可及 Windows 真机验收；公开候选必须来自干净版本提交，现阶段需显式按内部测试包构建，不能把打包成功视为公开发行完成。

已配置 [GitHub Actions 自动打包与草稿发行](docs/release/github-actions.md)：手动运行生成内部包；推送预发行标签后构建并创建草稿 Release。配置已纳入项目源码；实际推送和托管运行结果见任务记录，不自动公开发布。

## CLI 指令与维护

- [最新 CLI 指令大全](docs/cli/commands.md)：唯一当前指令目录，含参数、默认值、作用域、示例与功能覆盖缺口。
- [CLI 指令变更历史](docs/cli/changes.md)：记录指令增删改、参数与行为变化，以及兼容/迁移方式。
- [项目 CLI 文档 skill](.agents/skills/anchor-cli-docs/SKILL.md)：开发用户可操作功能和修改 CLI 时使用；项目约束见 [AGENTS.md](AGENTS.md)。

大部分用户可操作功能应有 CLI。每次实际 CLI 契约变化必须同时更新帮助、相关测试、最新大全和变更历史；设计计划不算已实现命令。

## 设计文档

- [初版技术设计（总览）](docs/design/architecture.md)：会话确认的范围、总体架构、
  代理与托管职责、生命周期、远程管理、验收目标和待定技术决策。
- [CLI 连接与实例管理](docs/design/cli-connections.md)：本机连接、SSH 远程管理、
  context 切换、认证、安全边界，以及未来桌面端复用契约。

- [托管层与 Agent 接入契约](docs/design/agent-management-layer.md)：内置适配器、
  声明式接入、绑定对象，以及常驻服务的自动接管和条件恢复。

- [桌面前端技术设计](docs/design/frontend.md)：UI 基调、React / TypeScript / shadcn/ui 选型、
  Wails 桌面容器与 Linux / Windows 跨平台目标；前后端对接另议。
