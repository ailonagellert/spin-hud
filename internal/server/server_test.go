package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"spin-hud/internal/db"
	"spin-hud/internal/session"
	"spin-hud/internal/workout"
)

func setupTestServer(t *testing.T, lanPIN string) (*Server, *db.DB, *session.State) {
	st := session.NewState(session.DefaultPlaylistID)
	tmpDir := t.TempDir()
	database, err := db.Open(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	srv := New(st, "<html><body>Test</body></html>", "<html><body>Launcher</body></html>", nil, database, lanPIN)
	return srv, database, st
}

func TestServerHistoryAndExports(t *testing.T) {
	srv, database, st := setupTestServer(t, "")
	defer database.Close()

	// Seed one ride
	now := time.Now()
	start := now.Add(-20 * time.Minute)
	hr := 150
	spd := 20.0
	st.UpdateTelemetry(session.Telemetry{HR: &hr, SpeedMPH: &spd})
	st.AddDistanceDelta(5.0)

	snap := st.GetSnapshot()
	rideID, err := database.SaveRide(snap, start, now, st.GetTrackpoints(), "Test HIIT")
	if err != nil || rideID <= 0 {
		t.Fatalf("SaveRide failed: %v", err)
	}

	handler := srv.Handler()

	// 1. GET /api/history
	req := httptest.NewRequest(http.MethodGet, "/api/history", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/history status = %d", rec.Code)
	}
	var rides []db.RideSummary
	if err := json.NewDecoder(rec.Body).Decode(&rides); err != nil || len(rides) != 1 {
		t.Fatalf("failed to decode history: %v, count=%d", err, len(rides))
	}

	// 2. GET /api/history/{id}
	req = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/history/%d", rideID), nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/history/{id} status = %d", rec.Code)
	}

	// 3. GET /api/history/{id}/export.fit
	req = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/history/%d/export.fit", rideID), nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET export.fit status = %d", rec.Code)
	}
	if rec.Header().Get("Content-Type") != "application/vnd.ant.fit" {
		t.Fatalf("expected FIT content type, got %s", rec.Header().Get("Content-Type"))
	}

	// 4. GET /api/workout/export.tcx
	req = httptest.NewRequest(http.MethodGet, "/api/workout/export.tcx", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "<TrainingCenterDatabase") {
		t.Fatalf("live TCX export failed: status=%d", rec.Code)
	}

	// 5. GET /api/workout/export.fit
	req = httptest.NewRequest(http.MethodGet, "/api/workout/export.fit", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.Len() < 16 {
		t.Fatalf("live FIT export failed: status=%d, len=%d", rec.Code, rec.Body.Len())
	}
}

func TestServerWorkoutsAndImports(t *testing.T) {
	srv, database, _ := setupTestServer(t, "")
	defer database.Close()
	handler := srv.Handler()

	// 1. GET /api/workouts (should list built-ins)
	req := httptest.NewRequest(http.MethodGet, "/api/workouts", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/workouts status = %d", rec.Code)
	}
	var list []*workout.Workout
	if err := json.NewDecoder(rec.Body).Decode(&list); err != nil || len(list) < 3 {
		t.Fatalf("failed to decode workouts: %v, count=%d", err, len(list))
	}

	// 2. POST /api/workouts/import (import a custom ZWO)
	zwoXML := `<?xml version="1.0" encoding="UTF-8"?>
<workout_file>
    <name>Imported Test Workout</name>
    <workout>
        <SteadyState Duration="300" Power="1.0" Cadence="90"/>
    </workout>
</workout_file>`
	req = httptest.NewRequest(http.MethodPost, "/api/workouts/import?filename=test.zwo", strings.NewReader(zwoXML))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/workouts/import status = %d, body = %s", rec.Code, rec.Body.String())
	}

	// 3. GET /api/sensors/status
	req = httptest.NewRequest(http.MethodGet, "/api/sensors/status", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/sensors/status status = %d", rec.Code)
	}
	var statusResp map[string]any
	json.NewDecoder(rec.Body).Decode(&statusResp)
	if statusResp["ok"] != true || statusResp["power_source"] != "estimated" {
		t.Fatalf("unexpected sensors status: %+v", statusResp)
	}
}

func TestServerLANAuth(t *testing.T) {
	pin := "849201"
	srv, database, _ := setupTestServer(t, pin)
	defer database.Close()
	handler := srv.Handler()

	// Simulate remote request (e.g. 192.168.1.50)
	req := httptest.NewRequest(http.MethodPost, "/api/workout/reset", nil)
	req.RemoteAddr = "192.168.1.50:54321"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("remote request without PIN should be 401, got %d", rec.Code)
	}

	// Authenticate with PIN
	pinBody := `{"pin":"849201"}`
	req = httptest.NewRequest(http.MethodPost, "/api/auth/pin", strings.NewReader(pinBody))
	req.RemoteAddr = "192.168.1.50:54321"
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/auth/pin failed: %d", rec.Code)
	}

	// With X-PIN header
	req = httptest.NewRequest(http.MethodPost, "/api/workout/reset", nil)
	req.RemoteAddr = "192.168.1.50:54321"
	req.Header.Set("X-PIN", pin)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("request with X-PIN header should succeed, got %d", rec.Code)
	}
}

func TestPauseDoesNotDuplicateHistory(t *testing.T) {
	srv, database, st := setupTestServer(t, "")
	defer database.Close()
	handler := srv.Handler()

	// Seed ride telemetry and simulate elapsed time
	hr := 140
	spd := 18.0
	st.UpdateTelemetry(session.Telemetry{HR: &hr, SpeedMPH: &spd})
	st.AddDistanceDelta(1.5)

	// Simulate 20 seconds of ride
	time.Sleep(50 * time.Millisecond)

	// Toggle pause
	req := httptest.NewRequest(http.MethodPost, "/api/workout/toggle", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/workout/toggle failed: %d", rec.Code)
	}

	// Toggle resume
	req = httptest.NewRequest(http.MethodPost, "/api/workout/toggle", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Toggle pause again
	req = httptest.NewRequest(http.MethodPost, "/api/workout/toggle", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Verify history has 0 records (pause should NOT save)
	rides, err := database.ListRides(10, 0)
	if err != nil {
		t.Fatalf("ListRides failed: %v", err)
	}
	if len(rides) != 0 {
		t.Fatalf("expected 0 saved rides after pause toggles, got %d", len(rides))
	}
}

func TestServerWorkoutFinishAndReset(t *testing.T) {
	srv, database, st := setupTestServer(t, "")
	defer database.Close()
	handler := srv.Handler()

	// Seed ride telemetry
	hr := 155
	spd := 21.5
	st.UpdateTelemetry(session.Telemetry{HR: &hr, SpeedMPH: &spd})
	st.AddDistanceDelta(3.2)
	st.SetWorkoutName("Pyramid HIIT Finish Test")

	// 1. POST /api/workout/finish
	req := httptest.NewRequest(http.MethodPost, "/api/workout/finish", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/workout/finish failed: %d", rec.Code)
	}
	var finishResp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&finishResp); err != nil {
		t.Fatalf("failed to decode finish response: %v", err)
	}
	if ok, _ := finishResp["ok"].(bool); !ok {
		t.Fatalf("expected ok: true, got: %v", finishResp)
	}

	// 2. POST /api/workout/reset
	req = httptest.NewRequest(http.MethodPost, "/api/workout/reset", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/workout/reset failed: %d", rec.Code)
	}
	var resetResp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resetResp); err != nil {
		t.Fatalf("failed to decode reset response: %v", err)
	}
	if ok, _ := resetResp["ok"].(bool); !ok {
		t.Fatalf("expected ok: true, got: %v", resetResp)
	}
}

func TestLauncherModeInjection(t *testing.T) {
	st := session.NewState(session.DefaultPlaylistID)
	tmpDir := t.TempDir()
	database, err := db.Open(filepath.Join(tmpDir, "launcher_test.db"))
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	defer database.Close()

	launcherHTML := `<html data-lan="__LAN__" data-pin="__PIN__" data-pl="__PLAYLIST_ID__"></html>`

	// 1. Local Mode (no PIN)
	srvLocal := New(st, "<html>HUD</html>", launcherHTML, nil, database, "")
	rec := httptest.NewRecorder()
	srvLocal.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/launcher", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /launcher local status = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `data-lan="false"`) {
		t.Errorf("expected data-lan=false in local mode, got: %s", body)
	}
	if !strings.Contains(body, `data-pin=""`) {
		t.Errorf("expected empty pin in local mode, got: %s", body)
	}
	if !strings.Contains(body, fmt.Sprintf(`data-pl="%s"`, session.DefaultPlaylistID)) {
		t.Errorf("expected playlist ID injected, got: %s", body)
	}

	// 2. LAN Mode (with PIN)
	srvLAN := New(st, "<html>HUD</html>", launcherHTML, nil, database, "482910")

	// 2a. Remote unauthenticated visitor: must NOT disclose PIN
	recRemote := httptest.NewRecorder()
	reqRemote := httptest.NewRequest(http.MethodGet, "/launcher", nil)
	reqRemote.RemoteAddr = "192.168.1.50:54321"
	srvLAN.Handler().ServeHTTP(recRemote, reqRemote)
	if recRemote.Code != http.StatusOK {
		t.Fatalf("GET /launcher remote status = %d", recRemote.Code)
	}
	bodyRemote := recRemote.Body.String()
	if !strings.Contains(bodyRemote, `data-lan="true"`) {
		t.Errorf("expected data-lan=true in LAN mode, got: %s", bodyRemote)
	}
	if !strings.Contains(bodyRemote, `data-pin=""`) {
		t.Errorf("expected data-pin to be withheld from remote unauthenticated visitor, got: %s", bodyRemote)
	}

	// 2b. Local console (127.0.0.1): displays PIN so rider can pair device
	recLocal := httptest.NewRecorder()
	reqLocal := httptest.NewRequest(http.MethodGet, "/launcher", nil)
	reqLocal.RemoteAddr = "127.0.0.1:54321"
	srvLAN.Handler().ServeHTTP(recLocal, reqLocal)
	if recLocal.Code != http.StatusOK {
		t.Fatalf("GET /launcher local status = %d", recLocal.Code)
	}
	bodyLocal := recLocal.Body.String()
	if !strings.Contains(bodyLocal, `data-pin="482910"`) {
		t.Errorf("expected pin 482910 for local console, got: %s", bodyLocal)
	}

	// 2c. Remote authenticated visitor: displays PIN
	recAuth := httptest.NewRecorder()
	reqAuth := httptest.NewRequest(http.MethodGet, "/launcher", nil)
	reqAuth.RemoteAddr = "192.168.1.50:54321"
	reqAuth.Header.Set("X-PIN", "482910")
	srvLAN.Handler().ServeHTTP(recAuth, reqAuth)
	if recAuth.Code != http.StatusOK {
		t.Fatalf("GET /launcher auth status = %d", recAuth.Code)
	}
	bodyAuth := recAuth.Body.String()
	if !strings.Contains(bodyAuth, `data-pin="482910"`) {
		t.Errorf("expected pin 482910 for authenticated session, got: %s", bodyAuth)
	}
}

func TestServerWorkoutFinishRetryAndSafeReset(t *testing.T) {
	srv, database, st := setupTestServer(t, "")
	defer database.Close()
	handler := srv.Handler()

	// Seed ride telemetry
	hr := 160
	spd := 22.0
	st.UpdateTelemetry(session.Telemetry{HR: &hr, SpeedMPH: &spd})
	st.AddDistanceDelta(4.0)
	st.SetWorkoutName("Retry & Safe Reset Test")
	st.SetStartedAtForTest(time.Now().Add(-10 * time.Minute))

	// 1. Double finish returns same ride ID without duplicate
	req := httptest.NewRequest(http.MethodPost, "/api/workout/finish", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/workout/finish failed: %d", rec.Code)
	}
	var res1 map[string]any
	json.NewDecoder(rec.Body).Decode(&res1)
	rideID1 := int64(res1["ride_id"].(float64))
	if rideID1 <= 0 || res1["saved"] != true {
		t.Fatalf("expected saved ride, got: %v", res1)
	}

	// Second finish
	req2 := httptest.NewRequest(http.MethodPost, "/api/workout/finish", nil)
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	var res2 map[string]any
	json.NewDecoder(rec2.Body).Decode(&res2)
	rideID2 := int64(res2["ride_id"].(float64))
	if rideID2 != rideID1 {
		t.Fatalf("expected second finish to return existing ride ID %d, got %d", rideID1, rideID2)
	}

	// Verify only 1 ride in history
	rides, err := database.ListRides(10, 0)
	if err != nil || len(rides) != 1 {
		t.Fatalf("expected 1 ride in history, got %d (err: %v)", len(rides), err)
	}
}

func TestActiveSessionCrashRecovery(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "recovery_test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	defer database.Close()

	now := time.Now()
	snap := session.Snapshot{
		ElapsedSec: 600,
		DistanceKm: 5.0,
		Calories:   250,
		IsRunning:  true,
	}
	pts := []session.Trackpoint{
		{Time: now.Add(-10 * time.Minute), Cadence: 85, Watts: 180},
		{Time: now.Add(-5 * time.Minute), Cadence: 90, Watts: 200},
	}

	// Checkpoint in-progress active workout
	if err := database.CheckpointActiveSession(snap, now.Add(-10*time.Minute), now, pts, "Morning Spin"); err != nil {
		t.Fatalf("CheckpointActiveSession failed: %v", err)
	}

	// Simulate restart / crash recovery
	recoveredID, err := database.RecoverActiveSession()
	if err != nil || recoveredID <= 0 {
		t.Fatalf("RecoverActiveSession failed: id=%d, err=%v", recoveredID, err)
	}

	// Verify it was archived with "(Recovered)" in history
	detail, err := database.GetRide(recoveredID)
	if err != nil || detail == nil {
		t.Fatalf("failed to get recovered ride detail: %v", err)
	}
	if !strings.Contains(detail.WorkoutName, "Recovered") {
		t.Errorf("expected Recovered in workout name, got: %s", detail.WorkoutName)
	}
	if len(detail.Trackpoints) != 2 {
		t.Errorf("expected 2 trackpoints, got %d", len(detail.Trackpoints))
	}

	// Subsequent recovery should find nothing
	rec2, err := database.RecoverActiveSession()
	if err != nil || rec2 != 0 {
		t.Fatalf("expected no further recoverable session, got %d", rec2)
	}
}

func TestSettingsPersistence(t *testing.T) {
	srv, database, st := setupTestServer(t, "")
	defer database.Close()
	handler := srv.Handler()

	body := `{"playlist_id":"PLTEST123","wheel_circ_mm":2105,"max_hr":185,"rider_weight_kg":78.5,"workout_name":"FTP Test"}`
	req := httptest.NewRequest(http.MethodPost, "/api/settings", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/settings failed: %d", rec.Code)
	}

	// Verify State in memory
	settings := st.GetSettings()
	if settings.PlaylistID != "PLTEST123" || settings.MaxHR != 185 || settings.RiderWeightKg != 78.5 || settings.WorkoutName != "FTP Test" {
		t.Fatalf("unexpected settings in state: %+v", settings)
	}

	// Verify SQLite database
	dbSettings, err := database.LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings failed: %v", err)
	}
	if dbSettings["playlist_id"] != "PLTEST123" || dbSettings["max_hr"] != "185" || dbSettings["workout_name"] != "FTP Test" {
		t.Fatalf("unexpected persisted settings: %+v", dbSettings)
	}
}

func TestServerWorkoutFinishSaveFailureAndRetry(t *testing.T) {
	st := session.NewState(session.DefaultPlaylistID)
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "fail_test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	srv := New(st, "<html>HUD</html>", "<html>Launcher</html>", nil, database, "")

	// Seed ride telemetry and set startedAt >5s ago
	hr := 160
	spd := 22.0
	st.UpdateTelemetry(session.Telemetry{HR: &hr, SpeedMPH: &spd})
	st.AddDistanceDelta(4.0)
	st.SetStartedAtForTest(time.Now().Add(-10 * time.Minute))
	st.SetWorkoutName("Failure & Recovery Ride")

	// 1. Force database write failure by closing database
	database.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/workout/finish", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected HTTP 500 on database failure, got %d", rec.Code)
	}
	var failResp map[string]any
	json.NewDecoder(rec.Body).Decode(&failResp)
	if failResp["ok"] == true || failResp["saved"] == true {
		t.Fatalf("expected ok:false and saved:false on write error, got: %v", failResp)
	}
	if failResp["error"] == "" {
		t.Fatalf("expected error message in response, got empty")
	}

	// 2. Restore storage
	restoredDB, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to reopen db: %v", err)
	}
	defer restoredDB.Close()
	srv.DB = restoredDB

	// 3. Retry POST /api/workout/finish
	reqRetry := httptest.NewRequest(http.MethodPost, "/api/workout/finish", nil)
	recRetry := httptest.NewRecorder()
	srv.Handler().ServeHTTP(recRetry, reqRetry)

	if recRetry.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200 on retry after storage restored, got %d", recRetry.Code)
	}
	var retryResp map[string]any
	json.NewDecoder(recRetry.Body).Decode(&retryResp)
	rideID := int64(retryResp["ride_id"].(float64))
	if rideID <= 0 || retryResp["saved"] != true {
		t.Fatalf("expected ride to be successfully saved on retry, got: %v", retryResp)
	}

	// Verify exactly 1 ride in history
	rides, err := restoredDB.ListRides(10, 0)
	if err != nil || len(rides) != 1 {
		t.Fatalf("expected exactly 1 ride in history, got %d (err: %v)", len(rides), err)
	}
}

func TestServerWorkoutResetSaveFailure(t *testing.T) {
	st := session.NewState(session.DefaultPlaylistID)
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "reset_fail_test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	srv := New(st, "<html>HUD</html>", "<html>Launcher</html>", nil, database, "")

	// Seed ride
	hr := 150
	st.UpdateTelemetry(session.Telemetry{HR: &hr})
	st.AddDistanceDelta(2.5)
	st.SetStartedAtForTest(time.Now().Add(-5 * time.Minute))
	st.SetWorkoutName("Unsaved Reset Ride")

	// Close database to force failure
	database.Close()

	// Normal reset without force must fail and NOT wipe the ride
	req := httptest.NewRequest(http.MethodPost, "/api/workout/reset", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected HTTP 500 when auto-save fails on reset, got %d", rec.Code)
	}

	// Verify workout is preserved in State
	snap := st.GetSnapshot()
	if snap.ElapsedSec < 100 {
		t.Fatalf("expected workout to remain active and preserved, got elapsed: %d", snap.ElapsedSec)
	}

	// Reset with force=true succeeds even if DB fails
	reqForce := httptest.NewRequest(http.MethodPost, "/api/workout/reset?force=true", nil)
	recForce := httptest.NewRecorder()
	srv.Handler().ServeHTTP(recForce, reqForce)

	if recForce.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200 on forced reset, got %d", recForce.Code)
	}
	snapAfter := st.GetSnapshot()
	if snapAfter.ElapsedSec > 1 {
		t.Fatalf("expected workout to be reset, got elapsed: %d", snapAfter.ElapsedSec)
	}
}

func TestWorkoutImportValidationAndLimits(t *testing.T) {
	srv, database, _ := setupTestServer(t, "")
	defer database.Close()
	handler := srv.Handler()

	// 1. Malformed JSON
	reqBad := httptest.NewRequest(http.MethodPost, "/api/workouts/import", strings.NewReader(`{bad json`))
	reqBad.Header.Set("Content-Type", "application/json")
	recBad := httptest.NewRecorder()
	handler.ServeHTTP(recBad, reqBad)
	if recBad.Code != http.StatusBadRequest {
		t.Fatalf("expected HTTP 400 on malformed workout, got %d", recBad.Code)
	}

	// 2. Database failure on save
	database.Close()
	validWorkout := `{"name":"Intervals 4x4","intervals":[{"duration_sec":240,"target_watts":250}]}`
	reqSaveFail := httptest.NewRequest(http.MethodPost, "/api/workouts/import", strings.NewReader(validWorkout))
	reqSaveFail.Header.Set("Content-Type", "application/json")
	recSaveFail := httptest.NewRecorder()
	handler.ServeHTTP(recSaveFail, reqSaveFail)
	if recSaveFail.Code != http.StatusInternalServerError {
		t.Fatalf("expected HTTP 500 when database save fails on import, got %d", recSaveFail.Code)
	}
	var res map[string]any
	json.NewDecoder(recSaveFail.Body).Decode(&res)
	if res["ok"] == true {
		t.Fatalf("expected ok:false when save fails, got: %v", res)
	}
}


