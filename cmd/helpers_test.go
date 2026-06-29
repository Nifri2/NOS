package cmd

import "testing"

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
		{3, 3, 0x13, "D+D (day/night toggle)"},
		{0, 3, 0x07, "A+D"},
		{3, 0, 0x10, "D+A"},
	}
	for _, c := range cases {
		got := EncodeRadioPress(c.p1, c.p2)
		if got != c.want {
			t.Errorf("%s: EncodeRadioPress(%d,%d) = 0x%02X, want 0x%02X", c.label, c.p1, c.p2, got, c.want)
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

func TestCrc8Bytes4_MatchesCrc8(t *testing.T) {
	// Hot-path zero-allocation form must agree with the generic Crc8 for any 4 bytes.
	cases := [][4]byte{
		{0x01, 0x03, 0x00, 0x10},
		{0xFF, 0x03, byte(Anim_EyeStare), byte(Anim_MouthStare)},
		{0xFF, 0x08, 74, 82},
		{0x00, 0x00, 0x00, 0x00},
	}
	for _, c := range cases {
		want := Crc8(c[:])
		got := Crc8Bytes4(c[0], c[1], c[2], c[3])
		if got != want {
			t.Errorf("Crc8Bytes4(%v) = 0x%02X, Crc8 = 0x%02X", c, got, want)
		}
	}
}

func TestParseRole(t *testing.T) {
	cases := map[string]Role{
		"worker":     Worker,
		"hud":        HUD,
		"dispatcher": Dispatcher,
		"":           Dispatcher, // default
		"bogus":      Dispatcher, // unknown defaults to dispatcher
	}
	for in, want := range cases {
		if got := ParseRole(in); got != want {
			t.Errorf("ParseRole(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestParseAddress(t *testing.T) {
	cases := map[string]Address{
		"worker-0": Worker_0,
		"worker-1": Worker_1,
		"worker-2": Worker_2,
		"worker-3": Worker_3,
		"dispatch": Dispatch,
		"":         Dispatch,
		"bogus":    Dispatch,
	}
	for in, want := range cases {
		if got := ParseAddress(in); got != want {
			t.Errorf("ParseAddress(%q) = %v, want %v", in, got, want)
		}
	}
}
