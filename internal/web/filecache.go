package web

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"rafactools-planner/internal/pdfgen"
	"rafactools-planner/internal/pngrender"
	"rafactools-planner/internal/programme"
	"rafactools-planner/internal/version"
)

// The PDF and PNG of a month are made once and kept until something that they
// show changes. The key is a hash of everything that goes into the file (the
// squadron's settings, the whole month, the calendar link and the app version),
// so any save, move, restore, reset, closure or settings change gives a new key
// and the next request makes a fresh file. Nothing has to remember to clear the
// cache. The hash also serves as the file's ETag, so a browser or calendar app
// that already has the file gets a short "not modified" answer.

// fileCache is a small in-memory cache of rendered files, oldest dropped first.
type fileCache struct {
	mu    sync.Mutex
	limit int
	order []string
	files map[string][]byte
}

func newFileCache(limit int) *fileCache {
	return &fileCache{limit: limit, files: map[string][]byte{}}
}

func (c *fileCache) get(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	b, ok := c.files[key]
	return b, ok
}

func (c *fileCache) put(key string, data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.files[key]; !ok {
		c.order = append(c.order, key)
	}
	c.files[key] = data
	for len(c.order) > c.limit {
		delete(c.files, c.order[0])
		c.order = c.order[1:]
	}
}

// fileHash hashes the given parts (anything JSON can encode), plus the app
// version, into a hex key.
func fileHash(parts ...any) string {
	h := sha256.New()
	enc := json.NewEncoder(h)
	_ = enc.Encode(version.Version)
	for _, part := range parts {
		_ = enc.Encode(part)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// fileKey hashes everything a file is drawn from.
func fileKey(kind string, sq *programme.Squadron, fp *programme.FullProgramme, pdfOpts pdfgen.Options) string {
	h := sha256.New()
	enc := json.NewEncoder(h)
	for _, part := range []any{kind, version.Version, sq, fp, pdfOpts} {
		_ = enc.Encode(part)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// monthFileData returns the PDF or PNG of a month, from the cache when the
// month has not changed, and the key to use as its ETag. A public file leaves
// out the requirement tracker, which names cadets.
func (s *server) monthFileData(kind string, sq *programme.Squadron, fp *programme.FullProgramme, public bool) ([]byte, string, error) {
	if public {
		stripped := *fp
		stripped.Requirements = nil
		fp = &stripped
	}
	opts := s.pdfOptions(sq.ID)
	key := fileKey(kind, sq, fp, opts)
	if data, ok := s.files.get(key); ok {
		return data, key, nil
	}
	var (
		data []byte
		err  error
	)
	switch kind {
	case "pdf":
		data, err = pdfgen.RenderWith(sq, fp, opts)
	case "png":
		data, err = pngrender.Render(sq, fp, pngrender.Options{CalendarURL: opts.CalendarURL, CalendarExpires: opts.CalendarExpires})
	default:
		err = errors.New("unknown file kind " + kind)
	}
	if err != nil {
		return nil, "", err
	}
	s.files.put(key, data)
	return data, key, nil
}

// serveMonthFile writes a rendered file with its ETag, or 304 if the caller
// already has this version.
func serveMonthFile(w http.ResponseWriter, r *http.Request, kind string, data []byte, key string, year, month int, cacheControl string) {
	etag := `"` + key[:32] + `"`
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", cacheControl)
	if strings.Contains(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	name := monthNames[month] + "_" + strconv.Itoa(year) + "_Programme." + kind
	if kind == "png" {
		w.Header().Set("Content-Type", "image/png")
	} else {
		w.Header().Set("Content-Type", "application/pdf")
	}
	w.Header().Set("Content-Disposition", `inline; filename="`+name+`"`)
	_, _ = w.Write(data)
}
