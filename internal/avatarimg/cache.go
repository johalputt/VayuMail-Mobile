// Package avatarimg fetches and caches mailbox profile pictures from a VayuPress
// server's federated avatar endpoint (Libravatar-compatible /avatar/<md5>), so the
// app can show a mailbox's real picture — an uploaded photo or a prebuilt cartoon —
// instead of only a letter. Raster formats decode natively; the server's
// self-contained cartoon SVGs are rasterised with a pure-Go renderer.
//
// Privacy: only addresses on a domain the app is signed into are ever requested
// (SetAllowedDomains). Opening a message therefore never pings a stranger's server,
// so this can never behave like a tracking pixel. All network work happens on a
// background goroutine — never the frame loop (constitution Rule 5).
package avatarimg

import (
	"bytes"
	"crypto/md5" //nolint:gosec // Libravatar/Gravatar federation mandates an md5 address digest; not a security primitive.
	"encoding/hex"
	"image"
	"image/draw"
	_ "image/gif"  // register GIF decoder
	_ "image/jpeg" // register JPEG decoder
	_ "image/png"  // register PNG decoder
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	_ "golang.org/x/image/webp" // register WebP decoder

	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
)

const (
	rasterPx     = 128     // size the server's SVG cartoons rasterise to
	maxBytes     = 1 << 20 // 1 MiB cap on a fetched avatar
	missRetry    = 15 * time.Minute
	fetchTimeout = 8 * time.Second
)

type entryState int8

const (
	stateUnknown entryState = iota
	stateLoading
	stateLoaded
	stateMissing
)

type entry struct {
	img   image.Image
	state entryState
	stamp time.Time
}

// Cache is a concurrency-safe, in-memory avatar store. The zero value is not
// usable; call New.
type Cache struct {
	mu      sync.Mutex
	entries map[string]*entry
	allowed map[string]bool
	client  *http.Client
	onLoad  func() // invalidate the UI so a freshly-loaded avatar redraws
}

// New returns a cache. onLoad is called (from a background goroutine) whenever a
// new avatar finishes loading, so the caller can invalidate/redraw its window.
func New(onLoad func()) *Cache {
	return &Cache{
		entries: map[string]*entry{},
		allowed: map[string]bool{},
		client:  &http.Client{Timeout: fetchTimeout},
		onLoad:  onLoad,
	}
}

// SetAllowedDomains restricts which domains the cache will ever contact — only
// addresses on a domain the app is signed into. Idempotent; call it whenever the
// account list changes.
func (c *Cache) SetAllowedDomains(domains []string) {
	next := make(map[string]bool, len(domains))
	for _, d := range domains {
		d = strings.ToLower(strings.TrimSpace(d))
		if d != "" {
			next[d] = true
		}
	}
	c.mu.Lock()
	c.allowed = next
	// Drop negative caches so addresses that were previously disallowed (e.g. an
	// account added after the first frame) get a fresh chance to load.
	for _, e := range c.entries {
		if e.state == stateMissing {
			e.state = stateUnknown
		}
	}
	c.mu.Unlock()
}

// Image returns the decoded avatar for an address if it is cached, else
// (nil, false) — kicking a one-shot background fetch the first time. It is cheap
// and safe to call every frame from the UI goroutine.
func (c *Cache) Image(email string) (image.Image, bool) {
	key := strings.ToLower(strings.TrimSpace(email))
	at := strings.LastIndexByte(key, '@')
	if at <= 0 || at == len(key)-1 {
		return nil, false
	}
	domain := key[at+1:]

	c.mu.Lock()
	e := c.entries[key]
	if e == nil {
		e = &entry{}
		c.entries[key] = e
	}
	switch e.state {
	case stateLoaded:
		img := e.img
		c.mu.Unlock()
		return img, img != nil
	case stateLoading:
		c.mu.Unlock()
		return nil, false
	case stateMissing:
		if time.Since(e.stamp) < missRetry {
			c.mu.Unlock()
			return nil, false
		}
	}
	if !c.allowed[domain] {
		e.state = stateMissing
		e.stamp = time.Now()
		c.mu.Unlock()
		return nil, false
	}
	e.state = stateLoading
	c.mu.Unlock()
	go c.fetch(key, domain)
	return nil, false
}

func (c *Cache) store(key string, img image.Image) {
	c.mu.Lock()
	e := c.entries[key]
	if e == nil {
		e = &entry{}
		c.entries[key] = e
	}
	e.img = img
	e.stamp = time.Now()
	if img != nil {
		e.state = stateLoaded
	} else {
		e.state = stateMissing
	}
	c.mu.Unlock()
	if img != nil && c.onLoad != nil {
		c.onLoad()
	}
}

func (c *Cache) fetch(key, domain string) {
	sum := md5.Sum([]byte(key)) //nolint:gosec // Libravatar address digest, not security.
	url := "https://" + domain + "/avatar/" + hex.EncodeToString(sum[:])
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		c.store(key, nil)
		return
	}
	req.Header.Set("Accept", "image/*")
	resp, err := c.client.Do(req)
	if err != nil {
		c.store(key, nil)
		return
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		c.store(key, nil)
		return
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil || len(raw) == 0 || len(raw) > maxBytes {
		c.store(key, nil)
		return
	}
	c.store(key, decode(raw)) // decode may return nil → negative-cached
}

// decode turns raw avatar bytes into an image: a rasterised SVG (the server's
// self-contained cartoons) or a natively-decoded raster (uploaded photos).
func decode(raw []byte) image.Image {
	if looksSVG(raw) {
		return rasterizeSVG(raw)
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil
	}
	return squareCrop(img)
}

func looksSVG(raw []byte) bool {
	s := bytes.TrimSpace(raw)
	return bytes.HasPrefix(s, []byte("<svg")) || bytes.HasPrefix(s, []byte("<?xml"))
}

func rasterizeSVG(raw []byte) image.Image {
	ic, err := oksvg.ReadIconStream(bytes.NewReader(raw))
	if err != nil {
		return nil
	}
	ic.SetTarget(0, 0, float64(rasterPx), float64(rasterPx))
	rgba := image.NewRGBA(image.Rect(0, 0, rasterPx, rasterPx))
	ic.Draw(rasterx.NewDasher(rasterPx, rasterPx, rasterx.NewScannerGV(rasterPx, rasterPx, rgba, rgba.Bounds())), 1.0)
	return rgba
}

// squareCrop centre-crops to a square so the round avatar frame never distorts a
// non-square uploaded photo.
func squareCrop(img image.Image) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w == h {
		return img
	}
	s := w
	if h < s {
		s = h
	}
	x0 := b.Min.X + (w-s)/2
	y0 := b.Min.Y + (h-s)/2
	out := image.NewRGBA(image.Rect(0, 0, s, s))
	draw.Draw(out, out.Bounds(), img, image.Pt(x0, y0), draw.Src)
	return out
}
