# 📡 Spin Studio HTTP & Telemetry API Reference

Spin Studio includes an embedded, high-performance pure Go HTTP and Server-Sent Events (SSE) server. By default, the server binds to `127.0.0.1:8080` (or `0.0.0.0:8080` when launched with `--lan`).

---

## 📑 Table of Contents
- [Real-Time Telemetry (SSE)](#1-real-time-telemetry-sse)
- [Ride Control & Session Management](#2-ride-control--session-management)
- [Workout Management & Imports](#3-workout-management--imports)
- [Ride History & Activity Exports](#4-ride-history--activity-exports)
- [Authentication & LAN Security](#5-authentication--lan-security)
- [Strava Integration](#6-strava-integration)
- [Sensor Status & System Information](#7-sensor-status--system-information)

---

## 1. Real-Time Telemetry (SSE)

### `GET /api/telemetry`
High-frequency (5 Hz) Server-Sent Events stream delivering real-time telemetry snapshots directly to web and HUD clients.

**Headers:**
```http
Accept: text/event-stream
```

**SSE Event Payload:**
```json
{
  "hr": 145,
  "avg_hr": 138,
  "max_hr": 168,
  "hr_zone": 3,
  "hr_zone_name": "Aerobic",
  "hr_zone_pct": 78,
  "hr_zone_color": "#22c55e",
  "cadence": 88,
  "avg_cadence": 82,
  "max_cadence": 112,
  "speed_mph": 19.4,
  "avg_speed_mph": 17.8,
  "max_speed_mph": 24.2,
  "speed_kmh": 31.2,
  "avg_speed_kmh": 28.6,
  "max_speed_kmh": 38.9,
  "watts": 210,
  "avg_watts": 195,
  "max_watts": 380,
  "power_source": "FTMS Trainer",
  "w_kg": 2.80,
  "avg_w_kg": 2.60,
  "distance_mi": 6.42,
  "distance_km": 10.33,
  "calories": 245,
  "elapsed_sec": 1240,
  "is_running": true,
  "sensors": {
    "hr": true,
    "cadence": true,
    "speed": true,
    "power": false,
    "ftms": true
  },
  "status": "Connected",
  "playlist_id": "PL...",
  "workout_name": "30m Sweet Spot Endurance",
  "rider_weight_kg": 75.0,
  "knob": "med",
  "knob_label": "Medium (Level 5)",
  "knob_turns": 2.5
}
```

---

## 2. Ride Control & Session Management

> 🔒 *Endpoints marked with 🔒 require valid session authentication when running with `--lan`.*

### `POST /api/workout/toggle` 🔒
Starts or pauses the active workout recording and timer.

**Response:**
```json
{
  "ok": true,
  "is_running": true
}
```

### `POST /api/workout/finish` 🔒
Pauses the active timer, finalizes session telemetry, and persists the completed workout to the SQLite database (`spin_hud.db`).

**Response:**
```json
{
  "ok": true,
  "saved": true,
  "ride_id": 42,
  "elapsed_sec": 1800,
  "calories": 425,
  "distance_mi": 12.4
}
```

### `POST /api/workout/reset` 🔒
Stops the current ride, automatically persists the session to SQLite history (if elapsed time >= 5s), and resets stopwatch, distance, calories, and interval state.

**Response:**
```json
{
  "ok": true,
  "saved": true,
  "ride_id": 42
}
```

### `POST /api/settings` 🔒
Updates real-time athlete profile and calibration parameters.

**Request Body:**
```json
{
  "weight_kg": 75.0,
  "wheel_circ_mm": 1436,
  "max_hr": 185,
  "rest_hr": 55
}
```

**Response:**
```json
{
  "ok": true,
  "settings": {
    "weight_kg": 75.0,
    "wheel_circ_mm": 1436,
    "max_hr": 185,
    "rest_hr": 55
  }
}
```

### `POST /api/knob` 🔒
Adjusts virtual resistance level (low / med / hard / numeric turns) for flywheel virtual power estimation.

**Request Body:**
```json
{
  "turns": 2.5,
  "label": "Medium"
}
```

---

## 3. Workout Management & Imports

### `GET /api/workouts`
Returns a list of all available workout profiles, including built-in video-matched programs and custom user-imported workouts from SQLite storage.

**Response:**
```json
[
  {
    "id": "sweet-spot-30m",
    "name": "30m Sweet Spot Endurance",
    "description": "20 timed phases with 80-90 RPM base holds and cadence surges",
    "total_duration_sec": 1800,
    "video_id": "V-HmspieYjE",
    "video_offset_sec": 0.0,
    "steps": [
      {
        "name": "Warmup",
        "type": "warmup",
        "duration_sec": 300,
        "power_low_pct": 0.50,
        "power_high_pct": 0.65,
        "cadence_target": 85,
        "knob": "low",
        "color": "#38bdf8",
        "cue_message": "Spin easy and build cadence"
      }
    ]
  }
]
```

### `GET /api/workouts/{id}`
Returns the full workout definition and interval steps for a specific workout ID.

### `POST /api/workouts/import` 🔒
Imports a structured workout file (`.zwo` Zwift XML, `.mrc`, `.erg`, or Spin-HUD JSON format).

**Request Format (Multipart Form or Direct JSON):**
- **Multipart Form**: Form field `file` containing the `.zwo`, `.mrc`, `.erg`, or `.json` file.
- **Direct JSON Schema**:
```json
{
  "name": "Custom 40m Threshold Intervals",
  "description": "4x 8-minute threshold intervals with 2-minute recoveries",
  "author": "Coach Honey",
  "category": "Threshold",
  "video_id": "pH4ISSkJ4bw",
  "steps": [
    {
      "name": "Warmup",
      "type": "warmup",
      "duration_sec": 300,
      "power_low_pct": 0.55,
      "cadence_target": 85,
      "knob": "low",
      "cue_message": "Gradual cadence ramp"
    },
    {
      "name": "Interval 1",
      "type": "interval",
      "duration_sec": 480,
      "power_low_pct": 0.95,
      "power_high_pct": 1.05,
      "cadence_low": 90,
      "cadence_high": 100,
      "knob": "hard",
      "cue_message": "Hold steady threshold power"
    }
  ]
}
```

---

## 4. Ride History & Activity Exports

### `GET /api/history`
Returns paginated list of completed ride sessions saved in SQLite database.

**Response:**
```json
[
  {
    "id": 1,
    "started_at": "2026-08-29T14:30:00Z",
    "ended_at": "2026-08-29T15:05:00Z",
    "duration_sec": 2100,
    "distance_m": 18420.5,
    "avg_hr": 142,
    "max_hr": 172,
    "avg_cadence": 84,
    "max_cadence": 110,
    "avg_watts": 205,
    "max_watts": 385,
    "calories": 480,
    "workout_name": "30m Sweet Spot Endurance"
  }
]
```

### `GET /api/history/{id}`
Returns detailed summary and 2-second trackpoint telemetry logs for saved ride `{id}`.

### `DELETE /api/history/{id}` 🔒
Deletes the specified ride session from the SQLite database.

### `GET /api/history/{id}/export.fit`
Downloads the saved historical ride activity as a standardized Garmin binary `.FIT` file.

### `GET /api/workout/export.fit`
Downloads the current or most recent in-memory session as a Garmin `.FIT` file.

### `GET /api/workout/export.tcx`
Downloads the current or most recent in-memory session as a standard XML `.TCX` file.

---

## 5. Authentication & LAN Security

### `POST /api/auth/pin`
Validates the 6-digit terminal PIN when connecting from a remote device over LAN.

**Request Body:**
```json
{
  "pin": "482910"
}
```

**Response:**
```json
{
  "ok": true,
  "message": "Authenticated"
}
```
*Sets the `spin_auth` session cookie upon successful PIN verification.*

---

## 6. Strava Integration

### `GET /api/strava/status`
Returns current Strava authorization status and athlete username.

### `GET /api/strava/login`
Redirects browser to Strava OAuth 2.0 authorization URL (`read,activity:write`).

### `GET /api/strava/callback`
OAuth redirect handler that exchanges authorization code for refresh & access tokens.

### `POST /api/strava/disconnect` 🔒
Revokes Strava credentials and clears local token storage.

---

## 7. Sensor Status & System Information

### `GET /api/sensors/status`
Returns real-time connection status of all Bluetooth LE devices, active power source, and LAN security status.

**Response:**
```json
{
  "ok": true,
  "sensors": {
    "hr": true,
    "cadence": true,
    "speed": true,
    "power": false,
    "ftms": true
  },
  "power_source": "FTMS Trainer",
  "status": "Connected",
  "lan_secured": true
}
```

### `GET /api/youtube/title?id={VIDEO_ID}`
Fetches and caches the YouTube video title for display in the workout video selector.
