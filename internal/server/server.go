package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"spin-hud/internal/db"
	"spin-hud/internal/fit"
	"spin-hud/internal/session"
	"spin-hud/internal/strava"
	"spin-hud/internal/workout"
)

var (
	titleCache   = make(map[string]string)
	titleCacheMu sync.RWMutex
	httpClient   = &http.Client{Timeout: 4 * time.Second}
)

type Server struct {
	State        *session.State
	indexHTML    string
	launcherHTML string
	Strava       *strava.Client
	DB           *db.DB
	LANPIN       string

	hubMu          sync.Mutex
	subscribers    map[chan []byte]struct{}
	lastSavedStart time.Time
}

// New builds the HTTP handler set; indexHTML is the embedded UI, launcherHTML is the start page.
func New(state *session.State, indexHTML, launcherHTML string, sc *strava.Client, database *db.DB, lanPIN string) *Server {
	s := &Server{
		State:        state,
		indexHTML:    indexHTML,
		launcherHTML: launcherHTML,
		Strava:       sc,
		DB:           database,
		LANPIN:       lanPIN,
		subscribers:  make(map[chan []byte]struct{}),
	}
	go s.runBroadcaster()
	return s
}

func (s *Server) runBroadcaster() {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for range ticker.C {
		s.hubMu.Lock()
		numSubs := len(s.subscribers)
		s.hubMu.Unlock()
		if numSubs == 0 {
			continue
		}

		snap := s.State.GetSnapshot()
		data, err := json.Marshal(snap)
		if err != nil {
			continue
		}
		msg := []byte(fmt.Sprintf("data: %s\n\n", data))

		s.hubMu.Lock()
		for ch := range s.subscribers {
			select {
			case ch <- msg:
			default:
			}
		}
		s.hubMu.Unlock()
	}
}

func (s *Server) checkAuth(r *http.Request) bool {
	if s.LANPIN == "" {
		return true
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	if host == "127.0.0.1" || host == "::1" || host == "localhost" || host == "" {
		return true
	}
	// Check PIN header or cookie
	pin := r.Header.Get("X-PIN")
	if pin == s.LANPIN {
		return true
	}
	cookie, err := r.Cookie("spin_pin")
	if err == nil && cookie.Value == s.LANPIN {
		return true
	}
	return false
}

func (s *Server) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.checkAuth(r) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]any{
				"ok":    false,
				"error": "Pairing PIN required for LAN control",
				"lan":   true,
			})
			return
		}
		next(w, r)
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/launcher", s.handleLauncher)
	mux.HandleFunc("/api/telemetry", s.handleTelemetry)
	mux.HandleFunc("/api/workout/export.tcx", s.handleTCXExport)
	mux.HandleFunc("/api/workout/export.fit", s.handleFITExport)
	mux.HandleFunc("/api/workout/reset", s.authMiddleware(s.handleReset))
	mux.HandleFunc("/api/workout/toggle", s.authMiddleware(s.handleToggle))
	mux.HandleFunc("/api/settings", s.authMiddleware(s.handleSettings))
	mux.HandleFunc("/api/knob", s.authMiddleware(s.handleKnob))
	mux.HandleFunc("/api/youtube/title", s.handleYouTubeTitle)

	// Auth & LAN PIN
	mux.HandleFunc("/api/auth/pin", s.handleAuthPIN)

	// History
	mux.HandleFunc("/api/history", s.handleHistory)
	mux.HandleFunc("/api/history/", s.handleHistorySubroutes)

	// Workouts
	mux.HandleFunc("/api/workouts", s.handleWorkouts)
	mux.HandleFunc("/api/workouts/import", s.authMiddleware(s.handleWorkoutImport))
	mux.HandleFunc("/api/workouts/", s.handleWorkoutSubroutes)

	// Sensor Health
	mux.HandleFunc("/api/sensors/status", s.handleSensorsStatus)

	// Strava
	mux.HandleFunc("/api/strava/status", s.handleStravaStatus)
	mux.HandleFunc("/api/strava/login", s.handleStravaLogin)
	mux.HandleFunc("/api/strava/callback", s.handleStravaCallback)
	mux.HandleFunc("/api/strava/disconnect", s.authMiddleware(s.handleStravaDisconnect))
	mux.HandleFunc("/api/strava/upload", s.authMiddleware(s.handleStravaUpload))

	return mux
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && !strings.HasPrefix(r.URL.Path, "/index.html") {
		http.NotFound(w, r)
		return
	}
	html := s.indexHTML
	if data, err := os.ReadFile("web/index.html"); err == nil && len(data) > 0 {
		html = string(data)
	}
	rendered := strings.ReplaceAll(html, "__PLAYLIST_ID__", s.State.PlaylistID)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(rendered))
}

func (s *Server) handleLauncher(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/launcher" && !strings.HasPrefix(r.URL.Path, "/launcher.html") {
		http.NotFound(w, r)
		return
	}
	html := s.launcherHTML
	if data, err := os.ReadFile("web/launcher.html"); err == nil && len(data) > 0 {
		html = string(data)
	}
	lanOn := "false"
	pin := ""
	if s.LANPIN != "" {
		lanOn = "true"
		pin = s.LANPIN
	}
	rendered := strings.ReplaceAll(html, "__LAN__", lanOn)
	rendered = strings.ReplaceAll(rendered, "__PIN__", pin)
	rendered = strings.ReplaceAll(rendered, "__PLAYLIST_ID__", s.State.PlaylistID)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(rendered))
}

func (s *Server) handleAuthPIN(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		PIN string `json:"pin"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	w.Header().Set("Content-Type", "application/json")
	if s.LANPIN == "" || strings.TrimSpace(req.PIN) == s.LANPIN {
		http.SetCookie(w, &http.Cookie{
			Name:     "spin_pin",
			Value:    s.LANPIN,
			Path:     "/",
			Expires:  time.Now().Add(30 * 24 * time.Hour),
			SameSite: http.SameSiteLaxMode,
		})
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "authenticated": true})
		return
	}
	w.WriteHeader(http.StatusUnauthorized)
	json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "Invalid PIN"})
}

// handleTelemetry streams the workout snapshot as SSE at 5Hz.
func (s *Server) handleTelemetry(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	snap := s.State.GetSnapshot()
	if data, err := json.Marshal(snap); err == nil {
		fmt.Fprintf(w, "data: %s\n\n", data)
		fl.Flush()
	}

	ch := make(chan []byte, 16)
	s.hubMu.Lock()
	s.subscribers[ch] = struct{}{}
	s.hubMu.Unlock()

	defer func() {
		s.hubMu.Lock()
		delete(s.subscribers, ch)
		s.hubMu.Unlock()
	}()

	for {
		select {
		case <-r.Context().Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			w.Write(msg)
			fl.Flush()
		}
	}
}

func (s *Server) handleTCXExport(w http.ResponseWriter, r *http.Request) {
	tcx := session.GenerateTCX(s.State)
	ts := s.State.WorkoutStartWall().Format("20060102_150405")
	w.Header().Set("Content-Type", "application/vnd.garmin.tcx+xml")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"spin_workout_%s.tcx\"", ts))
	fmt.Fprint(w, tcx)
}

func (s *Server) handleFITExport(w http.ResponseWriter, r *http.Request) {
	snap := s.State.GetSnapshot()
	pts := s.State.GetTrackpoints()
	var fitPts []fit.Trackpoint
	for _, p := range pts {
		fitPts = append(fitPts, fit.Trackpoint{
			Time:      p.Time,
			HR:        p.HR,
			Cadence:   p.Cadence,
			SpeedMps:  p.SpeedMps,
			DistanceM: p.DistM,
			Watts:     p.Watts,
		})
	}
	avgHR := 0
	if snap.AvgHR != nil {
		avgHR = *snap.AvgHR
	}
	maxHR := 0
	if snap.MaxHR != nil {
		maxHR = *snap.MaxHR
	}
	avgCad := 0
	if snap.AvgCadence != nil {
		avgCad = *snap.AvgCadence
	}
	maxCad := 0
	if snap.MaxCadence != nil {
		maxCad = *snap.MaxCadence
	}
	avgSpd := 0.0
	if snap.AvgSpeedMPH != nil {
		avgSpd = *snap.AvgSpeedMPH * 0.44704
	}
	maxSpd := 0.0
	if snap.MaxSpeedMPH != nil {
		maxSpd = *snap.MaxSpeedMPH * 0.44704
	}
	avgWatts := 0
	if snap.AvgWatts != nil {
		avgWatts = *snap.AvgWatts
	}
	maxWatts := 0
	if snap.MaxWatts != nil {
		maxWatts = *snap.MaxWatts
	}

	act := fit.ActivityData{
		StartTime:   s.State.WorkoutStartWall(),
		EndTime:     s.State.WorkoutEndWall(),
		ElapsedSec:  snap.ElapsedSec,
		DistanceM:   snap.DistanceKm * 1000.0,
		Calories:    snap.Calories,
		AvgHR:       avgHR,
		MaxHR:       maxHR,
		AvgCadence:  avgCad,
		MaxCadence:  maxCad,
		AvgSpeedMps: avgSpd,
		MaxSpeedMps: maxSpd,
		AvgWatts:    avgWatts,
		MaxWatts:    maxWatts,
		Trackpoints: fitPts,
	}

	data, err := fit.EncodeActivity(act)
	if err != nil {
		http.Error(w, fmt.Sprintf("FIT export failed: %v", err), http.StatusInternalServerError)
		return
	}
	ts := s.State.WorkoutStartWall().Format("20060102_150405")
	w.Header().Set("Content-Type", "application/vnd.ant.fit")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"spin_workout_%s.fit\"", ts))
	w.Write(data)
}

func (s *Server) autoSaveCurrentRide() {
	if s.DB == nil {
		return
	}
	snap := s.State.GetSnapshot()
	if snap.ElapsedSec < 15 {
		return // ignore accidental momentary restarts
	}
	start := s.State.WorkoutStartWall()
	s.hubMu.Lock()
	if !s.lastSavedStart.IsZero() && s.lastSavedStart.Equal(start) {
		s.hubMu.Unlock()
		return
	}
	s.lastSavedStart = start
	s.hubMu.Unlock()

	end := s.State.WorkoutEndWall()
	pts := s.State.GetTrackpoints()
	name := snap.WorkoutName
	if name == "" {
		name = "Spin Ride"
	}
	_, _ = s.DB.SaveRide(snap, start, end, pts, name)
}

func (s *Server) handleReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.autoSaveCurrentRide()
	s.State.ResetWorkout()
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `{"ok":true}`)
}

func (s *Server) handleToggle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	running := s.State.ToggleWorkoutTimer()
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"running":%t}`, running)
}

func (s *Server) handleKnob(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var data map[string]any
	_ = json.NewDecoder(r.Body).Decode(&data)
	if v, ok := data["knob"]; ok {
		s.State.SetKnob(strings.ToLower(strings.TrimSpace(fmt.Sprint(v))))
	} else if v, ok := data["dir"]; ok {
		dir := strings.ToLower(strings.TrimSpace(fmt.Sprint(v)))
		s.State.NudgeKnob(dir == "tighten" || dir == "up" || dir == "+")
	}
	name, label, turns := s.State.KnobSnapshot()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"ok":         true,
		"knob":       name,
		"knob_label": label,
		"knob_turns": turns,
	})
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var data map[string]any
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil && !errors.Is(err, io.EOF) {
		writeJSONError(w, err.Error())
		return
	}
	var errs []string

	newPl := s.State.PlaylistID
	if v, ok := data["playlist_id"]; ok {
		pl := session.ExtractPlaylistID(fmt.Sprint(v))
		if pl != "" {
			newPl = pl
		} else {
			errs = append(errs, "Playlist ID cannot be empty")
		}
	}

	newCirc := s.State.WheelCirc()
	if v, ok := data["wheel_circ_mm"]; ok {
		if f, ok2 := toFloat(v); ok2 && f >= 500 && f <= 3500 {
			newCirc = f / 1000.0
		} else {
			errs = append(errs, "Wheel circumference must be between 500mm and 3500mm")
		}
	}

	newMaxHR := s.State.MaxHR
	if v, ok := data["max_hr"]; ok {
		if n, ok2 := toInt(v); ok2 && n >= 80 && n <= 240 {
			newMaxHR = n
		} else {
			errs = append(errs, "Max HR must be between 80 and 240 BPM")
		}
	}

	newWeight := s.State.RiderWeightKg
	if v, ok := data["rider_weight_kg"]; ok {
		if f, ok2 := toFloat(v); ok2 && f >= 30.0 && f <= 250.0 {
			newWeight = f
		} else {
			errs = append(errs, "Rider weight must be between 30kg and 250kg")
		}
	}

	if v, ok := data["workout_name"]; ok {
		s.State.SetWorkoutName(fmt.Sprint(v))
	}

	if len(errs) > 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{"ok": false, "errors": errs})
		return
	}
	s.State.ApplySettings(newPl, newCirc, newMaxHR, newWeight)
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `{"ok":true}`)
}

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	if s.DB == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]db.RideSummary{})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	rides, err := s.DB.ListRides(limit, offset)
	if err != nil {
		writeJSONError(w, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rides)
}

func (s *Server) handleHistorySubroutes(w http.ResponseWriter, r *http.Request) {
	if s.DB == nil {
		http.NotFound(w, r)
		return
	}
	// /api/history/{id}[/export.tcx | /export.fit]
	path := strings.TrimPrefix(r.URL.Path, "/api/history/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}

	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		http.Error(w, "invalid ride id", http.StatusBadRequest)
		return
	}

	if r.Method == http.MethodDelete {
		if !s.checkAuth(r) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if err := s.DB.DeleteRide(id); err != nil {
			writeJSONError(w, err.Error())
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"ok":true}`)
		return
	}

	detail, err := s.DB.GetRide(id)
	if err != nil || detail == nil {
		http.NotFound(w, r)
		return
	}

	if len(parts) > 1 {
		sub := parts[1]
		if sub == "export.fit" {
			var fitPts []fit.Trackpoint
			for _, p := range detail.Trackpoints {
				fitPts = append(fitPts, fit.Trackpoint{
					Time:      p.Time,
					HR:        p.HR,
					Cadence:   p.Cadence,
					SpeedMps:  p.SpeedMps,
					DistanceM: p.DistM,
					Watts:     p.Watts,
				})
			}
			avgHR, maxHR, avgCad, maxCad, avgW, maxW := 0, 0, 0, 0, 0, 0
			if detail.AvgHR != nil {
				avgHR = *detail.AvgHR
			}
			if detail.MaxHR != nil {
				maxHR = *detail.MaxHR
			}
			if detail.AvgCadence != nil {
				avgCad = *detail.AvgCadence
			}
			if detail.MaxCadence != nil {
				maxCad = *detail.MaxCadence
			}
			if detail.AvgWatts != nil {
				avgW = *detail.AvgWatts
			}
			if detail.MaxWatts != nil {
				maxW = *detail.MaxWatts
			}
			avgSpd, maxSpd := 0.0, 0.0
			if detail.AvgSpeedMPH != nil {
				avgSpd = *detail.AvgSpeedMPH * 0.44704
			}
			if detail.MaxSpeedMPH != nil {
				maxSpd = *detail.MaxSpeedMPH * 0.44704
			}

			act := fit.ActivityData{
				StartTime:   detail.StartedAt,
				EndTime:     detail.EndedAt,
				ElapsedSec:  detail.DurationSec,
				DistanceM:   detail.DistanceM,
				Calories:    detail.Calories,
				AvgHR:       avgHR,
				MaxHR:       maxHR,
				AvgCadence:  avgCad,
				MaxCadence:  maxCad,
				AvgSpeedMps: avgSpd,
				MaxSpeedMps: maxSpd,
				AvgWatts:    avgW,
				MaxWatts:    maxW,
				Trackpoints: fitPts,
			}
			data, err := fit.EncodeActivity(act)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			ts := detail.StartedAt.Format("20060102_150405")
			w.Header().Set("Content-Type", "application/vnd.ant.fit")
			w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"spin_ride_%s.fit\"", ts))
			w.Write(data)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(detail)
}

func (s *Server) handleWorkouts(w http.ResponseWriter, r *http.Request) {
	builtinList := workout.GetBuiltinWorkouts()
	var all []*workout.Workout
	all = append(all, builtinList...)
	if s.DB != nil {
		if saved, err := s.DB.ListWorkouts(); err == nil {
			all = append(all, saved...)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(all)
}

func (s *Server) handleWorkoutImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var data []byte
	filename := "workout.json"

	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		file, header, err := r.FormFile("file")
		if err != nil {
			writeJSONError(w, fmt.Sprintf("failed to read file: %v", err))
			return
		}
		defer file.Close()
		filename = header.Filename
		data, err = io.ReadAll(file)
		if err != nil {
			writeJSONError(w, fmt.Sprintf("failed to read file data: %v", err))
			return
		}
	} else {
		var err error
		data, err = io.ReadAll(r.Body)
		if err != nil {
			writeJSONError(w, err.Error())
			return
		}
		if name := r.URL.Query().Get("filename"); name != "" {
			filename = name
		}
	}

	wo, err := workout.DetectAndParse(filename, data)
	if err != nil {
		writeJSONError(w, fmt.Sprintf("workout parse error: %v", err))
		return
	}

	if wo.ID == "" {
		wo.ID = fmt.Sprintf("custom_%d", time.Now().UnixNano())
	}

	if s.DB != nil {
		_ = s.DB.SaveWorkout(wo)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"ok":      true,
		"workout": wo,
	})
}

func (s *Server) handleWorkoutSubroutes(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/workouts/")
	if id == "" {
		http.NotFound(w, r)
		return
	}

	for _, bw := range workout.GetBuiltinWorkouts() {
		if bw.ID == id {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(bw)
			return
		}
	}

	if s.DB != nil {
		if wo, err := s.DB.GetWorkout(id); err == nil && wo != nil {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(wo)
			return
		}
	}

	http.NotFound(w, r)
}

func (s *Server) handleSensorsStatus(w http.ResponseWriter, r *http.Request) {
	snap := s.State.GetSnapshot()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"ok":           true,
		"sensors":      snap.Sensors,
		"power_source": snap.PowerSource,
		"status":       snap.Status,
		"lan_secured":  s.LANPIN != "",
	})
}

func writeJSONError(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": msg})
}

func toFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case string:
		var f float64
		if _, err := fmt.Sscanf(x, "%g", &f); err == nil {
			return f, true
		}
	}
	return 0, false
}

func toInt(v any) (int, bool) {
	if f, ok := toFloat(v); ok {
		return int(f), true
	}
	return 0, false
}

func (s *Server) handleYouTubeTitle(w http.ResponseWriter, r *http.Request) {
	vidID := strings.TrimSpace(r.URL.Query().Get("id"))
	if vidID == "" {
		http.Error(w, "missing video id", http.StatusBadRequest)
		return
	}

	titleCacheMu.RLock()
	cached, ok := titleCache[vidID]
	titleCacheMu.RUnlock()
	if ok {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"title": cached})
		return
	}

	resp, err := httpClient.Get(fmt.Sprintf("https://www.youtube.com/oembed?url=https://www.youtube.com/watch?v=%s&format=json", vidID))
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"title": ""})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"title": ""})
		return
	}

	var payload struct {
		Title string `json:"title"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err == nil && payload.Title != "" {
		titleCacheMu.Lock()
		if len(titleCache) > 500 {
			titleCache = make(map[string]string)
		}
		titleCache[vidID] = payload.Title
		titleCacheMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"title": payload.Title})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"title": ""})
}

// Listen binds the listener; on WSAEADDRINUSE-style errors it returns the error.
func Listen(host string, port int) (net.Listener, error) {
	return net.Listen("tcp", net.JoinHostPort(host, fmt.Sprintf("%d", port)))
}
