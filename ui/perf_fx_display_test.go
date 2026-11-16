package ui

import (
	"fmt"
	"image"
	"image/color"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
)

// Only immutable IDs live globally. Values, image resources and viewer sources
// belong to each driver's Context, including drivers sharing a Large closure.
var perfDisplayID = hashID("perf.display.changed")

func perfDisplayChanged(c *Context) *bool { return State[bool](c, perfDisplayID, nil) }

func perfDisplayLabel(changed bool) string {
	if changed {
		return "已更新 Updated"
	}
	return "默认内容 Default"
}

func perfDisplayToggle(r *perfRun, i int) {
	*perfDisplayChanged(r.D.r.c) = i%2 == 0
	r.D.r.Invalidate()
	r.D.Frame()
}

func perfDisplayFixture(name string, build func(*Context, bool), covers ...string) perfFixture {
	return perfFixture{
		Name: name, Covers: covers,
		Build:    func(c *Context) { build(c, *perfDisplayChanged(c)) },
		Interact: perfDisplayToggle,
	}
}

func perfDisplayClick(r *perfRun, label string) {
	box, ok := r.D.Find(label)
	if !ok {
		panic("perf: display control not found: " + label)
	}
	r.D.Click(box.Center())
	r.D.Click(perfOutside) // restore hover and focus, not just the bound value
}

func perfDisplayImages(c *Context) *[2]*image.RGBA {
	return State(c, hashID("perf.display.images"), func() [2]*image.RGBA {
		var imgs [2]*image.RGBA
		for v := range imgs {
			img := image.NewRGBA(image.Rect(0, 0, 96, 64))
			for y := 0; y < 64; y++ {
				for x := 0; x < 96; x++ {
					img.SetRGBA(x, y, color.RGBA{R: uint8(30 + x + v*80), G: uint8(60 + y), B: uint8(180 - v*90), A: 255})
				}
			}
			imgs[v] = img
		}
		return imgs
	})
}

// Collection Large builders consume the exact requested n. Nonvirtual APIs
// remain nonvirtual inside a bounded scroll region; no dataset is truncated.
func perfDisplayCollection(name string, n int, body func(*Context, int, bool)) perfFixture {
	page := func(n int) func(*Context) {
		return func(c *Context) {
			Scroll(c, func() { body(c, n, *perfDisplayChanged(c)) }, layout.Size(760, 360)).Always()
		}
	}
	return perfFixture{
		Name: name, Build: func(c *Context) { body(c, n, *perfDisplayChanged(c)) },
		Interact: perfDisplayToggle, Large: page,
	}
}

func perfDisplayAvatars(c *Context, n int, changed bool) {
	AvatarGroup(c).Max(4).Body(func() {
		for i := 0; i < n; i++ {
			label := "U" + strconv.Itoa(i+1)
			if i == 0 {
				label = perfDisplayLabel(changed)
			}
			Avatar(c, label).Size(32)
		}
	})
}

func perfDisplayDescriptions(c *Context, n int, changed bool) {
	items := make([]DescItem, n)
	for i := range items {
		items[i] = DescItem{Label: "字段 " + strconv.Itoa(i+1), Value: "云数据库 Database"}
	}
	if n > 0 {
		items[0].Value = perfDisplayLabel(changed)
	}
	Descriptions(c, items).Title("用户信息").Bordered().Column(3)
}

func perfDisplayTimeline(c *Context, n int, changed bool) {
	items := make([]TimelineItem, n)
	for i := range items {
		items[i] = TimelineItem{Content: "部署事件 Event " + strconv.Itoa(i+1), Time: "2026-01-01 09:00", Color: "success"}
	}
	if n > 0 {
		items[0].Content = perfDisplayLabel(changed)
	}
	Timeline(c, items).Show()
}

func perfDisplayCollapse(n int) func(*Context) {
	return func(c *Context) {
		open := State(c, hashID("perf.display.collapse"), func() []string { return []string{"0"} })
		items := make([]CollapseItem, n)
		for i := range items {
			items[i] = CollapseItem{Key: strconv.Itoa(i), Label: "面板 Panel " + strconv.Itoa(i+1), Body: func() { Text(c, "折叠面板内容 Panel content") }}
		}
		Scroll(c, func() { Collapse(c, open, items).Show() }, layout.Size(760, 360)).Always()
	}
}

func perfDisplayCarousel(n int) func(*Context) {
	return func(c *Context) {
		cur := State[int](c, hashID("perf.display.carousel"), nil)
		Carousel(c, n, func(i int) {
			Column(c, func() { Text(c, "幻灯片 Slide "+strconv.Itoa(i+1)).Size(24) }, layout.Grow(1), layout.Center()).Bg(c.Theme.Primary.Bg)
		}).Current(cur).Autoplay(3 * time.Second).Arrows("always").Height(200).Show()
	}
}

func perfDisplaySegments(n int) func(*Context) {
	return func(c *Context) {
		*State[bool](c, hashID("perf.display.rtl"), nil) = c.IsRTL()
		val := State(c, hashID("perf.display.segment"), func() string { return "0" })
		opts := make([]Option, n)
		for i := range opts {
			opts[i] = Option{Label: "视图 " + strconv.Itoa(i+1), Value: strconv.Itoa(i)}
		}
		ScrollH(c, func() { Segmented(c, val, opts) }, layout.Size(760, 100)).Always()
	}
}

func perfDisplayJSONData(c *Context, n int) *[]byte {
	return State(c, hashID("perf.display.json.bytes"), func() []byte {
		var b strings.Builder
		b.WriteString(`{"items":[`)
		for i := 0; i < n; i++ {
			if i > 0 {
				b.WriteByte(',')
			}
			fmt.Fprintf(&b, `{"id":%d,"name":"规则 Rule %d","active":true}`, i, i)
		}
		b.WriteString(`],"version":1}`)
		return []byte(b.String())
	})
}

type perfDisplayJSONItem struct {
	ID   int
	Name string
}

func perfDisplayJSON(name string) perfFixture {
	page := func(n int) func(*Context) {
		return func(c *Context) {
			// These stable per-driver resources avoid reparsing/replacing identity
			// on unchanged frames. Query is the reversible visible interaction.
			query := State[string](c, hashID("perf.display.json.query"), nil)
			var w *JSONViewW
			switch name {
			case "JSONView":
				w = JSONView(c, *perfDisplayJSONData(c, n))
			case "JSONViewSource":
				src := State(c, hashID("perf.display.json.source"), func() ValueSource {
					s, err := JSONSource(*perfDisplayJSONData(c, n))
					if err != nil {
						panic(err)
					}
					return s
				})
				w = JSONViewSource(c, *src)
			case "JSONViewValue":
				val := State(c, hashID("perf.display.json.value"), func() []perfDisplayJSONItem {
					v := make([]perfDisplayJSONItem, n)
					for i := range v {
						v[i].ID, v[i].Name = i, "规则 Rule "+strconv.Itoa(i)
					}
					return v
				})
				w = JSONViewValue(c, val)
			}
			w.Height(320).ExpandDepth(2).ShowTypes().Query(query).Layout(layout.W(760))
		}
	}
	return perfFixture{Name: name, Build: page(12), Large: page, Interact: func(r *perfRun, i int) {
		q := State[string](r.D.r.c, hashID("perf.display.json.query"), nil)
		// A real no-result search is reversible without retaining search's
		// revealChain expansion, cursor and scroll side effects.
		*q = "不存在 Missing"
		if i%2 == 1 {
			*q = ""
		}
		r.D.r.Invalidate()
		r.D.Frame()
	}}
}

func perfDisplayDocumentFixture(name string, build func(*Context, bool)) perfFixture {
	fx := perfDisplayFixture(name, build)
	fx.Large = func(n int) func(*Context) { return perfDisplayDocument(name, n) }
	return fx
}

func perfDisplayDocument(name string, n int) func(*Context) {
	// Immutable source definitions may be shared by drivers; editable spans
	// and all component caches are still per-Context.
	var sources [2]string
	for i := 0; name != "RichText" && i < len(sources); i++ {
		var b strings.Builder
		for j := 0; j < n; j++ {
			if name == "Markdown" {
				fmt.Fprintf(&b, "段落 %d：**中英混排** Markdown document.\n\n", j+1)
			} else {
				fmt.Fprintf(&b, "var rule%d = %d // 规则 Rule\n", j+1, j+1)
			}
		}
		sources[i] = "// " + perfDisplayLabel(i == 1) + "\n" + b.String()
	}
	return func(c *Context) {
		i := 0
		if *perfDisplayChanged(c) {
			i = 1
		}
		switch name {
		case "CodeBlock":
			CodeBlock(c, sources[i], layout.W(760)).Lang("go").Height(320)
		case "Markdown":
			Scroll(c, func() { Markdown(c, sources[i], layout.W(640)) }, layout.Size(760, 360)).Always()
		case "RichText":
			spans := State(c, hashID("perf.display.spans"), func() []Span {
				v := make([]Span, n)
				for j := range v {
					v[j] = Span{Text: "中英混排 Rich text " + strconv.Itoa(j+1) + "\n", Bold: j%2 == 0}
				}
				return v
			})
			if n > 0 {
				(*spans)[0].Text = perfDisplayLabel(i == 1) + "\n"
			}
			Scroll(c, func() { RichText(c, (*spans)...).Layout(layout.W(640)) }, layout.Size(760, 360)).Always()
		}
	}
}

func init() {
	avatars := perfDisplayCollection("AvatarGroup", 6, perfDisplayAvatars)
	// Max(4) is the real API's overflow behavior, not a fixture-side cap:
	// AvatarGroup still builds n avatars before folding them. Toggle a visible
	// avatar instead of wheel-scrolling a naturally one-row folded group.
	avatars.Large = func(n int) func(*Context) {
		return func(c *Context) { perfDisplayAvatars(c, n, *perfDisplayChanged(c)) }
	}
	avatars.LargeInteract = perfDisplayToggle
	countdown := perfDisplayFixture("Countdown", func(c *Context, changed bool) {
		Countdown(c, perfDisplayLabel(changed), time.Date(2026, 1, 3, 13, 30, 12, 0, time.UTC))
	})
	countdown.Animated = "Live countdown schedules WakeAt for the next displayed second even with NoAnimations."
	timer := perfDisplayFixture("StatisticTimer", func(c *Context, changed bool) {
		StatisticTimer(c, perfDisplayLabel(changed), TimerCountup, time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC))
	})
	timer.Animated = "Live countup schedules WakeAt for each displayed second even with NoAnimations."
	carousel := perfFixture{Name: "Carousel", Build: perfDisplayCarousel(4), Large: perfDisplayCarousel,
		Animated: "Autoplay schedules WakeAt every 3 seconds; NoAnimations removes transitions, not content advances.",
		Interact: func(r *perfRun, i int) {
			label := "下一张"
			if i%2 == 1 {
				label = "上一张"
			}
			perfDisplayClick(r, label)
		}}
	carousel.LargeInteract = carousel.Interact
	// Skeleton shimmer and Spin rotation honor Context.still(): NoAnimations
	// freezes decoration. Unlike the three live timers above they should idle.
	registerPerf(
		perfDisplayFixture("Avatar", func(c *Context, changed bool) { Avatar(c, perfDisplayLabel(changed)).Size(40) }),
		avatars,
		perfDisplayFixture("Badge", func(c *Context, changed bool) {
			n := 5
			if changed {
				n = 12
			}
			Badge(c).Count(n).Body(func() { Avatar(c, "U").Square().Size(40) })
		}),
		perfDisplayFixture("BadgeRibbon", func(c *Context, changed bool) {
			BadgeRibbon(c, perfDisplayLabel(changed), "", "end", func() {
				Card(c, "推荐卡片").Layout(layout.W(300)).Body(func() { Text(c, "卡片内容 Card content") })
			})
		}),
		perfDisplayFixture("Tag", func(c *Context, changed bool) {
			Tag(c, perfDisplayLabel(changed)).Color("success").Icon(icon.CheckCircle)
		}),
		perfFixture{Name: "CheckableTag", Build: func(c *Context) {
			CheckableTag(c, "收藏", State[bool](c, hashID("perf.display.tag"), nil)).Icon(icon.Star)
		}, Interact: func(r *perfRun, _ int) { perfDisplayClick(r, "收藏") }},
		perfDisplayFixture("Empty", func(c *Context, changed bool) {
			Empty(c).Description(perfDisplayLabel(changed)).Body(func() { Button(c, "立即创建").Primary() })
		}),
		perfDisplayFixture("Image", func(c *Context, changed bool) {
			i := 0
			if changed {
				i = 1
			}
			Image(c, perfDisplayImages(c)[i]).Size(240, 160).Fit("cover").Alt("风景 Scene")
		}),
		perfDisplayFixture("QRCode", func(c *Context, changed bool) { QRCode(c, "tango:"+perfDisplayLabel(changed)).Size(160).Level("M") }),
		perfDisplayFixture("Statistic", func(c *Context, changed bool) {
			v := 112893.0
			if changed {
				v = 113012
			}
			Statistic(c, "账户余额 (CNY)", v, 2).Prefix("¥")
		}), countdown, timer,
		perfDisplayFixture("Progress", func(c *Context, changed bool) {
			v := 30.0
			if changed {
				v = 70
			}
			Column(c, func() { Progress(c, v); Progress(c, v).Circle(120) }, layout.W(420), layout.Gap(16))
		}),
		perfDisplayFixture("Spin", func(c *Context, changed bool) { Spin(c).Tip(perfDisplayLabel(changed)).Size(Large) }),
		perfDisplayFixture("Loading", func(c *Context, changed bool) {
			Loading(c, !changed).Tip("加载中 Loading").Body(func() { Text(c, "正在获取订单信息 Order details").Layout(layout.Size(400, 120)) })
		}),
		perfDisplayFixture("Skeleton", func(c *Context, changed bool) {
			n := 3
			if changed {
				n = 4
			}
			Column(c, func() { Skeleton(c).Avatar(false).Rows(n).Active().Show() }, layout.W(480))
		}),
		perfDisplayFixture("SkeletonAvatar", func(c *Context, changed bool) { SkeletonAvatar(c, 40, changed, true) }),
		perfDisplayFixture("SkeletonButton", func(c *Context, changed bool) {
			shape := "default"
			if changed {
				shape = "round"
			}
			SkeletonButton(c, Middle, shape, true)
		}),
		perfDisplayFixture("SkeletonImage", func(c *Context, changed bool) {
			w := float32(96)
			if changed {
				w = 128
			}
			SkeletonImage(c, w, 96, false)
		}),
		perfDisplayFixture("SkeletonInput", func(c *Context, changed bool) {
			sz := Middle
			if changed {
				sz = Large
			}
			SkeletonInput(c, sz, true)
		}),
		perfDisplayFixture("Result", func(c *Context, changed bool) {
			Column(c, func() {
				Result(c, "success", perfDisplayLabel(changed)).SubTitle("订单已提交，预计 1–5 分钟完成。").Extra(func() { Button(c, "返回列表").Primary() }).Show()
			}, layout.W(640))
		}),
		perfDisplayFixture("Alert", func(c *Context, changed bool) {
			Alert(c, "success", perfDisplayLabel(changed)).Description("成功提示的辅助性文字介绍。").Show()
		}),
		perfDisplayFixture("Card", func(c *Context, changed bool) {
			Card(c, "默认卡片").Layout(layout.W(300)).Actions(
				func() { CardAction(c, icon.Setting, "设置") }, func() { CardAction(c, icon.Edit, "编辑") },
			).Body(func() { CardMeta(c, func() { Avatar(c, "T") }, perfDisplayLabel(changed), "This is the description") })
		}, "CardMeta", "CardAction"),
		perfDisplayCollection("Descriptions", 6, perfDisplayDescriptions),
		perfDisplayCollection("Timeline", 4, perfDisplayTimeline),
		perfFixture{Name: "Tooltip", Overlay: true, Build: func(c *Context) {
			t := Tooltip(c, "这是一段文字提示 Tooltip", func() { Button(c, "查看提示") }).Trigger("click").Placement("bottom")
			// Tooltip has no controlled Open API. Seed its actual retained
			// click state before its lazy build, not a replacement overlay.
			key := hashID(t.ID(), "tooltip")
			State(c, key, func() tooltipState { return tooltipState{open: true} })
			*State[ID](c, hashID("perf.display.tooltip.key"), nil) = key
		}, Interact: func(r *perfRun, i int) {
			c := r.D.r.c
			key := *State[ID](c, hashID("perf.display.tooltip.key"), nil)
			State[tooltipState](c, key, nil).open = i%2 == 1
			r.D.r.Invalidate()
			r.D.Frame()
		}},
		perfFixture{Name: "Popover", Overlay: true, Build: func(c *Context) {
			open := State(c, hashID("perf.display.popover"), func() bool { return true })
			Popover(c).Title("标题").Text("气泡卡片内容 More information").Trigger("manual").Open(open).Placement("bottom").Body(func() { Button(c, "气泡卡片") })
		}, Interact: func(r *perfRun, i int) {
			*State(r.D.r.c, hashID("perf.display.popover"), func() bool { return true }) = i%2 == 1
			r.D.r.Invalidate()
			r.D.Frame()
		}},
		perfFixture{Name: "Popconfirm", Overlay: true, Build: func(c *Context) {
			p := Popconfirm(c, "删除任务", "确定要删除这个任务吗？", func() { Button(c, "删除").Danger() }).Placement("bottom")
			// Popconfirm likewise exposes no controlled Open method. Keep the
			// real component state in this driver's Context and reopen it there.
			State(c, p.b.ID(), func() popState { return popState{open: true} })
			*State[ID](c, hashID("perf.display.popconfirm.key"), nil) = p.b.ID()
		}, Interact: func(r *perfRun, i int) {
			c := r.D.r.c
			key := *State[ID](c, hashID("perf.display.popconfirm.key"), nil)
			State[popState](c, key, nil).open = i%2 == 1
			r.D.r.Invalidate()
			r.D.Frame()
		}},
		perfFixture{Name: "Collapse", Build: perfDisplayCollapse(3), Large: perfDisplayCollapse,
			Interact: func(r *perfRun, _ int) { perfDisplayClick(r, "面板 Panel 1") }},
		carousel,
		perfDisplayFixture("Calendar", func(c *Context, changed bool) {
			v := State(c, hashID("perf.display.calendar"), func() time.Time { return time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC) })
			day := 15
			if changed {
				day = 16
			}
			*v = time.Date(2026, 1, day, 0, 0, 0, 0, time.UTC)
			Column(c, func() { Calendar(c, v).Card() }, layout.W(320))
		}),
		perfFixture{Name: "Segmented", Build: perfDisplaySegments(3), Large: perfDisplaySegments,
			Interact: func(r *perfRun, i int) {
				label := "视图 2"
				if i%2 == 1 {
					label = "视图 1"
				}
				perfDisplayClick(r, label)
			}, LargeInteract: func(r *perfRun, i int) {
				dx := float32(120)
				if i%40 >= 20 {
					dx = -dx
				}
				if *State[bool](r.D.r.c, hashID("perf.display.rtl"), nil) {
					dx = -dx
				}
				r.D.Inject(input.Scroll{Pos: r.Center(), DX: dx})
			}},
		perfFixture{Name: "Rate", Build: func(c *Context) {
			Rate(c, State(c, hashID("perf.display.rate"), func() float64 { return 2.5 })).AllowHalf().ShowScore()
		},
			Interact: func(r *perfRun, i int) {
				*State(r.D.r.c, hashID("perf.display.rate"), func() float64 { return 2.5 }) = 4
				if i%2 == 1 {
					*State[float64](r.D.r.c, hashID("perf.display.rate"), nil) = 2.5
				}
				r.D.r.Invalidate()
				r.D.Frame()
			}},
		perfDisplayDocumentFixture("Markdown", func(c *Context, changed bool) {
			Markdown(c, "# "+perfDisplayLabel(changed)+"\n\n中英混合 **Markdown** 文档。\n\n- 规则一\n- 规则二\n\n```go\nfmt.Println(\"hello\")\n```", layout.W(640))
		}),
		perfDisplayDocumentFixture("RichText", func(c *Context, changed bool) {
			RichText(c, Span{Text: perfDisplayLabel(changed), Bold: true}, Span{Text: "：中英混排 Rich text "}, Span{Text: "inline code", Code: true}, Span{Text: " 高亮", Mark: true}).Layout(layout.W(640))
		}),
		perfDisplayDocumentFixture("CodeBlock", func(c *Context, changed bool) {
			CodeBlock(c, "package main\n\nfunc main() {\n\tprintln(\""+perfDisplayLabel(changed)+"\")\n}\n", layout.W(760)).Lang("go").Title("main.go").Height(280)
		}),
		perfDisplayJSON("JSONView"), perfDisplayJSON("JSONViewSource"), perfDisplayJSON("JSONViewValue"),
	)
}

func TestPerfDisplayContextIsolation(t *testing.T) {
	for _, name := range []string{"Image", "Carousel", "JSONViewSource"} {
		t.Run(name, func(t *testing.T) {
			fx := perfRegistry[name]
			a, b := newPerfDriver(perfPage(fx.Build), false), newPerfDriver(perfPage(fx.Build), false)
			a.Settle()
			b.Settle()
			h := perfListHash(b)
			perfStep(&perfRun{D: a}, fx.Interact, 0)
			b.Frame()
			if perfListHash(b) != h {
				t.Fatal("interaction leaked into another driver")
			}
			if perfListHash(a) == h {
				t.Fatal("interaction did not change its own driver")
			}
		})
	}
}

func TestPerfDisplayOverlaysInitiallyOpen(t *testing.T) {
	for _, name := range []string{"Tooltip", "Popover", "Popconfirm"} {
		t.Run(name, func(t *testing.T) {
			d := newPerfDriver(perfPage(perfRegistry[name].Build), false)
			d.Frame()
			if len(d.r.c.overlays) == 0 {
				t.Fatal("overlay absent from first build")
			}
		})
	}
}

func TestPerfDisplayImagePixels(t *testing.T) {
	fx := perfRegistry["Image"]
	d := newPerfDriver(perfPage(fx.Build), false)
	d.Settle()
	imgs := perfDisplayImages(d.r.c)
	if imgs[0].Bounds() != imgs[1].Bounds() || imgs[0].At(20, 20) == imgs[1].At(20, 20) {
		t.Fatal("image variants must differ in pixels, not just bounds")
	}
	h := perfListHash(d)
	perfStep(&perfRun{D: d}, fx.Interact, 0)
	if perfListHash(d) == h {
		t.Fatal("real pixel content did not change")
	}
}

func TestPerfDisplayLargeData(t *testing.T) {
	const n = 1000
	for name, last := range map[string]string{
		"Descriptions": "字段 1000", "Timeline": "部署事件 Event 1000",
		"Collapse": "面板 Panel 1000", "Segmented": "视图 1000",
	} {
		t.Run(name, func(t *testing.T) {
			d := newPerfDriver(perfPage(perfRegistry[name].Large(n)), false)
			d.Settle()
			if _, ok := d.FindText(last); !ok {
				t.Fatalf("requested item %d was not built", n)
			}
		})
	}
	for _, name := range []string{"JSONView", "JSONViewSource", "JSONViewValue", "RichText", "CodeBlock", "Markdown"} {
		t.Run(name, func(t *testing.T) {
			d := newPerfDriver(perfPage(perfRegistry[name].Large(n)), false)
			d.Settle()
			switch name {
			case "JSONView", "JSONViewSource":
				if got := strings.Count(string(*perfDisplayJSONData(d.r.c, n)), `"id":`); got != n {
					t.Fatalf("JSON contains %d items, want %d", got, n)
				}
			case "JSONViewValue":
				if got := len(*State[[]perfDisplayJSONItem](d.r.c, hashID("perf.display.json.value"), nil)); got != n {
					t.Fatalf("Go value contains %d items, want %d", got, n)
				}
			case "RichText":
				if got := len(*State[[]Span](d.r.c, hashID("perf.display.spans"), nil)); got != n {
					t.Fatalf("RichText contains %d spans, want %d", got, n)
				}
			case "CodeBlock":
				var found bool
				for _, entry := range d.r.c.states {
					if st, ok := entry.v.(*codeBlockState); ok && strings.Contains(st.src, "var rule1000 = 1000") {
						found = true
					}
				}
				if !found {
					t.Fatal("last requested source line absent")
				}
			case "Markdown":
				if _, ok := d.FindText("段落 1000"); !ok {
					t.Fatal("last requested paragraph absent")
				}
			}
		})
	}
}
