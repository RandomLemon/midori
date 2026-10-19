# Architecture

The implementation contract for Midori: package boundaries, domain types, the backend interface, the apply and revert transaction, validation, layout math, and failure handling.

Status: Design draft. M1 implements the read-only path; service apply/revert and niri apply actions exist and are tested, but no editing UI uses them yet. This document is authoritative for types and behavior. Product scope is in [product.md](product.md); interaction details are in [ui.md](ui.md).

## Context and constraints

- Single desktop process; no daemon, no helper process, no systemd unit.
- Linux/Wayland only; Fyne renders the UI with the Wayland driver selected by the `wayland` build tag.
- Runtime-only: compositor configuration files are never written. niri documents output IPC changes as temporary; Hyprland `hyprctl eval` changes are lost on a config reload.
- No root privileges, no network access, no telemetry.
- niri `26.04` or newer is the first backend; Hyprland `0.55` or newer with the Lua config provider is a future backend.
- Module path `github.com/RandomLemon/midori`; binary `midori`; application ID `io.github.randomlemon.midori`; Go `1.24.0` minimum; Fyne `fyne.io/fyne/v2 v2.8.2`.

## Process and threading model

- The Fyne main goroutine owns all widget state and every widget mutation. Any code running outside a Fyne event callback MUST marshal UI updates with `fyne.Do` (Fyne 2.6+).
- Backend calls run in short-lived goroutines with a `context.Context`. Backend calls MUST NOT run in a Fyne event callback: a 5-second socket or subprocess wait would freeze the UI.
- `internal/display.Service` serializes transactions with a mutex. Only one apply or revert may run at a time. A second `Apply` call while one is running returns an error immediately rather than queuing.
- Timers (the 15-second confirmation countdown and the 3-second poll) are `time.Ticker`/`time.Timer` loops owned by `internal/app`. Timer callbacks marshal into the Fyne goroutine before touching state.
- The window close handler blocks Fyne's close path long enough to cancel and roll back per the apply algorithm; it does not block indefinitely, because every backend call has a 5-second timeout.

## Layers and dependencies

Dependencies point downward only:

1. `cmd/midori` — flags, backend selection, application bootstrap.
2. `internal/app` — Fyne application, controller, state machine, timers, close interception.
3. `internal/ui` and `internal/display.Service` — widgets and UI-independent display logic.
4. `internal/display.Backend` — the interface implemented by compositor adapters.
5. `internal/backend/*` — niri and Hyprland adapters, plus the registry.

`internal/layout` is pure math and imports only `internal/display` types. `internal/ui` imports Fyne, `internal/display`, and `internal/layout`; it never imports a backend package. `internal/backend/registry` imports the backend packages and returns `display.Backend` values.

```text
cmd/midori
    |
    v
internal/app (Fyne app, controller, state machine, timers, close intercept)
    |                              \
    v                               v
internal/ui                    internal/display (domain, Service, validation, errors)
    |                               |                     |
    v                               v                     v
internal/layout                display.Backend       backend/registry
                                    ^                     |
                                    |                     v
                        backend/niri, backend/hyprland (future)
```

Import rules:

- `internal/display` MUST NOT import Fyne, a backend package, or any subprocess package.
- `internal/layout` MUST NOT import Fyne, a backend package, or `internal/ui`.
- A backend package MUST NOT import `internal/ui` or `internal/app`.
- Backend-specific types MUST NOT appear in exported signatures outside their package.

## Target package layout

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

## Domain model

The following declarations are the contract. JSON wire types and backend-specific types stay private to each backend package.

```go
type OutputID string

type Point struct{ X, Y int }
type Size struct{ Width, Height int }

type Mode struct {
    Width          int
    Height         int
    RefreshMilliHz uint32
    Preferred      bool
}

type Transform uint8

const (
    TransformNormal Transform = iota
    Transform90
    Transform180
    Transform270
    TransformFlipped
    TransformFlipped90
    TransformFlipped180
    TransformFlipped270
)

type OutputState struct {
    Enabled   bool
    Mode      Mode
    Scale     float64
    Transform Transform
    Position  Point
    VRR       bool
}

type Output struct {
    ID           OutputID
    Name         string
    Make         string
    Model        string
    Serial       string
    PhysicalSize *Size // millimetres; nil when unknown
    IsCustomMode bool
    VRRSupported bool
    Modes        []Mode
    LogicalSize  Size
    OutputState
}

type Snapshot struct {
    Backend    string
    CapturedAt time.Time
    Outputs    []Output
}

type Change struct {
    Before Output
    After  OutputState
}

type Capabilities struct {
    Apply       bool
    VRR         bool
    Persistence bool
}

type Backend interface {
    ID() string
    DisplayName() string
    Version(ctx context.Context) (string, error)
    Capabilities() Capabilities
    Outputs(ctx context.Context) ([]Output, error)
    Apply(ctx context.Context, change Change) error
    Close() error
}
```

Semantics:

- `OutputState` is the desired/current subset of an output. Embedded in `Output`, it describes the current state in a snapshot or the desired state in a draft.
- `Modes` is read-only metadata advertised by the compositor. It is the only source of selectable modes.
- `Change.Before` is the pre-apply output, used for diffing and rollback. `Change.After` is the full desired state, not a patch.
- `PhysicalSize` is in millimetres; `nil` when the compositor reports no physical size.
- `LogicalSize` is the compositor-reported logical size for the current state. For a desired state being edited it is a computed preview using the formula in [Layout math](#layout-math).
- `Capabilities.VRR` means the backend can toggle VRR at all. `Output.VRRSupported` means this specific output advertises support. Both must be true for the VRR control to be enabled.
- `Capabilities.Apply` is false for a read-only backend. The UI then disables all editing and Apply and shows a read-only banner. niri sets `Apply: true`; the initial Hyprland backend sets `Apply: false`.
- `Capabilities.Persistence` is false for every v1 backend.
- `Version` is best-effort diagnostic text, for example `26.04`. A version error MUST NOT prevent listing outputs; the UI displays `unknown` when it fails.
- `OutputID` is the connector name, unique within one compositor instance. It is not a stable identity across reboots or reconnects.

## Service API

```go
type Transaction struct {
    Before  Snapshot
    After   Snapshot
    Changed []OutputID
}

func (s *Service) Load(ctx context.Context) (Snapshot, error)
func (s *Service) Refresh(ctx context.Context) (Snapshot, error)
func (s *Service) Apply(ctx context.Context, targets map[OutputID]OutputState) (*Transaction, error)
func (s *Service) Revert(ctx context.Context, before Snapshot) error
```

Service responsibilities:

- Sort outputs by connector name in every returned snapshot.
- Validate targets before calling the backend.
- Build `[]Change` for outputs whose desired state differs from the fresh snapshot.
- Call `Backend.Apply` once per output, sequentially.
- Reload outputs after applying and build the `After` snapshot.
- Roll back on failure, including the failing output.
- Never import or touch Fyne.

`Service.Apply` MUST return `ErrUnsupported` when `Backend.Capabilities().Apply` is false, even if a caller bypasses the UI. `Load` and `Refresh` are equivalent for v1 backends; both re-read outputs and return a fresh snapshot. The distinction exists so a future backend can cache a load path differently.

A second concurrent `Apply` or `Revert` returns an error stating that a transaction is already running; it never interleaves backend calls.

## Backend error taxonomy

```go
var (
    ErrNoBackend            = errors.New("no supported compositor detected")
    ErrConnection           = errors.New("compositor connection failed")
    ErrOutputMissing        = errors.New("output is no longer connected")
    ErrUnsupported          = errors.New("backend does not support this operation")
    ErrInvalidConfiguration = errors.New("invalid display configuration")
)
```

Rules:

- Backends wrap technical causes with `%w`, for example `fmt.Errorf("niri: apply mode on %s: %w", id, ErrOutputMissing)`.
- The UI maps sentinels to human messages; backend error strings are not shown verbatim.
- `Apply` errors include the output ID and identify which output failed.
- Validation failures wrap `ErrInvalidConfiguration` and identify the field.
- A malformed or unexpected compositor reply is a backend error wrapping `ErrConnection` or `ErrUnsupported`, never a panic.

## Backend selection and CLI

Flags:

- `--backend auto|niri|hyprland`, default `auto`.
- `--debug` enables debug logging on stderr.
- `--version` prints `midori <version>` and exits 0.

Auto-detection order:

1. niri if `NIRI_SOCKET` is non-empty and a probe `Outputs` call succeeds.
2. Hyprland if `HYPRLAND_INSTANCE_SIGNATURE` is non-empty, `hyprctl` is on `PATH`, and `hyprctl -j monitors all` succeeds.
3. Otherwise no backend.

niri wins if both probes succeed. Detection probes use the documented timeouts.

Behavior:

- A forced backend that fails to connect exits with code 1 and an actionable stderr message naming the backend and the missing prerequisite.
- Auto-detection failure does not exit; it opens the Unsupported screen with a `Retry` action.
- Exit codes: 0 success, 1 fatal, 2 usage.
- Version is injected with `-ldflags "-X main.version=..."`; an uninjected build prints `midori dev`.
- There is no configuration file and no environment override beyond the compositor variables above.

## State machine

States: `Unavailable`, `Loading`, `Ready`, `Dirty`, `Applying`, `Confirming`, `Reverting`, `Recovering`.

| From | Event | To | Side effects |
| --- | --- | --- | --- |
| Unavailable | Retry | Loading | Start a load; show the loading state. |
| Loading | Load succeeds | Ready | Replace the snapshot; clear the draft; enable editing. |
| Loading | Load fails | Unavailable | Show the mapped error and `Retry`; keep editing disabled. |
| Ready | Any edit | Dirty | Record the draft; enable `Reset` and `Apply`; validate. |
| Dirty | Reset | Ready | Discard the draft; no backend calls. |
| Dirty | Apply | Applying | Validate; start the transaction; disable controls. |
| Applying | All outputs applied, reload succeeds | Confirming | Build the transaction; start the 15-second countdown. |
| Applying | Apply failure, rollback succeeds | Dirty | Reload; keep the draft; show the error. |
| Applying | Apply failure, rollback fails | Recovering | Keep the error; offer retry/reload. |
| Confirming | Keep changes | Ready | Stop the countdown; clear the draft; snapshot becomes `After`. |
| Confirming | Revert now, timeout, or window close | Reverting | Stop the countdown; revert `Changed` outputs. |
| Reverting | Revert succeeds | Ready | Reload; snapshot is the restored state; clear the draft. |
| Reverting | Revert fails | Recovering | Keep the error; offer retry/reload. |
| Recovering | Retry | Loading | Attempt a fresh load of the compositor state. |

Connection loss:

- In `Ready` or `Dirty`, three consecutive poll failures move to `Unavailable`. A single transient failure is ignored.
- In `Applying`, `Confirming`, or `Reverting`, a backend connection error is handled by the transaction: cancel the current operation, attempt rollback or revert, then move to `Unavailable` on success or `Recovering` on failure. Never abandon a transaction because a poll failed.
- Polling is skipped in `Applying`, `Confirming`, and `Reverting`.

Editing gates:

- Only `Ready` and `Dirty` allow editing.
- `Applying`, `Confirming`, `Reverting`, and `Recovering` disable editing and `Apply`, along with `Reset` and `Auto arrange`. `Identify` remains available only for a read-only backend, where no apply transaction can run.

## Apply transaction algorithm

1. Reject if the state is not `Dirty`; reject if no target differs from the snapshot.
2. Validate all targets against the [validation rules](#validation).
3. Set `Applying` and disable controls.
4. Re-read `Backend.Outputs`. If it fails, abort without applying and return to `Dirty`. Use this fresh snapshot as `Before` and revalidate every target against it. If a target output is missing, its advertised modes changed, or its current state no longer matches the snapshot the draft was built from, abort to `Dirty` with a conflict error; do not apply stale targets.
5. Build `[]Change` for outputs whose desired state differs; sort by connector name.
6. Apply each `Change` sequentially with a 5-second context. On failure, attempt rollback for every output already attempted, including the failing one, using `Before`.
7. On success, re-read outputs. If the reload fails, attempt rollback and return an error; do not enter `Confirming` with an unverified state.
8. Build `Transaction{Before, After, Changed}` and set `Confirming`.
9. Start the 15-second countdown. On zero, on `Revert now`, or on window close, call `Revert`.
10. On `Keep changes`, stop the timer, clear the draft, update the snapshot to `After`, and set `Ready`.

Rollback and revert semantics:

- If rollback succeeds after a partial failure: reload, keep the draft, set `Dirty`, and show the error.
- If rollback fails: set `Recovering`, keep the error, and offer retry/reload.
- A target output that disappeared before rollback is skipped and logged at debug level. This alone does not set `Recovering`, because there is no connected output to restore. The next poll reflects its actual state if it returns.
- On `Revert`, apply `Before` for every output in `Changed` that is still present, sequentially, then reload. Missing outputs are skipped and logged at debug level. If any revert apply fails on a present output, set `Recovering`.

Close behavior:

- The confirmation window MUST NOT be dismissible by clicking outside or pressing Escape; Escape means `Revert now`.
- Closing the main window during `Confirming` MUST cancel the countdown and revert before exit.
- During `Applying`, close MUST cancel the apply context, wait for the in-flight output call to return (at most 5 seconds), attempt rollback of every attempted output, and only then exit.
- During `Reverting`, close MUST wait for the revert to finish.
- If rollback or revert fails, keep the window open and show the `Recovering` error.
- Closing during `Dirty` MUST show a `Discard unsaved changes?` confirmation with `Discard` and `Cancel`. Closing during `Ready`, `Loading`, or `Unavailable` exits immediately.

## Validation

- An enabled output requires a mode present in `Before.Modes` by width, height, and refresh millihertz. The current custom mode is allowed only when unchanged.
- Mode dimensions and refresh MUST be greater than 0.
- Scale MUST satisfy `0.5 <= scale <= 3.0`.
- Position coordinates are integers. No two enabled outputs may overlap; touching edges are allowed.
- At least one output MUST remain enabled.
- Disabled outputs are excluded from overlap and bounds checks but retain their draft state for re-enable.
- A disabled output has no current mode or logical geometry. Enabling it for the first time in a session defaults the draft to the preferred advertised mode, or the highest-refresh mode when none is preferred. Scale defaults to `1.0`, transform to `TransformNormal`, VRR to off, and position to an auto-placed slot to the right of the enabled bounds, or `(0,0)` when no output is enabled.
- Re-enabling within the same session reuses the output's last draft state.
- `VRR` may be enabled only when `Capabilities.VRR` and `Output.VRRSupported` are both true; the UI hides the control otherwise.
- Validation errors are field-specific and do not call the backend.

The validation entry point returns a structured error carrying `OutputID` and a field name, so the inspector can mark the offending control and the footer can show the message.

## Layout math

Logical size:

- Swap width and height for `Transform90`, `Transform270`, `TransformFlipped90`, and `TransformFlipped270`.
- Divide the physical dimensions by scale.
- Round to the nearest integer and clamp each dimension to at least 1.

Coordinate system:

- Origin top-left, +X right, +Y down, integer logical pixels; negative positions are allowed.

Overlap:

- Strict rectangle intersection. Edges that only touch do not overlap.

Snap:

- Snap within 12 screen pixels of another output's left, right, top, or bottom edge.
- Convert the screen threshold to logical pixels by dividing by zoom.
- Choose the smallest adjustment; reject a snap that creates overlap.

Auto-arrange:

- Enabled outputs are sorted by current Y, then X, then connector name.
- Place them in a single horizontal row at `y=0`, accumulating logical widths with zero gap.

Canvas zoom:

```text
zoom = min(1.0, max(0.1, min((canvasWidth-64)/boundsWidth, (canvasHeight-64)/boundsHeight)))
```

An empty layout uses zoom `1.0`. The 64-pixel term is the 32-pixel padding on both sides.

Preview:

- Preview layout uses desired modes, scales, and transforms. The compositor's reported `LogicalSize` is authoritative for the current snapshot; a computed preview applies only to edited states.

## Polling and hotplug

- Poll every 3 seconds while the window is open.
- Skip polling while `Applying`, `Confirming`, or `Reverting`.
- Compute a fingerprint over sorted connector names, enabled state, current mode, scale, transform, position, and VRR.
- On a fingerprint change:
  - `Ready`: update the snapshot; there is no draft to lose.
  - `Dirty`: reload and discard the draft; show a non-blocking banner: `Display configuration changed. Pending edits were discarded.`
  - `Confirming`/`Reverting`: do not reload; finish the transaction, then refresh. If a changed output disappears, the revert path reports it.
- Three consecutive poll failures move to `Unavailable`; transient single failures are ignored.
- niri has no output topology event in its IPC event stream, so polling is required. Hyprland also uses polling to keep one code path.

## Persistence

v1 is runtime-only:

- No Save button and no writes to niri KDL or Hyprland Lua/hyprlang configuration.
- niri documents output IPC changes as temporary and forgets them if the output configuration in the config file subsequently changes.
- Hyprland `hyprctl eval` changes on the Lua config provider are runtime-only and are lost on a config reload.
- UI copy explains session-only behavior; see [ui.md](ui.md).
- A crash or forced kill during an apply can leave a partial runtime configuration. The recovery path is restarting the compositor or reloading its configuration, not a write by Midori.
- Adding persistence requires a separate approved design covering backups, parsing and formatting preservation, and reload semantics.

## Security, logging, and failure handling

- No network access, no root privileges, no telemetry.
- Use `exec.CommandContext` with argv slices; never construct a shell command.
- Validate that `$NIRI_SOCKET` is non-empty before dialing.
- Limit niri reply lines to 8 MiB; a longer line is an error, not an allocation.
- Reject Hyprland output names containing control characters or newlines before interpolating them into a command.
- Log backend ID, operation, duration, and output names at debug level only.
- Do not log serial numbers or raw compositor configuration.
- Every backend operation has a 5-second timeout. niri dials with a 2-second timeout.
- No `panic` for expected failures. Validate all external JSON and command output.

## Adding a backend

1. Implement `display.Backend` in `internal/backend/<name>`.
2. Map the compositor's outputs and modes into `display.Output`; keep wire types private.
3. Declare `Capabilities` honestly, including `Persistence: false`.
4. Implement full-state apply: accept `Change{Before, After}` and diff internally.
5. Add a factory and detection probe to `internal/backend/registry.go`.
6. Add fixture tests for every parser and golden tests for every request or command.
7. Add an opt-in live test that reverts its changes.
8. Update [README.md](README.md), [roadmap.md](roadmap.md), and this document's package layout.

No UI changes beyond capability checks are permitted for a new backend.

## References

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
- Fyne Quick Start: https://docs.fyne.io/started/quick/
- Fyne compile options: https://docs.fyne.io/explore/compiling/
- Fyne v2.8.2 module: https://pkg.go.dev/fyne.io/fyne/v2@v2.8.2
