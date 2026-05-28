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

	// Configure GP26 (ADC0) for MAX9814 microphone input
	// InitADC must be called first — it sets ADC_CS.EN=1; without it
	// adc.Get() polls ADC_CS.READY forever (peripheral disabled at boot).
	machine.InitADC()
	adcPin := machine.GP26
	adcPin.Configure(machine.PinConfig{Mode: machine.PinAnalog})
	adc := machine.ADC{Pin: adcPin}
	adc.Configure(machine.ADCConfig{})
	fmt.Printf("[%d] ADC configured on GP26 (ADC0) for MAX9814 mic\n", Ts())

	fmt.Printf("[%d] ADC test read...\n", Ts())
	testSample := adc.Get()
	fmt.Printf("[%d] ADC test: %d (center ~32768)\n", Ts(), testSample)

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

	// Independent state variables for eye and mouth
	var eyeMode byte = 0x00  // 0x00=idle_blink, 0x01=happy
	var mouthMode byte = 0x00 // 0x00=idle, 0x01=talking

	// Blink state machine (active when eyeMode == 0x00)
	var nextBlinkMs int64 = Ts() + 3000
	var blinkEndMs int64 = 0
	var isBlinking bool = false

	// Helper to send UART packet via channel (zero allocation in hot path)
	sendPacket := func(addr Address, cmd Command, eye, mouth AnimationID) {
		header := byte(0xAA)
		a := byte(addr)
		c := byte(cmd)
		e := byte(eye)
		m := byte(mouth)
		checksum := Crc8Bytes4(a, c, e, m)

		select {
		case uartChan <- [6]byte{header, a, c, e, m, checksum}:
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
		default:
			if isBlinking {
				return Anim_EyeBlink
			}
			return Anim_EyeIdle
		}
	}

	// Returns the mouth animation for eye-only state-change packets.
	// Talking mode handles its own mouth sampling; for all other senders, idle is correct.
	currentMouthAnim := func() AnimationID {
		return Anim_MouthIdle
	}

	// Keepalive Goroutine - broadcast to all workers
	go func() {
		for {
			time.Sleep(2 * time.Second)
			// Single broadcast packet reaches all workers
			sendPacket(Address_All, Cmd_NoOp, Anim_EyeIdle, Anim_MouthIdle)
		}
	}()

	// 1Hz LED heartbeat and 10s heartbeat log (int64 ms to avoid time.Time allocations)
	lastLedToggleMs := Ts()
	lastHeartbeatMs := Ts()
	ledState := false

	var loopCounter int
	var lastMouthAnim AnimationID = 0xFF // invalid sentinel for talking mode state-change log

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
			fmt.Printf("[%d] [HB DISP] txQueued=%d txWritten=%d txDropped=%d radio=%d eye=0x%02X mouth=0x%02X writerStuckSec=%d\n",
				nowMs, txQueued, txWritten, txDropped, radioEvents, eyeMode, mouthMode, writerStuckSec)
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
				mouthMode = 0x00
				sendPacket(Address_All, Cmd_DisplayAnim, currentEyeAnim(), Anim_MouthIdle)
				fmt.Printf("[%d] Mouth -> idle\n", Ts())
			case 0x05: // A+B: mouth talking
				mouthMode = 0x01
				lastMouthAnim = 0xFF // reset state-change log sentinel
				fmt.Printf("[%d] Mouth -> talking\n", Ts())
			case 0x08: // B+A: eye idle blink
				eyeMode = 0x00
				isBlinking = false
				nextBlinkMs = nowMs + 3000
				sendPacket(Address_All, Cmd_DisplayAnim, Anim_EyeIdle, currentMouthAnim())
				fmt.Printf("[%d] Eyes -> idle blink\n", Ts())
			case 0x09: // B+B: happy eyes
				eyeMode = 0x01
				isBlinking = false
				sendPacket(Address_All, Cmd_DisplayAnim, Anim_EyeHappy, currentMouthAnim())
				fmt.Printf("[%d] Eyes -> happy\n", Ts())
			case 0x13: // D+D: reboot
				fmt.Printf("[%d] [REBOOT] D+D\n", Ts())
				sendPacket(Address_All, Cmd_Reboot, Anim_EyeIdle, Anim_MouthIdle)
				time.Sleep(500 * time.Millisecond)
				HardReset()
				for {
					time.Sleep(time.Second)
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
				sendPacket(Worker_0, Cmd_DisplayAnim, Anim_EyeIdle, currentMouthAnim())
				sendPacket(Worker_1, Cmd_DisplayAnim, Anim_EyeIdle, currentMouthAnim())
			} else if !isBlinking && nowMs >= nextBlinkMs {
				// Time to blink
				isBlinking = true
				blinkEndMs = nowMs + 200
				sendPacket(Worker_0, Cmd_DisplayAnim, Anim_EyeBlink, currentMouthAnim())
				sendPacket(Worker_1, Cmd_DisplayAnim, Anim_EyeBlink, currentMouthAnim())
			}
		case 0x01: // happy — static, initial send handled at mode switch
		}

		// Mouth state machine
		switch mouthMode {
		case 0x00: // idle — static, initial send handled at mode switch
		case 0x01: // talking — sample mic and send mouth state
			const adcCenter = 32768 // TinyGo normalises RP2350 12-bit ADC to 16-bit
			var maxDev uint16
			for i := 0; i < 64; i++ {
				sample := adc.Get()
				var dev uint16
				if sample > adcCenter {
					dev = sample - adcCenter
				} else {
					dev = adcCenter - sample
				}
				if dev > maxDev {
					maxDev = dev
				}
			}

			var mouthAnim AnimationID
			switch {
			case maxDev < 1000:
				mouthAnim = Anim_MouthIdle
			case maxDev < 3000:
				mouthAnim = Anim_MouthOpen1
			case maxDev < 6000:
				mouthAnim = Anim_MouthOpen2
			default:
				mouthAnim = Anim_MouthOpen3
			}

			sendPacket(Address_All, Cmd_DisplayAnim, currentEyeAnim(), mouthAnim)

			if mouthAnim != lastMouthAnim {
				fmt.Printf("[%d] [MIC] mouth=0x%02X maxDev=%d\n", Ts(), int(mouthAnim), maxDev)
				lastMouthAnim = mouthAnim
			}
			if debugLog && loopCounter%30 == 0 {
				fmt.Printf("[%d] [MIC dbg] maxDev=%d mouth=0x%02X\n", Ts(), maxDev, int(mouthAnim))
			}
		}

		// ~30Hz tick rate (matches mic update rate, fine for blink timing)
		time.Sleep(33 * time.Millisecond)
		machine.Watchdog.Update()
		loopCounter++
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

		var result byte
		if p2 != -1 {
			for getPressedPin() != -1 {
				time.Sleep(10 * time.Millisecond)
			}
			result = byte(4 + ((p1 << 2) | p2))
		} else {
			result = byte(p1)
		}

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
