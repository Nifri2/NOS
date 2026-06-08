//go:build tinygo

package cmd

import "machine"

// Hardware-facing battery sensing. Pure math (constants, conversion functions)
// lives in battery_math.go so it can be unit-tested without an ADC.
//
// Pack voltage is read through a resistor divider into GP26 (ADC0); see
// battery_math.go for the divider math.

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
	packV := adcAvgToPackVolts(avg)
	return packV, packVoltsToPercent(packV)
}
