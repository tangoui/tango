//go:build !((darwin || freebsd || linux || netbsd || openbsd || dragonfly) && !ios && !android) && !windows

package single

import "time"

// iOS and Android run one instance by design; elsewhere single instance is
// not implemented.
const mobile = ios || android

func userTag() string { return "" }

func listen(string, func(message)) (server, error) { return nil, ErrUnsupported }

func send(string, []byte, time.Duration) error { return ErrUnsupported }
