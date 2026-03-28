# sshj + eddsa
-keep class net.schmizz.** { *; }
-keep class com.hierynomus.** { *; }
-keep class net.i2p.crypto.eddsa.** { *; }
-dontwarn net.schmizz.**
-dontwarn com.hierynomus.**
-dontwarn net.i2p.crypto.eddsa.**
-dontwarn sun.security.x509.**

# Bouncy Castle
-keep class org.bouncycastle.** { *; }
-dontwarn org.bouncycastle.**

# SnakeYAML
-keep class org.yaml.** { *; }
-dontwarn org.yaml.**
