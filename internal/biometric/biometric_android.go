//go:build android

package biometric

// Android biometric unlock via the framework BiometricPrompt (API 29+),
// reached over JNI. There is no AndroidX dependency (gogio does no Gradle):
// the helper class VayuBiometric.java is compiled to VayuBiometric.jar next
// to this file, and gogio bundles every *.jar it finds in an imported
// package's directory into the APK.
//
// The Java helper is deliberately synchronous — authenticate() posts the
// prompt to the UI thread and blocks on a latch that the prompt callbacks
// release, returning the outcome — so this side is a plain blocking JNI
// call with no native-callback registration. Callers run it on a background
// goroutine (see AppState.UnlockWithBiometric), so blocking here never
// touches the frame loop.
//
//go:generate javac --release 8 -classpath $ANDROID_HOME/platforms/android-34/android.jar -d /tmp/vayubio VayuBiometric.java
//go:generate jar cf VayuBiometric.jar -C /tmp/vayubio .

/*
#cgo LDFLAGS: -landroid
#include <jni.h>
*/
import "C"

import (
	"sync"

	"gioui.org/app"
	"gioui.org/io/event"
	"git.wow.st/gmp/jni"
)

// className is the JVM path of the helper compiled into VayuBiometric.jar.
const className = "org/vayu/mail/VayuBiometric"

var (
	mu   sync.Mutex
	view uintptr // the current Android View; BiometricPrompt needs its Activity
)

func handleViewEvent(e event.Event) {
	if ve, ok := e.(app.AndroidViewEvent); ok {
		mu.Lock()
		view = ve.View
		mu.Unlock()
	}
}

// loadClass resolves the helper class through the app's class loader, the
// same way gioui.org/x/explorer loads its bundled class.
func loadClass(env jni.Env) (jni.Class, error) {
	loader := jni.ClassLoaderFor(env, jni.Object(app.AppContext()))
	return jni.LoadClass(env, loader, className)
}

func available() bool {
	var ok bool
	err := jni.Do(jni.JVMFor(app.JavaVM()), func(env jni.Env) error {
		cls, err := loadClass(env)
		if err != nil {
			return err
		}
		m := jni.GetStaticMethodID(env, cls, "canAuthenticate", "(Landroid/content/Context;)Z")
		ok, err = jni.CallStaticBooleanMethod(env, cls, m, jni.Value(app.AppContext()))
		return err
	})
	if err != nil {
		return false
	}
	return ok
}

func authenticate(title, subtitle string) bool {
	mu.Lock()
	v := view
	mu.Unlock()
	if v == 0 {
		return false
	}
	var res int
	err := jni.Do(jni.JVMFor(app.JavaVM()), func(env jni.Env) error {
		cls, err := loadClass(env)
		if err != nil {
			return err
		}
		m := jni.GetStaticMethodID(env, cls, "authenticate",
			"(Landroid/view/View;Ljava/lang/String;Ljava/lang/String;)I")
		res, err = jni.CallStaticIntMethod(env, cls, m,
			jni.Value(v),
			jni.Value(jni.JavaString(env, title)),
			jni.Value(jni.JavaString(env, subtitle)))
		return err
	})
	if err != nil {
		return false
	}
	return res == 1
}
