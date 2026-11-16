# API 稳定层保守第一批

## 意图

把已经完成组件覆盖验证、且 API 边界相对清晰的基础组件加入 `api/stable/ui.txt`，让文档生成器和 API 检查把它们视为稳定层；不把 Pro、移动端、图表、编辑器、虚拟化和高风险扩展一次性承诺出去。

## 范围

加入：

- `ButtonGroup`
- `FloatButton`、`FloatButtonGroup`、`BackTop`
- `AutoComplete`
- `Mentions`
- `Cascader`、`CascaderPanel`
- `TreeSelect`
- `Upload`、`DropZone`
- `Descriptions`
- `Timeline`
- `Menubar`
- `AspectRatio`
- `Item`、`ItemGroup`

不加入：`Masonry`、`Splitter`、`ColorPicker` 全套、日期时间扩展、`Tour`、`Skeleton`、`QRCode`、`Watermark`，以及全部 Pro、shadcn 专属、虚拟化、编辑器、移动端、图表和工作台组件。

## 约束

- 只修改 `api/stable/ui.txt` 及必要的 API/CHANGELOG 记录，不修改组件实现。
- 稳定类型签名引用的类型必须已经稳定；若 `apitxt -check` 发现跨层引用，则缩小本批范围，而不是扩大稳定层。
- 运行 `go run ./internal/cmd/apitxt -check`、相关组件测试和短测试；失败时不提交。
- 不纳入当前工作区其他会话的未跟踪文件。

## 方案

采用保守白名单方案：将上述组件的公开入口及其构建器类型加入稳定列表，由 `apitxt -check` 验证稳定闭包；不把“画廊通过”直接等同于完整 API 兼容承诺。
