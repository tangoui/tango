package ui

// SchemaLayoutType selects the form shell of a SchemaForm.
type SchemaLayoutType string

// Form shells of SchemaForm.LayoutType.
const (
	SchemaLayoutForm        SchemaLayoutType = "Form"        // ProForm (default)
	SchemaLayoutModalForm   SchemaLayoutType = "ModalForm"   // ProForm in a Modal (needs Open)
	SchemaLayoutDrawerForm  SchemaLayoutType = "DrawerForm"  // ProForm in a Drawer (needs Open)
	SchemaLayoutStepsForm   SchemaLayoutType = "StepsForm"   // one step per run of fields with the same Group
	SchemaLayoutQueryFilter SchemaLayoutType = "QueryFilter" // query form with search / reset
	SchemaLayoutLightFilter SchemaLayoutType = "LightFilter" // compact filter pills
)

// SchemaFormW renders a form from a field schema in one of the form shells
// (Ant Design Pro BetaSchemaForm): the same []ProField drives ProForm,
// ModalForm, DrawerForm, StepsForm, QueryFilter or LightFilter. Nested lists
// are fields of ValueType ValueFormList with ProField.List.
//
//	ui.SchemaForm(c, schema).LayoutType(ui.SchemaLayoutModalForm).
//		Open(&open).Title("新建规则").Values(&vals).
//		OnFinish(func(v ui.ProValues, done func(error)) { go save(v, done) }).
//		Show()
type SchemaFormW struct {
	_        [0]func()
	c        *Context
	schema   []ProField
	lt       SchemaLayoutType
	vals     *ProValues
	onFinish func(vals ProValues, done func(error))
	cols     int
	readonly bool
	open     *bool
	title    string
	width    float32

	finished bool
	searched bool
	params   map[string]string
}

// SchemaForm starts a form over schema; chain options, then Show.
//
// ConfigProvider.Defaults("SchemaForm", ...) keys: "layoutType" (one of
// the SchemaLayout* names).
func SchemaForm(c *Context, schema []ProField) *SchemaFormW {
	s := &SchemaFormW{c: c, schema: schema, lt: SchemaLayoutForm, cols: 1}
	if v := Prop(c, "SchemaForm", "layoutType", ""); v != "" {
		s.lt = SchemaLayoutType(v)
	}
	return s
}

// LayoutType selects the form shell (default SchemaLayoutForm).
func (s *SchemaFormW) LayoutType(t SchemaLayoutType) *SchemaFormW { s.lt = t; return s }

// Values binds the values (otherwise the shell keeps its own).
func (s *SchemaFormW) Values(v *ProValues) *SchemaFormW { s.vals = v; return s }

// OnFinish receives the validated values of the form shells; call done
// (from any goroutine) when saved.
func (s *SchemaFormW) OnFinish(fn func(vals ProValues, done func(error))) *SchemaFormW {
	s.onFinish = fn
	return s
}

// Columns lays the fields out in n grid columns (form shells).
func (s *SchemaFormW) Columns(n int) *SchemaFormW { s.cols = max(1, n); return s }

// Readonly shows the values read-only (Form, ModalForm, DrawerForm).
func (s *SchemaFormW) Readonly(on bool) *SchemaFormW { s.readonly = on; return s }

// Open binds the visibility of ModalForm and DrawerForm.
func (s *SchemaFormW) Open(open *bool) *SchemaFormW { s.open = open; return s }

// Title sets the dialog title of ModalForm and DrawerForm.
func (s *SchemaFormW) Title(t string) *SchemaFormW { s.title = t; return s }

// Width sets the dialog / drawer width, or the StepsForm form width.
func (s *SchemaFormW) Width(px float32) *SchemaFormW { s.width = px; return s }

// Finished reports a completed submit of a form shell, once.
func (s *SchemaFormW) Finished() bool { return s.finished }

// Searched reports a search of QueryFilter or a change of LightFilter.
func (s *SchemaFormW) Searched() bool { return s.searched }

// Params returns the filter values as text after a QueryFilter or
// LightFilter Show (nil for the form shells).
func (s *SchemaFormW) Params() map[string]string { return s.params }

// Show builds the shell.
func (s *SchemaFormW) Show() *SchemaFormW {
	c := s.c
	switch s.lt {
	case SchemaLayoutModalForm, SchemaLayoutDrawerForm:
		open := s.open
		if open == nil {
			open = State[bool](c, hashID(c.scope, "schema-form-open", s.title), nil)
		}
		var m *ModalFormW
		if s.lt == SchemaLayoutDrawerForm {
			m = DrawerForm(c, open, s.title, s.schema)
		} else {
			m = ModalForm(c, open, s.title, s.schema)
		}
		m.Columns(s.cols).Readonly(s.readonly)
		if s.vals != nil {
			m.Values(s.vals).KeepValues()
		}
		if s.width > 0 {
			m.Width(s.width)
		}
		if s.onFinish != nil {
			m.OnFinish(s.onFinish)
		}
		s.finished = m.Show().Finished()
	case SchemaLayoutStepsForm:
		sf := StepsForm(c, schemaSteps(s.schema, s.cols))
		if s.vals != nil {
			sf.Values(s.vals)
		}
		if s.width > 0 {
			sf.Width(s.width)
		}
		if s.onFinish != nil {
			sf.OnFinish(s.onFinish)
		}
		s.finished = sf.Show().Finished()
	case SchemaLayoutQueryFilter:
		q := QueryFilter(c, s.schema)
		if s.vals != nil {
			q.Values(s.vals)
		}
		q.Show()
		s.searched, s.params = q.Searched(), q.Params()
	case SchemaLayoutLightFilter:
		lf := LightFilter(c, s.schema)
		if s.vals != nil {
			lf.Values(s.vals)
		}
		lf.Show()
		s.searched, s.params = lf.Changed(), lf.Params()
	default:
		pf := ProForm(c, s.schema).Columns(s.cols).Readonly(s.readonly)
		if s.vals != nil {
			pf.Values(s.vals)
		}
		if s.onFinish != nil {
			pf.OnFinish(s.onFinish)
		}
		s.finished = pf.Show().Finished()
	}
	return s
}

// schemaSteps splits a schema into steps: each run of consecutive fields
// with the same Group becomes a step titled by the Group (the fields lose
// the Group so the step does not repeat it as a ProFormGroup title).
func schemaSteps(schema []ProField, cols int) []StepsFormStep {
	var steps []StepsFormStep
	for i := 0; i < len(schema); {
		j := i + 1
		for j < len(schema) && schema[j].Group == schema[i].Group {
			j++
		}
		fields := make([]ProField, j-i)
		copy(fields, schema[i:j])
		for k := range fields {
			fields[k].Group = ""
		}
		steps = append(steps, StepsFormStep{Title: schema[i].Group, Fields: fields, Columns: cols})
		i = j
	}
	return steps
}
