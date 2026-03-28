package dev.meumeu.hop

import android.app.Application
import org.bouncycastle.jce.provider.BouncyCastleProvider
import java.security.Security

class HopApplication : Application() {
    override fun onCreate() {
        super.onCreate()
        // Register Bouncy Castle for Argon2 + Ed25519
        Security.removeProvider("BC")
        Security.addProvider(BouncyCastleProvider())
    }
}
