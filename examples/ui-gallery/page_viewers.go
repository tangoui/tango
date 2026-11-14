package main

import (
	"fmt"
	"strings"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/codehl"
	"github.com/tangoui/tango/ui/layout"
)

// Data viewer pages: CodeBlock, LogView, JSONView. The code editor's
// highlighter colors CodeBlock and the Markdown code blocks.

const viewerGoSample = `package main

import (
	"fmt"
	"strings"
)

// Greeter 向调用者问好。
type Greeter struct {
	Name  string
	Times int
}

func (g *Greeter) Greet() string {
	var b strings.Builder
	for i := 0; i < g.Times; i++ {
		fmt.Fprintf(&b, "你好，%s！#%d\n", g.Name, i+1) // a very long line that scrolls sideways instead of wrapping, so the code keeps its shape
	}
	return b.String()
}`

const viewerTSSample = `export async function load(url: string): Promise<User[]> {
  const res = await fetch(url, { headers: { Accept: "application/json" } });
  if (!res.ok) throw new Error(` + "`HTTP ${res.status}`" + `);
  return (await res.json()) as User[];
}`

const viewerJSONSample = `{
  "name": "tango",
  "version": "0.4.0",
  "private": false,
  "license": null,
  "keywords": ["gui", "go", "自绘"],
  "author": {"name": "Tango Team", "email": "team@example.com", "url": "https://example.com"},
  "metrics": {"stars": 1280, "ratio": 0.875, "tags": {"stable": true, "beta": false}},
  "matrix": [[1, 2, 3], [4, 5, 6]],
  "empty": {},
  "items": [` + "%s" + `]
}`

var viewerDemo struct {
	logs      *ui.LogBuffer
	bigLogs   *ui.LogBuffer
	logQuery  string
	logWrap   bool
	json      []byte
	bigCode   string
	jsonQuery string
}

type viewerUser struct {
	ID      int               `json:"id"`
	Name    string            `json:"name"`
	Roles   []string          `json:"roles"`
	Profile *viewerProfile    `json:"profile"`
	Labels  map[string]string `json:"labels"`
	secret  string
}

type viewerProfile struct {
	Age    int     `json:"age"`
	Score  float64 `json:"score"`
	Active bool    `json:"active"`
}

var viewerUsers = []viewerUser{
	{ID: 1, Name: "张三", Roles: []string{"admin", "dev"}, Profile: &viewerProfile{Age: 31, Score: 98.5, Active: true}, Labels: map[string]string{"team": "core", "region": "cn"}},
	{ID: 2, Name: "Alice", Roles: nil, Labels: map[string]string{}},
}

func viewerLogLines(b *ui.LogBuffer) {
	b.Append(
		"\x1b[2m2026-11-14 09:00:00.120\x1b[0m \x1b[32mINFO \x1b[0m server: listening on \x1b[4mhttp://127.0.0.1:8080\x1b[24m",
		"\x1b[2m2026-11-14 09:00:00.348\x1b[0m \x1b[36mDEBUG\x1b[0m db: pool size=8 idle=8",
		"\x1b[2m2026-11-14 09:00:01.002\x1b[0m \x1b[33mWARN \x1b[0m cache: miss ratio \x1b[1m42%\x1b[22m above threshold",
		"\x1b[2m2026-11-14 09:00:01.517\x1b[0m \x1b[31mERROR\x1b[0m handler: request failed: \x1b[1;31mconnection reset by peer\x1b[0m",
		"    at net.(*conn).Read (net.go:183)",
		"    at main.handle (main.go:42)",
		"2026-11-14 09:00:02.000 INFO  build: \x1b[38;5;208m256 色\x1b[0m、\x1b[38;2;120;80;200m真彩色\x1b[0m、\x1b[7m反显\x1b[27m、\x1b[9m删除线\x1b[29m、\x1b[3m斜体\x1b[23m",
		"2026-11-14 09:00:02.410 TRACE scheduler: tick 1024 (queue=0)",
		"2026-11-14 09:00:03.000 WARN  这是一条很长的中文日志，用来演示自动换行：当一行日志超过视口宽度时，打开“自动换行”会把它折成多行显示，关闭时则可以横向滚动查看完整内容。",
		"2026-11-14 09:00:03.250 INFO  progress 10%\r2026-11-14 09:00:03.250 INFO  progress 100% done",
	)
}

func init() {
	ui.SetCodeHighlighter(codehl.New())
	viewerDemo.logs = ui.NewLogBuffer(0)
	viewerLogLines(viewerDemo.logs)
	viewerDemo.logQuery = "conn"
	var items []string
	for i := 0; i < 250; i++ {
		items = append(items, fmt.Sprintf(`{"id": %d, "ok": %t}`, i, i%3 != 0))
	}
	viewerDemo.json = []byte(fmt.Sprintf(viewerJSONSample, strings.Join(items, ", ")))

	gallery.Register(gallery.Page{Name: "CodeBlock", Title: "CodeBlock 代码块", Group: gallery.Display, Order: 72, Width: 1000,
		Render: func(c *ui.Context) {
			gallery.Demo(c, "带标题与行号", "Lang 选择语法高亮；LineNumbers 显示行号；HighlightLines 标出行；长行横向滚动；可拖动选择并 Cmd/Ctrl+C 复制。", func() {
				ui.CodeBlock(c, viewerGoSample).Lang("go").Title("main.go").LineNumbers(true).HighlightLines(16, 17)
			})
			gallery.Demo(c, "无标题", "复制按钮在悬停时出现于右上角；Markdown 的代码块也走同一套高亮。", func() {
				ui.Row(c, func() {
					ui.CodeBlock(c, viewerTSSample).Lang("ts").Layout(layout.Grow(1), layout.Basis(0))
					ui.Markdown(c, "```json\n{\"ok\": true, \"count\": 3}\n```", layout.Grow(1), layout.Basis(0))
				}, layout.Gap(16), layout.AlignItems(layout.AlignStart))
			})
		},
	})
	gallery.Register(gallery.Page{Name: "CodeBlockLarge", Title: "CodeBlock 大文件", Group: gallery.Display, Order: 73, Parent: "CodeBlock", Width: 1000,
		Render: func(c *ui.Context) {
			if viewerDemo.bigCode == "" {
				var b strings.Builder
				for i := 0; i < 100_000; i++ {
					fmt.Fprintf(&b, "func f%d(x int) int { return x*%d + 1 } // line %d\n", i, i, i+1)
				}
				viewerDemo.bigCode = b.String()
			}
			gallery.Demo(c, "10 万行", "MaxHeight 限高后只排版可见行；行号列固定不随横向滚动。", func() {
				ui.CodeBlock(c, viewerDemo.bigCode).Lang("go").LineNumbers(true).MaxHeight(320).Title("big.go")
			})
		},
	})
	gallery.Register(gallery.Page{Name: "LogView", Title: "LogView 日志查看器", Group: gallery.Display, Order: 74, Width: 1000,
		Render: func(c *ui.Context) {
			gallery.Demo(c, "ANSI 颜色、级别与搜索", "LogBuffer 解析 SGR（16/256/真彩色、粗体、下划线、反显）；警告/错误行按级别着色；工具栏搜索、级别过滤、自动换行、跟随底部、清空。", func() {
				ui.LogView(c, viewerDemo.logs).Height(320).LineNumbers(true).Query(&viewerDemo.logQuery).Wrap(&viewerDemo.logWrap)
			})
		},
	})
	gallery.Register(gallery.Page{Name: "LogViewLarge", Title: "LogView 百万行", Group: gallery.Display, Order: 75, Parent: "LogView", Width: 1000,
		Render: func(c *ui.Context) {
			if viewerDemo.bigLogs == nil {
				viewerDemo.bigLogs = ui.NewLogBuffer(1_000_000)
				lv := []string{"INFO ", "DEBUG", "WARN ", "ERROR"}
				lines := make([]string, 0, 1_000_000)
				for i := 0; i < 1_000_000; i++ {
					lines = append(lines, fmt.Sprintf("2026-11-14 10:%02d:%02d.%03d %s worker-%d: processed job %d", i/60000%60, i/1000%60, i%1000, lv[i%7%4], i%16, i))
				}
				viewerDemo.bigLogs.Append(lines...)
			}
			gallery.Demo(c, "1,000,000 行", "只排版可见行；跟随底部时追加的新行保持在视口内，上翻后停止跟随。", func() {
				ui.LogView(c, viewerDemo.bigLogs).Height(320).LineNumbers(true)
			})
		},
	})
	gallery.Register(gallery.Page{Name: "JSONView", Title: "JSONView 值树查看器", Group: gallery.Display, Order: 76, Width: 1000,
		Render: func(c *ui.Context) {
			gallery.Demo(c, "JSON 文档", "按类型着色、显示条目数；大数组每 100 项一组；右键或 Cmd/Ctrl+C、Cmd/Ctrl+Shift+C 复制值或路径（$.a.b[0]）；搜索键和值。", func() {
				ui.JSONView(c, viewerDemo.json).Height(360).ExpandDepth(2).Query(&viewerDemo.jsonQuery)
			})
			gallery.Demo(c, "Go 值", "JSONViewValue 反射展开结构体（json 标签命名）、map（键排序）、切片与指针；ShowTypes 显示类型。", func() {
				ui.JSONViewValue(c, &viewerUsers).Height(240).ExpandDepth(3).NoToolbar().ShowTypes()
			})
		},
	})
}
