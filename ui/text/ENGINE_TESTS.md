# Engine-specific tests (temporary; Task 6 tags them and deletes this file)

Untagged tests that fail under `-tags tango_coretext` because they depend on
go-text output or on a path the CoreText build does not have. Tests of layout
semantics were not weakened; none needed changing in Tasks 4 and 5.

| Test | Package | Why it is go-text specific |
| --- | --- | --- |
| TestCoreTextLightweightShapeOptIn | ui/text | Tests the go-text build's opt-in short-text CoreText path (`coreTextShape`), which the CoreText build replaces by shapeItems (returns nil). |
| TestGolden | ui | Golden pixels recorded with go-text shaping/metrics. |
| TestViewGolden | ui/editor | Golden pixels recorded with go-text shaping/metrics. |
| TestColorInjectedGolden | ui/editor | Golden pixels recorded with go-text shaping/metrics. |
