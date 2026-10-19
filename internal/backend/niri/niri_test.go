package niri

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/RandomLemon/midori/internal/display"
)

func TestRequestEncoding(t *testing.T) {
	transform, err := makeTransformAction(display.Transform90)
	if err != nil {
		t.Fatalf("makeTransformAction() error = %v", err)
	}
	tests := []struct {
		name   string
		action any
		want   string
	}{
		{name: "off", action: "Off", want: `{"Output":{"output":"HDMI-A-1","action":"Off"}}`},
		{name: "on", action: "On", want: `{"Output":{"output":"HDMI-A-1","action":"On"}}`},
		{
			name:   "mode",
			action: makeModeAction(display.Mode{Width: 2560, Height: 1440, RefreshMilliHz: 143912}),
			want:   `{"Output":{"output":"HDMI-A-1","action":{"Mode":{"mode":{"Specific":{"width":2560,"height":1440,"refresh":143.912}}}}}}`,
		},
		{
			name:   "scale",
			action: makeScaleAction(1.5),
			want:   `{"Output":{"output":"HDMI-A-1","action":{"Scale":{"scale":{"Specific":1.5}}}}}`,
		},
		{
			name:   "transform",
			action: transform,
			want:   `{"Output":{"output":"HDMI-A-1","action":{"Transform":{"transform":"90"}}}}`,
		},
		{
			name:   "position",
			action: makePositionAction(display.Point{X: 0, Y: 0}),
			want:   `{"Output":{"output":"HDMI-A-1","action":{"Position":{"position":{"Specific":{"x":0,"y":0}}}}}}`,
		},
		{
			name:   "vrr",
			action: makeVRRAction(true),
			want:   `{"Output":{"output":"HDMI-A-1","action":{"Vrr":{"vrr":{"vrr":true,"on_demand":false}}}}}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := encodeOutputRequest("HDMI-A-1", tt.action)
			if err != nil {
				t.Fatalf("encodeOutputRequest() error = %v", err)
			}
			if string(got) != tt.want {
				t.Fatalf("request = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestTransformRoundTrip(t *testing.T) {
	transforms := []display.Transform{
		display.TransformNormal,
		display.Transform90,
		display.Transform180,
		display.Transform270,
		display.TransformFlipped,
		display.TransformFlipped90,
		display.TransformFlipped180,
		display.TransformFlipped270,
	}
	for _, transform := range transforms {
		value, err := transformString(transform)
		if err != nil {
			t.Fatalf("transformString(%v) error = %v", transform, err)
		}
		got, err := parseTransform(value)
		if err != nil {
			t.Fatalf("parseTransform(%q) error = %v", value, err)
		}
		if got != transform {
			t.Fatalf("round trip = %v, want %v", got, transform)
		}
	}
	if _, err := parseTransform("flipped-90"); !errors.Is(err, display.ErrUnsupported) {
		t.Fatalf("parseTransform(flipped-90) error = %v, want ErrUnsupported", err)
	}
}

const outputsFixture = `{"Ok":{"Outputs":{` +
	`"eDP-1":{"name":"eDP-1","make":"AU Optronics","model":"0x1234","serial":"0x0000","physical_size":[344,215],` +
	`"modes":[{"width":1920,"height":1200,"refresh_rate":60000,"is_preferred":true},` +
	`{"width":1920,"height":1200,"refresh_rate":48000,"is_preferred":false}],` +
	`"current_mode":0,"is_custom_mode":false,"vrr_supported":false,"vrr_enabled":false,` +
	`"logical":{"x":0,"y":0,"width":1920,"height":1200,"scale":1.0,"transform":"Normal"}},` +
	`"HDMI-A-1":{"name":"HDMI-A-1","make":"Dell Inc.","model":"U2723QE","serial":null,"physical_size":[600,340],` +
	`"modes":[{"width":2560,"height":1440,"refresh_rate":143912,"is_preferred":true}],` +
	`"current_mode":null,"is_custom_mode":false,"vrr_supported":true,"vrr_enabled":false,"logical":null}` +
	`}}}`

func TestOutputsFixture(t *testing.T) {
	server := startTestServer(t, func(_ context.Context, request string) string {
		switch request {
		case `"Outputs"`:
			return outputsFixture
		case `"Version"`:
			return `{"Ok":{"Version":"26.04"}}`
		default:
			return `{"Err":"unexpected request"}`
		}
	})
	backend := New(server.Config())

	outputs, err := backend.Outputs(context.Background())
	if err != nil {
		t.Fatalf("Outputs() error = %v", err)
	}
	if len(outputs) != 2 {
		t.Fatalf("len(outputs) = %d, want 2", len(outputs))
	}
	if outputs[0].ID != "HDMI-A-1" || outputs[1].ID != "eDP-1" {
		t.Fatalf("outputs not sorted: %v, %v", outputs[0].ID, outputs[1].ID)
	}
	hdmi := outputs[0]
	if hdmi.Enabled {
		t.Fatal("HDMI-A-1 should be disabled")
	}
	if hdmi.VRRSupported != true || len(hdmi.Modes) != 1 {
		t.Fatalf("HDMI-A-1 mapping = %+v", hdmi)
	}
	edp := outputs[1]
	if !edp.Enabled {
		t.Fatal("eDP-1 should be enabled")
	}
	if edp.Mode.RefreshMilliHz != 60000 || edp.LogicalSize.Width != 1920 || edp.LogicalSize.Height != 1200 {
		t.Fatalf("eDP-1 mapping = %+v", edp)
	}
	if edp.PhysicalSize == nil || edp.PhysicalSize.Width != 344 || edp.PhysicalSize.Height != 215 {
		t.Fatalf("eDP-1 physical size = %+v", edp.PhysicalSize)
	}
	if edp.Serial != "0x0000" {
		t.Fatalf("eDP-1 serial = %q", edp.Serial)
	}

	version, err := backend.Version(context.Background())
	if err != nil {
		t.Fatalf("Version() error = %v", err)
	}
	if version != "26.04" {
		t.Fatalf("Version() = %q, want 26.04", version)
	}
}

func TestCustomModeIsReadOnly(t *testing.T) {
	fixture := `{"Ok":{"Outputs":{"DP-1":{"name":"DP-1","make":"Dell","model":"U2723QE","serial":"abc",` +
		`"modes":[{"width":2560,"height":1440,"refresh_rate":143912,"is_preferred":false},` +
		`{"width":1920,"height":1080,"refresh_rate":60000,"is_preferred":true}],` +
		`"current_mode":0,"is_custom_mode":true,"vrr_supported":true,"vrr_enabled":true,` +
		`"logical":{"x":100,"y":50,"width":1707,"height":960,"scale":1.5,"transform":"Flipped90"}}}}}`
	server := startTestServer(t, func(context.Context, string) string { return fixture })
	backend := New(server.Config())

	outputs, err := backend.Outputs(context.Background())
	if err != nil {
		t.Fatalf("Outputs() error = %v", err)
	}
	if len(outputs) != 1 {
		t.Fatalf("len(outputs) = %d, want 1", len(outputs))
	}
	output := outputs[0]
	if !output.Enabled || !output.IsCustomMode {
		t.Fatalf("output state = %+v", output)
	}
	if output.Mode.Width != 2560 || output.Mode.RefreshMilliHz != 143912 {
		t.Fatalf("current custom mode = %+v", output.Mode)
	}
	if len(output.Modes) != 1 || output.Modes[0].Width != 1920 {
		t.Fatalf("selectable modes = %+v, want only 1920x1080", output.Modes)
	}
	if output.Transform != display.TransformFlipped90 {
		t.Fatalf("transform = %v", output.Transform)
	}
	if output.Position != (display.Point{X: 100, Y: 50}) || output.VRR != true {
		t.Fatalf("state = %+v", output.OutputState)
	}
}

func TestOutOfRangeCurrentModeIsDisabled(t *testing.T) {
	fixture := `{"Ok":{"Outputs":{"DP-1":{"name":"DP-1","make":"Dell","model":"U2723QE","serial":"abc",` +
		`"modes":[{"width":1920,"height":1080,"refresh_rate":60000,"is_preferred":true}],` +
		`"current_mode":5,"is_custom_mode":false,"vrr_supported":false,"vrr_enabled":false,` +
		`"logical":{"x":0,"y":0,"width":1920,"height":1080,"scale":1,"transform":"Normal"}}}}}`
	server := startTestServer(t, func(context.Context, string) string { return fixture })
	backend := New(server.Config())

	outputs, err := backend.Outputs(context.Background())
	if err != nil {
		t.Fatalf("Outputs() error = %v", err)
	}
	if len(outputs) != 1 || outputs[0].Enabled {
		t.Fatalf("output = %+v, want disabled", outputs)
	}
}

func TestApplyOrder(t *testing.T) {
	server := startTestServer(t, func(context.Context, string) string {
		return `{"Ok":{"OutputConfigChanged":"Applied"}}`
	})
	backend := New(server.Config())
	before := display.Output{
		ID:   "HDMI-A-1",
		Name: "HDMI-A-1",
		OutputState: display.OutputState{
			Enabled:   true,
			Mode:      display.Mode{Width: 2560, Height: 1440, RefreshMilliHz: 143912},
			Scale:     1,
			Transform: display.TransformNormal,
			Position:  display.Point{X: 0, Y: 0},
		},
	}
	after := before.OutputState
	after.Mode.RefreshMilliHz = 60000
	after.Scale = 1.25
	after.Transform = display.Transform90
	after.Position = display.Point{X: 100, Y: 50}
	after.VRR = true

	if err := backend.Apply(context.Background(), display.Change{Before: before, After: after}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	requests := server.Requests()
	want := []string{
		`{"Output":{"output":"HDMI-A-1","action":{"Mode":{"mode":{"Specific":{"width":2560,"height":1440,"refresh":60}}}}}}`,
		`{"Output":{"output":"HDMI-A-1","action":{"Scale":{"scale":{"Specific":1.25}}}}}`,
		`{"Output":{"output":"HDMI-A-1","action":{"Transform":{"transform":"90"}}}}`,
		`{"Output":{"output":"HDMI-A-1","action":{"Position":{"position":{"Specific":{"x":100,"y":50}}}}}}`,
		`{"Output":{"output":"HDMI-A-1","action":{"Vrr":{"vrr":{"vrr":true,"on_demand":false}}}}}`,
	}
	if len(requests) != len(want) {
		t.Fatalf("requests = %d, want %d: %v", len(requests), len(want), requests)
	}
	for i := range want {
		if requests[i] != want[i] {
			t.Fatalf("request[%d] = %s, want %s", i, requests[i], want[i])
		}
	}
}

func TestApplyEnableAndDisable(t *testing.T) {
	server := startTestServer(t, func(context.Context, string) string {
		return `{"Ok":{"OutputConfigChanged":"Applied"}}`
	})
	backend := New(server.Config())
	disabled := display.Output{ID: "DP-1", Name: "DP-1"}
	enabledState := display.OutputState{
		Enabled:   true,
		Mode:      display.Mode{Width: 1920, Height: 1080, RefreshMilliHz: 60000},
		Scale:     1,
		Transform: display.TransformNormal,
		Position:  display.Point{X: 0, Y: 0},
	}
	if err := backend.Apply(context.Background(), display.Change{Before: disabled, After: enabledState}); err != nil {
		t.Fatalf("enable Apply() error = %v", err)
	}
	requests := server.Requests()
	if len(requests) != 6 || !strings.Contains(requests[0], `"action":"On"`) {
		t.Fatalf("enable requests = %v", requests)
	}
	if !strings.Contains(requests[5], `"Vrr"`) {
		t.Fatalf("enable requests do not end with VRR: %v", requests)
	}

	server2 := startTestServer(t, func(context.Context, string) string {
		return `{"Ok":{"OutputConfigChanged":"Applied"}}`
	})
	backend2 := New(server2.Config())
	before := display.Output{ID: "DP-1", Name: "DP-1", OutputState: enabledState}
	if err := backend2.Apply(context.Background(), display.Change{Before: before, After: display.OutputState{Enabled: false}}); err != nil {
		t.Fatalf("disable Apply() error = %v", err)
	}
	if got := server2.Requests(); len(got) != 1 || !strings.Contains(got[0], `"action":"Off"`) {
		t.Fatalf("disable requests = %v", got)
	}
}

func TestApplyOutputWasMissing(t *testing.T) {
	server := startTestServer(t, func(context.Context, string) string {
		return `{"Ok":{"OutputConfigChanged":"OutputWasMissing"}}`
	})
	backend := New(server.Config())
	change := display.Change{
		Before: display.Output{ID: "DP-1", Name: "DP-1"},
		After: display.OutputState{
			Enabled:   true,
			Mode:      display.Mode{Width: 1920, Height: 1080, RefreshMilliHz: 60000},
			Scale:     1,
			Transform: display.TransformNormal,
		},
	}
	err := backend.Apply(context.Background(), change)
	if !errors.Is(err, display.ErrOutputMissing) {
		t.Fatalf("Apply() error = %v, want ErrOutputMissing", err)
	}
}

func TestMalformedAndOversizedReplies(t *testing.T) {
	t.Run("malformed envelope", func(t *testing.T) {
		server := startTestServer(t, func(context.Context, string) string { return "not-json" })
		backend := New(server.Config())
		_, err := backend.Version(context.Background())
		if !errors.Is(err, display.ErrConnection) {
			t.Fatalf("Version() error = %v, want ErrConnection", err)
		}
	})
	t.Run("malformed payload", func(t *testing.T) {
		server := startTestServer(t, func(context.Context, string) string {
			return `{"Ok":{"Version":123}}`
		})
		backend := New(server.Config())
		_, err := backend.Version(context.Background())
		if !errors.Is(err, display.ErrUnsupported) {
			t.Fatalf("Version() error = %v, want ErrUnsupported", err)
		}
	})
	t.Run("oversized", func(t *testing.T) {
		server := startTestServer(t, func(context.Context, string) string {
			return `{"Ok":{"Version":"` + strings.Repeat("x", 200) + `"}}`
		})
		cfg := server.Config()
		cfg.MaxReplyBytes = 32
		backend := New(cfg)
		_, err := backend.Version(context.Background())
		if !errors.Is(err, display.ErrConnection) {
			t.Fatalf("Version() error = %v, want ErrConnection", err)
		}
	})
}

func TestContextTimeout(t *testing.T) {
	server := startTestServer(t, func(ctx context.Context, _ string) string {
		<-ctx.Done()
		return ""
	})
	backend := New(server.Config())
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := backend.Version(ctx); err == nil {
		t.Fatal("Version() error = nil, want timeout")
	}
}

func TestEmptySocketPath(t *testing.T) {
	backend := New(DefaultConfig(""))
	if _, err := backend.Version(context.Background()); !errors.Is(err, display.ErrConnection) {
		t.Fatalf("Version() error = %v, want ErrConnection", err)
	}
}
