//go:build android

package pushnotify

// Android tappable new-mail notifications over JNI. Like internal/biometric there
// is no AndroidX/Gradle dependency: the helper class VayuNotify.java is compiled
// to VayuNotify.jar next to this file, and gogio bundles every *.jar it finds in
// an imported package's directory into the APK.
//
// VayuNotify.post builds a Notification whose content PendingIntent re-launches
// the app's activity with the mailbox (account + folder ids) as extras;
// VayuNotify.consumeTap reads and clears those extras from the current activity's
// intent on resume, and we hand the result to the registered tap handler so the
// app opens that mailbox.
//
// UNVERIFIED — not compiled/run here (no NDK; CI does not build the APK). See
// README.md for the jar build and the AndroidManifest POST_NOTIFICATIONS
// requirement.
//
//go:generate javac --release 8 -classpath $ANDROID_HOME/platforms/android-34/android.jar -d /tmp/vayunotify VayuNotify.java
//go:generate jar cf VayuNotify.jar -C /tmp/vayunotify .

/*
#cgo LDFLAGS: -landroid
#include <jni.h>
*/
import "C"

import (
	"strconv"
	"strings"
	"sync"

	"gioui.org/app"
	"gioui.org/io/event"
	"git.wow.st/gmp/jni"
)

// className is the JVM path of the helper compiled into VayuNotify.jar.
const className = "org/vayu/mail/VayuNotify"

var (
	mu  sync.Mutex
	tap func(accountID, folderID int64)
)

func available() bool { return true }

func setTapHandler(fn func(int64, int64)) {
	mu.Lock()
	tap = fn
	mu.Unlock()
}

// loadClass resolves the helper class through the app's class loader, the same
// way gioui.org/x/explorer and internal/biometric load their bundled classes.
func loadClass(env jni.Env) (jni.Class, error) {
	loader := jni.ClassLoaderFor(env, jni.Object(app.AppContext()))
	return jni.LoadClass(env, loader, className)
}

func post(id int, title, body string, accountID, folderID int64) bool {
	err := jni.Do(jni.JVMFor(app.JavaVM()), func(env jni.Env) error {
		cls, err := loadClass(env)
		if err != nil {
			return err
		}
		m := jni.GetStaticMethodID(env, cls, "post",
			"(Landroid/content/Context;ILjava/lang/String;Ljava/lang/String;JJ)V")
		return jni.CallStaticVoidMethod(env, cls, m,
			jni.Value(app.AppContext()),
			jni.Value(C.jint(id)),
			jni.Value(jni.JavaString(env, title)),
			jni.Value(jni.JavaString(env, body)),
			jni.Value(C.jlong(accountID)),
			jni.Value(C.jlong(folderID)))
	})
	return err == nil
}

// handleViewEvent captures the current view and, on the resume/new-intent that a
// notification tap produces, reads and routes any tapped-mailbox target.
func handleViewEvent(e event.Event) {
	ve, ok := e.(app.AndroidViewEvent)
	if !ok {
		return
	}
	mu.Lock()
	fn := tap
	mu.Unlock()
	if fn == nil || ve.View == 0 {
		return
	}
	if acct, folder, ok := consumeTap(ve.View); ok {
		fn(acct, folder)
	}
}

// consumeTap asks the Java helper for the tapped mailbox ("account,folder") from
// the activity's intent extras, clearing them so a target is used once.
func consumeTap(v uintptr) (accountID, folderID int64, ok bool) {
	var packed string
	err := jni.Do(jni.JVMFor(app.JavaVM()), func(env jni.Env) error {
		cls, e := loadClass(env)
		if e != nil {
			return e
		}
		m := jni.GetStaticMethodID(env, cls, "consumeTap", "(Landroid/view/View;)Ljava/lang/String;")
		obj, e := jni.CallStaticObjectMethod(env, cls, m, jni.Value(v))
		if e != nil {
			return e
		}
		packed = jni.GoString(env, jni.String(uintptr(obj)))
		return nil
	})
	if err != nil || packed == "" {
		return 0, 0, false
	}
	parts := strings.SplitN(packed, ",", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	a, e1 := strconv.ParseInt(parts[0], 10, 64)
	f, e2 := strconv.ParseInt(parts[1], 10, 64)
	if e1 != nil || e2 != nil {
		return 0, 0, false
	}
	return a, f, true
}
