package niri

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/RandomLemon/midori/internal/display"
)

type wireMode struct {
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	RefreshRate uint32 `json:"refresh_rate"`
	IsPreferred bool   `json:"is_preferred"`
}

type wireLogical struct {
	X         int     `json:"x"`
	Y         int     `json:"y"`
	Width     int     `json:"width"`
	Height    int     `json:"height"`
	Scale     float64 `json:"scale"`
	Transform string  `json:"transform"`
}

type wireOutput struct {
	Name         string       `json:"name"`
	Make         string       `json:"make"`
	Model        string       `json:"model"`
	Serial       *string      `json:"serial"`
	PhysicalSize *[2]uint32   `json:"physical_size"`
	Modes        []wireMode   `json:"modes"`
	CurrentMode  *int         `json:"current_mode"`
	IsCustomMode bool         `json:"is_custom_mode"`
	VRRSupported bool         `json:"vrr_supported"`
	VRREnabled   bool         `json:"vrr_enabled"`
	Logical      *wireLogical `json:"logical"`
}

type outputsPayload struct {
	Outputs map[string]wireOutput `json:"Outputs"`
}

type versionPayload struct {
	Version string `json:"Version"`
}

type outputChangedPayload struct {
	OutputConfigChanged string `json:"OutputConfigChanged"`
}

type outputRequest struct {
	Output outputTarget `json:"Output"`
}

type outputTarget struct {
	Output string          `json:"output"`
	Action json.RawMessage `json:"action"`
}

type modeAction struct {
	Mode struct {
		Mode struct {
			Specific struct {
				Width   int     `json:"width"`
				Height  int     `json:"height"`
				Refresh float64 `json:"refresh"`
			} `json:"Specific"`
		} `json:"mode"`
	} `json:"Mode"`
}

type scaleAction struct {
	Scale struct {
		Scale struct {
			Specific float64 `json:"Specific"`
		} `json:"scale"`
	} `json:"Scale"`
}

type transformAction struct {
	Transform struct {
		Transform string `json:"transform"`
	} `json:"Transform"`
}

type positionAction struct {
	Position struct {
		Position struct {
			Specific struct {
				X int `json:"x"`
				Y int `json:"y"`
			} `json:"Specific"`
		} `json:"position"`
	} `json:"Position"`
}

type vrrAction struct {
	Vrr struct {
		VRR struct {
			VRR      bool `json:"vrr"`
			OnDemand bool `json:"on_demand"`
		} `json:"vrr"`
	} `json:"Vrr"`
}

func makeModeAction(mode display.Mode) modeAction {
	var action modeAction
	action.Mode.Mode.Specific.Width = mode.Width
	action.Mode.Mode.Specific.Height = mode.Height
	action.Mode.Mode.Specific.Refresh = mode.RefreshHz()
	return action
}

func makeScaleAction(scale float64) scaleAction {
	var action scaleAction
	action.Scale.Scale.Specific = scale
	return action
}

func makeTransformAction(transform display.Transform) (transformAction, error) {
	value, err := transformString(transform)
	if err != nil {
		return transformAction{}, err
	}
	var action transformAction
	action.Transform.Transform = value
	return action, nil
}

func makePositionAction(position display.Point) positionAction {
	var action positionAction
	action.Position.Position.Specific.X = position.X
	action.Position.Position.Specific.Y = position.Y
	return action
}

func makeVRRAction(vrr bool) vrrAction {
	var action vrrAction
	action.Vrr.VRR.VRR = vrr
	action.Vrr.VRR.OnDemand = false
	return action
}

func encodeOutputRequest(name string, action any) ([]byte, error) {
	raw, err := json.Marshal(action)
	if err != nil {
		return nil, fmt.Errorf("niri: encode action: %w", err)
	}
	return json.Marshal(outputRequest{
		Output: outputTarget{
			Output: name,
			Action: raw,
		},
	})
}

func mapOutput(name string, output wireOutput) (display.Output, error) {
	result := display.Output{
		ID:           display.OutputID(name),
		Name:         name,
		Make:         output.Make,
		Model:        output.Model,
		IsCustomMode: output.IsCustomMode,
		VRRSupported: output.VRRSupported,
	}
	if output.Serial != nil {
		result.Serial = *output.Serial
	}
	if output.PhysicalSize != nil {
		size := display.Size{Width: int(output.PhysicalSize[0]), Height: int(output.PhysicalSize[1])}
		result.PhysicalSize = &size
	}
	result.Modes = make([]display.Mode, 0, len(output.Modes))
	for _, mode := range output.Modes {
		result.Modes = append(result.Modes, display.Mode{
			Width:          mode.Width,
			Height:         mode.Height,
			RefreshMilliHz: mode.RefreshRate,
			Preferred:      mode.IsPreferred,
		})
	}

	if output.CurrentMode == nil || output.Logical == nil {
		if output.CurrentMode != nil || output.Logical != nil {
			// The compositor returned an inconsistent partial state. Treat it
			// as disabled rather than guessing at geometry.
			slog.Debug("niri: output has partial state; treating as disabled", "output", name)
			result.OutputState = display.OutputState{}
		}
		return result, nil
	}

	index := *output.CurrentMode
	if index < 0 || index >= len(output.Modes) {
		slog.Warn("niri: current mode index out of range; treating output as disabled", "output", name, "index", index, "modes", len(output.Modes))
		return result, nil
	}
	transform, err := parseTransform(output.Logical.Transform)
	if err != nil {
		return result, err
	}
	current := display.Mode{
		Width:          output.Modes[index].Width,
		Height:         output.Modes[index].Height,
		RefreshMilliHz: output.Modes[index].RefreshRate,
		Preferred:      output.Modes[index].IsPreferred,
	}
	result.OutputState = display.OutputState{
		Enabled:   true,
		Mode:      current,
		Scale:     output.Logical.Scale,
		Transform: transform,
		Position:  display.Point{X: output.Logical.X, Y: output.Logical.Y},
		VRR:       output.VRREnabled,
	}
	result.LogicalSize = display.Size{Width: output.Logical.Width, Height: output.Logical.Height}

	if output.IsCustomMode {
		selectable := make([]display.Mode, 0, len(result.Modes)-1)
		for i, mode := range result.Modes {
			if i == index {
				continue
			}
			selectable = append(selectable, mode)
		}
		result.Modes = selectable
	}
	return result, nil
}
