package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

const embeddedQbitRestoreGrace = 90 * time.Second

type embeddedQBittorrentManager struct {
	sync.Mutex
	process   *managedQBittorrent
	stopping  bool
	requested bool
	startDone chan struct{}
	cancel    context.CancelFunc
	failures  int
	retryAt   time.Time
}

func (p *managedQBittorrent) alive() bool {
	if p == nil {
		return false
	}
	select {
	case <-p.done:
		return false
	default:
		return true
	}
}

func embeddedQbitRetryDelay(failures int) time.Duration {
	return min(2*time.Second*time.Duration(1<<min(max(failures-1, 0), 5)), time.Minute)
}

// All exits, including a clean but unsolicited exit, use the same backoff.
// Reset only after sustained uptime, not merely a successful WebUI login.
func (m *embeddedQBittorrentManager) observeExit(now time.Time) {
	p := m.process
	if p == nil || p.alive() || !p.ready.Swap(false) || m.stopping {
		return
	}
	if now.Sub(p.readyAt) >= 5*time.Minute {
		m.failures = 0
	}
	m.failed(now)
	log.Printf("内置 qBittorrent 已退出，将在 %s 后自动恢复", embeddedQbitRetryDelay(m.failures))
}

func (m *embeddedQBittorrentManager) failed(now time.Time) {
	m.failures = min(m.failures+1, 7)
	m.retryAt = now.Add(embeddedQbitRetryDelay(m.failures))
}

func (m *embeddedQBittorrentManager) connection(launch func(context.Context) (*managedQBittorrent, error)) (*managedQBittorrent, error) {
	m.Lock()
	if m.stopping {
		m.Unlock()
		return nil, fmt.Errorf("内置 qBittorrent 正在关闭")
	}
	m.observeExit(time.Now())
	if m.startDone != nil || time.Now().Before(m.retryAt) {
		m.Unlock()
		return nil, fmt.Errorf("内置 qBittorrent 正在启动或恢复，请稍后重试")
	}
	if p := m.process; p.alive() {
		m.Unlock()
		return p, nil
	}
	m.requested = true
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	done := make(chan struct{})
	m.startDone, m.cancel = done, cancel
	m.Unlock()

	p, err := launch(ctx)
	if err == nil {
		err = ctx.Err()
	}
	cancel()
	m.Lock()
	if m.stopping && err == nil {
		err = fmt.Errorf("内置 qBittorrent 正在关闭")
	}
	if err == nil && !p.alive() {
		err = fmt.Errorf("内置 qBittorrent 在启动后退出")
	}
	if err != nil {
		// Keep startDone open until the process has been reaped. A concurrent
		// stop must not return while a startup process can still own the profile.
		m.Unlock()
		if p != nil {
			p.shutdown()
		}
		m.Lock()
		if !m.stopping {
			m.failed(time.Now())
		}
	} else {
		p.readyAt = time.Now()
		p.ready.Store(true)
		m.process = p
		m.retryAt = time.Time{}
		log.Println("内置 qBittorrent 已就绪，将继续跟踪原任务")
		wakeTaskPoller()
	}
	m.startDone, m.cancel = nil, nil
	close(done)
	m.Unlock()
	return p, err
}

func (m *embeddedQBittorrentManager) supervise(ctx context.Context, interval time.Duration, launch func(context.Context) (*managedQBittorrent, error)) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		m.Lock()
		m.observeExit(time.Now())
		retry := m.requested && !m.stopping && m.startDone == nil && !m.process.alive() && !time.Now().Before(m.retryAt)
		m.Unlock()
		if retry {
			if _, err := m.connection(launch); err != nil && ctx.Err() == nil {
				log.Printf("内置 qBittorrent 恢复失败，将继续退避重试: %v", err)
			}
		}
	}
}

func (m *embeddedQBittorrentManager) stop() {
	m.Lock()
	m.stopping = true
	if m.cancel != nil {
		m.cancel()
	}
	done := m.startDone
	m.Unlock()
	if done != nil {
		<-done
	}
	m.Lock()
	p := m.process
	m.Unlock()
	if p != nil {
		p.shutdown()
	}
}

func (m *embeddedQBittorrentManager) healthy() bool {
	m.Lock()
	defer m.Unlock()
	return !m.stopping && (m.process.alive() && m.process.ready.Load() || !m.requested)
}
