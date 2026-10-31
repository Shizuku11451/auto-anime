package com.autoanime.android.benchmark

import androidx.benchmark.macro.CompilationMode
import androidx.benchmark.macro.FrameTimingMetric
import androidx.benchmark.macro.StartupMode
import androidx.benchmark.macro.junit4.MacrobenchmarkRule
import androidx.test.uiautomator.By
import androidx.test.uiautomator.UiDevice
import androidx.test.uiautomator.Until
import androidx.test.platform.app.InstrumentationRegistry
import org.junit.Rule
import org.junit.Test
import java.util.concurrent.TimeUnit

class LibraryScrollBenchmark {
    @get:Rule val benchmark = MacrobenchmarkRule()

    @Test fun openLibraryAndScrollWhenDemoServerIsPreconfigured() = benchmark.measureRepeated(
        packageName = "com.autoanime.android.benchmark",
        metrics = listOf(FrameTimingMetric()),
        iterations = 20,
        startupMode = StartupMode.WARM,
        compilationMode = CompilationMode.None(),
        setupBlock = { pressHome() },
    ) {
        startActivityAndWait()
        val device = UiDevice.getInstance(InstrumentationRegistry.getInstrumentation())
        check(device.wait(Until.hasObject(By.res("com.autoanime.android.benchmark", "anime_list")), TimeUnit.SECONDS.toMillis(10))) {
            "Configure a temporary demo server with library data before running this benchmark"
        }
        repeat(3) {
            device.swipe(device.displayWidth / 2, device.displayHeight * 3 / 4, device.displayWidth / 2, device.displayHeight / 4, 8)
            device.waitForIdle()
        }
    }
}
