package session

import (
	"math"
	"time"
)

// Telemetry is the input bundle for UpdateTelemetry (mirrors Python kwargs).
// Only non-nil fields are applied.
type Telemetry struct {
	HR            *int
	Cadence       *float64
	SpeedMPH      *float64
	DistanceMiles *float64
	PowerWatts    *int
	PowerSource   *string
	Status        *string
}

// UpdateTelemetry applies new sensor values and accumulates statistics
// only for the fields that were updated (mirrors update_telemetry).
func (s *State) UpdateTelemetry(t Telemetry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()

	if t.PowerSource != nil {
		s.powerSource = *t.PowerSource
	}

	// Heart Rate update
	if t.HR != nil {
		newHR := *t.HR
		s.hr = &newHR
		if s.isRunning && newHR > 0 {
			s.hrSum += float64(newHR)
			s.hrCount++
			if newHR > s.maxHRVal {
				s.maxHRVal = newHR
			}
			// Calorie integration: forward only, bounded to sane dt windows
			dt := now.Sub(s.lastCalTime).Seconds()
			if dt > 0 && dt < 10.0 {
				if newHR > 75 {
					s.calories += (float64(newHR) - 55) * 0.0022 * dt
				}
			}
			s.lastCalTime = now
		}
	}

	// Cadence update
	if t.Cadence != nil {
		newCad := *t.Cadence
		s.cadence = &newCad
		if s.isRunning && newCad > 0 {
			s.cadSum += newCad
			s.cadCount++
			if newCad > s.maxCad {
				s.maxCad = newCad
			}
		}
	}

	// Direct Power update (BLE power meter or FTMS)
	if t.PowerWatts != nil {
		w := *t.PowerWatts
		s.powerWatts = &w
		if s.isRunning && w >= 0 {
			s.wattsSum += float64(w)
			s.wattsCount++
			if w > s.maxWatts {
				s.maxWatts = w
			}
		}
	}

	// Speed update & virtual power accumulation
	if t.SpeedMPH != nil {
		newSpd := *t.SpeedMPH
		s.speedMPH = &newSpd
		if s.isRunning && newSpd > 0.5 {
			s.spdSum += newSpd
			s.spdCount++
			if newSpd > s.maxSpdMPH {
				s.maxSpdMPH = newSpd
			}
			// Accumulate virtual power only if real meter is not active
			if s.powerSource == "estimated" || s.powerWatts == nil {
				w := VirtualWatts(newSpd, s.Knob)
				s.wattsSum += float64(w)
				s.wattsCount++
				if w > s.maxWatts {
					s.maxWatts = w
				}
			}
		}
	}

	// Distance override (if specified directly)
	if t.DistanceMiles != nil && s.isRunning {
		s.distanceMiles = *t.DistanceMiles
	}

	if t.Status != nil {
		s.status = *t.Status
	}

	s.maybeRecordTrackpointLocked(now)
}

// AddDistanceDelta adds delta distance accumulated from wheel revolutions.
func (s *State) AddDistanceDelta(deltaMiles float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.isRunning && deltaMiles > 0 {
		s.distanceMiles += deltaMiles
		s.maybeRecordTrackpointLocked(time.Now())
	}
}

func (s *State) maybeRecordTrackpointLocked(now time.Time) {
	if !s.isRunning {
		return
	}
	if !s.lastTrackpointAt.IsZero() && now.Sub(s.lastTrackpointAt) < 1*time.Second {
		// Enrich recent trackpoint if updated within 500ms window
		if len(s.trackpoints) > 0 && now.Sub(s.lastTrackpointAt) < 500*time.Millisecond {
			idx := len(s.trackpoints) - 1
			if s.hr != nil {
				s.trackpoints[idx].HR = s.hr
			}
			if s.cadence != nil {
				s.trackpoints[idx].Cadence = int(math.Round(*s.cadence))
			}
			if s.powerWatts != nil && s.powerSource != "estimated" {
				s.trackpoints[idx].Watts = *s.powerWatts
			} else if s.speedMPH != nil && *s.speedMPH > 0.5 {
				s.trackpoints[idx].Watts = VirtualWatts(*s.speedMPH, s.Knob)
			}
			if s.speedMPH != nil && *s.speedMPH > 0.5 {
				sMps := round2(*s.speedMPH * 0.44704)
				s.trackpoints[idx].SpeedMps = sMps
			}
			s.trackpoints[idx].DistM = round1(s.distanceMiles * 1609.344)
		}
		return
	}
	s.lastTrackpointAt = now
	cad := 0
	if s.cadence != nil {
		cad = int(math.Round(*s.cadence))
	}
	sMps := 0.0
	currentWatts := 0
	if s.powerWatts != nil && s.powerSource != "estimated" {
		currentWatts = *s.powerWatts
	} else if s.speedMPH != nil && *s.speedMPH > 0.5 {
		currentWatts = VirtualWatts(*s.speedMPH, s.Knob)
	}
	if s.speedMPH != nil && *s.speedMPH > 0.5 {
		sMps = round2(*s.speedMPH * 0.44704)
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


