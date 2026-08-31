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
	recLAN := httptest.NewRecorder()
	srvLAN.Handler().ServeHTTP(recLAN, httptest.NewRequest(http.MethodGet, "/launcher", nil))
	if recLAN.Code != http.StatusOK {
		t.Fatalf("GET /launcher LAN status = %d", recLAN.Code)
	}
	bodyLAN := recLAN.Body.String()
	if !strings.Contains(bodyLAN, `data-lan="true"`) {
		t.Errorf("expected data-lan=true in LAN mode, got: %s", bodyLAN)
	}
	if !strings.Contains(bodyLAN, `data-pin="482910"`) {
		t.Errorf("expected pin 482910 in LAN mode, got: %s", bodyLAN)
	}
}
