package com.phonessh.app.auth

import android.content.Context
import android.content.SharedPreferences
import java.security.MessageDigest
import java.security.SecureRandom
import java.util.Base64

/**
 * Manages authentication tokens for PhoneSSH.
 *
 * Uses plain SharedPreferences — the token's security comes from its 256-bit
 * randomness, not from storage encryption. EncryptedSharedPreferences caused
 * Keystore issues on Honor/Huawei devices.
 */
class KeyAuthManager(context: Context) {

    companion object {
        private const val PREFS_FILE = "psh_auth"
        private const val KEY_TOKEN = "server_token"
        private const val KEY_DEVICE_NAME = "device_name"
        private const val TOKEN_BYTES = 32
    }

    private val prefs: SharedPreferences =
        context.getSharedPreferences(PREFS_FILE, Context.MODE_PRIVATE)

    /** Returns the server token, generating one if needed. Thread-safe. */
    @Synchronized
    fun getOrCreateToken(): String {
        val stored = prefs.getString(KEY_TOKEN, null)
        // Regenerate if missing or old standard-base64 format (has +, /, =)
        if (stored == null || stored.any { it == '+' || it == '/' || it == '=' }) {
            val token = generateToken()
            prefs.edit().putString(KEY_TOKEN, token).commit() // commit() not apply() — synchronous
            return token
        }
        return stored
    }

    /** Regenerate the token (revokes all existing client access). */
    @Synchronized
    fun rotateToken(): String {
        val token = generateToken()
        prefs.edit().putString(KEY_TOKEN, token).commit()
        return token
    }

    /** Verify that a presented token matches the stored one. */
    fun verifyToken(presented: String): Boolean {
        val stored = prefs.getString(KEY_TOKEN, null) ?: return false
        return MessageDigest.isEqual(
            presented.toByteArray(Charsets.UTF_8),
            stored.toByteArray(Charsets.UTF_8)
        )
    }

    /** Short fingerprint for display — first 4 bytes of SHA-256 of the token. */
    fun tokenFingerprint(): String {
        val token = getOrCreateToken()
        val digest = MessageDigest.getInstance("SHA-256").digest(token.toByteArray(Charsets.UTF_8))
        return digest.take(4).joinToString(":") { "%02x".format(it) }
    }

    fun getDeviceName(): String =
        prefs.getString(KEY_DEVICE_NAME, android.os.Build.MODEL) ?: android.os.Build.MODEL

    fun setDeviceName(name: String) {
        prefs.edit().putString(KEY_DEVICE_NAME, name).apply()
    }

    private fun generateToken(): String {
        val bytes = ByteArray(TOKEN_BYTES)
        SecureRandom().nextBytes(bytes)
        return Base64.getUrlEncoder().withoutPadding().encodeToString(bytes)
    }
}
