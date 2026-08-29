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
High-frequency (5 Hz) Server-Sent Events stream delivering real-time telemetry snapshots.

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
  "cadence": 88,
  "avg_cadence": 82,
  "max_cadence": 112,
  "speed_mph": 19.4,
  "avg_speed_mph": 17.8,
  "distance_mi": 6.42,
  "power_w": 210,
  "avg_power_w": 195,
  "max_power_w": 380,
  "np_w": 204,
  "tss": 42.1,
  "if": 0.82,
  "hr_zone": 3,
  "power_zone": 3,
  "power_source": "FTMS Trainer",
  "calories": 245,
  "work_kj": 215.4,
  "elapsed_sec": 1240,
  "is_running": true,
  "resistance_knob": 6,
  "user_weight_kg": 75.0,
  "ftp": 250,
  "sensors": {
    "hr": true,
    "cadence": true,
    "speed": true,
    "power": false,
    "ftms": true
  },
  "status": "Connected"
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

### `POST /api/workout/reset` 🔒
Stops the current ride, automatically persists the session to SQLite history (if elapsed time > 10s and movement occurred), and resets stopwatch, distance, calories, and interval state.

**Response:**
```json
{
  "ok": true,
  "message": "Workout reset and saved to history"
}
```

### `POST /api/settings` 🔒
Updates real-time athlete profile and calibration parameters.

**Request Body:**
```json
{
  "weight_kg": 75.0,
  "ftp": 250,
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
    "ftp": 250,
    "wheel_circ_mm": 1436,
    "max_hr": 185,
    "rest_hr": 55
  }
}
```

### `POST /api/knob` 🔒
Adjusts virtual resistance level (0–10 scale) for flywheel virtual power estimation.

**Request Body:**
```json
{
  "knob": 6
}
```

---

## 3. Workout Management & Imports

### `GET /api/workouts`
Returns a list of all available workout profiles, including built-in video-matched programs and custom user-imported workouts.

**Response:**
```json
[
  {
    "id": "sweet-spot-30m",
    "name": "30m Sweet Spot Endurance",
    "description": "20 timed phases with 80-90 RPM base holds and cadence surges",
    "video_id": "V-HmspieYjE",
    "duration_sec": 1800,
    "intervals_count": 20,
    "is_builtin": true
  }
]
```

### `GET /api/workouts/{id}`
Returns full interval structure and targets for a specific workout ID.

### `POST /api/workouts/import` 🔒
Imports a structured workout file (`.zwo` Zwift XML, `.mrc`, `.erg`, or Spin-HUD `.json`).

**Request Format (Multipart Form or Direct JSON):**
- **Multipart Form**: Form field `file` containing the workout file.
- **Direct JSON**:
```json
{
  "name": "Custom 40m Threshold Intervals",
  "description": "4x 8-minute threshold intervals with 2-minute recoveries",
  "intervals": [
    {
      "start_sec": 0,
      "end_sec": 300,
      "target_rpm": 85,
      "target_rpe": 3,
      "target_power_pct": 55,
      "label": "Warmup"
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
    "id": "ride-20260829-143000",
    "started_at": "2026-08-29T14:30:00Z",
    "ended_at": "2026-08-29T15:05:00Z",
    "duration_sec": 2100,
    "distance_mi": 11.45,
    "avg_hr": 142,
    "max_hr": 172,
    "avg_cadence": 84,
    "avg_power_w": 205,
    "np_w": 218,
    "tss": 52.3,
    "calories": 480,
    "work_kj": 430.5
  }
]
```

### `GET /api/history/{id}/fit` or `GET /api/workout/export.fit`
Downloads the workout activity as a standardized binary Garmin `.FIT` file containing file ID, session summary, lap data, and 1 Hz per-second telemetry records (Heart Rate, Cadence, Speed, Power, Distance).

### `GET /api/history/{id}/tcx` or `GET /api/workout/export.tcx`
Downloads the workout activity as a standard XML Training Center XML (`.TCX`) file.

### `POST /api/history/{id}/strava` 🔒
Directly uploads the completed ride to Strava using authenticated Strava OAuth tokens.

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
*Sets `spin_auth` session cookie upon successful PIN verification.*

---

## 6. Strava Integration

### `GET /api/strava/status`
Returns current Strava authorization status and athlete username.

### `GET /api/strava/login`
Redirects browser to Strava OAuth 2.0 authorization URL (`read,activity:write`).

### `GET /api/strava/callback`
OAuth redirect handler that exchanges auth code for refresh & access tokens.

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
