# 编辑器的悬停、补全与诊断

`ui/editor` 的浮层构建于窗口坐标系。编辑器滚动和换行会改变锚点位置，浮层不占用文档行高。由应用保存 LSP 结果、校验文档版本，并决定每一帧打开哪个浮层。

## 悬停和快速修复

```go
if popup := view.MountContent(hoverOffset, ui.BottomStart, func() {
    ui.Markdown(c, documentation)
}, layout.MaxW(420)); popup != nil {
    popup.Bg(c.Theme.BgElevated).Border(1, c.Theme.BorderSecondary)
}
```

`HoverOffset() (int, bool)` 给出指针下最近的 UTF-8 字节偏移。进入 gutter、文档末尾下方、sticky header 或其他控件时查询失败。结果可用于安排 hover 请求。请求返回后保留当时的偏移，浮层内部的链接和按钮可照常处理指针。

`OffsetAnchor(offset int) (geom.Rect, bool)` 和 `MountContent(offset int, place ui.Placement, body func(), opts ...layout.Option) *ui.Box` 使用上一帧的编辑器布局和当前视口。位置在折叠区或可见文本区之外时，分别返回 `false` 和 `nil`。`MountContent` 支持普通 Tango 控件，通过返回的 `Box` 可设置背景、边框等。建议先检查 `popup != nil`。

快速修复可用诊断范围的 `Start` 做锚点。调用 `MountContent` 构建按钮或动作列表，或把 `OffsetAnchor` 的结果交给已有的弹层。用 `DecorationOptions.Squiggle color.NRGBA` 指定波浪线颜色以及透明度。`DecorationFromLSP` 可把 UTF-16 范围转换成 UTF-8 偏移，`SetDecorations` 按诊断来源维护各组范围。

## 补全

`(*editor.ViewW).Suggest(editor.SuggestOptions) *ui.Box` 挂到主光标。在查询 `Changed`、`OffsetAnchor` 或 `CompletionAnchor` 前注册它。用 `Labels []string` 提供候选，通过 `Selected *int` 保存选择，以 `-1` 表示无选中。`Accept func(int)` 和 `Dismiss func()` 由调用方提交编辑和关闭。`Details func(int)` 可在相邻的可滚动栏构建 Markdown、文字或控件。用 `Width`、`DetailsWidth`、`MaxHeight` 指定逻辑像素尺寸，`Name` 提供本地化的无障碍名称。

方向键循环选择，Home 和 End 定位首尾，Page Up 和 Page Down 按可见行数翻页。候选随选中项滚动，Enter、Tab 和鼠标点击提交，Escape 关闭。输入法正在组合时沿用编辑器的按键分发。保留候选数据及关闭条件在调用方，便于按 LSP 返回更新。

## Monaco 0.56.0 对照

2026-11-22 核对正式版依赖中的 `monaco-editor@0.56.0`。下表以同字号、相同字体的最大数字宽度 `d` 为比较条件，逻辑像素公式来自 Monaco 源码。gutter 以 5 位行号、开启折叠、关闭 Monaco 的独立 glyph margin 计算。

| 项目 | Tango 修改前 | Tango 修改后及差异 |
| --- | --- | --- |
| gutter | `ceil(5*d) + 42`，其中左垫 8、装饰 18、折叠槽 16 | `round(5*d) + 26`，与 `common/config/editorOptions.js` 在关闭 glyph margin 时一致。MinCode `use-editor-debug-decorations.ts` 的项目模式使用 3 位行号、4 px 装饰区和独立断点槽，每条 glyph lane 占一行高度。Tango 可以用现有 `GutterMark` 与 `OnGutterClick` 表达断点。 |
| 折叠图标 | 默认鼠标进入 gutter 时显示展开图标，已折叠图标常驻 | 保留同一可见性条件和 16 px 槽。Monaco `contrib/folding/browser/folding.css` 对可见性应用 500 ms 淡入淡出和 codicon 字形，Tango 即时切换并绘制 chevron。 |
| 当前行边框 | 普通和高对比主题统一 2 px，随视口画矩形 | 普通 2 px、高对比 1 px，文本边框随行横向滚动，和 gutter 同时显示时去掉 gutter 右边框。`browser/viewParts/currentLineHighlight/currentLineHighlight.js` 还支持自定义主题同时设置实底与显式边框，Tango 实底优先。 |
| 括号匹配 | 宽高各缩 1 px，再内描边 1 px | 直接在字符完整行盒内描 1 px，遵循 `contrib/bracketMatching/browser/bracketMatching.css` 的 `border-box`。两端的匹配与配色继续使用编辑器主题。 |

## MinCode Tango 接入

- `internal/modules/projects/projects.go` 的 `editorPane` 在 `editor.View(...).OnGutterClick(...)` 与 `w.Changed()` 之间调用补全构建。`lsp_ui.go` 可把 `lspInstallZones` 的候选绘制换成 `Suggest`，用 `sg.items[i].label` 填 `Labels`，传 `&sg.selected`，详情用候选的 `detail`，接受继续走 `lspAcceptCompletion(d)`，关闭时清空 `p.lsp.suggest`。保留触发补全的 Ctrl+Space，删去由 `Suggest` 接管的方向键、接受和关闭绑定。
- `internal/modules/projects/lsp.go` 的 hover 请求返回时，在 `hoverState` 保存请求的字节偏移和版本。`lsp_ui.go` 中将 hover ViewZone 改为 `MountContent`，从 `HoverOffset` 或当前光标取得请求位置。保存锚点到鼠标离开编辑器及浮层后再关闭，允许在 Markdown 内交互。`lsp_features.go` 中的签名帮助可沿用这条挂载路径。
- `internal/modules/projects/lsp.go` 发布诊断时继续用 `DecorationFromLSP` 和 `SetDecorations`，将不同严重度的颜色写入 `Squiggle`。`lsp_features.go` 发出 `codeAction` 请求的位置，可用 `OffsetAnchor` 或 `MountContent` 把现有动作入口挂到诊断的 `Start`。
- `internal/modules/terminal/contextmenu.go` 调用 `ContextMenuAtWith` 时在已有 `ContextMenuOptions` 中加 `Width: 238`。
- `internal/modules/terminal/tools_ui.go` 中的回放、收藏，以及 `snippets_ui.go` 的片段抽屉可在构建外层使用 `ui.ConfigProvider(c).Components(theme.DialogMask(theme.Hex("#00000080"))).Body(...)`。终端其他对话框需要同一遮罩时可把这一作用域放在终端页共同父节点上。
