package org.vayu.mail;

import android.app.Activity;
import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.content.Context;
import android.content.ContextWrapper;
import android.content.Intent;
import android.os.Build;
import android.view.View;

import java.security.SecureRandom;
import java.util.HashMap;

// VayuNotify posts a new-mail notification that opens a specific mailbox when
// tapped. The mailbox (account + folder ids) rides as intent extras on the
// content PendingIntent; on tap the app's launcher activity is re-delivered the
// intent, and consumeTap() reads and clears those extras so the Go side can open
// that mailbox exactly once.
//
// Compiled to VayuNotify.jar (see the //go:generate lines in notify_android.go);
// gogio bundles the jar into the APK. Every method swallows its own errors so a
// notification can never crash the app.
public final class VayuNotify {
    private static final String CHANNEL_ID = "vayumail_new_mail";
    private static final String EX_ACCOUNT = "vayu_account";
    private static final String EX_FOLDER = "vayu_folder";
    private static final String EX_ID = "vayu_id";
    private static final String EX_NONCE = "vayu_nonce";

    // The launcher activity is EXPORTED, so any co-installed app can
    // startActivity() with fabricated vayu_account/vayu_folder extras to force
    // our own client to jump to a chosen mailbox (audit L13). We defend by
    // minting a per-notification nonce here (in-process, never leaves the app)
    // and requiring consumeTap to match it: the real PendingIntent — immutable
    // and built by us — carries the right nonce, a forged intent cannot. Keyed
    // by notification id so several outstanding notifications each validate.
    private static final SecureRandom RNG = new SecureRandom();
    private static final HashMap<Integer, Long> NONCES = new HashMap<>();

    private VayuNotify() {
    }

    private static void ensureChannel(Context ctx) {
        if (Build.VERSION.SDK_INT < 26) {
            return;
        }
        NotificationManager nm =
                (NotificationManager) ctx.getSystemService(Context.NOTIFICATION_SERVICE);
        if (nm == null || nm.getNotificationChannel(CHANNEL_ID) != null) {
            return;
        }
        NotificationChannel ch = new NotificationChannel(
                CHANNEL_ID, "New mail", NotificationManager.IMPORTANCE_HIGH);
        ch.setDescription("New message notifications");
        nm.createNotificationChannel(ch);
    }

    public static void post(Context ctx, int id, String title, String body,
            long account, long folder) {
        try {
            ensureChannel(ctx);
            NotificationManager nm =
                    (NotificationManager) ctx.getSystemService(Context.NOTIFICATION_SERVICE);
            if (nm == null) {
                return;
            }

            Intent launch = ctx.getPackageManager()
                    .getLaunchIntentForPackage(ctx.getPackageName());
            if (launch == null) {
                launch = new Intent();
            }
            launch.setFlags(Intent.FLAG_ACTIVITY_SINGLE_TOP
                    | Intent.FLAG_ACTIVITY_CLEAR_TOP
                    | Intent.FLAG_ACTIVITY_NEW_TASK);
            long nonce = RNG.nextLong();
            synchronized (NONCES) {
                NONCES.put(id, nonce);
            }
            launch.putExtra(EX_ACCOUNT, account);
            launch.putExtra(EX_FOLDER, folder);
            launch.putExtra(EX_ID, id);
            launch.putExtra(EX_NONCE, nonce);

            int piFlags = PendingIntent.FLAG_UPDATE_CURRENT;
            if (Build.VERSION.SDK_INT >= 23) {
                piFlags |= PendingIntent.FLAG_IMMUTABLE;
            }
            PendingIntent pi = PendingIntent.getActivity(ctx, id, launch, piFlags);

            Notification.Builder b;
            if (Build.VERSION.SDK_INT >= 26) {
                b = new Notification.Builder(ctx, CHANNEL_ID);
            } else {
                b = new Notification.Builder(ctx);
                b.setPriority(Notification.PRIORITY_HIGH);
            }
            b.setContentTitle(title != null ? title : "New mail");
            if (body != null && !body.isEmpty()) {
                b.setContentText(body);
            }
            b.setSmallIcon(ctx.getApplicationInfo().icon);
            b.setAutoCancel(true);
            b.setContentIntent(pi);

            nm.notify(id, b.build());
        } catch (Throwable t) {
            // Never crash the app over a notification.
        }
    }

    // consumeTap returns "account,folder" from the current activity's intent
    // extras (set by a tapped notification), clearing them so the target is used
    // once. It returns "" — refusing to route — unless the intent carries the
    // one-time nonce that a real post() minted for this notification id (audit
    // L13), so a fabricated intent from a co-installed app cannot steer the
    // client. It also ignores an activity relaunched from the recents/history
    // list, where stale extras could otherwise re-fire.
    public static String consumeTap(View view) {
        try {
            Context c = view.getContext();
            while (c instanceof ContextWrapper && !(c instanceof Activity)) {
                c = ((ContextWrapper) c).getBaseContext();
            }
            if (!(c instanceof Activity)) {
                return "";
            }
            Activity a = (Activity) c;
            Intent it = a.getIntent();
            if (it == null || !it.hasExtra(EX_ACCOUNT)) {
                return "";
            }
            // A relaunch from the history/recents list carries whatever extras
            // the last launch had — not a fresh tap. Never act on it.
            boolean fromHistory =
                    (it.getFlags() & Intent.FLAG_ACTIVITY_LAUNCHED_FROM_HISTORY) != 0;

            long account = it.getLongExtra(EX_ACCOUNT, 0);
            long folder = it.getLongExtra(EX_FOLDER, 0);
            int id = it.getIntExtra(EX_ID, -1);
            long nonce = it.getLongExtra(EX_NONCE, 0);

            // Always clear the extras so a target — genuine or forged — is
            // considered at most once and never replays.
            it.removeExtra(EX_ACCOUNT);
            it.removeExtra(EX_FOLDER);
            it.removeExtra(EX_ID);
            it.removeExtra(EX_NONCE);
            a.setIntent(it);

            boolean authentic;
            synchronized (NONCES) {
                Long expected = NONCES.remove(id);
                authentic = expected != null && expected.longValue() == nonce;
            }
            if (fromHistory || !authentic) {
                return "";
            }
            return account + "," + folder;
        } catch (Throwable t) {
            return "";
        }
    }
}
