# 🚴 Spin Studio HUD & Telemetry Player

> A sleek, glassmorphic, Peloton-style workout HUD for indoor spin bikes with live Bluetooth LE telemetry (Garmin Heart Rate, Magene Cadence & Speed), virtual power meter, interval training programs, Strava/Garmin .TCX export, and embedded fullscreen YouTube playlist video player.

---

## ⚡ Standalone & Portable (`spin-hud.exe`)

`spin-hud` is compiled as a single, self-contained Windows executable (`spin-hud.exe`) with **zero external dependencies** (no Python, Node, or Git required):
- **Embedded Web UI**: HTML5/CSS/JS frontend baked directly inside the binary.
- **Native Windows BLE**: Direct WinRT Bluetooth LE integration for multi-sensor scanning and auto-reconnect.
- **1-Click Launch**: Double-click `spin-hud.exe` and it starts the engine and opens your default browser at `http://localhost:8080`.

---

## ✨ Features

- **🚴 Movable Glassmorphic Telemetry HUD**:
  - **3 Layout Modes**:
    - **⬇️ Bottom Dock** *(Default)*: Traditional horizontal dock spanning the bottom of the screen.
    - **⬅️ Left Stack**: 380px vertical sidebar stacked flush on the left below the workout cue card.
    - **➡️ Right Stack**: 380px vertical sidebar stacked flush on the right below the YouTube controls.
  - **Quick Layout Switching**: Press **`L`** on your keyboard, click the **⬇️ / ⬅️ / ➡️** icon in the top toolbar, or change it in **⚙️ Settings**.
  - **Cadence (RPM)**: Real-time crank RPM with synchronized rotating crank animation, target cadence guide (80–100 RPM), and live **Average / Max** cadence stats.
  - **Speed & Distance (MPH / km/h)**: Live wheel speed, 1-click unit toggle, cumulative workout distance (miles/km), and live **Average / Max** speed stats.
  - **Heart Rate & Dynamic Zones (BPM)**: Real-time pulse from Garmin / BLE chest straps, animated pulse icon, dynamic Heart Rate Zone badge (Zone 1 Warm Up to Zone 5 Max Peak with color coding), and live **Average / Max** HR stats.
  - **Virtual Power Meter (Watts & W/kg)**: Real-time calculated virtual cycling power based on speed, cadence, flywheel inertia, and rider weight.
  - **Workout Timer & Energy (kcal)**: Session stopwatch (`MM:SS`), **Start / Pause** toggle (`⏸` / `▶`), workout reset, and active estimated calorie burn counter (kcal).
  - **Live Wall Clock**: High-contrast real-time clock displayed directly in the top toolbar.

- **⏱️ Workout Interval Programs & Cue Card**:
  - Top-left HUD cue card with built-in structured workout interval selector:
    - *Free Ride (Open Telemetry)*
    - *20-Min HIIT Fat Burn*
    - *30-Min Cardio Climb & Power*
    - *45-Min Endurance & Cadence*
    - *15-Min Sprint Ladders*
    - *10-Min Spin Warm-Up / Cool-Down*
  - Synchronized workout timer with YouTube video playback.

- **📊 Post-Workout Summary & .TCX Export**:
  - Comprehensive ride achievement modal (**`S`** key or **`📊`** button) displaying duration, distance, total calories, and Average/Max breakdowns for Heart Rate, Cadence, Speed, and Power.
  - **Export .TCX**: 1-click export of your ride to standard `.TCX` activity files compatible with **Strava**, **Garmin Connect**, and **TrainingPeaks**.

- **🎬 Embedded Fullscreen YouTube Playlist Player**:
  - Fullscreen video embed with auto-play, playlist auto-advance, now-playing track HUD, and dedicated playback controls.
  - **Interactive Playlist Drawer (📑 List)**: Click to expand the full playlist drawer with async-resolved friendly song/video titles.
  - Full keyboard shortcuts (`Space` play/pause, `N`/`P` track skip, `F` fullscreen, `H` hide HUD, `L` change layout, `S` summary).

- **⚡ Robust Multi-Device BLE Manager**:
  - Concurrent independent connection loops for Heart Rate, Cadence, and Speed sensors.
  - **Auto-Reconnect**: Seamlessly recovers connection in 1–2 seconds when waking sensors from sleep mode without interrupting other active devices.
  - **Fast-Connect Cache**: Caches paired device MAC addresses to disk (`~/.spin_hud_devices.json`) for instant reconnection on startup.
  - **Smart Device Identification**: Automatic prioritized sensor mapping for Magene dual-mode sensors (`57177-1` crank cadence, `40452-1` hub speed).

---

## 🛠️ Hardware Setup Guide

### 1. Garmin Watch (Heart Rate)
- **Garmin Forerunner 965 / Fenix / Instinct / Venu**:
  - **Option A (Broadcast Heart Rate)**: Go to *Settings → Sensors & Accessories → Wrist Heart Rate → Broadcast Heart Rate → Start*.
  - **Option B (Virtual Run / Indoor Bike)**: Start a *Virtual Run* or *Indoor Bike* activity with HR broadcast enabled.
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

### 1. Run Prebuilt Binary (Windows)
Double click `spin-hud.exe` from `\\gnas\web\spin-hud\spin-hud.exe` (or your local checkout):
```cmd
spin-hud.exe
```

### 2. Command-Line Options
```bash
# Launch with custom YouTube playlist
spin-hud.exe --playlist "https://youtube.com/playlist?list=YOUR_PLAYLIST_ID"

# Enable LAN access (open on iPad/tablet at http://<laptop-ip>:8080)
spin-hud.exe --lan

# Run on custom port without auto-opening browser
spin-hud.exe --port 9000 --no-browser

# Scan nearby BLE fitness sensors
spin-hud.exe --scan

# Run parser & session engine self-check
spin-hud.exe --self-check
```

### 3. Python Prototype (Alternative)
If running from the Python prototype script:
```bash
pip install bleak
python spin_hud.py
```

---

## ⌨️ Keyboard Shortcuts

| Shortcut | Action |
|---|---|
| **`L`** | Cycle HUD Card Layouts (**⬇️ Bottom** → **⬅️ Left Stack** → **➡️ Right Stack**) |
| **`Space`** | Play / Pause YouTube workout video & sync timer |
| **`N`** | Next track in playlist |
| **`P`** | Previous track in playlist |
| **`F`** | Toggle Fullscreen mode |
| **`H`** | Toggle HUD Telemetry overlay (hide / show) |
| **`S`** | Toggle Post-Workout Summary dialog & .TCX export |

---

## 📡 API Endpoints

The built-in HTTP server provides lightweight REST & SSE endpoints:

- `GET /` — Fullscreen Spin Studio web interface.
- `GET /api/telemetry` — Server-Sent Events (SSE) stream providing 5Hz live telemetry snapshots (`hr`, `avg_hr`, `max_hr`, `cadence`, `avg_cadence`, `max_cadence`, `speed_mph`, `avg_speed_mph`, `distance_mi`, `power_w`, `w_kg`, `calories`, `elapsed_sec`, `is_running`, `sensors`).
- `GET /api/youtube/title?id=<VIDEO_ID>` — Async cached video title resolution.
- `POST /api/workout/toggle` — Toggle Start / Pause state of the workout timer.
- `POST /api/workout/reset` — Reset elapsed duration, distance, calories, and averages.
- `POST /api/settings` — Update YouTube playlist ID, wheel circumference (mm), maximum HR, or rider weight.

---

## 🧪 Testing & Validation

```bash
# Run Go unit test suite
go test ./...

# Run parser validation self-check
spin-hud.exe --self-check
```

---

## 📄 License
MIT License. Crafted for high-energy indoor spin workouts. 🚴💨
