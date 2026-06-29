//go:build tinygo

package cmd

import (
	"image/color"
	"machine"
)

// chunkedOLED is a minimal SSD1306 framebuffer writer that flushes the display
// over I2C in small pieces.
//
// Why this exists: the stock tinygo ssd1306 driver's flush() pushes the whole
// ~1KB framebuffer in a single I2C transaction. tinygo's rp2 I2C driver has a
// hardcoded 4ms per-transaction deadline (machine_rp2_i2c.go, timeout_us=4000).
// A 1KB transfer at 100kHz takes ~90ms, so it blows the deadline, aborts
// mid-transfer, and wedges the peripheral — every later Tx then fails with
// "i2c: peripheral timeout in disable". Splitting the buffer into small writes
// keeps every transaction far inside the timeout, so the flush actually lands.
//
// Panel init is still handled by the stock ssd1306 driver: Configure() only
// issues short command writes, which work fine. This type only takes over the
// pixel buffer and the flush. The buffer layout (page-major, byte = column,
// bit = row within an 8px page) matches the stock driver exactly, so the panel
// orientation set up by Configure() is preserved.
type chunkedOLED struct {
	i2c  *machine.I2C
	addr uint16
	buf  [1024]byte // 128 * 64 / 8
}

func newChunkedOLED(i2c *machine.I2C, addr uint16) *chunkedOLED {
	return &chunkedOLED{i2c: i2c, addr: addr}
}

// Size satisfies the tinyfont/drivers Displayer interface.
func (o *chunkedOLED) Size() (int16, int16) { return 128, 64 }

// SetPixel mirrors the stock ssd1306 driver's layout: byte index
// x + (y/8)*width, bit (y%8). Any non-black colour lights the pixel.
func (o *chunkedOLED) SetPixel(x, y int16, c color.RGBA) {
	if x < 0 || x >= 128 || y < 0 || y >= 64 {
		return
	}
	idx := x + (y/8)*128
	bit := uint8(1) << uint(y%8)
	if c.R != 0 || c.G != 0 || c.B != 0 {
		o.buf[idx] |= bit
	} else {
		o.buf[idx] &^= bit
	}
}

// ClearBuffer zeroes the framebuffer (does not touch the panel until Display).
func (o *chunkedOLED) ClearBuffer() {
	for i := range o.buf {
		o.buf[i] = 0
	}
}

// cmd sends a single command byte (0x00 control prefix).
func (o *chunkedOLED) cmd(c uint8) error {
	return o.i2c.Tx(o.addr, []byte{0x00, c}, nil)
}

// Display pushes the whole framebuffer to the panel in small chunks. Assumes
// horizontal addressing mode, which the stock driver's Configure() sets.
func (o *chunkedOLED) Display() error {
	// Reset the GDDRAM window to the full panel. In horizontal addressing mode
	// the internal pointer auto-increments and wraps, so the chunked data
	// writes below stay contiguous across separate transactions.
	if err := o.cmd(0x21); err != nil { // SET COLUMN ADDRESS
		return err
	}
	o.cmd(0)
	o.cmd(127)
	if err := o.cmd(0x22); err != nil { // SET PAGE ADDRESS
		return err
	}
	o.cmd(0)
	o.cmd(7)

	// 16 data bytes per transaction -> 17-byte Tx -> ~1.5ms at 100kHz, well
	// inside the rp2 driver's 4ms deadline.
	const chunk = 16
	var frame [1 + chunk]byte
	frame[0] = 0x40 // data mode
	for i := 0; i < len(o.buf); i += chunk {
		end := i + chunk
		if end > len(o.buf) {
			end = len(o.buf)
		}
		n := copy(frame[1:], o.buf[i:end])
		if err := o.i2c.Tx(o.addr, frame[:1+n], nil); err != nil {
			return err
		}
	}
	return nil
}
