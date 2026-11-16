//go:build darwin && !ios

package darwin

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

func TestReadAssetBodyLimit(t *testing.T) {
	body, err := readAssetBody(bytes.NewReader([]byte("small")), 16)
	if err != nil || string(body) != "small" {
		t.Fatalf("small body = %q, %v", body, err)
	}
	body, err = readAssetBody(bytes.NewReader(bytes.Repeat([]byte{'x'}, 17)), 16)
	if !errors.Is(err, errAssetRequestBodyTooLarge) || len(body) != 16 {
		t.Fatalf("large body = %d, %v", len(body), err)
	}
}

func TestReadAssetBodyPropagatesReaderError(t *testing.T) {
	want := errors.New("read failed")
	body, err := readAssetBody(errReader{err: want}, 16)
	if !errors.Is(err, want) || body == nil {
		t.Fatalf("body/error = %v, %v", body, err)
	}
}

type errReader struct{ err error }

func (r errReader) Read([]byte) (int, error) { return 0, r.err }

var _ io.Reader = errReader{}
