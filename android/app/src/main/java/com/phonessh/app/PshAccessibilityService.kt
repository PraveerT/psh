package com.phonessh.app

import android.accessibilityservice.AccessibilityService
import android.graphics.Bitmap
import android.os.Handler
import android.os.Looper
import android.view.Display
import android.view.accessibility.AccessibilityEvent
import java.io.ByteArrayOutputStream
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicReference

class PshAccessibilityService : AccessibilityService() {

    companion object {
        @Volatile
        var instance: PshAccessibilityService? = null
            private set

        /** Take a screenshot synchronously (blocks up to 5s). Returns PNG bytes or null. */
        fun captureScreenshot(): ByteArray? {
            val svc = instance ?: return null

            val result = AtomicReference<ByteArray?>(null)
            val latch = CountDownLatch(1)

            // takeScreenshot must be called on main thread
            Handler(Looper.getMainLooper()).post {
                svc.takeScreenshot(
                    Display.DEFAULT_DISPLAY,
                    svc.mainExecutor,
                    object : TakeScreenshotCallback {
                        override fun onSuccess(screenshotResult: ScreenshotResult) {
                            try {
                                val bitmap = Bitmap.wrapHardwareBuffer(
                                    screenshotResult.hardwareBuffer,
                                    screenshotResult.colorSpace
                                )?.copy(Bitmap.Config.ARGB_8888, false)

                                screenshotResult.hardwareBuffer.close()

                                if (bitmap != null) {
                                    val out = ByteArrayOutputStream()
                                    bitmap.compress(Bitmap.CompressFormat.PNG, 90, out)
                                    bitmap.recycle()
                                    result.set(out.toByteArray())
                                }
                            } finally {
                                latch.countDown()
                            }
                        }

                        override fun onFailure(errorCode: Int) {
                            latch.countDown()
                        }
                    }
                )
            }

            latch.await(5, TimeUnit.SECONDS)
            return result.get()
        }
    }

    override fun onServiceConnected() {
        instance = this
    }

    override fun onUnbind(intent: android.content.Intent?): Boolean {
        instance = null
        return super.onUnbind(intent)
    }

    override fun onAccessibilityEvent(event: AccessibilityEvent?) {}
    override fun onInterrupt() {}
}
