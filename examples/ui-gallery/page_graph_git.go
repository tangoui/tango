package main

import (
	"fmt"
	"hash/fnv"
	"time"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/graph"
	"github.com/tangoui/tango/ui/layout"
)

// demoHistory generates a deterministic history of n commits ending at
// now: a main line with feature branches that fork, advance and merge
// back, tags every few hundred commits.
func demoHistory(n int, now time.Time) []graph.Commit {
	authors := []string{"林小鹿", "Alice Chen", "王建国", "Bob Smith", "赵敏", "Carol Diaz"}
	verbs := []string{"修复", "新增", "重构", "优化", "更新", "移除"}
	things := []string{"登录页校验", "订单导出", "缓存失效策略", "支付回调重试", "暗色主题配色", "构建脚本", "日志采样", "数据库连接池", "图表 Tooltip", "权限菜单"}
	seed := uint32(2463534242)
	rnd := func(k int) int {
		seed ^= seed << 13
		seed ^= seed >> 17
		seed ^= seed << 5
		return int(seed % uint32(k))
	}
	hash := func(i int) string {
		h := fnv.New64a()
		fmt.Fprintf(h, "commit-%d", i)
		a := h.Sum64()
		fmt.Fprintf(h, "x")
		return fmt.Sprintf("%016x%016x%08x", a, h.Sum64(), uint32(a>>7))
	}
	type branch struct {
		name string
		tip  string
	}
	var made []graph.Commit // oldest first
	add := func(parents []string) string {
		i := len(made)
		h := hash(i)
		made = append(made, graph.Commit{Hash: h, Parents: parents, Author: authors[rnd(len(authors))],
			Subject: verbs[rnd(len(verbs))] + things[rnd(len(things))]})
		return h
	}
	mainTip := add(nil)
	var feats []branch
	featN := 0
	tags := map[string]string{}
	for len(made) < n {
		switch r := rnd(100); {
		case r < 8 && len(feats) < 5:
			featN++
			feats = append(feats, branch{fmt.Sprintf("feature/f%d", featN), mainTip})
		case r < 16 && len(feats) > 0:
			k := rnd(len(feats))
			mainTip = add([]string{mainTip, feats[k].tip})
			made[len(made)-1].Subject = "Merge branch '" + feats[k].name + "'"
			feats = append(feats[:k], feats[k+1:]...)
		case r < 45 && len(feats) > 0:
			k := rnd(len(feats))
			feats[k].tip = add([]string{feats[k].tip})
		default:
			mainTip = add([]string{mainTip})
			if len(made)%250 == 0 {
				tags[mainTip] = fmt.Sprintf("v1.%d.0", len(made)/250)
			}
		}
	}
	out := make([]graph.Commit, len(made))
	for i := range made {
		c := made[len(made)-1-i]
		c.Time = now.Add(-time.Duration(i)*47*time.Minute - time.Duration(i%7)*3*time.Minute)
		out[i] = c
	}
	for i := range out {
		c := &out[i]
		if c.Hash == mainTip {
			c.Refs = append(c.Refs, graph.Ref{Name: "main", Kind: graph.RefHead}, graph.Ref{Name: "origin/main", Kind: graph.RefRemote})
		}
		for _, f := range feats {
			if c.Hash == f.tip {
				c.Refs = append(c.Refs, graph.Ref{Name: f.name, Kind: graph.RefBranch})
			}
		}
		if t, ok := tags[c.Hash]; ok {
			c.Refs = append(c.Refs, graph.Ref{Name: t, Kind: graph.RefTag})
		}
	}
	return out
}

var (
	gitCommits []graph.Commit
	gitSel     string
	gitView    *graph.GitGraphView
)

func init() {
	graphPage(gallery.Page{Name: "GitGraph", Title: "GitGraph 提交图（10 000 个提交）", Order: 302,
		Render: func(c *ui.Context) {
			if gitCommits == nil {
				gitCommits = demoHistory(10000, c.Now())
			}
			ui.Card(c, "").Layout(layout.Margin(geom.Insets{})).Body(func() {
				gitView = graph.GitGraph(c, gitCommits).Selected(&gitSel).Height(560).Show()
			})
		},
		Setup: func(d *ui.Driver) {
			if gitView == nil {
				return
			}
			r := gitView.Rect()
			d.Click(geom.Pt(r.X+r.W/2, r.Y+30*3+15))
			d.Move(geom.Pt(r.X+r.W/2, r.Y+30*6+15))
		},
	})
}
