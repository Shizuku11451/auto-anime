# Android macrobenchmark

The `:benchmark` instrumentation module contains cold-start and library-scroll timing scenarios using `StartupTimingMetric`/`FrameTimingMetric`. Build the isolated, signed test APK with `./gradlew :benchmark:assembleBenchmark`; run on a physical device with:

```sh
./gradlew :benchmark:connectedCheck
```

The test APK is compiled but has not been run: no Android device is available. M1 performance acceptance remains blocked on device evidence. The scroll scenario needs fixed demo-server data; before recording results, include device/API/refresh-rate/ABI, build variant, startup mode, sample count, P50/P95 and frame outliers. Do not claim performance acceptance from compilation.
