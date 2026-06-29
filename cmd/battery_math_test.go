package cmd

import (
	"math"
	"testing"
)

func approxEqual(a, b, tol float32) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d <= tol
}

func TestAdcAvgToPackVolts(t *testing.T) {
	// Hand-derived expectations: V_node = avg/65535 * 3.3, V_pack = V_node *
	// 3.12766 * batteryCalibration (the empirical gain trim).
	cases := []struct {
		avg  float32
		want float32 // pack volts
	}{
		{0, 0},
		{65535, batteryRefVolts * batteryDivider * batteryCalibration},                              // full ADC range (impossible in practice; tests the math)
		{53400, 2.689 * batteryDivider * batteryCalibration},                                        // ~full 2S pack (8.4V on node math)
		{batteryEmptyV / batteryCalibration / batteryDivider / batteryRefVolts * 65535, batteryEmptyV}, // empty pack reading
	}
	for _, c := range cases {
		got := adcAvgToPackVolts(c.avg)
		if !approxEqual(got, c.want, 0.05) {
			t.Errorf("adcAvgToPackVolts(%.0f) = %.4fV, want ~%.4fV", c.avg, got, c.want)
		}
	}
}

func TestPackVoltsToPercent_Endpoints(t *testing.T) {
	if pct := packVoltsToPercent(batteryFullV); pct != 100 {
		t.Errorf("full pack (%.1fV) = %d%%, want 100%%", batteryFullV, pct)
	}
	if pct := packVoltsToPercent(batteryEmptyV); pct != 0 {
		t.Errorf("empty pack (%.1fV) = %d%%, want 0%%", batteryEmptyV, pct)
	}
}

func TestPackVoltsToPercent_Midpoint(t *testing.T) {
	var mid float32 = (batteryFullV + batteryEmptyV) / 2 // 7.5V
	pct := packVoltsToPercent(mid)
	if pct < 49 || pct > 51 {
		t.Errorf("midpoint pack (%.1fV) = %d%%, want ~50%%", mid, pct)
	}
}

func TestPackVoltsToPercent_Clamping(t *testing.T) {
	// Below empty: clamped to 0.
	if pct := packVoltsToPercent(batteryEmptyV - 1.0); pct != 0 {
		t.Errorf("below empty = %d%%, want 0%%", pct)
	}
	// Above full: clamped to 100.
	if pct := packVoltsToPercent(batteryFullV + 1.0); pct != 100 {
		t.Errorf("above full = %d%%, want 100%%", pct)
	}
	// NaN-safe path: int conversion of NaN is implementation-defined, but the
	// clamp guards must still produce 0..100. We don't actually pass NaN here;
	// just sanity-check a far-negative reading hits 0.
	if pct := packVoltsToPercent(float32(math.Inf(-1))); pct != 0 {
		t.Errorf("-Inf reading = %d%%, want 0%%", pct)
	}
}
