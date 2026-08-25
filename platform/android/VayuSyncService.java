package org.vayu.mail;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.Service;
import android.content.Context;
import android.content.Intent;
import android.content.pm.ServiceInfo;
import android.os.Build;
import android.os.IBinder;

/**
 * The foreground service that hosts the Go sync engine while the app is
 * backgrounded: a persistent "mail syncing" notification and a pinned
 * process, so the held IMAP IDLE sockets survive. The engine itself never
 * moves — start/stop over JNI (platform/android) simply pins and releases
 * this process around the goroutines that are already running.
 *
 * Started only while the app is foregrounded (engine startup), so the
 * background-start restrictions on newer Android versions do not apply.
 */
public class VayuSyncService extends Service {
    private static final String CHANNEL_ID = "vayumail_sync";
    private static final int NOTIFICATION_ID = 1;

    @Override
    public IBinder onBind(Intent intent) {
        return null;
    }

    @Override
    public int onStartCommand(Intent intent, int flags, int startId) {
        startAsForeground();
        // START_STICKY: if the system kills us under memory pressure it
        // recreates the service, and the Go runtime boots back into its
        // normal startup path.
        return START_STICKY;
    }

    private void startAsForeground() {
        NotificationManager nm =
                (NotificationManager) getSystemService(NOTIFICATION_SERVICE);
        if (Build.VERSION.SDK_INT >= 26) {
            NotificationChannel ch = new NotificationChannel(
                    CHANNEL_ID,
                    "Mail syncing",
                    NotificationManager.IMPORTANCE_LOW);
            ch.setDescription("Shown while VayuMail keeps your mail live");
            nm.createNotificationChannel(ch);
        }
        int icon = getApplicationInfo().icon;
        Notification.Builder b = new Notification.Builder(this)
                .setContentTitle("VayuMail")
                .setContentText("Keeping your mail live")
                .setSmallIcon(icon != 0 ? icon : android.R.drawable.stat_notify_sync)
                .setOngoing(true);
        if (Build.VERSION.SDK_INT >= 26) {
            b.setChannelId(CHANNEL_ID);
        }
        Notification n = b.build();
        if (Build.VERSION.SDK_INT >= 29) {
            startForeground(NOTIFICATION_ID, n,
                    ServiceInfo.FOREGROUND_SERVICE_TYPE_DATA_SYNC);
        } else {
            startForeground(NOTIFICATION_ID, n);
        }
    }

    /** Pins the process. Safe to call repeatedly; the OS dedupes. */
    public static void start(Context ctx) {
        Intent i = new Intent(ctx, VayuSyncService.class);
        if (Build.VERSION.SDK_INT >= 26) {
            ctx.startForegroundService(i);
        } else {
            ctx.startService(i);
        }
    }

    /** Releases the pin and removes the notification. */
    public static void stop(Context ctx) {
        ctx.stopService(new Intent(ctx, VayuSyncService.class));
    }
}
