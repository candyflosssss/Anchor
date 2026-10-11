# Anchor 供应商前端：原型与桌面模式

React / TypeScript / shadcn/ui 前端，按功能划分 Model / View / hook Controller。视觉保持官方 `sidebar-07` 可折叠侧栏、inset 内容区与 neutral 主题；来源、MIT 许可和适配差异见 [SHADCN_SOURCES.md](SHADCN_SOURCES.md)，不恢复初版自定义靛蓝布局。

## 两种模式

| 模式 | 入口 | 行为 |
| --- | --- | --- |
| prototype | `npm run dev` / `scripts/anchor-frontend-dev.sh` | 明确标记的内存 Mock，不连接本机核心，不读真实状态；勿输入真实密钥 |
| desktop | `npm run build` → Go embed → `anchor-desktop` | 只调用受控 Wails 桥接；缺绑定时报错，绝不回退 Mock；普通浏览器不能直接管理核心 |

Windows / Linux 桌面源码已切换为无系统标题栏窗口；原 sidebar-07 header 中集成拖动、双击最大化 / 还原与窗口按钮，窗口行为经独立 Model / hook Controller / View 和受限 runtime 适配器，不由 View 直接调用宿主。原型隐藏原生控件；运行时缺失不模拟成功。此项尚未做原生验收。

开发宿主在打开时自动安全初始化缺失状态、启动／核验复用本机核心，就绪后展示供应商。关窗不停止核心；显式停服另行确认，只报告请求接受，不假称恢复完成。未实现远程实例切换、系统服务或开机自启动设置；Windows 当前用户安装器／Ubuntu24.04 AMD64 `.deb` 已补齐打包入口，实际产物见[桌面安装说明](../../docs/release/desktop.md)。本轮按用户要求不做原生双平台或安装验收。

**注意**：桌面实际启动默认使用当前用户 Anchor 状态，并复用已有批准的托管策略。测试必须传入独立 `--data-dir`，不得用真实 Agent 配置做验收。构建本身不启动核心、不安装或替换用户程序。

## 工具链与契约

Node 22.22.1、npm 9.2.0 精确锁定；完整依赖见 package.json / package-lock.json。React 19.3.0、Vite 8.3.4、TypeScript 5.9.3、Tailwind 3.4.19 和 Radix 组件已经锁定；Go Wails v2.15.0 在根 go.mod / go.sum。Tailwind v3 并非已通过 WebKitGTK 验证的证明。

- [供应商界面设计](../../docs/design/providers-v1.md)
- [供应商接口语义](../../docs/design/provider-interface.md)、[桌面运行契约](../../docs/design/desktop-runtime.md)
- Go 桌面 DTO 为 `internal/desktop/contract.go`，TS 生成文件不手改；在仓库根目录运行 `go run ./cmd/desktop-contract > desktop/frontend/src/desktop/contract.generated.ts`。构建检查漂移，不偷偷重新生成。
- Go 与 Mock 共同读取合成样例 `desktop/contract/providers.json`。生产静态资源构建拒绝夹带供应商 Mock 样例／原型场景入口。2026-10-12 按用户要求新增独立且持续标注的日志 UI 设计预览，暂时在两种模式下均可打开；其合成日志不参与核心管理，不是连接失败回退。真实日志接入完成后移除该预览。

## 本机设计预览

在仓库根目录执行（本机重任务均受 guard 约束）：

```bash
/home/mosaki/.codex/skills/resource-guard/scripts/resource_guard.sh run \
  --expected-mib 1024 --max-mib 2048 --swap-max-mib 256 \
  --label anchor-ui-install -- timeout 180 npm --prefix desktop/frontend ci --no-audit --no-fund
/home/mosaki/.codex/skills/resource-guard/scripts/resource_guard.sh status
./scripts/anchor-frontend-dev.sh
```

浏览器打开 `http://127.0.0.1:14201`。仅监听回环，20 分钟自动结束，也可 Ctrl+C；端口占用直接失败，不复用其他服务。预览支持供应商搜索／筛选、详情、增改、启停确认、密钥三态、主题、窄窗口和错误／冲突／未知结果场景。刷新重置内存，只保存 `anchor.ui.appearance` 外观偏好；没有删除、余额或上游连通性测试。

2026-10-12 日志预览：点击侧栏“日志”，可体验请求/系统页签、搜索筛选、右侧详情和尝试时间线、摘要复制、示例 JSON 导出、保留设置及缺口/空/离线场景。全部为虚拟记录，不发起真实上游请求，不持久保存日志或保留设置；设计基线见[日志设计](../../docs/design/logging.md)。

桌面开发可执行文件的两平台构建方法、产物及缺失门禁见[桌面开发版构建](../../docs/release/desktop.md)，不是上述 Vite 开发服务器。

## 开发与校验

按资源约束串行运行：

```bash
/home/mosaki/.codex/skills/resource-guard/scripts/resource_guard.sh run \
  --expected-mib 1024 --max-mib 2048 --swap-max-mib 256 \
  --label anchor-ui-checks -- timeout 180 bash -c \
  'cd desktop/frontend && npm run typecheck && npm run lint && npm test'
/home/mosaki/.codex/skills/resource-guard/scripts/resource_guard.sh status

/home/mosaki/.codex/skills/resource-guard/scripts/resource_guard.sh run \
  --expected-mib 2048 --max-mib 3072 --swap-max-mib 256 \
  --label anchor-ui-browser -- timeout 240 bash -c \
  'cd desktop/frontend && ANCHOR_TEST_BROWSER_EXECUTABLE=/home/mosaki/.cache/ms-playwright/chromium_headless_shell-1234/chrome-headless-shell-linux64/chrome-headless-shell npm run test:ui && ANCHOR_TEST_BROWSER_EXECUTABLE=/home/mosaki/.cache/ms-playwright/chromium_headless_shell-1234/chrome-headless-shell-linux64/chrome-headless-shell npm run test:desktop'
/home/mosaki/.codex/skills/resource-guard/scripts/resource_guard.sh status
```

两套 Playwright 分别在 14201/14202 启动并关闭 Vite，不复用已有服务；单 worker，profile 使用 guard 的磁盘 TMPDIR。默认尝试 Google Chrome；本机使用缓存 Chromium headless shell，避免此前 Google Chrome 的启动 SIGTRAP。可设置 `ANCHOR_CAPTURE=1` 生成设计帧，默认不改截图。

## 验证边界

此前宿主实现轮的 TypeScript、ESLint、50 项 Vitest、15 项原型浏览器与 6 项 desktop 模拟绑定浏览器测试通过。Go 全包 race / vet 及隔离真实核心测试覆盖初始化、并发复用、密钥保留／清除、revision 冲突、认证／端口错误、宿主进程真正退出后的核心存活与显式停服。每类证据分别记录。

**尚未完成原生验收**：Windows WebView2 / Linux WebKitGTK 原生窗口、真实 JS↔Wails 序列化、输入法、缩放／跨屏、两端安装入口和自启动设置。Windows 交叉编译仅是编译；此前 Linux 直接构建因宿主缺开发库被阻止，安装包轮新增受限 Ubuntu24.04 容器构建，不在宿主安装开发包。该轮只进行编译／打包，不重跑此处测试、不运行安装器或原生应用。Mock 通过和 TS 通过不能替代这些门禁。

## 依赖安全边界

此前宿主实现轮运行 `npm audit --json`，仍报告 8 项（5 high、3 moderate），涉及 Tailwind v3 构建依赖链；安装包轮未修改依赖、未重新审计或修复，也未宣称清零。开发产物不构成正式发行安全批准；对外发行前须重新审计、修复或替换相关依赖，并完成原生验收。不得为了消除审计数字未经评估升级整个 Tailwind 主版本。
