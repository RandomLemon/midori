// Package fakebackend provides a scriptable display.Backend for tests.
package fakebackend

import (
	"context"
	"sync"
	"time"

	"github.com/RandomLemon/midori/internal/display"
)

type Call struct {
	Operation string
	Output    display.OutputID
	Change    display.Change
}

type Backend struct {
	mu sync.Mutex

	outputs    []display.Output
	outputsErr error
	caps       display.Capabilities
	version    string
	versionErr error
	delay      time.Duration
	applyErrs  map[display.OutputID]error
	applyOnce  map[display.OutputID]error
	applyFunc  func(display.Change) error
	calls      []Call
	closed     bool
}

func New(outputs ...display.Output) *Backend {
	return &Backend{
		outputs: cloneOutputs(outputs),
		caps:    display.Capabilities{Apply: true, VRR: true},
		version: "test",
	}
}

func (b *Backend) ID() string          { return "fake" }
func (b *Backend) DisplayName() string { return "Fake" }

func (b *Backend) Capabilities() display.Capabilities {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.caps
}

func (b *Backend) Version(ctx context.Context) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if b.versionErr != nil {
		return "", b.versionErr
	}
	return b.version, nil
}

func (b *Backend) Outputs(ctx context.Context) ([]display.Output, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.calls = append(b.calls, Call{Operation: "outputs"})
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if b.delay > 0 {
		select {
		case <-time.After(b.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if b.outputsErr != nil {
		return nil, b.outputsErr
	}
	return cloneOutputs(b.outputs), nil
}

func (b *Backend) Apply(ctx context.Context, change display.Change) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.calls = append(b.calls, Call{Operation: "apply", Output: change.Before.ID, Change: change})
	if err := ctx.Err(); err != nil {
		return err
	}
	if b.delay > 0 {
		select {
		case <-time.After(b.delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if b.applyFunc != nil {
		return b.applyFunc(change)
	}
	if err := b.applyErrs[change.Before.ID]; err != nil {
		return err
	}
	if err := b.applyOnce[change.Before.ID]; err != nil {
		delete(b.applyOnce, change.Before.ID)
		return err
	}

	found := false
	for i := range b.outputs {
		if b.outputs[i].ID != change.Before.ID {
			continue
		}
		found = true
		if change.After.Enabled {
			b.outputs[i].OutputState = change.After
			b.outputs[i].LogicalSize = display.LogicalSize(change.After.Mode, change.After.Scale, change.After.Transform)
		} else {
			b.outputs[i].OutputState = display.OutputState{Enabled: false}
			b.outputs[i].LogicalSize = display.Size{}
		}
		break
	}
	if !found {
		return display.ErrOutputMissing
	}
	return nil
}

func (b *Backend) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	return nil
}

func (b *Backend) SetOutputs(outputs ...display.Output) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.outputs = cloneOutputs(outputs)
}

func (b *Backend) SetOutputsError(err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.outputsErr = err
}

func (b *Backend) SetCapabilities(caps display.Capabilities) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.caps = caps
}

func (b *Backend) SetVersion(version string, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.version = version
	b.versionErr = err
}

func (b *Backend) SetDelay(delay time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.delay = delay
}

func (b *Backend) SetApplyError(id display.OutputID, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.applyErrs == nil {
		b.applyErrs = make(map[display.OutputID]error)
	}
	b.applyErrs[id] = err
}

func (b *Backend) SetApplyErrorOnce(id display.OutputID, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.applyOnce == nil {
		b.applyOnce = make(map[display.OutputID]error)
	}
	b.applyOnce[id] = err
}

func (b *Backend) SetApplyFunc(fn func(display.Change) error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.applyFunc = fn
}

func (b *Backend) Calls() []Call {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]Call(nil), b.calls...)
}

func (b *Backend) Closed() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.closed
}

func cloneOutputs(outputs []display.Output) []display.Output {
	cloned := make([]display.Output, len(outputs))
	for i, output := range outputs {
		cloned[i] = output
		cloned[i].Modes = append([]display.Mode(nil), output.Modes...)
		if output.PhysicalSize != nil {
			size := *output.PhysicalSize
			cloned[i].PhysicalSize = &size
		}
	}
	return cloned
}
