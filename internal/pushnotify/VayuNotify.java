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
            launch.putExtra(EX_ACCOUNT, account);
            launch.putExtra(EX_FOLDER, folder);

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
    // once. Returns "" when there is nothing to open.
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
            long account = it.getLongExtra(EX_ACCOUNT, 0);
            long folder = it.getLongExtra(EX_FOLDER, 0);
            it.removeExtra(EX_ACCOUNT);
            it.removeExtra(EX_FOLDER);
            a.setIntent(it);
            return account + "," + folder;
        } catch (Throwable t) {
            return "";
        }
    }
}
