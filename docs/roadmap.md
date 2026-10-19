# Roadmap

Phased implementation order with observable acceptance criteria for each milestone.

Status: Design draft. Documentation is milestone M0. Each milestone ends with the affected docs and tests updated, plus a manual smoke test where the milestone changes backend or UI behavior. Scope boundaries are defined in [product.md](product.md); contracts are in [architecture.md](architecture.md).

## M0 Documentation

Deliverable: this documentation set.

Status: complete.

Acceptance criteria:

- `AGENTS.md` and all documents in `docs/` exist and are English-only.
- Every relative link resolves.
- At M0 completion, no `.go` file, `go.mod`, `cmd/`, `internal/`, CI file, or root README existed.
- The document map in [README.md](README.md) lists every document.
- Constants are consistent across all documents.

Docs to update on completion: all of them.

## M1 Skeleton and niri read-only

Status: complete.

Work:

- Add `go.mod` and `cmd/midori`.
- Implement `internal/display` domain types, `Backend`, `Service.Load`, and validation scaffolding.
- Implement the niri IPC client and codec for `Outputs` and `Version`.
- Implement backend registry auto-detection.
- Implement a read-only UI: output list, arrangement canvas, inspector details without editing.
- Implement `internal/testutil/fakebackend`.
- Add the 3-second poll and the loading, unsupported, no-output, and lost-connection states.

Acceptance criteria:

- Running under niri lists every connected output with connector, mode, refresh, scale, transform, position, logical size, and VRR state.
- No mutation request is sent.
- Fake IPC tests cover `Outputs` and `Version`, including malformed replies.
- `go test ./...` passes.
- The UI does not freeze during backend calls.

Docs to update: [development.md](development.md) if commands differ, [README.md](README.md) status, [roadmap.md](roadmap.md) completion notes.

## M2 Inspector and apply/revert

Status: complete.

Work:

- Add mode, refresh, scale, rotation, enablement, and VRR editing with validation.
- Implement the apply transaction: fresh snapshot, per-output apply, reload, confirmation, rollback.
- Add the 15-second confirmation popup and automatic revert.
- Add `Reset` and the dirty/clean state transitions.
- Add close interception for `Dirty`, `Applying`, `Confirming`, and `Reverting`.

Acceptance criteria:

- Applying a mode changes the output.
- Leaving the confirmation countdown to expire restores the previous mode.
- `Revert now` restores immediately; `Keep changes` keeps the new state.
- A partial-failure test restores every attempted output and leaves the draft editable.
- Applying the last-enabled-output disable is rejected.
- No display configuration is written to disk.

Docs to update: [ui.md](ui.md) if copy or control order changes, [architecture.md](architecture.md) if the transaction changes, [README.md](README.md) status.

## M3 Arrangement editor

Status: complete.

Work:

- Add drag positioning with logical-pixel conversion.
- Add edge snapping within 12 screen pixels.
- Add overlap rejection, numeric X/Y entry, auto-arrange, and identify overlays.
- Add preview logical-size computation for desired states.

Acceptance criteria:

- Two enabled outputs can be placed side by side.
- An overlapping drop is rejected and the last valid position remains.
- Numeric X/Y entry and drag stay synchronized.
- Positions survive `Apply` and are restored by revert.
- `Auto arrange` produces a single row with zero gaps.
- `Identify` shows numbered overlays for 3 seconds and sends no backend requests.

Docs to update: [ui.md](ui.md), [architecture.md](architecture.md) if layout math changes.

## M4 Robustness

Status: complete.

Work:

- Complete 3-second polling and fingerprint comparison.
- Add hotplug banners and draft discard on change.
- Add lost-connection, unsupported, no-output, and recovering states.
- Add close interception for all states.
- Add structured logging and `--debug`.
- Add socket timeout, oversized-line, and malformed-reply handling.

Acceptance criteria:

- Unplugging and replugging an output updates the list and canvas without restarting.
- Killing the niri socket moves to the lost-connection state after three consecutive poll failures.
- Closing during apply performs rollback before exit.
- A malformed or oversized reply produces an error, not a panic.
- `gofmt -l .` is empty, and `go vet ./...`, `go test ./...`, and `go test -race ./...` pass.

Docs to update: [ui.md](ui.md) for any state copy changes, [development.md](development.md) for the manual matrix, [architecture.md](architecture.md) for polling changes.

## M5 Packaging

Status: not started; next milestone.

Work:

- Add the Fyne packaging step, icon, and desktop entry.
- Add version injection through ldflags.
- Add install instructions.

Acceptance criteria:

- The package launches from the application menu under niri.
- The application ID is `io.github.randomlemon.midori`.
- The packaged binary uses the Wayland driver.
- No daemon or systemd unit is installed.

Docs to update: [development.md](development.md), [README.md](README.md), this roadmap.

## M6 Hyprland read-only

Work:

- Add the Hyprland detection probe: `HYPRLAND_INSTANCE_SIGNATURE`, `hyprctl` on `PATH`, `hyprctl -j monitors all`.
- Add `hyprctl -j status` config-provider detection.
- Map monitors, modes, transforms, and capabilities into the domain.
- Report `Apply: false` and show the read-only banner.
- Return `ErrUnsupported` for apply on `hyprlang` and for any unverified Lua field-preservation situation.

Acceptance criteria:

- Outputs list correctly on Hyprland `0.55+` with the Lua provider.
- No apply controls are interactive; `Identify` and output details remain available.
- No UI forks: the only differences are capability-driven.
- Hyprland fixture, parser, and fake-`hyprctl` tests pass.
- No live Hyprland test runs in CI.

Docs to update: [backends/hyprland.md](backends/hyprland.md), [README.md](README.md), [architecture.md](architecture.md) package layout if files are added, this roadmap.

## M7 Hyprland apply (gated)

Work:

- Verify `hl.monitor` preservation of unmanaged fields, or carry every documented field from `monitors all`.
- Enable apply only on the Lua provider after the gate passes.
- Implement `hyprctl eval` expression construction with cached mode tokens and output-name validation.
- Reuse the same transaction, 15-second confirmation, and auto-revert.

Acceptance criteria:

- Applying a mode changes the output and, after the 15-second countdown expires, restores the previous mode.
- Unmanaged color, HDR, bit-depth, ICC, reserved-area, and mirror fields are unchanged across apply, verified in a live session.
- Output names outside `^[A-Za-z0-9._:-]+$` are rejected before command construction.
- The exact argv is asserted in tests.
- The backend remains read-only on the `hyprlang` provider.

Docs to update: [backends/hyprland.md](backends/hyprland.md), [architecture.md](architecture.md), [development.md](development.md) for the live test, [README.md](README.md), this roadmap.

## M8 Persistence (deferred)

Explicitly not started. Persistence requires a separate approved design covering configuration backups, parsing and formatting preservation, reload semantics, and failure recovery. Do not include implementation tasks for it in this roadmap, and do not add a Save action without that design.

Docs to update before work starts: [architecture.md](architecture.md), [product.md](product.md), [ui.md](ui.md), [development.md](development.md), [README.md](README.md), and this roadmap.

## See also

- [Product requirements](product.md)
- [Architecture](architecture.md)
- [Documentation index](README.md)
