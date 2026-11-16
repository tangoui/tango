package main

// The component scenes' numbers, texts and formulas (../scenes.md,
// "Component scenes"). This file is the same, byte for byte, in the
// tango, mygo and nsgo programs, so that every framework's scene shows
// the same text and changes the same items each frame.

import (
	"math"
	"strconv"
	"strings"
	"time"
)

const (
	pagePad = 16 // padding around a page, logical px
	pageGap = 8  // gap between controls, logical px

	buttonCount = 60
	buttonCols  = 6

	inputCount = 20
	inputMax   = 40 // characters; one more resets to inputSeed
	inputSeed  = "abcdefghij0123456789"

	textareaLines = 40   // of textareaLine characters, newline included
	textareaLine  = 50   //
	textareaMax   = 4000 // characters; reaching it resets to the seed

	toggleCount = 100 // check boxes, switches
	checkCols   = 5
	switchCols  = 10

	radioGroups  = 20
	radioChoices = 5

	sliderCount   = 20
	progressCount = 50

	tabCount = 10
	tabLines = 20
	tabEvery = 10 // frames between tab changes

	menuCount = 12 // fits one 600 px screen in all three (scenes.md)
	menuWidth = 240

	overlayEvery = 30 // frames between dialog toggles, between toasts
	toastFor     = time.Second

	treeGroups = 100
	treeGroup  = 100 // a group node and its 99 leaves
	treeNodes  = treeGroups * treeGroup
	treeMargin = 50 // rows kept off either end when centering a row

	pickHueS = 0.75 // the color picker's saturation and value
	pickHueV = 0.85
)

const (
	dialogTitle    = "Edit profile"
	dialogCancel   = "Cancel"
	dialogOK       = "Save"
	dialogBackdrop = "A dialog opens and closes over this page every 30 frames."
	toastBackdrop  = "A toast shows every 30 frames and goes after a second."
)

// dialogFields are the dialog's three inputs.
var dialogFields = [3]string{"Ada Lovelace", "ada@example.org", "London"}

func buttonLabel(i int) string  { return "Button " + strconv.Itoa(i+1) }
func clickedLabel(f int) string { return "Clicked " + strconv.Itoa(f) }
func checkLabel(i int) string   { return "Check " + strconv.Itoa(i+1) }
func radioLabel(i int) string   { return "Opt " + strconv.Itoa(i+1) }
func tabLabel(i int) string     { return "Tab " + strconv.Itoa(i+1) }
func menuKey(i int) string      { return "item-" + strconv.Itoa(i) }
func menuLabel(i int) string    { return "Menu item " + strconv.Itoa(i+1) }
func toastText(n int) string    { return "Toast number " + strconv.Itoa(n) }

func tabLine(tab, line int) string {
	return "Tab " + strconv.Itoa(tab+1) + ", line " + strconv.Itoa(line+1) + ": the content of the chosen tab."
}

// switchStart is switch i's state before the first frame.
func switchStart(i int) bool { return i%3 == 0 }

// sliderStart and progressStart are the starting values, 0..100.
func sliderStart(i int) float64   { return float64(i * 5) }
func progressStart(i int) float64 { return float64(i * 2) }

// nextPercent is one step of a slider or a bar: +1, past 100 back to 0.
func nextPercent(v float64) float64 {
	if v++; v > 100 {
		return 0
	}
	return v
}

// typeInto is input text after frame f types into it: one letter more,
// or the seed again once it would be longer than inputMax.
func typeInto(s string, f int) string {
	s += string(rune('a' + f%26))
	if len(s) > inputMax {
		return inputSeed
	}
	return s
}

// textareaSeed is the text area's starting text: textareaLines lines of
// textareaLine characters (newline included), 2000 in all.
func textareaSeed() string {
	var b strings.Builder
	for i := range textareaLines {
		line := "Line " + strconv.Itoa(i+1) + " of the text area scene"
		line += strings.Repeat(".", textareaLine-1-len(line))
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

// appendText is the text area's text after frame f: one letter more, or
// the seed again once it reaches textareaMax.
func appendText(s, seed string, f int) string {
	s += string(rune('a' + f%26))
	if len(s) >= textareaMax {
		return seed
	}
	return s
}

// treeTitle is a tree node's text. Node ids are their rows: group g is
// id g*treeGroup, its leaves follow it.
func treeTitle(id int) string {
	g, j := id/treeGroup, id%treeGroup
	if j == 0 {
		return "Group " + strconv.Itoa(g+1)
	}
	return "Item " + strconv.Itoa(g+1) + "." + strconv.Itoa(j)
}

// treeRow is the row frame f centers: one further each frame, kept
// treeMargin rows from either end, so that every frame scrolls one row.
func treeRow(f int) int { return treeMargin + f%(treeNodes-2*treeMargin) }

// pickDay is the date frame f selects: one day later each frame, through
// the 31 days of March 2026 and round again.
func pickDay(f int) time.Time { return time.Date(2026, time.March, 1+f%31, 0, 0, 0, 0, time.UTC) }

// hueRGB is the color picker's color at hue h degrees (saturation
// pickHueS, value pickHueV), in 8-bit RGB.
func hueRGB(h int) (r, g, b uint8) {
	c := pickHueV * pickHueS
	x := c * (1 - math.Abs(math.Mod(float64(h)/60, 2)-1))
	m := pickHueV - c
	var rf, gf, bf float64
	switch h / 60 {
	case 0:
		rf, gf = c, x
	case 1:
		rf, gf = x, c
	case 2:
		gf, bf = c, x
	case 3:
		gf, bf = x, c
	case 4:
		rf, bf = x, c
	default:
		rf, bf = c, x
	}
	to := func(v float64) uint8 { return uint8(math.Round((v + m) * 255)) }
	return to(rf), to(gf), to(bf)
}

// Nearest-counterpart scenes (Task 11; ../scenes.md, "Nearest-counterpart
// scenes"). Where an opponent has no such component, its program composes
// one from the components it has, as the mapping there says.
const (
	selectCount = 8 // options of the open select, all in view
	selectWidth = 240

	tableRows  = 1000
	tableCols  = 6
	tableShown = 8 // the rows whose last cell changes, in view in all three
	proShown   = 5 // the same for the ProTable scene, under its query form

	pageSize  = 10
	pageTotal = 500 // items, so 50 pages
	pageCount = pageTotal / pageSize

	cascadeFan   = 5 // options on each of the cascader's 3 levels, all in view
	cascadeWidth = 160

	transferCount = 10

	treeSelGroups = 2 // 8 rows in all, which fit antd's 256 px dropdown
	treeSelLeaves = 3 // leaves of each group, so 6 to choose from

	stepCount     = 5
	timelineCount = 8
	descCount     = 12
	descCols      = 3
	formFields    = 10 // in two columns of 5
	uploadCount   = 8

	carouselCount  = 5
	carouselHeight = 240

	collapseCount = 6
	collapseLines = 3

	drawerWidth = 360
	tourCount   = 3

	segCount   = 10 // segmented controls
	segChoices = 4

	rateCount = 20
	rateMax   = 5
	rateCols  = 2

	tagCount   = 60
	tagCols    = 6
	badgeCount = 40
	badgeCols  = 10
	badgeSize  = 40

	cardCount = 12
	cardCols  = 4
	statCount = 12
	statCols  = 4

	proRows = 20 // the ProTable scene's rows (one antd page)
)

const (
	tooltipAnchor  = "Hover target"
	tooltipText    = "Tooltip text for the target"
	tooltipEvery   = overlayEvery // frames between show and hide
	drawerTitle    = "Settings"
	drawerBackdrop = "A drawer opens and closes over this page every 30 frames."
	uploadButton   = "Upload"
	formSubmit     = "Submit"
	queryButton    = "Query"
	resetButton    = "Reset"
	transferLeft   = "Source"
	transferRight  = "Target"
	collapseEvery  = overlayEvery // frames between accordion changes (longer than the motion)
	carouselEvery  = overlayEvery
	tourEvery      = overlayEvery
)

// tableTitles are the table scenes' column titles.
var tableTitles = [tableCols]string{"ID", "Name", "City", "Score", "Latency", "Updated"}

// tableCell is the text of row r's column col (col < tableCols-1); the
// last column holds the frame that last updated the row, "0" at first.
func tableCell(r, col int) string {
	switch col {
	case 0:
		return "#" + strconv.Itoa(r+1)
	case 1:
		return "Name " + strconv.Itoa(r+1)
	case 2:
		return "City " + strconv.Itoa(r%20+1)
	case 3:
		return strconv.Itoa(r * 7 % 1000)
	default:
		return strconv.Itoa(r*13%100) + " ms"
	}
}

// tableRow and proRow are the rows frame f updates in the table and
// ProTable scenes: their Updated cell becomes f.
func tableRow(f int) int { return f % tableShown }
func proRow(f int) int   { return f % proShown }

func selectLabel(i int) string { return "Option " + strconv.Itoa(i+1) }

// pageOf is the page frame f shows, 1..pageCount.
func pageOf(f int) int { return 1 + f%pageCount }

// pageItem is the text of the i-th item on page p.
func pageItem(p, i int) string { return "Item " + strconv.Itoa((p-1)*pageSize+i+1) }

// cascadePath is the option chosen on each level at frame f: the last
// level moves every frame, the middle one every cascadeFan frames.
func cascadePath(f int) [3]int {
	return [3]int{f / (cascadeFan * cascadeFan) % cascadeFan, f / cascadeFan % cascadeFan, f % cascadeFan}
}

// cascadeLabel is the label of option i on the level under the options
// chosen above it (above holds the choices of the levels above).
func cascadeLabel(above []int, i int) string {
	name := [3]string{"Region ", "City ", "District "}[len(above)]
	for _, a := range above {
		name += strconv.Itoa(a+1) + "."
	}
	return name + strconv.Itoa(i+1)
}

// cascadeText is the path's text as the field shows it.
func cascadeText(p [3]int) string {
	return cascadeLabel(nil, p[0]) + " / " + cascadeLabel(p[:1], p[1]) + " / " + cascadeLabel(p[:2], p[2])
}

func transferLabel(i int) string { return "Item " + strconv.Itoa(i+1) }

// transferStart is whether item i starts on the target side.
func transferStart(i int) bool { return i%3 == 0 }

func treeSelGroup(g int) string   { return "Group " + strconv.Itoa(g+1) }
func treeSelLeaf(g, j int) string { return "Leaf " + strconv.Itoa(g+1) + "." + strconv.Itoa(j+1) }

// treeSelPick is the leaf frame f chooses: group, leaf.
func treeSelPick(f int) (int, int) {
	n := f % (treeSelGroups * treeSelLeaves)
	return n / treeSelLeaves, n % treeSelLeaves
}

func stepTitle(i int) string { return "Step " + strconv.Itoa(i+1) }
func stepDesc(i int) string  { return "Details " + strconv.Itoa(i+1) }

func timelineTime(i int) string { return "2026-03-" + twoDigits(i+1) + " 09:00" }

// timelineText is item i's text, once frame f (0 = never) changed it.
func timelineText(i, f int) string {
	if f == 0 {
		return "Event " + strconv.Itoa(i+1)
	}
	return "Event " + strconv.Itoa(i+1) + ", frame " + strconv.Itoa(f)
}

func descLabel(i int) string { return "Field " + strconv.Itoa(i+1) }

// descValue is item i's value, once frame f (0 = never) changed it.
func descValue(i, f int) string {
	if f == 0 {
		return "Value " + strconv.Itoa(i+1)
	}
	return "Value " + strconv.Itoa(i+1) + " @ " + strconv.Itoa(f)
}

func formLabel(i int) string  { return "Field " + strconv.Itoa(i+1) }
func uploadName(i int) string { return "file-" + strconv.Itoa(i+1) + ".png" }

// uploadStart is file i's starting percent; each frame moves one file
// with nextPercent, so a file never finishes and its row keeps its look.
func uploadStart(i int) float64 { return float64(i * 12) }

func slideLabel(i int) string    { return "Slide " + strconv.Itoa(i+1) }
func collapseLabel(i int) string { return "Panel " + strconv.Itoa(i+1) }
func collapseLine(i, l int) string {
	return "Panel " + strconv.Itoa(i+1) + ", line " + strconv.Itoa(l+1) + ": the content of the panel."
}

var (
	tourButtons = [tourCount]string{"Upload", "Save", "More"}
	tourTitles  = [tourCount]string{"Upload files", "Save changes", "More actions"}
	segLabels   = [segChoices]string{"Day", "Week", "Month", "Year"}
)

func tourDesc(i int) string { return "Step " + strconv.Itoa(i+1) + " of the tour." }

// segStart, rateStart are the starting choices.
func segStart(i int) int  { return i % segChoices }
func rateStart(i int) int { return i % (rateMax + 1) }

func tagLabel(i int) string { return "Tag " + strconv.Itoa(i+1) }

// taggedLabel is tag i's text after frame f changed it.
func taggedLabel(i, f int) string { return "Tag " + strconv.Itoa(i+1) + " · " + strconv.Itoa(f) }

// badgeStart is badge i's starting count; nextCount keeps counts in
// 1..99, where all three show the number itself (antd hides 0 and shows
// "99+" above 99).
func badgeStart(i int) int   { return i*7%99 + 1 }
func nextCount(n int) int    { return n%99 + 1 }
func cardTitle(i int) string { return "Card " + strconv.Itoa(i+1) }
func cardLine(i int) string  { return "Line one of card " + strconv.Itoa(i+1) + "." }

// cardUpdated is a card's second line, f the frame that last changed it.
func cardUpdated(f int) string { return "Updated at frame " + strconv.Itoa(f) }

func statTitle(i int) string { return "Metric " + strconv.Itoa(i+1) }

// statValue is metric i's value at frame f (f 0 before the first frame):
// 100..999.25, so no framework groups its digits; shown with 2 decimals
// (statText).
func statValue(i, f int) float64 { return float64((i*137+f*13)%900+100) + 0.25 }
func statText(v float64) string  { return strconv.FormatFloat(v, 'f', 2, 64) }

func twoDigits(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}

// pagerItems are the page buttons a pager of pageCount pages shows at
// page p, 0 standing for an ellipsis: antd's rule (two pages either side
// of p, the first and last always, an ellipsis for the pages left out),
// which Tango's Pagination follows and MyGo's composed pager copies.
func pagerItems(p int) []int {
	const buffer = 2
	if pageCount <= 2*buffer+5 {
		items := make([]int, pageCount)
		for i := range items {
			items[i] = i + 1
		}
		return items
	}
	left, right := max(1, p-buffer), min(pageCount, p+buffer)
	if p-1 <= buffer {
		right = 1 + 2*buffer
	}
	if pageCount-p <= buffer {
		left = pageCount - 2*buffer
	}
	var items []int
	if left != 1 {
		items = append(items, 1)
		if p-1 >= 2*buffer && p != 1+2 {
			items = append(items, 0)
		}
	}
	for i := left; i <= right; i++ {
		items = append(items, i)
	}
	if right != pageCount {
		if pageCount-p >= 2*buffer && p != pageCount-2 {
			items = append(items, 0)
		}
		items = append(items, pageCount)
	}
	return items
}
