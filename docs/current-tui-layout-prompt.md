# Prompt: Recreate the Current `esqlorer` TUI Layout

Create a terminal user interface for `esqlorer`, a Bubble Tea-based ES|QL exploration tool.

The goal is to reproduce the current layout and interaction model as faithfully as possible so it can be used as the baseline for a future redesign.

## Product Context

`esqlorer` is a keyboard-first TUI for querying Elasticsearch with ES|QL. The interface should feel compact, fast, and technical. It is not a dashboard and not a graphical app. It should look like a power-user terminal tool that helps inspect query results and navigate a single record in depth.

## Overall Layout

The screen is organized into three main states:

1. Query/results view
2. Detail preview view
3. Time-range picker modal

The default state is the query/results view. The top portion contains the header, query input, and status metadata. The lower portion contains the result table. When a row is opened, the UI switches to a dedicated preview page for that row. When the user opens the time range control, a centered modal appears over the app.

## Visual Style

Use a dark terminal aesthetic with restrained, purposeful color.

- Header background: dark slate or blue-gray
- Focus outline: green or bright cyan
- Section titles: bold with a contrasting background block
- Metadata chips: muted dark blocks with light foreground text
- Error text: red, bold
- Secondary text: dim gray

The style should feel closer to a focused terminal tool than a flashy app. Keep the visual language compact and utilitarian, but still polished.

## Query / Header Area

The top area contains:

- A full-width header bar with the application name and current server context
- A query label and editable query input
- A row of small metadata chips
- A status line
- A footer line with shortcuts

The query input is a single-line text field with a border. When focused, the border should become highlighted in green. When not focused, the border should remain muted.

The metadata chips currently show:

- Active time range
- Current export format
- Any applied filters

The status line shows:

- The latest error, if any
- Otherwise the current status message, such as `Executing query...` or `Loaded N rows`
- Otherwise the last query text or a short instruction

The footer should always show the relevant shortcuts for the current mode.

## Results View

The default lower section is a table of query results.

The table should:

- Fill the available width
- Use a visible header row
- Show row selection with a highlighted prefix or row style
- Use truncated cell content if a value is too long
- Stay compact and readable in a terminal window

The current layout does not split the results area into side-by-side preview and table panes. The results page is table-only. A separate row preview is opened as its own page.

## Detail Preview View

Selecting a row and pressing `Enter` opens a dedicated preview page.

This page should:

- Replace the table view
- Show the record as a key/value list
- Allow filtering within the record
- Allow copying key, value, or key=value
- Allow adding positive and negative filters from the selected key/value pair
- Allow hiding fields whose value is `null`

The preview page uses a title like `Preview` and then a focused filter input or filter hint area, followed by a list of fields.

The field list should:

- Highlight the selected field
- Keep the selected item visible while navigating
- Show key and value in a compact `key = value` format
- Use subdued styling for non-selected rows

If a field value is `null`, it can be hidden with a toggle on the preview page.

## Time Range Picker Modal

The time range control is a centered modal overlay. It is not part of the main page flow.

The modal should:

- Appear centered over the current screen
- Dim or visually separate itself from the background through framing and placement
- Show a small list of predefined ranges
- Support mouse clicks on individual rows
- Support arrow-key navigation
- Close with `Esc` without applying changes
- Apply the selected range with `Enter`

The current preset ranges are:

- `all time`
- `last 15 minutes`
- `last 1 hour`
- `last 2 hours`
- `last 24 hours`

## Query Behavior

When the user executes a query:

- Show a loading spinner in the header
- Keep the UI responsive while the query runs
- Replace the results table once the response arrives
- Preserve the current query text in the input
- Update the status line with the number of returned rows

The effective query is built from:

- The entered ES|QL query
- The selected time range filter
- Any positive or negative key/value filters added from the detail view

## Export Behavior

The UI supports exporting the current result set in multiple formats.

Supported formats:

- `table`
- `csv`
- `json`
- `yaml`

The export format can be cycled from the UI. The current format is shown as a metadata chip in the header.

## Controls

The layout is keyboard-driven.

Current shortcuts:

- `Enter` from the query input: run query
- `Tab`: switch between query input and results table
- `Enter` from a selected row: open preview page
- `Esc` or `Backspace` on preview page: return to table
- `Alt+T`: open time range picker
- `Alt+E`: cycle export format
- `Ctrl+E`: export current result set
- `n` on preview page: toggle hiding `null` values
- `Tab` or `f` on preview page: focus preview filter
- `c`: copy `key=value`
- `K`: copy key only
- `V`: copy value only
- `+`: add positive filter from selected field
- `-`: add negative filter from selected field

## Interaction Principles

- Prefer keyboard navigation first
- Keep the number of visible UI elements low
- Use clear visual focus
- Make state changes obvious through status text and chips
- Avoid cluttering the results page with extra panes

## What Not To Change

When recreating the current layout, do not:

- Turn the results page into a split view
- Add heavy dashboard-like widgets
- Remove the dedicated preview page
- Remove the centered time range modal
- Replace the keyboard-first control model with mouse-first interaction

## Output Expectations

Produce a Bubble Tea layout that matches the current `esqlorer` TUI closely, including:

- dark terminal styling
- compact header and metadata chips
- query input at the top
- full-width result table
- separate preview page for one selected row
- centered time range modal
- spinner during query execution

Use this as the baseline design prompt before building a new layout variant.
