# stopwatch-tui

A terminal stopwatch with split tracking, persistent state, and calendar export. Built with [Bubble Tea](https://charm.land/bubbletea/v2).


https://github.com/user-attachments/assets/db62626a-e81c-4e27-adac-b18d928aebc6


## Features

- Millisecond-precision stopwatch
- Split tracking with lap times and timestamps
- *Store State with SQLITE DB* — the timer keeps running even after the app is closed. Reopening the app resumes from where it left off
- Editable split names with inline text inputs
- Delete splits (merges elapsed time into the previous split)
- Fullscreen / inline display toggle
- Export splits to `.ics` (iCalendar) or as `gws` CLI commands for Google Calendar

## Installation

```bash
git clone https://github.com/jwc20/stopwatch-tui
cd stopwatch-tui
go build -o stopwatch-tui .
./stopwatch-tui
```

## Keybindings

### Normal mode

| Key | Action |
|-----|--------|
| `s` | Start / stop the timer |
| `p` | Record a split |
| `r` | Reset (only available when stopped) |
| `↓` | Enter edit mode — focus the first split |
| `e` | Export splits to `.ics` |
| `g` | Export splits as `gws` calendar commands |
| `f` | Toggle fullscreen |
| `q` / `ctrl+c` | Quit |

### Edit mode (when a split is focused)

| Key | Action |
|-----|--------|
| `↑` / `↓` | Move between splits |
| type | Edit the split name |
| `ctrl+d` | Delete the focused split |
| `esc` | Exit edit mode |

## Split list

Each split row shows:

```
>  1.  00:12.345  (+00:12.345)  2026-03-05 10:32:01  [My event name_]
   2.  00:25.678  (+00:13.333)  2026-03-05 10:32:14  [Split 2      ]
```

- Cumulative elapsed time
- Lap time since the previous split (in parentheses)
- Wall-clock timestamp when the split was recorded
- Editable name field (used as the event title on export)

When a split is deleted, its elapsed time is merged into the previous split so the total elapsed time is preserved.

## Exports

### `.ics` — press `e`

Exports splits as a standard iCalendar file (`splits_<timestamp>.ics`) using [golang-ical](https://github.com/arran4/golang-ical). Each split becomes a calendar event spanning from the previous split's timestamp to the current one. The event summary is the split name (or `Split N` if unnamed).

### `gws` commands — press `g`

**You must enable Google Calendar API and set up credentials on Google Cloud to use this feature.**

![Enable Google Calendar API](https://github.com/user-attachments/assets/f54fadaf-dc5d-45ef-a2e9-80db48fcdc38)

![Create OAuth Client](https://github.com/user-attachments/assets/0f905266-7479-461d-8f71-b125613cb588)



Exports splits as a shell script (`splits_<timestamp>.sh`) containing one `gws calendar +insert` command per split, ready to run against Google Calendar via the [gws](https://github.com/googleworkspace/cli/) CLI.

```sh
gws calendar +insert --summary "Morning run" \
  --start "2026-03-05T10:32:00-08:00" \
  --end "2026-03-05T10:44:12-08:00" \
  --description "Split 1 of 3 | Elapsed: 00:12.345 | Lap: 00:12.345"
```

Review the file before running: `cat splits_<timestamp>.sh`, then execute with `./splits_<timestamp>.sh`.

## Dependencies

- [charm.land/bubbletea](https://charm.land/bubbletea/v2)
- [charm.land/bubbles](https://charm.land/bubbles/v2)
- [charmbracelet/lipgloss](https://github.com/charmbracelet/lipgloss)
- [arran4/golang-ical](https://github.com/arran4/golang-ical)

## Other Tools

- [googleworkspace/cli](https://github.com/googleworkspace/cli)

## Project structure

```
.
├── main.go          # TUI model, keybindings, view
├── db.go            # Store state into db
├── ics.go           # iCalendar export
├── gws.go           # gws CLI command export
└── stopwatch/
    └── stopwatch.go # Stopwatch model (tick, split, delete, reset)
```
