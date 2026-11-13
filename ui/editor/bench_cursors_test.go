package editor

import "testing"

// BenchmarkTypeWith10kCursors: typing one character at 10 000 cursors
// (one per line, at the line ends), one buffer edit set; the undo that
// resets the text between iterations is not timed.
func BenchmarkTypeWith10kCursors(b *testing.B) {
	for _, file := range []string{"a.txt", "a.go"} {
		b.Run(file, func(b *testing.B) {
			doc, sels := manyCursorDoc(10000)
			st := NewState(doc, file, nil)
			st.SetSelections(sels)
			b.ReportAllocs()
			for b.Loop() {
				st.Insert("x")
				b.StopTimer()
				st.Undo()
				b.StartTimer()
			}
		})
	}
}
