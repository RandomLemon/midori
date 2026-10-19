# Midori Documentation

Documentation index for Midori, a Linux/Wayland desktop GUI that inspects and changes display configuration under niri, with a backend boundary for future compositors.

Status: Design draft. M3 is implemented: module skeleton, niri IPC backend, service transaction layer, editable inspector, apply/confirmation/revert flow, and arrangement editing with drag, snapping, numeric positioning, auto-arrange, and identify overlays. Packaging and Hyprland remain future milestones. [`../AGENTS.md`](../AGENTS.md) holds the operational rules for agents and contributors; this file is the orientation entry point.

## Product summary

Midori is a desktop Go/Fyne GUI for inspecting and changing display configuration under niri. It lists connected outputs, shows their mode, refresh rate, scale, transform, position, enablement, and variable refresh rate state, renders an arrangement canvas, and applies edits as a single reversible runtime transaction. A backend interface isolates compositor specifics so Hyprland, and later other compositors, can be added without UI forks.

## Design summary

- Go `1.24.0` with Fyne `fyne.io/fyne/v2 v2.8.2`; single binary.
- Wayland-only; niri `26.04` or newer is the first backend.
- Direct JSON IPC on `$NIRI_SOCKET`; no `niri msg` subprocess for core operations.
- Hyprland later via `hyprctl` with a read-only inventory first and a gated apply path using `hyprctl eval` on the Lua config provider.
- Runtime-only: v1 exposes no Save action and writes no compositor configuration files.
- Safe apply: every apply is a transaction with a 15-second confirmation and automatic rollback on timeout, explicit revert, or window close.
- Hotplug awareness through a 3-second poll of compositor outputs.
- No daemon, no root privileges, no network access, no telemetry.

## Implementation status

- M0 documentation: complete.
- M1 skeleton and niri read-only: complete. The binary lists outputs, polls every 3 seconds, and reports loading, unsupported, no-output, and lost-connection states.
- M2 inspector editing and apply/revert: complete. The inspector edits mode, refresh, scale, rotation, enablement, VRR, and position; apply uses a 15-second confirmation with automatic revert, rollback, and close interception.
- M3 arrangement editing: complete. Tiles drag in logical pixels, snap within 12 screen pixels, reject overlapping drops, stay synchronized with numeric X/Y entry, auto-arrange into a gap-free row, and show numbered identify overlays for 3 seconds.
- M4–M7: not started.
- M8 persistence: deferred; no configuration-file writes.

## Document map

| Document | Contents | Read when |
| --- | --- | --- |
| [`product.md`](product.md) | Scope, users, requirements, acceptance criteria. | Deciding whether a feature belongs in v1. |
| [`architecture.md`](architecture.md) | Layers, packages, domain model, backend contract, apply/revert state machine, backend selection, validation, polling, security. | Implementing or reviewing any code. |
| [`ui.md`](ui.md) | Window layout, canvas, inspector, confirmation dialog, copy, empty/error states, accessibility. | Changing any user-visible behavior. |
| [`backends/niri.md`](backends/niri.md) | niri IPC discovery, exact JSON schema, mapping, apply order, limitations. | Touching the niri client or codec. |
| [`backends/hyprland.md`](backends/hyprland.md) | Future adapter for Hyprland 0.55+ Lua config: read-only inventory first, apply design gated on rule-preservation verification. | Adding or reviewing Hyprland support. |
| [`development.md`](development.md) | Toolchain, dependencies, build, tests, package layout, CI. | Setting up a checkout or a CI job. |
| [`roadmap.md`](roadmap.md) | Milestones and observable acceptance criteria. | Planning the next implementation step. |

## Suggested reading order

For a new contributor: [`README.md`](README.md) → [`product.md`](product.md) → [`architecture.md`](architecture.md) → [`ui.md`](ui.md) → [`backends/niri.md`](backends/niri.md) → [`development.md`](development.md). Read [`backends/hyprland.md`](backends/hyprland.md) only when working on the future Hyprland backend, and [`roadmap.md`](roadmap.md) when picking up implementation work.

## Glossary

| Term | Meaning |
| --- | --- |
| output | The compositor's display connector, for example `eDP-1`. "Display" is the user-facing term for the same thing. |
| connector | The output name reported by the compositor. |
| mode | Resolution plus refresh rate. |
| refresh rate | Vertical refresh in hertz; stored internally as millihertz. |
| scale | The compositor's logical-to-physical scaling factor. |
| transform | The output rotation and flip applied by the compositor. |
| logical pixel | A compositor coordinate-space unit after scale and transform. |
| physical pixel | A mode dimension. |
| snapshot | The last successfully observed compositor state. |
| draft | Pending edits that have not been applied. |
| apply transaction | The sequence that snapshots, applies, confirms, and optionally reverts. |
| backend | A compositor-specific adapter implementing `display.Backend`. |
| service | UI-independent display logic in `internal/display.Service`. |
| capability | A backend-declared permission such as `Apply`, `VRR`, or `Persistence`. |

## Key decisions

- Direct niri JSON IPC; no `niri msg` subprocess.
- `hyprctl` with JSON output for Hyprland; no private socket protocol.
- No config-file writes in v1; changes are runtime-only and session-only.
- One apply transaction at a time; the service serializes apply and revert.
- Confirmation is required for every apply, with a 15-second automatic revert.
- Disabled outputs are excluded from overlap validation but remain listed and editable.
- At least one output must remain enabled.
- Hyprland starts read-only; apply is gated on verifying that `hl.monitor` preserves unmanaged per-output fields before `hyprctl eval` is enabled on the Lua config provider.

## See also

- [Agent and contributor instructions](../AGENTS.md)
- [Architecture contract](architecture.md)
- [niri IPC wiki](https://github.com/niri-wm/niri/wiki/IPC)
- [Hyprland monitors wiki](https://wiki.hypr.land/Configuring/Basics/Monitors/)
- [Fyne Quick Start](https://docs.fyne.io/started/quick/)
