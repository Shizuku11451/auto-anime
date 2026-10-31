package com.autoanime.android

import android.util.Base64
import android.content.Context
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.autoanime.android.core.EncryptedCookieStorage
import okhttp3.Cookie
import java.security.KeyStore
import java.util.UUID
import org.junit.Assert.*
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class CookieStorageInstrumentedTest {
    private val instrumentation get() = InstrumentationRegistry.getInstrumentation()
    // Instrumentation context has its own private preferences; don't touch app session data.
    private val context get() = instrumentation.context
    private val testAlias = "auto-anime-instrumented-${UUID.randomUUID()}"
    private val testPreferenceName = "session_ciphertext_test_${UUID.randomUUID()}"
    private val prefs get() = context.getSharedPreferences(testPreferenceName, Context.MODE_PRIVATE)

    private fun storage() = EncryptedCookieStorage(context, testAlias, testPreferenceName)

    private fun cleanup() {
        prefs.edit().clear().commit()
        runCatching { KeyStore.getInstance("AndroidKeyStore").apply { load(null) }.deleteEntry(testAlias) }
    }

    @Test fun encryptedCookieSurvivesStorageRecreation() {
        try {
            val origin = "https://storage-test.invalid:443|alice"
            val cookie = Cookie.Builder().name("auto_anime_session").value("opaque-test-only")
                .hostOnlyDomain("storage-test.invalid").path("/").build()
            val storage = storage()
            storage.save(origin, listOf(cookie))
            assertEquals("save should publish to the live jar immediately", "opaque-test-only", storage.cookies(origin).single().value)
            assertEquals("opaque-test-only", storage.cookies(origin).single().value)
            val restored = storage().apply { load(origin) }.cookies(origin)
            assertEquals("opaque-test-only", restored.single().value)
        } finally { cleanup() }
    }

    @Test fun malformedStoredCiphertextFailsClosedAndClearsOnlyThatOrigin() {
        try {
            val originA = "https://a.storage-test.invalid:443|alice"
            val originB = "https://b.storage-test.invalid:443|alice"
            fun key(origin: String) = "origin_" + Base64.encodeToString(origin.toByteArray(), Base64.URL_SAFE or Base64.NO_WRAP)
            val a = Cookie.Builder().name("auto_anime_session").value("only-a").hostOnlyDomain("a.storage-test.invalid").path("/").build()
            val b = Cookie.Builder().name("auto_anime_session").value("only-b").hostOnlyDomain("b.storage-test.invalid").path("/").build()
            val writer = storage()
            writer.save(originA, listOf(a)); writer.save(originB, listOf(b))
            prefs.edit().putString(key(originA), "broken-ciphertext").commit()
            val reader = storage()
            reader.load(originA)
            reader.load(originB)
            assertTrue(reader.cookies(originA).isEmpty())
            assertEquals("only-b", reader.cookies(originB).single().value)
        } finally { cleanup() }
    }

    @Test fun copiedCiphertextCannotAuthenticateForAnotherOrigin() {
        try {
            val originA = "https://a.storage-test.invalid:443|alice"
            val originB = "https://b.storage-test.invalid:443|alice"
            fun key(origin: String) = "origin_" + Base64.encodeToString(origin.toByteArray(), Base64.URL_SAFE or Base64.NO_WRAP)
            val cookie = Cookie.Builder().name("auto_anime_session").value("origin-a-secret").hostOnlyDomain("a.storage-test.invalid").path("/").build()
            storage().save(originA, listOf(cookie))
            val ciphertext = requireNotNull(prefs.getString(key(originA), null))
            prefs.edit().putString(key(originB), ciphertext).commit()
            val reader = storage()
            reader.load(originB)
            assertTrue(reader.cookies(originB).isEmpty())
        } finally { cleanup() }
    }

    @Test fun appBackupIsDisabledForSessionStorage() {
        val targetContext = instrumentation.targetContext
        val info = targetContext.packageManager.getApplicationInfo(targetContext.packageName, 0)
        assertEquals(0, info.flags and android.content.pm.ApplicationInfo.FLAG_ALLOW_BACKUP)
    }
}
