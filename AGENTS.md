# Tango 开发须知（给所有 AI 和开发者）

## UI 设计基线（必须遵守）

自绘 UI（`ui/`、`ui/theme`、`ui/icon`、`ui-gallery`、`site` 的组件文档）统一按 **Ant Design 6.7（`antd@6.7.0`）+ Ant Design Pro v6（配套 Pro Components v3）** 设计和实现。

- 组件 API 语义、默认值、尺寸、令牌、交互状态都以 antd 6.7 为准；Pro 系列（ProLayout、ProTable、ProForm……）以 Ant Design Pro v6 / Pro Components v3 为准，不要按 v4/v5 或 procomponents v2 的印象写。
- Element 主题的组件变体对照 Element Plus 2.14.8；shadcn 主题对照 shadcn/ui v4（new-york-v4）。
- 精确的令牌取值从 `antd@6.7.0` 源码读，不要估值；偏离基线时写明差异和原因；基线版本只有用户能改。
- 完整规则和核对来源见 [docs/design-baseline.md](docs/design-baseline.md)，编写组件的约定见 [ui/AUTHORING.md](ui/AUTHORING.md)。
