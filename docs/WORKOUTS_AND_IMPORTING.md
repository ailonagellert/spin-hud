# 🎬 Workouts & Importing Guide

Spin Studio ships with two flavors of training: **video-matched coaching rides** (a YouTube workout video drives the intervals) and **scripted interval programs**. It also imports **your** workouts from the industry-standard file formats — Zwift `.zwo`, Computrainer `.erg`/`.mrc`, and a simple JSON schema — so your favorite plans ride with you.

---

## 🗂️ What's Already Built In

### Video-Matched Coaching Profiles
These lock the interval countdown, target RPM, and effort cue card directly to the YouTube player clock (`player.getCurrentTime()`). Pause or scrub the video and the workout follows. Selecting one also auto-selects & queues that exact video; selecting the video in the playlist picks the matching program.

| Program | Video | Shape |
|---|---|---|
| 🚴 **30m Sweet Spot Endurance** | `V-HmspieYjE` | 20 phases, RPE 2–9, 80–90 RPM holds + 100–110 RPM surges, sprint finish |
| ⚡ **30m The Full Pyramid HIIT** | `eNyVyngn0l8` | 23 phases climbing 1→10/10 (115–130 RPM peak) and descending back |
| 🔥 **32m Sprint Surges HIIT** | `pH4ISSkJ4bw` | 25 phases with 8× 30s all-out 10/10 sprints (100–130 RPM) |
| 💨 **20m Cardio Spin** | `4Ek31_3PMW4` | 20 phases: standing climbs (70–80 RPM) ↔ seated surges (90–100 RPM) |

### Interval Programs (no video required)
Available from the **Workout Interval Program** dropdown in the cue bar or the ⚙️ modal:

- ⚡ **20-Min Generic HIIT Blast** — 5× 30s max-effort sprints (100–120 RPM) with 90s recoveries.
- ⛰️ **30-Min Climbs & Surges** — seated tempo climbs ↔ standing heavy climbs ↔ recovery spins.
- 🔥 **15-Min Tabata Fury** — classic 20s ON / 10s OFF × 8.
- 🚴 **Open Spin Session** — free ride.

With **Video-Matched (Auto-Sync)** selected, the program follows whatever video is playing in the playlist.

---

## 📥 Importing Your Own Workouts

### The easy way — drag, drop, ride
1. Click the **📥 Import Workout** button in the top toolbar,
2. Pick a `.zwo`, `.mrc`, `.erg`, or `.json` file,
3. Spin Studio parses it, **saves it into its library** (SQLite), and loads it immediately as **`📥 <name>`** in the program dropdown — all without a restart.

Behind the scenes it posts the file to `POST /api/workouts/import` (multipart field `file`), which returns the parsed workout and stores it. Imported workouts are listed alongside the built-ins by `GET /api/workouts`.

### Header data honored on import
`name`, `description`, `author`, and `category` are kept. A workout may also declare `video_id` (+ optional `video_offset_sec`) to become a **video-matched** profile that auto-cues its YouTube video.

---

## 📄 Supported File Formats

### 1. Zwift `.zwo` (XML)
Power values are **fractions of FTP** (e.g. `0.85` = 85%). Supported blocks:

```xml
<?xml version="1.0"?>
<workout_file>
  <author>Coach Honey</author>
  <name>Sweet Spot 40</name>
  <description>4x8min sweet spot</description>
  <sportType>bike</sportType>
  <workout>
    <Warmup Duration="300" PowerLow="0.45" PowerHigh="0.65" Cadence="85"/>
    <SteadyState Duration="480" Power="0.88" Cadence="90" CadenceLow="85" CadenceHigh="95"/>
    <IntervalsT Repeat="4" OnDuration="60" OffDuration="120" OnPower="1.05" OffPower="0.50"/>
    <Ramp Duration="120" PowerLow="0.70" PowerHigh="0.95" Cadence="85"/>
    <FreeRide Duration="60" Cadence="80"/>
    <Cooldown Duration="300" PowerLow="0.40" PowerHigh="0.55" Cadence="80"/>
  </workout>
</workout_file>
```

| Block | Step type | Notes |
|---|---|---|
| `Warmup` | warmup | power **range**, low knob |
| `SteadyState` | steady | red/hard when ≥105% FTP, blue/low when <75% |
| `IntervalsT` | interval + recovery | `Repeat`× (`OnDuration` at `OnPower`, then `OffDuration` at `OffPower` when >0) |
| `Ramp` | ramp | power ascending low→high |
| `FreeRide` | freeride | no power target |
| `Cooldown` | cooldown | power **range**, low knob |

### 2. Computrainer `.erg` / `.mrc` (text)
Both share the `[COURSE HEADER]` / `[COURSE DATA]` layout. The header line **`MINUTES WATTS`** switches the data column into **absolute watts** mode (`.erg`); without it, values are interpreted as **% FTP** (`.mrc`):

```
[COURSE HEADER]
DESCRIPTION=4x8min Threshold
FILE NAME=Threshold 4x8
MINUTES WATTS
[END COURSE HEADER]
[COURSE DATA]
0.00	100
5.00	100
13.00	240
21.00	100
29.00	240
[END COURSE DATA]
```

Consecutive time/value points are paired into blocks (e.g. 0:00–5:00 @100W, then 5:00–13:00 @240W). The two `[END …]` markers must be present exactly as shown.

### 3. Spin Studio `.json`
The native schema. `steps[]` is the important part:

```json
{
  "name": "Threshold Tuesday",
  "description": "4x8min threshold with 2min recoveries",
  "author": "Coach Honey",
  "category": "Threshold",
  "video_id": "V-HmspieYjE",
  "video_offset_sec": 0,
  "steps": [
    { "name": "Warm Up",      "type": "warmup",  "duration_sec": 300, "power_low_pct": 0.45, "power_high_pct": 0.65, "cadence_target": 85,  "knob": "low",  "color": "#38bdf8", "cue_message": "Easy spin" },
    { "name": "Threshold 1",  "type": "interval", "duration_sec": 480, "power_low_pct": 0.95, "power_high_pct": 1.05, "cadence_low": 85, "cadence_high": 95, "knob": "hard", "color": "#ef4444", "cue_message": "Smooth and strong" },
    { "name": "Recovery 1",   "type": "recovery", "duration_sec": 120, "power_low_pct": 0.50, "cadence_target": 80,  "knob": "med",  "color": "#22c55e", "cue_message": "Breathe" },
    { "name": "Cooldown",     "type": "cooldown", "duration_sec": 300, "power_low_pct": 0.40, "power_high_pct": 0.55, "cadence_target": 75,  "knob": "low",  "color": "#38bdf8", "cue_message": "Well earned" }
  ]
}
```

**Step fields (all optional except `duration_sec` + `name`):**

| Field | Meaning |
|---|---|
| `type` | `warmup`, `cooldown`, `steady`, `interval`, `recovery`, `freeride` |
| `duration_sec` | length of the block |
| `power_low_pct` / `power_high_pct` | target power as **fraction of FTP** (0.85 = 85%) |
| `target_watts` | absolute-watts alternative |
| `cadence_target`, `cadence_low`, `cadence_high` | RPM target / range for the cue card |
| `knob` | `low`, `med`, `hard` resistance hint |
| `color` | accent hex color drawn on the cue card |
| `cue_message` | coaching line ("Max effort sprint!") |

Anything missing (`total_duration_sec`) is calculated for you.

---

## 📊 What You See After Import

- The interval cue bar renders each block with its **color**, **countdown**, and **target** (shown as `X% FTP` when power is percentage-based, or `XW Target` for absolute watts).
- `cadence_target` is shown as the target; `cadence_low`/`cadence_high` draw a target **range**.
- If a step has an RPE-style message in `cue_message`, that's the line the coach cue card shows.

Imported workouts persist in the SQLite library, so they reappear next launch. Riding one? Exactly the same telemetry, summary, `.fit`/`.tcx` export, and Strava upload as any other workout — see [METRICS_AND_EXPORTS.md](METRICS_AND_EXPORTS.md).