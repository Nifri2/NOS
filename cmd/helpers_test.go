package cmd

import "testing"

func radioEncode(p1, p2 int) byte {
	if p2 == -1 {
		return byte(p1)
	}
	return byte(4 + ((p1 << 2) | p2))
}

func TestRadioEncoding(t *testing.T) {
	cases := []struct {
		p1, p2 int
		want   byte
		label  string
	}{
		{0, -1, 0x00, "A single"},
		{1, -1, 0x01, "B single"},
		{2, -1, 0x02, "C single"},
		{3, -1, 0x03, "D single"},
		{0, 0, 0x04, "A+A (mouth idle)"},
		{0, 1, 0x05, "A+B (mouth talk)"},
		{1, 0, 0x08, "B+A (eye idle)"},
		{1, 1, 0x09, "B+B (eye happy)"},
		{3, 3, 0x13, "D+D (reboot)"},
		{0, 3, 0x07, "A+D"},
		{3, 0, 0x10, "D+A"},
	}
	for _, c := range cases {
		got := radioEncode(c.p1, c.p2)
		if got != c.want {
			t.Errorf("%s: radioEncode(%d,%d) = 0x%02X, want 0x%02X", c.label, c.p1, c.p2, got, c.want)
		}
	}
}

func TestCrc8(t *testing.T) {
	// Reference test: crc8([]byte{0x01, 0x03, 0x00, 0x10}) = 0x12
	input := []byte{0x01, 0x03, 0x00, 0x10}
	expected := byte(0x12)
	result := Crc8(input)

	if result != expected {
		t.Errorf("Crc8(%v) = 0x%02X, expected 0x%02X", input, result, expected)
	}
}
