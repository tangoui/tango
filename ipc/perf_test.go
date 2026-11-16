package ipc

import (
	"fmt"
	"testing"
)

func BenchmarkPacketCodec(b *testing.B) {
	for _, size := range []int{64, 64 << 10} {
		b.Run(fmt.Sprint(size), func(b *testing.B) {
			frames := []PacketFrame{{Stream: 7, Seq: 9, Payload: make([]byte, size)}}
			packet := AppendPacket(nil, frames...)
			b.Run("append-reuse", func(b *testing.B) {
				dst := make([]byte, 0, len(packet))
				b.ReportAllocs()
				b.SetBytes(int64(size))
				for b.Loop() {
					dst = AppendPacket(dst[:0], frames...)
					if len(dst) != len(packet) {
						b.Fatal("bad length")
					}
				}
			})
			b.Run("parse", func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(size))
				for b.Loop() {
					fs, err := ParsePacket(packet)
					if err != nil || len(fs) != 1 || len(fs[0].Payload) != size {
						b.Fatal("invalid decode", err)
					}
				}
			})
		})
	}
}
