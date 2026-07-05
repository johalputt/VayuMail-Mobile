// NDK Camera2 bridge — C interface called from camera_android.go via cgo.
// Compiled only for GOOS=android (filename suffix _android). See the
// package doc; this cannot be built or run outside a device.
#ifndef VAYUMAIL_CAMERA_ANDROID_H
#define VAYUMAIL_CAMERA_ANDROID_H

#include <stdint.h>
#include <jni.h>

// vm_camera_start opens the back camera and streams YUV_420_888 into an
// AImageReader at the requested size. Returns 0 on success, a negative
// code on failure (the partial state is torn down internally).
int vm_camera_start(int width, int height);

// vm_camera_stop releases every camera object. Safe to call repeatedly.
void vm_camera_stop(void);

// vm_camera_copy hands the latest luminance (Y) frame to Go. If a new
// frame exists and cap >= w*h it copies the plane into dst, sets *w/*h,
// clears the ready flag and returns 1. If the buffer is too small it sets
// *w/*h and returns the negative required size (caller resizes; the frame
// is kept). If no new frame is available it returns 0.
int vm_camera_copy(uint8_t *dst, int cap, int *w, int *h);

// vm_camera_permission checks (and, when possible, requests) the runtime
// CAMERA permission over JNI without any Java source. Returns 1 if
// granted, 0 if not (a request is attempted when the context is an
// Activity), or -1 if the JNI plumbing was unavailable.
int vm_camera_permission(JavaVM *jvm, jobject ctx);

#endif
