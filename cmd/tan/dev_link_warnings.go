package main

import (
	"bytes"
	"io"
	"regexp"
)

var devMacOSDeploymentWarning = regexp.MustCompile(`^ld: warning: object file \(.+\) was built for newer 'macOS' version \([0-9.]+\) than being linked \([0-9.]+\)$`)

// devLinkWarningWriter filters complete lines only. Go's build subprocess
// can split a diagnostic across writes, including its final newline.
type devLinkWarningWriter struct {
	dst     io.Writer
	pending []byte
}

func (w *devLinkWarningWriter) Write(p []byte) (int, error) {
	written := 0
	for len(p) > 0 {
		i := bytes.IndexByte(p, '\n')
		if i < 0 {
			w.pending = append(w.pending, p...)
			return written + len(p), nil
		}
		w.pending = append(w.pending, p[:i+1]...)
		if err := w.flush(); err != nil {
			return written, err
		}
		written += i + 1
		p = p[i+1:]
	}
	return written, nil
}

func (w *devLinkWarningWriter) flush() error {
	if len(w.pending) == 0 {
		return nil
	}
	line := w.pending
	w.pending = w.pending[:0]
	text := bytes.TrimSuffix(bytes.TrimSuffix(line, []byte("\n")), []byte("\r"))
	if bytes.Equal(text, []byte("ld: warning: ignoring duplicate libraries: '-lobjc'")) || devMacOSDeploymentWarning.Match(text) {
		return nil
	}
	n, err := w.dst.Write(line)
	if err == nil && n != len(line) {
		return io.ErrShortWrite
	}
	return err
}
