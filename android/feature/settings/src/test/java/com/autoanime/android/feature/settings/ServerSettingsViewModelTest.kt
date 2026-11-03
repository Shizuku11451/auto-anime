package com.autoanime.android.feature.settings

import com.autoanime.android.core.model.ServerSettings
import com.autoanime.android.core.model.SettingsInput
import com.autoanime.android.core.model.SettingsSaveResult
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

@OptIn(ExperimentalCoroutinesApi::class)
class ServerSettingsViewModelTest {
    private val dispatcher = StandardTestDispatcher()
    private val scope = TestScope(dispatcher)
    private val api = FakeSettingsApi()

    @Before fun setUp() = Dispatchers.setMain(dispatcher)
    @After fun tearDown() = Dispatchers.resetMain()

    @Test fun onlyChangedFieldsAreSent() = scope.runTest {
        val model = ServerSettingsViewModel(connectedSessions(dispatcher, api), dispatcher)
        runCurrent()
        assertFalse(model.state.value.changed)
        model.update { it.copy(pollMinutes = "60", keepSourceFiles = false) }
        assertTrue(model.state.value.changed)
        model.save()
        runCurrent()
        assertEquals(listOf(SettingsInput(poll_minutes = 60, keep_source_files = false)), api.saved)
    }

    @Test fun outOfRangeValuesAreNotSent() = scope.runTest {
        val model = ServerSettingsViewModel(connectedSessions(dispatcher, api), dispatcher)
        runCurrent()
        model.update { it.copy(pollMinutes = "4") }
        model.save()
        model.update { it.copy(pollMinutes = "30", maxConcurrent = "33") }
        model.save()
        runCurrent()
        assertEquals(ServerField.MaxConcurrent, model.state.value.invalid)
        assertTrue(api.saved.isEmpty())
    }

    @Test fun reloadsAfterSavingSoTheFormMatchesTheServer() = scope.runTest {
        val model = ServerSettingsViewModel(connectedSessions(dispatcher, api), dispatcher)
        runCurrent()
        model.update { it.copy(maxConcurrent = "4") }
        model.save()
        runCurrent()
        assertEquals(2, api.reads)
        assertEquals("4", model.state.value.form.maxConcurrent)
        assertFalse(model.state.value.changed)
    }
}

private class FakeSettingsApi : BaseFakeApi() {
    var current = ServerSettings(max_concurrent_downloads = 2, keep_source_files = true, poll_minutes = 30, library_dir = "/media")
    val saved = mutableListOf<SettingsInput>()
    var reads = 0
    override suspend fun settings(): ServerSettings {
        reads++
        return current
    }
    override suspend fun saveSettings(input: SettingsInput): SettingsSaveResult {
        saved += input
        current = current.copy(
            max_concurrent_downloads = input.max_concurrent_downloads ?: current.max_concurrent_downloads,
            poll_minutes = input.poll_minutes ?: current.poll_minutes,
            keep_source_files = input.keep_source_files ?: current.keep_source_files,
        )
        return SettingsSaveResult()
    }
}
