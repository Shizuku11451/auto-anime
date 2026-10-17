# Auto-Anime Rain Patches

Source: github.com/cenkalti/rain v1.13.0, MIT (see LICENSE).
Only the runtime Go packages (`torrent/`, `internal/`) and module manifests are
included. Upstream command-line/UI code and test fixtures are not redistributed.

Local changes:

- Export `ErrNoFreePort` and `HasFreePort` for bounded capacity handling.
- Add serialized-call APIs `UnloadTorrent`, `LoadTorrent`, and `ForgetTorrent`.
  Unload preserves resume state and source files; Forget removes resume state
  but never removes source files. Both release runtime resources and ports.
- Allocate an available configured port when loading resume records. Records
  exceeding runtime capacity remain on disk and are not treated as corrupt.
- Invalidate the completion latch and resume bitfield when source files are
  recreated/resized, so missing payload cannot become a completed zero file.
- Reset the completion notification when verification finds missing pieces.
- Close allocator-owned files when cancellation prevents transferring them to
  the torrent, including successful allocations cancelled just before delivery.
- Detach unloaded torrents from DHT indexes/requests without mutating slices
  still held by concurrent DHT readers.

Regression coverage lives in `../../native_downloader_test.go` and
`../../native_downloader_lifecycle_test.go`, plus the local allocator cancellation
test. Update and review these patches
explicitly when upgrading Rain. Do not edit the Go module cache.
