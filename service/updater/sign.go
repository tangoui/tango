package updater

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// The manifest is signed as a whole: the published document is an
// envelope holding the manifest JSON and an Ed25519 signature over a
// context string followed by the manifest's bytes exactly as they appear
// in the envelope, so no canonical form is needed and the manifest stays
// readable. Every file is bound to the signature through its SHA-256
// digest in the manifest. Keys and signatures are standard base64.
const manifestContext = "tango-update-manifest-v1\x00"

// SignedManifest is the published envelope.
type SignedManifest struct {
	Manifest  json.RawMessage `json:"manifest"`
	Signature string          `json:"signature"`
}

// ErrBadSignature is returned when a manifest's signature does not verify
// with any of the Updater's public keys.
var ErrBadSignature = errors.New("updater: manifest signature does not verify")

// GenerateKey returns a new signing key pair, base64-encoded. Embed the
// public key in the app (Updater.PublicKey); keep the private key secret
// and pass it to SignManifest when publishing a release.
func GenerateKey() (publicKey, privateKey string, err error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}
	return base64.StdEncoding.EncodeToString(pub), base64.StdEncoding.EncodeToString(priv), nil
}

// PublicKeyOf returns the public key of privateKey (GenerateKey's private
// key, or a base64 32-byte seed).
func PublicKeyOf(privateKey string) (string, error) {
	priv, err := parsePrivateKey(privateKey)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(priv.Public().(ed25519.PublicKey)), nil
}

// SignManifest validates m and returns the signed envelope to publish.
func SignManifest(privateKey string, m *Manifest) ([]byte, error) {
	priv, err := parsePrivateKey(privateKey)
	if err != nil {
		return nil, err
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	body, err := json.MarshalIndent(m, "  ", "  ")
	if err != nil {
		return nil, err
	}
	sig := ed25519.Sign(priv, append([]byte(manifestContext), body...))
	var b bytes.Buffer
	b.WriteString("{\n  \"manifest\": ")
	b.Write(body)
	fmt.Fprintf(&b, ",\n  \"signature\": %q\n}\n", base64.StdEncoding.EncodeToString(sig))
	return b.Bytes(), nil
}

// VerifyManifest checks the envelope's signature against publicKeys (any
// one may match, which allows rotating keys) and only then decodes and
// validates the manifest.
func VerifyManifest(envelope []byte, publicKeys ...string) (*Manifest, error) {
	var env SignedManifest
	if err := json.Unmarshal(envelope, &env); err != nil {
		return nil, fmt.Errorf("updater: manifest: %w", err)
	}
	if len(env.Manifest) == 0 || env.Signature == "" {
		return nil, errors.New("updater: manifest is not signed")
	}
	sig, err := base64.StdEncoding.DecodeString(env.Signature)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return nil, errors.New("updater: malformed manifest signature")
	}
	msg := append([]byte(manifestContext), env.Manifest...)
	ok := false
	for _, k := range publicKeys {
		pub, err := parsePublicKey(k)
		if err != nil {
			return nil, err
		}
		if ed25519.Verify(pub, msg, sig) {
			ok = true
			break
		}
	}
	if !ok {
		return nil, ErrBadSignature
	}
	var m Manifest
	dec := json.NewDecoder(bytes.NewReader(env.Manifest))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("updater: manifest: %w", err)
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return &m, nil
}

// ParseManifest decodes and validates an unsigned manifest, as written by
// hand or by a release script before tan sign-update signs it.
func ParseManifest(b []byte) (*Manifest, error) {
	var m Manifest
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("updater: manifest: %w", err)
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return &m, nil
}

// FileInfo returns the File entry (size and digest) for the file at path,
// with url as its URL.
func FileInfo(path, url string) (File, error) {
	f, err := os.Open(path)
	if err != nil {
		return File{}, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return File{}, err
	}
	return File{URL: url, Size: n, SHA256: fmt.Sprintf("%x", h.Sum(nil))}, nil
}

func parsePublicKey(s string) (ed25519.PublicKey, error) {
	b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s))
	if err != nil || len(b) != ed25519.PublicKeySize {
		return nil, errors.New("updater: malformed public key")
	}
	return ed25519.PublicKey(b), nil
}

func parsePrivateKey(s string) (ed25519.PrivateKey, error) {
	b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s))
	switch {
	case err != nil:
	case len(b) == ed25519.PrivateKeySize:
		return ed25519.PrivateKey(b), nil
	case len(b) == ed25519.SeedSize:
		return ed25519.NewKeyFromSeed(b), nil
	}
	return nil, errors.New("updater: malformed private key")
}

func fileDigest(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}
