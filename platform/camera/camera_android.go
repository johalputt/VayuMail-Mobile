//go:build android && cgo

// Android camera frame source: the Go half of the NDK Camera2 bridge (the C
// half is camera_android.c / camera_android.h). Compiled ONLY by the
// Android toolchain and runnable only on a physical device — the desktop
// build uses camera_other.go via build tags. See the package doc.
package camera

/*
#cgo LDFLAGS: -landroid -lcamera2ndk -lmediandk -llog
#include "camera_android.h"
*/
import "C"

import (
	"image"
	"sync"
	"time"
	"unsafe"

	"gioui.org/app"
)

// Capture geometry: 640×480 luminance is ample for QR detection and keeps
// the per-frame copy and decode cheap. retryEvery throttles power-on
// attempts (e.g. while a permission dialog is up); idleRelease frees the
// device shortly after the scanner stops requesting frames.
const (
	camWidth     = 640
	camHeight    = 480
	retryEvery   = 700 * time.Millisecond
	idleRelease  = 2 * time.Second
	watchdogTick = 500 * time.Millisecond
)

// New returns the Android NDK camera bridge.
func New() Camera { return &androidCamera{} }

type androidCamera struct {
	opMu     sync.Mutex // serializes native start/stop and guards started
	started  bool
	nextTry  time.Time
	watchdog sync.Once

	lastUsed atomicTime // last Frame() call, read by the idle watchdog
	buf      []byte     // reusable luminance buffer, sized to the frame
}

func (c *androidCamera) Frame() image.Image {
	c.lastUsed.set(time.Now())
	c.ensureStarted()

	if len(c.buf) == 0 {
		c.buf = make([]byte, camWidth*camHeight) // reusable staging for the C copy
	}
	var w, h C.int
	switch r := C.vm_camera_copy((*C.uint8_t)(unsafe.Pointer(&c.buf[0])), C.int(len(c.buf)), &w, &h); {
	case r == 1:
		iw, ih := int(w), int(h)
		n := iw * ih
		if iw <= 0 || ih <= 0 || n > len(c.buf) {
			return nil
		}
		// Hand out a FRESH copy each frame. The scanner draws the frame as a
		// live GPU preview (Gio uploads it asynchronously at frame submission)
		// and may hold it briefly, so it must not alias the staging buffer that
		// the next Frame() call overwrites — otherwise the preview tears or
		// freezes on a cached texture.
		pix := make([]byte, n)
		copy(pix, c.buf[:n])
		return &image.Gray{Pix: pix, Stride: iw, Rect: image.Rect(0, 0, iw, ih)}
	case r < 0:
		// Buffer too small: grow to the reported size; a frame arrives next tick.
		c.buf = make([]byte, int(-r))
	}
	return nil
}

func (c *androidCamera) Stop() {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	if c.started {
		C.vm_camera_stop()
		c.started = false
	}
}

// ensureStarted powers the camera on (off the UI thread) the first time it
// is needed and after any idle release, throttled by retryEvery so a
// pending permission prompt does not cause a tight open/close loop.
func (c *androidCamera) ensureStarted() {
	c.opMu.Lock()
	if c.started || time.Now().Before(c.nextTry) {
		c.opMu.Unlock()
		return
	}
	c.nextTry = time.Now().Add(retryEvery)
	c.opMu.Unlock()

	go func() {
		jvm := app.JavaVM()
		ctx := app.AppContext()
		if jvm != 0 && ctx != 0 {
			// If not yet granted this attempts the request and returns 0;
			// we skip starting until a later attempt sees it granted.
			if C.vm_camera_permission((*C.JavaVM)(unsafe.Pointer(jvm)), C.jobject(unsafe.Pointer(ctx))) != 1 {
				return
			}
		}
		c.opMu.Lock()
		defer c.opMu.Unlock()
		if c.started {
			return
		}
		if rc := C.vm_camera_start(camWidth, camHeight); rc != 0 {
			C.vm_camera_stop() // tear down any partial state; retry later
			return
		}
		c.started = true
		c.watchdog.Do(func() { go c.idleWatch() })
	}()
}

// idleWatch releases the camera when the scanner has stopped requesting
// frames (i.e. the user left the scan screen), so the device is not held
// open needlessly. A subsequent Frame() call powers it back on.
func (c *androidCamera) idleWatch() {
	t := time.NewTicker(watchdogTick)
	defer t.Stop()
	for range t.C {
		c.opMu.Lock()
		idle := c.started && time.Since(c.lastUsed.get()) > idleRelease
		if idle {
			C.vm_camera_stop()
			c.started = false
		}
		c.opMu.Unlock()
	}
}

// atomicTime is a tiny mutex-guarded time holder shared by Frame() (writer)
// and the idle watchdog (reader).
type atomicTime struct {
	mu sync.Mutex
	t  time.Time
}

func (a *atomicTime) set(t time.Time) { a.mu.Lock(); a.t = t; a.mu.Unlock() }
func (a *atomicTime) get() time.Time  { a.mu.Lock(); defer a.mu.Unlock(); return a.t }
