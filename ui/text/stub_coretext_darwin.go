//go:build darwin && !ios && tango_coretext

package text

// Interim stubs of the engine primitives the CoreText build does not
// implement yet. Each later task deletes the stubs it replaces:
//
//   - Task 5: simpleNumericLayout, simpleNumericLayoutInto, fastPath,
//     (*LineCache).fastShape

func simpleNumericLayout(s string, st Style, opt Options, version int) *Layout { return nil }

func simpleNumericLayoutInto(dst *Layout, s string, st Style, opt Options, version int) *Layout {
	return nil
}

// fastPath memoizes the monospace fast path of LineCache.
type fastPath struct{}

func (c *LineCache) fastShape(line string, runs []Run, opt Options) *Layout { return nil }
