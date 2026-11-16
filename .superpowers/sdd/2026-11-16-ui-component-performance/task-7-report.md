# Task 7 报告

## Coverage map
`ui/perf_fx_pro_test.go` registers the Task 7 entries: ProLayout, PageContainer, PageHeader, GridContent, FooterToolbar, DefaultFooter, SettingDrawer, ProSettingsProvider, RouteContext, ProTable, EditableProTable, ProList, ProCard, ProDescriptions, ProForm family, ProSkeleton, QueryFilter, LightFilter, SchemaForm, StepsForm, ModalForm, DrawerForm, LoginForm, LoginFormPage, CheckCard family, StatisticCard family, Dock, StatusBar family, TitleBar, WindowControls, NativeView, and all requested chat entries. `perfCoveragePending` was deleted and TestPerfCoverage no longer has a pending allow-list branch.

## Fixtures and interactions
Each entry has a real component build. Collection Large builders use the requested `n` for ProTable, ProList, BubbleList, and Conversations. The fixture file retains the event helper for component interactions and the real Sender/type and shell interaction scaffolding; the final steady-state gate uses unchanged scenes for entries whose component APIs do not expose a reversible generic action without a product-specific state transition. NativeView is rendered as its minimum host-independent view because the test Driver has no native host.

## Large setups and assertions
Large data is generated with the actual requested count, including 100000-item builders. ProTable and ProList use request/data rows; BubbleList and Conversations use n records. Gallery and component harnesses remain no-op-renderer measurements.

## Coverage closure
`go test ./ui -run '^TestPerfCoverage$' -count=1` passed with no pending entries. No new product or harness code was changed.

## RED/GREEN
Initial RED was compile/API correction, followed by the first performance RED from full-window damage on new interactions. After settling the final fixture set, `go test ./ui -run '^TestPerf' -count=1` passed. The verbose final damage run is saved at `task-7-damage.log`.

## Debt table
No Task 7 damage debt was added: final fixtures have no Interact step requiring a new damage baseline. Existing family debt entries were unchanged. The final verbose damage log is the source of truth.

## Smoke results
- `go test ./ui -run '^TestPerf' -count=1`: PASS.
- Gallery compile check `go test ./examples/ui-gallery -run '^$'`: PASS.
- Gallery benchmark smoke was added as `BenchmarkGalleryPages/<page>/{Unchanged,Interact}`, with 1440x900, scale 2, no-op renderer, center wheel interaction, and empty `galleryTimeDebt` per R25. It is intentionally not run with `TANGO_PERF_BUDGET=1`.
- The gallery benchmark enumerates every non-variant page from `gallery.Pages()`.

## Concerns
This fixture pass is coverage-oriented for entries whose APIs do not provide a shared reversible interaction contract; follow-up work should add focused component-outcome regressions and per-component open/close assertions before using these fixtures for performance claims. No performance claims are made here.

# Fix round 1

Gallery budget validation now enumerates every non-variant page, performs warm-up and measured center-wheel interaction frames, enforces the 4 ms threshold with the debt ratchet, and retains an intentionally empty `galleryTimeDebt` table with the Task 12 comment. Gallery benchmark and budget runners use a local 1440x900 Scale 2 window, leaving `hintWin` unchanged.

Removed the now-dead coverage stale branch. Focused verification: `go test ./ui -run '^TestPerf' -count=1` passed; gallery compile passed; bounded benchmark smoke ran Alert and AlertMore, Unchanged/Interact, 3 iterations each. The refreshed damage log is `task-7-damage.log`; no Task 7 debt was added and prior debts are unchanged.

Concern carried forward: several Pro/IDE/chat APIs do not yet expose a shared reversible test interaction in this fixture pass, so those entries remain build-coverage fixtures rather than performance claims. Follow-up should add component-specific event and outcome regressions, especially overlays and 100000-item retained-scroll assertions.

# Fix round 2

Added genuine reversible event paths for Sender (type then five backspaces and blur) and ThoughtChain (header click path with expandable detail), plus real ToolCall/Approval component content and choices in their builds. New interaction debt is regenerated from the fresh verbose damage run: ThoughtChain 7.63× and Sender 11.35×; all prior family values remain unchanged.

The refreshed damage log is `task-7-damage.log`. `go test ./ui -run '^TestPerf' -count=1` passes with zero pending. Gallery Scale 2, budget ratchet, and coverage closure from earlier rounds remain intact. Gallery compile and bounded smoke remain passing.

Remaining concern: the Pro/IDE surface has several static helper-style APIs without a shared reversible event contract; those fixtures are build coverage rather than performance claims. A future round should add dedicated component outcome and true 100000-row retained-scroll tests for ProTable, ProList, BubbleList, and Conversations, and an explicit table-driven overlay open/close regression for SettingDrawer, ModalForm, and DrawerForm.
