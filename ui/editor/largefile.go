package editor

// Large files (Monaco 0.56's largeFileOptimizations, checked against its
// sources: TextModel.isTooLargeForTokenization and isTooLargeForSyncing,
// both decided once when the model is created).
//
// A Model larger than LargeFileSize bytes or LargeFileLines lines
// (Model.LargeFile, State.LargeFile; Monaco's isTooLargeForTokenization)
// turns off what Monaco turns off at that threshold:
//
//   - folding (FoldingController: no ranges, no hidden areas; folding.go);
//   - the occurrences of the word at the caret (WordHighlighterContribution
//     creates no WordHighlighter; occurrences.go);
//   - the selection's matches (SelectionHighlighter._setState draws
//     nothing; occurrences.go);
//   - word wrap (Monaco shows such a model through
//     ViewModelLinesFromModelAsIs, whose setWrappingSettings changes
//     nothing): Options.WordWrap and editor.action.toggleWordWrap have no
//     effect (wrapConfig, paint.go).
//
//   - sticky scroll (StickyScrollController/StickyLineCandidateProvider
//     check isTooLargeForTokenization; sticky.go);
//   - the minimap (Task 14 requirement; unlike Monaco 0.56, whose minimap
//     itself stays on and only loses section headers at the worker limit).
//
// Unlike Monaco, syntax highlighting stays on (the Highlighter tokenizes in
// the background). Bracket pair colorization is not tied to this
// threshold: as in Monaco (BracketPairsTextModelPart.canBuildAST) it is off
// only for documents over 5 000 000 bytes (characters in Monaco), which
// have no bracket tree (bracketpairs.go). The matching brackets
// (matchBrackets) stay on as in Monaco.
//
// A Model larger than 50 MB (Monaco's TextModel._MODEL_SYNC_LIMIT, its
// isTooLargeForSyncing: the model is not sent to the editor worker) also
// has no links and no color decorators, which Monaco computes in the
// worker (links.go, colors.go). The unicode highlighter stays on at every
// size: Monaco falls back to its viewport highlighter, which is how the
// View computes the boxes anyway (only the drawn rows; unicode.go).
// Monaco's other large-file degradations concern features the editor does
// not have (code lens and section headers).

// modelSyncLimit is Monaco's TextModel._MODEL_SYNC_LIMIT (50 MB), in
// UTF-8 bytes.
const modelSyncLimit = 50 * 1024 * 1024

// LargeFile reports whether the State's Model is a large file
// (Model.LargeFile): folding, word wrap and the occurrences and selection
// highlights are off.
func (s *State) LargeFile() bool { return s.Model.LargeFile() }

// tooLargeForWorker reports whether the Model is over Monaco's worker
// sync limit: no links, no color decorators.
func (s *State) tooLargeForWorker() bool { return s.Model.syncLarge }
