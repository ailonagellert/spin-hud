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
	dbPath := filepath.Join(tmpDir, "server_test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	srv := New(st, "<html><body>Test</body></html>", nil, database, lanPIN)
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
