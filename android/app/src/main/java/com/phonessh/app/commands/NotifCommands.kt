package com.phonessh.app.commands

import android.content.Context
import com.phonessh.app.PshNotificationListenerService
import com.phonessh.app.protocol.CmdMsg
import com.phonessh.app.protocol.resultErr
import com.phonessh.app.protocol.resultOk

class NotifCommands(private val context: Context) {

    /**
     * psh notifs                     — list recent notifications
     * psh notifs --clear <app>       — cancel notifications from an app
     * psh notifs --clear-all         — cancel all notifications
     */
    fun list(cmd: CmdMsg): String {
        val listener = PshNotificationListenerService.instance
            ?: return resultErr(cmd.id, "Notification Listener not enabled — grant in Settings > Apps > Special app access > Notification access > PhoneSSH")

        // Handle --clear
        if (cmd.flags.containsKey("clear")) {
            val pkg = cmd.flags["clear"]
            val cleared = listener.clearNotifications(pkg)
            return resultOk(cmd.id, mapOf("cleared" to cleared))
        }

        if (cmd.flags.containsKey("clear-all")) {
            listener.clearAllNotifications()
            return resultOk(cmd.id, mapOf("cleared" to "all"))
        }

        val filter = cmd.flags["app"] ?: cmd.flags["filter"]
        val limit = cmd.flags["limit"]?.toIntOrNull() ?: 50

        val notifs = listener.getNotifications()
            .let { list -> if (filter != null) list.filter { it.pkg.contains(filter, ignoreCase = true) } else list }
            .take(limit)
            .map { n ->
                mapOf(
                    "key" to n.key,
                    "app" to n.pkg,
                    "title" to n.title,
                    "text" to n.text,
                    "time" to n.postTime,
                    "ongoing" to n.ongoing,
                    "group" to n.groupKey
                )
            }

        return resultOk(cmd.id, mapOf(
            "count" to notifs.size,
            "notifications" to notifs
        ))
    }
}
