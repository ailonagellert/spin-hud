package workout

// GetBuiltinWorkouts returns the core workout library.
func GetBuiltinWorkouts() []*Workout {
	return []*Workout{
		{
			ID:          "hiit20",
			Name:        "20-Min Generic HIIT Blast",
			Description: "High-intensity intervals with active recovery",
			Category:    "HIIT",
			Steps: []WorkoutStep{
				{Name: "Warmup Spin", Type: "warmup", DurationSec: 180, CadenceLow: 75, CadenceHigh: 85, CadenceTarget: 80, Knob: "low", Color: "#38bdf8", CueMessage: "Easy aerobic warmup"},
				{Name: "Sprint 1/5", Type: "interval", DurationSec: 30, CadenceLow: 100, CadenceHigh: 120, CadenceTarget: 110, Knob: "hard", Color: "#ef4444", CueMessage: "Max effort sprint!"},
				{Name: "Recovery 1/5", Type: "recovery", DurationSec: 90, CadenceLow: 70, CadenceHigh: 80, CadenceTarget: 75, Knob: "low", Color: "#22c55e", CueMessage: "Spin easily and recover"},
				{Name: "Sprint 2/5", Type: "interval", DurationSec: 30, CadenceLow: 100, CadenceHigh: 120, CadenceTarget: 110, Knob: "hard", Color: "#ef4444", CueMessage: "Drive the cadence!"},
				{Name: "Recovery 2/5", Type: "recovery", DurationSec: 90, CadenceLow: 70, CadenceHigh: 80, CadenceTarget: 75, Knob: "low", Color: "#22c55e", CueMessage: "Breathe and relax"},
				{Name: "Sprint 3/5", Type: "interval", DurationSec: 30, CadenceLow: 100, CadenceHigh: 120, CadenceTarget: 110, Knob: "hard", Color: "#ef4444", CueMessage: "Halfway through!"},
				{Name: "Recovery 3/5", Type: "recovery", DurationSec: 90, CadenceLow: 70, CadenceHigh: 80, CadenceTarget: 75, Knob: "low", Color: "#22c55e", CueMessage: "Smooth pedal strokes"},
				{Name: "Sprint 4/5", Type: "interval", DurationSec: 30, CadenceLow: 100, CadenceHigh: 120, CadenceTarget: 110, Knob: "hard", Color: "#ef4444", CueMessage: "Push through the burn!"},
				{Name: "Recovery 4/5", Type: "recovery", DurationSec: 90, CadenceLow: 70, CadenceHigh: 80, CadenceTarget: 75, Knob: "low", Color: "#22c55e", CueMessage: "One more left"},
				{Name: "FINAL SPRINT 5/5", Type: "interval", DurationSec: 30, CadenceLow: 105, CadenceHigh: 125, CadenceTarget: 115, Knob: "hard", Color: "#ef4444", CueMessage: "ALL OUT EFFORT!"},
				{Name: "Cooldown Spin", Type: "cooldown", DurationSec: 300, CadenceLow: 65, CadenceHigh: 75, CadenceTarget: 70, Knob: "low", Color: "#38bdf8", CueMessage: "Gentle cool down spin"},
			},
		},
		{
			ID:          "climb30",
			Name:        "30-Min Hill Climbs & Cadence Surges",
			Description: "Sustained resistance climbs alternating with seated surges",
			Category:    "Climb",
			Steps: []WorkoutStep{
				{Name: "Warmup Spin", Type: "warmup", DurationSec: 240, CadenceLow: 80, CadenceHigh: 90, CadenceTarget: 85, Knob: "low", Color: "#38bdf8", CueMessage: "Prepare legs for heavy work"},
				{Name: "Seated Tempo Climb 1", Type: "steady", DurationSec: 300, CadenceLow: 70, CadenceHigh: 80, CadenceTarget: 75, Knob: "med", Color: "#eab308", CueMessage: "Rhythmic tempo climb"},
				{Name: "Standing Heavy Climb 1", Type: "interval", DurationSec: 180, CadenceLow: 60, CadenceHigh: 70, CadenceTarget: 65, Knob: "hard", Color: "#ef4444", CueMessage: "Out of saddle climb"},
				{Name: "Recovery Spin", Type: "recovery", DurationSec: 120, CadenceLow: 75, CadenceHigh: 85, CadenceTarget: 80, Knob: "low", Color: "#22c55e", CueMessage: "Flush legs"},
				{Name: "Seated Tempo Climb 2", Type: "steady", DurationSec: 300, CadenceLow: 70, CadenceHigh: 80, CadenceTarget: 75, Knob: "med", Color: "#eab308", CueMessage: "Find the climbing groove"},
				{Name: "Standing Heavy Climb 2", Type: "interval", DurationSec: 180, CadenceLow: 60, CadenceHigh: 70, CadenceTarget: 65, Knob: "hard", Color: "#ef4444", CueMessage: "Heavy resistance push"},
				{Name: "Recovery Spin", Type: "recovery", DurationSec: 120, CadenceLow: 75, CadenceHigh: 85, CadenceTarget: 80, Knob: "low", Color: "#22c55e", CueMessage: "Spin light"},
				{Name: "Final Mountain Surge", Type: "interval", DurationSec: 180, CadenceLow: 65, CadenceHigh: 75, CadenceTarget: 70, Knob: "hard", Color: "#ef4444", CueMessage: "Summit push!"},
				{Name: "Cooldown Spin", Type: "cooldown", DurationSec: 180, CadenceLow: 65, CadenceHigh: 75, CadenceTarget: 70, Knob: "low", Color: "#38bdf8", CueMessage: "Cool down and relax"},
			},
		},
		{
			ID:          "tabata",
			Name:        "15-Min Tabata Fury",
			Description: "Classic 20s maximum power / 10s rest intervals",
			Category:    "HIIT",
			Steps: []WorkoutStep{
				{Name: "Warmup Spin", Type: "warmup", DurationSec: 180, CadenceLow: 75, CadenceHigh: 85, CadenceTarget: 80, Knob: "low", Color: "#38bdf8", CueMessage: "Get warm and ready"},
				{Name: "Tabata 1 (20s ON)", Type: "interval", DurationSec: 20, CadenceLow: 105, CadenceHigh: 125, CadenceTarget: 110, Knob: "hard", Color: "#ef4444", CueMessage: "ALL OUT!"},
				{Name: "Rest (10s OFF)", Type: "recovery", DurationSec: 10, CadenceLow: 60, CadenceHigh: 75, CadenceTarget: 70, Knob: "low", Color: "#22c55e", CueMessage: "Rest"},
				{Name: "Tabata 2 (20s ON)", Type: "interval", DurationSec: 20, CadenceLow: 105, CadenceHigh: 125, CadenceTarget: 110, Knob: "hard", Color: "#ef4444", CueMessage: "GO GO GO!"},
				{Name: "Rest (10s OFF)", Type: "recovery", DurationSec: 10, CadenceLow: 60, CadenceHigh: 75, CadenceTarget: 70, Knob: "low", Color: "#22c55e", CueMessage: "Rest"},
				{Name: "Tabata 3 (20s ON)", Type: "interval", DurationSec: 20, CadenceLow: 105, CadenceHigh: 125, CadenceTarget: 110, Knob: "hard", Color: "#ef4444", CueMessage: "PUMP THE LEGS!"},
				{Name: "Rest (10s OFF)", Type: "recovery", DurationSec: 10, CadenceLow: 60, CadenceHigh: 75, CadenceTarget: 70, Knob: "low", Color: "#22c55e", CueMessage: "Rest"},
				{Name: "Tabata 4 (20s ON)", Type: "interval", DurationSec: 20, CadenceLow: 105, CadenceHigh: 125, CadenceTarget: 110, Knob: "hard", Color: "#ef4444", CueMessage: "HALFWAY!"},
				{Name: "Rest (10s OFF)", Type: "recovery", DurationSec: 10, CadenceLow: 60, CadenceHigh: 75, CadenceTarget: 70, Knob: "low", Color: "#22c55e", CueMessage: "Rest"},
				{Name: "Tabata 5 (20s ON)", Type: "interval", DurationSec: 20, CadenceLow: 105, CadenceHigh: 125, CadenceTarget: 110, Knob: "hard", Color: "#ef4444", CueMessage: "ATTACK!"},
				{Name: "Rest (10s OFF)", Type: "recovery", DurationSec: 10, CadenceLow: 60, CadenceHigh: 75, CadenceTarget: 70, Knob: "low", Color: "#22c55e", CueMessage: "Rest"},
				{Name: "Tabata 6 (20s ON)", Type: "interval", DurationSec: 20, CadenceLow: 105, CadenceHigh: 125, CadenceTarget: 110, Knob: "hard", Color: "#ef4444", CueMessage: "DON'T LET UP!"},
				{Name: "Rest (10s OFF)", Type: "recovery", DurationSec: 10, CadenceLow: 60, CadenceHigh: 75, CadenceTarget: 70, Knob: "low", Color: "#22c55e", CueMessage: "Rest"},
				{Name: "Tabata 7 (20s ON)", Type: "interval", DurationSec: 20, CadenceLow: 105, CadenceHigh: 125, CadenceTarget: 110, Knob: "hard", Color: "#ef4444", CueMessage: "ALMOST DONE!"},
				{Name: "Rest (10s OFF)", Type: "recovery", DurationSec: 10, CadenceLow: 60, CadenceHigh: 75, CadenceTarget: 70, Knob: "low", Color: "#22c55e", CueMessage: "Rest"},
				{Name: "Tabata 8 (20s FINAL ON)", Type: "interval", DurationSec: 20, CadenceLow: 110, CadenceHigh: 130, CadenceTarget: 115, Knob: "hard", Color: "#ef4444", CueMessage: "EMPTY THE TANK!"},
				{Name: "Cooldown Spin", Type: "cooldown", DurationSec: 260, CadenceLow: 65, CadenceHigh: 75, CadenceTarget: 70, Knob: "low", Color: "#38bdf8", CueMessage: "Well earned cool down"},
			},
		},
		{
			ID:          "open",
			Name:        "Open Spin Session",
			Description: "Free ride without interval structure",
			Category:    "Free",
			Steps: []WorkoutStep{
				{Name: "Open Ride", Type: "freeride", DurationSec: 7200, CadenceLow: 60, CadenceHigh: 110, CadenceTarget: 80, Knob: "low", Color: "#38bdf8", CueMessage: "Ride at your own pace"},
			},
		},
	}
}
