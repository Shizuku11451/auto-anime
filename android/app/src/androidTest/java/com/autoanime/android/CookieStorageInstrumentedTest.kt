package com.autoanime.android

import android.util.Base64
import android.content.Context
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.autoanime.android.core.EncryptedCookieStorage
import okhttp3.Cookie
import org.junit.Assert.*
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class CookieStorageInstrumentedTest {
    private val instrumentation get() = InstrumentationRegistry.getInstrumentation()
    // Instrumentation context has its own private preferences; don't touch app session data.
    private val context get() = instrumentation.context
    private val prefs get() = context.getSharedPreferences("session_ciphertext", Context.MODE_PRIVATE)

    @Test fun encryptedCookieSurvivesStorageRecreation() {
        prefs.edit().clear().commit()
        try {
            val origin = "https://storage-test.invalid:443|alice"
            val cookie = Cookie.Builder().name("auto_anime_session").value("opaque-test-only")
                .hostOnlyDomain("storage-test.invalid").path("/").build()
            val storage = EncryptedCookieStorage(context)
            storage.save(origin, listOf(cookie))
            assertEquals("save should publish to the live jar immediately", "opaque-test-only", storage.cookies(origin).single().value)
            assertEquals("opaque-test-only", storage.cookies(origin).single().value)
            val restored = EncryptedCookieStorage(context).apply { load(origin) }.cookies(origin)
            assertEquals("opaque-test-only", restored.single().value)
        } finally { prefs.edit().clear().commit() }
    }

    @Test fun malformedStoredCiphertextFailsClosedAndClearsOnlyThatOrigin() {
        prefs.edit().clear().commit()
        try {
            val originA = "https://a.storage-test.invalid:443|alice"
            val originB = "https://b.storage-test.invalid:443|alice"
            fun key(origin: String) = "origin_" + Base64.encodeToString(origin.toByteArray(), Base64.URL_SAFE or Base64.NO_WRAP)
            val a = Cookie.Builder().name("auto_anime_session").value("only-a").hostOnlyDomain("a.storage-test.invalid").path("/").build()
            val b = Cookie.Builder().name("auto_anime_session").value("only-b").hostOnlyDomain("b.storage-test.invalid").path("/").build()
            val writer = EncryptedCookieStorage(context)
            writer.save(originA, listOf(a)); writer.save(originB, listOf(b))
            prefs.edit().putString(key(originA), "broken-ciphertext").commit()
            val reader = EncryptedCookieStorage(context)
            reader.load(originA)
            reader.load(originB)
            assertTrue(reader.cookies(originA).isEmpty())
            assertEquals("only-b", reader.cookies(originB).single().value)
        } finally { prefs.edit().clear().commit() }
    }

    @Test fun copiedCiphertextCannotAuthenticateForAnotherOrigin() {
        prefs.edit().clear().commit()
        try {
            val originA = "https://a.storage-test.invalid:443|alice"
            val originB = "https://b.storage-test.invalid:443|alice"
            fun key(origin: String) = "origin_" + Base64.encodeToString(origin.toByteArray(), Base64.URL_SAFE or Base64.NO_WRAP)
            val cookie = Cookie.Builder().name("auto_anime_session").value("origin-a-secret").hostOnlyDomain("a.storage-test.invalid").path("/").build()
            EncryptedCookieStorage(context).save(originA, listOf(cookie))
            val ciphertext = requireNotNull(prefs.getString(key(originA), null))
            prefs.edit().putString(key(originB), ciphertext).commit()
            val reader = EncryptedCookieStorage(context)
            reader.load(originB)
            assertTrue(reader.cookies(originB).isEmpty())
        } finally { prefs.edit().clear().commit() }
    }

    @Test fun appBackupIsDisabledForSessionStorage() {
        val targetContext = instrumentation.targetContext
        val info = targetContext.packageManager.getApplicationInfo(targetContext.packageName, 0)
        assertEquals(0, info.flags and android.content.pm.ApplicationInfo.FLAG_ALLOW_BACKUP)
    }
}
