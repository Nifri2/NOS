package cmd

// Battery telemetry math, split out from battery.go so unit tests can cover it
// without an ADC. battery.go (tinygo-tagged) reads samples and calls these.

// Resistor divider on the dispatcher's battery rail. Pack+ -> R1 -> node -> R2
// -> GND, node tied to the ADC pin. V_node = V_pack * R2 / (R1 + R2), so
// V_pack = V_node * (R1 + R2) / R2. A full 2S pack (8.4V) sits at ~2.69V on the
// ADC node, safely under the 3.3V reference.
const (
	batteryDivider  = 3.12766 // (10k + 4.7k) / 4.7k
	batteryRefVolts = 3.3
	batteryFullV    = 8.4
	batteryEmptyV   = 6.6
	batterySamples  = 10

	// batteryCalibration trims the systematic gain error between the ideal
	// divider math and the real hardware: resistor tolerance plus the RP2350
	// ADC reference not being exactly 3.3V. Single-point bench trim — a 7.40V
	// supply read 6.50V, so true = measured * 7.40/6.50. Re-measure and adjust
	// if you swap the divider resistors or move to a different board.
	batteryCalibration = 1.1385 // 7.40 / 6.50
)

// adcAvgToPackVolts converts an averaged ADC reading (0..65535, machine.ADC
// returns 16-bit regardless of native width) to pack voltage at the battery.
func adcAvgToPackVolts(avg float32) float32 {
	nodeV := avg / 65535.0 * batteryRefVolts
	return nodeV * batteryDivider * batteryCalibration
}

// packVoltsToPercent maps a pack voltage to a 0..100 charge percent using a
// linear approximation between batteryEmptyV and batteryFullV. Rounds to the
// nearest integer (otherwise float32 precision makes a full pack read 99%).
// Clamps to the edges so callers can render the result without an extra check.
func packVoltsToPercent(packV float32) int {
	pct := int((packV-batteryEmptyV)/(batteryFullV-batteryEmptyV)*100 + 0.5)
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	return pct
}
