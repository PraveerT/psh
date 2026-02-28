package com.phonessh.app.commands

import android.Manifest
import android.content.ContentValues
import android.content.Context
import android.content.pm.PackageManager
import android.net.Uri
import android.telephony.SmsManager
import androidx.core.app.ActivityCompat
import com.phonessh.app.protocol.CmdMsg
import com.phonessh.app.protocol.resultErr
import com.phonessh.app.protocol.resultOk

class SmsCommands(private val context: Context) {

    /**
     * psh sms list [--unread] [--from <number>] [--limit <n>]
     * psh sms send <number> <message>
     * psh sms conversations
     */
    fun dispatch(cmd: CmdMsg): String {
        val subCmd = cmd.args.firstOrNull() ?: return resultErr(cmd.id, "usage: sms [list|send|conversations]")
        return when (subCmd) {
            "list"          -> list(cmd)
            "send"          -> send(cmd)
            "conversations" -> conversations(cmd)
            else            -> resultErr(cmd.id, "unknown sms subcommand: $subCmd")
        }
    }

    private fun list(cmd: CmdMsg): String {
        if (!hasReadSmsPermission()) return resultErr(cmd.id, "READ_SMS permission not granted")

        val unreadOnly = cmd.flags.containsKey("unread")
        val fromFilter = cmd.flags["from"]
        val limit = cmd.flags["limit"]?.toIntOrNull() ?: 30

        val selection = buildString {
            if (unreadOnly) append("read = 0")
            if (fromFilter != null) {
                if (isNotEmpty()) append(" AND ")
                append("address LIKE '%${fromFilter.replace("'", "")}%'")
            }
        }.takeIf { it.isNotEmpty() }

        val messages = mutableListOf<Map<String, Any?>>()
        val cursor = context.contentResolver.query(
            Uri.parse("content://sms"),
            arrayOf("_id", "address", "body", "date", "read", "type"),
            selection,
            null,
            "date DESC LIMIT $limit"
        )

        cursor?.use { c ->
            while (c.moveToNext()) {
                messages.add(mapOf(
                    "id"      to c.getLong(c.getColumnIndexOrThrow("_id")),
                    "from"    to (c.getString(c.getColumnIndexOrThrow("address")) ?: ""),
                    "body"    to (c.getString(c.getColumnIndexOrThrow("body")) ?: ""),
                    "time"    to c.getLong(c.getColumnIndexOrThrow("date")),
                    "read"    to (c.getInt(c.getColumnIndexOrThrow("read")) == 1),
                    "type"    to when (c.getInt(c.getColumnIndexOrThrow("type"))) {
                        1 -> "inbox"
                        2 -> "sent"
                        3 -> "draft"
                        else -> "other"
                    }
                ))
            }
        }

        return resultOk(cmd.id, mapOf("count" to messages.size, "messages" to messages))
    }

    private fun send(cmd: CmdMsg): String {
        if (!hasSendSmsPermission()) return resultErr(cmd.id, "SEND_SMS permission not granted")

        val number = cmd.args.getOrNull(1) ?: return resultErr(cmd.id, "usage: sms send <number> <message>")
        val message = cmd.args.drop(2).joinToString(" ")
            .ifEmpty { return resultErr(cmd.id, "message cannot be empty") }

        return try {
            val smsManager = context.getSystemService(SmsManager::class.java)
            val parts = smsManager.divideMessage(message)
            smsManager.sendMultipartTextMessage(number, null, parts, null, null)

            // Save to sent box
            val values = ContentValues().apply {
                put("address", number)
                put("body", message)
                put("type", 2) // sent
                put("date", System.currentTimeMillis())
                put("read", 1)
            }
            context.contentResolver.insert(Uri.parse("content://sms/sent"), values)

            resultOk(cmd.id, mapOf(
                "sent" to true,
                "to" to number,
                "parts" to parts.size,
                "message" to message
            ))
        } catch (e: Exception) {
            resultErr(cmd.id, "send failed: ${e.message}")
        }
    }

    private fun conversations(cmd: CmdMsg): String {
        if (!hasReadSmsPermission()) return resultErr(cmd.id, "READ_SMS permission not granted")

        val convos = mutableListOf<Map<String, Any?>>()
        val cursor = context.contentResolver.query(
            Uri.parse("content://sms/conversations"),
            arrayOf("thread_id", "snippet", "date", "msg_count"),
            null, null,
            "date DESC LIMIT 20"
        )

        cursor?.use { c ->
            while (c.moveToNext()) {
                convos.add(mapOf(
                    "thread_id"  to c.getLong(c.getColumnIndexOrThrow("thread_id")),
                    "snippet"    to (c.getString(c.getColumnIndexOrThrow("snippet")) ?: ""),
                    "date"       to c.getLong(c.getColumnIndexOrThrow("date")),
                    "msg_count"  to c.getInt(c.getColumnIndexOrThrow("msg_count"))
                ))
            }
        }

        return resultOk(cmd.id, mapOf("conversations" to convos))
    }

    private fun hasReadSmsPermission() =
        ActivityCompat.checkSelfPermission(context, Manifest.permission.READ_SMS) ==
                PackageManager.PERMISSION_GRANTED

    private fun hasSendSmsPermission() =
        ActivityCompat.checkSelfPermission(context, Manifest.permission.SEND_SMS) ==
                PackageManager.PERMISSION_GRANTED
}
