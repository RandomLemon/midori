# User Interface Specification

Pixel-level and interaction-level specification for the Fyne UI, including exact copy for empty, error, and confirmation states.

Status: Design draft. M1 implements the read-only subset: header, output list, canvas, inspector details, loading and error states. Editing controls, the apply bar, and confirmation remain M2/M3. Behavior contracts live in [architecture.md](architecture.md); scope lives in [product.md](product.md).

## Principles

- KDE-like mental model: a display list, an arrangement canvas, and a per-display inspector.
- Visual arrangement first: position and size changes are made on the canvas, with numeric entry as an alternative.
- Every apply is reversible: the confirmation popup is the only path from editing to a kept change.
- No hidden destructive action: `Reset` never calls the backend, and closing with a draft asks before discarding.
- Capability-driven controls: controls that a backend cannot honor are hidden or disabled with an explanation.
- English-only copy, sentence case, no icon-only actions.

## Window layout

- Default size: `1120×760`; minimum size: `900×620`.
- The window is a vertical stack: header, banner slot, main horizontal area, footer action bar.

Header (56 px):

- Title `Display Settings`.
- Backend badge: `niri 26.04` or `Hyprland <tag>` when the version is known, otherwise the backend display name plus `unknown`, for example `niri unknown`.
- `Refresh` button. It is enabled only in `Ready`. While `Dirty` it is disabled with the tooltip `Reset changes before refreshing`. While `Applying`, `Confirming`, or `Reverting` it is disabled.

Banner slot:

- Sits directly below the header.
- Hidden when there is no warning or error.
- Shows hotplug, stale-snapshot, read-only, and non-modal error messages.
- A banner never blocks the inspector or canvas.

Main horizontal area:

- Left: arrangement canvas, expands with the window.
- Right: inspector, fixed width `360`, vertically scrollable.
- A 1 px separator divides the two.

Canvas toolbar:

- `Auto arrange` and `Identify`, placed above the canvas and right-aligned.
- Both are disabled while busy; `Auto arrange` is also disabled for a read-only backend.

Footer action bar (64 px):

- Left: status text.
- Right: `Reset`, then `Apply`.

ASCII wireframe:

```text
+------------------------------------------------------------------------------+
| Display Settings                                  niri 26.04       [Refresh] |
+------------------------------------------------------------------------------+
| [!] Display configuration changed. Pending edits were discarded.             |
+-----------------------------------------------------+------------------------+
|                                          [Auto arrange] [Identify]           |
|                                                     |  Outputs               |
|   +----------------------+  +-------------------+   |  [eDP-1  enabled  ]    |
|   | eDP-1                |  | HDMI-A-1          |   |  [DP-2   disabled ]    |
|   | 1920x1200 @ 60.000   |  | 2560x1440 @ 143.9 |   |------------------------|
|   | 100%                 |  | 150%              |   |  HDMI-A-1              |
|   +----------------------+  +-------------------+   |  Dell Inc. U2723QE     |
|                                                     |  [x] Enabled           |
|                                                     |  Resolution  [2560x1440]|
|                                                     |  Refresh     [143.912 Hz]|
|                                                     |  Scale       [--o----] |
|                                                     |  Rotation    [Normal ] |
|                                                     |  [ ] Variable refresh  |
|                                                     |  Position X [0] Y [0]  |
+-----------------------------------------------------+------------------------+
| Unsaved changes                                    [Reset]        [Apply]     |
+------------------------------------------------------------------------------+
```

## Output list and selection

- The inspector top contains a compact list of all outputs, including disabled ones.
- Each row shows the connector and a status word: `enabled`, `disabled`, or `read-only`.
- Canvas tile selection and list selection are synchronized in both directions.
- Selecting a disabled output keeps it selected and shows its draft values.
- Selecting an output never sends a backend request.
- If the selected output disappears, select the first remaining output; if none remain, show the no-output state.
- List rows use the connector as the primary label and the make/model as the secondary label when present.

## Arrangement canvas

Behavior:

- Auto-fit; no manual pan or zoom in v1.
- 32 px padding on all sides.
- Tiles are positioned by logical coordinates and scaled by zoom from the [layout math](architecture.md#layout-math).
- Selected tile border uses the Fyne primary color, 2 px. Invalid tiles use the error color. Unselected tiles use a 1 px normal border.
- Disabled outputs are not drawn on the canvas but remain in the output list.
- The canvas background uses the theme's input background color; no grid or ruler in v1.

Tile content, top to bottom:

- Connector.
- `W×H @ refresh` using up to three decimals for refresh, trimming trailing zeros.
- Scale percentage, for example `150%`.
- Rotation label when the transform is not `Normal`, for example `90°`.
- `Disabled` label if applicable. Disabled tiles are not drawn, so this label appears only in transient preview states, not in the normal canvas.

If a tile is narrower than 100 screen pixels, show only the connector.

Drag:

- The tile implements `fyne.Draggable`.
- Convert screen deltas to logical pixels by dividing by zoom; round to integers; update the draft.
- Apply snapping from [architecture.md](architecture.md#layout-math).
- Reject an overlapping drop by reverting to the last valid position.
- Update the inspector X/Y fields when the drag ends, not on every motion event.

Selection:

- Click or tap selects the tile.
- No double-click action.

Canvas actions:

- `Auto arrange` applies the auto-arrange algorithm from [architecture.md](architecture.md#layout-math) to enabled outputs.
- `Identify` overlays large sequence numbers `1..N` on enabled tiles for 3 seconds, matching the order of the output list. It sends no backend requests.

## Inspector controls

Controls appear in this exact order.

1. **Output header** — connector, make/model, serial if present, physical size if known, in that order. Missing values are omitted, not shown as placeholders.
2. **Enabled** switch — labeled `Enabled`. Toggling off then on within the session restores the last draft values for that output.
3. **Resolution** select — unique `W×H` values from the output's `Modes`, sorted by area descending, then width, then height. Label `W×H`, for example `2560×1440`.
4. **Refresh rate** select — the values available for the selected resolution, sorted descending. Label with up to three decimal places and ` Hz`, for example `143.912 Hz`, trimming trailing zeros (`60 Hz`). Mark preferred values with ` (Preferred)`. Default to the preferred mode for that resolution, otherwise the highest refresh; preserve the current selection when possible.
5. **Scale** slider — range 0.5–3.0 in steps of 0.05, label as a percentage with no space, for example `150%`. If the current value is outside the range, expand the range to include it without changing the value until the user moves the slider.
6. **Rotation** select — `Normal`, `90°`, `180°`, `270°`, `Flipped`, `Flipped 90°`, `Flipped 180°`, `Flipped 270°`.
7. **Variable refresh rate** switch — labeled `Variable refresh rate`. Visible only when `Capabilities.VRR` and the output's `VRRSupported` are both true. Hidden otherwise, with no placeholder.
8. **Position** — X and Y numeric entries, integer validation. Edits update the draft and canvas immediately. An invalid or overlapping value shows an inline error on the field.
9. **Custom current mode** — when `IsCustomMode` is true, show a read-only row labeled `Current (custom)` with `W×H @ refresh`. Selecting an advertised mode replaces it. v1 never offers a custom-mode editor.
10. **Disabled output** — the inspector remains editable. Show the default draft values: preferred mode, scale `100%`, rotation `Normal`, VRR off, and the auto-placed position. Re-enabling in the same session restores that output's last draft values. Disabled outputs are omitted from the canvas and from overlap validation until enabled, but their draft position is retained and validated on re-enable.

Field-specific validation messages appear inline and mirror the exact strings in [Empty, unsupported, and error states](#empty-unsupported-and-error-states).

## Draft, reset, apply bar

- Edits go to a draft; the snapshot is unchanged until an apply succeeds and is kept.
- Dirty state enables `Reset` and `Apply` and shows `Unsaved changes` in the footer.
- Clean state disables both and shows `Session-only changes`, with the tooltip `Changes apply to the running compositor and are not saved to its configuration.`
- `Reset` discards the draft without backend calls and restores inspector and canvas values from the snapshot.
- `Apply` is disabled when clean, invalid, busy, or when `Capabilities.Apply` is false.
- While applying, the footer shows `Applying...` and all controls are disabled.
- Closing the window while dirty shows `Discard unsaved changes?` with `Discard` and `Cancel`; `Cancel` keeps the window and the draft.

## Confirmation popup

- A modal `widget.NewModalPopUp`, not a dismissible dialog.
- Title: `Keep this display configuration?`
- Countdown: `Reverting in 15 s`, updated every second.
- Buttons: `Revert now` and `Keep changes`.
- The popup is centered on the main window.
- Clicking outside the popup does nothing; there is no dimmed-area dismissal.
- Escape triggers `Revert now`.
- Closing the main window triggers the same revert path as `Revert now`.
- Timeout at zero triggers the revert path.
- While reverting, the popup shows `Reverting...` and disables both buttons.
- After `Keep changes`, the popup closes and the footer returns to clean state with `Session-only changes`.

## Empty, unsupported, and error states

Exact copy:

| State | Copy | Actions |
| --- | --- | --- |
| Loading | `Loading displays...` with a progress indicator. | None. |
| No supported compositor | `No supported compositor detected` / `Midori currently supports niri. Start it inside a niri session.` | `Retry` |
| No outputs | `No displays detected` | `Refresh` |
| Lost connection | `Lost connection to niri. Restart Midori.` | None; the window stays open. |
| Apply failure after successful rollback | `Could not apply display configuration` / `The previous configuration was restored.` | None; the draft remains editable. |
| Stale snapshot conflict before any change | `Display configuration changed. Review the settings and apply again.` | None; the draft is preserved for editing. |
| Rollback failure | `The display configuration could not be restored. Restart Midori or fix the configuration manually.` | `Retry` |
| Overlap | `Displays must not overlap.` | Inline on the position field. |
| All disabled | `At least one display must remain enabled.` | Inline on the `Enabled` switch. |
| Invalid mode | `Select an available mode.` | Inline on the resolution select. |
| Scale | `Scale must be between 50% and 300%.` | Inline on the scale slider. |
| Hyprland hyprlang provider | `Applying changes requires Hyprland 0.55+ with the Lua config provider.` | Banner; apply disabled. |
| Read-only backend capability | `This compositor backend is read-only in this build.` | Banner; inspector controls, `Auto arrange`, `Reset`, and `Apply` disabled; output details and `Identify` remain available. |

Rules:

- A banner is used for state-level messages; inline errors are used for field-level validation.
- The unsupported state hides the inspector and canvas and shows only the message and `Retry`.
- The no-output state keeps the header and output list empty, with `Refresh` available.
- The lost-connection state keeps the last rendered snapshot visible but disables editing and apply.
- Backend technical error strings are never shown verbatim; only the mapped copy above is displayed.

## Hotplug behavior

- The 3-second poll updates the output list, inspector, and canvas without closing the window.
- When outputs change while the draft is dirty, a banner shows `Display configuration changed. Pending edits were discarded.` and the draft is replaced by the new snapshot.
- In `Ready`, changes are silent apart from the updated list and canvas.
- If the selected output disappears, selection moves to the first remaining output; if none remain, the no-output state is shown.
- Disabled outputs remain listed through hotplug changes when the backend still reports them.

## Keyboard and accessibility

- Tab order: output list → inspector controls → footer actions.
- Enter activates the focused button.
- Space toggles the focused switch.
- Escape acts only in the confirmation popup, where it means `Revert now`.
- All controls have text labels; no icon-only actions.
- Use Fyne theme font sizes and colors; no fixed bitmap assets. High-DPI is handled by Fyne's scaling.
- Do not claim screen-reader support beyond what standard Fyne widgets provide.
- Focused controls use the theme focus color; focus rings are never removed.

## Theme and visual details

- Default Fyne theme.
- Primary color for selection; error color for invalid states; muted foreground for disabled rows.
- Selected tile border 2 px; unselected border 1 px; tile padding 8 px.
- No custom color literals outside theme constants.
- No animations beyond Fyne defaults.
- The window remembers no size or position across launches in v1; it opens at `1120×760`.

## Copy and punctuation

- Sentence case for labels and messages, except product names and proper nouns.
- `Hz` with a space before it, for example `60 Hz`.
- `×` for dimensions, for example `2560×1440`.
- `%` with no space, for example `150%`.
- `s` for seconds, for example `Reverting in 15 s`.
- Straight quotes in code and configuration examples; curly quotes are not required.
- All strings are English.

## See also

- [Product requirements](product.md)
- [Architecture](architecture.md)
- [Documentation index](README.md)
