# Android macrobenchmark

The `:benchmark` instrumentation module contains a cold-start timing test for the Connect screen using `StartupTimingMetric` and `FrameTimingMetric`. Build and run on a physical device with:

```sh
./gradlew :benchmark:connectedCheck
```

No measurements have been collected: this environment has no Android device. M1 performance acceptance remains blocked on device evidence. Before recording results, include device/API/refresh-rate/ABI, build variant, startup mode, sample count, P50/P95 and frame outliers. Playback and library-scroll scenarios are not yet implemented; do not claim their performance acceptance.
