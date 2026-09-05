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

func TestRecoverActiveSessionTransactional(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "recover_tx_test.db")

	database, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer database.Close()

	now := time.Now()
	start := now.Add(-15 * time.Minute)
	hr := 155
	snap := session.Snapshot{
		ElapsedSec:  900,
		DistanceKm:  6.5,
		DistanceMi:  4.04,
		Calories:    220,
		AvgHR:       &hr,
		WorkoutName: "Tempo Ride",
	}
	points := []session.Trackpoint{
		{Time: start, HR: &hr, Cadence: 90, SpeedMps: 7.5, DistM: 0},
		{Time: start.Add(15 * time.Minute), HR: &hr, Cadence: 90, SpeedMps: 7.5, DistM: 6500},
	}

	if err := database.CheckpointActiveSession(snap, start, now, points, "Tempo Ride"); err != nil {
		t.Fatalf("CheckpointActiveSession failed: %v", err)
	}

	// Recover session
	recID, err := database.RecoverActiveSession()
	if err != nil {
		t.Fatalf("RecoverActiveSession failed: %v", err)
	}
	if recID <= 0 {
		t.Fatalf("expected positive ride ID, got %d", recID)
	}

	// Verify ride is in database
	detail, err := database.GetRide(recID)
	if err != nil || detail == nil {
		t.Fatalf("GetRide failed: %v", err)
	}
	if detail.Calories != 220 || detail.WorkoutName != "Tempo Ride (Recovered)" {
		t.Fatalf("unexpected recovered ride details: %+v", detail)
	}
	if len(detail.Trackpoints) != 2 {
		t.Fatalf("expected 2 recovered trackpoints, got %d", len(detail.Trackpoints))
	}

	// Verify active session checkpoint was cleared
	var count int
	if err := database.db.QueryRow(`SELECT count(*) FROM active_session`).Scan(&count); err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected active_session to be empty after successful recovery, got %d", count)
	}
}

func TestRecoverActiveSessionRetainedOnFailure(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "recover_fail_test.db")

	database, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	now := time.Now()
	start := now.Add(-10 * time.Minute)
	hr := 145
	snap := session.Snapshot{
		ElapsedSec:  600,
		DistanceKm:  4.2,
		Calories:    150,
		AvgHR:       &hr,
		WorkoutName: "Interrupted Ride",
	}

	if err := database.CheckpointActiveSession(snap, start, now, nil, "Interrupted Ride"); err != nil {
		t.Fatalf("CheckpointActiveSession failed: %v", err)
	}

	// Close database connection to force recovery failure
	database.Close()

	recID, err := database.RecoverActiveSession()
	if err == nil {
		t.Fatalf("expected error from closed database, got ID %d", recID)
	}

	// Reopen database and verify active session is still retained!
	reopened, err := Open(dbPath)
	if err != nil {
		t.Fatalf("reopen failed: %v", err)
	}
	defer reopened.Close()

	var count int
	if err := reopened.db.QueryRow(`SELECT count(*) FROM active_session`).Scan(&count); err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected active_session checkpoint to be retained after failure, count = %d", count)
	}
}

func TestSaveSettingsTransactional(t *testing.T) {
	tmpDir := t.TempDir()
	database, err := Open(filepath.Join(tmpDir, "settings_tx_test.db"))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer database.Close()

	m := map[string]string{
		"playlist_id":     "PLXYZ999",
		"wheel_circ_m":    "2.105000",
		"max_hr":          "192",
		"rider_weight_kg": "75.000000",
	}
	if err := database.SaveSettings(m); err != nil {
		t.Fatalf("SaveSettings failed: %v", err)
	}

	loaded, err := database.LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings failed: %v", err)
	}
	for k, v := range m {
		if loaded[k] != v {
			t.Fatalf("mismatch for %s: expected %s, got %s", k, v, loaded[k])
		}
	}
}

func TestPing(t *testing.T) {
	tmpDir := t.TempDir()
	database, err := Open(filepath.Join(tmpDir, "ping_test.db"))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	if err := database.Ping(); err != nil {
		t.Fatalf("expected Ping to succeed on open db, got %v", err)
	}
	database.Close()
	if err := database.Ping(); err == nil {
		t.Fatalf("expected Ping to fail on closed db")
	}
}

func TestCheckWritable(t *testing.T) {
	tmpDir := t.TempDir()
	database, err := Open(filepath.Join(tmpDir, "writable_test.db"))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	if err := database.CheckWritable(); err != nil {
		t.Fatalf("expected CheckWritable to succeed on healthy db, got %v", err)
	}
	database.Close()
	if err := database.CheckWritable(); err == nil {
		t.Fatalf("expected CheckWritable to fail on closed db")
	}
}

func TestSaveRideAndClearCheckpoint(t *testing.T) {
	tmpDir := t.TempDir()
	database, err := Open(filepath.Join(tmpDir, "atomic_finish_test.db"))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer database.Close()

	now := time.Now()
	start := now.Add(-15 * time.Minute)
	hr140 := 140
	hr165 := 165
	snap := session.Snapshot{
		ElapsedSec: 900,
		DistanceKm: 7.5,
		Calories:   220,
		AvgHR:      &hr140,
		MaxHR:      &hr165,
	}
	pts := []session.Trackpoint{
		{Time: start, SpeedMps: 8.5, Cadence: 85},
		{Time: now, SpeedMps: 8.8, Cadence: 90},
	}

	// 1. Setup active checkpoint
	if err := database.CheckpointActiveSession(snap, start, now, pts, "Atomic Finish Ride"); err != nil {
		t.Fatalf("CheckpointActiveSession failed: %v", err)
	}

	// 2. Perform atomic save and clear
	rideID, err := database.SaveRideAndClearCheckpoint(snap, start, now, pts, "Atomic Finish Ride")
	if err != nil {
		t.Fatalf("SaveRideAndClearCheckpoint failed: %v", err)
	}
	if rideID <= 0 {
		t.Fatalf("expected positive rideID, got %d", rideID)
	}

	// 3. Confirm checkpoint is cleared
	recID, err := database.RecoverActiveSession()
	if err != nil {
		t.Fatalf("RecoverActiveSession failed: %v", err)
	}
	if recID != 0 {
		t.Fatalf("expected no active session to recover, got ride ID %d", recID)
	}

	// 4. Confirm ride was persisted
	rides, err := database.ListRides(10, 0)
	if err != nil {
		t.Fatalf("ListRides failed: %v", err)
	}
	if len(rides) != 1 || rides[0].ID != rideID {
		t.Fatalf("expected 1 ride with id %d, got %+v", rideID, rides)
	}
}
