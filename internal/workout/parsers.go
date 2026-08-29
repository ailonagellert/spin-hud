package workout

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

// WorkoutStep represents an interval block within a workout program.
type WorkoutStep struct {
	Name          string  `json:"name"`
	Type          string  `json:"type"` // warmup, cooldown, steady, interval, freeride
	DurationSec   int     `json:"duration_sec"`
	PowerLowPct   float64 `json:"power_low_pct,omitempty"`   // fraction of FTP (e.g. 0.85 = 85%)
	PowerHighPct  float64 `json:"power_high_pct,omitempty"`  // fraction of FTP
	TargetWatts   int     `json:"target_watts,omitempty"`    // absolute watts if specified
	CadenceTarget int     `json:"cadence_target,omitempty"`  // target RPM
	CadenceLow    int     `json:"cadence_low,omitempty"`     // RPM range low
	CadenceHigh   int     `json:"cadence_high,omitempty"`    // RPM range high
	Knob          string  `json:"knob,omitempty"`            // low, med, hard
	Color         string  `json:"color,omitempty"`           // UI hex color
	CueMessage    string  `json:"cue_message,omitempty"`     // coaching cue or description
}

// Workout represents a complete structured training session or scenario.
type Workout struct {
	ID               string        `json:"id"`
	Name             string        `json:"name"`
	Description      string        `json:"description,omitempty"`
	Author           string        `json:"author,omitempty"`
	Category         string        `json:"category,omitempty"`
	TotalDurationSec int           `json:"total_duration_sec"`
	VideoID          string        `json:"video_id,omitempty"`
	VideoOffsetSec   float64       `json:"video_offset_sec,omitempty"`
	Steps            []WorkoutStep `json:"steps"`
}

// CalculateTotalDuration calculates and sets the TotalDurationSec.
func (w *Workout) CalculateTotalDuration() {
	total := 0
	for _, st := range w.Steps {
		total += st.DurationSec
	}
	w.TotalDurationSec = total
}

// ParseJSON parses a JSON workout/scenario file.
func ParseJSON(data []byte) (*Workout, error) {
	var w Workout
	if err := json.Unmarshal(data, &w); err != nil {
		return nil, fmt.Errorf("invalid workout JSON: %w", err)
	}
	if w.Name == "" {
		w.Name = "Custom JSON Workout"
	}
	w.CalculateTotalDuration()
	return &w, nil
}

// ZWO XML Structures
type ZWOWorkoutFile struct {
	XMLName     xml.Name   `xml:"workout_file"`
	Author      string     `xml:"author"`
	Name        string     `xml:"name"`
	Description string     `xml:"description"`
	SportType   string     `xml:"sportType"`
	Tags        ZWOTags    `xml:"tags"`
	Workout     ZWOWorkout `xml:"workout"`
}

type ZWOTags struct {
	Tag []ZWOTag `xml:"tag"`
}

type ZWOTag struct {
	Name string `xml:"name,attr"`
}

type ZWOWorkout struct {
	Warmup      []ZWOWarmup      `xml:"Warmup"`
	SteadyState []ZWOSteadyState `xml:"SteadyState"`
	IntervalsT  []ZWOIntervalsT  `xml:"IntervalsT"`
	Cooldown    []ZWOCooldown    `xml:"Cooldown"`
	FreeRide    []ZWOFreeRide    `xml:"FreeRide"`
	Ramp        []ZWORamp        `xml:"Ramp"`
}

type ZWOWarmup struct {
	Duration    int     `xml:"Duration,attr"`
	PowerLow    float64 `xml:"PowerLow,attr"`
	PowerHigh   float64 `xml:"PowerHigh,attr"`
	Cadence     int     `xml:"Cadence,attr"`
	CadenceLow  int     `xml:"CadenceLow,attr"`
	CadenceHigh int     `xml:"CadenceHigh,attr"`
}

type ZWOSteadyState struct {
	Duration    int     `xml:"Duration,attr"`
	Power       float64 `xml:"Power,attr"`
	Cadence     int     `xml:"Cadence,attr"`
	CadenceLow  int     `xml:"CadenceLow,attr"`
	CadenceHigh int     `xml:"CadenceHigh,attr"`
}

type ZWOIntervalsT struct {
	Repeat         int     `xml:"Repeat,attr"`
	OnDuration     int     `xml:"OnDuration,attr"`
	OffDuration    int     `xml:"OffDuration,attr"`
	OnPower        float64 `xml:"OnPower,attr"`
	OffPower       float64 `xml:"OffPower,attr"`
	Cadence        int     `xml:"Cadence,attr"`
	CadenceResting int     `xml:"CadenceResting,attr"`
}

type ZWOCooldown struct {
	Duration    int     `xml:"Duration,attr"`
	PowerLow    float64 `xml:"PowerLow,attr"`
	PowerHigh   float64 `xml:"PowerHigh,attr"`
	Cadence     int     `xml:"Cadence,attr"`
	CadenceLow  int     `xml:"CadenceLow,attr"`
	CadenceHigh int     `xml:"CadenceHigh,attr"`
}

type ZWOFreeRide struct {
	Duration int `xml:"Duration,attr"`
	Cadence  int `xml:"Cadence,attr"`
}

type ZWORamp struct {
	Duration  int     `xml:"Duration,attr"`
	PowerLow  float64 `xml:"PowerLow,attr"`
	PowerHigh float64 `xml:"PowerHigh,attr"`
	Cadence   int     `xml:"Cadence,attr"`
}

// ParseZWO parses a Zwift XML (.zwo) workout file.
func ParseZWO(data []byte) (*Workout, error) {
	var zwo ZWOWorkoutFile
	if err := xml.Unmarshal(data, &zwo); err != nil {
		return nil, fmt.Errorf("invalid ZWO XML: %w", err)
	}

	w := &Workout{
		Name:        zwo.Name,
		Description: zwo.Description,
		Author:      zwo.Author,
		Category:    "Zwift",
	}
	if w.Name == "" {
		w.Name = "Zwift Workout"
	}

	// Process steps
	for _, wu := range zwo.Workout.Warmup {
		w.Steps = append(w.Steps, WorkoutStep{
			Name:          "Warm Up",
			Type:          "warmup",
			DurationSec:   wu.Duration,
			PowerLowPct:   wu.PowerLow,
			PowerHighPct:  wu.PowerHigh,
			CadenceTarget: wu.Cadence,
			CadenceLow:    wu.CadenceLow,
			CadenceHigh:   wu.CadenceHigh,
			Knob:          "low",
			Color:         "#38bdf8",
		})
	}
	for _, ss := range zwo.Workout.SteadyState {
		knob := "med"
		color := "#22c55e"
		if ss.Power >= 1.05 {
			knob = "hard"
			color = "#ef4444"
		} else if ss.Power < 0.75 {
			knob = "low"
			color = "#38bdf8"
		}
		w.Steps = append(w.Steps, WorkoutStep{
			Name:          "Steady State",
			Type:          "steady",
			DurationSec:   ss.Duration,
			PowerLowPct:   ss.Power,
			PowerHighPct:  ss.Power,
			CadenceTarget: ss.Cadence,
			CadenceLow:    ss.CadenceLow,
			CadenceHigh:   ss.CadenceHigh,
			Knob:          knob,
			Color:         color,
		})
	}
	for _, iv := range zwo.Workout.IntervalsT {
		repeat := iv.Repeat
		if repeat <= 0 {
			repeat = 1
		}
		for i := 1; i <= repeat; i++ {
			// ON interval
			w.Steps = append(w.Steps, WorkoutStep{
				Name:          fmt.Sprintf("Interval %d/%d (ON)", i, repeat),
				Type:          "interval",
				DurationSec:   iv.OnDuration,
				PowerLowPct:   iv.OnPower,
				PowerHighPct:  iv.OnPower,
				CadenceTarget: iv.Cadence,
				Knob:          "hard",
				Color:         "#ef4444",
			})
			// OFF interval
			if iv.OffDuration > 0 {
				w.Steps = append(w.Steps, WorkoutStep{
					Name:          fmt.Sprintf("Recovery %d/%d (OFF)", i, repeat),
					Type:          "recovery",
					DurationSec:   iv.OffDuration,
					PowerLowPct:   iv.OffPower,
					PowerHighPct:  iv.OffPower,
					CadenceTarget: iv.CadenceResting,
					Knob:          "low",
					Color:         "#22c55e",
				})
			}
		}
	}
	for _, rp := range zwo.Workout.Ramp {
		w.Steps = append(w.Steps, WorkoutStep{
			Name:          "Ramp",
			Type:          "ramp",
			DurationSec:   rp.Duration,
			PowerLowPct:   rp.PowerLow,
			PowerHighPct:  rp.PowerHigh,
			CadenceTarget: rp.Cadence,
			Knob:          "med",
			Color:         "#eab308",
		})
	}
	for _, fr := range zwo.Workout.FreeRide {
		w.Steps = append(w.Steps, WorkoutStep{
			Name:          "Free Ride",
			Type:          "freeride",
			DurationSec:   fr.Duration,
			CadenceTarget: fr.Cadence,
			Knob:          "low",
			Color:         "#38bdf8",
		})
	}
	for _, cd := range zwo.Workout.Cooldown {
		w.Steps = append(w.Steps, WorkoutStep{
			Name:          "Cool Down",
			Type:          "cooldown",
			DurationSec:   cd.Duration,
			PowerLowPct:   cd.PowerLow,
			PowerHighPct:  cd.PowerHigh,
			CadenceTarget: cd.Cadence,
			Knob:          "low",
			Color:         "#38bdf8",
		})
	}

	w.CalculateTotalDuration()
	return w, nil
}

// ParseERGMRC parses Computrainer ERG (watts) or MRC (% FTP) format.
func ParseERGMRC(data []byte) (*Workout, error) {
	lines := strings.Split(string(data), "\n")
	w := &Workout{
		Category: "ERG/MRC",
	}

	inHeader := false
	inData := false
	isWatts := false

	type point struct {
		min float64
		val float64
	}
	var pts []point

	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		upper := strings.ToUpper(line)
		if upper == "[COURSE HEADER]" {
			inHeader = true
			continue
		} else if upper == "[END COURSE HEADER]" {
			inHeader = false
			continue
		} else if upper == "[COURSE DATA]" {
			inData = true
			continue
		} else if upper == "[END COURSE DATA]" {
			inData = false
			continue
		}

		if inHeader {
			if strings.HasPrefix(upper, "DESCRIPTION") && strings.Contains(line, "=") {
				parts := strings.SplitN(line, "=", 2)
				w.Description = strings.TrimSpace(parts[1])
			} else if strings.HasPrefix(upper, "FILE NAME") && strings.Contains(line, "=") {
				parts := strings.SplitN(line, "=", 2)
				w.Name = strings.TrimSpace(parts[1])
			} else if strings.Contains(upper, "MINUTES WATTS") {
				isWatts = true
			}
		} else if inData {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				m, err1 := strconv.ParseFloat(fields[0], 64)
				v, err2 := strconv.ParseFloat(fields[1], 64)
				if err1 == nil && err2 == nil {
					pts = append(pts, point{min: m, val: v})
				}
			}
		}
	}

	if len(pts) < 2 {
		return nil, errors.New("insufficient course data points")
	}
	if w.Name == "" {
		if w.Description != "" {
			w.Name = w.Description
		} else {
			w.Name = "ERG/MRC Workout"
		}
	}

	// Pair consecutive points into interval blocks
	for i := 0; i < len(pts)-1; i += 2 {
		p1 := pts[i]
		p2 := pts[i+1]
		durSec := int(MathRound((p2.min - p1.min) * 60.0))
		if durSec <= 0 {
			// Single-point step transition, adjust index
			if i+2 < len(pts) {
				p2 = pts[i+2]
				durSec = int(MathRound((p2.min - p1.min) * 60.0))
				i++
			}
		}
		if durSec <= 0 {
			continue
		}

		step := WorkoutStep{
			DurationSec: durSec,
		}

		if isWatts {
			step.TargetWatts = int(MathRound(p1.val))
			step.Name = fmt.Sprintf("%dW Steady", step.TargetWatts)
			step.Type = "steady"
			if step.TargetWatts > 250 {
				step.Knob = "hard"
				step.Color = "#ef4444"
			} else if step.TargetWatts > 150 {
				step.Knob = "med"
				step.Color = "#eab308"
			} else {
				step.Knob = "low"
				step.Color = "#38bdf8"
			}
		} else {
			pLow := p1.val / 100.0
			pHigh := p2.val / 100.0
			step.PowerLowPct = pLow
			step.PowerHighPct = pHigh
			pctInt := int(MathRound(pLow * 100.0))
			if pctInt >= 105 {
				step.Name = fmt.Sprintf("%d%% FTP Surge", pctInt)
				step.Type = "interval"
				step.Knob = "hard"
				step.Color = "#ef4444"
			} else if pctInt >= 75 {
				step.Name = fmt.Sprintf("%d%% FTP Tempo", pctInt)
				step.Type = "steady"
				step.Knob = "med"
				step.Color = "#22c55e"
			} else {
				step.Name = fmt.Sprintf("%d%% FTP Recovery", pctInt)
				step.Type = "recovery"
				step.Knob = "low"
				step.Color = "#38bdf8"
			}
		}

		w.Steps = append(w.Steps, step)
	}

	w.CalculateTotalDuration()
	return w, nil
}

func MathRound(v float64) float64 {
	if v < 0 {
		return float64(int(v - 0.5))
	}
	return float64(int(v + 0.5))
}

// DetectAndParse parses workout content by inspecting file extension or content.
func DetectAndParse(filename string, data []byte) (*Workout, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	content := strings.TrimSpace(string(data))

	if ext == ".json" || strings.HasPrefix(content, "{") {
		return ParseJSON(data)
	}
	if ext == ".zwo" || strings.Contains(content, "<workout_file>") {
		return ParseZWO(data)
	}
	if ext == ".erg" || ext == ".mrc" || strings.Contains(content, "[COURSE HEADER]") {
		return ParseERGMRC(data)
	}

	// Try JSON then ZWO then ERG
	if w, err := ParseJSON(data); err == nil && len(w.Steps) > 0 {
		return w, nil
	}
	if w, err := ParseZWO(data); err == nil && len(w.Steps) > 0 {
		return w, nil
	}
	if w, err := ParseERGMRC(data); err == nil && len(w.Steps) > 0 {
		return w, nil
	}

	return nil, errors.New("unsupported workout format (must be JSON, ZWO, ERG, or MRC)")
}
