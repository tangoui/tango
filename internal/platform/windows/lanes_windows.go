//go:build windows && (amd64 || arm64)

package windows

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"unsafe"

	"github.com/tangoui/tango/internal/lane"
	"github.com/tangoui/tango/internal/platform"
)

var (
	_ platform.BinaryLane      = (*surface)(nil)
	_ platform.LaneGenerations = (*surface)(nil)
)

func (s *surface) LaneCaps() []string {
	if lanes := s.shm.Load(); lanes != nil && lanes.hasAssetDocument() {
		return []string{lane.DownShm, lane.UpShm}
	}
	return nil
}

func (s *surface) LaneGeneration(gen uint64, caps []string) {
	if lanes := s.shm.Load(); lanes != nil {
		lanes.LaneGeneration(gen, caps)
	}
}

func (s *surface) SendLanePacket(gen uint64, packet []byte) bool {
	lanes := s.shm.Load()
	return lanes != nil && lanes.SendLanePacket(gen, packet)
}

// initShmLanes runs before the first navigation. The Surface publishes the
// controller only if both interfaces and the document event are available.
func (s *surface) initShmLanes() {
	capacity := shmCapacityFromEnv(os.Getenv, log.Printf)
	host, ok := s.b.h.(platform.LaneHost)
	if !ok || len(laneCapsFromEnv(os.Getenv, true)) == 0 {
		return
	}
	env12 := queryInterface(s.b.env, &iidICoreWebView2Environment12)
	web17 := queryInterface(s.webview, &iidICoreWebView2_17)
	release(env12)
	release(web17)
	if env12 == 0 || web17 == 0 {
		return
	}

	// The create hook returns the mapped bytes. This UI-thread-only map
	// keeps their COM identities available to the post hook.
	buffers := make(map[*byte]uintptr)
	lanes := newShmLanes(capacity, shmOps{
		create: func(size int) ([]byte, func(), error) {
			mem, buf, err := createSharedBuffer(s.b.env, size)
			if err != nil {
				return nil, nil, err
			}
			key := &mem[0]
			buffers[key] = buf
			return mem, func() {
				delete(buffers, key)
				closeSharedBuffer(buf)
			}, nil
		},
		post: func(ring string, gen uint64, buf any) bool {
			if s.closed || s.shmSource() != assetOrigin {
				return false
			}
			mem := buf.([]byte)
			return postSharedBuffer(s.webview, buffers[&mem[0]], ring, gen)
		},
		message: func(msg string) {
			if !s.closed && s.shmSource() == assetOrigin {
				comCall(s.webview, wvPostWebMessageAsString, uintptr(unsafe.Pointer(u16(msg))))
			}
		},
		onUI: s.b.runDeferred,
		deliver: func(gen uint64, packet []byte) bool {
			lanes := s.shm.Load()
			if lanes == nil {
				return false
			}
			origin := lanes.originForGeneration(gen)
			return origin == assetOrigin && host.OnLanePacket(s.id, origin, gen, packet)
		},
	})
	// NavigationStarting invalidates the previous document. ContentLoading
	// identifies the replacement before its scripts can send a hello.
	hr := addWebViewEvent(s.webview, wvAddContentLoading, func(_, _ uintptr) {
		if !s.closed {
			lanes.documentLoaded(s.shmSource())
		}
	})
	if !failed(hr) {
		s.shm.Store(lanes)
	}
}

func (s *surface) shmSource() string { return originOf(comString(s.webview, wvGetSource)) }

func createSharedBuffer(env uintptr, size int) ([]byte, uintptr, error) {
	env12 := queryInterface(env, &iidICoreWebView2Environment12)
	if env12 == 0 {
		return nil, 0, fmt.Errorf("tango: ICoreWebView2Environment12 unavailable")
	}
	defer release(env12)

	var buf uintptr
	hr := comCall(env12, envCreateSharedBuffer, uintptr(size), uintptr(unsafe.Pointer(&buf)))
	if failed(hr) {
		return nil, 0, hresultError("CreateSharedBuffer", hr)
	}
	if buf == 0 {
		return nil, 0, fmt.Errorf("tango: CreateSharedBuffer returned no buffer")
	}
	actual, hr := comOut[uint64](buf, sharedBufferGetSize)
	if failed(hr) || actual != uint64(size) {
		closeSharedBuffer(buf)
		if failed(hr) {
			return nil, 0, hresultError("ICoreWebView2SharedBuffer.get_Size", hr)
		}
		return nil, 0, fmt.Errorf("tango: shared buffer size %d, want %d", actual, size)
	}
	addr, hr := comOut[uintptr](buf, sharedBufferGetBuffer)
	if failed(hr) || addr == 0 {
		closeSharedBuffer(buf)
		if failed(hr) {
			return nil, 0, hresultError("ICoreWebView2SharedBuffer.get_Buffer", hr)
		}
		return nil, 0, fmt.Errorf("tango: get_Buffer returned no mapping")
	}
	return unsafe.Slice((*byte)(native(addr)), size), buf, nil
}

func closeSharedBuffer(buf uintptr) {
	comCall(buf, sharedBufferClose)
	release(buf)
}

func postSharedBuffer(web, buf uintptr, ring string, gen uint64) bool {
	web17 := queryInterface(web, &iidICoreWebView2_17)
	if web17 == 0 {
		return false
	}
	defer release(web17)

	data, _ := json.Marshal(shmMessage{Tango: "lane", Ring: ring, Gen: gen})
	hr := comCall(web17, wvPostSharedBufferToScript, buf, sharedBufferReadWrite,
		uintptr(unsafe.Pointer(u16(string(data)))))
	return !failed(hr)
}
