package transfer

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"net/url"
	"path/filepath"
	"strings"
	"unicode/utf16"
)

// jpegQuality is the offered JPEG's quality: the image editors that
// only read JPEG get a copy indistinguishable at a glance.
const jpegQuality = 90

// Image is img offered as PNG (exact) and JPEG (flattened onto white:
// JPEG has no alpha), both encoded now.
func Image(img image.Image) (Content, error) {
	b := img.Bounds()
	if b.Dx() <= 0 || b.Dy() <= 0 {
		return Content{}, errors.New("transfer: image content needs a non-empty image")
	}
	var p, j bytes.Buffer
	if err := png.Encode(&p, img); err != nil {
		return Content{}, fmt.Errorf("transfer: encode png: %w", err)
	}
	flat := image.NewRGBA(b)
	draw.Draw(flat, b, image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(flat, b, img, b.Min, draw.Over)
	if err := jpeg.Encode(&j, flat, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return Content{}, fmt.Errorf("transfer: encode jpeg: %w", err)
	}
	return Payloads([]string{MimePNG, MimeJPEG}, map[string][]byte{
		MimePNG: p.Bytes(), MimeJPEG: j.Bytes(),
	}), nil
}

// URIs is a uri list (RFC 2483: CRLF-terminated lines), with the uris
// one per line as plain text for targets that only take text.
func URIs(uris ...string) Content {
	var list strings.Builder
	for _, u := range uris {
		list.WriteString(u)
		list.WriteString("\r\n")
	}
	return Merge(Bytes(MimeURIList, []byte(list.String())), Text(strings.Join(uris, "\n")))
}

// Files is local paths as a uri list of file URIs - what a file
// manager copies and takes; relative paths resolve against the
// working directory.
func Files(paths ...string) Content {
	uris := make([]string, 0, len(paths))
	for _, p := range paths {
		if abs, err := filepath.Abs(p); err == nil {
			p = abs
		}
		uris = append(uris, FileURI(p))
	}
	return URIs(uris...)
}

// FileURI encodes an absolute local path as a file:// uri, percent
// escapes included; anything else returns unchanged, for the consumer
// to reject.
func FileURI(path string) string {
	if !filepath.IsAbs(path) {
		return path
	}
	return (&url.URL{Scheme: "file", Path: path}).String()
}

// URIPath decodes a local file:// uri to its path; remote hosts,
// other schemes, bad escapes, and empty paths are not local files.
func URIPath(uri string) (string, bool) {
	if !strings.HasPrefix(uri, "file:") {
		return "", false
	}
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "file" || u.Host != "" && u.Host != "localhost" || u.Path == "" {
		return "", false
	}
	return u.Path, true
}

// HTML is formatted text: html for rich targets, plain for the rest.
func HTML(html, plain string) Content {
	return Merge(Bytes(MimeHTML, []byte(html)), Text(plain))
}

// ParseURIList reads a uri list: one uri per line, comment lines
// (starting with #) and blank lines skipped, CR tolerated.
func ParseURIList(data []byte) []string {
	var uris []string
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		uris = append(uris, line)
	}
	return uris
}

// FilePaths keeps the local file uris of uris, as paths.
func FilePaths(uris []string) []string {
	var paths []string
	for _, u := range uris {
		if p, ok := URIPath(u); ok {
			paths = append(paths, p)
		}
	}
	return paths
}

// DecodeHTML returns an HTML payload as a string: UTF-8 as sent, or
// UTF-16 when it opens with a byte-order mark (some browsers write
// their HTML that way).
func DecodeHTML(data []byte) string {
	var be bool
	switch {
	case bytes.HasPrefix(data, []byte{0xFF, 0xFE}):
	case bytes.HasPrefix(data, []byte{0xFE, 0xFF}):
		be = true
	default:
		return string(bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF}))
	}
	data = data[2:]
	units := make([]uint16, len(data)/2)
	for i := range units {
		lo, hi := data[2*i], data[2*i+1]
		if be {
			lo, hi = hi, lo
		}
		units[i] = uint16(lo) | uint16(hi)<<8
	}
	return string(utf16.Decode(units))
}
