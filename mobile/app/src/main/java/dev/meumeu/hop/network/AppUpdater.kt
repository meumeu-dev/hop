package dev.meumeu.hop.network

import android.app.DownloadManager
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.net.Uri
import android.os.Build
import android.os.Environment
import android.util.Log
import androidx.core.content.FileProvider
import com.google.gson.Gson
import okhttp3.OkHttpClient
import okhttp3.Request
import java.io.File
import java.util.concurrent.TimeUnit

private const val TAG = "HOP-UPDATE"
private const val REPO = "meumeu-dev/hop"
private const val APK_NAME = "hop-android.apk"

data class UpdateInfo(
    val currentVersion: String,
    val latestVersion: String,
    val downloadUrl: String,
    val apkSize: Long,
    val hasUpdate: Boolean
)

object AppUpdater {

    private val client = OkHttpClient.Builder()
        .connectTimeout(15, TimeUnit.SECONDS)
        .readTimeout(15, TimeUnit.SECONDS)
        .build()

    private val gson = Gson()

    /** Check if a new version is available on GitHub releases */
    fun checkUpdate(currentVersion: String): UpdateInfo? {
        return try {
            val request = Request.Builder()
                .url("https://api.github.com/repos/$REPO/releases/latest")
                .header("Accept", "application/vnd.github.v3+json")
                .build()

            val response = client.newCall(request).execute()
            if (!response.isSuccessful) return null

            val body = response.body?.string() ?: return null
            val release = gson.fromJson(body, Map::class.java)

            val tagName = release["tag_name"] as? String ?: return null
            val latestVersion = tagName.removePrefix("v")
            val currentClean = currentVersion.removePrefix("v")

            val assets = release["assets"] as? List<*> ?: return null
            val apkAsset = assets.filterIsInstance<Map<*, *>>().find {
                it["name"] == APK_NAME
            } ?: return null

            val downloadUrl = apkAsset["browser_download_url"] as? String ?: return null
            val size = (apkAsset["size"] as? Double)?.toLong() ?: 0L

            val hasUpdate = compareVersions(currentClean, latestVersion) < 0

            Log.i(TAG, "Current: $currentClean, Latest: $latestVersion, Update: $hasUpdate")

            UpdateInfo(
                currentVersion = currentClean,
                latestVersion = latestVersion,
                downloadUrl = downloadUrl,
                apkSize = size,
                hasUpdate = hasUpdate
            )
        } catch (e: Exception) {
            Log.e(TAG, "Check update failed", e)
            null
        }
    }

    /** Download APK via DownloadManager and trigger install when done */
    fun downloadAndInstall(context: Context, updateInfo: UpdateInfo) {
        Log.i(TAG, "Downloading ${updateInfo.downloadUrl}")

        val downloadManager = context.getSystemService(Context.DOWNLOAD_SERVICE) as DownloadManager

        val request = DownloadManager.Request(Uri.parse(updateInfo.downloadUrl))
            .setTitle("Hop v${updateInfo.latestVersion}")
            .setDescription("Mise a jour en cours...")
            .setNotificationVisibility(DownloadManager.Request.VISIBILITY_VISIBLE_NOTIFY_COMPLETED)
            .setDestinationInExternalPublicDir(Environment.DIRECTORY_DOWNLOADS, "hop-update.apk")
            .setMimeType("application/vnd.android.package-archive")

        // Delete old download if exists
        val oldFile = File(
            Environment.getExternalStoragePublicDirectory(Environment.DIRECTORY_DOWNLOADS),
            "hop-update.apk"
        )
        oldFile.delete()

        val downloadId = downloadManager.enqueue(request)

        // Register receiver to install when download completes
        val receiver = object : BroadcastReceiver() {
            override fun onReceive(ctx: Context, intent: Intent) {
                val id = intent.getLongExtra(DownloadManager.EXTRA_DOWNLOAD_ID, -1)
                if (id == downloadId) {
                    ctx.unregisterReceiver(this)
                    installApk(ctx)
                }
            }
        }

        context.registerReceiver(
            receiver,
            IntentFilter(DownloadManager.ACTION_DOWNLOAD_COMPLETE),
            Context.RECEIVER_NOT_EXPORTED
        )
    }

    private fun installApk(context: Context) {
        val file = File(
            Environment.getExternalStoragePublicDirectory(Environment.DIRECTORY_DOWNLOADS),
            "hop-update.apk"
        )

        if (!file.exists()) {
            Log.e(TAG, "APK not found: ${file.absolutePath}")
            return
        }

        val uri = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.N) {
            FileProvider.getUriForFile(context, "${context.packageName}.fileprovider", file)
        } else {
            Uri.fromFile(file)
        }

        val intent = Intent(Intent.ACTION_VIEW).apply {
            setDataAndType(uri, "application/vnd.android.package-archive")
            flags = Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_GRANT_READ_URI_PERMISSION
        }
        context.startActivity(intent)
    }

    /** Compare semantic versions: returns -1 if a < b, 0 if equal, 1 if a > b */
    private fun compareVersions(a: String, b: String): Int {
        val partsA = a.split(".").map { it.toIntOrNull() ?: 0 }
        val partsB = b.split(".").map { it.toIntOrNull() ?: 0 }
        val maxLen = maxOf(partsA.size, partsB.size)
        for (i in 0 until maxLen) {
            val va = partsA.getOrElse(i) { 0 }
            val vb = partsB.getOrElse(i) { 0 }
            if (va < vb) return -1
            if (va > vb) return 1
        }
        return 0
    }
}
