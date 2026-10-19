# Product Requirements

Defines the user problem, v1 scope, and observable acceptance criteria for Midori.

Status: Design draft. M3 implements inspection, inspector editing, arrangement editing, apply/confirmation, and revert. See the [documentation index](README.md) and the [architecture contract](architecture.md) for implementation details.

## Problem

Wayland compositors expose display configuration through their own text formats. niri configures outputs in KDL inside `~/.config/niri/config.kdl`; changing a mode, scale, rotation, position, enablement, or variable refresh rate means editing that file, running a reload, and guessing at connector names and advertised modes. Mistakes can leave a display unusable: a wrong scale can make the UI unusable, an overlapping position can hide an output, and an unsupported refresh rate can blank a screen. Recovery then requires editing configuration from a TTY or another machine.

niri provides runtime output IPC, but it is a command-line interface with JSON payloads and no discovery aid. Users need a KDE-like graphical way to inspect the current arrangement, change settings, see the result immediately, and keep the change only if the display remains usable. A rollback path is mandatory, not a convenience.

## Users

- niri users on laptops with a single internal display and occasional external monitors.
- Multi-monitor desktop users who arrange outputs and change modes regularly.
- Mixed-DPI setups where scale and logical positions interact.
- High-refresh display owners who switch between refresh rates for games, video, and power saving.
- Secondary: users of future supported compositors, reached through the same UI once a backend exists.

## Goals

- Inspect every connected output and its compositor-reported state without editing or applying anything.
- Arrange outputs visually, with positions expressed in logical pixels and validated for overlap.
- Change mode, refresh rate, scale, rotation, enablement, and variable refresh rate from one place.
- Apply changes safely: a 15-second confirmation, automatic revert on timeout, explicit revert, or window close.
- Roll back predictably on partial failure so a rejected change does not leave a half-configured desktop.
- Report unsupported compositors, missing outputs, lost connections, and invalid configuration clearly.
- Keep the UI backend-independent: adding a compositor backend adds no UI conditional branches beyond capability checks.

## v1 scope

v1 targets niri only and is runtime-only. It changes the running compositor configuration through niri IPC and never writes compositor configuration files. Closing Midori does not persist anything; closing niri or reloading its configuration reverts to the configuration file. A future persistence feature requires a separate approved design.

### v1 non-goals

- Configuration persistence, save-to-file, or config generation for any compositor.
- HDR, color management, ICC profiles, or gamma control.
- Brightness control.
- Mirroring.
- Custom modeline editing.
- Per-output layout overrides beyond the compositor's own state.
- Monitor calibration.
- A background daemon, autostart handling, or a system tray.
- Multi-seat or remote display management.
- X11, Windows, or macOS support.

## v1 functional requirements

| ID | Requirement |
| --- | --- |
| FR-1 | Detect niri and show the active backend with version or diagnostic status. |
| FR-2 | List outputs with connector, make/model/serial, enabled state, current mode, refresh, scale, transform, position, logical size, and VRR support/state. |
| FR-3 | Render outputs to relative scale in an arrangement canvas. |
| FR-4 | Select a mode from the backend's advertised modes (resolution plus refresh). |
| FR-5 | Change scale in the range 0.5–3.0 in steps of 0.05. |
| FR-6 | Change transform among all eight niri transforms. |
| FR-7 | Change position by drag and by numeric X/Y; snap edges; prevent overlap. |
| FR-8 | Enable or disable an output, keeping at least one output enabled. |
| FR-9 | Toggle variable refresh rate only when the output reports support and the backend supports it. |
| FR-10 | Apply changes with a 15-second confirmation and automatic revert on timeout. |
| FR-11 | Roll back on partial apply failure and on application close during confirmation. |
| FR-12 | Poll for output changes every 3 seconds while the window is open. |
| FR-13 | Show clear unsupported, no-output, connection-lost, and invalid-configuration states. |
| FR-14 | Auto-arrange enabled outputs into a single row. |
| FR-15 | Identify outputs by temporary numbered overlays. |

## Non-functional requirements

| ID | Requirement |
| --- | --- |
| NFR-1 | No root privileges. |
| NFR-2 | No network access and no telemetry. |
| NFR-3 | All backend work runs off the UI goroutine and uses a 5-second timeout. |
| NFR-4 | Race-free under `go test -race ./...` and at runtime with a single in-flight transaction. |
| NFR-5 | English-only UI strings and documentation. |
| NFR-6 | A single binary; no helper process and no daemon. |
| NFR-7 | Linux/Wayland only. |
| NFR-8 | No configuration-file writes. |

## User stories with acceptance criteria

### Inspect displays

Opening Midori inside a niri session lists every connected output and its current mode without changing anything.

Acceptance criteria:

- Every output from the niri `Outputs` reply appears in the list, including disabled outputs.
- The canvas shows enabled outputs at proportional positions and sizes.
- The inspector shows the current mode, refresh rate, scale, transform, position, and VRR state for the selected output.
- No backend mutation request is sent during load or selection.
- The version badge shows the niri version, or `unknown` when the version request fails, without blocking the output list.

### Change mode

Selecting a different refresh rate for the same resolution and applying changes the output. After 15 seconds without confirmation, the previous mode is restored.

Acceptance criteria:

- The apply request names the exact mode from the backend's advertised list.
- The confirmation popup appears with a countdown starting at 15 seconds.
- Leaving the countdown to expire restores the pre-apply mode on the compositor.
- Choosing `Keep changes` leaves the new mode active and clears `Unsaved changes`.
- Choosing `Revert now` restores the pre-apply mode immediately.

### Arrange displays

Dragging a display to the right of another creates adjacent positions; releasing with an overlap is rejected and the last valid position remains.

Acceptance criteria:

- Dragging updates the canvas and the inspector numeric X/Y fields.
- Releasing the drag near another output's edge snaps to that edge when the snap is valid.
- An overlapping drop is rejected; the tile returns to the last valid position and the draft keeps that position.
- Applying the arrangement updates compositor positions to the displayed logical coordinates.

### Disable and re-enable

Disabling removes the output from the canvas and from overlap checks; re-enabling places it at its draft position when valid, otherwise auto-places it.

Acceptance criteria:

- A disabled output remains listed in the inspector's output list with a `disabled` status.
- The canvas omits disabled outputs and overlap validation ignores them.
- Disabling the last enabled output is rejected with `At least one display must remain enabled.`
- Re-enabling in the same session restores the last draft values for that output when the position is still valid; otherwise the output is placed to the right of the enabled bounds.

### Unsupported compositor

Opening outside niri shows an explanation and no apply controls.

Acceptance criteria:

- The window shows `No supported compositor detected` with the niri-specific guidance and a `Retry` button.
- No output list, inspector, `Reset`, `Auto arrange`, or `Apply` control is interactive.
- `Retry` re-runs detection without restarting the process.

### Partial failure

If one output rejects a change, the other applied outputs are restored and the draft remains editable.

Acceptance criteria:

- Changes are applied sequentially and stop at the first failing output.
- Every attempted output, including the failing one, receives a best-effort rollback using the pre-apply snapshot.
- After a successful rollback the state returns to `Dirty` with the draft intact and an error banner is shown.
- If rollback also fails, the state becomes `Recovering` with a retry action and the error persists.

## Constraints and assumptions

- Runtime-only operation: compositor configuration files are never read for editing or written.
- niri baseline is `26.04` or newer; older versions may return unsupported replies, which surface as `ErrUnsupported`.
- One compositor instance per process; Midori does not coordinate with other display tools.
- A single Midori instance is assumed. Concurrent instances are not prevented, but serialization is only guaranteed inside one process.
- No privileged helper; Midori runs with the user's session permissions and uses the compositor's own IPC credentials.
- The compositor variables `$NIRI_SOCKET` and `$HYPRLAND_INSTANCE_SIGNATURE` are the only environment inputs for backend detection.
- The GUI must remain usable on a single 1366×768 display; the minimum window is `900×620`.

## Success criteria

- A user can inspect every output, change mode, scale, rotation, position, enablement, and VRR, apply, and safely revert without editing a file.
- No display configuration is written to disk at any point.
- Unsupported environments fail with an actionable message and no destructive control.
- Timeout, explicit revert, and window close during confirmation all restore the pre-apply state.
- Partial failure restores previously applied outputs or clearly reports that recovery is needed.
- Adding a second backend requires no UI conditional branches beyond capability checks.

## See also

- [Documentation index](README.md)
- [Architecture](architecture.md)
- [UI specification](ui.md)
- [niri backend](backends/niri.md)
- [Roadmap](roadmap.md)
