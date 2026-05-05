package cmd

import "testing"

func TestCrc8(t *testing.T) {
	// Reference test: crc8([]byte{0x01, 0x03, 0x00, 0x10}) = 0x12
	input := []byte{0x01, 0x03, 0x00, 0x10}
	expected := byte(0x12)
	result := Crc8(input)

	if result != expected {
		t.Errorf("Crc8(%v) = 0x%02X, expected 0x%02X", input, result, expected)
	}
}
