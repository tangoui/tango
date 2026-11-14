package main

import (
	"errors"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/icon"
)

func init() {
	vals := ui.ProValues{"phone": "13800000000", "mode": "sms"}
	failed := false
	gallery.Register(gallery.Page{
		Name: "ProFormAlign", Title: "ProForm v3 Captcha / Segmented", Parent: "ProForm",
		Group: gallery.Form, Order: 39, Width: 800, Height: 840,
		Render: func(c *ui.Context) {
			tbDemo(c, "Captcha and Segmented", "One ProField entry per value type. The disabled option cannot be selected; captcha failures do not start a countdown.", func() {
				fields := []ui.ProField{
					{Key: "mode", Label: "Delivery", ValueType: ui.ValueSegmented,
						ValueEnum: []ui.ProEnum{{Value: "sms", Label: "SMS"}, {Value: "email", Label: "Email", Disabled: true}, {Value: "voice", Label: "Voice"}},
						Segmented: func(w *ui.SegmentedW) {
							w.ItemRender(func(o ui.Option, selected bool) {
								ui.Icon(c, icon.Mail).Size(14)
								ui.Text(c, o.Label)
							})
						}},
					{Key: "phone", Label: "Phone", Required: true},
					{Key: "code", Label: "Code", ValueType: ui.ValueCaptcha, CaptchaSeconds: 5,
						Captcha: &ui.ProFieldCaptcha{PhoneKey: "phone"},
						OnCaptcha: func(v ui.ProValues, done func(error)) {
							if failed {
								done(errors.New("delivery unavailable"))
								return
							}
							done(nil)
						}},
				}
				ui.Checkbox(c, "Simulate delivery failure", &failed)
				ui.ProForm(c, fields).Values(&vals).Show()
			})
			tbDemo(c, "Readonly", "The selected enum label is displayed, not the editor.", func() {
				ui.ProForm(c, []ui.ProField{{Key: "mode", Label: "Delivery", ValueType: ui.ValueSegmented,
					ValueEnum: []ui.ProEnum{{Value: "sms", Label: "SMS"}, {Value: "voice", Label: "Voice"}}}}).Values(&vals).Readonly(true).Show()
			})
		},
	})
}
