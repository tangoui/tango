package ipc

import "io"

// streamRW is the io.ReadWriteCloser view of a Stream. It is the Stream
// itself under another method set, so every ReadWriter call returns the
// same view, and its state (the read leftover) lives in the Stream.
type streamRW Stream

// ReadWriter returns s as an io.ReadWriteCloser over byte frames:
//
//   - Read reads the byte frames as one continuous byte stream: frame
//     boundaries are invisible, and the unread rest of a frame is kept for
//     the next Read. Empty byte frames contribute nothing. It returns
//     io.EOF once the peer ended its direction and everything before the
//     end was read. A JSON frame at the head of the queue makes Read return
//     an *Error with CodeBadArgs and stays queued (Receive can take it).
//     Other errors are those of ReceiveBytes.
//   - Write sends p as one byte frame, or, when p is larger than
//     MaxFrameBytes, as consecutive frames of at most MaxFrameBytes each
//     (the view keeps no boundaries, so a write may be split); an empty p
//     returns 0, nil and sends nothing (as net.Conn does). Frames of one
//     Write are not interleaved with another Write's through the view. On
//     error it returns the bytes of the frames already sent.
//   - Close is CloseSend: a half close, so the peer sees the end (say after
//     io.Copy into the view) while this side keeps reading.
//
// Every call returns the same view, with a single read leftover. One
// reader and one writer may use it concurrently; concurrent Reads (or
// Writes) are serialized. Receive* called directly on s does not see the
// read leftover: it takes the next whole frame after it.
func (s *Stream) ReadWriter() io.ReadWriteCloser { return (*streamRW)(s) }

func (v *streamRW) stream() *Stream { return (*Stream)(v) }

// Read implements io.Reader; see ReadWriter.
func (v *streamRW) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	s := v.stream()
	s.rwReadMu.Lock()
	defer s.rwReadMu.Unlock()
	for len(s.rwRest) == 0 {
		b, err := s.ReceiveBytes()
		if err != nil {
			return 0, err
		}
		s.rwRest = b // an empty frame loops for the next one
	}
	n := copy(p, s.rwRest)
	s.rwRest = s.rwRest[n:]
	if len(s.rwRest) == 0 {
		s.rwRest = nil // let the frame be collected
	}
	return n, nil
}

// Write implements io.Writer; see ReadWriter.
func (v *streamRW) Write(p []byte) (int, error) {
	s := v.stream()
	if len(p) == 0 {
		return 0, nil // like net.Conn: nothing to send, no empty frame
	}
	s.rwWriteMu.Lock()
	defer s.rwWriteMu.Unlock()
	n := 0
	for {
		chunk := p[n:min(n+MaxFrameBytes, len(p))]
		// WriteBytes copies or encodes chunk, so p is not retained.
		if err := s.WriteBytes(chunk); err != nil {
			return n, err
		}
		n += len(chunk)
		if n == len(p) {
			return n, nil
		}
	}
}

// Close implements io.Closer: it is CloseSend.
func (v *streamRW) Close() error { return v.stream().CloseSend() }
