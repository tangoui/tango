//go:build darwin && !ios

package darwin

import (
	"errors"

	"github.com/tangoui/tango/internal/platform"
)

var _ platform.ClipboardRich = (*backend)(nil)

const (
	pasteboardTypePNG  = "public.png"  // NSPasteboardTypePNG
	pasteboardTypeTIFF = "public.tiff" // NSPasteboardTypeTIFF

	nsBitmapImageFileTypePNG = 4
)

// ClipboardReadImage returns public.png data, converting TIFF (what many
// macOS apps put on the pasteboard, screenshots included) to PNG.
func (b *backend) ClipboardReadImage() ([]byte, error) {
	var out []byte
	var err error
	withPool(func() {
		pb := send(class("NSPasteboard"), "generalPasteboard")
		if d := send(pb, "dataForType:", uintptr(nsString(pasteboardTypePNG))); d != 0 {
			out = goBytes(d)
			return
		}
		tiff := send(pb, "dataForType:", uintptr(nsString(pasteboardTypeTIFF)))
		if tiff == 0 {
			return
		}
		rep := send(class("NSBitmapImageRep"), "imageRepWithData:", uintptr(tiff))
		if rep == 0 {
			err = errors.New("tango/darwin: cannot decode the pasteboard's TIFF image")
			return
		}
		props := send(class("NSDictionary"), "dictionary")
		png := send(rep, "representationUsingType:properties:", nsBitmapImageFileTypePNG, uintptr(props))
		if png == 0 {
			err = errors.New("tango/darwin: cannot convert the pasteboard image to PNG")
			return
		}
		out = goBytes(png)
	})
	return out, err
}

// ClipboardWriteImage puts the PNG on the pasteboard, plus a TIFF copy for
// apps that only read TIFF.
func (b *backend) ClipboardWriteImage(png []byte) error {
	var err error
	withPool(func() {
		data := nsData(png)
		rep := send(class("NSBitmapImageRep"), "imageRepWithData:", uintptr(data))
		if rep == 0 {
			err = errors.New("tango/darwin: cannot decode the PNG image")
			return
		}
		pb := send(class("NSPasteboard"), "generalPasteboard")
		send(pb, "clearContents")
		if !sendBool(pb, "setData:forType:", uintptr(data), uintptr(nsString(pasteboardTypePNG))) {
			err = errors.New("tango/darwin: cannot write to the pasteboard")
			return
		}
		if tiff := send(rep, "TIFFRepresentation"); tiff != 0 {
			send(pb, "setData:forType:", uintptr(tiff), uintptr(nsString(pasteboardTypeTIFF)))
		}
	})
	return err
}

// ClipboardReadFiles reads file URLs (Finder copies) from the pasteboard.
func (b *backend) ClipboardReadFiles() ([]string, error) {
	var paths []string
	withPool(func() {
		pb := send(class("NSPasteboard"), "generalPasteboard")
		key := constPtr(libAppKit, "NSPasteboardURLReadingFileURLsOnlyKey")
		var opts id
		if key != 0 {
			opts = send(class("NSDictionary"), "dictionaryWithObject:forKey:",
				uintptr(send(class("NSNumber"), "numberWithBool:", 1)), key)
		}
		urls := send(pb, "readObjectsForClasses:options:", uintptr(nsArray(class("NSURL"))), uintptr(opts))
		for _, u := range arrayItems(urls) {
			if !sendBool(u, "isFileURL") {
				continue
			}
			if p := goString(send(u, "path")); p != "" {
				paths = append(paths, p)
			}
		}
	})
	return paths, nil
}

// ClipboardWriteFiles puts file URLs on the pasteboard, as Finder's Copy
// does.
func (b *backend) ClipboardWriteFiles(paths []string) error {
	var err error
	withPool(func() {
		urls := make([]id, 0, len(paths))
		for _, p := range paths {
			urls = append(urls, send(class("NSURL"), "fileURLWithPath:", uintptr(nsString(p))))
		}
		pb := send(class("NSPasteboard"), "generalPasteboard")
		send(pb, "clearContents")
		if len(urls) > 0 && !sendBool(pb, "writeObjects:", uintptr(nsArray(urls...))) {
			err = errors.New("tango/darwin: cannot write to the pasteboard")
		}
	})
	return err
}
