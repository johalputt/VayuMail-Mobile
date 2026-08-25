//go:build android

package android

/*
#cgo LDFLAGS: -landroid
#include <jni.h>
*/
import "C"

import (
	"fmt"

	"gioui.org/app"
	"git.wow.st/gmp/jni"

	appcrypto "github.com/johalputt/VayuMail-Mobile/internal/crypto"
)

// The helper class compiled into VayuPlatform.jar by release.yml (gogio
// bundles every jar next to an imported package's files into the APK).
// Resolved through the app class loader exactly like internal/biometric,
// because gogio apps load their own classes, not the system's.
const keystoreClass = "org/vayu/mail/VayuKeystore"

func loadClassNamed(env jni.Env, name string) (jni.Class, error) {
	loader := jni.ClassLoaderFor(env, jni.Object(app.AppContext()))
	return jni.LoadClass(env, loader, name)
}

func withClass(name string, fn func(env jni.Env, cls jni.Class) error) error {
	return jni.Do(jni.JVMFor(app.JavaVM()), func(env jni.Env) error {
		cls, err := loadClassNamed(env, name)
		if err != nil {
			return err
		}
		return fn(env, cls)
	})
}

// wrapOnDevice seals plaintext under the AndroidKeyStore AES-GCM key via
// org.vayu.mail.VayuKeystore.wrap(Context, byte[]) → base64(iv||ct).
func wrapOnDevice(plain []byte) (string, error) {
	var out string
	err := withClass(keystoreClass, func(env jni.Env, cls jni.Class) error {
		m, err := jni.GetStaticMethodID(env, cls, "wrap",
			"(Landroid/content/Context;[B)Ljava/lang/String;")
		if err != nil {
			return err
		}
		arr := jni.NewByteArray(env, plain)
		defer jni.DeleteLocalRef(env, jni.Object(arr))
		obj, err := jni.CallStaticObjectMethod(env, cls, m,
			jni.Value(app.AppContext()),
			jni.Value(arr))
		if err != nil {
			return err
		}
		out = jni.GoString(env, jni.String(uintptr(obj)))
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("wrap: %w", err)
	}
	return out, nil
}

// unwrapOnDevice opens a wrapOnDevice result via
// org.vayu.mail.VayuKeystore.unwrap(Context, String) → byte[].
func unwrapOnDevice(sealed string) ([]byte, error) {
	var out []byte
	err := withClass(keystoreClass, func(env jni.Env, cls jni.Class) error {
		m, err := jni.GetStaticMethodID(env, cls, "unwrap",
			"(Landroid/content/Context;Ljava/lang/String;)[B")
		if err != nil {
			return err
		}
		obj, err := jni.CallStaticObjectMethod(env, cls, m,
			jni.Value(app.AppContext()),
			jni.Value(jni.JavaString(env, sealed)))
		if err != nil {
			return err
		}
		elems := jni.GetByteArrayElements(env, jni.ByteArray(uintptr(obj)))
		out = append([]byte(nil), elems...)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("unwrap: %w", err)
	}
	return out, nil
}

// HardwareKeystore returns a KeyProvider whose master key exists at rest
// only as ciphertext under an AndroidKeyStore key (StrongBox attempted
// first), or nil when the device cannot host one — in which case main()
// falls back to exactly the sealed-file provider it used before. A probe
// failure here is a capability answer, not an error to report.
func HardwareKeystore(dataDir string) appcrypto.KeyProvider {
	ok := false
	err := withClass(keystoreClass, func(env jni.Env, cls jni.Class) error {
		m, err := jni.GetStaticMethodID(env, cls, "probe",
			"(Landroid/content/Context;)Z")
		if err != nil {
			return err
		}
		v, err := jni.CallStaticBooleanMethod(env, cls, m,
			jni.Value(app.AppContext()))
		ok = bool(v)
		return err
	})
	if err != nil || !ok {
		return nil
	}
	return NewWrappedKeyProvider(dataDir, wrapOnDevice, unwrapOnDevice)
}

var _ appcrypto.KeyProvider = (*WrappedKeyProvider)(nil)
