# 📱 LAN Remote Control & Multi-Screen Setup

Spin Studio is designed to run seamlessly in multi-screen and remote control configurations. You can run the Bluetooth LE telemetry engine on a dedicated host computer (e.g. a Windows laptop or mini-PC next to your spin bike) and view/control the glassmorphic HUD on any device with a modern web browser:
- 📱 **iPhones & Android Phones** (mounted on bike handlebars)
- 📲 **iPads & Android Tablets** (mounted on bike tablet holder)
- 📺 **Smart TVs & Casting Screens** (Apple TV, Chromecast, Samsung/LG TV Browser)
- 💻 **Laptops & Secondary Monitors**

---

## 🚀 Quickstart: Enabling LAN Mode

By default, Spin Studio listens only on `127.0.0.1` (local loopback). To allow connections from other devices on your local Wi-Fi / Ethernet network:

### 1. Launch with `--lan`
```powershell
spin-hud.exe --lan
```
Or with custom port and PIN:
```powershell
spin-hud.exe --lan --port 8080 --pin 123456
```

### 2. Terminal Output
When `--lan` is enabled, the host terminal displays the local network URLs and the 6-digit pairing PIN:

```text
======================================================================
  🚴 Spin Studio HUD & Telemetry Server
======================================================================
  Local HUD URL   : http://localhost:8080
  LAN HUD URL     : http://192.168.1.150:8080
  LAN Pairing PIN : 482910
  Security        : PIN Required for Remote Controls
  Storage         : SQLite History & FIT / TCX Export Enabled
======================================================================
```

---

## 🔒 6-Digit PIN Pairing & Security Model

To protect your spin session from unauthorized interference across your home or gym network, Spin Studio employs a dual-tier security model:

| Action / Area | Unauthenticated LAN Device | Authenticated / Local Host |
|---|:---:|:---:|
| **View Live HUD & Telemetry** (5Hz SSE Stream) | ✅ Allowed | ✅ Allowed |
| **Watch Workout Video & Interval Cues** | ✅ Allowed | ✅ Allowed |
| **Browse Completed Ride History** | ✅ Allowed | ✅ Allowed |
| **Start / Pause Workout Timer** (`/api/workout/toggle`) | ❌ Requires PIN | ✅ Allowed |
| **Finish & Save Workout** (`/api/workout/finish`) | ❌ Requires PIN | ✅ Allowed |
| **Reset & Save Workout** (`/api/workout/reset`) | ❌ Requires PIN | ✅ Allowed |
| **Import Custom Workouts** (`/api/workouts/import`) | ❌ Requires PIN | ✅ Allowed |
| **Modify Profile & Calibration** (`/api/settings`) | ❌ Requires PIN | ✅ Allowed |
| **Adjust Resistance Level** (`/api/knob`) | ❌ Requires PIN | ✅ Allowed |

---

## 📲 How to Connect & Pair from a Mobile Device or Tablet

1. **Connect to Same Wi-Fi**: Ensure your phone/tablet is connected to the same Wi-Fi network as the Spin-HUD host computer.
2. **Open Browser**: Open Safari, Chrome, Edge, or Firefox and navigate to the LAN URL displayed in the terminal (e.g. `http://192.168.1.150:8080`).
3. **Authenticate for Control**:
   - **Manual Pairing**: Tap the **🔒 (Lock)** icon in the top header bar to open the **LAN Pairing** modal.
   - **Automatic Prompt**: If you attempt to start, pause, reset, or change settings before authenticating, the PIN modal will automatically appear.
4. **Enter PIN**: Type the 6-digit terminal PIN and tap **Connect / Unlock**.
5. **Full Control Unlocked**: Once authenticated, the 🔒 icon updates to unlocked and your mobile browser gains full remote control over workout recording, settings, and timers!

---

## 💡 Recommended Setup Configurations

### 1. Dedicated Bike Hub + Handlebar Phone Control
- **Host**: Laptop or Mini PC placed near the bike running `spin-hud.exe --lan` connected to Garmin HR and Magene sensors via Bluetooth LE.
- **Display**: Large TV / monitor in front of the bike streaming the YouTube workout video and overlay HUD.
- **Controller**: Smartphone mounted on handlebars opened to `http://<host-ip>:8080` for easy tap control (pause, interval scrub, resistance adjust).

### 2. Standalone Tablet Display
- **Host**: Windows laptop running `spin-hud.exe --lan`.
- **Display**: iPad or Android tablet mounted in the bike's tablet holder running the full Spin Studio web app in fullscreen mode (`Add to Home Screen` / PWA mode for zero-browser-chrome experience).
