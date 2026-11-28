package linux

import (
	"strconv"

	"github.com/tangoui/tango/internal/imebridge/proto"
	"github.com/tangoui/tango/internal/platform"
)

const (
	ibusPropNormal = iota
	ibusPropToggle
	ibusPropRadio
	ibusPropMenu
	ibusPropSeparator
)

type ibusProperty struct {
	key, label        string
	kind              uint32
	checked, disabled bool
	children          []ibusProperty
}

func ibusProps(modes []proto.Mode, current string, menu []proto.MenuItem) []ibusProperty {
	var props []ibusProperty
	status := ibusProperty{key: "tango-mode", kind: ibusPropNormal, disabled: true}
	commands := ibusProperty{key: "tango-menu", label: "Tango", kind: ibusPropMenu}
	for _, mode := range modes {
		label := mode.Label
		if label == "" {
			label = mode.ID
		}
		commands.children = append(commands.children, ibusProperty{key: "mode:" + mode.ID,
			label: label, kind: ibusPropRadio, checked: mode.ID == current})
		if mode.ID == current {
			status.label = mode.Short
			if status.label == "" {
				status.label = label
			}
		}
	}
	if status.label != "" {
		props = append(props, status)
	}
	for i, item := range menu {
		prop := ibusProperty{key: "menu:" + item.ID, label: item.Title,
			checked: item.Checked, disabled: item.Disabled}
		switch {
		case item.Separator:
			prop.kind, prop.key = ibusPropSeparator, "separator:"+strconv.Itoa(i)
		case item.Checked:
			prop.kind = ibusPropToggle
		}
		commands.children = append(commands.children, prop)
	}
	if len(commands.children) != 0 {
		props = append(props, commands)
	}
	return props
}

type ibusLookup struct {
	pageSize, cursor int
	cursorVisible    bool
	candidates       []platform.IMECandidate
}

func newIBusLookup(list []platform.IMECandidate, selected int) ibusLookup {
	visible := selected >= 0 && selected < len(list)
	return ibusLookup{pageSize: 9, cursor: max(0, min(selected, len(list)-1)),
		cursorVisible: visible, candidates: list}
}

func (table ibusLookup) clickedIndex(row uint32) (int, bool) {
	if table.pageSize == 0 || row >= uint32(table.pageSize) {
		return 0, false
	}
	index := table.cursor - table.cursor%table.pageSize + int(row)
	return index, index < len(table.candidates)
}

func ibusCandidateLabel(c platform.IMECandidate) string {
	if c.Comment == "" {
		return c.Text
	}
	return c.Text + "  " + c.Comment
}
