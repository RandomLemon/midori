# AGENTS.md

Operational instructions for coding agents working on Midori, a Linux/Wayland display-configuration GUI for niri.

## Project

Midori is a Linux/Wayland display-configuration GUI for the niri compositor, written in Go with Fyne, and designed so additional compositor backends can be added without changing the UI. The repository contains the M3 implementation: module skeleton, domain and service layers, the niri IPC backend, a Fyne window with editable output controls, the apply/confirmation/revert flow, and the arrangement editor with drag, snapping, numeric positioning, auto-arrange, and identify overlays. Packaging and Hyprland remain future milestones. The authoritative design starts at [`docs/README.md`](docs/README.md).

- Module path: `github.com/RandomLemon/midori`
- Binary name: `midori`
- Application ID: `io.github.randomlemon.midori`
- Go: `1.24.0` minimum
- Fyne: `fyne.io/fyne/v2 v2.8.2`
- niri baseline: `26.04` or newer
- Hyprland future baseline: `0.55` or newer with the Lua config provider
- Backend IDs: `niri` and `hyprland`; display names `niri` and `Hyprland`

## Language policy

All tracked content is English: documentation, code, identifiers, comments, UI strings, commit messages, and pull request descriptions. A change containing non-English prose is incomplete, including partial translations and copied non-English source comments.

## Read first

Read the documentation in this order before changing behavior:

1. [`docs/README.md`](docs/README.md) — index, glossary, key decisions.
2. [`docs/architecture.md`](docs/architecture.md) — package boundaries, domain types, transaction contract.
3. The relevant backend document: [`docs/backends/niri.md`](docs/backends/niri.md) or [`docs/backends/hyprland.md`](docs/backends/hyprland.md).

[`docs/product.md`](docs/product.md) defines scope, [`docs/ui.md`](docs/ui.md) defines interaction behavior, [`docs/development.md`](docs/development.md) defines toolchain and tests, and [`docs/roadmap.md`](docs/roadmap.md) defines milestone order.

## Architecture invariants

- The UI never talks to a compositor. It calls `internal/display.Service`; only `internal/backend/*` implements `display.Backend`.
- Backend-specific types MUST NOT escape the backend package. The domain uses only `internal/display` types.
- Every apply is a transaction: snapshot, apply per output, reload, confirm with a 15-second countdown, and revert on timeout, explicit revert, or window close. Partial failure triggers best-effort rollback.
- v1 is runtime-only. NEVER write `~/.config/niri/config.kdl`, Hyprland config files, or any compositor configuration. Do not add a Save action without a new approved design.
- niri uses direct JSON IPC on `$NIRI_SOCKET`; do not shell out to `niri msg` for core operations.
- Hyprland inventory uses `hyprctl` with `-j`; do not implement Hyprland's private socket protocol. Initial Hyprland support is read-only. Apply only through `hyprctl eval` for a Lua config provider, and only after the rule-preservation gate in [`docs/backends/hyprland.md`](docs/backends/hyprland.md) passes.
- All Fyne object mutations happen on the Fyne goroutine. Background results are marshaled with `fyne.Do`; backend calls MUST NOT run in a Fyne event callback.
- All backend calls take a `context.Context` with the documented timeout. No unbounded subprocess or socket reads.
- No root privileges, no network access, no telemetry, no shell command construction.

Documented timeouts and limits: backend operation timeout `5` seconds, niri dial timeout `2` seconds, niri reply line limit `8 MiB`, hotplug poll interval `3` seconds, apply confirmation `15` seconds.

## Target repository layout

This tree is the package contract. M3 implements `cmd/midori`, `internal/app`, `internal/display`, `internal/backend`, `internal/backend/niri`, `internal/layout`, `internal/ui`, and `internal/testutil/fakebackend`; `internal/backend/hyprland` remains future work.

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

## Toolchain and commands

These commands are active for the M3 implementation. The `wayland` build tag is mandatory for running under niri because niri is Wayland-only; Fyne's default Linux build targets X11.

```sh
CGO_ENABLED=1 go build -tags wayland -trimpath -o bin/midori ./cmd/midori
go run -tags wayland ./cmd/midori
go test ./...
go test -race ./...
go vet ./...
gofmt -l .
```

`gofmt -l .` must print nothing. Linux dependencies from Fyne:

- Debian/Ubuntu: `golang gcc libgl1-mesa-dev xorg-dev libwayland-dev libxkbcommon-dev`
- Fedora: `golang golang-misc gcc libXcursor-devel libXrandr-devel mesa-libGL-devel libXi-devel libXinerama-devel libXxf86vm-devel libxkbcommon-devel wayland-devel`
- Arch: `go xorg-server-devel libxcursor libxrandr libxinerama libxi wayland libxkbcommon`

Exact install commands and caveats are in [`docs/development.md`](docs/development.md).

## Coding conventions

- Standard library first; Fyne is the only required third-party dependency.
- Use `log/slog` for logging; `--debug` raises the level to debug on stderr.
- Wrap errors with `%w`; define typed sentinel errors in `internal/display`; no `panic` for expected failures; no `log.Fatal` outside `main`.
- Pass dependencies through constructors; no package-level mutable state.
- Keep `internal/display` free of Fyne imports.
- Validate all external JSON and command output. Reject control characters and newlines in backend-provided text that is interpolated into a command.
- Use `exec.CommandContext` with an argv slice; never build a shell command string.

## Testing

- Unit tests for domain validation, layout math, JSON codecs, transaction state, and Hyprland parsing.
- A scriptable fake backend in `internal/testutil/fakebackend` for service and controller tests.
- niri codec golden JSON tests and a fake Unix-socket server round trip.
- Live niri tests are opt-in, read-only by default, and MUST revert any change they make.
- UI tests use `fyne.io/fyne/v2/test`.
- Backend or UI behavior changes require a manual niri smoke test.

The full strategy, including the manual matrix, is in [`docs/development.md`](docs/development.md).

## Safety and data

- Do not modify user configuration. v1 never writes compositor configuration.
- Do not log raw compositor configuration or serial numbers unnecessarily; output names may be logged. Log backend ID, operation, duration, and output names at debug level only.
- Reject control characters and newlines in backend text before display or command construction.
- No root privileges, no network access, no telemetry.

## Documentation maintenance

- Update the affected documents in the same change as the behavior.
- Keep the constants in this document and the design docs consistent: versions, timeouts, window sizes, scale range, snap threshold, and application ID.
- Update `AGENTS.md` when build or test commands, package boundaries, or invariants change.

## Definition of done

- Affected docs updated in the same change.
- `gofmt -l .` prints nothing.
- `go vet ./...` passes.
- `go test ./...` passes.
- `go test -race ./...` passes.
- Manual smoke test on niri for backend or UI changes, including an apply and an automatic revert.
- No CJK or other non-English prose.

## See also

- [Documentation index](docs/README.md)
- [Architecture](docs/architecture.md)
- [niri backend](docs/backends/niri.md)
- [Hyprland backend](docs/backends/hyprland.md)
- [Development](docs/development.md)
- [niri IPC wiki](https://github.com/niri-wm/niri/wiki/IPC)
- [niri outputs wiki](https://github.com/niri-wm/niri/wiki/Configuration:-Outputs)
- [Hyprland monitors wiki](https://wiki.hypr.land/Configuring/Basics/Monitors/)
- [hyprctl man page](https://github.com/hyprwm/Hyprland/blob/main/docs/hyprctl.1.rst)
- [Fyne Quick Start](https://docs.fyne.io/started/quick/)
- [Fyne compile options](https://docs.fyne.io/explore/compiling/)
