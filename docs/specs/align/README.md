# 规范对齐记录

本目录存放 [组件规范逐项对齐](../2026-11-14-spec-alignment-design.md) 的逐组件对照记录。基线规则见 [docs/design-baseline.md](../../design-baseline.md)。

## 参考源码（2026-11-14 锁定）

参考源码只放在本地 `/tmp/tango-ref`，不进仓库。需要重新准备时，按下表的版本下载：

| 基线 | 版本 | 本地路径 | 获取方式 |
|---|---|---|---|
| Ant Design | `antd@6.7.0` | `/tmp/tango-ref/antd/package` | `npm pack antd@6.7.0` |
| Pro Components（Ant Design Pro v6.0.3 依赖的版本） | `@ant-design/pro-components@3.1.14-2` | `/tmp/tango-ref/pro/package` | `npm pack @ant-design/pro-components@3.1.14-2` |
| Element Plus | `element-plus@2.14.8` | `/tmp/tango-ref/ep/package` | `npm pack element-plus@2.14.8` |
| Tailwind CSS（shadcn 灰阶） | `tailwindcss@4.3.5` | `/tmp/tango-ref/tw/package/theme.css` | `npm pack tailwindcss@4.3.5` |
| shadcn/ui v4 | `shadcn-ui/ui@e62c9435fff729a175f38983dee73b2cb4f6a069` | `/tmp/tango-ref/shadcn-ui/apps/v4/registry` | 稀疏克隆后 checkout 该 commit |

官网文档页可以作为补充；默认值、令牌、尺寸以上面的源码为准。

## 文件命名

每批每组一个文件，命名为 `<批号>-<组名>.md`，例如 `1-entry.md`、`2-data.md`、`4-pro-table.md`。

## 每个组件一张表

```markdown
### Segmented

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| 方向 | EP `direction: horizontal/vertical`，2.8.7 起（ep/es/components/segmented/src/segmented.d.ts，2026-11-14） | entry_segmented.go `Vertical()` | 已对齐 |
| 自定义每项内容 | EP default 插槽（同上）；antd `options[].label: ReactNode` | 只有 `Icons` | 已补齐（TestSegmentedItemRender） |
| props 字段映射 | EP `props`，2.9.8 起 | — | 不需要（Go 的 Option 有类型） |
```

- 规范项：功能、默认值、受控和非受控、交互和键盘、禁用和加载、尺寸和令牌、明暗主题，各列一行。
- 官方定义：写清是哪一家（antd / Pro / EP / shadcn）、源码文件或官网 URL，以及核对日期。只凭记忆、没有核对的，写“依据知识，未核对”。
- Tango 现状：写 `文件:符号`；没有就写“—”。
- 结论只能取下面五种之一：
  - **已对齐**
  - **已补齐**（括号里写测试名）
  - **等价写法**（说明 Go 里怎么写）
  - **不需要**（说明原因）
  - **暂不做**（写清缺什么）

## 基础能力缺口

每批记录文件的末尾汇总本批遇到的、需要改基础层才能做的缺口：

| 缺口 | 影响的组件 | 需要的基础能力 | 现状 |
|---|---|---|---|
| 背景模糊 | FooterToolbar | paint 层 backdrop blur 原语 | 暂不做，等用户决定 |
