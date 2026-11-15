package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
)

// AI chat components: Bubble, BubbleList, Sender, ThoughtChain, ToolCall,
// Approval, Conversations, Prompts.

const chatAnswer = "可以。`BubbleList` 只构建视口附近的消息，高度按 key 缓存：\n\n" +
	"1. 新内容流入时贴底跟随；\n2. 向上滚动后停止跟随，出现 **回到底部** 按钮；\n3. 在顶部前插历史消息时视图保持不动。\n\n" +
	"```go\nui.BubbleList(c, len(msgs), key, item, layout.Grow(1))\n```"

// ---------------------------------------------------------------------------
// Bubble

var bubbleDemo struct{ last string }

func bubblePage(c *ui.Context) {
	gallery.Demo(c, "角色与变体", "助手默认无底色并渲染 Markdown，用户默认填充气泡、纯文本，系统消息居中。", func() {
		ui.Column(c, func() {
			ui.Bubble(c, ui.ChatSystem, "已切换到 GPT-5 · 上下文 128k")
			ui.Bubble(c, ui.ChatUser, "长对话会不会卡？列表能虚拟化吗？")
			b := ui.Bubble(c, ui.ChatAssistant, chatAnswer).Name("Tango 助手").Meta("10:24 · 1.2k tokens").
				Actions(ui.BubbleCopy | ui.BubbleRetry).ActionsAlways()
			if b.Retried() {
				bubbleDemo.last = "retry"
			}
			ui.Bubble(c, ui.ChatAssistant, "Outlined 变体。").Variant(ui.BubbleOutlined)
			ui.Bubble(c, ui.ChatAssistant, "Shadow 变体，头像用文字。").Variant(ui.BubbleShadow).AvatarText("AI")
			ui.Bubble(c, ui.ChatAssistant, "Filled 变体，不显示头像。").Variant(ui.BubbleFilled).NoAvatar()
			if s, ok := ui.Bubble(c, ui.ChatUser, "用户消息可以原地编辑，悬停后出现操作。").Actions(ui.BubbleCopy | ui.BubbleEdit).ActionsAlways().Edited(); ok {
				bubbleDemo.last = s
			}
		}, layout.Gap(16), layout.W(720))
	})
	gallery.Demo(c, "加载与流式", "Loading 显示跳动的点；Typing 按速度逐字显示，末尾带光标。", func() {
		ui.Column(c, func() {
			ui.Bubble(c, ui.ChatAssistant, "").Loading(true)
			ui.Bubble(c, ui.ChatAssistant, "正在生成回答，内容随流式追加").Typing(true)
		}, layout.Gap(16), layout.W(720))
	})
	gallery.Demo(c, "附加内容", "Extra 在气泡下方放置思考链、工具卡片等。", func() {
		ui.Column(c, func() {
			ui.Bubble(c, ui.ChatAssistant, "我先读一下配置文件。").Extra(func() {
				ui.ToolCall(c, "read_file").Summary("tango.json").Status(ui.ChainSuccess).Duration(120 * time.Millisecond).
					Args(`{"path": "tango.json"}`).Result(`{"name": "demo", "frameless": true}`)
			})
		}, layout.W(720))
	})
}

// ---------------------------------------------------------------------------
// BubbleList: a whole chat panel with a simulated streaming reply.

type chatMsg struct {
	key  string
	role ui.ChatRole
	text string
}

var chatDemo = struct {
	msgs    []chatMsg
	draft   string
	busy    bool
	reply   string // full text of the reply being streamed
	started time.Time
	seq     int
}{msgs: func() []chatMsg {
	qs := []string{"怎么把 Tango 窗口做成无边框？", "IME 组字时按 Enter 会误发送吗？", "长对话性能怎么样？"}
	as := []string{
		"在窗口选项里打开无边框，再用 `ui.Header` 自绘标题栏，并把可拖动区域标记出来。",
		"不会。`Sender` 在组字期间（以及组字刚结束的那一帧）忽略 Enter，只有提交后的 Enter 才发送。",
		chatAnswer,
	}
	var out []chatMsg
	for i := range qs {
		out = append(out, chatMsg{fmt.Sprintf("q%d", i), ui.ChatUser, qs[i]}, chatMsg{fmt.Sprintf("a%d", i), ui.ChatAssistant, as[i]})
	}
	return out
}()}

// chatStream advances the simulated reply (40 characters per second).
func chatStream(c *ui.Context) {
	d := &chatDemo
	if !d.busy {
		return
	}
	full := []rune(d.reply)
	n := min(len(full), int(c.Now().Sub(d.started).Seconds()*40))
	last := &d.msgs[len(d.msgs)-1]
	last.text = string(full[:n])
	if n >= len(full) {
		d.busy = false
		return
	}
	c.WakeAt(c.Now().Add(50 * time.Millisecond))
}

func chatPanel(c *ui.Context) {
	d := &chatDemo
	chatStream(c)
	ui.Column(c, func() {
		ui.BubbleList(c, len(d.msgs), func(i int) string { return d.msgs[i].key }, func(i int) {
			m := d.msgs[i]
			last := i == len(d.msgs)-1
			acts := ui.BubbleCopy | ui.BubbleEdit
			if m.role == ui.ChatAssistant {
				acts = ui.BubbleCopy | ui.BubbleRetry
			}
			b := ui.Bubble(c, m.role, m.text).Typing(last && d.busy).Loading(last && d.busy && m.text == "").Actions(acts)
			if b.Retried() && !d.busy {
				d.msgs[i].text = ""
				d.reply, d.started, d.busy = chatAnswer, c.Now(), true
			}
			if s, ok := b.Edited(); ok {
				d.msgs[i].text = s
			}
		}, layout.Grow(1))
		s := ui.Sender(c, &d.draft).Loading(d.busy).AttachButton().
			Commands([]ui.Option{{Label: "清空对话", Value: "clear"}, {Label: "解释选中代码", Value: "explain"}, {Label: "生成单元测试", Value: "test"}}).
			Mentions(ui.Opts("main.go", "README.md", "go.mod")).
			Layout(layout.Margin(geom.Insets{Left: 16, Right: 16, Bottom: 16}))
		if s.Submitted() {
			d.seq++
			d.msgs = append(d.msgs, chatMsg{fmt.Sprintf("u%d", d.seq), ui.ChatUser, strings.TrimSpace(d.draft)},
				chatMsg{fmt.Sprintf("r%d", d.seq), ui.ChatAssistant, ""})
			d.draft = ""
			d.reply, d.started, d.busy = chatAnswer, c.Now(), true
		}
		if s.Stopped() {
			d.busy = false
		}
	}, layout.W(760), layout.H(560))
}

func bubbleListPage(c *ui.Context) {
	gallery.Demo(c, "消息流", "只构建可见区附近的消息；流式追加时贴底跟随，向上滚动后停止并出现“回到底部”。在输入框里试试 / 与 @。", func() {
		chatPanel(c)
	})
}

// ---------------------------------------------------------------------------
// Sender

var senderDemo = struct {
	a, b, c, d string
	files      []ui.ChatAttachment
	sent       string
}{
	b: "第一行\n第二行：Shift+Enter 换行，内容变多时输入框自动增高",
	files: []ui.ChatAttachment{
		{Name: "design-spec.pdf", Size: 1_482_000},
		{Name: "screenshot.png", Size: 220_000, Status: "uploading", Percent: 62},
		{Name: "huge-log.txt", Status: "error"},
	},
}

func senderPage(c *ui.Context) {
	d := &senderDemo
	gallery.Demo(c, "基础", "Enter 发送、Shift+Enter 换行；输入法组字时不发送。", func() {
		ui.Column(c, func() {
			if ui.Sender(c, &d.a).Submitted() {
				d.sent, d.a = d.a, ""
			}
			ui.Sender(c, &d.b)
			if d.sent != "" {
				ui.Text(c, "已发送："+d.sent).Tertiary()
			}
		}, layout.Gap(12), layout.W(640))
	})
	gallery.Demo(c, "附件、工具栏与加载", "附件条可移除；Toolbar 放模型选择等；Loading 时显示停止按钮。", func() {
		ui.Column(c, func() {
			ui.Sender(c, &d.c).Attachments(&d.files).AttachButton().Toolbar(func() {
				ui.Tag(c, "GPT-5")
				ui.Tag(c, "Agent")
			})
			ui.Sender(c, &d.d).Loading(true).Placeholder("正在生成回答…")
			ui.Sender(c, new(string)).Disabled(true).Placeholder("已禁用")
		}, layout.Gap(12), layout.W(640))
	})
}

// ---------------------------------------------------------------------------
// ThoughtChain

var chainSteps = []ui.ChainItem{
	{Key: "plan", Title: "理解问题", Description: "拆解为 3 个子任务", Status: ui.ChainSuccess, Duration: 800 * time.Millisecond,
		Detail: "用户想知道 **BubbleList** 的性能表现，需要：\n\n- 查看虚拟化实现\n- 跑基准测试\n- 总结结论"},
	{Key: "read", Title: "阅读 ui/chat_bubble_list.go", Status: ui.ChainSuccess, Duration: 1200 * time.Millisecond},
	{Key: "bench", Title: "运行基准测试", Description: "go test -bench BubbleList", Status: ui.ChainRunning, Duration: 3400 * time.Millisecond},
	{Key: "sum", Title: "总结结论", Status: ui.ChainPending},
}

func thoughtChainPage(c *ui.Context) {
	gallery.Demo(c, "步骤链", "每步显示状态、耗时；有详情的步骤可展开。", func() {
		ui.Column(c, func() {
			ui.ThoughtChain(c, chainSteps)
		}, layout.W(560))
	})
	gallery.Demo(c, "可折叠卡片", "Title 显示进度并可整体折叠；失败步骤用红色标记。", func() {
		ui.Column(c, func() {
			steps := append([]ui.ChainItem(nil), chainSteps[:2]...)
			steps = append(steps, ui.ChainItem{Key: "net", Title: "下载依赖", Description: "网络超时", Status: ui.ChainError, Duration: 30 * time.Second})
			ui.ThoughtChain(c, steps).Title("思考过程").Card()
			ui.ThoughtChain(c, chainSteps).Title("已折叠").Card().DefaultCollapsed()
		}, layout.Gap(12), layout.W(560))
	})
}

// ---------------------------------------------------------------------------
// ToolCall

func toolCallPage(c *ui.Context) {
	gallery.Demo(c, "工具调用", "标题栏显示工具名、摘要、耗时与状态；展开后查看参数与结果。", func() {
		ui.Column(c, func() {
			ui.ToolCall(c, "read_file").Summary("ui/chat_sender.go").Status(ui.ChainSuccess).Duration(85*time.Millisecond).
				Args(`{"path": "ui/chat_sender.go", "offset": 1, "limit": 40}`).
				Result("package ui\n\nimport (\n\t\"strings\"\n)").Lang("json", "go").DefaultOpen()
			ui.ToolCall(c, "run_shell").Summary("go test ./ui -run Chat").Status(ui.ChainRunning).Duration(4200 * time.Millisecond).Icon(icon.Terminal)
			ui.ToolCall(c, "web_fetch").Summary("https://example.com/api").Status(ui.ChainError).Duration(30 * time.Second).
				Args(`{"url": "https://example.com/api"}`).Result("timeout after 30s").DefaultOpen()
			ui.ToolCall(c, "search_code").Summary("BubbleList").Status(ui.ChainPending)
		}, layout.Gap(8), layout.W(600))
	})
}

// ---------------------------------------------------------------------------
// Approval

var approvalDemo = struct {
	note     string
	decision ui.ApprovalChoice
}{}

func approvalPage(c *ui.Context) {
	d := &approvalDemo
	gallery.Demo(c, "权限确认", "说明 + 详情 + 允许 / 拒绝 / 总是允许；决定后收起为结果行。", func() {
		ui.Column(c, func() {
			a := ui.Approval(c, "").Description("助手想要运行一条 shell 命令。").
				Field("工具", "run_shell").Field("目录", "~/work/tango").
				Command("go test ./ui/... -run Chat", "sh").Feedback(&d.note).Decision(d.decision)
			if ch, ok := a.Decided(); ok {
				d.decision = ch
			}
			ui.Approval(c, "删除文件").Description("将永久删除 build/ 目录下的 312 个文件。").Risk("error").
				Choices(ui.ApproveDeny, ui.ApproveOnce)
			ui.Approval(c, "写入文件").Description("修改 ui/chat_sender.go").Decision(ui.ApproveSession)
		}, layout.Gap(12), layout.W(600))
	})
}

// ---------------------------------------------------------------------------
// Conversations

var convDemo = struct {
	items  []ui.Conversation
	active string
}{active: "c2"}

func convItems(now time.Time) []ui.Conversation {
	if convDemo.items == nil {
		day := 24 * time.Hour
		convDemo.items = []ui.Conversation{
			{Key: "c1", Title: "BubbleList 虚拟化方案", Time: now.Add(-time.Hour), Running: true},
			{Key: "c2", Title: "Sender 输入法处理", Time: now.Add(-3 * time.Hour)},
			{Key: "c3", Title: "主题令牌命名", Time: now.Add(-day)},
			{Key: "c4", Title: "", Time: now.Add(-3 * day)},
			{Key: "c5", Title: "RTL 镜像检查清单", Time: now.Add(-12 * day)},
			{Key: "c6", Title: "无障碍审计结果", Time: now.Add(-90 * day), Disabled: true},
		}
	}
	return convDemo.items
}

func conversationsPage(c *ui.Context) {
	d := &convDemo
	gallery.Demo(c, "会话列表", "按时间分组；悬停出现重命名与删除；键盘 ↑↓ 切换、F2 重命名、Delete 删除。", func() {
		ui.Column(c, func() {
			cv := ui.Conversations(c, convItems(c.Now()), &d.active)
			if k, t, ok := cv.Renamed(); ok {
				for i := range d.items {
					if d.items[i].Key == k {
						d.items[i].Title = t
					}
				}
			}
			if k, ok := cv.Deleted(); ok {
				for i := range d.items {
					if d.items[i].Key == k {
						d.items = append(d.items[:i:i], d.items[i+1:]...)
						break
					}
				}
			}
		}, layout.W(280))
	})
}

// ---------------------------------------------------------------------------
// Prompts

var promptItems = []ui.PromptItem{
	{Key: "explain", Label: "解释这段代码", Description: "逐行说明选中代码的作用", Icon: icon.Bulb},
	{Key: "test", Label: "生成单元测试", Description: "覆盖边界条件与错误路径", Icon: icon.Bug},
	{Key: "refactor", Label: "重构建议", Description: "找出重复与可简化之处", Icon: icon.Tool},
	{Key: "doc", Label: "补充文档", Description: "为导出符号写注释", Icon: icon.FileText, Disabled: true},
}

var promptDemo string

func promptsPage(c *ui.Context) {
	gallery.Demo(c, "提示词卡片", "空会话或回答下方的推荐问题；可换行、纵向排列。", func() {
		ui.Column(c, func() {
			if k, ok := ui.Prompts(c, promptItems).Title("试试这样问").Wrap().Clicked(); ok {
				promptDemo = k
			}
			ui.Prompts(c, []ui.PromptItem{{Label: "继续"}, {Label: "换一种写法"}, {Label: "给出完整代码"}}).Vertical().Layout(layout.W(240))
		}, layout.Gap(16), layout.W(720))
	})
}

func init() {
	for _, p := range []gallery.Page{
		{Name: "Bubble", Title: "Bubble 对话气泡", Order: 80, Render: bubblePage},
		{Name: "BubbleList", Title: "BubbleList 消息流", Order: 81, Render: bubbleListPage},
		{Name: "Sender", Title: "Sender 对话输入框", Order: 82, Render: senderPage},
		{Name: "ThoughtChain", Title: "ThoughtChain 思考链", Order: 83, Render: thoughtChainPage},
		{Name: "ToolCall", Title: "ToolCall 工具调用", Order: 84, Render: toolCallPage},
		{Name: "Approval", Title: "Approval 权限确认", Order: 85, Render: approvalPage},
		{Name: "Conversations", Title: "Conversations 会话列表", Order: 86, Render: conversationsPage, Setup: func(d *ui.Driver) {
			if r, ok := d.Find("主题令牌命名"); ok {
				d.Move(geom.Pt(r.X+r.W/2, r.Y+r.H/2))
				d.Settle()
			}
		}},
		{Name: "Prompts", Title: "Prompts 提示词", Order: 87, Render: promptsPage},
	} {
		p.Group = gallery.Display
		gallery.Register(p)
	}
}
