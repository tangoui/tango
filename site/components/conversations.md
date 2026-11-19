---
title: Conversations 会话列表
---
# Conversations 会话列表

在聊天侧栏中展示会话历史，支持按时间分组、选择、就地重命名和删除。

## 在线体验

<GalleryPreview scene="Conversations" />

## 基本用法

`Conversations` 接收 `[]ui.Conversation`，通过 `*string` 绑定当前选中项的 `Key`。每个会话使用唯一且稳定的非空 `Key`。显式 `Group` 优先，未指定时根据 `Time` 和当前语言显示今天、昨天、过去 7 天、过去 30 天或更早等分组。组件按传入顺序显示会话。

```go
import "github.com/tangoui/tango/ui"

var activeConversation = "project"
var conversations = []ui.Conversation{
    {Key: "project", Title: "项目讨论", Group: "工作"},
    {Key: "design", Title: "设计评审", Group: "工作"},
}

func page(c *ui.Context) {
    list := ui.Conversations(c, conversations, &activeConversation).
        Height(320).Virtual().A11yLabel("会话历史")
    if key, title, ok := list.Renamed(); ok {
        for i := range conversations {
            if conversations[i].Key == key {
                conversations[i].Title = title
                break
            }
        }
    }
    if key, ok := list.Deleted(); ok {
        for i, conversation := range conversations {
            if conversation.Key == key {
                conversations = append(conversations[:i], conversations[i+1:]...)
                if key == activeConversation {
                    activeConversation = ""
                }
                break
            }
        }
    }
}
```

`Selected()` 返回本帧新选中的键。应用通过 `Renamed()` 读出键和新标题，通过 `Deleted()` 读出待删除的键，再更新自己的切片。`NoGroups()` 隐藏分组标题，`NoRename()` 和 `NoDelete()` 分别隐藏并关闭相应操作。悬停或聚焦会话行时显示可用操作，聚焦后可用 F2 重命名、Delete 删除。

## 长列表与滚动

- `Height(px)` 为列表设置固定高度和内部滚动视口。默认高度为 0，列表随内容增长，可放在外层 `Scroll` 中。
- `Virtual()` 配合正的 `Height` 启用虚拟化。组件构建视口附近的行，并保留选中行和正在重命名的行。↑ / ↓、Home / End 可以跨整个列表移动焦点并滚动到目标行。手动滚动使聚焦行离开视口时，焦点可能丢失。无障碍选项会报告自己在整个列表中的位置。
- `ScrollToKey(key, align, offset)` 在指定了 `Height` 时滚动到对应会话。`align` 使用 `ui.AlignTop`、`ui.AlignBottom` 或 `ui.AlignAuto`，`offset` 以像素计。把滚动请求放在触发它的帧，并在调用 `Selected()`、`Renamed()` 或 `Deleted()` 之前设置滚动和虚拟化选项。
- 虚拟列表缓存分组布局，并用当前视口中的数据校验缓存。同一切片中的 `Time` 或 `Group` 改动会在相关行进入视口时得到核对。筛选掉当前选中的会话后，组件每帧会扫描列表寻找该选中键。

## 相关链接

- [源码 ui/chat_conversations.go](https://github.com/tangoui/tango/blob/main/ui/chat_conversations.go)、[ui/chat_conversations_view.go](https://github.com/tangoui/tango/blob/main/ui/chat_conversations_view.go)
- [ui Go API](/reference/api/ui)
