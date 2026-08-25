//go:build android

package anim

// scale_android.go reads the system animator-duration-scale so the
// reduce-motion gate follows the OS accessibility setting automatically
// (plan Phase 5.6 tail): a scale of 0 means the user asked the whole
// device to stop animating.
//
// No helper jar needed: android.provider.Settings$Global is public API,
// read straight through JNI reflection. We go through getString (a static
// method returning an object — this JNI binding has no static-float call)
// and parse the number in Go. Any failure falls back to 1.0, deferring to
// the in-app toggle rather than guessing motion off.

import (
	"strconv"

	"gioui.org/app"
	"git.wow.st/gmp/jni"
)

const animatorScaleSetting = "animator_duration_scale"

// SystemAnimatorScale reports Android's animator duration scale, or 1.0
// when it cannot be read. A return of 0 means the user disabled system
// animations.
func SystemAnimatorScale() float32 {
	var out float32 = 1.0
	err := jni.Do(jni.JVMFor(app.JavaVM()), func(env jni.Env) error {
		cls := jni.FindClass(env, "android/provider/Settings$Global")
		resolver, err := jni.CallObjectMethod(
			env, jni.Object(app.AppContext()),
			jni.GetMethodID(env, jni.GetObjectClass(env, jni.Object(app.AppContext())),
				"getContentResolver", "()Landroid/content/ContentResolver;"))
		if err != nil {
			return err
		}
		m := jni.GetStaticMethodID(env, cls, "getString",
			"(Landroid/content/ContentResolver;Ljava/lang/String;)Ljava/lang/String;")
		obj, err := jni.CallStaticObjectMethod(env, cls, m,
			jni.Value(resolver), jni.Value(jni.JavaString(env, animatorScaleSetting)))
		if err != nil {
			return err
		}
		s := jni.GoString(env, jni.String(uintptr(obj)))
		if s == "" {
			return nil // setting absent: keep 1.0
		}
		if f, perr := strconv.ParseFloat(s, 32); perr == nil {
			out = float32(f)
		}
		return nil
	})
	if err != nil {
		return 1.0
	}
	return out
}
