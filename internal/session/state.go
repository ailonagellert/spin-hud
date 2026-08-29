package session

import (
	"math"
	"strings"
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
	Power   SensorState `json:"power"`
	FTMS    SensorState `json:"ftms"`
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
	PowerSource   string         `json:"power_source"`
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
	WorkoutName   string         `json:"workout_name,omitempty"`
	RiderWeightKg float64        `json:"rider_weight_kg"`
	Knob          string         `json:"knob"`
	KnobLabel     string         `json:"knob_label"`
	KnobTurns     float64        `json:"knob_turns"`
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

// Knob stops on Erich's pad-brake: lock, then CCW 1 turn / 1/4 / 1/16.
const (
	KnobLow  = "low"
	KnobMed  = "med"
	KnobHard = "hard"
)

func knobFactor(knob string) float64 {
	switch knob {
	case KnobMed:
		return 2.0
	case KnobHard:
		return 3.5
	default:
		return 1.0
	}
}

func KnobLabelOf(knob string) string {
	switch knob {
	case KnobMed:
		return "MED 1/4"
	case KnobHard:
		return "HARD 1/16"
	default:
		return "LOW 1"
	}
}

func KnobTurnsOf(knob string) float64 {
	switch knob {
	case KnobMed:
		return 0.25
	case KnobHard:
		return 1.0 / 16
	default:
		return 1.0
	}
}

func ParseKnob(knob string) string {
	switch knob {
	case KnobMed, KnobHard:
		return knob
	default:
		return KnobLow
	}
}

func nextKnob(cur string, tighten bool) string {
	order := []string{KnobLow, KnobMed, KnobHard}
	i := 0
	for n, k := range order {
		if k == cur {
			i = n
			break
		}
	}
	if tighten && i < len(order)-1 {
		return order[i+1]
	}
	if !tighten && i > 0 {
		return order[i-1]
	}
	return order[i]
}

// VirtualWatts is the fluid curve at LOW, times the pad-brake stop.
// ponytail: 2.0 / 3.5 are guesses from the 1 / 1/4 / 1/16 stops. Retune when a power meter exists.
func VirtualWatts(speedMPH float64, knob string) int {
	if speedMPH <= 0.5 {
		return 0
	}
	v := speedMPH * 0.44704
	return int(math.Round((3.5*v + 0.35*math.Pow(v, 3)) * knobFactor(knob)))
}

// State is the Go port of Python WorkoutState.
type State struct {
	mu sync.RWMutex

	PlaylistID    string
	WheelCircM    float64
	MaxHR         int
	RiderWeightKg float64
	Knob          string
	WorkoutName   string

	hr          *int
	cadence     *float64
	speedMPH    *float64
	powerWatts  *int
	powerSource string
	sensors     SensorsSnapshot
	status      string

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
	workoutEndWall   time.Time

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
		Knob:          KnobLow,
		powerSource:   "estimated",
		WorkoutName:   "Open Spin Session",
		sensors: SensorsSnapshot{
			HR:      SensorState{Name: "Searching…"},
			Cadence: SensorState{Name: "Searching…"},
			Speed:   SensorState{Name: "Searching…"},
			Power:   SensorState{Name: "Searching…"},
			FTMS:    SensorState{Name: "Searching…"},
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
	s.workoutEndWall = time.Time{}
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
		s.workoutEndWall = now
	} else {
		s.isRunning = true
		if !s.lastPauseTime.IsZero() {
			s.pausedDuration += now.Sub(s.lastPauseTime)
		}
		s.lastCalTime = now
	}
	return s.isRunning
}

func (s *State) SetWorkoutName(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.WorkoutName = name
}

func (s *State) WorkoutEndWall() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.workoutEndWall.IsZero() {
		return time.Now()
	}
	return s.workoutEndWall
}

func (s *State) GetTrackpoints() []Trackpoint {
	s.mu.RLock()
	defer s.mu.RUnlock()
	pts := make([]Trackpoint, len(s.trackpoints))
	copy(pts, s.trackpoints)
	return pts
}

func (s *State) SetKnob(knob string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Knob = ParseKnob(knob)
}

func (s *State) NudgeKnob(tighten bool) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Knob = nextKnob(s.Knob, tighten)
	return s.Knob
}

func (s *State) KnobSnapshot() (name, label string, turns float64) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Knob, KnobLabelOf(s.Knob), KnobTurnsOf(s.Knob)
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
	case "power":
		s.sensors.Power = st
		if connected {
			s.powerSource = "meter"
		} else if !s.sensors.FTMS.Connected {
			s.powerSource = "estimated"
		}
	case "ftms":
		s.sensors.FTMS = st
		if connected {
			s.powerSource = "ftms"
		} else if !s.sensors.Power.Connected {
			s.powerSource = "estimated"
		}
	}
}

// WheelCirc returns the current wheel circumference in meters under read lock.
func (s *State) WheelCirc() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.WheelCircM
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

// GetSnapshot computes the SSE payload.
func (s *State) GetSnapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

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

	// Power: use real meter watts if available, otherwise virtual power
	currentWatts := 0
	pSource := s.powerSource
	if pSource == "" {
		pSource = "estimated"
	}
	if pSource != "estimated" && s.powerWatts != nil {
		currentWatts = *s.powerWatts
	} else if speedMPH != nil && *speedMPH > 0.5 {
		currentWatts = VirtualWatts(*speedMPH, s.Knob)
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
		PowerSource:   pSource,
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
		WorkoutName:   s.WorkoutName,
		RiderWeightKg: s.RiderWeightKg,
		Knob:          s.Knob,
		KnobLabel:     KnobLabelOf(s.Knob),
		KnobTurns:     KnobTurnsOf(s.Knob),
	}
}

// ExtractPlaylistID parses the playlist ID from a raw string or full YouTube URL.
func ExtractPlaylistID(s string) string {
	s = strings.TrimSpace(s)
	if strings.Contains(s, "list=") {
		parts := strings.SplitN(s, "list=", 2)
		return strings.SplitN(parts[1], "&", 2)[0]
	}
	return s
}

