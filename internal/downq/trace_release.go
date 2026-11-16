//go:build !tango_dev

package downq

import "time"

func TraceEnabled() bool       { return false }
func TraceReset()              {}
func TraceStats() []TraceBatch { return nil }

func traceEnd(*State)                                                     {}
func tracePush(*State, uint64)                                            {}
func traceAck(*State, uint64)                                             {}
func tracePollStart(*State, uint64, uint64, time.Time) *traceRound        { return nil }
func traceTake(*traceRound, *State, uint64, []Item, time.Time, time.Time) {}
func traceEncoded(*traceRound, int, time.Time, time.Time)                 {}
func traceWritten(*traceRound, time.Time, time.Time)                      {}
