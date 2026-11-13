// Package media lets the user pick photos and videos from the library or
// take a photo with the camera.
//
//	paths, err := media.Pick(a, media.PickOptions{Limit: 5})
//	path, err := media.TakePhoto(a)
//
// Pick uses the system picker (PHPickerViewController on iOS, the Android
// Photo Picker), which runs out of process and needs no photo library
// permission. TakePhoto uses the system camera UI (UIImagePickerController,
// ACTION_IMAGE_CAPTURE); iOS needs NSCameraUsageDescription (permission
// "camera" in tan.json) and returns ErrNotConfigured without it.
//
// The returned paths are copies in the app's cache directory (iOS:
// Library/Caches/tango-media, Android: cache/tango-media), readable with
// os.Open; delete them when done. Canceling returns ErrCanceled.
//
// Functions return ErrUnsupported where the backend has no picker or
// camera (desktop: use package dialog) and tango.ErrNotRunning after the
// app quit. On Android call them from a goroutine, not from a main-thread
// callback.
package media

import (
	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/sys/internal/capcall"
)

var (
	ErrUnsupported      = platform.ErrUnsupported
	ErrCanceled         = platform.ErrCanceled
	ErrNotAvailable     = platform.ErrNotAvailable
	ErrPermissionDenied = platform.ErrPermissionDenied
	ErrNotConfigured    = platform.ErrNotConfigured
)

// Type selects what Pick offers.
type Type = platform.MediaType

const (
	Images = platform.MediaImages
	Videos = platform.MediaVideos
)

// PickOptions configures Pick.
type PickOptions struct {
	Types Type // Images, Videos or both; 0 means Images
	Limit int  // most items the user may choose; 0 or 1 = one
}

// Pick shows the photo picker and returns the chosen items' paths in the
// order picked.
func Pick(a *tango.App, o PickOptions) ([]string, error) {
	p, err := capcall.Backend[platform.MediaPicker](a)
	if err != nil {
		return nil, err
	}
	if o.Types == 0 {
		o.Types = Images
	}
	if o.Limit < 1 {
		o.Limit = 1
	}
	opts := platform.PickMediaOptions{Types: o.Types, Limit: o.Limit}
	return capcall.Async(a, func(done func([]string, error)) { p.PickMedia(opts, done) })
}

// TakePhoto opens the camera and returns the path of the JPEG taken.
// ErrNotAvailable means the device has no camera (the iOS simulator).
func TakePhoto(a *tango.App) (string, error) {
	c, err := capcall.Backend[platform.Camera](a)
	if err != nil {
		return "", err
	}
	return capcall.Async(a, func(done func(string, error)) { c.CapturePhoto(done) })
}
