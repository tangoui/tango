package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/geom"
)

func TestEntryUploadPickRemoveMaxCount(t *testing.T) {
	files := []UploadFile{{Name: "a.txt", Status: "done"}, {Name: "b.txt", Status: "error"}}
	picks, removed := 0, -1
	d := NewDriver(func(c *Context) {
		u := Upload(c, &files).MaxCount(3).OnPick(func() {
			picks++
			files = append(files, UploadFile{Name: "new.txt", Status: "uploading", Percent: 10})
		}).Width(300)
		if i := u.Removed(); i >= 0 {
			removed = i
		}
	}, DriverOptions{Width: 600, Height: 400})
	d.Frame()
	d.ClickLabel("点击上传")
	if picks != 1 || len(files) != 3 {
		t.Fatalf("picks=%d files=%d", picks, len(files))
	}
	d.ClickLabel("点击上传") // max count reached: button disabled
	if picks != 1 {
		t.Fatal("picked beyond MaxCount")
	}
	r := mustFind(t, d, "b.txt")
	d.Move(geom.Pt(r.Right()-10, r.Y+8))
	d.Click(geom.Pt(r.Right()-10, r.Y+8))
	if removed != 1 || len(files) != 2 || files[1].Name != "new.txt" {
		t.Fatalf("removed=%d files=%v", removed, files)
	}
	// Clicking the name does not remove.
	d.ClickLabel("a.txt")
	if len(files) != 2 {
		t.Fatal("click on name removed file")
	}
}

func TestEntryUploadCardsAndDisabled(t *testing.T) {
	files := []UploadFile{{Name: "x.png", Status: "done"}}
	picks, prev := 0, -1
	dis := false
	d := NewDriver(func(c *Context) {
		u := Upload(c, &files).PictureCard().MaxCount(2).Disabled(dis).OnPick(func() { picks++ })
		if i := u.Previewed(); i >= 0 {
			prev = i
		}
	}, DriverOptions{Width: 600, Height: 400})
	d.Frame()
	d.ClickLabel("上传")
	if picks != 1 {
		t.Fatal("card trigger did not pick")
	}
	r := mustFind(t, d, "x.png")
	eye, _ := upCardIcons(r)
	d.Click(eye.Center())
	if prev != 0 {
		t.Fatalf("preview = %d", prev)
	}
	files = append(files, UploadFile{Name: "y.png"})
	d.Frame()
	if _, ok := d.Find("上传"); ok {
		t.Fatal("card trigger shown at MaxCount")
	}
	files = files[:1]
	dis = true
	d.Frame()
	d.ClickLabel("上传")
	_, del := upCardIcons(mustFind(t, d, "x.png"))
	d.Click(del.Center())
	if picks != 1 || len(files) != 1 {
		t.Fatalf("disabled upload reacted: picks=%d files=%d", picks, len(files))
	}
}

func TestEntryUploadDragger(t *testing.T) {
	var files []UploadFile
	picked := false
	d := NewDriver(func(c *Context) {
		if Upload(c, &files).Dragger().Picked() {
			picked = true
		}
	}, DriverOptions{Width: 600, Height: 400})
	d.Frame()
	d.ClickLabel("dragger")
	if !picked {
		t.Fatal("dragger click did not report Picked")
	}
}
