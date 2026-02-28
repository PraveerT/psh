package com.phonessh.app

import android.accessibilityservice.AccessibilityService
import android.accessibilityservice.GestureDescription
import android.graphics.Bitmap
import android.graphics.Path
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.view.Display
import android.view.accessibility.AccessibilityEvent
import android.view.accessibility.AccessibilityNodeInfo
import java.io.ByteArrayOutputStream
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicBoolean
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

        /** Tap the screen at (x, y). Returns true on success. */
        fun dispatchTap(x: Float, y: Float): Boolean {
            val svc = instance ?: return false
            val success = AtomicBoolean(false)
            val latch = CountDownLatch(1)

            Handler(Looper.getMainLooper()).post {
                val path = Path().apply { moveTo(x, y) }
                val stroke = GestureDescription.StrokeDescription(path, 0, 100)
                val gesture = GestureDescription.Builder().addStroke(stroke).build()

                svc.dispatchGesture(gesture, object : GestureResultCallback() {
                    override fun onCompleted(gestureDescription: GestureDescription) {
                        success.set(true)
                        latch.countDown()
                    }
                    override fun onCancelled(gestureDescription: GestureDescription) {
                        latch.countDown()
                    }
                }, null)
            }

            latch.await(2, TimeUnit.SECONDS)
            return success.get()
        }

        /** Swipe from (x1,y1) to (x2,y2) over durationMs. Returns true on success. */
        fun dispatchSwipe(x1: Float, y1: Float, x2: Float, y2: Float, durationMs: Long): Boolean {
            val svc = instance ?: return false
            val success = AtomicBoolean(false)
            val latch = CountDownLatch(1)

            Handler(Looper.getMainLooper()).post {
                val path = Path().apply {
                    moveTo(x1, y1)
                    lineTo(x2, y2)
                }
                val stroke = GestureDescription.StrokeDescription(path, 0, durationMs.coerceAtLeast(1))
                val gesture = GestureDescription.Builder().addStroke(stroke).build()

                svc.dispatchGesture(gesture, object : GestureResultCallback() {
                    override fun onCompleted(gestureDescription: GestureDescription) {
                        success.set(true)
                        latch.countDown()
                    }
                    override fun onCancelled(gestureDescription: GestureDescription) {
                        latch.countDown()
                    }
                }, null)
            }

            latch.await(durationMs + 2000, TimeUnit.MILLISECONDS)
            return success.get()
        }

        /** Set text in the currently focused input field. Returns true on success. */
        fun typeText(text: String): Boolean {
            val svc = instance ?: return false

            val root = svc.rootInActiveWindow ?: return false
            val focused = root.findFocus(AccessibilityNodeInfo.FOCUS_INPUT) ?: return false

            val args = Bundle().apply {
                putCharSequence(AccessibilityNodeInfo.ACTION_ARGUMENT_SET_TEXT_CHARSEQUENCE, text)
            }
            val result = focused.performAction(AccessibilityNodeInfo.ACTION_SET_TEXT, args)
            focused.recycle()
            root.recycle()
            return result
        }

        /**
         * Click a UI element by its visible text or content description.
         * Tries ACTION_CLICK first; falls back to gesture tap on the element center.
         */
        fun clickByText(text: String): Pair<Boolean, String> {
            val svc = instance ?: return Pair(false, "accessibility service not running")
            val root = svc.rootInActiveWindow ?: return Pair(false, "no active window")

            val nodes = root.findAccessibilityNodeInfosByText(text)
            if (nodes.isNullOrEmpty()) {
                root.recycle()
                return Pair(false, "no element found with text: $text")
            }

            val node = nodes.first()
            val rect = android.graphics.Rect()
            node.getBoundsInScreen(rect)

            // Try accessibility ACTION_CLICK first
            var success = node.performAction(AccessibilityNodeInfo.ACTION_CLICK)
            if (!success) {
                // Fall back to gesture tap at element center
                val cx = (rect.left + rect.right) / 2f
                val cy = (rect.top + rect.bottom) / 2f
                success = dispatchTap(cx, cy)
            }

            val label = node.text?.toString() ?: node.contentDescription?.toString() ?: text
            nodes.forEach { it.recycle() }
            root.recycle()
            return Pair(success, "clicked: $label at (${rect.centerX()}, ${rect.centerY()})")
        }

        /**
         * Dump all interactive/labelled UI elements in the current window.
         * Returns a list of maps with text, description, class, bounds, and center.
         */
        fun dumpElements(): List<Map<String, Any>> {
            val svc = instance ?: return emptyList()
            val root = svc.rootInActiveWindow ?: return emptyList()
            val results = mutableListOf<Map<String, Any>>()
            collectNodes(root, results)
            root.recycle()
            return results
        }

        private fun collectNodes(node: AccessibilityNodeInfo, out: MutableList<Map<String, Any>>) {
            val rect = android.graphics.Rect()
            node.getBoundsInScreen(rect)
            val text = node.text?.toString() ?: ""
            val desc = node.contentDescription?.toString() ?: ""

            if ((text.isNotEmpty() || desc.isNotEmpty() || node.isClickable) && rect.width() > 0) {
                out.add(mapOf(
                    "text"      to text,
                    "desc"      to desc,
                    "class"     to (node.className?.toString()?.substringAfterLast('.') ?: ""),
                    "clickable" to node.isClickable,
                    "cx"        to rect.centerX(),
                    "cy"        to rect.centerY(),
                    "bounds"    to "${rect.left},${rect.top},${rect.right},${rect.bottom}"
                ))
            }

            for (i in 0 until node.childCount) {
                val child = node.getChild(i) ?: continue
                collectNodes(child, out)
                child.recycle()
            }
        }

        /**
         * Press a navigation/hardware key.
         * Valid actions: back, home, recents, notifications
         */
        fun pressKey(action: String): Boolean {
            val svc = instance ?: return false

            val globalAction = when (action.lowercase()) {
                "back"          -> GLOBAL_ACTION_BACK
                "home"          -> GLOBAL_ACTION_HOME
                "recents"       -> GLOBAL_ACTION_RECENTS
                "notifications" -> GLOBAL_ACTION_NOTIFICATIONS
                else            -> return false
            }
            return svc.performGlobalAction(globalAction)
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
