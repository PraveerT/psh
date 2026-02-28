package com.phonessh.app

import android.Manifest
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.content.pm.PackageManager
import android.net.wifi.WifiManager
import android.os.Build
import android.os.Bundle
import android.provider.Settings
import android.view.View
import android.widget.Toast
import androidx.activity.result.contract.ActivityResultContracts
import androidx.appcompat.app.AlertDialog
import androidx.appcompat.app.AppCompatActivity
import androidx.core.content.ContextCompat
import com.google.zxing.BarcodeFormat
import com.journeyapps.barcodescanner.BarcodeEncoder
import com.phonessh.app.auth.KeyAuthManager
import com.phonessh.app.databinding.ActivityMainBinding
import java.net.Inet4Address
import java.net.NetworkInterface

class MainActivity : AppCompatActivity() {

    private lateinit var binding: ActivityMainBinding
    private lateinit var auth: KeyAuthManager
    private var serviceRunning = false

    private val statusReceiver = object : BroadcastReceiver() {
        override fun onReceive(context: Context, intent: Intent) {
            when (intent.action) {
                PhoneSSHService.BROADCAST_CLIENT_CONNECTED -> {
                    val host = intent.getStringExtra("host") ?: "?"
                    addLogEntry("Client connected from $host")
                    updateStatus()
                }
                PhoneSSHService.BROADCAST_CLIENT_DISCONNECTED -> {
                    addLogEntry("Client disconnected")
                    updateStatus()
                }
                PhoneSSHService.BROADCAST_COMMAND_EXECUTED -> {
                    val cmd = intent.getStringExtra("cmd") ?: "?"
                    addLogEntry("→ $cmd")
                }
            }
        }
    }

    private val permissionLauncher = registerForActivityResult(
        ActivityResultContracts.RequestMultiplePermissions()
    ) { results ->
        val denied = results.entries.filter { !it.value }.map { it.key }
        if (denied.isNotEmpty()) {
            Toast.makeText(this, "Some permissions denied — some features may not work", Toast.LENGTH_LONG).show()
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        binding = ActivityMainBinding.inflate(layoutInflater)
        setContentView(binding.root)

        auth = KeyAuthManager(this)
        setupUI()
        requestRequiredPermissions()
        startService()
    }

    override fun onResume() {
        super.onResume()
        val filter = IntentFilter().apply {
            addAction(PhoneSSHService.BROADCAST_CLIENT_CONNECTED)
            addAction(PhoneSSHService.BROADCAST_CLIENT_DISCONNECTED)
            addAction(PhoneSSHService.BROADCAST_COMMAND_EXECUTED)
        }
        registerReceiver(statusReceiver, filter, RECEIVER_NOT_EXPORTED)
        updateStatus()
        generateQrCode()
    }

    override fun onPause() {
        super.onPause()
        runCatching { unregisterReceiver(statusReceiver) }
    }

    private fun setupUI() {
        binding.btnStartStop.setOnClickListener {
            if (serviceRunning) stopService() else startService()
        }

        binding.btnRotateToken.setOnClickListener {
            AlertDialog.Builder(this)
                .setTitle("Rotate Token")
                .setMessage("This will disconnect all existing clients and invalidate the current token. They will need to re-pair.\n\nContinue?")
                .setPositiveButton("Rotate") { _, _ ->
                    val intent = Intent(this, PhoneSSHService::class.java).apply {
                        action = PhoneSSHService.ACTION_ROTATE_TOKEN
                    }
                    startForegroundService(intent)
                    generateQrCode()
                    addLogEntry("Token rotated — all clients disconnected")
                }
                .setNegativeButton("Cancel", null)
                .show()
        }

        binding.btnCopyUrl.setOnClickListener {
            val url = binding.tvPairUrl.text.toString()
            if (url.isNotEmpty()) {
                val cm = getSystemService(Context.CLIPBOARD_SERVICE) as android.content.ClipboardManager
                cm.setPrimaryClip(android.content.ClipData.newPlainText("psh pair URL", url))
                Toast.makeText(this, "Pairing URL copied!", Toast.LENGTH_SHORT).show()
            }
        }

        binding.btnNotifAccess.setOnClickListener {
            startActivity(Intent(Settings.ACTION_NOTIFICATION_LISTENER_SETTINGS))
        }

        binding.btnDndAccess.setOnClickListener {
            startActivity(Intent(Settings.ACTION_NOTIFICATION_POLICY_ACCESS_SETTINGS))
        }

        binding.btnWriteSettings.setOnClickListener {
            startActivity(Intent(Settings.ACTION_MANAGE_WRITE_SETTINGS).apply {
                data = android.net.Uri.parse("package:$packageName")
            })
        }

        binding.btnAccessibilityAccess.setOnClickListener {
            startActivity(Intent(Settings.ACTION_ACCESSIBILITY_SETTINGS))
        }
    }

    private fun startService() {
        val intent = Intent(this, PhoneSSHService::class.java)
        startForegroundService(intent)
        serviceRunning = true
        updateStatus()
    }

    private fun stopService() {
        val intent = Intent(this, PhoneSSHService::class.java).apply {
            action = PhoneSSHService.ACTION_STOP
        }
        startService(intent)
        serviceRunning = false
        updateStatus()
    }

    private fun generateQrCode() {
        val token = auth.getOrCreateToken()
        val deviceName = auth.getDeviceName()
        val ips = getLocalIps()
        val primaryIp = ips.firstOrNull() ?: "unknown"

        // QR payload: psh://pair?host=IP&port=PORT&token=TOKEN&name=DEVICE
        val payload = "psh://pair?host=$primaryIp&port=${PhoneSSHService.PORT}" +
            "&token=${android.net.Uri.encode(token)}" +
            "&name=${android.net.Uri.encode(deviceName)}"

        binding.tvPairUrl.text = payload
        binding.tvIpAddresses.text = "IP: ${ips.joinToString(", ")}"
        binding.tvFingerprint.text = "Token fingerprint: ${auth.tokenFingerprint()}"

        try {
            val encoder = BarcodeEncoder()
            val bitmap = encoder.encodeBitmap(payload, BarcodeFormat.QR_CODE, 512, 512)
            binding.ivQrCode.setImageBitmap(bitmap)
        } catch (e: Exception) {
            binding.ivQrCode.visibility = View.GONE
        }
    }

    private fun updateStatus() {
        binding.tvStatus.text = if (serviceRunning) "Running on :${PhoneSSHService.PORT}" else "Stopped"
        binding.btnStartStop.text = if (serviceRunning) "Stop Service" else "Start Service"

        // Permission status indicators
        binding.tvNotifStatus.text = if (PshNotificationListenerService.instance != null)
            "Notification access: granted" else "Notification access: not granted"

        val nm = getSystemService(android.app.NotificationManager::class.java)
        binding.tvDndStatus.text = if (nm.isNotificationPolicyAccessGranted)
            "DND access: granted" else "DND access: not granted"

        binding.tvWriteSettingsStatus.text = if (Settings.System.canWrite(this))
            "Write settings: granted" else "Write settings: not granted"

        binding.tvAccessibilityStatus.text = if (PshAccessibilityService.instance != null)
            "Accessibility (screenshot): granted" else "Accessibility (screenshot): not granted"
    }

    private fun addLogEntry(text: String) {
        val current = binding.tvLog.text.toString()
        val timestamp = java.text.SimpleDateFormat("HH:mm:ss", java.util.Locale.US)
            .format(java.util.Date())
        val newLog = "[$timestamp] $text\n$current"
        binding.tvLog.text = newLog.lines().take(50).joinToString("\n")
    }

    private fun getLocalIps(): List<String> {
        return try {
            NetworkInterface.getNetworkInterfaces().toList()
                .flatMap { it.inetAddresses.toList() }
                .filterIsInstance<Inet4Address>()
                .filter { !it.isLoopbackAddress }
                .map { it.hostAddress ?: "" }
                .filter { it.isNotEmpty() }
        } catch (e: Exception) {
            emptyList()
        }
    }

    private fun requestRequiredPermissions() {
        val permissions = mutableListOf(
            Manifest.permission.READ_EXTERNAL_STORAGE,
            Manifest.permission.READ_SMS,
            Manifest.permission.SEND_SMS,
            Manifest.permission.READ_CONTACTS,
            Manifest.permission.ACCESS_FINE_LOCATION,
            Manifest.permission.POST_NOTIFICATIONS
        )
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            permissions.add(Manifest.permission.READ_MEDIA_IMAGES)
            permissions.add(Manifest.permission.READ_MEDIA_VIDEO)
        }

        val toRequest = permissions.filter {
            ContextCompat.checkSelfPermission(this, it) != PackageManager.PERMISSION_GRANTED
        }

        if (toRequest.isNotEmpty()) {
            permissionLauncher.launch(toRequest.toTypedArray())
        }
    }
}
