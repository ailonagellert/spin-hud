#!/usr/bin/env python3
"""Always-on-top spin HUD: Garmin 965 HR + Magene S3+ cadence over BLE.

On the laptop (not this Hermes box):
  python -m pip install bleak
  python spin_hud.py --scan          # see what's advertising
  python spin_hud.py                 # HUD
  python spin_hud.py --self-check    # parser tests, no radio

Watch: start Indoor Bike with Broadcast Heart Rate, or Virtual Run.
Magene: crank mount, red flash = cadence. Pair the 965 over ANT+ so
BLE stays free for this laptop. Spin the crank — it sleeps after 1 min.
Do not pair either device in Windows/macOS Bluetooth settings.
"""

from __future__ import annotations

import argparse
import asyncio
import struct
import sys
import threading
import time

HR_UUID = "00002a37-0000-1000-8000-00805f9b34fb"
CSC_UUID = "00002a5b-0000-1000-8000-00805f9b34fb"
HR_SVC = "0000180d-0000-1000-8000-00805f9b34fb"
CSC_SVC = "00001816-0000-1000-8000-00805f9b34fb"


def parse_hr(data: bytes) -> int | None:
    if not data:
        return None
    flags = data[0]
    if flags & 0x01:
        if len(data) < 3:
            return None
        return struct.unpack_from("<H", data, 1)[0]
    if len(data) < 2:
        return None
    return data[1]


def parse_csc(data: bytes, prev: tuple[int, int] | None) -> tuple[float | None, tuple[int, int] | None, str]:
    """Return (rpm or None, new prev, mode). mode is crank|wheel|empty."""
    if not data:
        return None, prev, "empty"
    flags = data[0]
    off = 1
    if flags & 0x01:
        if len(data) < off + 6:
            return None, prev, "wheel"
        off += 6  # skip wheel revs + event time
    if not (flags & 0x02):
        return None, prev, "wheel" if flags & 0x01 else "empty"
    if len(data) < off + 4:
        return None, prev, "crank"
    revs, ev = struct.unpack_from("<HH", data, off)
    now = (revs, ev)
    if prev is None:
        return None, now, "crank"
    d_revs = (revs - prev[0]) & 0xFFFF
    d_ticks = (ev - prev[1]) & 0xFFFF
    if d_ticks == 0:
        return None, now, "crank"
    rpm = d_revs * 1024 * 60 / d_ticks
    return rpm, now, "crank"


def _self_check() -> int:
    assert parse_hr(b"\x00\x8c") == 140
    assert parse_hr(b"\x01\x2c\x01") == 300
    assert parse_hr(b"") is None
    rpm, prev, mode = parse_csc(b"\x02" + struct.pack("<HH", 10, 0), None)
    assert rpm is None and mode == "crank" and prev == (10, 0)
    # 20 revs in 10s = 10240 ticks → 120 rpm
    rpm, prev, mode = parse_csc(b"\x02" + struct.pack("<HH", 30, 10240), prev)
    assert mode == "crank" and prev == (30, 10240)
    assert rpm is not None and abs(rpm - 120.0) < 0.01
    rpm, _, mode = parse_csc(b"\x01" + struct.pack("<IH", 100, 0), None)
    assert rpm is None and mode == "wheel"
    print("self-check ok")
    return 0


class Hud:
    def __init__(self) -> None:
        import tkinter as tk

        self.hr: int | None = None
        self.rpm: float | None = None
        self.hr_ok = False
        self.cad_ok = False
        self.cad_mode = ""
        self.status = "scanning…"
        self.started = time.monotonic()
        self.lock = threading.Lock()
        self.root = tk.Tk()
        self.root.title("spin hud")
        self.root.configure(bg="#111")
        self.root.attributes("-topmost", True)
        self.root.geometry("280x170+40+40")
        self.root.resizable(False, False)
        font_big = ("Segoe UI", 36, "bold")
        font_lbl = ("Segoe UI", 10)
        font_st = ("Segoe UI", 9)
        row = tk.Frame(self.root, bg="#111")
        row.pack(fill="both", expand=True, padx=12, pady=8)
        left = tk.Frame(row, bg="#111")
        right = tk.Frame(row, bg="#111")
        left.pack(side="left", expand=True)
        right.pack(side="left", expand=True)
        tk.Label(left, text="HR", fg="#888", bg="#111", font=font_lbl).pack()
        self.hr_l = tk.Label(left, text="—", fg="#fff", bg="#111", font=font_big)
        self.hr_l.pack()
        tk.Label(right, text="CAD", fg="#888", bg="#111", font=font_lbl).pack()
        self.cad_l = tk.Label(right, text="—", fg="#fff", bg="#111", font=font_big)
        self.cad_l.pack()
        self.time_l = tk.Label(self.root, text="0:00", fg="#aaa", bg="#111", font=font_st)
        self.time_l.pack()
        self.st_l = tk.Label(self.root, text=self.status, fg="#666", bg="#111", font=font_st, wraplength=260)
        self.st_l.pack(pady=(0, 6))
        self.root.bind("<Escape>", lambda e: self.root.destroy())
        self.root.after(200, self._tick)

    def _tick(self) -> None:
        with self.lock:
            hr, rpm = self.hr, self.rpm
            status = self.status
            mode = self.cad_mode
        elapsed = int(time.monotonic() - self.started)
        self.hr_l.config(text="—" if hr is None else str(hr))
        if rpm is None:
            self.cad_l.config(text="spd?" if mode == "wheel" else "—")
        else:
            self.cad_l.config(text=str(int(round(rpm))))
        self.time_l.config(text=f"{elapsed // 60}:{elapsed % 60:02d}")
        self.st_l.config(text=status)
        self.root.after(200, self._tick)

    def set(self, **kwargs) -> None:
        with self.lock:
            for k, v in kwargs.items():
                setattr(self, k, v)

    def run(self) -> None:
        self.root.mainloop()


async def _scan(seconds: float = 8.0):
    from bleak import BleakScanner

    print(f"scanning {seconds:.0f}s — spin the crank, start HR broadcast on the 965")
    devices = await BleakScanner.discover(timeout=seconds, return_adv=True)
    for dev, adv in devices.values():
        uuids = " ".join(adv.service_uuids or [])
        print(f"  {dev.address}  rssi={adv.rssi}  name={dev.name!r}  {uuids}")


async def _connect_loop(hud: Hud) -> None:
    from bleak import BleakClient, BleakScanner

    hr_prev_csc: tuple[int, int] | None = None
    last_crank = 0.0

    while True:
        hud.set(status="scanning…")
        found = await BleakScanner.discover(timeout=8.0, return_adv=True)
        hr_dev = cad_dev = None
        for dev, adv in found.values():
            uuids = {u.lower() for u in (adv.service_uuids or [])}
            name = (dev.name or "").lower()
            if hr_dev is None and (HR_SVC in uuids or any(n in name for n in ("forerunner", "garmin", "fenix", "instinct"))):
                hr_dev = dev
            if cad_dev is None and (CSC_SVC in uuids or any(n in name for n in ("magene", "s3", "cadence"))):
                cad_dev = dev
        bits = []
        if hr_dev:
            bits.append(f"HR {hr_dev.name or hr_dev.address}")
        if cad_dev:
            bits.append(f"CAD {cad_dev.name or cad_dev.address}")
        if not bits:
            hud.set(status="nothing found — broadcast HR + spin crank")
            await asyncio.sleep(2)
            continue
        hud.set(status=" · ".join(bits))

        clients: list = []

        def on_hr(_, data: bytearray) -> None:
            bpm = parse_hr(bytes(data))
            if bpm is not None:
                hud.set(hr=bpm, hr_ok=True)

        def on_csc(_, data: bytearray) -> None:
            nonlocal hr_prev_csc, last_crank
            rpm, hr_prev_csc, mode = parse_csc(bytes(data), hr_prev_csc)
            now = time.monotonic()
            if rpm is not None:
                last_crank = now
                hud.set(rpm=rpm, cad_ok=True, cad_mode=mode)
            elif mode == "wheel":
                hud.set(cad_mode="wheel", status="Magene is in SPEED mode — pop battery, want red flash")
            elif now - last_crank > 3 and hud.rpm is not None:
                hud.set(rpm=0.0, cad_mode=mode)

        try:
            if hr_dev:
                c = BleakClient(hr_dev)
                await c.connect()
                await c.start_notify(HR_UUID, on_hr)
                clients.append(c)
            if cad_dev:
                c = BleakClient(cad_dev)
                await c.connect()
                await c.start_notify(CSC_UUID, on_csc)
                clients.append(c)
            hud.set(status=" · ".join(bits) + "  live")
            while all(c.is_connected for c in clients):
                await asyncio.sleep(1)
        except Exception as e:
            hud.set(status=f"drop: {e}")
        finally:
            for c in clients:
                try:
                    await c.disconnect()
                except Exception:
                    pass
        hud.set(hr_ok=False, cad_ok=False)
        await asyncio.sleep(1)


def main() -> int:
    p = argparse.ArgumentParser(description="965 + Magene S3+ desktop HUD")
    p.add_argument("--scan", action="store_true")
    p.add_argument("--self-check", action="store_true")
    args = p.parse_args()
    if args.self_check:
        return _self_check()
    if args.scan:
        asyncio.run(_scan())
        return 0
    hud = Hud()

    def runner() -> None:
        asyncio.run(_connect_loop(hud))

    threading.Thread(target=runner, daemon=True).start()
    hud.run()
    return 0


if __name__ == "__main__":
    sys.exit(main())
