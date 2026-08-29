# 🎛️ Hardware & Sensor Setup Guide

Spin Studio connects to your **Garmin watch** (heart rate), **Magene S3+** sensors (cadence & speed), and — when you have them — a **BLE power meter** or a **smart trainer (FTMS)**. Everything talks over **Bluetooth LE**; the only ANT+ link is Garmin's quirky dance to keep watches from stealing BLE slots.

Want to browse what's nearby? Launch once with `spin-hud.exe --scan` and it prints every BLE beacon it can see for ~8 seconds.

---

## 📡 Supported Devices at a Glance

| Role | Service | Devices | What it feeds |
|---|---|---|---|
| **Heart Rate** | BLE Heart Rate (`0x180D` / `0x2A37`) | Garmin Forerunner 965, Fenix, Instinct, Venu, any BLE chest strap emitting HR | `hr`, zones, calories |
| **Cadence** | BLE Cycling Speed & Cadence (`0x1816` / `0x2A5B`) | Magene S3+ in **RED** (crank) mode | `cadence` |
| **Speed** | BLE Cycling Speed & Cadence (`0x1816` / `0x2A5B`) | Magene S3+ in **GREEN** (wheel/hub) mode | `speed`, `distance` |
| **Power Meter** | BLE Cycling Power (`0x1818` / `0x2A63`) | Stages, Assioma, 4iiii, Quarq, Garmin Rally, PowerTap Vector | `power_w` (real watts) |
| **Smart Trainer / Smart Bike** | BLE FTMS Indoor Bike (`0x1826` / `0x2AD2`) | Wahoo KICKR, Tacx NEO / Flux / Suito, Saris H3, Zwift Ride | `power_w`, `cadence`, `speed`, optionally `hr` |

The HUD shows a live status line for each role (`Searching…` → `Connecting…` → device name), and every sensor auto-reconnects within a few seconds of waking from sleep.

---

## ❤️ Garmin Watch — Heart Rate

### Broadcast HR (recommended)
1. On the watch: **Settings → Sensors & Accessories → Wrist Heart Rate → Broadcast Heart Rate → Start**.
2. On a `Forerunner`/`Fenix`, a **Virtual Run** or **Indoor Bike** activity with broadcast enabled also works.

> **Why ANT+ matters here:** the Magene sensors allow only **one** Bluetooth connection at a time — the laptop — so pair them to your Garmin watch over **ANT+** (`Settings → Sensors & Accessories → Add Sensor`). The watch logs your full workout to Garmin while the laptop takes the BLE link.

> **Know the thief:** Garmin Connect and Strava on nearby phones will happily steal a watch's BLE broadcast slot. Force-close them (or put the phone in airplane mode) before a ride, or the HR feed can die mysteriously.

---

## 🚴 Magene S3+ — Cadence & Speed (Two Sensors, One Model)

The S3+ ships as one hardware unit that you flip between two roles by **popping the CR2032 battery out and back in**:

| LED on wake | Role | Mount | Software identity |
|---|---|---|---|
| 🔴 **Red** | **Cadence** | inside the non-drive crank arm | `57177-1` / matches "cad", "cadence", "crank" |
| 🟢 **Green** | **Speed** | flywheel hub / wheel center | `40452-1` / matches "spd", "speed", "wheel" |

- Spin the crank **before** scanning — a Magene that has been idle for ~1 minute falls asleep and won't advertise.
- Do **not** pair the sensors in Windows Bluetooth settings. Spin Studio discovers and connects directly via WinRT BLE.
- If cadence ever reports a "wheel" reading, the sensor is in the wrong mode — flip the battery to change the LED color.

### How the HUD tells the two apart
When both CSC sensors are broadcasting, Spin Studio first looks for the **exact model numbers** (`57177` = crank/cadence, `40452` = hub/speed), then falls back to keywords in the device name (cad/cadence/crank vs spd/speed/wheel), and finally assigns any remaining CSC sensors to whichever role is still missing.

---

## ⚡ Power Meters & Smart Trainers (Optional Upgrade)

- **Power meter (`0x1818`)**: connects the same way as HR/CSC. Live watts replace the estimated values instantly; the HUD shows a `[METER]` badge.
- **FTMS trainer (`0x1826`)**: Wahoo, Tacx, Saris, and smart-bike brands broadcast everything — power, cadence, speed, and often heart rate — in one stream. Spin Studio prefers the trainer's readings for whichever metrics it provides, and shows a `[FTMS]` badge.
- Metrics the FTMS frame doesn't carry (or carries at 0) simply stay blank; the HUD falls back gracefully.

> A power meter or trainer is **not required** — without one, Spin Studio estimates watts from wheel speed and your resistance knob (see [METRICS_AND_EXPORTS.md](METRICS_AND_EXPORTS.md)).

---

## 📐 Wheel Circumference Calibration

Speed and distance are computed from **full wheel revolutions** × circumference, so an accurate circumference matters — especially on a spin bike where the "wheel" is a heavy flywheel.

| Setup / Bike Type | Diameter | Circumference |
|---|---|---|
| **Indoor Spin Bike — 18" flywheel** *(default)* | 18.0 in | **1436 mm** |
| Indoor Spin Bike — 20" flywheel | 20.0 in | 1596 mm |
| Road bike — 700×23c | 26.3 in | 2096 mm |
| Road bike — 700×25c | 26.5 in | 2105 mm |
| Mountain bike — 29×2.25 | 29.0 in | 2282 mm |

Change it anytime in **⚙️ Settings → Wheel Circumference (mm)**. Valid range: `500–3500 mm`.

---

## 🧯 Troubleshooting

| Symptom | Why / Fix |
|---|---|
| HR won't connect | Is **Broadcast Heart Rate** actually running on the watch? Is Garmin Connect / Strava open on a phone that snapped up the BLE slot? Close them. |
| Sensor "not found" | Magene asleep — **spin the cranks / wheel** first, then relaunch or run `--scan`. |
| Cadence reports wheel speeds | Wrong S3+ mode — pop battery until **RED** flashes. |
| Speed reports crank cadence | Wrong S3+ mode — pop battery until **GREEN** flashes. |
| Values freeze at last reading | 3-second **stale-data watchdog** zeroes the metric when the sensor stops broadcasting while still connected. If it stays zero, the sensor went to sleep — spin to wake it; reconnect is automatic. |
| Power meter shows nothing | Sensor parked for 30+ minutes is in standby; also confirm the battery. |
| Reconnects to the wrong sensor | Pairing cache is per-role (`~/.spin_hud_devices.json`). Delete that file and relaunch to force fresh auto-detection for all roles. |
| "Bluetooth adapter error" | Windows BLE is off, or another app is monopolizing the adapter. Enable Bluetooth, then relaunch. |

---

## 🔁 How Reconnection Works (for the curious)

On startup the engine connects to the **last known device address for each role** (fast path, ~1s). It then runs a continuous loop that:
1. Watches each connection for drops and clears the metric if the data goes stale >3s,
2. Re-scans every few seconds for any role that isn't connected,
3. Lets the sensors sleep and wake freely mid-workout without killing the session.

Steering the session from a second screen? See [LAN_REMOTE_CONTROL.md](LAN_REMOTE_CONTROL.md) to pair your phone/tablet. Developer-facing API for sensor state lives in [API_REFERENCE.md](API_REFERENCE.md) (`GET /api/sensors/status`).