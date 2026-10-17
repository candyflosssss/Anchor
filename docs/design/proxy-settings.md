# 出站代理（proxy-v1）

独立于供应商协议适配。管理 API `/control/v1/proxies` 返回脱敏配置、供应商及 Key 的绑定和 mihomo 状态；`POST /control/v1/proxies/action` 为唯一写入入口，要求实例身份、明确批准、workspace revision。CLI 与桌面使用同一类型与校验。

## 选择规则

Key 指定代理 > 供应商默认代理；Key `direct` 强制直连、`inherit` 继承。供应商默认 `direct`。没有绑定不会读取环境代理。代理失效、节点无效或内核缺失报错，绝不降级直连。适用于推理、流式对话、测试、模型发现和供应商账户查询；公共模型元数据下载、连接 Anchor 的 SSH 隧道不使用供应商代理。

## 配置与秘密

HTTP、HTTPS 和 SOCKS5 以 URL 导入，支持认证；mihomo 导入单节点 YAML，或从 Clash YAML 的 `proxies` 列表按 `node_name` 选取一个节点。API 只返回 ID、名称、类型、revision；认证 URL/节点 YAML 单独存储，列表不回显。编辑时 secret 留空保持。只提取节点，不执行用户提供的监听器、规则、DNS、TUN、控制器或脚本。暂不包含订阅定时刷新、URI/Base64 订阅、测速或全局系统代理。

## 生命周期与分发

固定版本 mihomo v1.19.32，发布前下载并校验 Windows/Linux amd64 compatible 构件，嵌入对应平台二进制。源码 checkout 未准备 payload 时外部代理仍可用，mihomo 显示未内置。核心按需为使用中的节点启动独立子进程（最多 16 个活动/退役实例），仅监听随机 loopback 端口并启用随机认证；没有外部控制 API、不写系统代理。配置代际隔离连接池，旧代在响应 Body 关闭后退出，保证修改不会切断活动 SSE；核心退出统一回收。进程日志不转发（避免秘密泄漏）。配置和解压内核在实例私有目录。

删除在用的代理返回 referenced；供应商/Key 重命名、删除遵循数据库外键，复制供应商同时复制默认代理，复制 Key 时复制覆盖。列表和更改均不访问真实上游。Windows 本机进程/窗口和安装包仍需 Windows 原生验收；交叉编译不等价于原生验证。

## DTO

规范 Go DTO 位于 `internal/state/proxies.go`，桌面 `internal/desktop/contract.go` 使用类型别名，TypeScript 通过现有生成器生成。绑定模式 `direct|proxy`（供应商）和 `inherit|direct|proxy`（Key）。`ProxyAction.action` 为 `put|delete|bind`。`put` 的 `secret` 可选，`bind` 空 `key_id` 表示供应商默认。`ProxyWorkspace` 包含 `revision/profiles/bindings/targets/engine`。远端旧核心返回 unsupported，不用 Mock 代替。

## 升级与构建

状态库从 schema8 自动迁移至 schema9；请先备份并同步升级客户端与核心，旧核心不能再打开升级后的库。当前是源码实现，未生成或发布新安装包。独立源码构建先运行 `python3 scripts/prepare-mihomo.py --target linux-amd64`（Windows 改为 `windows-amd64`）；已有桌面和 CLI 发行脚本会自动准备对应平台并校验固定 SHA256。

## 本轮验证边界

- Linux 固定 mihomo 构件已实际启动：回环 HTTP 节点转发、拒绝未认证请求、异常退出后重建和核心关闭回收均通过；另外覆盖外部 HTTP/SOCKS5、HTTPS CONNECT 和旧代连接池排空。
- 六个受影响 Go 包测试、代理相关 race 测试、Go/TypeScript 契约漂移检查、前端类型检查及全量 ESLint 通过；前端单元测试 298 项通过。
- 两项 Chromium 隔离桥接交互测试覆盖节点秘密不回显、保留秘密、Key 覆盖和结果未知后显式重读；这是浏览器中的真实前端与模拟桥接，不是原生 Wails 或真实远程核心验收。
- Windows AMD64 egress / desktop 测试程序仅交叉编译通过，未执行 Windows 原生进程与窗口测试；构建脚本回归 63 项，9 项原生 Windows 条件测试跳过。
- 没有构建发行安装包，没有调用真实付费供应商、改动真实 Agent 配置或设置系统代理。
