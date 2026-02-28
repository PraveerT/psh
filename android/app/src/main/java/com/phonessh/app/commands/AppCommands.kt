package com.phonessh.app.commands

import android.content.ActivityNotFoundException
import android.content.Context
import android.content.Intent
import android.content.pm.ApplicationInfo
import android.content.pm.PackageManager
import android.net.Uri
import com.phonessh.app.protocol.CmdMsg
import com.phonessh.app.protocol.resultErr
import com.phonessh.app.protocol.resultOk
import java.io.File

class AppCommands(private val context: Context) {

    /**
     * psh apps list [--system]
     * psh apps launch <name-or-package>
     * psh apps kill <name-or-package>
     * psh apps info <name-or-package>
     * psh apps install <local-apk-path>
     * psh apps uninstall <package>
     */
    fun dispatch(cmd: CmdMsg): String {
        val subCmd = cmd.args.firstOrNull() ?: return resultErr(cmd.id, "usage: apps [list|launch|kill|info|install|uninstall]")
        return when (subCmd) {
            "list"      -> list(cmd)
            "launch"    -> launch(cmd)
            "kill"      -> kill(cmd)
            "info"      -> info(cmd)
            "install"   -> install(cmd)
            "uninstall" -> uninstall(cmd)
            else        -> resultErr(cmd.id, "unknown apps subcommand: $subCmd")
        }
    }

    private fun list(cmd: CmdMsg): String {
        val pm = context.packageManager
        val includeSystem = cmd.flags.containsKey("system")
        val filter = cmd.flags["filter"] ?: cmd.args.getOrNull(1)

        val apps = pm.getInstalledApplications(PackageManager.GET_META_DATA)
            .filter { app ->
                val isSystem = (app.flags and ApplicationInfo.FLAG_SYSTEM) != 0
                if (!includeSystem && isSystem) return@filter false
                if (filter != null) {
                    val name = pm.getApplicationLabel(app).toString()
                    name.contains(filter, ignoreCase = true) || app.packageName.contains(filter, ignoreCase = true)
                } else true
            }
            .sortedBy { pm.getApplicationLabel(it).toString().lowercase() }
            .map { app ->
                mapOf(
                    "name"    to pm.getApplicationLabel(app).toString(),
                    "package" to app.packageName,
                    "system"  to ((app.flags and ApplicationInfo.FLAG_SYSTEM) != 0),
                    "enabled" to app.enabled
                )
            }

        return resultOk(cmd.id, mapOf("count" to apps.size, "apps" to apps))
    }

    private fun launch(cmd: CmdMsg): String {
        val query = cmd.args.getOrNull(1) ?: return resultErr(cmd.id, "usage: apps launch <name-or-package>")
        val pkg = resolvePackage(query) ?: return resultErr(cmd.id, "app not found: $query")

        val intent = context.packageManager.getLaunchIntentForPackage(pkg)
            ?: return resultErr(cmd.id, "no launch intent for: $pkg")

        intent.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
        return try {
            context.startActivity(intent)
            resultOk(cmd.id, mapOf("launched" to pkg))
        } catch (e: ActivityNotFoundException) {
            resultErr(cmd.id, "could not launch: $pkg")
        }
    }

    private fun kill(cmd: CmdMsg): String {
        val query = cmd.args.getOrNull(1) ?: return resultErr(cmd.id, "usage: apps kill <name-or-package>")
        val pkg = resolvePackage(query) ?: return resultErr(cmd.id, "app not found: $query")

        // Force-stop requires FORCE_STOP_PACKAGES permission (system/privileged).
        // The best we can do without root is ask via ActivityManager.killBackgroundProcesses.
        val am = context.getSystemService(Context.ACTIVITY_SERVICE) as android.app.ActivityManager
        am.killBackgroundProcesses(pkg)
        return resultOk(cmd.id, mapOf(
            "killed" to pkg,
            "note" to "killBackgroundProcesses used — full force-stop requires root or FORCE_STOP_PACKAGES permission"
        ))
    }

    private fun info(cmd: CmdMsg): String {
        val query = cmd.args.getOrNull(1) ?: return resultErr(cmd.id, "usage: apps info <name-or-package>")
        val pkg = resolvePackage(query) ?: return resultErr(cmd.id, "app not found: $query")

        val pm = context.packageManager
        val appInfo = try {
            pm.getApplicationInfo(pkg, PackageManager.GET_META_DATA)
        } catch (e: PackageManager.NameNotFoundException) {
            return resultErr(cmd.id, "package not found: $pkg")
        }
        val pkgInfo = pm.getPackageInfo(pkg, 0)

        return resultOk(cmd.id, mapOf(
            "name"         to pm.getApplicationLabel(appInfo).toString(),
            "package"      to pkg,
            "version_name" to pkgInfo.versionName,
            "version_code" to pkgInfo.longVersionCode,
            "installed"    to pkgInfo.firstInstallTime,
            "updated"      to pkgInfo.lastUpdateTime,
            "system"       to ((appInfo.flags and ApplicationInfo.FLAG_SYSTEM) != 0),
            "enabled"      to appInfo.enabled,
            "apk_path"     to appInfo.sourceDir,
            "data_dir"     to appInfo.dataDir
        ))
    }

    private fun install(cmd: CmdMsg): String {
        val apkPath = cmd.args.getOrNull(1) ?: return resultErr(cmd.id, "usage: apps install <path-to-apk>")
        val apkFile = File(apkPath)
        if (!apkFile.exists()) return resultErr(cmd.id, "file not found: $apkPath")
        if (!apkPath.endsWith(".apk", ignoreCase = true)) return resultErr(cmd.id, "not an APK: $apkPath")

        val uri = Uri.fromFile(apkFile)
        val intent = Intent(Intent.ACTION_VIEW).apply {
            setDataAndType(uri, "application/vnd.android.package-archive")
            addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
            addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
        }

        return try {
            context.startActivity(intent)
            resultOk(cmd.id, mapOf(
                "installing" to apkPath,
                "note" to "Installation prompt opened on device"
            ))
        } catch (e: Exception) {
            resultErr(cmd.id, "install failed: ${e.message}")
        }
    }

    private fun uninstall(cmd: CmdMsg): String {
        val query = cmd.args.getOrNull(1) ?: return resultErr(cmd.id, "usage: apps uninstall <package>")
        val pkg = resolvePackage(query) ?: query  // try as literal package name

        val intent = Intent(Intent.ACTION_DELETE).apply {
            data = Uri.parse("package:$pkg")
            addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
        }

        return try {
            context.startActivity(intent)
            resultOk(cmd.id, mapOf(
                "uninstalling" to pkg,
                "note" to "Uninstall prompt opened on device"
            ))
        } catch (e: Exception) {
            resultErr(cmd.id, "uninstall failed: ${e.message}")
        }
    }

    /** Resolve a human-readable name or partial package name to a full package name. */
    private fun resolvePackage(query: String): String? {
        val pm = context.packageManager
        val apps = pm.getInstalledApplications(0)

        // Exact package match first
        if (apps.any { it.packageName == query }) return query

        // Name/package contains match
        return apps.firstOrNull { app ->
            pm.getApplicationLabel(app).toString().equals(query, ignoreCase = true) ||
            app.packageName.contains(query, ignoreCase = true) ||
            pm.getApplicationLabel(app).toString().contains(query, ignoreCase = true)
        }?.packageName
    }
}
