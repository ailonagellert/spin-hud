# 📊 Metrics & Exports Guide

Everything you see on the HUD — and everything you take away from it — is grounded in a simple, explainable model. Here's what each number means, how it's computed, and how to get it off the machine (`.fit`, `.tcx`, Strava, SQLite history).

---

## 💗 Heart Rate & Zones

Live HR comes from the Garmin watch / chest strap (see [HARDWARE_AND_SENSORS.md](HARDWARE_AND_SENSORS.md)). The HUD tags your current HR against your configured **Max HR** (⚙️ Settings, default 190 BPM, valid 80–240) to derive a **5-zone** ladder — the standard percentage-of-max model:

| % of Max HR | Zone | Badge | Color |
|:---:|---|---|---|
| < 60% | 1 | Warm Up | `#38bdf8` |
| 60–69% | 2 | Fat Burn | `#22c55e` |
| 70–79% | 3 | Aerobic | `#eab308` |
| 80–89% | 4 | Anaerobic | `#f97316` |
| ≥ 90% | 5 | Max Peak | `#ef4444` |

The HUD also tracks **Average** and **Max** HR for the session, and shows your current **% of Max HR**.

> Set Max HR to *your* real number, not the default — otherwise every zone line shifts.

---

## ⚡ Power: Real vs. Estimated

The HUD always shows a **power source badge** in three flavors:

### `[METER]` — BLE Power Meter (`0x1818`)
Real crank/crankarm watts stream in and replace everything below.

### `[FTMS]` — Smart Trainer (FTMS, `0x1826`)
The trainer reports power (plus cadence/speed, often HR). Trainer and meter readings always take priority over estimation.

### `[EST]` — Estimated (Virtual Power)
No meter yet? Spin Studio **estimates** watts from wheel speed + your resistance **knob**:

```
estimated watts ≈ (3.5·v + 0.35·v³) × knobFactor
  where v = speed in m/s
  knobFactor: LOW = 1.0 · MED = 2.0 · HARD = 3.5
```

The knob is a stand-in for the bike's resistance control (**LOW 1** turn → **MED 1/4** → **HARD 1/16** on the pad brake). Adjust it with the UI or `POST /api/knob`, so the curve roughly tracks how heavy the flywheel feels. It's a parlor-model for a bike with no instrumented cranks — treat the numbers as **relative training load, not lab data**.

**Power stats you'll see:** live watts, **W/kg** (watts ÷ your rider weight, ⚙️ Settings, default 75 kg), session **Average** and **Max**, and watts embedded in every export trackpoint.

---

## 📏 Speed, Distance & Energy

- **Speed / Distance**: computed from wheel (flywheel) **revolutions × circumference**. Set `wheel_circ_mm` in ⚙️ Settings — the default 1436 mm assumes an 18" flywheel (see the calibration table in [HARDWARE_AND_SENSORS.md](HARDWARE_AND_SENSORS.md#-wheel-circumference-calibration)). Unit toggle: MPH ↔ km/h.
- **Calories (kcal)**: a live estimate integrated from HR while the timer runs. The model only charges once HR exceeds 75 BPM: `(HR − 55) × 0.0022 kcal/s`. Expect it to look conservative vs. gym machines.
- **Timer**: running elapsed time is what Average/Max values and exports count against; pausing the timer freezes accumulation (it continues to sample for the live HUD).

While a workout runs, Spin Studio records a **trackpoint roughly every 1–2 seconds** (HR, cadence, speed, distance, watts). These points are what make exports detailed — and what lands in your ride history.

---

## 📈 What's NOT computed (yet)

Structured load metrics — **Normalized Power (NP), Training Stress Score (TSS), Intensity Factor (IF), FTP**, and power zones — are **not currently computed** by the code. `power_low_pct`/`power_high_pct` in imported workouts point at a percentage-of-FTP target that the HUD uses for cue display only. If those metrics matter to you, sync an export to Strava/TrainingPeaks and they'll be derived there.

---

## 💾 Export: `.fit` & `.tcx`

Both exports reflect the **live session** at the moment you download; both embed trackpoints when available.

| Format | Endpoint | What's in it |
|---|---|---|
| **`.fit`** (binary, Garmin FIT Protocol 2.0) | `GET /api/workout/export.fit` | FileId + Session (Sport=Cycling, SubSport=Indoor Cycling) + Lap + per-trackpoint records: timestamp, distance, speed, power, HR, cadence |
| **`.tcx`** (XML Garmin Training Center) | `GET /api/workout/export.tcx` | Activity `Sport="Biking"`, lap summary (time, distance, max speed, calories, Avg/Max HR, cadence), per-trackpoint HR/cadence + TPX speed/watts extensions |

Download from the **🏆 Workout Summary** modal (`S` key / 📊 button), or hit the endpoints directly. Filenames embed the workout start time: `spin_workout_20260829_143000.fit`.

> The summary modal also downloads/upload counts as the same ride — one workout, three ways out: **💾 .FIT**, **💾 .TCX**, and **Post to Strava**.

---

## 🗄️ Ride History (SQLite)

Every ride you finish gets recorded locally in `spin_hud.db` (next to the app, or `--db <path>`):

- **Auto-save**: hitting **Reset** (↺) auto-saves the session you just ended. Rides shorter than **15 seconds** are ignored as accidental restarts.
- **What's saved**: start/end time, duration, distance, calories, Avg/Max for HR, cadence, speed, and watts, the workout's name, and the per-second trackpoints.
- **Browse & re-export**: the **History** modal lists past rides (`GET /api/history`); each ride can re-download its `.fit` (`GET /api/history/{id}/export.fit`) or be deleted.
- History is purely local — GDPR-friendly and fully yours.

---

## 🚴 Strava Integration

OAuth 2.0 with `read, activity:write` scope, via the **Summary modal → Post to Strava**.

### First-time setup
1. Create a Strava API app at `https://www.strava.com/settings/api`. The **Callback/Authorization domain must include `localhost`** — e.g. `localhost` (port is not part of the domain check).
2. Drop the credentials into `strava-app.json` (see `strava-app.json.example`) next to the executable, or export `STRAVA_CLIENT_ID` / `STRAVA_CLIENT_SECRET`:
   ```json
   { "client_id": "12345", "client_secret": "abcdef0123456789abcdef0123456789abcdef01" }
   ```
3. In the ⚙️ Settings modal, hit **Connect Strava** — you'll be walked through Strava's authorization and redirected back.

### Uploading
- Authorized and riding? **Post to Strava** uploads the **current ride as a TCX** (≥ 30 seconds required).
- Access tokens are stored in `strava-tokens.json` and **auto-refresh** (~5 minutes before expiry).
- A duplicate guard tracks the last successful upload start time, so re-posting the same ride won't create doubles.
- **Disconnect** in ⚙️ Settings deletes your local tokens.

Strava uploads carry HR, cadence, speed, and (virtual or real) watts — so Strava can compute NP, TSS, and IF for you on their side.

---

Related: [HARDWARE_AND_SENSORS.md](HARDWARE_AND_SENSORS.md) · [WORKOUTS_AND_IMPORTING.md](WORKOUTS_AND_IMPORTING.md) · [LAN_REMOTE_CONTROL.md](LAN_REMOTE_CONTROL.md) · [API_REFERENCE.md](API_REFERENCE.md)