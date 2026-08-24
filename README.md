# 🚴 Spin Studio HUD & Telemetry Player

> A sleek, glassmorphic, Peloton-style workout HUD for indoor spin bikes with live Bluetooth LE telemetry (Garmin Heart Rate, Magene Cadence & Speed) and embedded fullscreen YouTube playlist video player.

---

## ✨ Features

- **🚴 Glassmorphic Telemetry HUD**:
  - **Cadence (RPM)**: Real-time crank RPM with synchronized rotating crank animation, target cadence guide (80–100 RPM), and live **Average / Max** cadence stats.
  - **Speed & Distance (MPH / km/h)**: Live wheel speed, 1-click unit toggle, cumulative workout distance (miles/km), and live **Average / Max** speed stats.
  - **Heart Rate & Dynamic Zones (BPM)**: Real-time pulse from Garmin / BLE chest straps, animated pulse icon, dynamic Heart Rate Zone badge (Zone 1 Warm Up to Zone 5 Max Peak with color coding), and live **Average / Max** HR stats.
  - **Workout Timer & Energy (kcal)**: Session stopwatch (`MM:SS`), **Start / Pause** toggle (`⏸` / `▶`), workout reset, and active estimated calorie burn counter (kcal).
  - **Live Wall Clock**: High-contrast real-time clock displayed directly in the top toolbar.

- **📊 Post-Workout Summary Modal**:
  - Pop up a comprehensive ride achievement summary (`S` key or `📊` button) displaying duration, distance, total calories, and Average/Max breakdowns for Heart Rate, Cadence, and Speed.

- **🎬 Embedded Fullscreen YouTube Playlist Player**:
  - Clean video embed with auto-play, playlist auto-advance, now-playing track HUD, and dedicated playback controls.
  - Full keyboard shortcuts (`Space` play/pause, `N`/`P` next/previous track, `F` fullscreen, `H` hide/show telemetry overlay).

- **⚡ Robust Multi-Device BLE Manager**:
  - Concurrent independent connection loops for Heart Rate, Cadence, and Speed sensors.
  - **Auto-Reconnect**: Seamlessly recovers connection in 1–2 seconds when waking sensors from sleep mode without interrupting other active devices.
  - **Fast-Connect Cache**: Caches paired device MAC addresses to disk (`~/.spin_hud_devices.json`) for instant reconnection on startup.
  - **Smart Device Identification**: Automatic prioritized sensor mapping for Magene dual-mode sensors (`57177-1` crank cadence, `40452-1` hub speed).

- **📱 Responsive & Remote-Screen Ready**:
  - Pure Python + SSE (Server-Sent Events) web stack with zero heavy C++/Qt compilation dependencies.
  - Accessible on your primary laptop screen or opened on an iPad/tablet mounted to your bike handlebars (`http://<laptop-ip>:8080`).

---

## 🛠️ Hardware Setup Guide

### 1. Garmin Watch (Heart Rate)
- **Garmin Forerunner 965 / Fenix / Instinct**:
  - **Option A (Broadcast Heart Rate)**: Go to *Settings → Sensors & Accessories → Wrist Heart Rate → Broadcast Heart Rate → Start*.
  - **Option B (Virtual Run)**: Start a *Virtual Run* activity. (Virtual Run broadcasts Heart Rate over BLE while tracking your workout on the watch).
  - *Tip*: Pair your Magene cadence/speed sensors to your Garmin watch using **ANT+** so the watch records your full workout while leaving the single Bluetooth LE channel open for your laptop.

### 2. Magene S3+ Cadence Sensor (Crank)
- Remove and re-insert the CR2032 battery until the **RED LED** flashes (Cadence Mode).
- Mount on the inside of the non-drive crank arm using the rubber strap.

### 3. Magene S3+ Speed Sensor (Wheel / Flywheel Hub)
- Remove and re-insert the CR2032 battery until the **GREEN LED** flashes (Speed Mode).
- Mount on the flywheel hub or wheel center.
- *Note*: Ensure phone Bluetooth is disconnected or fitness apps on your phone are closed so the phone does not lock the Magene sensor's single Bluetooth slot.

---

## 📐 Wheel Circumference Calibration

The HUD calculates speed and distance by tracking full 360° angular revolutions of the wheel/flywheel.

| Setup / Bike Type | Diameter | Circumference (mm) |
|---|---|---|
| **Indoor Spin Bike (18" Flywheel)** *(Default)* | **18.0 in** | **`1436 mm`** |
| Indoor Spin Bike (20" Flywheel) | 20.0 in | `1596 mm` |
| Standard Road Bike (700x23c) | 26.3 in | `2096 mm` |
| Standard Road Bike (700x25c) | 26.5 in | `2105 mm` |
| Mountain Bike (29 x 2.25) | 29.0 in | `2282 mm` |

*You can customize the wheel circumference or YouTube playlist at any time via the in-app **⚙️ Settings** modal.*

---

## 🚀 Quickstart

### Prerequisites
- Python 3.10+
- `bleak` Bluetooth LE library:
```bash
pip install bleak
```

### Running the App
```bash
# Launch Spin Studio Live (opens your browser automatically)
python spin_hud.py

# Launch with custom YouTube playlist
python spin_hud.py --playlist "https://youtube.com/playlist?list=YOUR_PLAYLIST_ID"

# Scan nearby BLE fitness sensors
python spin_hud.py --scan

# Run parser validation test suite
python spin_hud.py --self-check
```

---

## ⌨️ Keyboard Shortcuts

| Shortcut | Action |
|---|---|
| `Space` | Play / Pause YouTube workout video |
| `N` | Next track in playlist |
| `P` | Previous track in playlist |
| `F` | Toggle Fullscreen mode |
| `H` | Toggle HUD Telemetry overlay (hide / show) |
| `S` | Toggle Post-Workout Summary dialog |

---

## 📡 API Endpoints

The built-in HTTP server provides lightweight REST & SSE endpoints:

- `GET /` — Fullscreen Spin Studio Studio web interface.
- `GET /api/telemetry` — Server-Sent Events (SSE) stream providing 5Hz live telemetry snapshots (`hr`, `avg_hr`, `max_hr`, `cadence`, `avg_cadence`, `max_cadence`, `speed_mph`, `avg_speed_mph`, `distance_mi`, `calories`, `elapsed_sec`, `is_running`, `sensors`).
- `POST /api/workout/toggle` — Toggle Start / Pause state of the workout timer.
- `POST /api/workout/reset` — Reset elapsed duration, distance, calories, and averages.
- `POST /api/settings` — Update YouTube playlist ID, wheel circumference (mm), or maximum HR.

---

## 🧪 Testing & Validation

```bash
# Run unit test suite
python test_build.py
```

---

## 📄 License
MIT License. Crafted for high-energy indoor spin workouts. 🚴💨
