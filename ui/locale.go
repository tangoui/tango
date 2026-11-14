package ui

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tangoui/tango/ui/text"
)

// ---------------------------------------------------------------------------
// Locale

// Locale holds the built-in texts of every component. Components read it
// with c.Locale() while building, so wrapping a page in
// ConfigProvider(c).Locale(ui.EnUS) switches all of them at once.
//
// Component code never writes interface text literally: every label,
// placeholder, tooltip and accessible name comes from here (see
// AUTHORING.md section 15). The built-in packs are ZhCN (default), EnUS,
// ZhTW and Ja (locale_packs.go).
//
// Texts with a number use fmt verbs (TotalFmt "共 %d 条"); call the helper
// methods (Total, PageSizeLabel, YearMonth ...) instead of formatting by
// hand. A format may ignore trailing arguments or pick them with explicit
// indexes ("%[3]s %[1]d"), the helpers tolerate both. Texts of a single
// component are grouped in a nested struct (l.Image.ZoomIn, l.Tour.Next);
// texts of third-party components go to Extra and are read with
// Locale.T(key, fallback).
type Locale struct {
	_ [0]func()

	Code string // BCP 47 tag, e.g. "zh-CN"
	Name string // native display name, e.g. "简体中文"

	// Dir is the layout direction of the language; the zero value DirAuto
	// infers it from Code (ar, he, fa ... and Arab / Hebr script tags are
	// RTL). Not copied by FillLocale.
	Dir Direction

	// Common actions.
	OK, Cancel, Close, Confirm, Back, Clear, Reset, Search, Save, Delete, Edit, Copy, Copied, More string
	Expand, Collapse, SelectAll, Loading, Retry                                                    string
	GotIt                                                                                          string // acknowledge-only button of an alert box ("知道了")

	// Empty & lists.
	NoData, NoMore, LoadMore, LoadFailed string

	// Pagination.
	TotalFmt, RangeTotalFmt, PageSizeFmt, PrevPage, NextPage, Prev5, Next5, JumpTo, PageSuffix string

	// Form controls.
	Placeholder, SelectPlaceholder, Required, NotFound string
	LabelColon                                         string // appended to form / description labels ("：", ":")
	SelectedFmt                                        string // selection count, arg: n ("已选择 %d 项")

	// Date & time.
	Today, Now, SelectDate, SelectTime, SelectDateTime, StartDate, EndDate string
	PrevYear, NextYear, PrevMonth, NextMonth                               string
	RangeSeparator                                                         string       // between the two ends of a range input ("至")
	YearFmt                                                                string       // year label, arg: year ("%d年")
	MonthFmt                                                               string       // month label, args: month number, Months entry ("%d月", "%[2]s")
	YearMonthFmt                                                           string       // panel title, args: year, month number, Months entry ("%d年%d月", "%[3]s %[1]d")
	Weekdays                                                               [7]string    // short names, Sunday first
	Months                                                                 [12]string   // short names, January first
	WeekStart                                                              time.Weekday // first column of month grids (zero value: Sunday)

	// Table.
	FilterConfirm, FilterReset, SortAsc, SortDesc, CancelSort, SelectionAll, SelectionInvert, ExpandRow, CollapseRow string

	// Counts & search.
	ItemUnit, ItemsUnit, Searching string

	// Currency is the symbol or ISO 4217 code of money values (ProField /
	// ProColumn money type) unless the field sets its own ("¥", "$", "NT$",
	// "EUR"). See FormatMoney.
	Currency string

	// Feedback & navigation.
	MessageBoxTitle, Info, Success, Warning, Error, BackTop, CollapseMenu, ExpandMenu, Fullscreen, ExitFullscreen string
	PromptPlaceholder, PromptInvalid, CloseAll, Steps                                                             string

	// Component groups.
	Calendar       CalendarLocale
	Carousel       CarouselLocale
	Image          ImageLocale
	ColorPicker    ColorPickerLocale
	Tour           TourLocale
	QRCode         QRCodeLocale
	Table          TableLocale
	ProTable       ProTableLocale
	Transfer       TransferLocale
	Upload         UploadLocale
	Markdown       MarkdownLocale
	Form           FormLocale
	MarkdownEditor MarkdownEditorLocale
	Command        CommandLocale
	A11y           A11yLocale
	Chart          ChartLocale
	Mobile         MobileLocale

	// Groups of the Pro / entry components added later.
	DatePicker      DatePickerLocale
	TimePicker      TimePickerLocale
	Tabs            TabsLocale
	Pagination      PaginationLocale
	Slider          SliderLocale
	DropZone        DropZoneLocale
	EditableTable   EditableTableLocale
	PageContainer   PageContainerLocale
	ProCard         ProCardLocale
	StatisticCard   StatisticCardLocale
	ProField        ProFieldLocale
	ProDescriptions ProDescriptionsLocale
	LightFilter     LightFilterLocale
	LoginForm       LoginFormLocale
	SettingDrawer   SettingDrawerLocale

	// Extra holds texts of other components by key ("MyWidget.title", ...).
	Extra map[string]string
}

// CalendarLocale holds the Calendar mode switch.
type CalendarLocale struct {
	_           [0]func()
	Month, Year string
}

// CarouselLocale holds the accessible names of the Carousel arrows.
type CarouselLocale struct {
	_          [0]func()
	Prev, Next string
}

// ImageLocale holds the Image placeholder and preview toolbar texts.
type ImageLocale struct {
	_ [0]func()

	Preview, PreviewLabel, ClosePreview, Prev, Next                        string
	ZoomIn, ZoomOut, RotateLeft, RotateRight, FlipHorizontal, FlipVertical string
}

// ColorPickerLocale holds the ColorPicker panel texts.
type ColorPickerLocale struct {
	_ [0]func()

	Presets, None, ClearColor, Clear, Saturation, Hue, Alpha string
	Format                                                   string // accessible name of the HEX / RGB format select
	Value                                                    string // accessible name of the value input of ColorPickerPanel ("颜色值")
}

// CommandLocale holds the Command palette texts.
type CommandLocale struct {
	_                  [0]func()
	Placeholder, Empty string
}

// TourLocale holds the Tour buttons.
type TourLocale struct {
	_                         [0]func()
	Close, Prev, Next, Finish string
}

// QRCodeLocale holds the QRCode status masks.
type QRCodeLocale struct {
	_                         [0]func()
	Expired, Refresh, Scanned string
}

// TableLocale holds the Table filter dropdown texts not covered by the
// common Filter* fields.
type TableLocale struct {
	_ [0]func()

	FilterSearch string // search box placeholder
	FilterAll    string // "all" entry of single-choice filters
	Filter       string // Element's apply button ("筛选")

	// Column settings: group titles and pin actions.
	FixedLeft, NoFixed, FixedRight, PinLeft, PinRight, Unpin string

	// SelectRowFmt names a row's selection checkbox, arg: row number
	// ("选择第 %d 行").
	SelectRowFmt string

	// Selection dropdown next to the header checkbox: its accessible name
	// and the built-in SelectionPage / SelectionNone items.
	Selections, SelectPage, SelectNone string
}

// ChartLocale holds the accessible names and summaries of ui/chart charts:
// the chart kinds (default names) and the summary read as the description.
type ChartLocale struct {
	_ [0]func()

	Line, Area, Column, Bar, Pie, Donut, Scatter, Radar, Gauge, Funnel, Heatmap, DualAxes, Liquid, Sparkline string
	SummaryFmt                                                                                               string // args: kind, series count, point count ("%[1]s，%[2]d 个系列，%[3]d 个数据点")
	XRangeFmt                                                                                                string // appended to the summary, args: first, last x ("；横轴从 %s 到 %s")
	YRangeFmt                                                                                                string // appended to the summary, args: min, max value ("；数值从 %s 到 %s")
	ValueFmt                                                                                                 string // single-value charts, args: kind, value ("%s：%s")
	Total                                                                                                    string // pie / donut centre title ("总计")
	Conversion                                                                                               string // funnel conversion-rate label ("转化率")
}

// MobileLocale holds the texts of ui/mobile components.
type MobileLocale struct {
	_ [0]func()

	PullToRefresh, ReleaseToRefresh, Refreshed string // pull-to-refresh states (the loading state uses Loading)
	TapToRetry                                 string // appended to LoadFailed in infinite lists ("，点击重试")
	DayFmt, HourFmt, MinuteFmt                 string // date-time picker columns, arg: number ("%d日", "%02d时", "%02d分")
}

// ProTableLocale holds the ProTable search form and toolbar texts.
type ProTableLocale struct {
	_ [0]func()

	Query, Reload, Density, DensityDefault, DensityMiddle, DensityCompact string
	ColumnSetting, ColumnDisplay, ClearSelection                          string
	LoadErrorFmt                                                          string // alert text, arg: error message
}

// FormLocale holds the Form / ProForm texts.
type FormLocale struct {
	_ [0]func()

	Validating                          string // help text while an async rule runs
	RequiredInputFmt, RequiredSelectFmt string // default required message, arg: field label
	Submit, Query, PrevStep, NextStep   string
	MoveUp, MoveDown, AddRow            string // FormList actions
}

// TransferLocale holds the Transfer defaults.
type TransferLocale struct {
	_                                 [0]func()
	Source, Target, SearchPlaceholder string
}

// UploadLocale holds the Upload texts. The dragger has no default hint
// line; set one with DragText.
type UploadLocale struct {
	_ [0]func()

	Trigger    string // picture-card add tile ("上传")
	Uploading  string // progress text of a file ("文件上传中")
	Remove     string // remove action of a file
	Preview    string // preview action of a file
	Button     string // select button / Element dragger link ("点击上传")
	DragTitle  string // Ant dragger title
	DropPrefix string // Element dragger text before the link ("将文件拖到此处，或")

	// Announcements and rejection messages of dropped / picked files.
	DoneFmt     string // upload finished, arg: file name ("%s 上传成功")
	FailedFmt   string // upload failed, arg: file name ("%s 上传失败")
	TooManyFmt  string // list full, arg: file name
	BadTypeFmt  string // type not accepted, arg: file name
	TooLargeFmt string // over the size limit, args: file name, limit ("%s 超过 %s")
	IsDirFmt    string // a folder was dropped, arg: folder name
}

// DatePickerLocale holds the texts of the week / month / quarter / year
// pickers and DatePickerPanel.
type DatePickerLocale struct {
	_ [0]func()

	SelectWeek, SelectMonth, SelectQuarter, SelectYear string // placeholders by granularity
	// RangePicker placeholders by granularity (date ranges use the
	// top-level StartDate / EndDate).
	StartWeek, EndWeek       string
	StartMonth, EndMonth     string
	StartQuarter, EndQuarter string
	StartYear, EndYear       string
	PrevDecade, NextDecade   string // year panel arrows
	WeekFmt                  string // week value, args: week-based year, week number, English ordinal suffix ("%[1]d-第%[2]d周")
	QuarterFmt               string // quarter value, args: year, quarter ("%[1]d-Q%[2]d")
	QuarterCellFmt           string // quarter cell of the panel, arg: quarter ("Q%d")
}

// TimePickerLocale holds the TimePicker / TimeRangePicker texts.
type TimePickerLocale struct {
	_ [0]func()

	StartTime, EndTime string // range placeholders
	AM, PM             string // 12-hour clock words
	Meridiem           string // accessible name of the AM / PM column
}

// TabsLocale holds the accessible names of the Tabs overflow and add buttons.
type TabsLocale struct {
	_         [0]func()
	More, Add string
}

// PaginationLocale holds the accessible names of the Pagination inputs
// (quick jumper, simple mode current page).
type PaginationLocale struct {
	_             [0]func()
	Jump, Current string
}

// SliderLocale holds the accessible names of the two SliderRange handles.
type SliderLocale struct {
	_        [0]func()
	Min, Max string
}

// DropZoneLocale holds the DropZone accessible name and drag-over
// announcement.
type DropZoneLocale struct {
	_             [0]func()
	Name, Release string
}

// EditableTableLocale holds the EditableProTable texts.
type EditableTableLocale struct {
	_ [0]func()

	Actions       string // title of the action column
	DeleteConfirm string // Popconfirm of the delete action
}

// PageContainerLocale holds the PageContainer / FooterToolbar texts.
type PageContainerLocale struct {
	_ [0]func()

	FooterToolbar string // accessible name of the FooterToolbar ("页面操作")
}

// ProCardLocale holds the ProCard / StatisticCard texts.
type ProCardLocale struct {
	_ [0]func()

	MoreInfo string // accessible name of the title tooltip icon
}

// StatisticCardLocale holds the accessible names of the trend marks.
type StatisticCardLocale struct {
	_        [0]func()
	Up, Down string
}

// ProFieldLocale holds texts of ProField / ProColumn value types (editors
// and read-only views).
type ProFieldLocale struct {
	_ [0]func()

	On, Off          string // switch value text
	StartFmt, EndFmt string // time range ends, arg: field label ("%s开始")
	Min, Max         string // digit range placeholders
	RangeEndFmt      string // accessible name of a digit range end, args: field label, Min / Max ("%s：%s")
}

// ProDescriptionsLocale holds the ProDescriptions texts.
type ProDescriptionsLocale struct {
	_ [0]func()

	EditFmt string // inline edit button, arg: item label ("编辑%s")
}

// LightFilterLocale holds the LightFilter texts.
type LightFilterLocale struct {
	_ [0]func()

	Name     string // accessible name of the toolbar
	ClearFmt string // clear button of a field, arg: field label ("清除%s")
	PillFmt  string // accessible name of a field pill, args: label, value ("%s：%s")
	More     string // collapsed fields button
	MoreFmt  string // collapsed fields button with values set, arg: count ("更多筛选（%d）")
}

// LoginFormLocale holds the LoginForm / LoginFormPage texts.
type LoginFormLocale struct {
	_ [0]func()

	AccountTab, MobileTab      string
	Username, UsernameRequired string
	Password, PasswordRequired string
	Mobile, MobileRequired     string
	MobileInvalid              string
	Captcha, CaptchaRequired   string
	GetCode                    string
	ResendFmt                  string // code button while counting down, arg: seconds ("%d 秒后重新获取")
	CodeSentFmt                string // announcement after sending, arg: seconds
	CodeReady                  string // announcement when the countdown ends
	Remember, Forgot           string
	Submit                     string
	Others                     string // title of the other-methods row
}

// SettingDrawerLocale holds the ProLayout SettingDrawer texts.
type SettingDrawerLocale struct {
	_ [0]func()

	Open, Title                         string // trigger (accessible name, tooltip) and drawer title
	NavTheme, Light, Dark, RealDark     string // menu style section and its choices
	PrimaryColor                        string
	Layout, Side, Top, Mix              string // navigation mode section and its choices
	ContentWidth, Fluid, Fixed          string
	FixedHeader, FixSiderbar, ColorWeak string
	Other, Compact                      string
	ColorFmt                            string // swatch name, arg: hex ("主题色 %s")
}

// MarkdownLocale holds the Markdown viewer texts.
type MarkdownLocale struct {
	_ [0]func()

	ImagePrefix string // shown before the alt text of an image ("[图片] ")
	CopyCode    string
	Copied      string
}

// MarkdownEditorLocale holds the MarkdownEditor toolbar, samples inserted
// by the toolbar and the view mode switch.
type MarkdownEditorLocale struct {
	_ [0]func()

	Placeholder                                                        string
	Heading, Bold, Italic, Strike, Code, Link, Quote                   string
	BulletList, OrderedList, Task, CodeBlock, Table, Divider           string
	BoldSample, ItalicSample, StrikeSample, LinkSample, TableColumnFmt string // TableColumnFmt arg: column number
	Edit, Split, Preview                                               string
}

// A11yLocale holds accessible names and descriptions that are read by
// screen readers but not shown (icon-only controls, regions, summaries).
type A11yLocale struct {
	_ [0]func()

	Navigation, Breadcrumb, Pagination, Notifications, Toolbar, Menu string
	PageFmt                                                          string // pagination item, arg: page number ("第 %d 页")
	MoreActions, OpenMenu, Remove, DragHandle, Resize, Splitter      string
	ShowPassword, HidePassword, Increase, Decrease                   string
	Avatar, Image, Chart, Progress, Rating, Color, Loading           string
	SlideFmt                                                         string // carousel slide, args: n, total ("第 %d 张，共 %d 张")
	RateFmt                                                          string // rating value, args: value, count ("%[1]g 星，共 %[2]d 星")
	DateFmt                                                          string // full date of a calendar cell, args: year, month number, day, Months entry ("%d年%d月%d日")
	DigitFmt                                                         string // one-time-code cell, args: n, total ("第 %d 位，共 %d 位")
	RemoveFmt                                                        string // remove button of a tag / file, arg: its label ("移除 %s")
	Hour, Minute, Second                                             string // time picker columns
	MoveToTarget, MoveToSource                                       string // Transfer move buttons
	PageSize                                                         string // page-size select of Pagination ("每页条数")
	Day                                                              string // day column of a date picker wheel (mobile)
}

// T returns Extra[key] or fallback.
func (l *Locale) T(key, fallback string) string {
	if l != nil && l.Extra != nil {
		if s, ok := l.Extra[key]; ok {
			return s
		}
	}
	return fallback
}

// locFmt formats like fmt.Sprintf but drops the "%!(EXTRA ...)" note, so a
// locale format may use only some of the arguments a helper passes.
func locFmt(format string, args ...any) string {
	s := fmt.Sprintf(format, args...)
	if i := strings.Index(s, "%!(EXTRA "); i >= 0 {
		s = s[:i]
	}
	return s
}

// Total formats the pagination total ("共 85 条").
func (l *Locale) Total(n int) string { return locFmt(l.TotalFmt, n) }

// RangeTotal formats "第 1-10 条/共 85 条".
func (l *Locale) RangeTotal(from, to, total int) string {
	return locFmt(l.RangeTotalFmt, from, to, total)
}

// PageSizeLabel formats the page size option ("10 条/页").
func (l *Locale) PageSizeLabel(n int) string { return locFmt(l.PageSizeFmt, n) }

// Items formats a count with the item unit ("3 项" / "3 items").
func (l *Locale) Items(n int) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, l.ItemUnit)
	}
	return fmt.Sprintf("%d %s", n, l.ItemsUnit)
}

// Page formats the accessible name of a pagination item ("第 3 页").
func (l *Locale) Page(n int) string { return locFmt(l.A11y.PageFmt, n) }

// Slide formats a carousel slide position ("第 2 张，共 5 张").
func (l *Locale) Slide(n, total int) string { return locFmt(l.A11y.SlideFmt, n, total) }

// Rating formats a rate value ("3.5 星，共 5 星").
func (l *Locale) Rating(v float64, count int) string { return locFmt(l.A11y.RateFmt, v, count) }

// Date formats the full date of a calendar cell ("2026年11月12日" / "Nov 12, 2026").
func (l *Locale) Date(y, m, d int) string { return locFmt(l.A11y.DateFmt, y, m, d, l.monthName(m)) }

// Digit names cell n of a total-cell one-time code ("第 1 位，共 6 位").
func (l *Locale) Digit(n, total int) string { return locFmt(l.A11y.DigitFmt, n, total) }

// RemoveItem names the remove button of a tag or file ("移除 标签").
func (l *Locale) RemoveItem(label string) string { return locFmt(l.A11y.RemoveFmt, label) }

// Selected formats the selection count ("已选择 3 项").
func (l *Locale) Selected(n int) string { return locFmt(l.SelectedFmt, n) }

// WithColon appends LabelColon to a non-empty label.
func (l *Locale) WithColon(label string) string {
	if label == "" {
		return ""
	}
	return label + l.LabelColon
}

// monthName returns the short name of month m (1-12).
func (l *Locale) monthName(m int) string {
	if m < 1 || m > 12 {
		return ""
	}
	return l.Months[m-1]
}

// Year formats a year label ("2026年" / "2026").
func (l *Locale) Year(y int) string { return locFmt(l.YearFmt, y) }

// Month formats a month label, m in 1-12 ("11月" / "Nov").
func (l *Locale) Month(m int) string { return locFmt(l.MonthFmt, m, l.monthName(m)) }

// YearMonth formats a month title ("2026年11月" / "Nov 2026").
func (l *Locale) YearMonth(y, m int) string {
	return locFmt(l.YearMonthFmt, y, m, l.monthName(m))
}

// WeekdayNames returns the short weekday names starting at WeekStart, in
// the column order of month grids.
func (l *Locale) WeekdayNames() []string {
	out := make([]string, 7)
	for i := range out {
		out[i] = l.Weekdays[(int(l.WeekStart)+i)%7]
	}
	return out
}

// ---------------------------------------------------------------------------
// Registry

var (
	localeMu      sync.RWMutex
	locales       = map[string]*Locale{}
	defaultLocale = ZhCN
)

func init() {
	// The built-in packs are complete (TestLocalePacksComplete), so they
	// are registered without FillLocale: a missing text shows up in tests
	// instead of silently falling back to English.
	for _, l := range []*Locale{EnUS, ZhCN, ZhTW, Ja} {
		locales[normLocale(l.Code)] = l
	}
}

func normLocale(code string) string {
	code = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(code), "_", "-"))
	// POSIX suffixes: "zh_TW.UTF-8", "de_DE@euro".
	if i := strings.IndexAny(code, ".@"); i >= 0 {
		code = code[:i]
	}
	return code
}

// RegisterLocale adds (or replaces) a locale under l.Code. Empty texts are
// filled from EnUS (see FillLocale), so a partial translation works:
//
//	ko := &ui.Locale{Code: "ko-KR", Name: "한국어", OK: "확인", Cancel: "취소"}
//	ui.RegisterLocale(ko)
func RegisterLocale(l *Locale) {
	if l == nil || l.Code == "" {
		return
	}
	if l != EnUS {
		FillLocale(l, EnUS)
	}
	localeMu.Lock()
	locales[normLocale(l.Code)] = l
	localeMu.Unlock()
}

// FillLocale copies every empty text of l (string fields, entries of the
// fixed arrays, fields of the component groups and missing Extra keys)
// from base. RegisterLocale calls it with EnUS so a partial translation
// never shows blank labels, and new fields added later fall back
// automatically. WeekStart is not copied (its zero value, Sunday, is valid).
func FillLocale(l, base *Locale) {
	if l == nil || base == nil {
		return
	}
	fillValue(reflect.ValueOf(l).Elem(), reflect.ValueOf(base).Elem())
	for k, v := range base.Extra {
		if l.Extra == nil {
			l.Extra = map[string]string{}
		}
		if _, ok := l.Extra[k]; !ok {
			l.Extra[k] = v
		}
	}
}

func fillValue(dst, src reflect.Value) {
	switch dst.Kind() {
	case reflect.String:
		if dst.String() == "" {
			dst.SetString(src.String())
		}
	case reflect.Array:
		for j := 0; j < dst.Len(); j++ {
			fillValue(dst.Index(j), src.Index(j))
		}
	case reflect.Struct:
		for i := 0; i < dst.NumField(); i++ {
			fillValue(dst.Field(i), src.Field(i))
		}
	}
}

// zhTraditional reports whether a normalized Chinese tag asks for
// Traditional script: zh-Hant(-*), zh-TW, zh-HK, zh-MO.
func zhTraditional(k string) bool {
	parts := strings.Split(k, "-")
	for _, p := range parts[1:] {
		switch p {
		case "hant", "tw", "hk", "mo":
			return true
		case "hans":
			return false
		}
	}
	return false
}

// LocaleByCode looks up a registered locale ("zh-CN", "en_us", "en",
// "zh_TW.UTF-8"). Chinese is matched by script: zh-Hant, zh-TW, zh-HK and
// zh-MO find zh-TW (ZhTW), other zh tags find zh-CN; the two never stand
// in for each other. Otherwise a tag matches the same language without
// regard to region ("ja" → ja-JP, "en-GB" → en-US); the language-only
// entry wins, then the first registered region in code order.
func LocaleByCode(code string) (*Locale, bool) {
	localeMu.RLock()
	defer localeMu.RUnlock()
	k := normLocale(code)
	if l, ok := locales[k]; ok {
		return l, true
	}
	lang, _, _ := strings.Cut(k, "-")
	if lang == "zh" {
		want := "zh-cn"
		if zhTraditional(k) {
			want = "zh-tw"
		}
		l, ok := locales[want]
		return l, ok
	}
	if l, ok := locales[lang]; ok {
		return l, true
	}
	keys := make([]string, 0, len(locales))
	for key := range locales {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if l, _, _ := strings.Cut(key, "-"); l == lang {
			return locales[key], true
		}
	}
	return nil, false
}

// Locales lists the registered locales sorted by code.
func Locales() []*Locale {
	localeMu.RLock()
	defer localeMu.RUnlock()
	out := make([]*Locale, 0, len(locales))
	for _, l := range locales {
		out = append(out, l)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out
}

// defaultLocaleNow returns the locale used outside any ConfigProvider.
func defaultLocaleNow() *Locale {
	localeMu.RLock()
	defer localeMu.RUnlock()
	return defaultLocale
}

// SetDefaultLocale sets the locale used outside any ConfigProvider.
func SetDefaultLocale(l *Locale) {
	if l != nil {
		localeMu.Lock()
		defaultLocale = l
		localeMu.Unlock()
		text.SetDefaultLang(l.Code) // shaping language of text without Style.Lang
	}
}
