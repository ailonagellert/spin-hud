package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"spin-hud/internal/session"
)

var (
	titleCache   = make(map[string]string)
	titleCacheMu sync.RWMutex
	httpClient   = &http.Client{Timeout: 4 * time.Second}
)

type Server struct {
	State      *session.State
	indexHTML  string
}

// New builds the HTTP handler set; indexHTML is the embedded UI with the
// __PLAYLIST_ID__ placeholder still present.
func New(state *session.State, indexHTML string) *Server {
	return &Server{State: state, indexHTML: indexHTML}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/api/telemetry", s.handleTelemetry)
	mux.HandleFunc("/api/workout/export.tcx", s.handleTCXExport)
	mux.HandleFunc("/api/workout/reset", s.handleReset)
	mux.HandleFunc("/api/workout/toggle", s.handleToggle)
	mux.HandleFunc("/api/settings", s.handleSettings)
	mux.HandleFunc("/api/youtube/title", s.handleYouTubeTitle)
	return mux
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && !strings.HasPrefix(r.URL.Path, "/index.html") {
		http.NotFound(w, r)
		return
	}
	rendered := strings.ReplaceAll(s.indexHTML, "__PLAYLIST_ID__", s.State.PlaylistID)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	fmt.Fprint(w, rendered)
}

// handleTelemetry streams the workout snapshot as SSE at 5Hz (matches Python).
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

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			snap := s.State.GetSnapshot()
			data, err := json.Marshal(snap)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
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

func (s *Server) handleReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
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
		pl := strings.TrimSpace(fmt.Sprint(v))
		if strings.Contains(pl, "list=") {
			pl = strings.SplitN(pl, "list=", 2)[1]
			pl = strings.SplitN(pl, "&", 2)[0]
		}
		if pl != "" {
			newPl = pl
		} else {
			errs = append(errs, "Playlist ID cannot be empty")
		}
	}

	newCirc := s.State.WheelCircM
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

	// Fetch via YouTube oEmbed
	resp, err := httpClient.Get(fmt.Sprintf("https://www.youtube.com/oembed?url=https://www.youtube.com/watch?v=%s&format=json", vidID))
	if err != nil || resp.StatusCode != http.StatusOK {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"title": ""})
		return
	}
	defer resp.Body.Close()

	var payload struct {
		Title string `json:"title"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err == nil && payload.Title != "" {
		titleCacheMu.Lock()
		titleCache[vidID] = payload.Title
		titleCacheMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"title": payload.Title})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"title": ""})
}

// Listen binds the listener; on WSAEADDRINUSE-style errors it returns the
// error so the caller can open the browser to the existing instance and exit.
func Listen(host string, port int) (net.Listener, error) {
	return net.Listen("tcp", net.JoinHostPort(host, fmt.Sprintf("%d", port)))
}
