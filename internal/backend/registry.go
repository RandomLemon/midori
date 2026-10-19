package backend

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/RandomLemon/midori/internal/backend/niri"
	"github.com/RandomLemon/midori/internal/display"
)

const probeTimeout = 5 * time.Second

// Open selects a backend. Auto-detection failures return ErrNoBackend so the
// application can show the unsupported screen. An explicitly requested backend
// failure is returned to the caller.
func Open(ctx context.Context, name string) (display.Backend, error) {
	switch name {
	case "", "auto":
		return detect(ctx)
	case "niri":
		return openNiri(ctx)
	case "hyprland":
		return nil, fmt.Errorf("hyprland backend is not available in this build; supported backends: niri")
	default:
		return nil, fmt.Errorf("unknown backend %q; supported values: auto, niri, hyprland", name)
	}
}

func detect(ctx context.Context) (display.Backend, error) {
	if os.Getenv("NIRI_SOCKET") != "" {
		backend, err := openNiri(ctx)
		if err == nil {
			return backend, nil
		}
	}
	return nil, display.ErrNoBackend
}

func openNiri(ctx context.Context) (display.Backend, error) {
	if os.Getenv("NIRI_SOCKET") == "" {
		return nil, fmt.Errorf("niri backend: NIRI_SOCKET is not set: %w", display.ErrNoBackend)
	}
	backend := niri.NewFromEnv()
	probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	if _, err := backend.Outputs(probeCtx); err != nil {
		_ = backend.Close()
		return nil, fmt.Errorf("niri backend: probe outputs: %w", err)
	}
	return backend, nil
}
