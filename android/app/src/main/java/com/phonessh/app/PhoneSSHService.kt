package com.phonessh.app

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Intent
import android.os.IBinder
import androidx.core.app.NotificationCompat
import androidx.lifecycle.LifecycleService
import com.phonessh.app.auth.KeyAuthManager
import com.phonessh.app.commands.CommandRouter
import com.phonessh.app.protocol.*
import kotlinx.coroutines.*
import java.io.*
import java.net.ServerSocket
import java.net.Socket
import java.util.UUID
import java.util.concurrent.ConcurrentHashMap

class PhoneSSHService : LifecycleService() {

    companion object {
        const val PORT = 8765
        const val NOTIF_ID = 1001
        const val CHANNEL_ID = "psh_service"
        const val ACTION_STOP = "com.phonessh.app.STOP"
        const val ACTION_ROTATE_TOKEN = "com.phonessh.app.ROTATE_TOKEN"

        // Broadcast back to MainActivity
        const val BROADCAST_CLIENT_CONNECTED = "com.phonessh.app.CLIENT_CONNECTED"
        const val BROADCAST_CLIENT_DISCONNECTED = "com.phonessh.app.CLIENT_DISCONNECTED"
        const val BROADCAST_COMMAND_EXECUTED = "com.phonessh.app.COMMAND_EXECUTED"
    }

    private val scope = CoroutineScope(Dispatchers.IO + SupervisorJob())
    private var serverSocket: ServerSocket? = null
    private val sessions = ConcurrentHashMap<String, ClientSession>()
    private lateinit var auth: KeyAuthManager
    private lateinit var router: CommandRouter

    override fun onCreate() {
        super.onCreate()
        auth = KeyAuthManager(this)
        router = CommandRouter(this)
        createNotificationChannel()
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        super.onStartCommand(intent, flags, startId)

        when (intent?.action) {
            ACTION_STOP -> {
                stopSelf()
                return START_NOT_STICKY
            }
            ACTION_ROTATE_TOKEN -> {
                auth.rotateToken()
                // Disconnect all existing sessions
                sessions.values.forEach { it.close() }
                sessions.clear()
                return START_STICKY
            }
        }

        startForeground(NOTIF_ID, buildNotification(0))
        scope.launch { runServer() }
        return START_STICKY
    }

    override fun onBind(intent: Intent): IBinder? {
        super.onBind(intent)
        return null
    }

    override fun onDestroy() {
        scope.cancel()
        serverSocket?.close()
        sessions.values.forEach { it.close() }
        super.onDestroy()
    }

    // ── Server loop ─────────────────────────────────────────────────────────────

    private suspend fun runServer() {
        try {
            serverSocket = ServerSocket(PORT)
            while (scope.isActive) {
                val socket = serverSocket!!.accept()
                scope.launch { handleClient(socket) }
            }
        } catch (e: Exception) {
            if (scope.isActive) {
                // Restart after a short delay
                delay(3000)
                runServer()
            }
        }
    }

    private suspend fun handleClient(socket: Socket) {
        val sessionId = UUID.randomUUID().toString().take(8)
        val session = ClientSession(sessionId, socket)

        try {
            val reader = BufferedReader(InputStreamReader(socket.getInputStream()))
            val writer = PrintWriter(BufferedWriter(OutputStreamWriter(socket.getOutputStream())), true)

            // ── Handshake ────────────────────────────────────────────────────────
            val hello = HelloMsg(
                deviceName = auth.getDeviceName(),
                phonePubkeyFingerprint = auth.tokenFingerprint()
            )
            writer.println(hello.toJson())

            val authLine = reader.readLine() ?: return
            if (authLine.msgType() != "auth") {
                writer.println(AuthFailMsg(error = "expected auth message").toJson())
                return
            }

            val authMsg = authLine.parseAuthMsg()
            if (!auth.verifyToken(authMsg.token)) {
                writer.println(AuthFailMsg(error = "invalid token").toJson())
                logAudit(sessionId, socket.inetAddress.hostAddress ?: "?", "AUTH_FAIL")
                return
            }

            writer.println(AuthOkMsg(sessionId = sessionId).toJson())
            sessions[sessionId] = session
            logAudit(sessionId, socket.inetAddress.hostAddress ?: "?", "AUTH_OK")
            updateNotification()

            sendBroadcast(Intent(BROADCAST_CLIENT_CONNECTED).apply {
                putExtra("sessionId", sessionId)
                putExtra("host", socket.inetAddress.hostAddress)
            })

            // ── Command loop ─────────────────────────────────────────────────────
            while (scope.isActive && !socket.isClosed) {
                val line = reader.readLine() ?: break
                if (line.msgType() != "cmd") continue

                val cmd = line.parseCmdMsg()
                logAudit(sessionId, socket.inetAddress.hostAddress ?: "?", cmd.cmd)

                sendBroadcast(Intent(BROADCAST_COMMAND_EXECUTED).apply {
                    putExtra("sessionId", sessionId)
                    putExtra("cmd", cmd.cmd)
                })

                val response = try {
                    router.dispatch(cmd)
                } catch (e: Exception) {
                    resultErr(cmd.id, e.message ?: "internal error")
                }

                writer.println(response)
            }

        } catch (e: Exception) {
            // Client disconnected or IO error — expected
        } finally {
            sessions.remove(sessionId)
            session.close()
            updateNotification()
            sendBroadcast(Intent(BROADCAST_DISCONNECTED).apply {
                putExtra("sessionId", sessionId)
            })
        }
    }

    // ── Notifications ────────────────────────────────────────────────────────────

    private fun createNotificationChannel() {
        val channel = NotificationChannel(
            CHANNEL_ID,
            "PhoneSSH Service",
            NotificationManager.IMPORTANCE_LOW
        ).apply {
            description = "PhoneSSH background service"
        }
        val nm = getSystemService(NotificationManager::class.java)
        nm.createNotificationChannel(channel)
    }

    private fun buildNotification(clientCount: Int = sessions.size): Notification {
        val stopIntent = PendingIntent.getService(
            this, 0,
            Intent(this, PhoneSSHService::class.java).apply { action = ACTION_STOP },
            PendingIntent.FLAG_IMMUTABLE
        )
        val openIntent = PendingIntent.getActivity(
            this, 0,
            Intent(this, MainActivity::class.java),
            PendingIntent.FLAG_IMMUTABLE
        )

        val status = if (clientCount == 0) "Listening on :$PORT" else "$clientCount client(s) connected"

        return NotificationCompat.Builder(this, CHANNEL_ID)
            .setContentTitle("PhoneSSH")
            .setContentText(status)
            .setSmallIcon(android.R.drawable.ic_menu_share)
            .setContentIntent(openIntent)
            .addAction(android.R.drawable.ic_menu_close_clear_cancel, "Stop", stopIntent)
            .setOngoing(true)
            .build()
    }

    private fun updateNotification() {
        val nm = getSystemService(NotificationManager::class.java)
        nm.notify(NOTIF_ID, buildNotification())
    }

    private fun logAudit(sessionId: String, host: String, event: String) {
        android.util.Log.i("PhoneSSH", "[$sessionId] $host → $event")
        // TODO: persist to audit log file in Phase 2
    }

    data class ClientSession(val id: String, private val socket: Socket) {
        fun close() = runCatching { socket.close() }
    }
}

private const val BROADCAST_DISCONNECTED = PhoneSSHService.BROADCAST_CLIENT_DISCONNECTED
