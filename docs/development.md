# Development Guide

How to build, test, and extend Midori, including toolchain, dependencies, test strategy, CI, and packaging.

Status: M3 implemented. The commands below are active for the current module. See [architecture.md](architecture.md) for package contracts and [AGENTS.md](../AGENTS.md) for agent rules.

## Toolchain

- Go `1.24.0` or newer.
- Fyne `fyne.io/fyne/v2 v2.8.2`.
- CGO enabled: `CGO_ENABLED=1`.
- A C compiler and the Wayland development headers.
- Linux/Wayland. niri is Wayland-only.

Fyne's default Linux build targets X11. The `wayland` build tag selects the Wayland driver, so every build and run command uses it. Building without the tag produces a binary that cannot open a window under a pure niri session.

The binary re-executes once with an English locale (`LANG` and `LC_ALL` set to `en_US.UTF-8`, `LANGUAGE=en`) because Fyne resolves its widget locale before `main` runs. This keeps built-in Fyne menus and placeholders English, matching the project language policy.

## System dependencies

Official Fyne commands:

Debian/Ubuntu:

```sh
sudo apt-get install golang gcc libgl1-mesa-dev xorg-dev libwayland-dev libxkbcommon-dev
```

Fedora:

```sh
sudo dnf install golang golang-misc gcc libXcursor-devel libXrandr-devel mesa-libGL-devel libXi-devel libXinerama-devel libXxf86vm-devel libxkbcommon-devel wayland-devel
```

Arch:

```sh
sudo pacman -S go xorg-server-devel libxcursor libxrandr libxinerama libxi wayland libxkbcommon
```

On NixOS, the same dependencies are available in a shell:

```sh
nix-shell -p go gcc pkg-config wayland libxkbcommon libGL libx11 libxcursor libxrandr libxinerama libxi libxxf86vm xorgproto
```

The distro commands come from Fyne's Quick Start; the NixOS shell provides the equivalent package set. See [References](#references).

## Commands

Build:

```sh
CGO_ENABLED=1 go build -tags wayland -trimpath -o bin/midori ./cmd/midori
```

Run from source:

```sh
go run -tags wayland ./cmd/midori
```

Tests:

```sh
go test ./...
go test -race ./...
go vet ./...
```

Formatting check:

```sh
gofmt -l .
```

`gofmt -l .` must print nothing. Use `gofmt -w .` to fix formatting.

`bin/` is ignored by `.gitignore`; build output is never committed.

## Package layout

```text
cmd/midori/main.go              flags, backend selection, app bootstrap
internal/app/                   Fyne application, controller/state machine, timers, close intercept
internal/display/               domain types, Backend interface, Service, validation, errors
internal/backend/registry.go    ordered backend factories and auto-detection
internal/backend/niri/          niri IPC client, codec, backend adapter
internal/backend/hyprland/      hyprctl command adapter and parsers (future)
internal/layout/                logical-size, bounds, snapping, overlap, auto-arrange
internal/ui/                    window, arrangement canvas, tile, inspector, confirmation popup
internal/testutil/fakebackend/  scriptable test backend
```

Dependency direction:

- `cmd` → `app` → `ui` and `display.Service`.
- `display.Service` → `display.Backend`; `backend/registry` constructs implementations.
- `ui` → `display` and `layout`; `ui` never imports a backend package.
- `layout` → `display` types only.

Rules:

- `internal/display` MUST NOT import Fyne.
- `internal/layout` MUST NOT import Fyne or a backend.
- Backend packages MUST NOT import `internal/ui` or `internal/app`.
- Backend-specific wire types stay unexported inside their package.

## Dependencies

- Standard library plus Fyne. Fyne is the only required third-party dependency.
- No CLI framework: `flag` is sufficient for `--backend`, `--debug`, and `--version`.
- No KDL, YAML, or TOML parser. v1 never reads or writes compositor configuration.
- No direct D-Bus or logind integration.
- No CGO outside Fyne's own dependencies.

A new dependency requires a documented reason in the change description and tests that cover the code path it enables.

## Testing strategy

Pure unit tests:

- Domain validation and error classification.
- Mode sorting, deduplication, and selection defaults.
- Transform logical-size math for all eight transforms.
- Snap, overlap, bounds, and auto-arrange.
- niri JSON codec encode/decode.
- Hyprland parser and mode-token mapping.

Service and controller tests with `fakebackend`:

- Successful apply, including the `After` snapshot and `Changed` list.
- Partial failure with successful rollback.
- Partial failure with failed rollback, leading to `Recovering`.
- Confirmation timeout.
- Window close during `Confirming`, `Applying`, and `Reverting`.
- No-op apply rejection.
- At-least-one-enabled rejection.
- Overlap rejection.
- Mode not advertised rejection.
- Concurrent apply rejection.

niri IPC tests:

- Golden request bytes for every action.
- Fixture replies for enabled, disabled, custom, and VRR outputs.
- Fake Unix-socket server round trip.
- Malformed JSON, oversized reply lines, and context timeout.
- `OutputWasMissing` mapping.

UI tests:

- Use `fyne.io/fyne/v2/test` for widget construction, button enablement, and dirty/clean transitions.
- No screenshot golden tests.

Live tests:

```sh
MIDORI_INTEGRATION=1 go test -tags integration ./internal/backend/niri -run Live
```

- Opt-in only, niri only, read-only by default.
- Any apply test must use a disposable output configuration and revert before it finishes.
- Never run live apply tests in CI.

Manual matrix for backend or UI changes:

- One, two, and three outputs.
- Mixed scale and mixed DPI.
- Rotation on one output.
- Disable and re-enable.
- Hotplug while clean and while dirty.
- Config reload during the confirmation countdown.
- Window close during apply.
- Invalid mode selection.
- Drag with edge snapping, numeric X/Y synchronization, and overlapping-drop rejection.
- Auto arrange into a gap-free row and identify overlays that disappear after 3 seconds.
- Socket loss and niri restart.
- Malformed reply from a fake server.

## Fixtures and fake backend

- Fixtures are synthetic JSON only. No real serial numbers, hostnames, or user paths.
- The fake backend supports:
  - Scripted outputs per call.
  - Per-output apply errors.
  - Configurable delays and context cancellation.
  - A call log with operation, output ID, and desired state.
  - Rollback observation, so tests can assert the exact restore sequence.

## Concurrency rules

- Every backend call takes a `context.Context` with the documented timeout.
- One transaction at a time; `Service` serializes apply and revert with a mutex.
- UI updates from background goroutines use `fyne.Do`; never mutate widgets directly off the Fyne goroutine.
- No sleeps in tests for synchronization. Use channels, fake clocks, or injected tickers.
- Timers are stoppable and do not leak after a transaction ends.

## Logging and diagnostics

- Use `log/slog`.
- `--debug` sets the level to debug on stderr; the default level is info.
- Log backend selection, operation name, duration, and output names at debug level.
- Never log raw compositor configuration or serial numbers.
- A debug dump includes the selected backend ID, its version or `unknown`, the output count, and the poll fingerprint state.

## CI

Linux job:

- Go `1.24`.
- Fyne Wayland build dependencies from [System dependencies](#system-dependencies).
- `gofmt -l .` must print nothing.
- `go vet ./...`.
- `go test ./...`.
- `CGO_ENABLED=1 go build -tags wayland -trimpath ./cmd/midori`.

No GUI tests and no live compositor in CI. A future matrix can add the latest Go release.

## Packaging

- Single binary; no daemon and no systemd unit.
- Future packaging command:

```sh
fyne package -os linux -tags wayland -name midori
```

- Application ID: `io.github.randomlemon.midori`.
- Version is injected with `-ldflags "-X main.version=..."`.
- Install instructions ship the binary on `PATH`; no privileged install step is designed in v1.

## Versioning and commits

- SemVer, starting at `0.1.0`.
- Imperative English commit messages, for example `Add niri mode parsing`.
- One logical change per commit.
- Documentation and behavior change in the same commit.
- No commit is created by this documentation task.

## References

- Fyne Quick Start: https://docs.fyne.io/started/quick/
- Fyne compile options: https://docs.fyne.io/explore/compiling/
- Fyne v2.8.2 module: https://pkg.go.dev/fyne.io/fyne/v2@v2.8.2
- niri IPC wiki: https://github.com/niri-wm/niri/wiki/IPC
- niri outputs wiki: https://github.com/niri-wm/niri/wiki/Configuration:-Outputs
- niri-ipc source: https://github.com/niri-wm/niri/blob/main/niri-ipc/src/lib.rs
- niri releases: https://github.com/niri-wm/niri/releases
- Hyprland monitors wiki: https://wiki.hypr.land/Configuring/Basics/Monitors/
- Hyprland monitor modes source: https://github.com/hyprwm/hyprland-wiki/blob/main/content/configuring/core/monitors/modes.md
- Hyprland monitor positioning source: https://github.com/hyprwm/hyprland-wiki/blob/main/content/configuring/core/monitors/positioning.md
- Hyprland hyprctl man page: https://github.com/hyprwm/Hyprland/blob/main/docs/hyprctl.1.rst
- Hyprland monitors JSON source: https://github.com/hyprwm/Hyprland/blob/main/src/ipc/s1/Commands.cpp
- Hyprland status JSON source: https://github.com/hyprwm/Hyprland/blob/main/src/helpers/SystemInfo.cpp
