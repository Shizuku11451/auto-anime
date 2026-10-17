package allocator

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/cenkalti/rain/internal/metainfo"
	"github.com/cenkalti/rain/internal/storage"
)

type cancellationFile struct {
	closed atomic.Bool
}

func (*cancellationFile) ReadAt(p []byte, _ int64) (int, error)  { return len(p), nil }
func (*cancellationFile) WriteAt(p []byte, _ int64) (int, error) { return len(p), nil }
func (f *cancellationFile) Close() error {
	f.closed.Store(true)
	return nil
}

type cancellationStorage struct{ file *cancellationFile }

func (s cancellationStorage) Open(string, int64) (storage.File, bool, error) {
	return s.file, false, nil
}
func (cancellationStorage) RootDir() string { return "" }

func TestCancelBeforeResultClosesAllocatedFile(t *testing.T) {
	a := New()
	f := &cancellationFile{}
	progress := make(chan Progress)
	info := &metainfo.Info{Files: []metainfo.File{{Path: "episode.mkv", Length: 1024}}}
	go a.Run(info, cancellationStorage{f}, progress, make(chan *Allocator))
	select {
	case <-progress:
	case <-time.After(5 * time.Second):
		t.Fatal("allocator did not open file")
	}
	a.Close()
	if !f.closed.Load() {
		t.Fatal("cancelled allocation leaked an open file")
	}
}
