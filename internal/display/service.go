package display

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"sync"
	"time"
)

type Transaction struct {
	Before  Snapshot
	After   Snapshot
	Changed []OutputID
}

// Service is the UI-independent display service. It serializes apply and revert
// transactions and never imports Fyne.
type Service struct {
	backend Backend

	mu   sync.Mutex
	busy bool
}

func NewService(backend Backend) *Service {
	return &Service{backend: backend}
}

// Load reads the current compositor state.
func (s *Service) Load(ctx context.Context) (Snapshot, error) {
	if s.backend == nil {
		return Snapshot{}, ErrNoBackend
	}
	outputs, err := s.backend.Outputs(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	sort.Slice(outputs, func(i, j int) bool {
		return outputs[i].Name < outputs[j].Name
	})
	return Snapshot{
		Backend:    s.backend.ID(),
		CapturedAt: time.Now().UTC(),
		Outputs:    outputs,
	}, nil
}

// Refresh re-reads the current compositor state.
func (s *Service) Refresh(ctx context.Context) (Snapshot, error) {
	return s.Load(ctx)
}

// Apply validates and applies the target states, reloads, and returns a
// transaction. On failure it attempts to roll back every attempted output.
func (s *Service) Apply(ctx context.Context, targets map[OutputID]OutputState) (*Transaction, error) {
	if err := s.begin(); err != nil {
		return nil, err
	}
	defer s.end()

	if s.backend == nil {
		return nil, ErrNoBackend
	}
	if !s.backend.Capabilities().Apply {
		return nil, fmt.Errorf("display: apply: %w", ErrUnsupported)
	}
	if len(targets) == 0 {
		return nil, invalid("", "targets", "no outputs selected")
	}

	before, err := s.Load(ctx)
	if err != nil {
		return nil, fmt.Errorf("display: read state before apply: %w", err)
	}

	desired := make(map[OutputID]Output, len(before.Outputs))
	for _, output := range before.Outputs {
		desired[output.ID] = output
	}
	for id, state := range targets {
		output, ok := desired[id]
		if !ok {
			return nil, fmt.Errorf("display: apply %s: %w", id, ErrOutputMissing)
		}
		output.OutputState = state
		desired[id] = output
	}

	if err := validate(s.backend.Capabilities(), desired, targets, before); err != nil {
		return nil, err
	}

	changes := make([]Change, 0, len(targets))
	for id := range targets {
		current := outputByID(before, id)
		if current == nil {
			return nil, fmt.Errorf("display: apply %s: %w", id, ErrOutputMissing)
		}
		if !stateEqual(current.OutputState, targets[id]) {
			changes = append(changes, Change{Before: *current, After: targets[id]})
		}
	}
	sort.Slice(changes, func(i, j int) bool {
		return changes[i].Before.ID < changes[j].Before.ID
	})
	if len(changes) == 0 {
		return nil, invalid("", "targets", "no changes to apply")
	}

	attempted := make([]Change, 0, len(changes))
	for _, change := range changes {
		attempted = append(attempted, change)
		if err := s.applyChange(ctx, change); err != nil {
			applyErr := fmt.Errorf("display: apply %s: %w", change.Before.ID, err)
			if rollbackErr := s.rollback(ctx, attempted, desired); rollbackErr != nil {
				return nil, &ApplyError{Err: applyErr, RollbackErr: rollbackErr}
			}
			return nil, applyErr
		}
	}

	after, err := s.Load(ctx)
	if err != nil {
		reloadErr := fmt.Errorf("display: reload after apply: %w", err)
		if rollbackErr := s.rollback(ctx, attempted, desired); rollbackErr != nil {
			return nil, &ApplyError{Err: reloadErr, RollbackErr: rollbackErr}
		}
		return nil, reloadErr
	}

	changed := make([]OutputID, 0, len(changes))
	for _, change := range changes {
		changed = append(changed, change.Before.ID)
	}
	return &Transaction{Before: before, After: after, Changed: changed}, nil
}

// Revert restores the states in before for every output that is still present
// and differs from the current state.
func (s *Service) Revert(ctx context.Context, before Snapshot) error {
	if err := s.begin(); err != nil {
		return err
	}
	defer s.end()

	if s.backend == nil {
		return ErrNoBackend
	}
	if !s.backend.Capabilities().Apply {
		return fmt.Errorf("display: revert: %w", ErrUnsupported)
	}

	current, err := s.Load(ctx)
	if err != nil {
		return fmt.Errorf("display: read state before revert: %w", err)
	}

	changes := make([]Change, 0, len(before.Outputs))
	for _, previous := range before.Outputs {
		now, ok := current.Output(previous.ID)
		if !ok {
			slog.Debug("display: skip revert for missing output", "output", previous.ID)
			continue
		}
		if stateEqual(now.OutputState, previous.OutputState) {
			continue
		}
		changes = append(changes, Change{Before: now, After: previous.OutputState})
	}
	sort.Slice(changes, func(i, j int) bool {
		return changes[i].Before.ID < changes[j].Before.ID
	})

	for _, change := range changes {
		if err := s.applyChange(ctx, change); err != nil {
			return fmt.Errorf("display: revert %s: %w", change.Before.ID, err)
		}
	}
	if _, err := s.Load(ctx); err != nil {
		return fmt.Errorf("display: reload after revert: %w", err)
	}
	return nil
}

func (s *Service) begin() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.busy {
		return errors.New("display: a transaction is already running")
	}
	s.busy = true
	return nil
}

func (s *Service) end() {
	s.mu.Lock()
	s.busy = false
	s.mu.Unlock()
}

func (s *Service) rollback(ctx context.Context, attempted []Change, desired map[OutputID]Output) error {
	var errs []error
	for i := len(attempted) - 1; i >= 0; i-- {
		change := attempted[i]
		now, ok := desired[change.Before.ID]
		if !ok {
			continue
		}
		rb := Change{
			Before: outputWithState(change.Before, now.OutputState),
			After:  change.Before.OutputState,
		}
		if err := s.applyChange(ctx, rb); err != nil {
			if errors.Is(err, ErrOutputMissing) {
				slog.Debug("display: skip rollback for missing output", "output", change.Before.ID)
				continue
			}
			errs = append(errs, fmt.Errorf("rollback %s: %w", change.Before.ID, err))
		}
	}
	return errors.Join(errs...)
}

func validate(caps Capabilities, desired map[OutputID]Output, targets map[OutputID]OutputState, current Snapshot) error {
	enabled := 0
	for _, output := range desired {
		if output.Enabled {
			enabled++
		}
	}
	if enabled == 0 {
		return invalid("", "enabled", "at least one display must remain enabled")
	}

	for id, state := range targets {
		output, ok := desired[id]
		if !ok {
			return fmt.Errorf("display: validate %s: %w", id, ErrOutputMissing)
		}
		if !state.Enabled {
			continue
		}
		if state.Mode.Width <= 0 || state.Mode.Height <= 0 || state.Mode.RefreshMilliHz == 0 {
			return invalid(id, "mode", "select an available mode")
		}
		if !modeAllowed(output, state.Mode, current) {
			return invalid(id, "mode", "select an available mode")
		}
		if math.IsNaN(state.Scale) || math.IsInf(state.Scale, 0) || state.Scale < 0.5 || state.Scale > 3.0 {
			return invalid(id, "scale", "scale must be between 50%% and 300%%")
		}
		if state.VRR && (!caps.VRR || !output.VRRSupported) {
			return invalid(id, "vrr", "variable refresh rate is not supported by this output")
		}
	}

	ids := make([]OutputID, 0, len(desired))
	for id := range desired {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for i := range ids {
		a := desired[ids[i]]
		if !a.Enabled {
			continue
		}
		ar := StateRect(a.OutputState)
		for _, otherID := range ids[i+1:] {
			b := desired[otherID]
			if !b.Enabled {
				continue
			}
			if ar.Intersects(StateRect(b.OutputState)) {
				return invalid(otherID, "position", "displays must not overlap")
			}
		}
	}

	return nil
}

func modeAllowed(output Output, mode Mode, current Snapshot) bool {
	if output.IsCustomMode {
		if original, ok := current.Output(output.ID); ok && sameMode(original.Mode, mode) {
			return true
		}
	}
	for _, advertised := range output.Modes {
		if sameMode(advertised, mode) {
			return true
		}
	}
	return false
}

func sameMode(a, b Mode) bool {
	return a.Width == b.Width &&
		a.Height == b.Height &&
		a.RefreshMilliHz == b.RefreshMilliHz
}

func stateEqual(a, b OutputState) bool {
	if a.Enabled != b.Enabled {
		return false
	}
	if !a.Enabled {
		return true
	}
	return sameMode(a.Mode, b.Mode) &&
		math.Abs(a.Scale-b.Scale) < 1e-9 &&
		a.Transform == b.Transform &&
		a.Position == b.Position &&
		a.VRR == b.VRR
}

func outputByID(snapshot Snapshot, id OutputID) *Output {
	for i := range snapshot.Outputs {
		if snapshot.Outputs[i].ID == id {
			return &snapshot.Outputs[i]
		}
	}
	return nil
}

func outputWithState(output Output, state OutputState) Output {
	output.OutputState = state
	return output
}

// ApplyError reports a failed apply and, when non-nil, a failed rollback.
type ApplyError struct {
	Err         error
	RollbackErr error
}

func (e *ApplyError) Error() string {
	if e.RollbackErr != nil {
		return fmt.Sprintf("%v; rollback failed: %v", e.Err, e.RollbackErr)
	}
	return e.Err.Error()
}

func (e *ApplyError) Unwrap() error {
	return e.Err
}

// IsRollbackFailure reports whether err includes a failed rollback.
func IsRollbackFailure(err error) bool {
	var applyErr *ApplyError
	return errors.As(err, &applyErr) && applyErr.RollbackErr != nil
}

// StatesEqual reports whether two states produce the same compositor effect.
func StatesEqual(a, b OutputState) bool {
	return stateEqual(a, b)
}

// ValidateDraft validates a complete set of desired output states.
func ValidateDraft(caps Capabilities, current Snapshot, outputs []Output) error {
	desired := make(map[OutputID]Output, len(outputs))
	targets := make(map[OutputID]OutputState, len(outputs))
	for _, output := range outputs {
		desired[output.ID] = output
		targets[output.ID] = output.OutputState
	}
	return validate(caps, desired, targets, current)
}

func (s *Service) applyChange(ctx context.Context, change Change) error {
	opCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return s.backend.Apply(opCtx, change)
}
