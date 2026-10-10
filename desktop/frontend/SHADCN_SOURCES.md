# shadcn Blocks / 组件源码来源

- 核对 / 引入日期：2026-10-11。
- 用户参考：https://ui.shadcn.com/blocks。
- 兼容源码：官方 v3 registry（new-york；Tailwind v3 / Radix），不是最新 Tailwind v4 / Base UI 实现。
- 许可：MIT，完整版权与许可保留于 `SHADCN_LICENSE.md`。
- 手工按需复制官方源码；未运行 shadcn CLI，也未引入整套 dashboard。

## 获取的官方 registry 与 SHA-256

| 内容 | 上游 URL | 获取内容 SHA-256 |
| --- | --- | --- |
| sidebar-07 | https://v3.shadcn.com/r/styles/new-york/sidebar-07.json | `5ecc58badcf64e243732a286e4b0dc1d4cac0996a1b3fd1fca81f20f5d345995` |
| sidebar | https://v3.shadcn.com/r/styles/new-york/sidebar.json | `2a92072fbe9c8ae55a99259cd459b026c428c2fe7bde2cc412d6b0ef8c200a35` |
| button | https://v3.shadcn.com/r/styles/new-york/button.json | `4d8f39c3bd25e630b5962667722e8707e7b18122ad6842a5c22acf8a3ff9f93a` |
| input | https://v3.shadcn.com/r/styles/new-york/input.json | `4d1a3b126cc62485b225e3da32d4a56df851c95c5a24035af1f1c80f33726cfa` |
| table | https://v3.shadcn.com/r/styles/new-york/table.json | `0cf28e873dde65e036d5805297dbcedaa0eb98bc33723b7f337293971ad1aa02` |
| separator | https://v3.shadcn.com/r/styles/new-york/separator.json | `01173bde1937c35da6f84f92bb85f7e882a9091064b4532a058177fe1a96d17f` |
| sheet | https://v3.shadcn.com/r/styles/new-york/sheet.json | `f9d5c635be7785736a4d716975fc3d34eecaf09552dd4805569fd989e56fda8a` |
| tooltip | https://v3.shadcn.com/r/styles/new-york/tooltip.json | `aef6ec3c142b054ebcd6daaf9e0b46e67f0d50e51eefbd7b392c83845a3f1926` |
| skeleton | https://v3.shadcn.com/r/styles/new-york/skeleton.json | `eb09c61378d058280deede279f924cc5394593693f30c2477d64468e8dc92d77` |
| breadcrumb | https://v3.shadcn.com/r/styles/new-york/breadcrumb.json | `617dfb02af217c0a8bd7fc1bc38dc739fe87570c7654408c6a6e6ebbf4084dce` |
| use-mobile | https://v3.shadcn.com/r/styles/new-york/use-mobile.json | `80ac2179b446ae77e08b378d82d564f3e1f2c75be5d737df20ebe778093fb588` |
| switch | https://v3.shadcn.com/r/styles/new-york/switch.json | `90aa753ab629ab1c7cc5dcd7ed8404b761eb99c90f3531aa66bbbb4ea9716a26` |

## 本地适配

- app/app.tsx 与 components/shared/app-sidebar.tsx 适配 sidebar-07 的 Provider、Sidebar、header、breadcrumb 和分组导航；使用官方 Sidebar 的 inset 变体。
- 移除示例团队 / 用户 / 项目 / 子菜单和占位图表；只保留供应商切片与明确禁用的未来导航。
- ui/sidebar.tsx 等保留官方结构与样式，移除 Next.js 的 use client、改成本地相对导入；VariantProps 按严格 TS 使用 type-only import。
- sidebar：取消默认 cookie 写入；Ctrl/Cmd+B 在输入与弹窗内不触发；移动 Sheet 提供中文标题、可见关闭按钮（提升层级与点击区，避免被内容遮挡）和触发器焦点恢复。
- Button / Input / Table / Breadcrumb / Separator / Sheet / Tooltip / Skeleton / Switch 使用上述源码；主题为 neutral，输入边界为满足非文本对比度单独增强。
- 业务编辑 Dialog 保留既有 Radix 受控关闭 / 未保存确认逻辑，采用与官方相同的 overlay / rounded border / shadow 视觉结构；不是完整官方 Dialog 文件复制。
- 未复制无用的账户或团队菜单，未加入假的请求图表、健康监测或实例切换。
- 桌面窗口控制是 Anchor 自行添加的宿主适配，不是官方 Block 自带能力：在原 header 中加入拖动区域和最小化 / 最大化 / 关闭按钮，图标复用现有 lucide-react，样式沿用 neutral 语义变量；浏览器原型隐藏窗口按钮。
