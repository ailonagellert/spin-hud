#!/usr/bin/env python3
"""Comprehensive build and regression test suite for spin-hud."""
from __future__ import annotations

import struct
import sys
from spin_hud import (
    INDEX_HTML,
    WorkoutState,
    _self_check,
    calculate_hr_zone,
    parse_csc_crank,
    parse_csc_wheel,
    parse_hr,
)


def test_core_parsers() -> None:
    """Validate BLE packet parsers and self-check."""
    _self_check()


def test_csc_counter_reboot_and_discontinuity() -> None:
    """Ensure sensor reboot, counter reset, or invalid packets do not produce wild spikes."""
    # Crank discontinuity / reboot test
    wprev = (5000, 10000)
    # Sudden drop in counter (sensor rebooted to 2 revs)
    rpm, new_prev = parse_csc_crank(b"\x02" + struct.pack("<HH", 2, 1024), wprev)
    assert rpm == 0.0 or rpm is None
    assert new_prev == (2, 1024)

    # Crank extreme rpm clamping (> 250 RPM)
    # 50 revs in 100 ticks = ~30720 RPM -> must be clamped to 0.0
    rpm, _ = parse_csc_crank(b"\x02" + struct.pack("<HH", 52, 1124), new_prev)
    assert rpm == 0.0

    # Wheel discontinuity / reboot test
    wheel_prev = (50000, 20000)
    # Counter resets to 10
    spd, delta, new_wprev = parse_csc_wheel(b"\x01" + struct.pack("<IH", 10, 1024), wheel_prev)
    assert spd == 0.0
    assert delta == 0.0
    assert new_wprev == (10, 1024)

    # Wheel extreme speed clamping (> 120 mph)
    # 50 revs in 100 ticks -> ~1142 m/s -> ~2556 mph -> must clamp to 0.0
    spd, delta, _ = parse_csc_wheel(b"\x01" + struct.pack("<IH", 60, 1124), new_wprev, wheel_circ_m=2.10)
    assert spd == 0.0
    assert delta == 0.0


def test_distance_accumulation_and_reset_isolation() -> None:
    """Validate that distance accumulates strictly from running deltas and resets cleanly."""
    state = WorkoutState()
    assert state.distance_miles == 0.0

    # Normal accumulation while running
    state.add_distance_delta(0.15)
    state.add_distance_delta(0.25)
    assert round(state.distance_miles, 2) == 0.40

    # Paused workout ignores distance delta
    state.toggle_workout_timer()
    assert state.is_running is False
    state.add_distance_delta(0.50)
    assert round(state.distance_miles, 2) == 0.40  # unchanged

    # Resume workout
    state.toggle_workout_timer()
    assert state.is_running is True
    state.add_distance_delta(0.10)
    assert round(state.distance_miles, 2) == 0.50

    # Reset workout re-zeros distance
    state.reset_workout()
    assert state.distance_miles == 0.0
    snap = state.get_snapshot()
    assert snap["distance_mi"] == 0.0

    # Next incoming delta must not resurrect old 0.50 mi
    state.add_distance_delta(0.02)
    assert round(state.distance_miles, 2) == 0.02


def test_event_frequency_independent_averages() -> None:
    """Validate that frequent sensor updates (e.g. cadence) do not skew other metric averages."""
    state = WorkoutState()

    # Step 1: HR packet 100 BPM
    state.update_telemetry(hr=100)

    # Step 2: Five Cadence packets (80 RPM) with no new HR
    for _ in range(5):
        state.update_telemetry(cadence=80.0)

    # Step 3: HR packet 160 BPM
    state.update_telemetry(hr=160)

    snap = state.get_snapshot()
    # True average of [100, 160] is 130, NOT 108 or 120 from repeated updates
    assert snap["avg_hr"] == 130
    assert snap["max_hr"] == 160
    assert snap["avg_cadence"] == 80
    assert snap["max_cadence"] == 80


def test_hr_zone_and_settings_bounds() -> None:
    """Validate HR zone calculations and settings boundaries."""
    # Zero or negative max_hr must not raise ZeroDivisionError
    z_zero = calculate_hr_zone(140, max_hr=0)
    assert z_zero["zone"] > 0
    z_neg = calculate_hr_zone(140, max_hr=-50)
    assert z_neg["zone"] > 0

    # Normal zone thresholds
    assert calculate_hr_zone(None, 190)["zone"] == 0
    assert calculate_hr_zone(90, 190)["zone"] == 1
    assert calculate_hr_zone(120, 190)["zone"] == 2
    assert calculate_hr_zone(140, 190)["zone"] == 3
    assert calculate_hr_zone(160, 190)["zone"] == 4
    assert calculate_hr_zone(180, 190)["zone"] == 5

    # State bounds
    st = WorkoutState(max_hr=0, wheel_circ_m=-1.0)
    assert st.max_hr == 190
    assert st.wheel_circ_m == 1.4363


def test_html_dynamic_playlist_injection() -> None:
    """Ensure startup playlist is properly injected into HTML template."""
    custom_pl = "PL_CUSTOM_WORKOUT_123"
    rendered = INDEX_HTML.replace("__PLAYLIST_ID__", custom_pl)
    assert f'let playlistId = "{custom_pl}";' in rendered
    assert f'value="{custom_pl}"' in rendered
    assert "btn-export-tcx" in rendered
    assert "val-watts" in rendered
    assert "interval-cue-bar" in rendered


def test_virtual_power_and_tcx_export() -> None:
    """Validate indoor cycling virtual power calculation and TCX XML export."""
    from spin_hud import generate_tcx

    state = WorkoutState(rider_weight_kg=75.0)
    state.add_distance_delta(1.0)
    state.update_telemetry(hr=150, cadence=85.0, speed_mph=20.0)

    snap = state.get_snapshot()
    assert snap["watts"] > 0
    assert snap["w_kg"] > 0.0
    assert snap["rider_weight_kg"] == 75.0

    tcx = generate_tcx(state)
    assert '<?xml version="1.0" encoding="UTF-8"?>' in tcx
    assert '<TrainingCenterDatabase' in tcx
    assert '<Activity Sport="Biking">' in tcx
    assert '<ns2:Watts>' in tcx
    assert '<HeartRateBpm>' in tcx
    assert '<Cadence>' in tcx


def main() -> int:
    test_core_parsers()
    test_csc_counter_reboot_and_discontinuity()
    test_distance_accumulation_and_reset_isolation()
    test_event_frequency_independent_averages()
    test_hr_zone_and_settings_bounds()
    test_html_dynamic_playlist_injection()
    test_virtual_power_and_tcx_export()
    print("All spin-hud regression test suites passed successfully!")
    return 0


if __name__ == "__main__":
    sys.exit(main())


