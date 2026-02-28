package com.phonessh.app.commands

import android.Manifest
import android.app.NotificationManager
import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.content.pm.PackageManager
import android.location.Location
import android.location.LocationManager
import android.media.AudioManager
import android.net.wifi.WifiManager
import android.os.BatteryManager
import android.os.Build
import android.os.Environment
import android.os.StatFs
import android.provider.Settings
import androidx.core.app.ActivityCompat
import com.phonessh.app.protocol.CmdMsg
import com.phonessh.app.protocol.resultErr
import com.phonessh.app.protocol.resultOk
import java.io.File

class SystemCommands(private val context: Context) {

    /** psh status — battery, wifi, storage, signal overview */
    fun status(cmd: CmdMsg): String {
        val battery = getBatteryInfo()
        val storage = getStorageInfo()
        val wifi = getWifiInfo()

        return resultOk(cmd.id, mapOf(
            "device" to mapOf(
                "model" to Build.MODEL,
                "manufacturer" to Build.MANUFACTURER,
                "android" to Build.VERSION.RELEASE,
                "sdk" to Build.VERSION.SDK_INT
            ),
            "battery" to battery,
            "storage" to storage,
            "wifi" to wifi
        ))
    }

    /** psh battery — detailed battery stats */
    fun battery(cmd: CmdMsg): String =
        resultOk(cmd.id, getBatteryInfo())

    /** psh location — GPS coordinates */
    fun location(cmd: CmdMsg): String {
        if (ActivityCompat.checkSelfPermission(context, Manifest.permission.ACCESS_FINE_LOCATION)
            != PackageManager.PERMISSION_GRANTED) {
            return resultErr(cmd.id, "location permission not granted — grant in PhoneSSH app")
        }

        val lm = context.getSystemService(Context.LOCATION_SERVICE) as LocationManager
        val providers = listOf(LocationManager.GPS_PROVIDER, LocationManager.NETWORK_PROVIDER)

        var best: Location? = null
        for (provider in providers) {
            if (!lm.isProviderEnabled(provider)) continue
            val loc = lm.getLastKnownLocation(provider) ?: continue
            if (best == null || loc.accuracy < best.accuracy) best = loc
        }

        return if (best != null) {
            resultOk(cmd.id, mapOf(
                "latitude" to best.latitude,
                "longitude" to best.longitude,
                "accuracy" to best.accuracy,
                "altitude" to best.altitude,
                "speed" to best.speed,
                "provider" to best.provider,
                "time" to best.time,
                "maps_url" to "https://maps.google.com/?q=${best.latitude},${best.longitude}"
            ))
        } else {
            resultErr(cmd.id, "no location available — ensure GPS is enabled")
        }
    }

    /** psh screenshot — uses Accessibility Service takeScreenshot() (Android 9+) */
    fun screenshot(cmd: CmdMsg): String {
        val bytes = com.phonessh.app.PshAccessibilityService.captureScreenshot()
            ?: return resultErr(cmd.id,
                "screenshot requires Accessibility access — in PhoneSSH app tap 'Grant Accessibility Access'")

        val b64 = java.util.Base64.getEncoder().encodeToString(bytes)
        return resultOk(cmd.id, mapOf(
            "filename" to "screenshot.png",
            "size" to bytes.size,
            "content" to b64,
            "encoding" to "base64"
        ))
    }

    /** psh volume [set <0-100> | get] [--stream music|ring|alarm] */
    fun volume(cmd: CmdMsg): String {
        val audio = context.getSystemService(Context.AUDIO_SERVICE) as AudioManager
        val subCmd = cmd.args.firstOrNull() ?: "get"
        val streamType = when (cmd.flags["stream"]) {
            "ring"  -> AudioManager.STREAM_RING
            "alarm" -> AudioManager.STREAM_ALARM
            "call"  -> AudioManager.STREAM_VOICE_CALL
            else    -> AudioManager.STREAM_MUSIC
        }

        return when (subCmd) {
            "get" -> {
                val current = audio.getStreamVolume(streamType)
                val max = audio.getStreamMaxVolume(streamType)
                resultOk(cmd.id, mapOf(
                    "current" to current,
                    "max" to max,
                    "percent" to (current * 100 / max)
                ))
            }
            "set" -> {
                val pct = cmd.args.getOrNull(1)?.toIntOrNull()
                    ?: return resultErr(cmd.id, "usage: volume set <0-100>")
                val max = audio.getStreamMaxVolume(streamType)
                val level = (pct.coerceIn(0, 100) * max / 100)
                audio.setStreamVolume(streamType, level, 0)
                resultOk(cmd.id, mapOf("set" to level, "max" to max))
            }
            "mute" -> {
                audio.adjustStreamVolume(streamType, AudioManager.ADJUST_MUTE, 0)
                resultOk(cmd.id, mapOf("muted" to true))
            }
            "unmute" -> {
                audio.adjustStreamVolume(streamType, AudioManager.ADJUST_UNMUTE, 0)
                resultOk(cmd.id, mapOf("muted" to false))
            }
            else -> resultErr(cmd.id, "usage: volume [get|set <0-100>|mute|unmute]")
        }
    }

    /** psh brightness [get | set <0-100>] */
    fun brightness(cmd: CmdMsg): String {
        val subCmd = cmd.args.firstOrNull() ?: "get"
        return when (subCmd) {
            "get" -> {
                val b = Settings.System.getInt(context.contentResolver,
                    Settings.System.SCREEN_BRIGHTNESS, -1)
                resultOk(cmd.id, mapOf("raw" to b, "percent" to (b * 100 / 255)))
            }
            "set" -> {
                val pct = cmd.args.getOrNull(1)?.toIntOrNull()
                    ?: return resultErr(cmd.id, "usage: brightness set <0-100>")
                if (!Settings.System.canWrite(context))
                    return resultErr(cmd.id, "WRITE_SETTINGS permission needed — enable in Settings > Apps > PhoneSSH > Modify system settings")
                val raw = (pct.coerceIn(0, 100) * 255 / 100)
                Settings.System.putInt(context.contentResolver,
                    Settings.System.SCREEN_BRIGHTNESS, raw)
                resultOk(cmd.id, mapOf("set" to pct, "raw" to raw))
            }
            else -> resultErr(cmd.id, "usage: brightness [get|set <0-100>]")
        }
    }

    /** psh dnd [on|off|status] [--until HH:MM] */
    fun dnd(cmd: CmdMsg): String {
        val nm = context.getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
        if (!nm.isNotificationPolicyAccessGranted) {
            return resultErr(cmd.id, "DND access not granted — grant in Settings > Apps > PhoneSSH > Do Not Disturb access")
        }

        val subCmd = cmd.args.firstOrNull() ?: "status"
        return when (subCmd) {
            "on" -> {
                nm.setInterruptionFilter(NotificationManager.INTERRUPTION_FILTER_NONE)
                resultOk(cmd.id, mapOf("dnd" to "on"))
            }
            "off" -> {
                nm.setInterruptionFilter(NotificationManager.INTERRUPTION_FILTER_ALL)
                resultOk(cmd.id, mapOf("dnd" to "off"))
            }
            "status" -> {
                val filter = nm.currentInterruptionFilter
                val status = when (filter) {
                    NotificationManager.INTERRUPTION_FILTER_NONE -> "on (total silence)"
                    NotificationManager.INTERRUPTION_FILTER_PRIORITY -> "on (priority only)"
                    NotificationManager.INTERRUPTION_FILTER_ALARMS -> "on (alarms only)"
                    else -> "off"
                }
                resultOk(cmd.id, mapOf("dnd" to status, "filter" to filter))
            }
            "priority" -> {
                nm.setInterruptionFilter(NotificationManager.INTERRUPTION_FILTER_PRIORITY)
                resultOk(cmd.id, mapOf("dnd" to "on (priority only)"))
            }
            else -> resultErr(cmd.id, "usage: dnd [on|off|priority|status]")
        }
    }

    /** psh wifi [status | connect <ssid> [--password <pwd>] | disconnect | list] */
    fun wifi(cmd: CmdMsg): String {
        val wm = context.applicationContext.getSystemService(Context.WIFI_SERVICE) as WifiManager
        val subCmd = cmd.args.firstOrNull() ?: "status"

        return when (subCmd) {
            "status" -> {
                val info = wm.connectionInfo
                resultOk(cmd.id, mapOf(
                    "enabled" to wm.isWifiEnabled,
                    "ssid" to info.ssid.removePrefix("\"").removeSuffix("\""),
                    "rssi" to info.rssi,
                    "signal" to WifiManager.calculateSignalLevel(info.rssi, 5),
                    "ip" to intToIp(info.ipAddress),
                    "mac" to info.macAddress
                ))
            }
            "enable" -> {
                // wm.setWifiEnabled deprecated in API 29 — guide user via Settings
                resultErr(cmd.id, "Enabling WiFi programmatically requires user action in Android 10+. Open Settings > WiFi.")
            }
            "list" -> {
                val results = wm.scanResults.map { r ->
                    mapOf("ssid" to r.SSID, "bssid" to r.BSSID, "level" to r.level, "frequency" to r.frequency)
                }
                resultOk(cmd.id, mapOf("networks" to results))
            }
            else -> resultErr(cmd.id, "usage: wifi [status|list]")
        }
    }

    /** psh clipboard [get | set <text>] */
    fun clipboard(cmd: CmdMsg): String {
        val cm = context.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
        val subCmd = cmd.args.firstOrNull() ?: "get"

        return when (subCmd) {
            "get" -> {
                val text = cm.primaryClip?.getItemAt(0)?.text?.toString()
                resultOk(cmd.id, mapOf("text" to text))
            }
            "set" -> {
                val text = cmd.args.drop(1).joinToString(" ")
                cm.setPrimaryClip(ClipData.newPlainText("psh", text))
                resultOk(cmd.id, mapOf("set" to text))
            }
            else -> resultErr(cmd.id, "usage: clipboard [get|set <text>]")
        }
    }

    /** psh lock */
    fun lock(cmd: CmdMsg): String {
        // Requires Device Admin - guide the user
        return resultErr(cmd.id, "lock requires Device Admin permission — enable PhoneSSH as device admin in Settings")
    }

    // ── Private helpers ──────────────────────────────────────────────────────────

    private fun getBatteryInfo(): Map<String, Any?> {
        val intent = context.registerReceiver(null,
            IntentFilter(Intent.ACTION_BATTERY_CHANGED)) ?: return emptyMap()

        val level = intent.getIntExtra(BatteryManager.EXTRA_LEVEL, -1)
        val scale = intent.getIntExtra(BatteryManager.EXTRA_SCALE, -1)
        val pct = if (level >= 0 && scale > 0) level * 100 / scale else -1
        val status = when (intent.getIntExtra(BatteryManager.EXTRA_STATUS, -1)) {
            BatteryManager.BATTERY_STATUS_CHARGING -> "charging"
            BatteryManager.BATTERY_STATUS_FULL -> "full"
            BatteryManager.BATTERY_STATUS_DISCHARGING -> "discharging"
            BatteryManager.BATTERY_STATUS_NOT_CHARGING -> "not charging"
            else -> "unknown"
        }
        val plugged = when (intent.getIntExtra(BatteryManager.EXTRA_PLUGGED, -1)) {
            BatteryManager.BATTERY_PLUGGED_AC -> "AC"
            BatteryManager.BATTERY_PLUGGED_USB -> "USB"
            BatteryManager.BATTERY_PLUGGED_WIRELESS -> "wireless"
            else -> "none"
        }
        val health = when (intent.getIntExtra(BatteryManager.EXTRA_HEALTH, -1)) {
            BatteryManager.BATTERY_HEALTH_GOOD -> "good"
            BatteryManager.BATTERY_HEALTH_OVERHEAT -> "overheat"
            BatteryManager.BATTERY_HEALTH_DEAD -> "dead"
            BatteryManager.BATTERY_HEALTH_OVER_VOLTAGE -> "over_voltage"
            else -> "unknown"
        }
        val temp = intent.getIntExtra(BatteryManager.EXTRA_TEMPERATURE, -1) / 10.0
        val voltage = intent.getIntExtra(BatteryManager.EXTRA_VOLTAGE, -1)

        return mapOf(
            "percent" to pct,
            "status" to status,
            "plugged" to plugged,
            "health" to health,
            "temperature_c" to temp,
            "voltage_mv" to voltage
        )
    }

    private fun getStorageInfo(): Map<String, Any?> {
        val ext = Environment.getExternalStorageDirectory()
        val stat = StatFs(ext.path)
        val total = stat.totalBytes
        val free = stat.availableBytes
        val used = total - free
        return mapOf(
            "total_bytes" to total,
            "used_bytes" to used,
            "free_bytes" to free,
            "used_percent" to (used * 100 / total),
            "path" to ext.absolutePath
        )
    }

    private fun getWifiInfo(): Map<String, Any?> {
        val wm = context.applicationContext.getSystemService(Context.WIFI_SERVICE) as WifiManager
        val info = wm.connectionInfo
        return mapOf(
            "enabled" to wm.isWifiEnabled,
            "ssid" to info.ssid.removePrefix("\"").removeSuffix("\""),
            "rssi" to info.rssi,
            "ip" to intToIp(info.ipAddress)
        )
    }

    private fun intToIp(ip: Int): String {
        return "${ip and 0xff}.${ip shr 8 and 0xff}.${ip shr 16 and 0xff}.${ip shr 24 and 0xff}"
    }
}
