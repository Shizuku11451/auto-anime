package com.autoanime.android.benchmark

import androidx.benchmark.macro.CompilationMode
import androidx.benchmark.macro.FrameTimingMetric
import androidx.benchmark.macro.junit4.MacrobenchmarkRule
import androidx.benchmark.macro.StartupTimingMetric
import androidx.benchmark.macro.StartupMode
import org.junit.Rule
import org.junit.Test

class StartupBenchmark {
    @get:Rule val benchmark = MacrobenchmarkRule()

    @Test fun coldStartConnectScreen() = benchmark.measureRepeated(
        packageName = "com.autoanime.android.benchmark",
        metrics = listOf(StartupTimingMetric(), FrameTimingMetric()),
        iterations = 30,
        startupMode = StartupMode.COLD,
        compilationMode = CompilationMode.None(),
        setupBlock = { pressHome() },
    ) { startActivityAndWait() }
}
