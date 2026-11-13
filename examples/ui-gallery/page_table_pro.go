package main

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
)

// tbRule is a ProTable record.
type tbRule struct {
	key      string
	name     string
	owner    string
	calls    int
	amount   float64
	status   string
	tag      string
	progress float64
	rate     float64
	updated  time.Time
}

// tbRuleServer is a fake backend filtering, sorting and paging rules.
func tbRuleServer(all []tbRule, req ui.ProRequest) ([]tbRule, int, error) {
	if req.Params["name"] == "error" {
		return nil, 0, errors.New("服务器返回 500")
	}
	var out []tbRule
	for _, r := range all {
		if v := req.Params["name"]; v != "" && !strings.Contains(r.name, v) {
			continue
		}
		if v := req.Params["status"]; v != "" && r.status != v {
			continue
		}
		if fs := req.Filters["status"]; len(fs) > 0 {
			ok := false
			for _, f := range fs {
				ok = ok || f == r.status
			}
			if !ok {
				continue
			}
		}
		out = append(out, r)
	}
	for _, s := range req.Sort {
		s := s
		sort.SliceStable(out, func(i, j int) bool {
			var less bool
			switch s.Column {
			case "calls":
				less = out[i].calls < out[j].calls
			case "amount":
				less = out[i].amount < out[j].amount
			default:
				less = out[i].updated.Before(out[j].updated)
			}
			if s.Order == ui.SortDesc {
				return !less
			}
			return less
		})
	}
	total := len(out)
	a := min(total, (req.Page-1)*req.PageSize)
	b := min(total, a+req.PageSize)
	return out[a:b], total, nil
}

func tbRules(n int) []tbRule {
	ps := tbPeople(n)
	st := []string{"running", "online", "closed", "error"}
	tags := []string{"核心", "实验", "灰度"}
	out := make([]tbRule, n)
	for i, p := range ps {
		out[i] = tbRule{key: fmt.Sprint("rule-", i+1), name: fmt.Sprintf("TradeCode %d", 99-i), owner: p.name,
			calls: (i*7919)%1000 + 10, amount: p.amount, status: st[i%4], tag: tags[i%3], progress: p.score,
			rate: float64((i*37)%2000) / 100, updated: p.created}
	}
	return out
}

func init() {
	tbPage("ProTable", "ProTable 高级表格 · 查询列表", 109, 1200, 0, false, func() (func(*ui.Context), func(*ui.Driver)) {
		all := tbRules(57)
		sel := []string{"rule-2", "rule-3"}
		return func(c *ui.Context) {
				enum := []ui.ProEnum{{Value: "running", Label: "运行中", Status: "processing"}, {Value: "online", Label: "已上线", Status: "success"},
					{Value: "closed", Label: "关闭", Status: "default"}, {Value: "error", Label: "异常", Status: "error"}}
				tagEnum := []ui.ProEnum{{Value: "核心", Label: "核心", Color: "processing"}, {Value: "实验", Label: "实验", Color: "warning"}, {Value: "灰度", Label: "灰度", Color: "success"}}
				cols := []ui.ProColumn[tbRule]{
					{Key: "name", Title: "规则名称", Width: 140, ValueType: ui.ValueLink, Value: func(r tbRule) any { return r.name }},
					{Key: "owner", Title: "负责人", Width: 100, Value: func(r tbRule) any { return r.owner }},
					{Key: "status", Title: "状态", Width: 110, ValueType: ui.ValueStatus, ValueEnum: enum, Filterable: true, Value: func(r tbRule) any { return r.status }},
					{Key: "tag", Title: "标签", Width: 90, ValueType: ui.ValueTag, ValueEnum: tagEnum, HideInSearch: true, Value: func(r tbRule) any { return r.tag }},
					{Key: "calls", Title: "调用次数", Width: 116, ValueType: ui.ValueDigit, Sortable: true, HideInSearch: true, Value: func(r tbRule) any { return r.calls }},
					{Key: "amount", Title: "金额", Width: 130, ValueType: ui.ValueMoney, Sortable: true, HideInSearch: true, Value: func(r tbRule) any { return r.amount }},
					{Key: "progress", Title: "进度", Width: 130, ValueType: ui.ValueProgress, HideInSearch: true, Value: func(r tbRule) any { return r.progress }},
					{Key: "rate", Title: "转化率", Width: 90, Hidden: true, ValueType: ui.ValuePercent, HideInSearch: true, Value: func(r tbRule) any { return r.rate }},
					{Key: "updated", Title: "更新时间", Width: 180, ValueType: ui.ValueDateTime, Value: func(r tbRule) any { return r.updated }},
					{Key: "op", Title: "操作", ValueType: ui.ValueOption, Render: func(r tbRule, i int) {
						ui.Link(c, "配置")
						ui.Link(c, "订阅")
					}},
				}
				ui.Column(c, func() { tbPro(c, cols, all, &sel) }, layout.Pad(16)).Bg(c.Theme.FillTertiary).Radius(8)
			}, func(d *ui.Driver) {
				if r, ok := d.Find("调用次数"); ok {
					d.Click(r.Center())
				}
			}
	})
}

func tbPro(c *ui.Context, cols []ui.ProColumn[tbRule], all []tbRule, sel *[]string) {
	ui.ProTable(c, cols).
		Title("查询表格").
		RowKey(func(r tbRule) string { return r.key }).
		Selection(sel).
		ToolBar(func() {
			ui.Button(c, "新建").Primary().Icon(icon.Plus)
		}).
		Request(func(req ui.ProRequest, done func(ui.ProResult[tbRule])) {
			rows, total, err := tbRuleServer(all, req)
			done(ui.ProResult[tbRule]{Rows: rows, Total: total, Err: err})
		}).
		Show()
}
