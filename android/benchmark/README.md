# Android macrobenchmark

The `:benchmark` instrumentation module contains cold-start and library-scroll timing scenarios using `StartupTimingMetric`/`FrameTimingMetric`. Build the isolated, signed test APK with `./gradlew :benchmark:assembleBenchmark`; run on a physical device with:

```sh
./gradlew :benchmark:connectedCheck
```

The benchmark module uses AGP's experimental self-instrumenting mode (`android.experimental.self-instrumenting=true`) so Macrobenchmark runs with correctly instrumented code. Keep the target app's dedicated benchmark build minified/profileable; do not apply broad cross-APK dependency keep rules. Its R8 config narrowly ignores the JDK-only `javax.lang.model.element.Modifier` reference from Error Prone/J2ObjC annotation metadata; annotation processing APIs are not loaded at runtime. On the dedicated emulator, a 30-iteration cold-start connect-screen run passed (`OK (1 test)`): time-to-full-display median 538.81 ms, P95 673.16 ms; frame-overrun P95 386.55 ms. Trace output: `C:/aa-tmp/startup30-fixed-instrumentation.log`, with JSON metrics in the accompanying `startup30-traces` output. These emulator-only observations predate the final WindowInsets change; they are not a final baseline or performance acceptance. Do not run Gradle `connected*` tasks in a multi-device environment: they may select unintended transports. Install/run only against the explicitly selected dedicated benchmark emulator/device and record its serial. M1 performance acceptance remains open. The scroll scenario needs fixed demo-server data; report device/API/refresh-rate/ABI, build variant, startup mode, sample count, P50/P95 and frame outliers. Do not claim performance acceptance from compilation.
