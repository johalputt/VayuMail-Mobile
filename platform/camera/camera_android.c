// NDK Camera2 bridge implementation. Compiled only for GOOS=android (the
// _android filename suffix is a build constraint for .c files too), so it
// never affects the desktop/CI build. See camera_android.h and the package
// doc. This file cannot be compiled or run outside the Android toolchain
// on a physical device.

#include "camera_android.h"

#include <stdlib.h>
#include <string.h>
#include <pthread.h>
#include <android/log.h>
#include <android/native_window.h>
#include <camera/NdkCameraManager.h>
#include <camera/NdkCameraDevice.h>
#include <camera/NdkCameraCaptureSession.h>
#include <camera/NdkCameraMetadata.h>
#include <camera/NdkCaptureRequest.h>
#include <media/NdkImageReader.h>

#define VM_TAG "vayumail-camera"
#define VM_LOGE(...) __android_log_print(ANDROID_LOG_ERROR, VM_TAG, __VA_ARGS__)

// ---- latest-frame buffer: Y/luminance plane, tightly packed w*h bytes ----
static pthread_mutex_t vm_mu = PTHREAD_MUTEX_INITIALIZER;
static uint8_t *vm_luma = NULL;
static int vm_luma_cap = 0;
static int vm_w = 0, vm_h = 0;
static int vm_ready = 0;

// ---- Camera2 objects (single camera at a time) ----
static ACameraManager *vm_mgr = NULL;
static ACameraDevice *vm_dev = NULL;
static AImageReader *vm_reader = NULL;
static ANativeWindow *vm_window = NULL;
static ACaptureSessionOutputContainer *vm_outputs = NULL;
static ACaptureSessionOutput *vm_output = NULL;
static ACameraCaptureSession *vm_session = NULL;
static ACaptureRequest *vm_request = NULL;
static ACameraOutputTarget *vm_target = NULL;
static int vm_running = 0;

// onImageAvailable runs on a camera thread: acquire the newest image, copy
// its luminance plane into vm_luma (respecting the plane's row stride), and
// flag a frame ready. Everything else about the image is discarded.
static void vm_on_image(void *ctx, AImageReader *reader) {
	(void)ctx;
	AImage *img = NULL;
	if (AImageReader_acquireLatestImage(reader, &img) != AMEDIA_OK || img == NULL) {
		return;
	}
	int32_t w = 0, h = 0, rowStride = 0, len = 0;
	uint8_t *y = NULL;
	AImage_getWidth(img, &w);
	AImage_getHeight(img, &h);
	AImage_getPlaneRowStride(img, 0, &rowStride);
	if (AImage_getPlaneData(img, 0, &y, &len) != AMEDIA_OK || y == NULL || w <= 0 || h <= 0) {
		AImage_delete(img);
		return;
	}
	pthread_mutex_lock(&vm_mu);
	int need = w * h;
	if (vm_luma_cap < need) {
		uint8_t *nb = (uint8_t *)realloc(vm_luma, need);
		if (nb == NULL) {
			pthread_mutex_unlock(&vm_mu);
			AImage_delete(img);
			return;
		}
		vm_luma = nb;
		vm_luma_cap = need;
	}
	for (int r = 0; r < h; r++) {
		memcpy(vm_luma + (size_t)r * w, y + (size_t)r * rowStride, w);
	}
	vm_w = w;
	vm_h = h;
	vm_ready = 1;
	pthread_mutex_unlock(&vm_mu);
	AImage_delete(img);
}

static void vm_dev_disconnected(void *ctx, ACameraDevice *dev) { (void)ctx; (void)dev; VM_LOGE("camera disconnected"); }
static void vm_dev_error(void *ctx, ACameraDevice *dev, int err) { (void)ctx; (void)dev; VM_LOGE("camera device error %d", err); }
static void vm_ses_ready(void *ctx, ACameraCaptureSession *s) { (void)ctx; (void)s; }
static void vm_ses_active(void *ctx, ACameraCaptureSession *s) { (void)ctx; (void)s; }
static void vm_ses_closed(void *ctx, ACameraCaptureSession *s) { (void)ctx; (void)s; }

// vm_pick_camera returns a malloc'd copy of the back-facing camera id, or
// the first camera if none reports LENS_FACING_BACK. Caller frees.
static char *vm_pick_camera(ACameraManager *mgr) {
	ACameraIdList *ids = NULL;
	if (ACameraManager_getCameraIdList(mgr, &ids) != ACAMERA_OK || ids == NULL) {
		return NULL;
	}
	char *chosen = NULL;
	char *first = NULL;
	for (int i = 0; i < ids->numCameras; i++) {
		const char *id = ids->cameraIds[i];
		if (first == NULL) {
			first = strdup(id);
		}
		ACameraMetadata *meta = NULL;
		if (ACameraManager_getCameraCharacteristics(mgr, id, &meta) != ACAMERA_OK) {
			continue;
		}
		ACameraMetadata_const_entry e;
		if (ACameraMetadata_getConstEntry(meta, ACAMERA_LENS_FACING, &e) == ACAMERA_OK &&
			e.count > 0 && e.data.u8[0] == ACAMERA_LENS_FACING_BACK) {
			chosen = strdup(id);
		}
		ACameraMetadata_free(meta);
		if (chosen != NULL) {
			break;
		}
	}
	if (chosen == NULL) {
		chosen = first;
	} else if (first != NULL) {
		free(first);
	}
	ACameraManager_deleteCameraIdList(ids);
	return chosen;
}

int vm_camera_start(int width, int height) {
	if (vm_running) {
		return 0;
	}
	vm_mgr = ACameraManager_create();
	if (vm_mgr == NULL) {
		return -1;
	}
	char *id = vm_pick_camera(vm_mgr);
	if (id == NULL) {
		return -2;
	}
	ACameraDevice_StateCallbacks devCbs;
	memset(&devCbs, 0, sizeof(devCbs));
	devCbs.onDisconnected = vm_dev_disconnected;
	devCbs.onError = vm_dev_error;
	camera_status_t st = ACameraManager_openCamera(vm_mgr, id, &devCbs, &vm_dev);
	free(id);
	if (st != ACAMERA_OK || vm_dev == NULL) {
		return -3;
	}
	if (AImageReader_new(width, height, AIMAGE_FORMAT_YUV_420_888, 2, &vm_reader) != AMEDIA_OK) {
		return -4;
	}
	AImageReader_ImageListener listener;
	listener.context = NULL;
	listener.onImageAvailable = vm_on_image;
	AImageReader_setImageListener(vm_reader, &listener);
	if (AImageReader_getWindow(vm_reader, &vm_window) != AMEDIA_OK || vm_window == NULL) {
		return -5;
	}
	ANativeWindow_acquire(vm_window);
	ACaptureSessionOutputContainer_create(&vm_outputs);
	ACaptureSessionOutput_create(vm_window, &vm_output);
	ACaptureSessionOutputContainer_add(vm_outputs, vm_output);
	ACameraCaptureSession_StateCallbacks sesCbs;
	memset(&sesCbs, 0, sizeof(sesCbs));
	sesCbs.onReady = vm_ses_ready;
	sesCbs.onActive = vm_ses_active;
	sesCbs.onClosed = vm_ses_closed;
	if (ACameraDevice_createCaptureSession(vm_dev, vm_outputs, &sesCbs, &vm_session) != ACAMERA_OK) {
		return -6;
	}
	if (ACameraDevice_createCaptureRequest(vm_dev, TEMPLATE_PREVIEW, &vm_request) != ACAMERA_OK) {
		return -7;
	}
	ACameraOutputTarget_create(vm_window, &vm_target);
	ACaptureRequest_addTarget(vm_request, vm_target);
	if (ACameraCaptureSession_setRepeatingRequest(vm_session, NULL, 1, &vm_request, NULL) != ACAMERA_OK) {
		return -8;
	}
	vm_running = 1;
	return 0;
}

void vm_camera_stop(void) {
	if (vm_session != NULL) {
		ACameraCaptureSession_stopRepeating(vm_session);
		ACameraCaptureSession_close(vm_session);
		vm_session = NULL;
	}
	if (vm_request != NULL) {
		if (vm_target != NULL) {
			ACaptureRequest_removeTarget(vm_request, vm_target);
		}
		ACaptureRequest_free(vm_request);
		vm_request = NULL;
	}
	if (vm_target != NULL) {
		ACameraOutputTarget_free(vm_target);
		vm_target = NULL;
	}
	if (vm_outputs != NULL) {
		if (vm_output != NULL) {
			ACaptureSessionOutputContainer_remove(vm_outputs, vm_output);
		}
		ACaptureSessionOutputContainer_free(vm_outputs);
		vm_outputs = NULL;
	}
	if (vm_output != NULL) {
		ACaptureSessionOutput_free(vm_output);
		vm_output = NULL;
	}
	if (vm_window != NULL) {
		ANativeWindow_release(vm_window);
		vm_window = NULL;
	}
	if (vm_reader != NULL) {
		AImageReader_delete(vm_reader);
		vm_reader = NULL;
	}
	if (vm_dev != NULL) {
		ACameraDevice_close(vm_dev);
		vm_dev = NULL;
	}
	if (vm_mgr != NULL) {
		ACameraManager_delete(vm_mgr);
		vm_mgr = NULL;
	}
	vm_running = 0;
	pthread_mutex_lock(&vm_mu);
	vm_ready = 0;
	pthread_mutex_unlock(&vm_mu);
}

int vm_camera_copy(uint8_t *dst, int cap, int *w, int *h) {
	int rc = 0;
	pthread_mutex_lock(&vm_mu);
	if (vm_ready && vm_w > 0 && vm_h > 0) {
		int need = vm_w * vm_h;
		*w = vm_w;
		*h = vm_h;
		if (dst != NULL && cap >= need) {
			memcpy(dst, vm_luma, need);
			vm_ready = 0;
			rc = 1;
		} else {
			rc = -need;
		}
	}
	pthread_mutex_unlock(&vm_mu);
	return rc;
}

int vm_camera_permission(JavaVM *jvm, jobject ctx) {
	if (jvm == NULL || ctx == NULL) {
		return -1;
	}
	JNIEnv *env = NULL;
	int didAttach = 0;
	jint res = (*jvm)->GetEnv(jvm, (void **)&env, JNI_VERSION_1_6);
	if (res == JNI_EDETACHED) {
		if ((*jvm)->AttachCurrentThread(jvm, &env, NULL) != JNI_OK || env == NULL) {
			return -1;
		}
		didAttach = 1;
	} else if (res != JNI_OK || env == NULL) {
		return -1;
	}
	int result = -1;
	jclass ctxCls = (*env)->GetObjectClass(env, ctx);
	jmethodID checkPerm = (*env)->GetMethodID(env, ctxCls, "checkSelfPermission", "(Ljava/lang/String;)I");
	if (checkPerm != NULL) {
		jstring perm = (*env)->NewStringUTF(env, "android.permission.CAMERA");
		jint granted = (*env)->CallIntMethod(env, ctx, checkPerm, perm); // PERMISSION_GRANTED == 0
		if (granted == 0) {
			result = 1;
		} else {
			result = 0;
			jclass actCls = (*env)->FindClass(env, "android/app/Activity");
			if (actCls != NULL && (*env)->IsInstanceOf(env, ctx, actCls)) {
				jmethodID reqPerm = (*env)->GetMethodID(env, actCls, "requestPermissions", "([Ljava/lang/String;I)V");
				jclass strCls = (*env)->FindClass(env, "java/lang/String");
				if (reqPerm != NULL && strCls != NULL) {
					jobjectArray arr = (*env)->NewObjectArray(env, 1, strCls, perm);
					if (arr != NULL) {
						(*env)->CallVoidMethod(env, ctx, reqPerm, arr, 0x7A11);
						(*env)->DeleteLocalRef(env, arr);
					}
				}
				if (strCls != NULL) {
					(*env)->DeleteLocalRef(env, strCls);
				}
			}
			if (actCls != NULL) {
				(*env)->DeleteLocalRef(env, actCls);
			}
		}
		(*env)->DeleteLocalRef(env, perm);
	}
	if (ctxCls != NULL) {
		(*env)->DeleteLocalRef(env, ctxCls);
	}
	if ((*env)->ExceptionCheck(env)) {
		(*env)->ExceptionClear(env);
	}
	if (didAttach) {
		(*jvm)->DetachCurrentThread(jvm);
	}
	return result;
}
