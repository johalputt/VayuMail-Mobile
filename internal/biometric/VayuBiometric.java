package org.vayu.mail;

// VayuBiometric — the Java side of VayuMail's fingerprint/face unlock.
//
// It wraps the framework android.hardware.biometrics.BiometricPrompt (API 29+)
// with a SYNCHRONOUS surface so the Go/JNI side is a plain blocking call with
// no native-callback registration: authenticate() posts the prompt to the UI
// thread and blocks the calling (background) thread on a latch that the prompt
// callbacks release, then returns the outcome. It never throws across the JNI
// boundary — any failure is caught and reported as "unavailable/failed" so the
// app always falls back to the PIN.
//
// No AndroidX: everything here is framework-only (in android.jar), so gogio can
// bundle the compiled jar without a Gradle/AAR step.

import android.app.Activity;
import android.content.Context;
import android.content.ContextWrapper;
import android.content.DialogInterface;
import android.hardware.biometrics.BiometricManager;
import android.hardware.biometrics.BiometricPrompt;
import android.os.Build;
import android.os.CancellationSignal;
import android.view.View;

import java.util.concurrent.CountDownLatch;
import java.util.concurrent.Executor;

public class VayuBiometric {

    // canAuthenticate reports whether the device can prompt for a biometric
    // right now: supported OS, hardware present, and a credential enrolled.
    public static boolean canAuthenticate(Context ctx) {
        try {
            if (Build.VERSION.SDK_INT < 29 || ctx == null) {
                return false;
            }
            BiometricManager bm = (BiometricManager) ctx.getSystemService(Context.BIOMETRIC_SERVICE);
            if (bm == null) {
                return false;
            }
            return bm.canAuthenticate() == BiometricManager.BIOMETRIC_SUCCESS;
        } catch (Throwable t) {
            return false;
        }
    }

    // authenticate shows the system biometric sheet and blocks until it
    // resolves. Returns 1 on success, 0 on cancel/failure, -1 when biometrics
    // are unsupported or the Activity could not be resolved.
    public static int authenticate(final View view, final String title, final String subtitle) {
        if (Build.VERSION.SDK_INT < 29) {
            return -1;
        }
        final Activity activity = activityOf(view);
        if (activity == null) {
            return -1;
        }
        final int[] result = new int[]{-1};
        final CountDownLatch latch = new CountDownLatch(1);

        activity.runOnUiThread(new Runnable() {
            @Override
            public void run() {
                try {
                    final Executor exec = activity.getMainExecutor();
                    // A negative button is mandatory for BiometricPrompt; it is
                    // the user's explicit "decline biometrics, use the PIN".
                    BiometricPrompt prompt = new BiometricPrompt.Builder(activity)
                            .setTitle(title != null ? title : "Unlock VayuMail")
                            .setSubtitle(subtitle != null ? subtitle : "")
                            .setNegativeButton("Use PIN", exec, new DialogInterface.OnClickListener() {
                                @Override
                                public void onClick(DialogInterface dialog, int which) {
                                    result[0] = 0;
                                    latch.countDown();
                                }
                            })
                            .build();

                    prompt.authenticate(new CancellationSignal(), exec,
                            new BiometricPrompt.AuthenticationCallback() {
                                @Override
                                public void onAuthenticationSucceeded(BiometricPrompt.AuthenticationResult r) {
                                    result[0] = 1;
                                    latch.countDown();
                                }

                                @Override
                                public void onAuthenticationError(int errorCode, CharSequence errString) {
                                    result[0] = 0;
                                    latch.countDown();
                                }
                                // onAuthenticationFailed (one non-matching finger)
                                // is intentionally not terminal: the OS lets the
                                // user try again until it errors or cancels.
                            });
                } catch (Throwable t) {
                    result[0] = -1;
                    latch.countDown();
                }
            }
        });

        try {
            latch.await();
        } catch (InterruptedException e) {
            Thread.currentThread().interrupt();
            return -1;
        }
        return result[0];
    }

    // activityOf unwraps a View's context chain to the hosting Activity, which
    // BiometricPrompt needs to attach its window.
    static Activity activityOf(View view) {
        if (view == null) {
            return null;
        }
        Context c = view.getContext();
        while (c instanceof ContextWrapper) {
            if (c instanceof Activity) {
                return (Activity) c;
            }
            c = ((ContextWrapper) c).getBaseContext();
        }
        return null;
    }
}
