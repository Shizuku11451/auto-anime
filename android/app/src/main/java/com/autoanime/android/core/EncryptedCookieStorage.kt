package com.autoanime.android.core

import android.content.Context
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import android.util.Base64
import kotlinx.serialization.Serializable
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.Json
import okhttp3.Cookie
import java.security.KeyStore
import java.security.UnrecoverableKeyException
import java.security.InvalidKeyException
import java.nio.charset.StandardCharsets
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import android.security.keystore.KeyPermanentlyInvalidatedException
import javax.crypto.spec.GCMParameterSpec

class EncryptedCookieStorage(
    context: Context,
    private val keyAlias: String = KEY_ALIAS,
    preferenceName: String = PREFERENCE_NAME
) : CookieStorage {
    private val prefs = context.getSharedPreferences(preferenceName, Context.MODE_PRIVATE)
    private val json = Json { ignoreUnknownKeys = true }
    private val current = java.util.concurrent.ConcurrentHashMap<String, List<Cookie>>()
    private fun clearAll() {
        current.clear()
        prefs.edit().clear().apply()
    }
    @Synchronized private fun key(): SecretKey {
        val store = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }
        (store.getKey(keyAlias, null) as? SecretKey)?.let { return it }
        // Ciphertexts encrypted by a missing key cannot be recovered.
        clearAll()
        return KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, "AndroidKeyStore").run {
            init(KeyGenParameterSpec.Builder(keyAlias, KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT)
                .setBlockModes(KeyProperties.BLOCK_MODE_GCM).setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE).build())
            generateKey()
        }
    }
    @Synchronized override fun load(origin: String) {
        val raw = prefs.getString(keyFor(origin), null) ?: run { current.remove(origin); return }
        try {
            val bytes = Base64.decode(raw, Base64.NO_WRAP)
            require(bytes.size > 12)
            val cipher = Cipher.getInstance("AES/GCM/NoPadding")
            cipher.init(Cipher.DECRYPT_MODE, key(), GCMParameterSpec(128, bytes.copyOfRange(0, 12)))
            cipher.updateAAD(origin.toByteArray(StandardCharsets.UTF_8))
            current[origin] = json.decodeFromString<List<StoredCookie>>(String(cipher.doFinal(bytes.copyOfRange(12, bytes.size))))
                .mapNotNull { it.toCookie() }
        } catch (error: Exception) {
            if (isPermanentlyInvalidated(error)) {
                runCatching { KeyStore.getInstance("AndroidKeyStore").apply { load(null) }.deleteEntry(keyAlias) }
                clearAll()
            } else {
                current.remove(origin)
                prefs.edit().remove(keyFor(origin)).apply()
            }
        }
    }
    @Synchronized override fun save(origin: String, cookies: List<Cookie>) {
        try {
            val cipher = Cipher.getInstance("AES/GCM/NoPadding").apply { init(Cipher.ENCRYPT_MODE, key()) }
            cipher.updateAAD(origin.toByteArray(StandardCharsets.UTF_8))
            val data = cipher.iv + cipher.doFinal(json.encodeToString(cookies.map(StoredCookie::from)).toByteArray())
            if (!prefs.edit().putString(keyFor(origin), Base64.encodeToString(data, Base64.NO_WRAP)).commit()) {
                current.remove(origin)
                return
            }
            current[origin] = cookies.toList()
        } catch (error: Exception) {
            if (isPermanentlyInvalidated(error)) {
                runCatching { KeyStore.getInstance("AndroidKeyStore").apply { load(null) }.deleteEntry(keyAlias) }
                clearAll()
            } else clear(origin)
        }
    }
    override fun cookies(origin: String) = current[origin].orEmpty()
    @Synchronized override fun clear(origin: String) {
        current.remove(origin)
        prefs.edit().remove(keyFor(origin)).apply()
    }
    private fun keyFor(origin: String) = "origin_" + Base64.encodeToString(origin.toByteArray(), Base64.URL_SAFE or Base64.NO_WRAP)

    private fun isPermanentlyInvalidated(error: Throwable): Boolean = generateSequence(error) { it.cause }
        .any { it is KeyPermanentlyInvalidatedException || it is UnrecoverableKeyException || (it is InvalidKeyException && it.cause is KeyPermanentlyInvalidatedException) }

    @Serializable private data class StoredCookie(val name: String, val value: String, val expires: Long, val domain: String, val path: String, val secure: Boolean, val httpOnly: Boolean, val hostOnly: Boolean) {
        fun toCookie(): Cookie? = try {
            Cookie.Builder().name(name).value(value).expiresAt(expires).apply { if (hostOnly) hostOnlyDomain(domain) else domain(domain); path(path); if (secure) secure(); if (httpOnly) httpOnly() }.build()
        } catch (_: Exception) { null }
        companion object { fun from(c: Cookie) = StoredCookie(c.name, c.value, c.expiresAt, c.domain, c.path, c.secure, c.httpOnly, c.hostOnly) }
    }
    companion object {
        private const val KEY_ALIAS = "auto-anime-session-v1"
        private const val PREFERENCE_NAME = "session_ciphertext"
    }
}
