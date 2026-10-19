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
	confirmSeconds   = 15
)

type State uint8

const (
	StateUnavailable State = iota
	StateLoading
	StateReady
	StateDirty
	StateApplying
	StateConfirming
	StateReverting
	StateRecovering
)

type Controller struct {
	backend display.Backend
	service *display.Service
	view    *ui.Window

	mu           sync.Mutex
	state        State
	snapshot     display.Snapshot
	draft        []display.Output
	failures     int
	transaction  *display.Transaction
	countdown    chan struct{}
	applyCancel  context.CancelFunc
	exitAfter    bool
	stateChanged chan State
	confirmAfter int

	inFlight atomic.Bool
	stop     chan struct{}
	stopOnce sync.Once
}

func New(backend display.Backend, service *display.Service, view *ui.Window) *Controller {
	if service == nil {
		service = display.NewService(backend)
	}
	return &Controller{
		backend:      backend,
		service:      service,
		view:         view,
		state:        StateLoading,
		stateChanged: make(chan State, 64),
		confirmAfter: confirmSeconds,
		stop:         make(chan struct{}),
	}
}

func (c *Controller) Start() {
	c.view.SetCloseHandler(c.RequestClose)
	if c.backend != nil {
		c.view.SetBackend(c.backend.DisplayName(), "")
		c.view.SetCapabilities(c.backend.Capabilities())
	}
	c.load(true)
	go c.pollLoop()
}

func (c *Controller) Stop() {
	c.stopOnce.Do(func() {
		close(c.stop)
	})
	c.stopCountdown()
}

func (c *Controller) Refresh() {
	if c.getState() != StateReady {
		return
	}
	c.load(false)
}

func (c *Controller) Retry() {
	c.load(true)
}

func (c *Controller) RequestClose() {
	c.mu.Lock()
	state := c.state
	c.mu.Unlock()

	switch state {
	case StateDirty:
		c.view.ShowDiscardDialog(func() {
			c.Stop()
			c.view.Close()
		})
	case StateConfirming:
		c.setExitAfter(true)
		c.RevertNow()
	case StateApplying:
		c.setExitAfter(true)
		c.cancelApply()
	case StateReverting:
		c.setExitAfter(true)
	case StateRecovering:
		c.view.ShowRecovering()
	default:
		c.Stop()
		c.view.Close()
	}
}

func (c *Controller) load(showLoading bool) {
	if !c.inFlight.CompareAndSwap(false, true) {
		return
	}
	if showLoading {
		c.setState(StateLoading)
		c.view.ShowLoading()
		c.view.SetEditable(false)
		c.view.SetReady(false)
	} else {
		c.view.SetBusy(true)
	}
	go func() {
		defer c.inFlight.Store(false)

		ctx, cancel := context.WithTimeout(context.Background(), operationTimeout)
		snapshot, loadErr := c.service.Load(ctx)
		cancel()

		var version string
		if loadErr == nil && c.backend != nil {
			versionCtx, versionCancel := context.WithTimeout(context.Background(), operationTimeout)
			version, _ = c.backend.Version(versionCtx)
			versionCancel()
		}

		select {
		case <-c.stop:
			return
		default:
		}
		fyne.Do(func() {
			c.applyLoad(snapshot, version, loadErr)
		})
	}()
}

func (c *Controller) applyLoad(snapshot display.Snapshot, version string, err error) {
	switch c.getState() {
	case StateApplying, StateConfirming, StateReverting, StateRecovering:
		return
	}
	c.view.SetBusy(false)
	if err != nil {
		slog.Debug("display load failed", "error", err)
		c.setState(StateUnavailable)
		c.view.SetEditable(false)
		c.view.SetDirty(false)
		c.view.SetValid(false)
		c.view.SetReady(false)
		if errors.Is(err, display.ErrNoBackend) {
			c.view.ShowUnsupported()
		} else {
			c.view.ShowLostConnection()
		}
		return
	}

	c.mu.Lock()
	c.setStateLocked(StateReady)
	c.snapshot = snapshot
	c.draft = c.buildDraft(snapshot)
	c.failures = 0
	draft := cloneOutputs(c.draft)
	c.mu.Unlock()

	slog.Debug("display state loaded", "backend", snapshot.Backend, "outputs", len(snapshot.Outputs), "version", version)
	if c.backend != nil {
		c.view.SetBackend(c.backend.DisplayName(), version)
		c.view.SetCapabilities(c.backend.Capabilities())
	}
	c.view.HideConfirmation()
	c.renderReady(draft, "")
}

func (c *Controller) EditOutput(id display.OutputID, state display.OutputState) {
	c.editOutput(id, state)
}

func (c *Controller) editOutput(id display.OutputID, state display.OutputState) {
	if c.backend == nil {
		return
	}
	current := c.getState()
	if current != StateReady && current != StateDirty {
		return
	}

	c.mu.Lock()
	found := false
	for index := range c.draft {
		if c.draft[index].ID != id {
			continue
		}
		c.draft[index].OutputState = state
		if state.Enabled {
			c.draft[index].LogicalSize = display.LogicalSize(state.Mode, state.Scale, state.Transform)
		} else {
			c.draft[index].LogicalSize = display.Size{}
		}
		found = true
		break
	}
	if !found {
		c.mu.Unlock()
		return
	}
	draft := cloneOutputs(c.draft)
	snapshot := c.snapshot
	c.mu.Unlock()

	err := display.ValidateDraft(c.backend.Capabilities(), snapshot, draft)
	dirty := c.isDirty(draft, snapshot)
	if dirty {
		c.setState(StateDirty)
	} else {
		c.setState(StateReady)
	}

	c.view.SetEditable(true)
	c.view.SetDirty(dirty)
	c.view.SetValid(err == nil)
	c.view.SetValidation(err)
	c.view.SetBusy(false)
	c.view.SetReady(true)
	if dirty {
		c.view.SetStatus("Unsaved changes")
	} else {
		c.view.SetStatus("Session-only changes")
	}
}

func (c *Controller) Reset() {
	state := c.getState()
	if state != StateReady && state != StateDirty {
		return
	}
	c.mu.Lock()
	c.draft = c.buildDraft(c.snapshot)
	c.setStateLocked(StateReady)
	draft := cloneOutputs(c.draft)
	c.mu.Unlock()

	c.renderReady(draft, "")
}

func (c *Controller) Apply() {
	state := c.getState()
	if state != StateReady && state != StateDirty {
		return
	}
	if c.backend == nil {
		return
	}

	c.mu.Lock()
	targets := make(map[display.OutputID]display.OutputState)
	for _, output := range c.draft {
		before, ok := c.snapshot.Output(output.ID)
		if !ok || !display.StatesEqual(before.OutputState, output.OutputState) {
			targets[output.ID] = output.OutputState
		}
	}
	snapshot := c.snapshot
	draft := cloneOutputs(c.draft)
	c.mu.Unlock()

	if err := display.ValidateDraft(c.backend.Capabilities(), snapshot, draft); err != nil {
		c.view.SetValidation(err)
		c.view.SetValid(false)
		return
	}
	if len(targets) == 0 {
		c.view.SetValidation(errors.New("No changes to apply."))
		c.view.SetValid(false)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	c.mu.Lock()
	c.applyCancel = cancel
	c.setStateLocked(StateApplying)
	c.mu.Unlock()

	c.view.SetEditable(false)
	c.view.SetBusy(true)
	c.view.SetDirty(false)
	c.view.SetValid(false)
	c.view.SetValidation(nil)
	c.view.SetReady(false)
	c.view.SetStatus("Applying...")
	c.view.ClearBanner()

	go func() {
		preflightCtx, preflightCancel := context.WithTimeout(ctx, operationTimeout)
		fresh, refreshErr := c.service.Refresh(preflightCtx)
		preflightCancel()
		if refreshErr != nil {
			fyne.Do(func() {
				c.finishPreflight(refreshErr)
			})
			return
		}
		if fingerprint(fresh) != fingerprint(snapshot) {
			fyne.Do(func() {
				c.applyConflict(fresh)
			})
			return
		}
		tx, applyErr := c.service.Apply(ctx, targets)
		var freshAfter display.Snapshot
		var reloadErr error
		if applyErr != nil && !display.IsRollbackFailure(applyErr) {
			freshAfter, reloadErr = c.loadSnapshot()
		}
		fyne.Do(func() {
			c.finishApply(tx, applyErr, freshAfter, reloadErr)
		})
	}()
}

func (c *Controller) applyConflict(fresh display.Snapshot) {
	c.clearApplyCancel()
	c.mu.Lock()
	c.snapshot = fresh
	c.draft = c.buildDraft(fresh)
	c.setStateLocked(StateReady)
	draft := cloneOutputs(c.draft)
	exit := c.exitAfter
	c.mu.Unlock()

	if exit {
		c.finishExit()
		return
	}
	c.renderReady(draft, "Display configuration changed. Review the settings and apply again.")
}

func (c *Controller) finishPreflight(err error) {
	c.clearApplyCancel()
	slog.Debug("display apply preflight failed", "error", err)
	c.mu.Lock()
	draft := cloneOutputs(c.draft)
	snapshot := c.snapshot
	c.setStateLocked(StateDirty)
	exit := c.exitAfter
	c.mu.Unlock()
	if exit {
		c.finishExit()
		return
	}
	validation := display.ValidateDraft(c.backend.Capabilities(), snapshot, draft)
	c.view.SetOutputs(draft)
	c.view.SetEditable(true)
	c.view.SetDirty(true)
	c.view.SetValid(validation == nil)
	c.view.SetValidation(validation)
	c.view.SetBusy(false)
	c.view.SetReady(true)
	c.view.SetStatus("Unsaved changes")
	c.view.ShowBanner("Could not apply display configuration. Review the settings and try again.")
}

func (c *Controller) finishApply(tx *display.Transaction, err error, fresh display.Snapshot, loadErr error) {
	c.clearApplyCancel()
	if err != nil {
		slog.Debug("display apply failed", "error", err)
		if display.IsRollbackFailure(err) || loadErr != nil {
			c.recovering()
			return
		}
		c.mu.Lock()
		c.snapshot = fresh
		c.draft = c.mergeDraft(fresh, c.draft)
		c.setStateLocked(StateDirty)
		draft := cloneOutputs(c.draft)
		exit := c.exitAfter
		c.mu.Unlock()
		if exit {
			c.finishExit()
			return
		}
		validation := display.ValidateDraft(c.backend.Capabilities(), fresh, draft)
		c.view.SetOutputs(draft)
		c.view.SetEditable(true)
		c.view.SetDirty(true)
		c.view.SetValid(validation == nil)
		c.view.SetValidation(validation)
		c.view.SetBusy(false)
		c.view.SetReady(true)
		c.view.SetStatus("Unsaved changes")
		c.view.ShowBanner("Could not apply display configuration\n\nThe previous configuration was restored.")
		return
	}

	c.mu.Lock()
	c.transaction = tx
	c.draft = cloneOutputs(tx.After.Outputs)
	exit := c.exitAfter
	c.mu.Unlock()

	c.view.SetOutputs(tx.After.Outputs)
	c.view.SetEditable(false)
	c.view.SetDirty(false)
	c.view.SetValid(false)
	c.view.SetBusy(false)
	c.view.SetReady(false)
	c.view.SetStatus("Confirm changes")
	c.view.ShowConfirmation(c.confirmAfter)
	c.mu.Lock()
	c.setStateLocked(StateConfirming)
	c.mu.Unlock()
	if exit {
		c.RevertNow()
		return
	}
	c.startCountdown()
}

func (c *Controller) Keep() {
	c.mu.Lock()
	if c.state != StateConfirming || c.transaction == nil {
		c.mu.Unlock()
		return
	}
	tx := c.transaction
	c.transaction = nil
	c.snapshot = tx.After
	c.draft = c.buildDraft(tx.After)
	draft := cloneOutputs(c.draft)
	c.mu.Unlock()

	c.stopCountdown()
	c.view.HideConfirmation()
	c.renderReady(draft, "")
	c.mu.Lock()
	c.setStateLocked(StateReady)
	c.mu.Unlock()
}

func (c *Controller) RevertNow() {
	c.mu.Lock()
	if c.state != StateConfirming || c.transaction == nil {
		c.mu.Unlock()
		return
	}
	c.setStateLocked(StateReverting)
	tx := c.transaction
	countdown := c.countdown
	c.countdown = nil
	c.mu.Unlock()

	if countdown != nil {
		close(countdown)
	}
	c.view.ShowReverting()
	c.view.SetEditable(false)
	c.view.SetBusy(true)
	c.view.SetReady(false)
	c.view.SetStatus("Reverting...")

	go func() {
		revertErr := c.service.Revert(context.Background(), tx.Before)
		if revertErr != nil {
			fyne.Do(func() {
				c.recovering()
			})
			return
		}
		fresh, loadErr := c.loadSnapshot()
		fyne.Do(func() {
			c.finishRevert(fresh, loadErr)
		})
	}()
}

func (c *Controller) finishRevert(fresh display.Snapshot, err error) {
	if err != nil {
		c.recovering()
		return
	}
	c.mu.Lock()
	c.snapshot = fresh
	c.draft = c.buildDraft(fresh)
	c.transaction = nil
	draft := cloneOutputs(c.draft)
	exit := c.exitAfter
	c.mu.Unlock()

	if exit {
		c.mu.Lock()
		c.setStateLocked(StateReady)
		c.mu.Unlock()
		c.finishExit()
		return
	}
	c.view.HideConfirmation()
	c.renderReady(draft, "")
	c.mu.Lock()
	c.setStateLocked(StateReady)
	c.mu.Unlock()
}

func (c *Controller) recovering() {
	c.mu.Lock()
	c.setStateLocked(StateRecovering)
	c.exitAfter = false
	c.transaction = nil
	c.applyCancel = nil
	countdown := c.countdown
	c.countdown = nil
	c.mu.Unlock()
	if countdown != nil {
		close(countdown)
	}

	c.view.SetEditable(false)
	c.view.SetDirty(false)
	c.view.SetValid(false)
	c.view.SetBusy(false)
	c.view.SetReady(false)
	c.view.SetStatus("Recovery required")
	c.view.ShowRecovering()
}

func (c *Controller) startCountdown() {
	countdown := make(chan struct{})
	c.mu.Lock()
	c.countdown = countdown
	c.mu.Unlock()

	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for remaining := c.confirmAfter - 1; remaining >= 0; remaining-- {
			select {
			case <-ticker.C:
			case <-countdown:
				return
			case <-c.stop:
				return
			}
			seconds := remaining
			fyne.Do(func() {
				c.view.SetCountdown(seconds)
			})
			if remaining == 0 {
				fyne.Do(func() {
					c.RevertNow()
				})
				return
			}
		}
	}()
}

func (c *Controller) stopCountdown() {
	c.mu.Lock()
	countdown := c.countdown
	c.countdown = nil
	c.mu.Unlock()
	if countdown != nil {
		close(countdown)
	}
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
	switch c.getState() {
	case StateLoading, StateApplying, StateConfirming, StateReverting, StateRecovering:
		return
	}
	if !c.inFlight.CompareAndSwap(false, true) {
		return
	}
	c.mu.Lock()
	snapshot := c.snapshot
	c.mu.Unlock()
	go func() {
		defer c.inFlight.Store(false)
		ctx, cancel := context.WithTimeout(context.Background(), operationTimeout)
		fresh, err := c.service.Refresh(ctx)
		cancel()
		select {
		case <-c.stop:
			return
		default:
		}
		fyne.Do(func() {
			c.applyPoll(snapshot, fresh, err)
		})
	}()
}

func (c *Controller) applyPoll(previous display.Snapshot, fresh display.Snapshot, err error) {
	switch c.getState() {
	case StateLoading, StateApplying, StateConfirming, StateReverting, StateRecovering:
		return
	}
	if err != nil {
		c.mu.Lock()
		c.failures++
		failures := c.failures
		state := c.state
		if failures >= 3 {
			c.setStateLocked(StateUnavailable)
		}
		c.mu.Unlock()
		if failures >= 3 && state != StateUnavailable {
			slog.Debug("display poll failed", "error", err)
			c.view.SetEditable(false)
			c.view.SetDirty(false)
			c.view.SetReady(false)
			c.view.SetBusy(false)
			c.view.SetStatus("Lost connection")
			c.view.ShowLostConnection()
		}
		return
	}

	c.mu.Lock()
	c.failures = 0
	wasUnavailable := c.state == StateUnavailable
	wasDirty := c.state == StateDirty
	changed := fingerprint(previous) != fingerprint(fresh)
	if !changed && !wasUnavailable {
		c.mu.Unlock()
		c.view.SetBusy(false)
		c.view.SetReady(true)
		return
	}
	c.setStateLocked(StateReady)
	c.snapshot = fresh
	c.draft = c.buildDraft(fresh)
	draft := cloneOutputs(c.draft)
	c.mu.Unlock()

	banner := ""
	if wasDirty {
		banner = "Display configuration changed. Pending edits were discarded."
	}
	c.renderReady(draft, banner)
}

func (c *Controller) loadSnapshot() (display.Snapshot, error) {
	ctx, cancel := context.WithTimeout(context.Background(), operationTimeout)
	defer cancel()
	return c.service.Load(ctx)
}

func (c *Controller) buildDraft(snapshot display.Snapshot) []display.Output {
	draft := cloneOutputs(snapshot.Outputs)
	right := 0
	hasOutput := false
	for _, output := range draft {
		if !output.Enabled {
			continue
		}
		rect := display.StateRect(output.OutputState)
		if end := rect.X + rect.Width; end > right {
			right = end
		}
		hasOutput = true
	}
	for index := range draft {
		if draft[index].Enabled {
			continue
		}
		mode := defaultMode(draft[index].Modes)
		size := display.LogicalSize(mode, 1, display.TransformNormal)
		x := 0
		if hasOutput {
			x = right
		}
		draft[index].OutputState = display.OutputState{
			Enabled:   false,
			Mode:      mode,
			Scale:     1,
			Transform: display.TransformNormal,
			Position:  display.Point{X: x, Y: 0},
		}
		right = x + size.Width
		hasOutput = true
	}
	return draft
}

func (c *Controller) mergeDraft(fresh display.Snapshot, previous []display.Output) []display.Output {
	draft := c.buildDraft(fresh)
	old := make(map[display.OutputID]display.Output, len(previous))
	for _, output := range previous {
		old[output.ID] = output
	}
	for index := range draft {
		if output, ok := old[draft[index].ID]; ok {
			draft[index].OutputState = output.OutputState
			if output.Enabled {
				draft[index].LogicalSize = display.LogicalSize(output.Mode, output.Scale, output.Transform)
			} else {
				draft[index].LogicalSize = display.Size{}
			}
		}
	}
	return draft
}

func (c *Controller) isDirty(draft []display.Output, snapshot display.Snapshot) bool {
	if len(draft) != len(snapshot.Outputs) {
		return true
	}
	for _, output := range draft {
		before, ok := snapshot.Output(output.ID)
		if !ok || !display.StatesEqual(before.OutputState, output.OutputState) {
			return true
		}
	}
	return false
}

func (c *Controller) setState(state State) {
	c.mu.Lock()
	c.setStateLocked(state)
	c.mu.Unlock()
}

func (c *Controller) setStateLocked(state State) {
	c.state = state
	select {
	case c.stateChanged <- state:
	default:
	}
}

func (c *Controller) getState() State {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state
}

func (c *Controller) setExitAfter(exit bool) {
	c.mu.Lock()
	c.exitAfter = exit
	c.mu.Unlock()
}

func (c *Controller) clearApplyCancel() {
	c.mu.Lock()
	if c.applyCancel != nil {
		c.applyCancel()
		c.applyCancel = nil
	}
	c.mu.Unlock()
}

func (c *Controller) cancelApply() {
	c.mu.Lock()
	if c.applyCancel != nil {
		c.applyCancel()
	}
	c.mu.Unlock()
}

func (c *Controller) finishExit() {
	c.mu.Lock()
	exit := c.exitAfter
	c.exitAfter = false
	c.mu.Unlock()
	if !exit {
		return
	}
	c.Stop()
	c.view.Close()
}

func defaultMode(modes []display.Mode) display.Mode {
	if len(modes) == 0 {
		return display.Mode{}
	}
	var best display.Mode
	found := false
	for _, mode := range modes {
		if mode.Preferred {
			return mode
		}
		if !found || mode.RefreshMilliHz > best.RefreshMilliHz {
			best = mode
			found = true
		}
	}
	return best
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

func cloneOutputs(outputs []display.Output) []display.Output {
	cloned := make([]display.Output, len(outputs))
	for index, output := range outputs {
		cloned[index] = output
		cloned[index].Modes = append([]display.Mode(nil), output.Modes...)
		if output.PhysicalSize != nil {
			size := *output.PhysicalSize
			cloned[index].PhysicalSize = &size
		}
	}
	return cloned
}

func (c *Controller) renderReady(draft []display.Output, banner string) {
	c.view.SetOutputs(draft)
	c.view.SetEditable(true)
	c.view.SetDirty(false)
	c.view.SetValid(true)
	c.view.SetValidation(nil)
	c.view.SetBusy(false)
	c.view.SetReady(true)
	if len(draft) == 0 {
		c.view.SetStatus("No displays detected")
		c.view.ShowBanner("No displays detected")
		return
	}
	c.view.SetStatus("Session-only changes")
	if banner != "" {
		c.view.ShowBanner(banner)
	} else {
		c.view.ClearBanner()
	}
}
