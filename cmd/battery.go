//go:build tinygo

package cmd

import "machine"

// Battery sensing lives on whichever board owns the battery rail (the
// dispatcher). Pack voltage is read through a resistor divider into GP26 (ADC0):
//
//	R1 = 10k (battery side)   R2 = 4.7k (ground side)
//	V_node = V_pack * R2 / (R1 + R2)  ->  V_pack = V_node * (R1 + R2) / R2
//
// A full 2S pack (8.4V) sits at ~2.69V on the ADC node, safely under the 3.3V ref.
const (
	batteryDivider  = 3.12766 // (10k + 4.7k) / 4.7k
	batteryRefVolts = 3.3
	batteryFullV    = 8.4
	batteryEmptyV   = 6.6
	batterySamples  = 10
)

var batteryADC machine.ADC

// InitBattery brings up the ADC. The RP2350 ADC is DISABLED at boot: machine.InitADC()
// MUST run before the first Get(), or Get() hangs forever polling ADC_CS.READY.
func InitBattery() {
	machine.InitADC()
	batteryADC = machine.ADC{Pin: machine.GP26}
	batteryADC.Configure(machine.ADCConfig{})
}

// ReadBattery averages a handful of samples and returns (pack volts, percent 0-100).
// float32 math is fine here: the M33 has an FPU and this runs off the hot path.
func ReadBattery() (float32, int) {
	var sum uint32
	for i := 0; i < batterySamples; i++ {
		sum += uint32(batteryADC.Get())
	}
	avg := float32(sum) / float32(batterySamples)

	// ADC.Get() returns a 16-bit value (0..65535) regardless of native ADC width.
	nodeV := avg / 65535.0 * batteryRefVolts
	packV := nodeV * batteryDivider
	pct := int((packV - batteryEmptyV) / (batteryFullV - batteryEmptyV) * 100)
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	return packV, pct
}
