package niri

import (
	"context"
	"fmt"
	"math"
	"os"
	"sort"

	"github.com/RandomLemon/midori/internal/display"
)

type Backend struct {
	client *client
}

func New(cfg Config) *Backend {
	return &Backend{client: &client{cfg: cfg}}
}

func NewFromEnv() *Backend {
	return New(DefaultConfig(os.Getenv("NIRI_SOCKET")))
}

func (b *Backend) ID() string          { return "niri" }
func (b *Backend) DisplayName() string { return "niri" }

func (b *Backend) Capabilities() display.Capabilities {
	return display.Capabilities{Apply: true, VRR: true, Persistence: false}
}

func (b *Backend) Version(ctx context.Context) (string, error) {
	raw, err := b.client.roundTrip(ctx, []byte(`"Version"`))
	if err != nil {
		return "", err
	}
	var payload versionPayload
	if err := decodePayload(raw, &payload); err != nil {
		return "", err
	}
	if payload.Version == "" {
		return "", fmt.Errorf("niri: version reply is empty: %w", display.ErrUnsupported)
	}
	return payload.Version, nil
}

func (b *Backend) Outputs(ctx context.Context) ([]display.Output, error) {
	raw, err := b.client.roundTrip(ctx, []byte(`"Outputs"`))
	if err != nil {
		return nil, err
	}
	var payload outputsPayload
	if err := decodePayload(raw, &payload); err != nil {
		return nil, err
	}

	names := make([]string, 0, len(payload.Outputs))
	for name := range payload.Outputs {
		names = append(names, name)
	}
	sort.Strings(names)

	outputs := make([]display.Output, 0, len(names))
	for _, name := range names {
		output, err := mapOutput(name, payload.Outputs[name])
		if err != nil {
			return nil, fmt.Errorf("niri: output %s: %w", name, err)
		}
		outputs = append(outputs, output)
	}
	return outputs, nil
}

func (b *Backend) Apply(ctx context.Context, change display.Change) error {
	steps, err := stepsFor(change)
	if err != nil {
		return err
	}
	if len(steps) == 0 {
		return nil
	}
	name := change.Before.Name
	if name == "" {
		name = string(change.Before.ID)
	}
	return b.applySteps(ctx, name, steps)
}

func (b *Backend) Close() error {
	return nil
}

type applyStep struct {
	label  string
	action any
}

func (b *Backend) applySteps(ctx context.Context, name string, steps []applyStep) error {
	for _, step := range steps {
		request, err := encodeOutputRequest(name, step.action)
		if err != nil {
			return err
		}
		raw, err := b.client.roundTrip(ctx, request)
		if err != nil {
			return fmt.Errorf("niri: apply %s %s: %w", name, step.label, err)
		}
		var payload outputChangedPayload
		if err := decodePayload(raw, &payload); err != nil {
			return fmt.Errorf("niri: apply %s %s: %w", name, step.label, err)
		}
		switch payload.OutputConfigChanged {
		case "Applied":
			continue
		case "OutputWasMissing":
			return fmt.Errorf("niri: apply %s %s: %w", name, step.label, display.ErrOutputMissing)
		default:
			return fmt.Errorf("niri: apply %s %s: unexpected reply %q: %w", name, step.label, payload.OutputConfigChanged, display.ErrUnsupported)
		}
	}
	return nil
}

func stepsFor(change display.Change) ([]applyStep, error) {
	before := change.Before.OutputState
	after := change.After
	switch {
	case !before.Enabled && !after.Enabled:
		return nil, nil
	case !after.Enabled:
		return []applyStep{{label: "off", action: "Off"}}, nil
	case !before.Enabled:
		transform, err := makeTransformAction(after.Transform)
		if err != nil {
			return nil, err
		}
		return []applyStep{
			{label: "on", action: "On"},
			{label: "mode", action: makeModeAction(after.Mode)},
			{label: "scale", action: makeScaleAction(after.Scale)},
			{label: "transform", action: transform},
			{label: "position", action: makePositionAction(after.Position)},
			{label: "vrr", action: makeVRRAction(after.VRR)},
		}, nil
	}

	var steps []applyStep
	if !sameMode(before.Mode, after.Mode) {
		steps = append(steps, applyStep{label: "mode", action: makeModeAction(after.Mode)})
	}
	if math.Abs(before.Scale-after.Scale) >= 1e-4 {
		steps = append(steps, applyStep{label: "scale", action: makeScaleAction(after.Scale)})
	}
	if before.Transform != after.Transform {
		transform, err := makeTransformAction(after.Transform)
		if err != nil {
			return nil, err
		}
		steps = append(steps, applyStep{label: "transform", action: transform})
	}
	if before.Position != after.Position {
		steps = append(steps, applyStep{label: "position", action: makePositionAction(after.Position)})
	}
	if before.VRR != after.VRR {
		steps = append(steps, applyStep{label: "vrr", action: makeVRRAction(after.VRR)})
	}
	return steps, nil
}

func sameMode(a, b display.Mode) bool {
	return a.Width == b.Width &&
		a.Height == b.Height &&
		a.RefreshMilliHz == b.RefreshMilliHz
}
