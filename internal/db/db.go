package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"

	"spin-hud/internal/session"
	"spin-hud/internal/workout"
)

type DB struct {
	db *sql.DB
	mu sync.RWMutex
}

type RideSummary struct {
	ID          int64     `json:"id"`
	StartedAt   time.Time `json:"started_at"`
	EndedAt     time.Time `json:"ended_at"`
	DurationSec int       `json:"duration_sec"`
	DistanceM   float64   `json:"distance_m"`
	DistanceMi  float64   `json:"distance_mi"`
	DistanceKm  float64   `json:"distance_km"`
	Calories    int       `json:"calories"`
	AvgHR       *int      `json:"avg_hr"`
	MaxHR       *int      `json:"max_hr"`
	AvgCadence  *int      `json:"avg_cadence"`
	MaxCadence  *int      `json:"max_cadence"`
	AvgSpeedMPH *float64  `json:"avg_speed_mph"`
	MaxSpeedMPH *float64  `json:"max_speed_mph"`
	AvgWatts    *int      `json:"avg_watts"`
	MaxWatts    *int      `json:"max_watts"`
	WorkoutName string    `json:"workout_name"`
	Notes       string    `json:"notes,omitempty"`
}

type RideDetail struct {
	RideSummary
	Trackpoints []session.Trackpoint `json:"trackpoints"`
}

// Open initializes or opens the SQLite database and runs migrations.
func Open(path string) (*DB, error) {
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			path = "spin_hud.db"
		} else {
			path = filepath.Join(home, ".spin_hud.db")
		}
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	// Pragmas for fast reliable writes
	if _, err := db.Exec(`
		PRAGMA journal_mode = WAL;
		PRAGMA synchronous = NORMAL;
		PRAGMA foreign_keys = ON;
	`); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to set pragmas: %w", err)
	}

	schema := `
	CREATE TABLE IF NOT EXISTS rides (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		started_at TEXT NOT NULL,
		ended_at TEXT NOT NULL,
		duration_sec INTEGER NOT NULL,
		distance_m REAL NOT NULL,
		calories INTEGER NOT NULL,
		avg_hr INTEGER,
		max_hr INTEGER,
		avg_cadence INTEGER,
		max_cadence INTEGER,
		avg_speed_mph REAL,
		max_speed_mph REAL,
		avg_watts INTEGER,
		max_watts INTEGER,
		workout_name TEXT,
		notes TEXT
	);

	CREATE INDEX IF NOT EXISTS idx_rides_started_at ON rides(started_at DESC);

	CREATE TABLE IF NOT EXISTS trackpoints (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		ride_id INTEGER NOT NULL REFERENCES rides(id) ON DELETE CASCADE,
		time TEXT NOT NULL,
		elapsed_sec INTEGER NOT NULL,
		hr INTEGER,
		cadence INTEGER,
		speed_mph REAL,
		watts INTEGER,
		distance_m REAL
	);

	CREATE INDEX IF NOT EXISTS idx_trackpoints_ride_id ON trackpoints(ride_id, elapsed_sec);

	CREATE TABLE IF NOT EXISTS saved_workouts (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		category TEXT,
		content_json TEXT NOT NULL,
		created_at TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS active_session (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		updated_at TEXT NOT NULL,
		started_at TEXT NOT NULL,
		ended_at TEXT NOT NULL,
		workout_name TEXT NOT NULL,
		snapshot_json TEXT NOT NULL,
		trackpoints_json TEXT NOT NULL
	);
	`

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create tables: %w", err)
	}

	return &DB{db: db}, nil
}

func (d *DB) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.db != nil {
		return d.db.Close()
	}
	return nil
}

// Ping verifies the database connection is alive and responsive.
func (d *DB) Ping() error {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.db == nil {
		return errors.New("database closed")
	}
	return d.db.Ping()
}

// SaveRide saves a completed workout ride session and its 1Hz trackpoints.
func (d *DB) SaveRide(snap session.Snapshot, startWall, endWall time.Time, points []session.Trackpoint, workoutName string) (int64, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.saveRideLocked(snap, startWall, endWall, points, workoutName)
}

func saveRideTx(tx *sql.Tx, snap session.Snapshot, startWall, endWall time.Time, points []session.Trackpoint, workoutName, notes string) (int64, error) {
	distM := snap.DistanceKm * 1000.0

	res, err := tx.Exec(`
		INSERT INTO rides (
			started_at, ended_at, duration_sec, distance_m, calories,
			avg_hr, max_hr, avg_cadence, max_cadence, avg_speed_mph, max_speed_mph,
			avg_watts, max_watts, workout_name, notes
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		startWall.Format(time.RFC3339),
		endWall.Format(time.RFC3339),
		snap.ElapsedSec,
		distM,
		snap.Calories,
		snap.AvgHR,
		snap.MaxHR,
		snap.AvgCadence,
		snap.MaxCadence,
		snap.AvgSpeedMPH,
		snap.MaxSpeedMPH,
		snap.AvgWatts,
		snap.MaxWatts,
		workoutName,
		notes,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to insert ride: %w", err)
	}

	rideID, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}

	if len(points) > 0 {
		stmt, err := tx.Prepare(`
			INSERT INTO trackpoints (ride_id, time, elapsed_sec, hr, cadence, speed_mph, watts, distance_m)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`)
		if err != nil {
			return 0, err
		}
		defer stmt.Close()

		for idx, pt := range points {
			spdMph := pt.SpeedMps * 2.236936
			var hrVal *int
			if pt.HR != nil {
				hrVal = pt.HR
			}
			elapsed := idx * 2
			if !startWall.IsZero() {
				elapsed = int(pt.Time.Sub(startWall).Seconds())
				if elapsed < 0 {
					elapsed = idx * 2
				}
			}

			if _, err := stmt.Exec(
				rideID,
				pt.Time.Format(time.RFC3339),
				elapsed,
				hrVal,
				pt.Cadence,
				spdMph,
				pt.Watts,
				pt.DistM,
			); err != nil {
				return 0, fmt.Errorf("failed to insert trackpoint: %w", err)
			}
		}
	}

	return rideID, nil
}

func (d *DB) saveRideLocked(snap session.Snapshot, startWall, endWall time.Time, points []session.Trackpoint, workoutName string) (int64, error) {
	tx, err := d.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	rideID, err := saveRideTx(tx, snap, startWall, endWall, points, workoutName, "")
	if err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return rideID, nil
}

// ListRides retrieves recent ride summaries.
func (d *DB) ListRides(limit, offset int) ([]RideSummary, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if limit <= 0 {
		limit = 50
	}

	rows, err := d.db.Query(`
		SELECT id, started_at, ended_at, duration_sec, distance_m, calories,
		       avg_hr, max_hr, avg_cadence, max_cadence, avg_speed_mph, max_speed_mph,
		       avg_watts, max_watts, workout_name, COALESCE(notes, '')
		FROM rides
		ORDER BY started_at DESC
		LIMIT ? OFFSET ?
	`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rides []RideSummary
	for rows.Next() {
		var r RideSummary
		var startStr, endStr string
		var avgHR, maxHR, avgCad, maxCad, avgW, maxW sql.NullInt64
		var avgSpd, maxSpd sql.NullFloat64

		if err := rows.Scan(
			&r.ID, &startStr, &endStr, &r.DurationSec, &r.DistanceM, &r.Calories,
			&avgHR, &maxHR, &avgCad, &maxCad, &avgSpd, &maxSpd,
			&avgW, &maxW, &r.WorkoutName, &r.Notes,
		); err != nil {
			return nil, err
		}

		r.StartedAt, _ = time.Parse(time.RFC3339, startStr)
		r.EndedAt, _ = time.Parse(time.RFC3339, endStr)
		r.DistanceKm = r.DistanceM / 1000.0
		r.DistanceMi = r.DistanceM / 1609.344

		if avgHR.Valid {
			v := int(avgHR.Int64)
			r.AvgHR = &v
		}
		if maxHR.Valid {
			v := int(maxHR.Int64)
			r.MaxHR = &v
		}
		if avgCad.Valid {
			v := int(avgCad.Int64)
			r.AvgCadence = &v
		}
		if maxCad.Valid {
			v := int(maxCad.Int64)
			r.MaxCadence = &v
		}
		if avgSpd.Valid {
			v := avgSpd.Float64
			r.AvgSpeedMPH = &v
		}
		if maxSpd.Valid {
			v := maxSpd.Float64
			r.MaxSpeedMPH = &v
		}
		if avgW.Valid {
			v := int(avgW.Int64)
			r.AvgWatts = &v
		}
		if maxW.Valid {
			v := int(maxW.Int64)
			r.MaxWatts = &v
		}

		rides = append(rides, r)
	}

	return rides, rows.Err()
}

// GetRide retrieves full details of a specific ride including its trackpoints.
func (d *DB) GetRide(id int64) (*RideDetail, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var r RideSummary
	var startStr, endStr string
	var avgHR, maxHR, avgCad, maxCad, avgW, maxW sql.NullInt64
	var avgSpd, maxSpd sql.NullFloat64

	err := d.db.QueryRow(`
		SELECT id, started_at, ended_at, duration_sec, distance_m, calories,
		       avg_hr, max_hr, avg_cadence, max_cadence, avg_speed_mph, max_speed_mph,
		       avg_watts, max_watts, workout_name, COALESCE(notes, '')
		FROM rides
		WHERE id = ?
	`, id).Scan(
		&r.ID, &startStr, &endStr, &r.DurationSec, &r.DistanceM, &r.Calories,
		&avgHR, &maxHR, &avgCad, &maxCad, &avgSpd, &maxSpd,
		&avgW, &maxW, &r.WorkoutName, &r.Notes,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	r.StartedAt, _ = time.Parse(time.RFC3339, startStr)
	r.EndedAt, _ = time.Parse(time.RFC3339, endStr)
	r.DistanceKm = r.DistanceM / 1000.0
	r.DistanceMi = r.DistanceM / 1609.344
	if avgHR.Valid {
		v := int(avgHR.Int64)
		r.AvgHR = &v
	}
	if maxHR.Valid {
		v := int(maxHR.Int64)
		r.MaxHR = &v
	}
	if avgCad.Valid {
		v := int(avgCad.Int64)
		r.AvgCadence = &v
	}
	if maxCad.Valid {
		v := int(maxCad.Int64)
		r.MaxCadence = &v
	}
	if avgSpd.Valid {
		v := avgSpd.Float64
		r.AvgSpeedMPH = &v
	}
	if maxSpd.Valid {
		v := maxSpd.Float64
		r.MaxSpeedMPH = &v
	}
	if avgW.Valid {
		v := int(avgW.Int64)
		r.AvgWatts = &v
	}
	if maxW.Valid {
		v := int(maxW.Int64)
		r.MaxWatts = &v
	}

	rows, err := d.db.Query(`
		SELECT time, hr, cadence, speed_mph, watts, distance_m
		FROM trackpoints
		WHERE ride_id = ?
		ORDER BY elapsed_sec ASC
	`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []session.Trackpoint
	for rows.Next() {
		var pt session.Trackpoint
		var tStr string
		var hrVal sql.NullInt64
		var spdMph float64
		if err := rows.Scan(&tStr, &hrVal, &pt.Cadence, &spdMph, &pt.Watts, &pt.DistM); err != nil {
			return nil, err
		}
		pt.Time, _ = time.Parse(time.RFC3339, tStr)
		if hrVal.Valid {
			v := int(hrVal.Int64)
			pt.HR = &v
		}
		pt.SpeedMps = spdMph * 0.44704
		points = append(points, pt)
	}

	return &RideDetail{
		RideSummary: r,
		Trackpoints: points,
	}, nil
}

// DeleteRide deletes a ride and cascading trackpoints.
func (d *DB) DeleteRide(id int64) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, err := d.db.Exec("DELETE FROM rides WHERE id = ?", id)
	return err
}

// SaveWorkout persists an imported workout into SQLite.
func (d *DB) SaveWorkout(w *workout.Workout) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	data, err := json.Marshal(w)
	if err != nil {
		return err
	}

	_, err = d.db.Exec(`
		INSERT INTO saved_workouts (id, name, category, content_json, created_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			category = excluded.category,
			content_json = excluded.content_json
	`, w.ID, w.Name, w.Category, string(data), time.Now().Format(time.RFC3339))
	return err
}

// ListWorkouts retrieves all saved workouts.
func (d *DB) ListWorkouts() ([]*workout.Workout, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	rows, err := d.db.Query(`SELECT content_json FROM saved_workouts ORDER BY name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*workout.Workout
	for rows.Next() {
		var content string
		if err := rows.Scan(&content); err != nil {
			return nil, err
		}
		var w workout.Workout
		if err := json.Unmarshal([]byte(content), &w); err == nil {
			list = append(list, &w)
		}
	}
	return list, nil
}

// GetWorkout retrieves a saved workout by ID.
func (d *DB) GetWorkout(id string) (*workout.Workout, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var content string
	err := d.db.QueryRow(`SELECT content_json FROM saved_workouts WHERE id = ?`, id).Scan(&content)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	var w workout.Workout
	if err := json.Unmarshal([]byte(content), &w); err != nil {
		return nil, err
	}
	return &w, nil
}

// SaveSetting stores a key-value setting.
func (d *DB) SaveSetting(key, val string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, err := d.db.Exec(`INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, val)
	return err
}

// SaveSettings stores multiple key-value settings in a single transaction.
func (d *DB) SaveSettings(settings map[string]string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.db == nil {
		return errors.New("database closed")
	}

	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for k, v := range settings {
		if _, err := stmt.Exec(k, v); err != nil {
			return fmt.Errorf("failed to save setting %s: %w", k, err)
		}
	}

	return tx.Commit()
}

// GetSetting retrieves a setting value by key.
func (d *DB) GetSetting(key string) (string, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	var val string
	err := d.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&val)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return val, nil
}

// LoadSettings returns all stored settings.
func (d *DB) LoadSettings() (map[string]string, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	rows, err := d.db.Query(`SELECT key, value FROM settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		m[k] = v
	}
	return m, nil
}

// CheckpointActiveSession updates the recoverable in-progress workout checkpoint.
func (d *DB) CheckpointActiveSession(snap session.Snapshot, startWall, endWall time.Time, points []session.Trackpoint, workoutName string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	snapJSON, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	ptsJSON, err := json.Marshal(points)
	if err != nil {
		return err
	}

	_, err = d.db.Exec(`
		INSERT INTO active_session (id, updated_at, started_at, ended_at, workout_name, snapshot_json, trackpoints_json)
		VALUES (1, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			updated_at = excluded.updated_at,
			started_at = excluded.started_at,
			ended_at = excluded.ended_at,
			workout_name = excluded.workout_name,
			snapshot_json = excluded.snapshot_json,
			trackpoints_json = excluded.trackpoints_json
	`,
		time.Now().Format(time.RFC3339),
		startWall.Format(time.RFC3339),
		endWall.Format(time.RFC3339),
		workoutName,
		string(snapJSON),
		string(ptsJSON),
	)
	return err
}

// ClearActiveSession removes any active workout checkpoint.
func (d *DB) ClearActiveSession() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, err := d.db.Exec(`DELETE FROM active_session WHERE id = 1`)
	return err
}

// RecoverActiveSession checks if an unfinished workout checkpoint exists from a crash.
// If valid (>5s elapsed), it archives it into the rides table as a recovered session and clears the checkpoint atomically.
func (d *DB) RecoverActiveSession() (int64, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.db == nil {
		return 0, errors.New("database closed")
	}

	var startedStr, endedStr, name, snapJSON, ptsJSON string
	err := d.db.QueryRow(`SELECT started_at, ended_at, workout_name, snapshot_json, trackpoints_json FROM active_session WHERE id = 1`).
		Scan(&startedStr, &endedStr, &name, &snapJSON, &ptsJSON)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return 0, err
	}

	var snap session.Snapshot
	if err := json.Unmarshal([]byte(snapJSON), &snap); err != nil {
		return 0, fmt.Errorf("failed to decode active session snapshot: %w", err)
	}
	var pts []session.Trackpoint
	if err := json.Unmarshal([]byte(ptsJSON), &pts); err != nil {
		return 0, fmt.Errorf("failed to decode active session trackpoints: %w", err)
	}

	startWall, _ := time.Parse(time.RFC3339, startedStr)
	endWall, _ := time.Parse(time.RFC3339, endedStr)
	if endWall.IsZero() {
		endWall = time.Now()
	}

	if snap.ElapsedSec < 5 {
		_, _ = d.db.Exec(`DELETE FROM active_session WHERE id = 1`)
		return 0, nil
	}

	recoveredName := name
	if recoveredName == "" {
		recoveredName = "Spin Ride"
	}
	if !strings.Contains(recoveredName, "Recovered") {
		recoveredName += " (Recovered)"
	}

	tx, err := d.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("failed to begin recovery transaction: %w", err)
	}
	defer tx.Rollback()

	rideID, err := saveRideTx(tx, snap, startWall, endWall, pts, recoveredName, "Recovered from previous session crash")
	if err != nil {
		return 0, fmt.Errorf("failed to insert recovered ride: %w", err)
	}

	if _, err := tx.Exec(`DELETE FROM active_session WHERE id = 1`); err != nil {
		return 0, fmt.Errorf("failed to clear active session checkpoint: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("failed to commit recovery transaction: %w", err)
	}

	return rideID, nil
}

