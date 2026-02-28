package com.phonessh.app

import android.service.notification.NotificationListenerService
import android.service.notification.StatusBarNotification
import java.util.concurrent.CopyOnWriteArrayList

/**
 * Captures live notifications for the `psh notifs` command.
 * Must be enabled by the user in Settings > Apps > Special app access > Notification access.
 */
class PshNotificationListenerService : NotificationListenerService() {

    data class CapturedNotification(
        val key: String,
        val pkg: String,
        val title: String,
        val text: String,
        val postTime: Long,
        val ongoing: Boolean,
        val groupKey: String?
    )

    companion object {
        @Volatile
        var instance: PshNotificationListenerService? = null
            private set

        private const val MAX_STORED = 200
    }

    private val notifications = CopyOnWriteArrayList<CapturedNotification>()

    override fun onCreate() {
        super.onCreate()
        instance = this
    }

    override fun onDestroy() {
        instance = null
        super.onDestroy()
    }

    override fun onNotificationPosted(sbn: StatusBarNotification) {
        val extras = sbn.notification.extras
        val n = CapturedNotification(
            key      = sbn.key,
            pkg      = sbn.packageName,
            title    = extras.getCharSequence("android.title")?.toString() ?: "",
            text     = extras.getCharSequence("android.text")?.toString() ?: "",
            postTime = sbn.postTime,
            ongoing  = sbn.isOngoing,
            groupKey = sbn.groupKey
        )

        // Remove stale entry for the same key if it exists
        notifications.removeIf { it.key == n.key }
        notifications.add(0, n)

        // Cap size
        while (notifications.size > MAX_STORED) notifications.removeLastOrNull()
    }

    override fun onNotificationRemoved(sbn: StatusBarNotification) {
        notifications.removeIf { it.key == sbn.key }
    }

    fun getNotifications(): List<CapturedNotification> = notifications.toList()

    /**
     * Cancel notifications from a specific package (or null to cancel all).
     * Returns count of cancelled notifications.
     */
    fun clearNotifications(pkg: String?): Int {
        val toCancel = if (pkg != null) {
            notifications.filter { it.pkg.contains(pkg, ignoreCase = true) }
        } else {
            notifications.toList()
        }

        toCancel.forEach { n ->
            runCatching { cancelNotification(n.key) }
        }
        return toCancel.size
    }

    fun clearAllNotifications() {
        runCatching { cancelAllNotifications() }
        notifications.clear()
    }
}

private fun <T> CopyOnWriteArrayList<T>.removeLastOrNull(): T? {
    if (isEmpty()) return null
    return removeAt(size - 1)
}
