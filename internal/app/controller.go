package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"

	"github.com/RandomLemon/midori/internal/display"
	"github.com/RandomLemon/midori/internal/ui"
)

const (
	operationTimeout = 5 * time.Second
	pollInterval     = 3 * time.Second
)

type State uint8

const (
	StateLoading State = iota
	StateReady
	StateUnavailable
)

type Controller struct {
	backend display.Backend
	service *display.Service
	view    *ui.Window

	mu       sync.Mutex
	state    State
	snapshot display.Snapshot
	failures int

	inFlight atomic.Bool
	stop     chan struct{}
	stopOnce sync.Once
}

func New(backend display.Backend, service *display.Service, view *ui.Window) *Controller {
	if service == nil {
		service = display.NewService(backend)
	}
	return &Controller{
		backend: backend,
		service: service,
		view:    view,
		state:   StateLoading,
		stop:    make(chan struct{}),
	}
}

func (c *Controller) Start() {
	c.view.SetOnClosed(c.Stop)
	if c.backend != nil {
		c.view.SetBackend(c.backend.DisplayName(), "")
	}
	c.load(true)
	go c.pollLoop()
}

func (c *Controller) Stop() {
	c.stopOnce.Do(func() {
		close(c.stop)
	})
}

func (c *Controller) Refresh() {
	c.mu.Lock()
	state := c.state
	c.mu.Unlock()
	if state != StateReady {
		return
	}
	c.load(false)
}

func (c *Controller) Retry() {
	c.load(true)
}

func (c *Controller) load(showLoading bool) {
	if !c.inFlight.CompareAndSwap(false, true) {
		return
	}
	if showLoading {
		c.view.ShowLoading()
	} else {
		c.view.SetBusy(true)
	}
	go func() {
		defer c.inFlight.Store(false)

		ctx, cancel := context.WithTimeout(context.Background(), operationTimeout)
		snapshot, loadErr := c.service.Load(ctx)
		cancel()
		select {
		case <-c.stop:
			return
		default:
		}

		var version string
		if loadErr == nil && c.backend != nil {
			versionCtx, versionCancel := context.WithTimeout(context.Background(), operationTimeout)
			version, _ = c.backend.Version(versionCtx)
			versionCancel()
		}

		fyne.Do(func() {
			c.applyLoad(snapshot, version, loadErr)
		})
	}()
}

func (c *Controller) applyLoad(snapshot display.Snapshot, version string, err error) {
	c.view.SetBusy(false)
	if err != nil {
		slog.Debug("display load failed", "error", err)
		c.setUnavailable()
		switch {
		case errors.Is(err, display.ErrNoBackend):
			c.view.ShowUnsupported()
		case errors.Is(err, display.ErrConnection):
			c.view.ShowLostConnection()
		default:
			c.view.ShowLostConnection()
		}
		return
	}

	c.mu.Lock()
	c.state = StateReady
	c.snapshot = snapshot
	c.failures = 0
	c.mu.Unlock()

	if c.backend != nil {
		c.view.SetBackend(c.backend.DisplayName(), version)
	}
	slog.Debug("display state loaded", "backend", snapshot.Backend, "outputs", len(snapshot.Outputs), "version", version)
	c.view.SetSnapshot(snapshot)
	c.view.SetReady(true)
}

func (c *Controller) pollLoop() {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			c.poll()
		case <-c.stop:
			return
		}
	}
}

func (c *Controller) poll() {
	if c.backend == nil {
		return
	}
	c.mu.Lock()
	state := c.state
	c.mu.Unlock()
	if state == StateLoading {
		return
	}
	if !c.inFlight.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer c.inFlight.Store(false)
		ctx, cancel := context.WithTimeout(context.Background(), operationTimeout)
		snapshot, err := c.service.Refresh(ctx)
		cancel()
		select {
		case <-c.stop:
			return
		default:
		}
		fyne.Do(func() {
			c.applyPoll(snapshot, err)
		})
	}()
}

func (c *Controller) applyPoll(snapshot display.Snapshot, err error) {
	if err != nil {
		slog.Debug("display poll failed", "error", err)
		c.mu.Lock()
		c.failures++
		failures := c.failures
		state := c.state
		if failures >= 3 {
			c.state = StateUnavailable
		}
		c.mu.Unlock()
		if failures >= 3 && state != StateUnavailable {
			c.view.SetReady(false)
			c.view.ShowLostConnection()
		}
		return
	}

	c.mu.Lock()
	c.failures = 0
	previous := c.snapshot
	wasUnavailable := c.state == StateUnavailable
	c.state = StateReady
	c.snapshot = snapshot
	c.mu.Unlock()

	c.view.SetBusy(false)
	c.view.SetReady(true)
	if wasUnavailable || fingerprint(previous) != fingerprint(snapshot) {
		c.view.SetSnapshot(snapshot)
	}
}

func (c *Controller) setUnavailable() {
	c.mu.Lock()
	c.state = StateUnavailable
	c.mu.Unlock()
}

func fingerprint(snapshot display.Snapshot) string {
	var b strings.Builder
	for _, output := range snapshot.Outputs {
		fmt.Fprintf(&b, "%s|%t|%d|%d|%d|%.6f|%d|%d|%d|%t\n",
			output.ID,
			output.Enabled,
			output.Mode.Width,
			output.Mode.Height,
			output.Mode.RefreshMilliHz,
			output.Scale,
			output.Transform,
			output.Position.X,
			output.Position.Y,
			output.VRR,
		)
	}
	return b.String()
}
