package downq

// TraceBatch is one development-only down:poll response sample. The trace is
// enabled by TANGO_IPC_TRACE=1 in a tango_dev build and is otherwise empty.
// Durations are microseconds; timestamps are Unix nanoseconds.
type TraceBatch struct {
	Round        uint64  `json:"round"`
	Generation   uint64  `json:"generation"`
	RequestAt    int64   `json:"requestAt"`
	FirstSeq     uint64  `json:"firstSeq"`
	Ack          uint64  `json:"ack"`
	Items        int     `json:"items"`
	Frames       int     `json:"frames"`
	DataBytes    int64   `json:"dataBytes"`
	BatchBytes   int     `json:"batchBytes"`
	PushToTakeUs float64 `json:"pushToTakeUs"`
	TakeUs       float64 `json:"takeUs"`
	EncodeUs     float64 `json:"encodeUs"`
	WriteUs      float64 `json:"writeUs"`
	PollGapUs    float64 `json:"pollGapUs"`
}

type traceRound struct {
	state *State
	batch TraceBatch
}
