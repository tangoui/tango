# 设计基线：Ant Design 6.7 + Ant Design Pro v6

> 用户在 2026-11-14 定下。所有人和所有 AI 在设计或开发 Tango 自绘 UI（`ui/`、`ui/theme`、`ui/icon`、`ui-gallery`、`site` 的组件文档）时都按这份基线来。改基线版本要用户本人决定。

## 1. 对齐的版本

| 范围 | 基线 | 核对来源（2026-11-14） |
|---|---|---|
| 组件、设计令牌、交互细节 | **Ant Design 6.7**（`antd@6.7.0`） | npm `antd` 的 latest 是 6.7.0；官网 https://ant.design/index-cn |
| 中后台页面与布局模式（ProLayout、菜单、页头、设置抽屉、登录页、列表页/表单页/详情页模板） | **Ant Design Pro v6**（`ant-design-pro` v6.0.3，2026-08-17 发布） | https://pro.ant.design 、GitHub `ant-design/ant-design-pro` master 的 package.json |
| Pro 组件（ProTable、ProForm、ProLayout、ProCard、ProDescriptions、ProList 等） | **Pro Components v3**（`@ant-design/pro-components` 3.1.x，Ant Design Pro v6 用的就是这一版） | Ant Design Pro v6 的 package.json 依赖 `^3.1.14-2`；npm 上 3.x 目前挂在 `beta` 标签（3.1.15-9），`latest` 标签还是 2.8.10 |

| Element Plus 主题（只用于 `theme.Element` 风格及其组件变体） | **Element Plus 2.14.8**（`element-plus@2.14.8`） | npm `element-plus` 的 latest 是 2.14.8；官网组件页显示的也是 2.14.8 |
| shadcn 主题（只用于 `theme.Shadcn`） | **shadcn/ui v4**（registry `new-york-v4`，仓库 commit e62c943，CLI `shadcn@4.23.0`；颜色取自 `apps/v4/registry/themes.ts`，灰阶取自 `tailwindcss@4.3.5` 的 `theme.css`） | 2026-11-14 核对 GitHub `shadcn-ui/ui` main |

说明：用户说的「pro v6.7」按 **Ant Design Pro v6 + Ant Design 6.7** 理解。Pro 本身的版本号是 6.0.x，6.7 指的是 antd。

## 2. 规则

1. **以 antd 6.7 为准**：组件名、属性/方法语义、默认值、尺寸、间距、圆角、阴影、动效时长、颜色算法、状态色，以及 hover/active/focus/disabled 的表现，都对照 antd 6.7。不要凭 v4/v5 的印象写，两版之间有改动的（比如 v6 新增的组件、语义化 `classNames`/`styles`、`List` 已标记弃用），都按 v6。
2. **Pro 的部分对照 Ant Design Pro v6 和 Pro Components v3**，不再按 procomponents v2.8 的 API。`docs/specs/ui-coverage.md` 里“依据 procomponents v2 的知识整理”的条目，后续要按 v3 复核。
3. **设计令牌的取值从源码拿**：要精确数值（seed/map/alias token、组件 token、暗色和紧凑算法）时，读 `antd@6.7.0` 包源码（`npm pack antd@6.7.0` 解到临时目录），不要估值。写进 `ui/theme` 时在注释里注明对应的 antd token 名。
4. **Ant Design 主题是默认主题、也是参考实现**。Element Plus 和 shadcn 主题只在各自主题里保留自己的视觉差异，不能反过来改动 Ant 主题的行为。
5. **偏离基线要写明**：因为平台限制或 Go API 风格（见 [ui/AUTHORING.md](../ui/AUTHORING.md)）必须和 antd 不一样时，在代码注释和对应文档里写出“与 antd 6.7 的差异及原因”。
6. **核对时注明来源和日期**：对照官网或源码做的结论，写清楚看的是哪个页面或文件、哪一天；没核对过、只凭记忆的，标“依据知识，未核对”。
7. **不要擅自升级基线**：antd 或 Pro 出了新版本，可以在文档里记下差异，但切换基线要用户决定，并同步更新本文件。

## 3. 参考入口

- Ant Design 组件总览：https://ant.design/components/overview-cn
- Ant Design 设计令牌与主题：https://ant.design/docs/react/customize-theme-cn
- Ant Design Pro：https://pro.ant.design/zh-CN
- Pro Components：https://procomponents.ant.design
- Ant Design Pro 源码（页面模板、布局配置）：https://github.com/ant-design/ant-design-pro
