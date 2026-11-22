package ui

import (
	"html"
	"image"
	"strconv"
	"strings"

	"github.com/tangoui/tango/ui/geom"
	htmltoken "golang.org/x/net/html"
)

// MarkdownImage describes a Markdown or HTML image. Width and Height
// are positive HTML pixel dimensions, or zero when unspecified.
type MarkdownImage struct {
	Src, Alt      string
	Width, Height float32
}

func mdImageSpec(alt, src string) MarkdownImage {
	return MarkdownImage{Alt: html.UnescapeString(alt), Src: html.UnescapeString(src)}
}

func mdRefKey(label string) string { return strings.ToLower(strings.Join(strings.Fields(label), " ")) }

func mdImageReference(s string) (label, src string, ok bool) {
	j := mdLabelEnd(s, 0)
	if j < 2 || j+1 >= len(s) || s[j+1] != ':' {
		return "", "", false
	}
	label, src = mdRefKey(s[1:j]), strings.TrimSpace(s[j+2:])
	if label == "" || src == "" {
		return "", "", false
	}
	if src[0] == '<' {
		if end := strings.IndexByte(src, '>'); end > 1 {
			return label, src[1:end], true
		}
		return "", "", false
	}
	return label, strings.Fields(src)[0], true
}

func mdMarkdownImage(s string, refs map[string]string) (MarkdownImage, int, bool) {
	if alt, src, end, ok := mdLink(s, 1); ok {
		return mdImageSpec(alt, src), end, true
	}
	if len(refs) == 0 {
		return MarkdownImage{}, 0, false
	}
	j := mdLabelEnd(s, 1)
	if j < 0 {
		return MarkdownImage{}, 0, false
	}
	alt, label, end := s[2:j], s[2:j], j+1
	if end < len(s) && s[end] == '[' {
		k := mdLabelEnd(s, end)
		if k < 0 {
			return MarkdownImage{}, 0, false
		}
		if k > end+1 {
			label = s[end+1 : k]
		}
		end = k + 1
	}
	src, ok := refs[mdRefKey(label)]
	return mdImageSpec(alt, src), end, ok
}

func mdImageAt(s string, refs map[string]string) (MarkdownImage, int, bool) {
	if strings.HasPrefix(s, "![") {
		return mdMarkdownImage(s, refs)
	}
	return mdHTMLImage(s)
}

func mdHTMLImage(s string) (MarkdownImage, int, bool) {
	var im MarkdownImage
	if len(s) <= 4 || !strings.EqualFold(s[:4], "<img") || !strings.ContainsRune(" \t\r\n/>", rune(s[4])) {
		return im, 0, false
	}
	t := htmltoken.NewTokenizer(strings.NewReader(s))
	kind := t.Next()
	if kind != htmltoken.StartTagToken && kind != htmltoken.SelfClosingTagToken {
		return im, 0, false
	}
	end := len(t.Raw())
	seen := make(map[string]bool, 4)
	for _, a := range t.Token().Attr {
		if seen[a.Key] {
			continue
		}
		seen[a.Key] = true
		switch a.Key {
		case "src":
			im.Src = strings.TrimSpace(a.Val)
		case "alt":
			im.Alt = a.Val
		case "width":
			im.Width = mdImageDimension(a.Val)
		case "height":
			im.Height = mdImageDimension(a.Val)
		}
	}
	return im, end, im.Src != ""
}

func mdImageDimension(s string) float32 {
	n, err := strconv.ParseUint(strings.TrimSpace(s), 10, 31)
	if err != nil {
		return 0
	}
	return float32(n)
}

func (m *MarkdownW) loadImage(spec MarkdownImage) (richImage, bool) {
	var img image.Image
	switch {
	case m.resolveImage != nil:
		img = m.resolveImage(spec)
	case m.images != nil:
		img = m.images(spec.Src)
	}
	if img == nil || img.Bounds().Empty() {
		return richImage{}, false
	}
	size := img.Bounds().Size()
	w, h := float32(size.X), float32(size.Y)
	switch {
	case spec.Width > 0 && spec.Height > 0:
		w, h = spec.Width, spec.Height
	case spec.Width > 0:
		w, h = spec.Width, h*spec.Width/w
	case spec.Height > 0:
		w, h = w*spec.Height/h, spec.Height
	}
	scale := float32(1)
	if m.imageMax.W > 0 {
		scale = min(scale, m.imageMax.W/w)
	}
	if m.imageMax.H > 0 {
		scale = min(scale, m.imageMax.H/h)
	}
	return richImage{image: img, w: w * scale, h: h * scale}, true
}

type richImage struct {
	image image.Image
	w, h  float32
}

func (im richImage) fit(maxW float32) geom.Size {
	w := max(0, min(im.w, maxW))
	return geom.Size{W: w, H: im.h * w / im.w}
}
