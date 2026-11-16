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

	menuCount = 50
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
