//go:build tinygo

package cmd

import "device/arm"

// HardReset performs an immediate system reset via the Cortex-M AIRCR register.
// machine.CPUReset() and machine.Watchdog both silently no-op on RP2350 in current
// TinyGo, so we write the register directly instead.
func HardReset() {
	// VECTKEY (0x05FA) | SYSRESETREQ (bit 2)
	arm.SCB.AIRCR.Set(0x05FA0004)
	for {} // chip resets before this loops
}
