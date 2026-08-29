package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"spin-hud/internal/session"
)

func (s *Server) handleStravaStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, s.Strava.GetStatus())
}

func (s *Server) handleStravaLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	u, err := s.Strava.AuthURL()
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": err.Error()})
		return
	}
	http.Redirect(w, r, u, http.StatusFound)
}

func (s *Server) handleStravaCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if errStr := r.URL.Query().Get("error"); errStr != "" {
		http.Redirect(w, r, "/?strava=denied", http.StatusFound)
		return
	}
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	if err := s.Strava.Exchange(code, state); err != nil {
		http.Redirect(w, r, "/?strava=error", http.StatusFound)
		return
	}
	http.Redirect(w, r, "/?strava=ok", http.StatusFound)
}

func (s *Server) handleStravaDisconnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	_ = s.Strava.Disconnect()
	writeJSON(w, map[string]any{"ok": true})
}

type stravaUploadReq struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (s *Server) handleStravaUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req stravaUploadReq
	_ = json.NewDecoder(r.Body).Decode(&req)
	req.Name = strings.TrimSpace(req.Name)
	req.Description = strings.TrimSpace(req.Description)
	if req.Name == "" {
		req.Name = "Spin Studio"
	}
	snap := s.State.GetSnapshot()
	tcx := session.GenerateTCX(s.State)
	res := s.Strava.UploadTCX(req.Name, req.Description, []byte(tcx), s.State.WorkoutStartWall().Unix(), snap.ElapsedSec)
	if !res.OK && res.Error != "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(res)
		return
	}
	writeJSON(w, res)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		fmt.Fprint(w, `{"ok":false}`)
	}
}
