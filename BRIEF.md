# Spin HUD — handoff to Gemini (Windows laptop)

From: Hermes (NAS). Erich/Ailona asked you to **build this locally on the Windows laptop**. Load **project-init** first and use it to stand the project up. Do not start from a blank repo if that skill already has a house style.

## Goal

Always-on-top desktop HUD on the **spin-bike Windows laptop** while YouTube plays:

- Heart rate from **Garmin Forerunner 965** (BLE broadcast)
- Cadence from **Magene S3+** already on the bike (`Amazon B08252X8VS`)

Not a trainer app. Not Zwift. Not a Garmin Connect client.

## Do this

1. Load **project-init**. Init a small local Windows project (Python is fine).
2. Start from `spin_hud.py` in this folder. Parsers already pass `--self-check`.
3. Install `bleak`, run `python spin_hud.py --scan` on the laptop with radio on.
4. Make the HUD work: always-on-top, HR + cadence + elapsed, Esc to quit.
5. Prove it: `--self-check` green, `--scan` lists Garmin + Magene, then a live ride screenshot or log of numbers changing.
6. Reply on Buzz with path + how to launch + what `--scan` saw.

## Pairing (will fail if ignored)

- Magene S3+ = dual BLE + ANT+. **BLE = 1 client.** ANT+ = many.
- 965 must use the Magene over **ANT+**. Laptop takes Magene **BLE**.
- If the watch already holds the puck on Bluetooth, forget it on the watch and re-pair ANT+.
- Crank mount. **Red flash = cadence**, green = speed (pop battery to flip).
- Sensor sleeps after ~1 min still — spin the crank before scan.
- Do **not** pair either device in Windows Bluetooth settings. Bleak connects in-app.
- 965: start Indoor Bike + **Broadcast Heart Rate**, or **Virtual Run**. Virtual Run is the more reliable BLE HR path. Kill Garmin Connect on a nearby phone so it does not steal the one BLE HR slot.
- Magene hardware only reports cadence about 30–160 rpm; below that it shows 0.

## Do not

- Garmin Connect / unofficial `garminconnect` / LiveTrack / Health SDK / Activity API. Those are after-sync or enterprise/mobile. Useless for a live HUD.
- Connect IQ watch app.
- Extra deps beyond `bleak` (+ stdlib tkinter).
- Zones, charts, accounts, installers, Electron.

## Evidence (already researched)

Official / vendor:

- FR965 Broadcast HR: https://www8.garmin.com/manuals/webhelp/GUID-0221611A-992D-495E-8DED-1DD448F7A066/EN-US/GUID-D8D363C2-0690-48D4-95E2-A3557E7D53C2.html
- Broadcast during activity: https://www8.garmin.com/manuals/webhelp/GUID-0221611A-992D-495E-8DED-1DD448F7A066/EN-US/GUID-57A88A77-3813-4E79-9DB1-FC95B06F01BA.html
- Cycling cadence field requires a cadence accessory: https://www8.garmin.com/manuals/webhelp/GUID-0221611A-992D-495E-8DED-1DD448F7A066/EN-US/GUID-73BCE454-042E-420D-96A4-9DBA46626CD4.html
- Bike speed/cadence sensor: https://www8.garmin.com/manuals/webhelp/GUID-0221611A-992D-495E-8DED-1DD448F7A066/EN-US/GUID-132B03EE-2064-4066-AB6D-718C2A51FEDD.html
- Extended Display = Edge only, not a laptop: https://www8.garmin.com/manuals/webhelp/GUID-0221611A-992D-495E-8DED-1DD448F7A066/EN-US/GUID-1E3CECCF-0343-431C-95F0-5716E0341C75.html
- Activity API after Connect sync: https://developer.garmin.com/gc-developer-program/activity-api/
- Health API after upload: https://developer.garmin.com/gc-developer-program/health-api/
- Health SDK = enterprise Android/iOS only: https://developer.garmin.com/health-sdk/
- Virtual Run BLE HR/pace/cadence to Zwift etc: https://support.garmin.com/en-US/?faq=pyniXQfLiu3BS1yKFlLn36
- LiveTrack needs phone+cell; not a HUD API: https://support.garmin.com/en-US/?faq=HbqxxbiBGA3mDhlLX4GUw8
- FR965 CIQ API 5.2: https://developer.garmin.com/connect-iq/compatible-devices/
- Magene S3+ BLE+ANT+, 1 BLE client, sleep 1 min: Amazon B08252X8VS + https://support.magene.com/hc/en-us/categories/900000170603-S3-Speed-Cadence-Sensor
- Mode switch (red=cadence, green=speed): https://support.magene.com/hc/en-us/articles/900002205946-How-to-switch-the-operating-mode-of-Magene-S3

Field reports:

- DC Rainmaker BLE Virtual Run vs ANT+ Broadcast HR: https://www.dcrainmaker.com/2020/04/garmin-wearable-broadcasting.html
- Same, Virtual Run details: https://www.dcrainmaker.com/2020/01/garmin-adds-bluetooth-heart-rate-running-data-broadcasting-for-fr245-fr945.html
- FR965 + Zwift: Virtual Run BLE works; Broadcast HR often not found: https://forums.zwift.com/t/no-signal-in-heart-rate-with-forerunner-965/639185
- Watch BLE HR is typically one channel; Garmin app can steal it: https://forums.trainerday.com/t/heart-rate-no-longer-works-in-app-using-garmin-broadcast-resolved/1698

BLE profiles: HR `0x180D` / `0x2A37`. CSC `0x1816` / `0x2A5B`. Cadence = crank revs + event time (1/1024 s). Virtual Run cadence is **running** steps/min — ignore it on a bike.

Hermes already wrote `/mnt/web/spin-hud/spin_hud.py`. `--self-check` passed on the NAS (no radio there, no tkinter). You have the radio.

## Paths

| Where | Path |
|---|---|
| This share (NAS web) | `/mnt/web/spin-hud/` |
| DSM | `/volume2/web/spin-hud/` |
| Windows if Z: is the web share | `Z:\spin-hud\` |
| UNC | `\\192.168.128.1\web\spin-hud\` |

Hermes source copy: `/opt/data/projects/spin-hud/spin_hud.py`

## Reply format

`path · launch cmd · scan result · live HR? · live CAD? · blockers`
