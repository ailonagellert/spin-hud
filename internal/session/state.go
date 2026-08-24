package session

import (
	"math"
	"sync"
	"time"
)

// SensorState mirrors the Python sensors dict entries.
type SensorState struct {
	Connected bool   `json:"connected"`
	Name      string `json:"name"`
}

type SensorsSnapshot struct {
	HR      SensorState `json:"hr"`
	Cadence SensorState `json:"cadence"`
	Speed   SensorState `json:"speed"`
}

// Snapshot is the exact JSON shape served on /api/telemetry (SSE).
// Field names and null semantics must match spin_hud.py get_snapshot.
type Snapshot struct {
	HR            *int           `json:"hr"`
	AvgHR         *int           `json:"avg_hr"`
	MaxHR         *int           `json:"max_hr"`
	HRZone        int            `json:"hr_zone"`
	HRZoneName    string         `json:"hr_zone_name"`
	HRZonePct     int            `json:"hr_zone_pct"`
	HRZoneColor   string         `json:"hr_zone_color"`
	Cadence       *int           `json:"cadence"`
	AvgCadence    *int           `json:"avg_cadence"`
	MaxCadence    *int           `json:"max_cadence"`
	SpeedMPH      *float64       `json:"speed_mph"`
	AvgSpeedMPH   *float64       `json:"avg_speed_mph"`
	MaxSpeedMPH   *float64       `json:"max_speed_mph"`
	SpeedKMH      *float64       `json:"speed_kmh"`
	AvgSpeedKMH   *float64       `json:"avg_speed_kmh"`
	MaxSpeedKMH   *float64       `json:"max_speed_kmh"`
	Watts         int            `json:"watts"`
	AvgWatts      *int           `json:"avg_watts"`
	MaxWatts      *int           `json:"max_watts"`
	WKg           float64        `json:"w_kg"`
	AvgWKg        *float64       `json:"avg_w_kg"`
	DistanceMi    float64        `json:"distance_mi"`
	DistanceKm    float64        `json:"distance_km"`
	Calories      int            `json:"calories"`
	ElapsedSec    int            `json:"elapsed_sec"`
	IsRunning     bool           `json:"is_running"`
	Sensors       SensorsSnapshot `json:"sensors"`
	Status        string         `json:"status"`
	PlaylistID    string         `json:"playlist_id"`
	RiderWeightKg float64        `json:"rider_weight_kg"`
}

// Trackpoint is a periodic sample recorded for TCX export (every 2s while running).
type Trackpoint struct {
	Time     time.Time
	HR       *int
	Cadence  int
	SpeedMps float64
	DistM    float64
	Watts    int
}

const (
	DefaultMaxHR        = 190
	DefaultPlaylistID   = "PLBE6A702D02AB879D"
	DefaultWheelCircM   = 1.4363 // 18" flywheel circumference (pi * 18" = 1436 mm)
	DefaultRiderWeightKg = 75.0
)

// State is the Go port of Python WorkoutState.
type State struct {
	mu sync.RWMutex

	PlaylistID    string
	WheelCircM    float64
	MaxHR         int
	RiderWeightKg float64

	hr       *int
	cadence  *float64
	speedMPH *float64
	sensors  SensorsSnapshot
	status   string

	hrSum      float64
	hrCount    int
	maxHRVal   int
	cadSum     float64
	cadCount   int
	maxCad     float64
	spdSum     float64
	spdCount   int
	maxSpdMPH  float64
	wattsSum   float64
	wattsCount int
	maxWatts   int
	calories   float64

	distanceMiles float64

	lastCalTime time.Time

	isRunning        bool
	startedAt        time.Time
	pausedDuration   time.Duration
	lastPauseTime    time.Time
	workoutStartWall time.Time

	trackpoints      []Trackpoint
	lastTrackpointAt time.Time
}

func NewState(playlistID string) *State {
	now := time.Now()
	return &State{
		PlaylistID:    playlistID,
		WheelCircM:    DefaultWheelCircM,
		MaxHR:         DefaultMaxHR,
		RiderWeightKg: DefaultRiderWeightKg,
		sensors: SensorsSnapshot{
			HR:      SensorState{Name: "Searching…"},
			Cadence: SensorState{Name: "Searching…"},
			Speed:   SensorState{Name: "Searching…"},
		},
		status:           "Initializing sensors…",
		startedAt:        now,
		workoutStartWall: now,
		lastCalTime:      now,
		isRunning:        true,
	}
}

// ResetWorkout restarts the timer from zero (POST /api/workout/reset).
// CSC baselines must also be invalidated by the BLE layer on reset.
func (s *State) ResetWorkout() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	s.startedAt = now
	s.workoutStartWall = now
	s.pausedDuration = 0
	s.lastPauseTime = time.Time{}
	s.isRunning = true

	s.hrSum, s.hrCount = 0, 0
	s.maxHRVal = 0
	s.cadSum, s.cadCount = 0, 0
	s.maxCad = 0
	s.spdSum, s.spdCount = 0, 0
	s.maxSpdMPH = 0
	s.wattsSum, s.wattsCount = 0, 0
	s.maxWatts = 0
	s.calories = 0
	s.lastCalTime = now
	s.distanceMiles = 0
	s.trackpoints = nil
	s.lastTrackpointAt = time.Time{}
}

// ToggleWorkoutTimer pauses/resumes the timer; returns new running state.
func (s *State) ToggleWorkoutTimer() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	if s.isRunning {
		s.isRunning = false
		s.lastPauseTime = now
	} else {
		s.isRunning = true
		if !s.lastPauseTime.IsZero() {
			s.pausedDuration += now.Sub(s.lastPauseTime)
		}
		s.lastCalTime = now
	}
	return s.isRunning
}

// SetSensor updates sensor connectivity display state.
func (s *State) SetSensor(kind string, connected bool, name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := SensorState{Connected: connected, Name: name}
	switch kind {
	case "hr":
		s.sensors.HR = st
	case "cadence":
		s.sensors.Cadence = st
	case "speed":
		s.sensors.Speed = st
	}
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }
func round2(v float64) float64 { return math.Round(v*100) / 100 }

func iptr(v int) *int         { return &v }
func fptr(v float64) *float64 { return &v }

func iRoundPtr(v float64) *int { r := int(math.Round(v)); return &r }

// HRZone mirrors calculate_hr_zone in spin_hud.py.
func HRZone(bpm *int, maxHR int) (zone int, name string, pct int, color string) {
	if bpm == nil || *bpm <= 0 {
		return 0, "Resting", 0, "#64748b"
	}
	r := float64(*bpm) / float64(maxHR)
	switch {
	case r < 0.60:
		return 1, "Warm Up", int(r * 100), "#38bdf8"
	case r < 0.70:
		return 2, "Fat Burn", int(r * 100), "#22c55e"
	case r < 0.80:
		return 3, "Aerobic", int(r * 100), "#eab308"
	case r < 0.90:
		return 4, "Anaerobic", int(r * 100), "#f97316"
	default:
		return 5, "Max Peak", int(r * 100), "#ef4444"
	}
}

// GetSnapshot computes the SSE payload; also records trackpoints while running.
func (s *State) GetSnapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	var elapsed int
	if s.isRunning {
		elapsed = int(now.Sub(s.startedAt).Seconds() - s.pausedDuration.Seconds())
	} else {
		elapsed = int(s.lastPauseTime.Sub(s.startedAt).Seconds() - s.pausedDuration.Seconds())
	}
	if elapsed < 0 {
		elapsed = 0
	}

	zone, zoneName, zonePct, zoneColor := HRZone(s.hr, s.MaxHR)

	speedMPH := s.speedMPH
	var speedKMH *float64
	if speedMPH != nil {
		speedKMH = fptr(round1(*speedMPH * 1.60934))
	}
	distKM := round2(s.distanceMiles * 1.60934)

	// Virtual power: watts = 3.5*v + 0.35*v^3 (v m/s), only above 0.5 mph
	currentWatts := 0
	if speedMPH != nil && *speedMPH > 0.5 {
		v := *speedMPH * 0.44704
		currentWatts = int(math.Round(3.5*v + 0.35*math.Pow(v, 3)))
	}

	var avgWatts *int
	if s.wattsCount > 0 {
		avgWatts = iRoundPtr(s.wattsSum / float64(s.wattsCount))
	}
	wKg := 0.0
	if s.RiderWeightKg > 0 && currentWatts > 0 {
		wKg = round1(float64(currentWatts) / s.RiderWeightKg)
	}
	var avgWKg *float64
	if s.RiderWeightKg > 0 && avgWatts != nil {
		avgWKg = fptr(round1(float64(*avgWatts) / s.RiderWeightKg))
	}

	// Trackpoint recording every 2s while running (first one immediately)
	if s.isRunning && now.Sub(s.lastTrackpointAt).Seconds() >= 2.0 {
		s.lastTrackpointAt = now
		cad := 0
		if s.cadence != nil {
			cad = int(math.Round(*s.cadence))
		}
		sMps := 0.0
		if speedMPH != nil {
			sMps = round2(*speedMPH * 0.44704)
		}
		s.trackpoints = append(s.trackpoints, Trackpoint{
			Time:     now,
			HR:       s.hr,
			Cadence:  cad,
			SpeedMps: sMps,
			DistM:    round1(s.distanceMiles * 1609.344),
			Watts:    currentWatts,
		})
	}

	var avgHR, maxHROut, avgCad, maxCadOut, cadOut, maxWattsOut *int
	var avgSpdMPH, maxSpdMPHOut, avgSpdKMH, maxSpdKMHOut *float64

	if s.maxWatts > 0 {
		maxWattsOut = iptr(s.maxWatts)
	}
	if s.hrCount > 0 {
		avgHR = iRoundPtr(s.hrSum / float64(s.hrCount))
	}
	if s.maxHRVal > 0 {
		maxHROut = iptr(s.maxHRVal)
	}
	if s.cadCount > 0 {
		avgCad = iRoundPtr(s.cadSum / float64(s.cadCount))
	}
	if s.maxCad > 0 {
		maxCadOut = iRoundPtr(s.maxCad)
	}
	if s.cadence != nil {
		cadOut = iRoundPtr(*s.cadence)
	}
	if s.spdCount > 0 {
		v := round1(s.spdSum / float64(s.spdCount))
		avgSpdMPH = &v
		k := round1(v * 1.60934)
		avgSpdKMH = &k
	}
	if s.maxSpdMPH > 0 {
		maxSpdMPHOut = fptr(round1(s.maxSpdMPH))
		maxSpdKMHOut = fptr(round1(s.maxSpdMPH * 1.60934))
	}

	return Snapshot{
		HR:            s.hr,
		AvgHR:         avgHR,
		MaxHR:         maxHROut,
		HRZone:        zone,
		HRZoneName:    zoneName,
		HRZonePct:     zonePct,
		HRZoneColor:   zoneColor,
		Cadence:       cadOut,
		AvgCadence:    avgCad,
		MaxCadence:    maxCadOut,
		SpeedMPH:      speedMPH,
		AvgSpeedMPH:   avgSpdMPH,
		MaxSpeedMPH:   maxSpdMPHOut,
		SpeedKMH:      speedKMH,
		AvgSpeedKMH:   avgSpdKMH,
		MaxSpeedKMH:   maxSpdKMHOut,
		Watts:         currentWatts,
		AvgWatts:      avgWatts,
		MaxWatts:      maxWattsOut,
		WKg:           wKg,
		AvgWKg:        avgWKg,
		DistanceMi:    round2(s.distanceMiles),
		DistanceKm:    distKM,
		Calories:      int(math.Round(s.calories)),
		ElapsedSec:    elapsed,
		IsRunning:     s.isRunning,
		Sensors:       s.sensors,
		Status:        s.status,
		PlaylistID:    s.PlaylistID,
		RiderWeightKg: s.RiderWeightKg,
	}
}
