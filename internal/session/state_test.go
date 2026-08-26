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
