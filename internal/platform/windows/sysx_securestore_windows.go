//go:build windows && (amd64 || arm64)

package windows

// platform.SecureStore on Windows: generic credentials in the Credential
// Manager (CredWriteW / CredReadW / CredDeleteW), persisted for the user
// on this machine (CRED_PERSIST_LOCAL_MACHINE: not roamed). Values are
// limited to 2560 bytes (CRED_MAX_CREDENTIAL_BLOB_SIZE). The Credential
// Manager is thread-safe, so these run on any goroutine.

import (
	"fmt"
	"syscall"
	"unsafe"

	"github.com/tangoui/tango/internal/platform"
)

var _ platform.SecureStore = (*Backend)(nil)

var (
	sxProcCredWriteW  = advapi32.NewProc("CredWriteW")
	sxProcCredReadW   = advapi32.NewProc("CredReadW")
	sxProcCredDeleteW = advapi32.NewProc("CredDeleteW")
	sxProcCredFree    = advapi32.NewProc("CredFree")
)

const (
	sxCredTypeGeneric         = 1
	sxCredPersistLocalMachine = 2
	sxErrorNotFound           = syscall.Errno(1168) // ERROR_NOT_FOUND
)

// sxCredential is CREDENTIALW.
type sxCredential struct {
	Flags              uint32
	Type               uint32
	TargetName         *uint16
	Comment            *uint16
	LastWritten        uint64 // FILETIME
	CredentialBlobSize uint32
	CredentialBlob     *byte
	Persist            uint32
	AttributeCount     uint32
	Attributes         uintptr
	TargetAlias        *uint16
	UserName           *uint16
}

var _ = [1]struct{}{}[unsafe.Sizeof(sxCredential{})-80]

func (b *Backend) sxTarget(key string) *uint16 {
	return u16(sxSecureTarget(b.opts.ID, b.opts.Name, key))
}

func (b *Backend) SecureSet(key string, value []byte) error {
	if len(value) > sxCredMaxBlob {
		return sxErrTooLarge
	}
	c := sxCredential{
		Type:               sxCredTypeGeneric,
		TargetName:         b.sxTarget(key),
		Comment:            u16("Stored by " + sxAppKey(b.opts.ID, b.opts.Name)),
		CredentialBlobSize: uint32(len(value)),
		Persist:            sxCredPersistLocalMachine,
		UserName:           u16(key),
	}
	if len(value) > 0 {
		c.CredentialBlob = &value[0]
	}
	if r, _, err := sxProcCredWriteW.Call(uintptr(unsafe.Pointer(&c)), 0); r == 0 {
		return fmt.Errorf("tango: CredWriteW: %v", err)
	}
	return nil
}

func (b *Backend) SecureGet(key string) ([]byte, error) {
	var p uintptr
	r, _, err := sxProcCredReadW.Call(uintptr(unsafe.Pointer(b.sxTarget(key))), sxCredTypeGeneric, 0, uintptr(unsafe.Pointer(&p)))
	if r == 0 {
		if err == sxErrorNotFound {
			return nil, platform.ErrNotFound
		}
		return nil, fmt.Errorf("tango: CredReadW: %v", err)
	}
	defer sxProcCredFree.Call(p)
	c := (*sxCredential)(native(p))
	out := []byte{}
	if c.CredentialBlobSize > 0 && c.CredentialBlob != nil {
		out = append(out, unsafe.Slice(c.CredentialBlob, c.CredentialBlobSize)...)
	}
	return out, nil
}

func (b *Backend) SecureDelete(key string) error {
	r, _, err := sxProcCredDeleteW.Call(uintptr(unsafe.Pointer(b.sxTarget(key))), sxCredTypeGeneric, 0)
	if r == 0 && err != sxErrorNotFound {
		return fmt.Errorf("tango: CredDeleteW: %v", err)
	}
	return nil
}
