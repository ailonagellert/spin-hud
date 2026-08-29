package db

import (
	"path/filepath"
	"testing"
	"time"

	"spin-hud/internal/session"
	"spin-hud/internal/workout"
)

func TestDBSaveAndListRides(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_spin_hud.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	now := time.Now()
	start := now.Add(-30 * time.Minute)
	hr140 := 140
	maxHR := 170
	cad85 := 85
	maxCad := 115
	spd18 := 18.5
	maxSpd := 24.2
	w190 := 190
	maxW := 350

	snap := session.Snapshot{
		ElapsedSec:  1800,
		DistanceMi:  9.25,
		DistanceKm:  14.88,
		Calories:    350,
		AvgHR:       &hr140,
		MaxHR:       &maxHR,
		AvgCadence:  &cad85,
		MaxCadence:  &maxCad,
		AvgSpeedMPH: &spd18,
		MaxSpeedMPH: &maxSpd,
		AvgWatts:    &w190,
		MaxWatts:    &maxW,
	}

	points := []session.Trackpoint{
		{Time: start, HR: &hr140, Cadence: 85, SpeedMps: 8.2, DistM: 0, Watts: 180},
		{Time: start.Add(15 * time.Minute), HR: &maxHR, Cadence: 100, SpeedMps: 10.0, DistM: 7000, Watts: 280},
	}

	rideID, err := db.SaveRide(snap, start, now, points, "30-Min Sweet Spot")
	if err != nil {
		t.Fatalf("SaveRide failed: %v", err)
	}
	if rideID <= 0 {
		t.Fatalf("invalid ride ID: %d", rideID)
	}

	rides, err := db.ListRides(10, 0)
	if err != nil {
		t.Fatalf("ListRides failed: %v", err)
	}
	if len(rides) != 1 {
		t.Fatalf("expected 1 ride, got %d", len(rides))
	}
	if rides[0].Calories != 350 || rides[0].WorkoutName != "30-Min Sweet Spot" {
		t.Fatalf("ride data mismatch: %+v", rides[0])
	}

	detail, err := db.GetRide(rideID)
	if err != nil {
		t.Fatalf("GetRide failed: %v", err)
	}
	if detail == nil {
		t.Fatal("ride detail is nil")
	}
	if len(detail.Trackpoints) != 2 {
		t.Fatalf("expected 2 trackpoints, got %d", len(detail.Trackpoints))
	}

	// Test Saved Workouts
	w := &workout.Workout{
		ID:          "custom_1",
		Name:        "Custom Intervals",
		Description: "Testing workout save",
		Steps: []workout.WorkoutStep{
			{Name: "Warmup", DurationSec: 300},
		},
	}
	if err := db.SaveWorkout(w); err != nil {
		t.Fatalf("SaveWorkout failed: %v", err)
	}
	wList, err := db.ListWorkouts()
	if err != nil || len(wList) != 1 {
		t.Fatalf("ListWorkouts failed: %v, count=%d", err, len(wList))
	}

	// Test Delete
	if err := db.DeleteRide(rideID); err != nil {
		t.Fatalf("DeleteRide failed: %v", err)
	}
	ridesAfter, _ := db.ListRides(10, 0)
	if len(ridesAfter) != 0 {
		t.Fatalf("expected 0 rides after delete, got %d", len(ridesAfter))
	}
}
