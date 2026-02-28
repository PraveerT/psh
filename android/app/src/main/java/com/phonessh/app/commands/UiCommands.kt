package com.phonessh.app.commands

import android.content.Context
import android.content.Intent
import android.net.Uri
import com.phonessh.app.PshAccessibilityService
import com.phonessh.app.protocol.CmdMsg
import com.phonessh.app.protocol.resultErr
import com.phonessh.app.protocol.resultOk

class UiCommands(private val context: Context) {

    /**
     * psh open <url-or-deep-link>
     * Opens any URL or deep link via Android Intent system.
     */
    fun open(cmd: CmdMsg): String {
        val url = cmd.args.firstOrNull()
            ?: return resultErr(cmd.id, "usage: open <url-or-deep-link>")

        return try {
            val intent = Intent(Intent.ACTION_VIEW, Uri.parse(url)).apply {
                addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
            }
            context.startActivity(intent)
            resultOk(cmd.id, mapOf("opened" to url))
        } catch (e: Exception) {
            resultErr(cmd.id, "open failed: ${e.message}")
        }
    }

    /**
     * psh tap <x> <y>
     * Taps the screen at the given coordinates.
     */
    fun tap(cmd: CmdMsg): String {
        val x = cmd.args.getOrNull(0)?.toFloatOrNull()
            ?: return resultErr(cmd.id, "usage: tap <x> <y>")
        val y = cmd.args.getOrNull(1)?.toFloatOrNull()
            ?: return resultErr(cmd.id, "usage: tap <x> <y>")

        return if (PshAccessibilityService.dispatchTap(x, y)) {
            resultOk(cmd.id, mapOf("tapped" to mapOf("x" to x, "y" to y)))
        } else {
            resultErr(cmd.id, "tap failed — is PhoneSSH Accessibility Service enabled?")
        }
    }

    /**
     * psh swipe <x1> <y1> <x2> <y2> [--duration <ms>]
     * Performs a swipe gesture.
     */
    fun swipe(cmd: CmdMsg): String {
        val x1 = cmd.args.getOrNull(0)?.toFloatOrNull()
            ?: return resultErr(cmd.id, "usage: swipe <x1> <y1> <x2> <y2> [--duration ms]")
        val y1 = cmd.args.getOrNull(1)?.toFloatOrNull()
            ?: return resultErr(cmd.id, "usage: swipe <x1> <y1> <x2> <y2> [--duration ms]")
        val x2 = cmd.args.getOrNull(2)?.toFloatOrNull()
            ?: return resultErr(cmd.id, "usage: swipe <x1> <y1> <x2> <y2> [--duration ms]")
        val y2 = cmd.args.getOrNull(3)?.toFloatOrNull()
            ?: return resultErr(cmd.id, "usage: swipe <x1> <y1> <x2> <y2> [--duration ms]")
        val durationMs = cmd.flags["duration"]?.toLongOrNull() ?: 300L

        return if (PshAccessibilityService.dispatchSwipe(x1, y1, x2, y2, durationMs)) {
            resultOk(cmd.id, mapOf(
                "swiped" to mapOf("from" to mapOf("x" to x1, "y" to y1), "to" to mapOf("x" to x2, "y" to y2)),
                "duration_ms" to durationMs
            ))
        } else {
            resultErr(cmd.id, "swipe failed — is PhoneSSH Accessibility Service enabled?")
        }
    }

    /**
     * psh type "<text>"
     * Sets text in the currently focused input field.
     */
    fun type(cmd: CmdMsg): String {
        val text = cmd.args.firstOrNull()
            ?: return resultErr(cmd.id, "usage: type \"<text>\"")

        return if (PshAccessibilityService.typeText(text)) {
            resultOk(cmd.id, mapOf("typed" to text))
        } else {
            resultErr(cmd.id, "type failed — focus an input field first, and ensure Accessibility Service is enabled")
        }
    }

    /**
     * psh key <back|home|recents|notifications>
     * Presses a navigation/hardware key.
     */
    fun key(cmd: CmdMsg): String {
        val action = cmd.args.firstOrNull()
            ?: return resultErr(cmd.id, "usage: key <back|home|recents|notifications>")

        return if (PshAccessibilityService.pressKey(action)) {
            resultOk(cmd.id, mapOf("key" to action))
        } else {
            resultErr(cmd.id, "key '$action' failed — valid keys: back, home, recents, notifications")
        }
    }
}
