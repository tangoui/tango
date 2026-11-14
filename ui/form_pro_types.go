package ui

import (
	"bytes"
	"encoding/json"
	"image"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/layout"
)

// Value types of the third Pro round: date granularities, a date-time
// range, media (image, avatar), code, relative time, durations, row
// numbers, a verification code and nested lists. Like the others they
// describe both a field's editor and the read-only view in ProForm
// (readonly), ProField, ProTable and ProDescriptions.
const (
	ValueDateWeek      = "dateWeek"      // time.Time, shown as its week ("2026-46周")
	ValueDateMonth     = "dateMonth"     // time.Time, shown as "2006-01"
	ValueDateQuarter   = "dateQuarter"   // time.Time, shown as its quarter ("2026-Q4")
	ValueDateYear      = "dateYear"      // time.Time, shown as "2006"
	ValueDateTimeRange = "dateTimeRange" // [2]time.Time with the times of day
	// ValueImage is an image.Image, []image.Image or UploadFile(s) with a
	// Thumb; the editor is a one-picture Upload card (ProField.Pick adds).
	ValueImage = "image"
	// ValueAvatar is an image.Image (or UploadFile) or a name string (its
	// first letter, edited as text); a picture is edited like image.
	ValueAvatar   = "avatar"
	ValueCode     = "code"     // string in a monospace block
	ValueJSONCode = "jsonCode" // JSON text (string / []byte) or any value, shown indented; validated when edited
	ValueFromNow  = "fromNow"  // time.Time shown relative to now ("3 分钟前"); edited with a date-time picker
	ValueSecond   = "second"   // number of seconds shown as a duration ("1 小时 5 秒")
	ValueIndex    = "index"    // ProTable: the 1-based row number (the value is ignored)
	// ValueCaptcha is a string code with a "get code" button and its
	// countdown (ProField.OnCaptcha sends the code).
	ValueCaptcha = "captcha"
	// ValueFormList is a list of rows ([]ProValues) edited with Form.List;
	// ProField.List gives the fields of a row.
	ValueFormList = "formList"
)

// ProFieldList configures a ValueFormList field: the fields of one row,
// the row count limits and the add button's text.
type ProFieldList struct {
	_        [0]func()
	Fields   []ProField
	Min, Max int    // Max 0: no limit
	AddText  string // default: the locale's "add row"
}

// proPicker maps the date granularity value types to picker types.
func proPicker(vt string) (PickerType, bool) {
	switch vt {
	case ValueDateWeek:
		return PickerWeek, true
	case ValueDateMonth:
		return PickerMonth, true
	case ValueDateQuarter:
		return PickerQuarter, true
	case ValueDateYear:
		return PickerYear, true
	}
	return PickerDate, false
}

// proTypesControl builds the editor of the third-round value types.
func proTypesControl(c *Context, fd *ProField, vals ProValues, sel, inp string, grow []layout.Option) {
	k := fd.Key
	switch fd.ValueType {
	case ValueDateWeek, ValueDateMonth, ValueDateQuarter, ValueDateYear:
		p, _ := proPicker(fd.ValueType)
		t := vals.Time(k)
		dp := DatePicker(c, &t).Picker(p).Layout(grow...)
		if fd.Placeholder != "" {
			dp.Placeholder(fd.Placeholder) // otherwise the picker's own ("请选择周")
		}
		if fd.Label != "" {
			dp.A11yLabel(fd.Label)
		}
		if dp.Changed() {
			proSetOrDelete(vals, k, t, t.IsZero())
		}
	case ValueDateTimeRange:
		r := vals.Range(k)
		w := RangePicker(c, &r).ShowTime().Layout(grow...)
		if fd.Label != "" {
			w.A11yLabel(fd.Label)
		}
		if w.Changed() {
			proSetOrDelete(vals, k, r, r[0].IsZero() && r[1].IsZero())
		}
	case ValueFromNow:
		t := vals.Time(k)
		w := DateTimePicker(c, &t).Placeholder(sel).Layout(grow...)
		if fd.Label != "" {
			w.A11yLabel(fd.Label)
		}
		if w.Changed() {
			proSetOrDelete(vals, k, t, t.IsZero())
		}
	case ValueCode, ValueJSONCode:
		s := vals.String(k)
		if fd.ValueType == ValueJSONCode {
			if txt, ok := proJSONText(vals[k]); ok {
				s = txt
			}
		}
		w := TextArea(c, &s).Placeholder(inp).AutoSize(4, 12).Layout(grow...)
		if fd.Label != "" {
			w.A11yLabel(fd.Label)
		}
		if w.Changed() {
			proSetOrDelete(vals, k, s, s == "")
		}
	case ValueImage, ValueAvatar:
		if s, ok := vals[k].(string); ok && fd.ValueType == ValueAvatar {
			// A name avatar is edited as text (its first letter is shown).
			w := Input(c, &s).Placeholder(inp).AllowClear().Layout(grow...)
			if fd.Label != "" {
				w.A11yLabel(fd.Label)
			}
			if w.Changed() {
				proSetOrDelete(vals, k, s, s == "")
			}
			return
		}
		proImageControl(c, fd, vals, grow)
	case ValueCaptcha:
		proCaptchaControl(c, fd, vals, inp, grow)
	}
}

// proSetOrDelete stores v under k, or removes k when empty.
func proSetOrDelete(vals ProValues, k string, v any, empty bool) {
	if empty {
		delete(vals, k)
		return
	}
	vals[k] = v
}

// proImageControl edits an image / avatar value with a one-picture
// Upload card; a picked file's Thumb becomes the value.
func proImageControl(c *Context, fd *ProField, vals ProValues, grow []layout.Option) {
	k := fd.Key
	var files []UploadFile
	switch x := vals[k].(type) {
	case UploadFile:
		files = []UploadFile{x}
	case []UploadFile:
		files = append(files, x...)
	default:
		for _, img := range proImages(x) {
			files = append(files, UploadFile{Name: fd.Label, Status: "done", Thumb: img})
		}
	}
	before := len(files)
	Column(c, func() {
		changed := false
		up := Upload(c, &files).PictureCard().MaxCount(1)
		st := State[p2eUploadState](c, hashID(up.n.id, "pro-image"), nil)
		for {
			got, ok := st.res.poll()
			if !ok {
				break
			}
			files = append(files[:0], got...)
			changed = true
			c.invalid = true
		}
		if fd.Pick != nil {
			pick, ch, inv := fd.Pick, st.res.chInit(), c.Invalidator()
			up.OnPick(func() { pick(func(fs []UploadFile) { ch <- fs; inv() }) })
		}
		if up.Changed() || changed || len(files) != before {
			switch {
			case len(files) == 0:
				delete(vals, k)
			case files[0].Thumb != nil:
				vals[k] = files[0].Thumb
			default:
				vals[k] = files[0]
			}
		}
	}, grow...)
}

// proFirstImage returns the first image of a value (nil when none).
func proFirstImage(v any) image.Image {
	if imgs := proImages(v); len(imgs) > 0 {
		return imgs[0]
	}
	return nil
}

// proImages returns the pictures of an image / avatar value.
func proImages(v any) []image.Image {
	switch x := v.(type) {
	case image.Image:
		if x != nil {
			return []image.Image{x}
		}
	case []image.Image:
		var out []image.Image
		for _, img := range x {
			if img != nil {
				out = append(out, img)
			}
		}
		return out
	case UploadFile:
		if x.Thumb != nil {
			return []image.Image{x.Thumb}
		}
	case []UploadFile:
		var out []image.Image
		for _, f := range x {
			if f.Thumb != nil {
				out = append(out, f.Thumb)
			}
		}
		return out
	}
	return nil
}

// proCaptchaState is the countdown of a captcha field.
type proCaptchaState struct {
	until   time.Time
	sending bool
	res     pro3Async[error]
}

// proCaptchaControl is the code input with the "get code" button. The
// button counts down ProField.CaptchaSeconds (default 60) after
// OnCaptcha reports success.
func proCaptchaControl(c *Context, fd *ProField, vals ProValues, ph string, grow []layout.Option) {
	loc := c.Locale()
	k := fd.Key
	s := vals.String(k)
	secs := fd.CaptchaSeconds
	if secs <= 0 {
		secs = 60
	}
	Row(c, func() {
		in := Input(c, &s).Placeholder(ph).AllowClear().Layout(layout.Grow(1), layout.MinW(0))
		if fd.Label != "" {
			in.A11yLabel(fd.Label)
		}
		if in.Changed() {
			proSetOrDelete(vals, k, s, s == "")
		}
		st := State[proCaptchaState](c, hashID(in.ID(), "pro-captcha"), nil)
		for {
			err, ok := st.res.poll()
			if !ok {
				break
			}
			st.sending = false
			if err == nil {
				st.until = c.Now().Add(time.Duration(secs) * time.Second)
				c.Announce(locFmt(loc.LoginForm.CodeSentFmt, secs), a11y.LivePolite)
			}
			c.invalid = true
		}
		n := pro3Remain(st.until, c.Now())
		label := loc.LoginForm.GetCode
		if n > 0 {
			label = locFmt(loc.LoginForm.ResendFmt, n)
			left := st.until.Sub(c.Now())
			c.WakeAt(c.Now().Add(left - time.Duration(n-1)*time.Second))
		}
		b := Button(c, label).Disabled(n > 0).Loading(st.sending).Layout(layout.MinW(120))
		if b.Clicked() && !st.sending {
			if fd.OnCaptcha == nil {
				st.until = c.Now().Add(time.Duration(secs) * time.Second)
				c.invalid = true
				return
			}
			st.sending = true
			ch, inv := st.res.chInit(), c.Invalidator()
			fd.OnCaptcha(vals.Clone(), func(err error) { ch <- err; inv() })
			c.invalid = true
		}
	}, append([]layout.Option{layout.Gap(8), layout.Middle()}, grow...)...)
}

// proJSONText indents JSON: text (string / []byte) is re-indented, other
// values are marshalled. ok is false for text that is not valid JSON.
func proJSONText(v any) (string, bool) {
	var raw []byte
	switch x := v.(type) {
	case nil:
		return "", false
	case string:
		if strings.TrimSpace(x) == "" {
			return "", false
		}
		raw = []byte(x)
	case []byte:
		raw = x
	default:
		b, err := json.MarshalIndent(x, "", "  ")
		if err != nil {
			return "", false
		}
		return string(b), true
	}
	var out bytes.Buffer
	if err := json.Indent(&out, raw, "", "  "); err != nil {
		return string(raw), false
	}
	return out.String(), true
}

// proFromNow formats t relative to now: JustNow under a minute, then
// minutes, hours, days, months (30 days) and years (365 days).
func proFromNow(loc *Locale, t, now time.Time) string {
	if t.IsZero() {
		return "-"
	}
	d := now.Sub(t)
	later := d < 0
	if later {
		d = -d
	}
	pf := loc.ProField
	var amt string
	switch day := 24 * time.Hour; {
	case d < time.Minute:
		return pf.JustNow
	case d < time.Hour:
		amt = locFmt(pf.MinuteFmt, int(d/time.Minute))
	case d < day:
		amt = locFmt(pf.HourFmt, int(d/time.Hour))
	case d < 30*day:
		amt = locFmt(pf.DayFmt, int(d/day))
	case d < 365*day:
		amt = locFmt(pf.MonthFmt, int(d/(30*day)))
	default:
		amt = locFmt(pf.YearFmt, int(d/(365*day)))
	}
	if later {
		return locFmt(pf.LaterFmt, amt)
	}
	return locFmt(pf.AgoFmt, amt)
}

// proSeconds formats a number of seconds as days, hours, minutes and
// seconds, leaving out zero parts ("1 小时 5 秒"; "0 秒" for zero).
func proSeconds(loc *Locale, f float64) string {
	pf := loc.ProField
	neg := f < 0
	s := int64(math.Round(math.Abs(f)))
	d, h, m := s/86400, s/3600%24, s/60%60
	s %= 60
	var parts []string
	if d > 0 {
		parts = append(parts, locFmt(pf.DayFmt, d))
	}
	if h > 0 {
		parts = append(parts, locFmt(pf.HourFmt, h))
	}
	if m > 0 {
		parts = append(parts, locFmt(pf.MinuteFmt, m))
	}
	if s > 0 || len(parts) == 0 {
		parts = append(parts, locFmt(pf.SecondFmt, s))
	}
	out := strings.Join(parts, " ")
	if neg {
		out = "-" + out
	}
	return out
}

// proTypesText formats the third-round value types as text (ok false:
// not one of them). v is not nil.
func proTypesText(c *Context, valueType string, v any) (string, bool) {
	loc := c.Locale()
	switch valueType {
	case ValueDateWeek, ValueDateMonth, ValueDateQuarter, ValueDateYear:
		p, _ := proPicker(valueType)
		t, _ := v.(time.Time)
		if t.IsZero() {
			return "-", true
		}
		return p2aFormatPeriod(loc, t, p, p2aPickerLayout(p), loc.WeekStart), true
	case ValueDateTimeRange:
		r, _ := v.([2]time.Time)
		if r[0].IsZero() && r[1].IsZero() {
			return "-", true
		}
		f := func(t time.Time) string {
			if t.IsZero() {
				return "-"
			}
			return t.Format("2006-01-02 15:04:05")
		}
		return f(r[0]) + " " + loc.RangeSeparator + " " + f(r[1]), true
	case ValueFromNow:
		t, _ := v.(time.Time)
		return proFromNow(loc, t, c.Now()), true
	case ValueSecond:
		if f, ok := proNumber(v); ok {
			return proSeconds(loc, f), true
		}
		return "-", true
	case ValueIndex:
		if f, ok := proNumber(v); ok {
			return strconv.FormatFloat(f, 'f', -1, 64), true
		}
		return "-", true
	case ValueCode, ValueCaptcha:
		s := proValueText(v)
		if s == "" {
			return "-", true
		}
		return s, true
	case ValueJSONCode:
		if s, ok := proJSONText(v); ok || s != "" {
			return s, true
		}
		return "-", true
	case ValueImage:
		if n := len(proImages(v)); n > 0 {
			return loc.A11y.Image, true
		}
		return "-", true
	case ValueAvatar:
		if s, ok := v.(string); ok && s != "" {
			return s, true
		}
		if proFirstImage(v) != nil {
			return loc.A11y.Avatar, true
		}
		return "-", true
	case ValueFormList:
		rows, _ := v.([]ProValues)
		if len(rows) == 0 {
			return "-", true
		}
		return loc.Items(len(rows)), true
	}
	return "", false
}

// proTypesView renders the read-only views that are not plain text:
// pictures, avatars and code blocks. size is the picture size (tables
// use a smaller one); alt names a picture ("" for the locale's). It
// reports false for other value types and empty values.
func proTypesView(c *Context, valueType string, v any, size float32, alt string) bool {
	loc := c.Locale()
	switch valueType {
	case ValueImage:
		imgs := proImages(v)
		if len(imgs) == 0 {
			return false
		}
		if alt == "" {
			alt = loc.A11y.Image
		}
		Row(c, func() {
			for i, img := range imgs {
				Key(c, i, func() {
					w := Image(c, img).Size(size, size).Fit("cover").Preview(true).A11yLabel(alt)
					if len(imgs) > 1 {
						w.PreviewGroup(imgs, i)
					}
				})
			}
		}, layout.Gap(8), layout.Middle(), layout.WrapItems())
		return true
	case ValueAvatar:
		if s, ok := v.(string); ok {
			if s == "" {
				return false
			}
			Avatar(c, string([]rune(s)[:1])).Size(size).A11yLabel(s)
			return true
		}
		img := proFirstImage(v)
		if img == nil {
			return false
		}
		if alt == "" {
			alt = loc.A11y.Avatar
		}
		Avatar(c, "").Image(img).Size(size).A11yLabel(alt)
		return true
	case ValueCode, ValueJSONCode:
		s := proValueText(v)
		if valueType == ValueJSONCode {
			s, _ = proJSONText(v)
		}
		if strings.TrimSpace(s) == "" {
			return false
		}
		proCodeBlock(c, s, size < 40)
		return true
	}
	return false
}

// proCodeBlock shows code in a monospace block on a muted fill; compact
// (table cells) keeps it on one line.
func proCodeBlock(c *Context, s string, compact bool) {
	th := c.Theme
	if compact {
		line, _, _ := strings.Cut(s, "\n")
		Text(c, line).Code().NoWrap().Ellipsis().Layout(layout.MinW(0))
		return
	}
	Column(c, func() {
		t := Text(c, strings.TrimRight(s, "\n")).Size(th.FontSizeSM)
		t.st.Mono = true
	}, layout.PadVH(8, 12), layout.Grow(1), layout.MinW(0)).Bg(th.FillTertiary).Radius(th.RadiusSM).
		Semantics(a11y.RoleGroup, "")
}

// proFieldView renders the read-only value of a field: lists show their
// rows, the other types go through pro3ValueView.
func proFieldView(c *Context, fd *ProField, v any, size float32) {
	if fd.ValueType == ValueFormList && fd.List != nil {
		rows, _ := v.([]ProValues)
		if len(rows) == 0 {
			Text(c, "-")
			return
		}
		loc := c.Locale()
		Column(c, func() {
			for i, row := range rows {
				Key(c, i, func() {
					Row(c, func() {
						for j := range fd.List.Fields {
							sub := &fd.List.Fields[j]
							Row(c, func() {
								if sub.Label != "" {
									Text(c, loc.WithColon(sub.Label)).Color(c.Theme.TextSecondary).NoWrap()
								}
								proFieldView(c, sub, row[sub.Key], size)
							}, layout.Gap(4), layout.Middle())
						}
					}, layout.Gap(16), layout.Middle(), layout.WrapItems())
				})
			}
		}, layout.Gap(8))
		return
	}
	if proTypesView(c, fd.ValueType, v, size, fd.Label) {
		return
	}
	pro3ValueView(c, fd.ValueType, fd.ValueEnum, v)
}
