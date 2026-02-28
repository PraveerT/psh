package com.phonessh.app.protocol

import com.google.gson.Gson
import com.google.gson.JsonObject
import com.google.gson.JsonParser
import com.google.gson.reflect.TypeToken

val gson = Gson()

// ── Wire messages ──────────────────────────────────────────────────────────────

data class HelloMsg(
    val type: String = "hello",
    val version: String = "1",
    val deviceName: String,
    val phonePubkeyFingerprint: String
)

data class AuthMsg(
    val type: String = "auth",
    val token: String
)

data class AuthOkMsg(
    val type: String = "auth_ok",
    val sessionId: String
)

data class AuthFailMsg(
    val type: String = "auth_fail",
    val error: String
)

data class CmdMsg(
    val type: String = "cmd",
    val id: String,
    val cmd: String,
    val args: List<String> = emptyList(),
    val flags: Map<String, String> = emptyMap(),
    val payload: String? = null   // base64 for uploads
)

data class ResultMsg(
    val type: String = "result",
    val id: String,
    val ok: Boolean,
    val data: Map<String, Any?> = emptyMap(),
    val error: String? = null
)

data class StreamMsg(
    val type: String = "stream",
    val id: String,
    val chunk: Map<String, Any?>
)

// ── Helpers ────────────────────────────────────────────────────────────────────

fun Any.toJson(): String = gson.toJson(this)

fun String.msgType(): String? = try {
    JsonParser.parseString(this).asJsonObject.get("type")?.asString
} catch (e: Exception) { null }

fun String.parseCmdMsg(): CmdMsg = gson.fromJson(this, CmdMsg::class.java)
fun String.parseAuthMsg(): AuthMsg = gson.fromJson(this, AuthMsg::class.java)

fun resultOk(id: String, data: Map<String, Any?> = emptyMap()) =
    ResultMsg(id = id, ok = true, data = data).toJson()

fun resultErr(id: String, error: String) =
    ResultMsg(id = id, ok = false, error = error).toJson()
