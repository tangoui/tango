package main

import (
	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/graph"
)

var (
	erTables = []graph.Table{
		{Name: "users", Comment: "用户", Columns: []graph.Column{
			{Name: "id", Type: "bigint", PK: true}, {Name: "email", Type: "varchar(255)"}, {Name: "nickname", Type: "varchar(64)"}, {Name: "created_at", Type: "timestamptz"},
		}},
		{Name: "addresses", Columns: []graph.Column{
			{Name: "id", Type: "bigint", PK: true}, {Name: "user_id", Type: "bigint"}, {Name: "city", Type: "varchar(64)"}, {Name: "detail", Type: "text"},
		}},
		{Name: "orders", Comment: "订单", Columns: []graph.Column{
			{Name: "id", Type: "bigint", PK: true}, {Name: "user_id", Type: "bigint"}, {Name: "address_id", Type: "bigint"},
			{Name: "status", Type: "smallint"}, {Name: "total", Type: "numeric(12,2)"}, {Name: "created_at", Type: "timestamptz"},
		}},
		{Name: "order_items", Columns: []graph.Column{
			{Name: "order_id", Type: "bigint", PK: true}, {Name: "product_id", Type: "bigint", PK: true}, {Name: "quantity", Type: "int"}, {Name: "price", Type: "numeric(12,2)"},
		}},
		{Name: "products", Comment: "商品", Columns: []graph.Column{
			{Name: "id", Type: "bigint", PK: true}, {Name: "category_id", Type: "int"}, {Name: "name", Type: "varchar(128)"}, {Name: "price", Type: "numeric(12,2)"}, {Name: "stock", Type: "int"},
		}},
		{Name: "categories", Columns: []graph.Column{
			{Name: "id", Type: "int", PK: true}, {Name: "parent_id", Type: "int"}, {Name: "name", Type: "varchar(64)"},
		}},
		{Name: "payments", Columns: []graph.Column{
			{Name: "id", Type: "bigint", PK: true}, {Name: "order_id", Type: "bigint"}, {Name: "channel", Type: "varchar(16)"}, {Name: "paid_at", Type: "timestamptz"},
		}},
		{Name: "reviews", Columns: []graph.Column{
			{Name: "id", Type: "bigint", PK: true}, {Name: "product_id", Type: "bigint"}, {Name: "user_id", Type: "bigint"}, {Name: "rating", Type: "smallint"}, {Name: "body", Type: "text"},
		}},
	}
	erRelations = []graph.Relation{
		{From: "addresses", FromColumn: "user_id", To: "users", ToColumn: "id"},
		{From: "orders", FromColumn: "user_id", To: "users", ToColumn: "id"},
		{From: "orders", FromColumn: "address_id", To: "addresses", ToColumn: "id"},
		{From: "order_items", FromColumn: "order_id", To: "orders", ToColumn: "id"},
		{From: "order_items", FromColumn: "product_id", To: "products", ToColumn: "id"},
		{From: "products", FromColumn: "category_id", To: "categories", ToColumn: "id"},
		{From: "categories", FromColumn: "parent_id", To: "categories", ToColumn: "id"},
		{From: "payments", FromColumn: "order_id", To: "orders", ToColumn: "id"},
		{From: "reviews", FromColumn: "product_id", To: "products", ToColumn: "id"},
		{From: "reviews", FromColumn: "user_id", To: "users", ToColumn: "id"},
	}
	erView *graph.DiagramView
)

func init() {
	graphPage(gallery.Page{Name: "ERDiagram", Title: "ER 图（graph.ER）", Order: 301,
		Render: func(c *ui.Context) {
			erView = graph.ER(c, erTables, erRelations).Height(620).MiniMap().Grid(20).Show()
		},
		Setup: func(d *ui.Driver) {
			if erView == nil {
				return
			}
			if r, ok := erView.ScreenRect("orders"); ok {
				d.Click(r.Center())
			}
		},
	})
}
