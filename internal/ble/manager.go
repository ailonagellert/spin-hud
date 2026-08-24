package ble

import (
	"time"

	"spin-hud/internal/session"
)

// SensorManager is the seam between the app and the BLE stack. The real
// implementation (tinygo-org/bluetooth, WinRT backend) lands in milestone 3;
// unit tests use mocks against this interface.
type SensorManager interface {
	// ConnectLoop scans, connects, subscribes, and auto-reconnects until stopped.
	ConnectLoop(state *session.State)
}

// ConnectLoop runs the default sensor manager.
func ConnectLoop(state *session.State) {
	newManager().ConnectLoop(state)
}

// Scan scans for nearby BLE sensors for the given duration.
func Scan(d time.Duration) {
	newManager().Scan(d)
}

// SelfCheck runs parser & engine validation (mirrors --self-check).
func SelfCheck() int { return selfCheck() }
