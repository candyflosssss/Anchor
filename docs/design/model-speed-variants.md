# Anchor 固定速度档位与模型分身

状态：2026-10-15 已随 desktop.36 发布到 dev 在线更新渠道。核心、CLI 源码、桌面接口和前端共享此契约；独立 CLI 公开预发行仍为 alpha.6，不含本功能。实际供应商支持及原生 Codex 界面仍需单独验收。

## 用户行为

在供应商的「添加／编辑模型映射」中，可以在普通映射之外勾选 Fast、Ultrafast，生成独立公开模型：

| 公开模型 ID | 上游模型 ID | Anchor 固定 `service_tier` |
| --- | --- | --- |
| `gpt-6.1-sol` | `gpt-6.1-sol` | 无；沿用调用方原参数 |
| `gpt-6.1-sol(fast)` | `gpt-6.1-sol` | `priority` |
| `gpt-6.1-sol(ultrafast)` | `gpt-6.1-sol` | `ultrafast` |

Codex 可将分身作为普通模型选择，不依赖其 Fast／Ultrafast 原生速度入口。可见模型仍服从 Agent 的 single／all-compatible／all-mapped 范围、版本匹配元数据和客户端目录刷新规则，不自动扩大已批准的绑定范围，也不改写真实 Agent 配置来验证。

名称不会触发隐式参数规则：真正控制转发的是模型持久化的 `service_tier` 字段。仅将普通映射改名为带后缀的 ID，不等于启用加速。编辑界面的「Anchor 固定速度档位」可以显式设置或解除固定档位。

## Go／管理／桌面契约

- `state.Model.service_tier`：省略／空字符串表示不固定，允许的非空值只有 `priority`、`ultrafast`。模型级属性对这个公开 ID 的所有供应商渠道生效。
- `catalog.Action` / `Bridge.ProviderAction` 的 `model-put` 新增可选 `service_tier`、`speed_variants`；方法、鉴权、实例身份与 revision 规则不变。结构源为 `internal/catalog/workspace.go`，桌面 TS 从 Go 生成。
- `service_tier` 省略／JSON null：保留已有共享模型档位；新建不固定。显式空字符串：解除固定；非空值：设置固定档位。加入已有共享别名时不得借此改变其档位，不一致返回 conflict；带当前 model_revision 的编辑才可修改。
- `speed_variants` 省略／null／空数组：不新增分身。否则最多两个、不重复，值只能为 `fast`、`ultrafast`。仅可从未固定档位、无速度后缀的普通别名派生。原别名须为原始安全 ID，生成后的完整 ID 不超过 128 个 Unicode 码点。
- 同一事务写入／更新普通映射并创建全部所选分身。分身名称已被任一供应商使用、模型或供应商 revision 冲突，整体回滚，不覆盖、不自动加入既有分身；非法档位、重复选项、超长名称等为 invalid。不自动重试写请求，结果未知时重新读取核对。
- 分身 revision 从 1 开始，仅复制当前供应商渠道及其来源 Key、倍率、能力、策略、Codex 元数据和可移植模型配置。**不是动态链接**，后续增删渠道或修改上游名称不自动同步；可单独编辑／删除。
- 不复制普通档位的参考价格快照和供应商实际价格。固定档位新模型自动填充能力时同样不捕获普通档位参考价格；不凭固定倍率猜测真实加速费用。
- `Workspace.models[].service_tier` 为可选字符串，无固定档位时省略；旧共享样例不新增零值字段。界面根据该字段显示策略，不从模型名反推。
- Go 与 TS 共用 `desktop/contract/model-speed-variants.json` 检查分身 ID、上游名称及档位。

模型 ID 额外允许单个 `(fast)`／`(ultrafast)` 保留后缀；任意括号或嵌套后缀仍拒绝。供应商、Key、连接与 Binding ID 的规则不变。模型查询、设置、路由、价格、绑定与导出路径均使用模型专用校验。

## 请求转发与日志

1. 先按公开模型 ID 执行原有绑定范围授权与路由查找。
2. 若模型固定了档位，Anchor 覆盖调用方的 `service_tier`（包括调用方的 default／auto），所有候选和重试使用相同策略；普通映射保持透传。
3. 发往上游的 `model` 始终为候选的真实上游名称，不带本地分身后缀；不改变正文、流式、工具、认证与错误处理语义。
4. Responses 与 OpenAI-compatible Chat Completions 转换保留档位。Anthropic 转换无法表达此字段时明确拒绝，不静默丢弃。
5. 日志 `requested_service_tier` 记录调用方原请求；`mapping_service_tier` 记录 Anchor 固定策略；`upstream_service_tier` 仍只来自最后候选的上游响应。详情和复制摘要分开显示三者，不用请求档位冒充实际档位。上游返回 default 时显示 Standard，缺失时显示未知。

## 能力与费用边界

这是**显式请求上游服务档位**，不是绕过 OpenAI 套餐或供应商权限。创建时不发送付费探测，也不推断任意上游支持哪种速度；操作者需核对上游的模型、权限、协议及价格。上游可能拒绝、忽略或降级，映射保存成功不代表实际加速成功。分身不改变 Codex 官方账号身份，也不强开其原生速度菜单。

官方参数依据：[Ultrafast API](https://developers.openai.com/api/docs/guides/ultrafast-mode)、[速度模式说明](https://learn.chatgpt.com/docs/agent-configuration/speed)。API 路径的开放范围与 ChatGPT 订阅内的速度资格不同，第三方供应商仍以其实际能力为准。

CLI 完整用法见 [最新指令大全](../cli/commands.md)。测试使用隔离状态、共享样例和模拟上游，不请求真实付费推理。
