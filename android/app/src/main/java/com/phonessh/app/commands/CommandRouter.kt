package com.phonessh.app.commands

import android.content.Context
import com.phonessh.app.protocol.CmdMsg
import com.phonessh.app.protocol.resultErr

class CommandRouter(private val context: Context) {

    private val files = FileCommands(context)
    private val system = SystemCommands(context)
    private val notifs = NotifCommands(context)
    private val sms = SmsCommands(context)
    private val apps = AppCommands(context)
    private val ui = UiCommands(context)

    fun dispatch(cmd: CmdMsg): String = when (cmd.cmd) {
        // ── File system ──────────────────────────────────────────────────────────
        "ls"         -> files.ls(cmd)
        "find"       -> files.find(cmd)
        "pull"       -> files.pull(cmd)
        "push"       -> files.push(cmd)
        "rm"         -> files.rm(cmd)
        "mkdir"      -> files.mkdir(cmd)
        "stat"       -> files.stat(cmd)

        // ── System ───────────────────────────────────────────────────────────────
        "status"     -> system.status(cmd)
        "battery"    -> system.battery(cmd)
        "location"   -> system.location(cmd)
        "screenshot" -> system.screenshot(cmd)
        "volume"     -> system.volume(cmd)
        "brightness" -> system.brightness(cmd)
        "dnd"        -> system.dnd(cmd)
        "wifi"       -> system.wifi(cmd)
        "clipboard"  -> system.clipboard(cmd)
        "lock"       -> system.lock(cmd)

        // ── Notifications ────────────────────────────────────────────────────────
        "notifs"     -> notifs.list(cmd)

        // ── SMS ──────────────────────────────────────────────────────────────────
        "sms"        -> sms.dispatch(cmd)

        // ── Apps ─────────────────────────────────────────────────────────────────
        "apps"       -> apps.dispatch(cmd)

        // ── UI / Screen interaction ───────────────────────────────────────────
        "open"       -> ui.open(cmd)
        "tap"        -> ui.tap(cmd)
        "swipe"      -> ui.swipe(cmd)
        "type"       -> ui.type(cmd)
        "key"        -> ui.key(cmd)
        "click"      -> ui.click(cmd)
        "ui"         -> ui.ui(cmd)

        else         -> resultErr(cmd.id, "unknown command: ${cmd.cmd}")
    }
}
