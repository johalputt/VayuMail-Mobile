package org.vayu.mail;

import android.content.Context;
import android.os.Build;
import android.security.keystore.KeyGenParameterSpec;
import android.security.keystore.KeyProperties;
import android.security.keystore.StrongBoxUnavailableException;
import android.util.Base64;

import java.security.KeyStore;

import javax.crypto.Cipher;
import javax.crypto.KeyGenerator;
import javax.crypto.SecretKey;
import javax.crypto.spec.GCMParameterSpec;

/**
 * Wraps the app's master key under an AES-GCM key that lives inside the
 * Android Keystore (StrongBox when the device offers it) and never leaves
 * it. Go calls wrap/unwrap over JNI from platform/android; the master key
 * bytes therefore exist at rest only as ciphertext in hardware.key, and
 * nowhere as a file this device cannot open.
 *
 * Synchronous like VayuBiometric: every call blocks until the keystore
 * answers. Callers run off the UI thread.
 */
public final class VayuKeystore {
    private static final String ALIAS = "vayumail-wrap";
    private static final String ANDROID_KEYSTORE = "AndroidKeyStore";
    private static final int GCM_TAG_BITS = 128;
    private static final int GCM_IV_LEN = 12;

    private VayuKeystore() {}

    /** Capability probe: true when this device can host the wrapping key. */
    public static boolean probe(Context ctx) {
        try {
            getOrCreate(ctx);
            return true;
        } catch (Throwable t) {
            return false;
        }
    }

    /** Seals plaintext, returning base64(iv || ciphertext). */
    public static String wrap(Context ctx, byte[] plain) throws Exception {
        Cipher c = Cipher.getInstance("AES/GCM/NoPadding");
        c.init(Cipher.ENCRYPT_MODE, getOrCreate(ctx));
        byte[] iv = c.getIV();
        byte[] ct = c.doFinal(plain);
        byte[] out = new byte[iv.length + ct.length];
        System.arraycopy(iv, 0, out, 0, iv.length);
        System.arraycopy(ct, 0, out, iv.length, ct.length);
        return Base64.encodeToString(out, Base64.NO_WRAP);
    }

    /** Opens a wrap() result. Throws rather than guessing on tamper. */
    public static byte[] unwrap(Context ctx, String sealed) throws Exception {
        byte[] blob = Base64.decode(sealed, Base64.NO_WRAP);
        if (blob.length <= GCM_IV_LEN) {
            throw new IllegalArgumentException("wrapped blob too short");
        }
        Cipher c = Cipher.getInstance("AES/GCM/NoPadding");
        c.init(Cipher.DECRYPT_MODE, getOrCreate(ctx),
                new GCMParameterSpec(GCM_TAG_BITS, blob, 0, GCM_IV_LEN));
        return c.doFinal(blob, GCM_IV_LEN, blob.length - GCM_IV_LEN);
    }

    private static synchronized SecretKey getOrCreate(Context ctx) throws Exception {
        KeyStore ks = KeyStore.getInstance(ANDROID_KEYSTORE);
        ks.load(null);
        SecretKey existing = (SecretKey) ks.getKey(ALIAS, null);
        if (existing != null) {
            return existing;
        }
        KeyGenerator kg = KeyGenerator.getInstance(
                KeyProperties.KEY_ALGORITHM_AES, ANDROID_KEYSTORE);
        KeyGenParameterSpec.Builder b = new KeyGenParameterSpec.Builder(
                ALIAS,
                KeyProperties.PURPOSE_ENCRYPT | KeyProperties.PURPOSE_DECRYPT)
                .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
                .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
                .setKeySize(256);
        // StrongBox is the separate secure element; not every device has one
        // and the API only exists since 28. Try it first, fall back to TEE.
        if (Build.VERSION.SDK_INT >= 28 && ctx != null
                && ctx.getPackageManager().hasSystemFeature("android.hardware.strongbox_keystore")) {
            b.setIsStrongBoxBacked(true);
            try {
                kg.init(b.build());
                return kg.generateKey();
            } catch (StrongBoxUnavailableException e) {
                // fall through to the plain spec below
                kg = KeyGenerator.getInstance(
                        KeyProperties.KEY_ALGORITHM_AES, ANDROID_KEYSTORE);
            }
        }
        kg.init(b.build());
        return kg.generateKey();
    }
}
