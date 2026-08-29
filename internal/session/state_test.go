package session

import (
	"math"
	"strings"
	"testing"
)

func TestDistanceAccumulationAndResetIsolation(t *testing.T) {
	st := NewState(DefaultPlaylistID)
	if st.distanceMiles != 0.0 {
		t.Fatalf("initial distance = %f; want 0", st.distanceMiles)
	}

	st.AddDistanceDelta(0.15)
	st.AddDistanceDelta(0.25)
	if round2(st.distanceMiles) != 0.40 {
		t.Fatalf("accumulated distance = %f; want 0.40", st.distanceMiles)
	}

	// Paused workout ignores distance delta
	st.ToggleWorkoutTimer()
	if st.isRunning {
		t.Fatal("timer should be paused")
	}
	st.AddDistanceDelta(0.50)
	if round2(st.distanceMiles) != 0.40 {
		t.Fatalf("distance while paused = %f; want 0.40", st.distanceMiles)
	}

	// Resume workout
	st.ToggleWorkoutTimer()
	if !st.isRunning {
		t.Fatal("timer should be running")
	}
	st.AddDistanceDelta(0.10)
	if round2(st.distanceMiles) != 0.50 {
		t.Fatalf("resumed distance = %f; want 0.50", st.distanceMiles)
	}

	// Reset workout
	st.ResetWorkout()
	if st.distanceMiles != 0.0 {
		t.Fatalf("reset distance = %f; want 0.0", st.distanceMiles)
	}
	snap := st.GetSnapshot()
	if snap.DistanceMi != 0.0 {
		t.Fatalf("snap distance = %f; want 0.0", snap.DistanceMi)
	}

	// Next incoming delta must not resurrect old distance
	st.AddDistanceDelta(0.02)
	if round2(st.distanceMiles) != 0.02 {
		t.Fatalf("distance after reset = %f; want 0.02", st.distanceMiles)
	}
}

func TestEventFrequencyIndependentAverages(t *testing.T) {
	st := NewState(DefaultPlaylistID)

	// Step 1: HR 100 BPM
	hr1 := 100
	st.UpdateTelemetry(Telemetry{HR: &hr1})

	// Step 2: Five Cadence packets (80 RPM)
	cad := 80.0
	for i := 0; i < 5; i++ {
		st.UpdateTelemetry(Telemetry{Cadence: &cad})
	}

	// Step 3: HR 160 BPM
	hr2 := 160
	st.UpdateTelemetry(Telemetry{HR: &hr2})

	snap := st.GetSnapshot()
	if snap.AvgHR == nil || *snap.AvgHR != 130 {
		t.Fatalf("avg HR = %v; want 130", snap.AvgHR)
	}
	if snap.MaxHR == nil || *snap.MaxHR != 160 {
		t.Fatalf("max HR = %v; want 160", snap.MaxHR)
	}
	if snap.AvgCadence == nil || *snap.AvgCadence != 80 {
		t.Fatalf("avg cadence = %v; want 80", snap.AvgCadence)
	}
	if snap.MaxCadence == nil || *snap.MaxCadence != 80 {
		t.Fatalf("max cadence = %v; want 80", snap.MaxCadence)
	}
}

func TestSettingsAndPlaylistHelpers(t *testing.T) {
	st := NewState(DefaultPlaylistID)

	// Apply settings
	st.ApplySettings("PL_CUSTOM_999", 2.105, 185, 80.0)
	if st.PlaylistID != "PL_CUSTOM_999" {
		t.Fatalf("playlist = %s; want PL_CUSTOM_999", st.PlaylistID)
	}
	if math.Abs(st.WheelCirc()-2.105) > 1e-6 {
		t.Fatalf("wheel circ = %f; want 2.105", st.WheelCirc())
	}
	if st.MaxHR != 185 {
		t.Fatalf("max HR = %d; want 185", st.MaxHR)
	}
	if st.RiderWeightKg != 80.0 {
		t.Fatalf("rider weight = %f; want 80.0", st.RiderWeightKg)
	}

	// ExtractPlaylistID
	if ExtractPlaylistID("PL12345") != "PL12345" {
		t.Fatal("raw playlist id failed")
	}
	if ExtractPlaylistID("https://www.youtube.com/watch?v=abc&list=PL12345&index=2") != "PL12345" {
		t.Fatal("url list param extract failed")
	}
}

func TestTCXExportValidation(t *testing.T) {
	st := NewState(DefaultPlaylistID)
	st.AddDistanceDelta(1.0)
	hr := 150
	cad := 85.0
	spd := 20.0
	st.UpdateTelemetry(Telemetry{HR: &hr, Cadence: &cad, SpeedMPH: &spd})

	snap := st.GetSnapshot()
	if snap.Watts <= 0 {
		t.Fatalf("watts = %d; want > 0", snap.Watts)
	}
	if snap.WKg <= 0.0 {
		t.Fatalf("w_kg = %f; want > 0", snap.WKg)
	}

	tcx := GenerateTCX(st)
	for _, want := range []string{
		`<?xml version="1.0" encoding="UTF-8"?>`,
		`<TrainingCenterDatabase`,
		`<Activity Sport="Biking">`,
		`<HeartRateBpm>`,
		`<Cadence>`,
		`<ns2:Watts>`,
	} {
		if !strings.Contains(tcx, want) {
			t.Fatalf("TCX missing expected tag: %s", want)
		}
	}
}

func TestSensorDisconnectTelemetryClearing(t *testing.T) {
	st := NewState(DefaultPlaylistID)
	st.SetSensor("hr", true, "Garmin HR")
	st.SetSensor("cadence", true, "Cadence")
	st.SetSensor("speed", true, "Speed")
	st.SetSensor("power", true, "Power Meter")

	hr := 145
	cad := 92.0
	spd := 22.5
	watts := 210
	pSrc := "meter"
	st.UpdateTelemetry(Telemetry{
		HR:          &hr,
		Cadence:     &cad,
		SpeedMPH:    &spd,
		PowerWatts:  &watts,
		PowerSource: &pSrc,
	})

	snap := st.GetSnapshot()
	if snap.HR == nil || *snap.HR != 145 {
		t.Fatalf("expected HR 145, got %v", snap.HR)
	}
	if snap.Cadence == nil || *snap.Cadence != 92 {
		t.Fatalf("expected Cadence 92, got %v", snap.Cadence)
	}
	if snap.SpeedMPH == nil || *snap.SpeedMPH != 22.5 {
		t.Fatalf("expected SpeedMPH 22.5, got %v", snap.SpeedMPH)
	}
	if snap.Watts != 210 {
		t.Fatalf("expected Watts 210, got %d", snap.Watts)
	}

	// 1. Test SetSensor(..., false, ...) clears live values
	st.SetSensor("hr", false, "Disconnected")
	snap = st.GetSnapshot()
	if snap.HR != nil {
		t.Fatalf("expected nil HR after SetSensor disconnect, got %v", snap.HR)
	}

	st.SetSensor("cadence", false, "Disconnected")
	snap = st.GetSnapshot()
	if snap.Cadence != nil {
		t.Fatalf("expected nil Cadence after SetSensor disconnect, got %v", snap.Cadence)
	}

	st.SetSensor("speed", false, "Disconnected")
	snap = st.GetSnapshot()
	if snap.SpeedMPH != nil {
		t.Fatalf("expected nil SpeedMPH after SetSensor disconnect, got %v", snap.SpeedMPH)
	}

	st.SetSensor("power", false, "Disconnected")
	snap = st.GetSnapshot()
	if snap.Watts != 0 {
		t.Fatalf("expected 0 Watts after power disconnect, got %d", snap.Watts)
	}

	// 2. Test explicit UpdateTelemetry Clear* flags
	st.SetSensor("hr", true, "Garmin HR")
	st.SetSensor("cadence", true, "Cadence")
	st.SetSensor("speed", true, "Speed")
	st.SetSensor("power", true, "Power Meter")
	st.UpdateTelemetry(Telemetry{HR: &hr, Cadence: &cad, SpeedMPH: &spd, PowerWatts: &watts, PowerSource: &pSrc})
	st.UpdateTelemetry(Telemetry{
		ClearHR:      true,
		ClearCadence: true,
		ClearSpeed:   true,
		ClearPower:   true,
	})
	snap = st.GetSnapshot()
	if snap.HR != nil || snap.Cadence != nil || snap.SpeedMPH != nil || snap.Watts != 0 {
		t.Fatalf("expected all cleared telemetry, got HR=%v Cad=%v Spd=%v Watts=%d", snap.HR, snap.Cadence, snap.SpeedMPH, snap.Watts)
	}
}
