# Hyprland Backend (Future)

Future Hyprland adapter design: read-only inventory first, with an apply path that stays gated until rule preservation is verified.

Status: Design draft, not part of v1 implementation. Detection, listing, and capability reporting are specified for implementation in milestone M6; apply is specified for milestone M7 and must not ship before the gate in this document passes. See [../architecture.md](../architecture.md) and [../roadmap.md](../roadmap.md).

## Status and compatibility

- Future backend; do not implement as part of the v1 niri deliverable.
- Target Hyprland `0.55` or newer with `configProvider: "lua"`.
- Read-only inventory is the first supported Hyprland mode. It reports `Capabilities{Apply: false, VRR: true, Persistence: false}` and the UI shows the read-only banner.
- Detection and listing may work on older versions and on the `hyprlang` provider; apply on `hyprlang` is permanently read-only in this backend and returns `ErrUnsupported`.
- Apply is gated on one verification: the implementer MUST first confirm that `hl.monitor` preserves per-output fields that Midori does not manage. Relevant fields include color management, HDR, bit depth, ICC profiles, reserved area, and mirroring.
- If `hl.monitor` replaces the output rule and resets omitted fields, the backend MUST either carry the current values for every documented field from `hyprctl -j monitors all`, or remain read-only and return `ErrUnsupported`.
- Never ship an apply path that silently resets unmanaged display settings.
- Runtime-only: no Hyprland configuration file writes. `hyprctl eval` changes are lost on a config reload or restart.
- Capabilities after the gate passes: `Capabilities{Apply: true, VRR: true, Persistence: false}`.

## Discovery

Detection requires all of:

1. `HYPRLAND_INSTANCE_SIGNATURE` is non-empty.
2. `hyprctl` is on `PATH`.
3. `hyprctl -j monitors all` succeeds.

Status probe:

```sh
hyprctl -j status
```

```json
{"configProvider":"lua","backend":"drm"}
```

Rules:

- If `configProvider` is not `lua`, list outputs but set apply unavailable; `Backend.Apply` returns `ErrUnsupported`. The UI shows `Applying changes requires Hyprland 0.55+ with the Lua config provider.`
- The initial backend reports `Apply: false` and a read-only UI banner, even on Lua, until the preservation gate passes.
- Detection order in the registry is niri first, then Hyprland. niri wins when both probes succeed.
- `Backend.Version` runs `hyprctl -j version` and returns its `tag`, falling back to `commit`. If the command fails or neither field is present, return `unknown` without blocking listing.

## Read mapping

`hyprctl -j monitors all` returns an array. Compact example object with the fields used:

```json
[
  {
    "id": 0,
    "name": "DP-1",
    "description": "Dell Inc. U2723QE",
    "make": "Dell Inc.",
    "model": "U2723QE",
    "serial": "ABC123",
    "width": 2560,
    "height": 1440,
    "refreshRate": 143.912,
    "x": 0,
    "y": 0,
    "scale": 1.5,
    "transform": 0,
    "disabled": false,
    "vrr": false,
    "availableModes": ["2560x1440@143.91Hz", "2560x1440@60.00Hz"],
    "physicalWidth": 600,
    "physicalHeight": 340,
    "hardwareDetails": {"vrrCapable": true}
  }
]
```

Field mapping:

| hyprctl field | Domain field | Notes |
| --- | --- | --- |
| `name` | `Output.ID`, `Output.Name` | Connector name; sort key. |
| `make` | `Output.Make` | Empty string when absent. |
| `model` | `Output.Model` | Empty string when absent. |
| `serial` | `Output.Serial` | Empty string when absent. |
| `description` | Diagnostic only | Never used as identity. |
| `physicalWidth`, `physicalHeight` | `Output.PhysicalSize` | Millimetres. `nil` when missing or zero. |
| `width`, `height` | Current mode dimensions | Physical pixels. |
| `refreshRate` | Current mode refresh | Hertz; convert to millihertz. |
| `x`, `y` | `OutputState.Position` | Logical coordinates. |
| `scale` | `OutputState.Scale` | |
| `transform` | `OutputState.Transform` | Integer 0–7; see mapping below. |
| `disabled` | `OutputState.Enabled` | Inverted. |
| `vrr` | `OutputState.VRR` | |
| `hardwareDetails.vrrCapable` | `Output.VRRSupported` | Missing becomes false. |
| `availableModes` | `Output.Modes` | Strings; parsed as below. |

Mode parsing:

- Parse each entry with `^([0-9]+)x([0-9]+)@([0-9]+(?:\.[0-9]+)?)Hz$`.
- Unparsable entries are skipped with a debug log; they are not a fatal error.
- Convert hertz to millihertz by rounding to the nearest millihertz.

Current mode:

- Match an available mode by width and height and refresh within `0.01 Hz`.
- If no available mode matches, synthesize a current mode from `width`, `height`, and `refreshRate`, and make it non-selectable. The UI shows the read-only `Current (custom)` row.
- If `width` or `height` is zero, the monitor is disabled; do not synthesize a mode.

Logical size:

- Compute it from physical width and height, scale, and transform using the formula in [../architecture.md](../architecture.md#layout-math).
- Hyprland does not report a separate logical size in `monitors all`; use the computed value.

Disabled monitors:

- Include them in the output list.
- Their `width` and `height` may be zero; keep them listed and do not treat zero geometry as a parse failure.

Robustness:

- Treat missing optional JSON fields (`serial`, `vrr`, `hardwareDetails`, `availableModes`) as empty, false, or unsupported.
- Never panic on schema drift. Unknown fields are ignored; missing required fields such as `name` cause that monitor to be skipped with a debug log.

Mode token cache:

- During `Outputs`, cache a per-output map from `(width, height, refreshMilliHz)` to the original `availableModes` token with the trailing `Hz` removed, for example `2560x1440@143.91`.
- `Apply` uses the cached token verbatim and falls back to `WxH@%.2f` only when the cache has no entry.
- Guard the cache with a mutex; `Outputs` and `Apply` can run from different goroutines.

Sorting:

- Sort by monitor name before returning.

## Transform mapping

| Domain | hyprctl integer |
| --- | --- |
| `TransformNormal` | 0 |
| `Transform90` | 1 |
| `Transform180` | 2 |
| `Transform270` | 3 |
| `TransformFlipped` | 4 |
| `TransformFlipped90` | 5 |
| `TransformFlipped180` | 6 |
| `TransformFlipped270` | 7 |

Reads convert the integer back to the domain enum. An integer outside 0–7 is `ErrUnsupported`, not a silent `Normal`.

## Apply mapping (gated)

This path is enabled only after the rule-preservation gate in [Status and compatibility](#status-and-compatibility) passes, and only on the Lua config provider.

Example command:

```sh
hyprctl eval 'hl.monitor({ output = "DP-1", mode = "2560x1440@143.91", position = "0x0", scale = 1.5, transform = 0, disabled = false, vrr = 0 })'
```

The implementation calls `exec.CommandContext("hyprctl", "eval", luaExpr)`, passing the expression as one argv element. There is no shell, so the single quotes above are documentation of an interactive shell invocation, not part of the argv.

Expression construction:

- Build one `hl.monitor` table per changed output with the full desired state: `output`, `mode`, `position`, `scale`, `transform`, `disabled`, `vrr`.
- Also carry every unmanaged field that the preservation gate requires from the current monitor JSON.
- Preserve the original `availableModes` token for the selected mode, with the trailing `Hz` removed, and use it verbatim in `mode`. This avoids refresh formatting mismatches. If no token exists, format `WxH@%.2f`.
- Position format is `"XxY"` including negative coordinates, for example `"-1920x0"`.
- Scale uses `%g`.
- `vrr` maps to `0` or `1` in v1.
- A disabled output sends `disabled = true` and omits `mode`, `position`, `scale`, `transform`, and `vrr`.

Output name safety:

- Validate the output name against `^[A-Za-z0-9._:-]+$` before interpolating it into the expression.
- Reject any other name with `ErrUnsupported` rather than attempting Lua escaping. This prevents injection through connector names.

Result handling:

- Non-zero exit or output other than `ok` is an error wrapping `ErrConnection` or `ErrUnsupported` as appropriate.
- Timeout is 5 seconds per call.
- No configuration-file writes; `eval` is runtime-only.

## Sequence and rollback

- Same service transaction as niri: fresh snapshot, per-output apply in connector-name order, reload, 15-second confirmation, and automatic revert on timeout, explicit revert, or window close.
- Because each `hl.monitor` call replaces the full output rule, apply exactly one output per call; never batch multiple outputs into one expression.
- The service sends `Change{Before, After}`. Reverting re-applies `Before` with the same mapping.
- Partial failure follows the same best-effort rollback contract as [../architecture.md](../architecture.md#apply-transaction-algorithm).

## Limitations

- No apply on the `hyprlang` provider; apply requires `0.55+` with Lua.
- No VRR on-demand modes in v1; only on/off.
- `availableModes` strings are the source of truth for selectable modes.
- Hyprland positions are logical and interact with scale and transform; Midori uses the compositor's logical coordinates directly.
- No persistence and no configuration-file writes; `hyprctl eval` changes are runtime-only and are lost on config reload or restart.
- Future Hyprland versions may change Lua table fields. Probe and return `ErrUnsupported` on command errors rather than guessing.
- The preservation gate can keep the backend read-only indefinitely if `hl.monitor` cannot preserve unmanaged fields safely.

## Testing

- JSON fixture parsing for `monitors all`, including disabled monitors, missing optional fields, zero geometry, and empty `availableModes`.
- Mode token parser table tests: valid tokens, fractional refresh, malformed tokens, and rounding to millihertz.
- Transform round-trip tests for all eight values and rejection of out-of-range integers.
- Status provider detection: `lua`, `hyprlang`, missing field, and command failure.
- Version parsing: `tag` present, `commit` fallback, and neither field.
- Fake `hyprctl` executable script placed first on a test-specific `PATH` for success, error, and timeout cases.
- Exact argv assertion for the `eval` call, including the Lua expression string and the output name validation rejection.
- No live Hyprland test in CI.

## References

- Hyprland monitors wiki: https://wiki.hypr.land/Configuring/Basics/Monitors/
- Hyprland monitor modes source: https://github.com/hyprwm/hyprland-wiki/blob/main/content/configuring/core/monitors/modes.md
- Hyprland monitor positioning source: https://github.com/hyprwm/hyprland-wiki/blob/main/content/configuring/core/monitors/positioning.md
- Hyprland hyprctl man page: https://github.com/hyprwm/Hyprland/blob/main/docs/hyprctl.1.rst
- Hyprland monitors JSON source: https://github.com/hyprwm/Hyprland/blob/main/src/ipc/s1/Commands.cpp
- Hyprland status JSON source: https://github.com/hyprwm/Hyprland/blob/main/src/helpers/SystemInfo.cpp
