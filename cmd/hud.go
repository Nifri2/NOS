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
		Frequency: 400000, // 400 kHz
		SDA:       machine.GP4,
		SCL:       machine.GP5,
	})
	display := ssd1306.NewI2C(machine.I2C0)
	display.Configure(ssd1306.Config{
		Address: 0x3C,
		Width:   128,
		Height:  64,
	})
	display.ClearDisplay()
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
				drawHUD(display, on, &st)
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
func drawHUD(display *ssd1306.Device, c color.RGBA, st *hudState) {
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

	display.Display()
}

// formatVolts turns deci-volts into e.g. "7.4V" without float formatting.
func formatVolts(deci uint8) string {
	return strconv.Itoa(int(deci)/10) + "." + strconv.Itoa(int(deci)%10) + "V"
}

// animName maps an animation ID to a short label for the HUD. The dispatcher
// broadcasts logical IDs; the side-specific variants are included for safety.
func animName(id AnimationID) string {
	switch id {
	case Anim_EyeIdle:
		return "Idle"
	case Anim_EyeBlink:
		return "Blink"
	case Anim_EyeHappy:
		return "Happy"
	case Anim_EyeExcited, Anim_EyeExcitedLeft, Anim_EyeExcitedRight:
		return "Excited"
	case Anim_EyeStare, Anim_MouthStare, Anim_MouthStareLeft, Anim_MouthStareRight:
		return "Stare"
	case Anim_EyeFlushed, Anim_MouthFlushed, Anim_MouthFlushedLeft, Anim_MouthFlushedRight:
		return "Flushed"
	case Anim_MouthIdle, Anim_MouthIdleLeft, Anim_MouthIdleRight:
		return "Idle"
	case Anim_MouthYap1, Anim_MouthYap1Left, Anim_MouthYap1Right:
		return "Yap1"
	case Anim_MouthYap2, Anim_MouthYap2Left, Anim_MouthYap2Right:
		return "Yap2"
	case Anim_MouthYap3, Anim_MouthYap3Left, Anim_MouthYap3Right:
		return "Yap3"
	default:
		return "?"
	}
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

// expressionSet reconstructs which expression "set" is active from the eye anim.
// The HUD can't see the dispatcher's internal eyeMode, so it infers it. Stare and
// Flushed are full eye+mouth sets; Happy/Excited are eye-only modes.
func expressionSet(eye AnimationID) string {
	switch eye {
	case Anim_EyeHappy:
		return "Happy"
	case Anim_EyeExcited, Anim_EyeExcitedLeft, Anim_EyeExcitedRight:
		return "Excited"
	case Anim_EyeStare:
		return "Stare"
	case Anim_EyeFlushed:
		return "Flushed"
	default:
		return "None"
	}
}
