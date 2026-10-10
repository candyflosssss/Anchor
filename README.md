# Anchor

本地 Agent 模型中转与管理工具。管理用户选择的 Codex、CC、DSH 等平台，
提供供应商配置、模型映射、自动路由和协议转换，不提供自有 Agent 执行引擎。

## 当前阶段

已开始实现首个开发里程碑：Go 无头核心与 CLI、本机 Context、独立管理 / 网关鉴权、
SQLite 供应商与模型配置、Responses 同协议代理，以及托管层/条件恢复、Codex 基础配置与模型目录、多模型绑定授权。当前版本为 `0.1.0-dev`，
不等于整体初版已完成；SSH、其他 Agent 适配器、Codex profile和跨协议转换仍待实现。

必须支持 Linux / Windows；Windows 公开候选必须通过原生运行门禁。未来注重桌面端，
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
Codex 托管使用稳定 Anchor 二进制及明确批准的配置根，登记后先审核计划再接管。操作契约见 [最新 CLI 指令大全](docs/cli/commands.md#8-agent-托管与独立恢复)。已公开的 Linux `0.1.0-alpha.3` 包含上述能力；验收结果见任务记录，旧测试包不包含它。

桌面端通过 SSH 使用远端 Codex 的主路径：在远端批准 CODEX_HOME 基础配置 → Anchor provider + 默认模型 + 版本匹配模型目录 → 用户手动选择其他授权模型 → Anchor 路由实际供应商。新 CLI 绑定默认 `all-compatible`，旧绑定保留单模型权限；无需额外 profile。目录元数据需操作者显式导入并核对真实上游能力，修改目录后重启远端 app-server。Anchor 自己的 SSH Connector 和桌面 UI 验收不在本轮范围。

完整配置命令、实际 API、安全限制和未实现能力见
[首个实施里程碑](docs/design/implementation.md)。
凭据当前依赖用户文件权限 / Windows ACL，**不是静态加密**。

## 打包与发行

已提供 [一键安装脚本](docs/release/install.md)：识别 Linux/Windows AMD64、下载公开 Release 的单文件并校验 SHA256，默认安装到用户目录，不需要 sudo/管理员权限、不改 Codex 配置。可固定版本，已有程序必须显式允许替换；未设置 PATH 时只提示，不编辑 shell 配置。**Linux AMD64 `0.1.0-alpha.3` 已公开发布，并实际验证远程一键安装成功；后续双平台候选须通过 Windows 原生门禁，具体公开版本见 Releases**。不从 Actions artifact 或草稿偷偷安装。

**运行只需要一个文件**：Linux 的 `anchor` 或 Windows 的 `anchor.exe`，核心、CLI 和认证 helper 都在其中；不依赖旁边的文档或资源文件。配置/数据库仍独立存放于用户状态目录，首次使用仍需 init 和供应商/模型配置。

已提供 [CLI 发行流程](docs/release/README.md)：输出两个平台各自的单文件可执行程序，并保留含文档/依赖声明的可选归档；进行校验与程序验收。入口为 `scripts/release.sh`，不会自动提交、打标签或上传。
项目采用 **AGPL-3.0-only**：允许商用，分发及修改版网络服务的源码义务见 [许可说明](docs/license.md) 和 [LICENSE](LICENSE)。公开候选必须来自干净版本提交，并附对应源码（含 vendored 依赖）及声明；双平台候选必须通过 Windows 原生 ACL/锁、初始化、中文供应商、模拟网关和正常退出检查，不能把打包成功视为公开发行完成。

已配置 [GitHub Actions 自动打包与草稿发行](docs/release/github-actions.md)：手动运行生成内部测试程序，分别提供每个平台仅含一个程序的 artifact（GitHub 下载外层仍为 ZIP）；推送预发行标签后构建两平台公开候选（含对应源码），经 Windows 原生验收与安装器矩阵通过后创建草稿 Release。配置已纳入项目源码；实际推送和托管运行结果见任务记录，不自动公开发布。

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
