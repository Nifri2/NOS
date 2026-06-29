//go:build tinygo

package cmd

import (
	"fmt"
	"machine"
	"runtime"
	"time"
)

const (
	ProjectedFPS = 47
	Radio_Pin_0  = machine.GP16
	Radio_Pin_1  = machine.GP17
	Radio_Pin_2  = machine.GP18
	Radio_Pin_3  = machine.GP19
)

var Radio_Pins = []machine.Pin{Radio_Pin_0, Radio_Pin_1, Radio_Pin_2, Radio_Pin_3}

// Reworked Protocol to support 2 animation channels (eye + mouth)
// Example: [address, command, animID_eye, animID_mouth]

func RunDispatcher(config Settings, uart *machine.UART, led machine.Pin) {
	// Independent watchdog/diagnostic goroutine - spawned FIRST, before anything else
	// This goroutine proves the scheduler is alive even if other goroutines freeze
	go func() {
		for {
			func() {
				defer func() {
					if r := recover(); r != nil {
						fmt.Printf("[%d] [WD DISP PANIC] %v\n", Ts(), r)
						time.Sleep(time.Second)
					}
				}()
				var tick int
				var ms runtime.MemStats
				for {
					time.Sleep(time.Second)
					tick++
					if tick%5 == 0 {
						fmt.Printf("[%d] [WD DISP] alive\n", Ts())
					}
					if tick%30 == 0 {
						runtime.ReadMemStats(&ms)
						fmt.Printf("[%d] [MEM DISP] alloc=%d totalAlloc=%d sys=%d\n",
							Ts(), ms.Alloc, ms.TotalAlloc, ms.Sys)
					}
				}
			}()
			fmt.Printf("[%d] [WD DISP] Restarting watchdog goroutine...\n", Ts())
		}
	}()

	fmt.Printf("[%d] ========================================\n", Ts())
	fmt.Printf("[%d] RunDispatcher ENTERED - addr=%d\n", Ts(), int(config.Address))
	fmt.Printf("[%d] UART: 38400 baud, TX=GP0, RX=GP1\n", Ts())
	fmt.Printf("[%d] ========================================\n", Ts())

	fmt.Printf("[%d] Starting Dispatcher Loop\n", Ts())

	fmt.Printf("[%d] Configuring watchdog (1.5s timeout)...\n", Ts())
	machine.Watchdog.Configure(machine.WatchdogConfig{TimeoutMillis: 1500})
	fmt.Printf("[%d] [DISP] Watchdog timeout: 1500ms\n", Ts())

	// Radio Channel
	radioChan := make(chan byte, 10)

	// UART Output Channel to ensure atomic packet writes
	uartChan := make(chan [6]byte, 20)

	// TX diagnostic counters
	// txQueued/txDropped owned by sendPacket (main goroutine)
	// txWritten owned by UART writer goroutine
	// radioEvents owned by radio goroutine (incremented via channel read in main loop)
	var txQueued uint32
	var txDropped uint32
	var txWritten uint32
	var radioEvents uint32

	// UART writer health tracking (int64 ms to avoid time.Time allocations)
	lastTxWriteCompletedMs := Ts()

	// Rate-limit drop log to avoid log spam if writer is stuck (int64 ms)
	lastDropLogTimeMs := Ts() - 60000 // Allow first log immediately

	// UART Writer Goroutine with recoverable supervisor
	go func() {
		for {
			func() {
				defer func() {
					if r := recover(); r != nil {
						fmt.Printf("[%d] [UART WRITER PANIC] %v\n", Ts(), r)
						time.Sleep(time.Second)
					}
				}()
				for packet := range uartChan {
					if packet[2] != byte(Cmd_NoOp) {
						fmt.Printf("[%d] [TX] writing cmd=%02X addr=%02X\n", Ts(), packet[2], packet[1])
					}
					uart.Write(packet[:])
					txWritten++
					lastTxWriteCompletedMs = Ts()
				}
			}()
			fmt.Printf("[%d] [UART WRITER] Restarting...\n", Ts())
		}
	}()

	// Radio handling goroutine with recoverable supervisor
	go runRadioLogic(radioChan)

	var eyeMode byte = 0x00 // 0x00=idle_blink, 0x01=happy, 0x02=excited, 0x03=stare, 0x04=flushed

	// Blink state machine (active when eyeMode == 0x00)
	var nextBlinkMs int64 = Ts() + 3000
	var blinkEndMs int64 = 0
	var isBlinking bool = false

	// Day/night brightness mode — night is the boot default
	var isDayMode bool = false

	// Helper to send UART packet via channel (zero allocation in hot path).
	// BuildPacket (protocol.go) owns the wire format so the dispatcher's encoding
	// is the same code the host loopback tests exercise.
	sendPacket := func(addr Address, cmd Command, eye, mouth AnimationID) {
		select {
		case uartChan <- BuildPacket(addr, cmd, eye, mouth):
			txQueued++
		default:
			txDropped++
			// Rate-limit drop log to 1 per second max
			nowMs := Ts()
			if nowMs-lastDropLogTimeMs >= 1000 {
				fmt.Printf("[%d] [WARN] UART Queue Full! dropped=%d\n", nowMs, txDropped)
				lastDropLogTimeMs = nowMs
			}
		}
	}

	// Returns the eye animation matching the current blink state.
	// Used when building combined packets so the eye portion is always correct.
	currentEyeAnim := func() AnimationID {
		switch eyeMode {
		case 0x01:
			return Anim_EyeHappy
		case 0x02:
			return Anim_EyeExcited
		case 0x03:
			return Anim_EyeStare
		case 0x04:
			return Anim_EyeFlushed
		default:
			if isBlinking {
				return Anim_EyeBlink
			}
			return Anim_EyeIdle
		}
	}

	// Keepalive Goroutine - broadcast to all workers
	go func() {
		for {
			time.Sleep(2 * time.Second)
			// Single broadcast packet reaches all workers
			sendPacket(Address_All, Cmd_NoOp, Anim_EyeIdle, Anim_MouthIdle)
		}
	}()

	// Battery telemetry - the dispatcher owns the battery rail, reads the ADC and
	// broadcasts the result so the HUD board can display it. Payload reuses the
	// eye/mouth slots: eye = deci-volts (7.4V -> 74), mouth = percent (0-100).
	InitBattery()
	go func() {
		for {
			time.Sleep(5 * time.Second)
			v, pct := ReadBattery()
			deci := uint8(v*10 + 0.5)
			sendPacket(Address_All, Cmd_Battery, AnimationID(deci), AnimationID(pct))
			if debugLog {
				fmt.Printf("[%d] [BATT] %d.%dV %d%%\n", Ts(), deci/10, deci%10, pct)
			}
		}
	}()

	// 1Hz LED heartbeat and 10s heartbeat log (int64 ms to avoid time.Time allocations)
	lastLedToggleMs := Ts()
	lastHeartbeatMs := Ts()
	ledState := false

	fmt.Printf("[%d] Entering main dispatcher loop...\n", Ts())

	for {
		nowMs := Ts()

		// 1Hz LED heartbeat - proves loop is running
		if nowMs-lastLedToggleMs >= 1000 {
			ledState = !ledState
			if ledState {
				led.High()
			} else {
				led.Low()
			}
			lastLedToggleMs = nowMs
		}

		// Heartbeat log every 10 seconds with counters
		if nowMs-lastHeartbeatMs >= 10000 {
			writerStuckSec := int((nowMs - lastTxWriteCompletedMs) / 1000)
			fmt.Printf("[%d] [HB DISP] txQueued=%d txWritten=%d txDropped=%d radio=%d eye=0x%02X writerStuckSec=%d\n",
				nowMs, txQueued, txWritten, txDropped, radioEvents, eyeMode, writerStuckSec)
			if writerStuckSec > 10 {
				fmt.Printf("[%d] [ALERT] UART writer appears stuck for %d seconds!\n", nowMs, writerStuckSec)
			}
			machine.Watchdog.Update()
			lastHeartbeatMs = nowMs
		}

		machine.Watchdog.Update()

		// Check radio input (non-blocking)
		select {
		case code := <-radioChan:
			radioEvents++
			switch code {
			case 0x04: // A+A: mouth idle
				sendPacket(Address_All, Cmd_DisplayAnim, currentEyeAnim(), Anim_MouthIdle)
				fmt.Printf("[%d] Mouth -> idle\n", Ts())
			case 0x05: // A+B: reserved (was mouth talking via mic)
				if debugLog {
					fmt.Printf("[%d] A+B: reserved\n", Ts())
				}
			case 0x08: // B+A: eye idle blink
				eyeMode = 0x00
				isBlinking = false
				nextBlinkMs = nowMs + 3000
				sendPacket(Address_All, Cmd_DisplayAnim, Anim_EyeIdle, Anim_MouthIdle)
				fmt.Printf("[%d] Eyes -> idle blink\n", Ts())
			case 0x09: // B+B: happy eyes
				eyeMode = 0x01
				isBlinking = false
				sendPacket(Address_All, Cmd_DisplayAnim, Anim_EyeHappy, Anim_MouthIdle)
				fmt.Printf("[%d] Eyes -> happy\n", Ts())
			case 0x0A: // B+C: excited eyes
				eyeMode = 0x02
				isBlinking = false
				sendPacket(Address_All, Cmd_DisplayAnim, Anim_EyeExcited, Anim_MouthIdle)
				fmt.Printf("[%d] Eyes -> excited\n", Ts())
			case 0x0D: // C+B: stare animation set (eye + mouth)
				eyeMode = 0x03
				isBlinking = false
				sendPacket(Address_All, Cmd_DisplayAnim, Anim_EyeStare, Anim_MouthStare)
				fmt.Printf("[%d] Set -> stare\n", Ts())
			case 0x0E: // C+C: flushed animation set (eye + mouth)
				eyeMode = 0x04
				isBlinking = false
				sendPacket(Address_All, Cmd_DisplayAnim, Anim_EyeFlushed, Anim_MouthFlushed)
				fmt.Printf("[%d] Set -> flushed\n", Ts())
			case 0x13: // D+D: toggle day/night brightness mode
				isDayMode = !isDayMode
				if isDayMode {
					sendPacket(Address_All, Cmd_DayMode, 0, 0)
					fmt.Printf("[%d] Mode -> day (+%d%%)\n", Ts(), DayModeBrightnessPercent-100)
				} else {
					sendPacket(Address_All, Cmd_NightMode, 0, 0)
					fmt.Printf("[%d] Mode -> night\n", Ts())
				}
			default:
				if debugLog {
					fmt.Printf("[%d] Unhandled radio code: 0x%02X\n", Ts(), code)
				}
			}
		default:
		}

		// Eye state machine
		switch eyeMode {
		case 0x00: // idle blink
			if isBlinking && nowMs >= blinkEndMs {
				// Blink finished — return to idle
				isBlinking = false
				nextBlinkMs = nowMs + 3000
				sendPacket(Worker_0, Cmd_DisplayAnim, Anim_EyeIdle, Anim_MouthIdle)
				sendPacket(Worker_1, Cmd_DisplayAnim, Anim_EyeIdle, Anim_MouthIdle)
			} else if !isBlinking && nowMs >= nextBlinkMs {
				// Time to blink
				isBlinking = true
				blinkEndMs = nowMs + 200
				sendPacket(Worker_0, Cmd_DisplayAnim, Anim_EyeBlink, Anim_MouthIdle)
				sendPacket(Worker_1, Cmd_DisplayAnim, Anim_EyeBlink, Anim_MouthIdle)
			}
		case 0x01: // happy — static, initial send handled at mode switch
		case 0x02: // excited — static, initial send handled at mode switch
		}

		// ~30Hz tick rate
		time.Sleep(33 * time.Millisecond)
		machine.Watchdog.Update()
	}
}

// runRadioLogic is a recoverable supervisor for radio input handling
func runRadioLogic(out chan byte) {
	for {
		func() {
			defer func() {
				if r := recover(); r != nil {
					fmt.Printf("[%d] [RADIO PANIC] %v\n", Ts(), r)
					time.Sleep(time.Second)
				}
			}()
			runRadioLogicLoop(out)
		}()
		fmt.Printf("[%d] [RADIO] Restarting radio logic...\n", Ts())
	}
}

// runRadioLogicLoop handles the actual radio pin monitoring
func runRadioLogicLoop(out chan byte) {
	for _, pin := range Radio_Pins {
		pin.Configure(machine.PinConfig{Mode: machine.PinInput})
	}

	getPressedPin := func() int {
		for i, pin := range Radio_Pins {
			if pin.Get() {
				return i
			}
		}
		return -1
	}

	for {
		// 1. Wait for first press
		var p1 int = -1
		for {
			p1 = getPressedPin()
			if p1 != -1 {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}

		// Wait for release
		for getPressedPin() != -1 {
			time.Sleep(10 * time.Millisecond)
		}

		// 2. Wait up to 500ms for second press
		var p2 int = -1
		timeout := time.Now().Add(500 * time.Millisecond)
		for time.Now().Before(timeout) {
			p := getPressedPin()
			if p != -1 {
				p2 = p
				break
			}
			time.Sleep(10 * time.Millisecond)
		}

		if p2 != -1 {
			for getPressedPin() != -1 {
				time.Sleep(10 * time.Millisecond)
			}
		}
		result := EncodeRadioPress(p1, p2)

		// Non-blocking send if possible
		select {
		case out <- result:
			if debugLog {
				fmt.Printf("[%d] Radio -> 0x%02X\n", Ts(), result)
			}
		default:
			fmt.Printf("[%d] [WARN] Radio channel full, dropping input\n", Ts())
		}
	}
}
