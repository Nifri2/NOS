//go:build tinygo

package cmd

import (
	"fmt"
	"image/color"
	"machine"
	"strconv"
	"time"

	pio "github.com/tinygo-org/pio/rp2-pio"
	"github.com/tinygo-org/pio/rp2-pio/piolib"
	"tinygo.org/x/drivers/ssd1306"
	"tinygo.org/x/tinyfont"
	"tinygo.org/x/tinyfont/proggy"
)

// Insignia_Pin_0 / Insignia_Pin_1 are the two WS2812 data lines for the static
// insignia panels the HUD board drives once at boot. The HUD board shares the
// worker PCB layout, so the same GP18/GP12 strip pins are wired through; both
// panels display the same frame. Change here if your wiring differs.
const (
	Insignia_Pin_0 = machine.GP18
	Insignia_Pin_1 = machine.GP12
)

// The HUD board is a PASSIVE bus sniffer. Because the UART bus is shared, it sees
// every packet the dispatcher broadcasts. Unlike a worker it does NOT filter by
// address and drives no LED strips; it reconstructs global state purely from the
// traffic it observes and renders it to a 128x64 SSD1306 OLED.
//
// Display panel: two-colour SSD1306 @ I2C 0x3C. Rows 0-15 are yellow (headline),
// rows 16-63 are blue. There is a ~2px dead gap between the regions.

// hudState, displayChanged, and hudApplyPacket live in hud_state.go (pure Go).
// Keeping them out of this tinygo-tagged file lets unit tests cover the packet
// reducer without a board.

func RunHUD(config Settings, uart *machine.UART, led machine.Pin) {
	// USB-CDC warmup so the boot banner has a chance to land on a monitor.
	time.Sleep(2 * time.Second)
	fmt.Printf("[%d] BOOT main: role=hud addr=%d\n", Ts(), int(config.Address))

	// Match the workers' 5s watchdog; the 1Hz LED heartbeat below is the primary feed.
	machine.Watchdog.Configure(machine.WatchdogConfig{TimeoutMillis: 5000})

	// I2C0 on GP4 (SDA) / GP5 (SCL): clear of GP0/GP1 (UART).
	machine.I2C0.Configure(machine.I2CConfig{
		Frequency: 100000, // 100 kHz — tolerant of weak pull-ups / hand-wired runs
		SDA:       machine.GP4,
		SCL:       machine.GP5,
	})

	// Bus scan: probe every 7-bit address and log which ones ACK. A healthy
	// SSD1306 answers at 0x3C (or 0x3D); silence means dead panel or bad wiring.
	// Diagnostic only — remove once the display is confirmed working.
	for a := uint8(0x08); a < 0x78; a++ {
		if err := machine.I2C0.Tx(uint16(a), []byte{0x00}, nil); err == nil {
			fmt.Printf("[%d] [HUD] I2C device ACK at 0x%02X\n", Ts(), a)
		}
	}
	fmt.Printf("[%d] [HUD] I2C scan complete\n", Ts())

	// Init via the stock driver (short command writes only — these work). The
	// stock driver's framebuffer flush does NOT work on rp2350 (single 1KB Tx
	// blows the I2C driver's 4ms deadline and wedges the peripheral), so all
	// pixel pushes go through chunkedOLED instead. See oled.go.
	display := ssd1306.NewI2C(machine.I2C0)
	display.Configure(ssd1306.Config{
		Address: 0x3C,
		Width:   128,
		Height:  64,
	})
	oled := newChunkedOLED(machine.I2C0, 0x3C)
	oled.ClearBuffer()
	if err := oled.Display(); err != nil {
		fmt.Printf("[%d] [HUD] initial clear flush err=%v\n", Ts(), err)
	}
	fmt.Printf("[%d] [HUD] SSD1306 configured on I2C0 GP4/GP5 @0x3C\n", Ts())

	// Insignia: one static frame written to the WS2812 strip on Insignia_Pin.
	// The HUD board replaces the planned Worker_2 insignia firmware, so it still
	// owns this strip. There's no animation loop, brightness scaling, or response
	// to bus traffic; the frame is the .anim's compiled brightness, written once.
	displayInsigniaOnce()

	on := color.RGBA{R: 255, G: 255, B: 255, A: 255}

	st := hudState{eyeAnim: Anim_EyeIdle, mouthAnim: Anim_MouthIdle}
	var lastRendered hudState
	firstDraw := true
	lastDrawMs := Ts()
	const minRedrawMs = 250 // cap redraw to ~4Hz; a full framebuffer I2C push is slow

	const (
		HeaderByte = 0xAA
		PacketSize = 6
	)
	buf := make([]byte, PacketSize)
	bufIdx := 0
	lastByteTime := time.Now()

	lastLedToggle := time.Now()
	ledState := false

	drawCount := 0 // diagnostic: count draws + log first few flush results

	fmt.Printf("[%d] [HUD] entering sniff loop\n", Ts())
	for {
		now := time.Now()

		// 1Hz LED heartbeat doubles as the watchdog feed.
		if time.Since(lastLedToggle) >= time.Second {
			ledState = !ledState
			if ledState {
				led.High()
			} else {
				led.Low()
			}
			lastLedToggle = now
			machine.Watchdog.Update()
		}

		// Inter-byte timeout: drop a partial frame if the bus goes quiet mid-packet.
		if bufIdx > 0 && time.Since(lastByteTime) > 20*time.Millisecond {
			bufIdx = 0
		}

		if uart.Buffered() > 0 {
			b, _ := uart.ReadByte()
			lastByteTime = now
			if bufIdx == 0 {
				if b == HeaderByte {
					buf[0] = b
					bufIdx++
				}
			} else {
				buf[bufIdx] = b
				bufIdx++
				if bufIdx == PacketSize {
					// buf = [AA, Addr, Cmd, Eye, Mouth, Checksum]
					// hudApplyPacket validates CRC and updates st. No address
					// filtering: the HUD acts on every valid packet.
					hudApplyPacket(&st, buf[1], buf[2], buf[3], buf[4], buf[5])
					bufIdx = 0
				}
			}
		} else {
			// Bus is idle: a safe moment to push the (slow) display without
			// starving the RX path. Only redraw on a real change, throttled.
			if (firstDraw || displayChanged(&st, &lastRendered)) && Ts()-lastDrawMs >= minRedrawMs {
				derr := drawHUD(oled, on, &st)
				drawCount++
				if drawCount <= 8 {
					fmt.Printf("[%d] [HUD] draw #%d err=%v\n", Ts(), drawCount, derr)
				}
				lastRendered = st
				lastDrawMs = Ts()
				firstDraw = false
			}
			time.Sleep(time.Millisecond)
		}
	}
}

// drawHUD renders the current state to the OLED. Headline goes in the yellow
// region (rows 0-15); status lines fill the blue region (rows 16-63).
func drawHUD(display *chunkedOLED, c color.RGBA, st *hudState) error {
	font := &proggy.TinySZ8pt7b

	display.ClearBuffer()

	// Yellow headline row.
	mode := "NIGHT"
	if st.dayMode {
		mode = "DAY"
	}
	tinyfont.WriteLine(display, font, 0, 12, "NOS   "+mode, c)

	// Blue status rows.
	tinyfont.WriteLine(display, font, 0, 28, "Eye: "+animName(st.eyeAnim), c)
	tinyfont.WriteLine(display, font, 0, 40, "Mth: "+animName(st.mouthAnim), c)
	tinyfont.WriteLine(display, font, 0, 52, "Set: "+expressionSet(st.eyeAnim), c)
	tinyfont.WriteLine(display, font, 0, 63,
		"Bat: "+formatVolts(st.battDeci)+" "+strconv.Itoa(int(st.battPct))+"%", c)

	return display.Display()
}

// displayInsigniaOnce locates the "insignia" animation loaded by main.go,
// converts its first frame to the PIO's GRB-packed uint32 format, and writes
// it to both WS2812 strips (Insignia_Pin_0 and Insignia_Pin_1). The two panels
// share the same frame. Per-strip failures are logged but non-fatal: the HUD's
// primary job is the OLED, and the sniff loop must still come up.
func displayInsigniaOnce() {
	var anim *Animation
	for _, a := range LoadedAnimations {
		if a.Name == "insignia" {
			anim = a
			break
		}
	}
	if anim == nil || len(anim.Frames) == 0 {
		fmt.Printf("[%d] [HUD] insignia animation not loaded, skipping LED draw\n", Ts())
		return
	}

	frame := anim.Frames[0]
	pixels := len(frame) / 3
	buf := make([]uint32, pixels)
	for i := 0; i < pixels; i++ {
		g := uint32(frame[i*3])
		r := uint32(frame[i*3+1])
		b := uint32(frame[i*3+2])
		buf[i] = g<<24 | r<<16 | b<<8
	}

	writeInsigniaStrip(Insignia_Pin_0, buf, pixels)
	writeInsigniaStrip(Insignia_Pin_1, buf, pixels)
}

// writeInsigniaStrip claims a PIO state machine, brings up a WS2812B on `pin`,
// and writes `buf` once. Each strip is independent; a failure here only loses
// that panel, not the other.
func writeInsigniaStrip(pin machine.Pin, buf []uint32, pixels int) {
	sm, err := pio.PIO0.ClaimStateMachine()
	if err != nil {
		fmt.Printf("[%d] [HUD] insignia: claim PIO SM for GP%d failed: %s\n", Ts(), pin, err.Error())
		return
	}
	strip, err := piolib.NewWS2812B(sm, pin)
	if err != nil {
		fmt.Printf("[%d] [HUD] insignia: WS2812B init on GP%d failed: %s\n", Ts(), pin, err.Error())
		return
	}
	strip.EnableDMA(true)
	strip.WriteRaw(buf)
	fmt.Printf("[%d] [HUD] insignia rendered: %d pixels on GP%d\n", Ts(), pixels, pin)
}
