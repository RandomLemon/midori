# niri Backend

Exact niri IPC integration contract: discovery, wire framing, JSON mapping, apply order, and limitations.

Status: Design draft. The backend uses Outputs, Version, and the apply actions through the service transaction. Fake-socket tests cover the wire format and action order. Backend behavior is invoked by the transaction in [../architecture.md](../architecture.md). Product scope is in [../product.md](../product.md).

## Scope and compatibility

- niri `26.04` or newer.
- Direct Unix-socket IPC on `$NIRI_SOCKET`; no `niri msg` subprocess for core operations.
- Runtime-only: requests change the running compositor and never write configuration files.
- Replies that this backend does not recognize, including newer action or reply variants, become `ErrUnsupported`.
- Capabilities: `Capabilities{Apply: true, VRR: true, Persistence: false}`.
- niri documents output IPC changes as temporary. They are forgotten if the output configuration in the config file subsequently changes, and they do not survive a compositor restart.

## Discovery and connection

- Read `$NIRI_SOCKET`. A missing or empty value means niri is not available; detection fails with `ErrNoBackend` at the registry level and the backend itself reports `ErrConnection`.
- Dial the Unix socket with a 2-second timeout.
- Use one short-lived connection per request: dial, write one JSON line, read one JSON line, close.
- Do not subscribe to the event stream. niri's event stream has no output topology event, so the application polls `Outputs` every 3 seconds instead.
- Per-request timeout is 5 seconds, covering dial, write, and read.
- Reply line limit is 8 MiB. A longer line is a protocol error and is never fully buffered.
- If niri restarts, the socket path changes. The backend reports `ErrConnection`; the UI shows `Lost connection to niri. Restart Midori.` because the stored path is stale and Midori does not re-read the environment.

## Protocol framing

- Newline-delimited JSON: one request line, one reply line.
- A reply is either `{"Ok": ...}` or `{"Err": "..."}`.
- Requests:

```json
"Outputs"
```

```json
"Version"
```

```json
{"Output":{"output":"HDMI-A-1","action":"Off"}}
```

```json
{"Output":{"output":"HDMI-A-1","action":{"Mode":{"mode":{"Specific":{"width":2560,"height":1440,"refresh":143.912}}}}}}
```

```json
{"Output":{"output":"HDMI-A-1","action":{"Scale":{"scale":{"Specific":1.5}}}}}
```

```json
{"Output":{"output":"HDMI-A-1","action":{"Transform":{"transform":"90"}}}}
```

```json
{"Output":{"output":"HDMI-A-1","action":{"Position":{"position":{"Specific":{"x":0,"y":0}}}}}}
```

```json
{"Output":{"output":"HDMI-A-1","action":{"Vrr":{"vrr":{"vrr":true,"on_demand":false}}}}}
```

- Replies:

```json
{"Ok":{"OutputConfigChanged":"Applied"}}
```

```json
{"Ok":{"OutputConfigChanged":"OutputWasMissing"}}
```

```json
{"Ok":{"Version":"26.04"}}
```

```json
{"Err":"..."}
```

Wire-format detail: unit variants are JSON strings, while struct variants are externally tagged objects with a single key naming the variant. This is Rust serde's externally tagged enum representation and is the exact wire format; it is not a stylistic choice. The `Vrr` action contains a `VrrToSet` struct, so its `vrr` object nests: `{"Vrr":{"vrr":{"vrr":true,"on_demand":false}}}`. Do not flatten it: a form such as `{"Vrr":{"vrr":true,"on_demand":false}}` is rejected by the derived encoder. Confirm every request against the target niri version's `niri-ipc` serialization, and where available with `niri msg --print-request`, then use the captured bytes in golden tests.

`Version` behavior:

- `Backend.Version` sends `"Version"` and returns the string from `{"Ok":{"Version":"..."}}`.
- A parse or connection error is returned to the caller; the UI shows `unknown` without blocking output listing.

`Outputs` behavior:

- `Backend.Outputs` sends `"Outputs"` and decodes `{"Ok":{"Outputs":{...}}}`.
- Any `{"Err":...}` reply becomes a wrapped backend error; an unrecognized top-level reply becomes `ErrUnsupported`.

## `Outputs` reply mapping

Compact example with one enabled and one disabled output:

```json
{"Ok":{"Outputs":{
  "eDP-1":{
    "name":"eDP-1",
    "make":"AU Optronics",
    "model":"0x1234",
    "serial":"0x0000",
    "physical_size":[344,215],
    "modes":[
      {"width":1920,"height":1200,"refresh_rate":60000,"is_preferred":true},
      {"width":1920,"height":1200,"refresh_rate":48000,"is_preferred":false}
    ],
    "current_mode":0,
    "is_custom_mode":false,
    "vrr_supported":false,
    "vrr_enabled":false,
    "logical":{"x":0,"y":0,"width":1920,"height":1200,"scale":1.0,"transform":"Normal"}
  },
  "HDMI-A-1":{
    "name":"HDMI-A-1",
    "make":"Dell Inc.",
    "model":"U2723QE",
    "serial":null,
    "physical_size":[600,340],
    "modes":[
      {"width":2560,"height":1440,"refresh_rate":143912,"is_preferred":true}
    ],
    "current_mode":null,
    "is_custom_mode":false,
    "vrr_supported":true,
    "vrr_enabled":false,
    "logical":null
  }
}}}
```

Field mapping:

| niri field | Domain field | Notes |
| --- | --- | --- |
| `name` | `Output.ID` and `Output.Name` | Connector name; sort key. |
| `make` | `Output.Make` | Empty string when absent. |
| `model` | `Output.Model` | Empty string when absent. |
| `serial` | `Output.Serial` | Empty string when absent or null. |
| `physical_size` `[width,height]` | `Output.PhysicalSize` | Millimetres; null or missing becomes `nil`. |
| `modes[].width`, `modes[].height` | `Mode.Width`, `Mode.Height` | The custom entry at index 0 is excluded from `Output.Modes` when `is_custom_mode` is true. |
| `modes[].refresh_rate` | `Mode.RefreshMilliHz` | niri reports millihertz. |
| `modes[].is_preferred` | `Mode.Preferred` | |
| `current_mode` | Index into the wire `modes` array; resolve it to `OutputState.Mode` before filtering custom modes. | |
| `is_custom_mode` | `Output.IsCustomMode` | True when the current mode is a user-defined custom mode. |
| `vrr_supported` | `Output.VRRSupported` | |
| `vrr_enabled` | `OutputState.VRR` | |
| `logical.x`, `logical.y` | `OutputState.Position` | Logical pixels. |
| `logical.width`, `logical.height` | `Output.LogicalSize` | Compositor-reported logical size. |
| `logical.scale` | `OutputState.Scale` | |
| `logical.transform` | `OutputState.Transform` | Serde spelling, see below. |

Enabled detection:

- An output is enabled when `current_mode != nil && logical != nil`.
- Otherwise it is disabled, including inconsistent combinations such as a current mode without logical geometry. Log inconsistent combinations at debug level and treat them as disabled.
- When `current_mode` is out of range, treat the output as disabled and surface a diagnostic; do not panic.
- A disabled output's `Mode`, `Scale`, `Transform`, and `Position` are zero values in the snapshot. The draft derives defaults per [../architecture.md](../architecture.md#validation).

Sorting:

- Sort the response map by connector name before returning.

Custom modes:

- When `is_custom_mode` is true, niri inserts the custom current mode at index `0` of the wire `modes` array and reports `current_mode: 0`. Resolve that entry into `OutputState.Mode` with `IsCustomMode: true`, then exclude it from the selectable `Modes` list so the UI cannot offer it as an advertised mode. The custom mode is shown read-only and validation allows it only when unchanged.

## Transform mapping

niri IPC uses serde spellings, not KDL spellings:

| Domain | JSON value | KDL config spelling |
| --- | --- | --- |
| `TransformNormal` | `"Normal"` | `normal` |
| `Transform90` | `"90"` | `90` |
| `Transform180` | `"180"` | `180` |
| `Transform270` | `"270"` | `270` |
| `TransformFlipped` | `"Flipped"` | `flipped` |
| `TransformFlipped90` | `"Flipped90"` | `flipped-90` |
| `TransformFlipped180` | `"Flipped180"` | `flipped-180` |
| `TransformFlipped270` | `"Flipped270"` | `flipped-270` |

The KDL spellings (`flipped-90`) are not accepted by the IPC socket. Send and expect the JSON serde spellings exactly as listed. An unknown transform string is `ErrUnsupported`, not a silent `Normal`.

## Mode and refresh mapping

- Convert millihertz to hertz with `refresh = float64(RefreshMilliHz) / 1000.0`.
- Send refresh as a JSON number in the `Mode` request, for example `143.912`.
- The refresh must match niri's advertised value exactly to three decimal places; do not round a user-selected mode to a different value.
- Display three decimals for niri modes and trim trailing zeros: `143.912 Hz`, `60 Hz`.
- Mode comparison for change detection compares width, height, and `RefreshMilliHz` exactly.
- The request uses the specific mode `{width, height, refresh}`; niri matches it against an advertised mode or rejects it.

## Apply algorithm

For each `Change{Before, After}`:

- If both `Before.Enabled` and `After.Enabled` are false: no-op.
- If disabling: send `Off` only.
- If enabling from disabled: send `On`, then mode, scale, transform, position, and VRR. Send all of them because the compositor may have selected a default mode on enable.
- If the output stays enabled: send only the fields that differ, in the order mode → scale → transform → position → VRR.
- Field comparison:
  - Mode: width, height, and refresh millihertz.
  - Scale: epsilon `1e-4`.
  - Transform: exact.
  - Position: exact X and Y.
  - VRR: exact.
- Each action is a separate `Output` request; one request per changed field.
- `OutputConfigChanged:"Applied"` is success.
- `OutputConfigChanged:"OutputWasMissing"` becomes `ErrOutputMissing`.
- An `{"Err":...}` reply becomes a wrapped backend error that includes the output name and the action.
- Do not retry automatically. The service owns the 15-second confirmation, rollback, and auto-revert; this backend only executes per-output actions.

Example sequence when enabling a previously disabled output and changing its mode, scale, and position:

```text
{"Output":{"output":"HDMI-A-1","action":"On"}}
{"Output":{"output":"HDMI-A-1","action":{"Mode":{"mode":{"Specific":{"width":2560,"height":1440,"refresh":143.912}}}}}}
{"Output":{"output":"HDMI-A-1","action":{"Scale":{"scale":{"Specific":1.5}}}}}
{"Output":{"output":"HDMI-A-1","action":{"Transform":{"transform":"90"}}}}
{"Output":{"output":"HDMI-A-1","action":{"Position":{"position":{"Specific":{"x":0,"y":0}}}}}}
{"Output":{"output":"HDMI-A-1","action":{"Vrr":{"vrr":{"vrr":true,"on_demand":false}}}}}
```

## Limitations

- No output-topology events: niri's IPC event stream has no output event, so the application polls every 3 seconds.
- Runtime-only: changes are forgotten on a config change, config reload, or compositor restart.
- Disabled outputs have no logical geometry; the snapshot keeps zero values and the draft supplies defaults.
- VRR on-demand state is not reported by `Outputs`, so v1 exposes VRR on/off only and always sends `on_demand: false`.
- Custom modes are read-only; v1 has no custom-mode editor.
- `MaxBpc` exists in niri IPC but is not exposed in v1.
- A stale `$NIRI_SOCKET` after a niri restart is not re-resolved; the user must restart Midori.

## Testing

- Golden JSON tests for every request: `Outputs`, `Version`, `Off`, `On`, `Mode`, `Scale`, `Transform`, `Position`, `Vrr`.
- Fixture tests for `Outputs` covering: enabled output, disabled output, custom mode, VRR-supported output, output without logical geometry, null serial, null physical size, out-of-range `current_mode`.
- Fake Unix-socket server test for the full round trip: request bytes in, fixture reply out, decoded snapshot compared.
- Timeout test: a server that never replies produces a context deadline error within the 5-second budget (use a shortened context in tests).
- Dial timeout test for a missing socket path.
- Malformed JSON and oversized-line tests produce errors, not panics, and do not allocate beyond the 8 MiB limit.
- `OutputWasMissing` test maps to `ErrOutputMissing`.
- No live niri test runs in CI; an opt-in live test is described in [../development.md](../development.md).

## References

- niri IPC wiki: https://github.com/niri-wm/niri/wiki/IPC
- niri outputs wiki: https://github.com/niri-wm/niri/wiki/Configuration:-Outputs
- niri-ipc source: https://github.com/niri-wm/niri/blob/main/niri-ipc/src/lib.rs
- niri releases: https://github.com/niri-wm/niri/releases
