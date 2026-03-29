package dev.meumeu.hop.network

import android.content.Context
import android.content.Intent
import android.net.Uri
import android.os.Build
import android.util.Log
import androidx.core.content.FileProvider
import com.google.gson.Gson
import okhttp3.OkHttpClient
import okhttp3.Request
import java.io.File
import java.io.FileOutputStream
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
        .readTimeout(60, TimeUnit.SECONDS)
        .followRedirects(true)
        .build()

    private val gson = Gson()

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

    /**
     * Download APK to app cache dir then trigger install intent.
     * Must be called from a background thread.
     * Returns the downloaded file on success.
     */
    fun downloadApk(context: Context, updateInfo: UpdateInfo, onProgress: (Int) -> Unit): File {
        Log.i(TAG, "Downloading ${updateInfo.downloadUrl}")

        val request = Request.Builder()
            .url(updateInfo.downloadUrl)
            .build()

        val response = client.newCall(request).execute()
        if (!response.isSuccessful) {
            throw Exception("Download failed: HTTP ${response.code}")
        }

        val apkFile = File(context.cacheDir, "hop-update.apk")
        apkFile.delete()

        val body = response.body ?: throw Exception("Empty response")
        val totalBytes = body.contentLength()

        body.byteStream().use { input ->
            FileOutputStream(apkFile).use { output ->
                val buffer = ByteArray(8192)
                var downloaded = 0L
                var read: Int
                while (input.read(buffer).also { read = it } > 0) {
                    output.write(buffer, 0, read)
                    downloaded += read
                    if (totalBytes > 0) {
                        onProgress((downloaded * 100 / totalBytes).toInt())
                    }
                }
            }
        }

        Log.i(TAG, "Downloaded to ${apkFile.absolutePath} (${apkFile.length()} bytes)")
        return apkFile
    }

    /** Launch the APK install intent */
    fun installApk(context: Context, apkFile: File) {
        val uri = FileProvider.getUriForFile(
            context,
            "${context.packageName}.fileprovider",
            apkFile
        )

        val intent = Intent(Intent.ACTION_VIEW).apply {
            setDataAndType(uri, "application/vnd.android.package-archive")
            flags = Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_GRANT_READ_URI_PERMISSION
        }

        Log.i(TAG, "Launching install intent for $uri")
        context.startActivity(intent)
    }

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
