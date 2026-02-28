package com.phonessh.app.commands

import android.content.Context
import android.os.Environment
import com.phonessh.app.protocol.CmdMsg
import com.phonessh.app.protocol.resultErr
import com.phonessh.app.protocol.resultOk
import java.io.File
import java.util.Base64

class FileCommands(private val context: Context) {

    /** psh ls [path]  — list directory contents */
    fun ls(cmd: CmdMsg): String {
        val path = cmd.args.firstOrNull() ?: Environment.getExternalStorageDirectory().path
        val dir = File(path)

        if (!dir.exists()) return resultErr(cmd.id, "path does not exist: $path")
        if (!dir.canRead()) return resultErr(cmd.id, "permission denied: $path")

        return if (dir.isFile) {
            resultOk(cmd.id, mapOf("entries" to listOf(fileEntry(dir))))
        } else {
            val entries = (dir.listFiles() ?: emptyArray())
                .sortedWith(compareBy({ !it.isDirectory }, { it.name }))
                .map { fileEntry(it) }
            resultOk(cmd.id, mapOf("path" to path, "entries" to entries))
        }
    }

    /** psh find <pattern> [path] — recursive file search */
    fun find(cmd: CmdMsg): String {
        val pattern = cmd.args.firstOrNull() ?: return resultErr(cmd.id, "usage: find <pattern> [path]")
        val root = File(cmd.args.getOrElse(1) { Environment.getExternalStorageDirectory().path })
        val regex = Regex(pattern.replace("*", ".*").replace("?", "."), RegexOption.IGNORE_CASE)

        val matches = mutableListOf<Map<String, Any?>>()
        root.walkTopDown()
            .onEnter { it.canRead() }
            .filter { regex.matches(it.name) }
            .take(500)
            .forEach { matches.add(fileEntry(it)) }

        return resultOk(cmd.id, mapOf("pattern" to pattern, "root" to root.path, "matches" to matches))
    }

    /** psh pull <remote-path> — download file (base64 encoded in response) */
    fun pull(cmd: CmdMsg): String {
        val path = cmd.args.firstOrNull() ?: return resultErr(cmd.id, "usage: pull <path>")
        val file = File(path)

        if (!file.exists()) return resultErr(cmd.id, "file not found: $path")
        if (!file.isFile) return resultErr(cmd.id, "not a file: $path")
        if (!file.canRead()) return resultErr(cmd.id, "permission denied: $path")

        val maxSize = 50 * 1024 * 1024L // 50 MB limit
        if (file.length() > maxSize) return resultErr(cmd.id, "file too large (>${maxSize / 1024 / 1024}MB): use chunked transfer")

        val content = Base64.getEncoder().encodeToString(file.readBytes())
        return resultOk(cmd.id, mapOf(
            "filename" to file.name,
            "path" to path,
            "size" to file.length(),
            "content" to content,
            "encoding" to "base64"
        ))
    }

    /** psh push <remote-path> — upload file (base64 in payload field) */
    fun push(cmd: CmdMsg): String {
        val path = cmd.args.firstOrNull() ?: return resultErr(cmd.id, "usage: push <path>")
        val payload = cmd.payload ?: return resultErr(cmd.id, "no payload provided")

        val file = File(path)
        file.parentFile?.mkdirs()

        return try {
            val bytes = Base64.getDecoder().decode(payload)
            file.writeBytes(bytes)
            resultOk(cmd.id, mapOf("path" to path, "written" to bytes.size))
        } catch (e: Exception) {
            resultErr(cmd.id, "write failed: ${e.message}")
        }
    }

    /** psh rm <path> */
    fun rm(cmd: CmdMsg): String {
        val path = cmd.args.firstOrNull() ?: return resultErr(cmd.id, "usage: rm <path>")
        val file = File(path)
        if (!file.exists()) return resultErr(cmd.id, "not found: $path")

        return if (file.deleteRecursively()) {
            resultOk(cmd.id, mapOf("deleted" to path))
        } else {
            resultErr(cmd.id, "failed to delete: $path")
        }
    }

    /** psh mkdir <path> */
    fun mkdir(cmd: CmdMsg): String {
        val path = cmd.args.firstOrNull() ?: return resultErr(cmd.id, "usage: mkdir <path>")
        val dir = File(path)
        return if (dir.mkdirs()) {
            resultOk(cmd.id, mapOf("created" to path))
        } else {
            resultErr(cmd.id, "failed to create: $path")
        }
    }

    /** psh stat <path> */
    fun stat(cmd: CmdMsg): String {
        val path = cmd.args.firstOrNull() ?: return resultErr(cmd.id, "usage: stat <path>")
        val file = File(path)
        if (!file.exists()) return resultErr(cmd.id, "not found: $path")
        return resultOk(cmd.id, fileEntry(file))
    }

    private fun fileEntry(f: File): Map<String, Any?> = mapOf(
        "name" to f.name,
        "path" to f.absolutePath,
        "type" to if (f.isDirectory) "dir" else "file",
        "size" to f.length(),
        "modified" to f.lastModified(),
        "readable" to f.canRead(),
        "writable" to f.canWrite()
    )
}
