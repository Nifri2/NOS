package cmd

import (
	"fmt"
	"machine"
	"math/rand"
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
	// Startup banner 3x with 200ms gaps (matches worker pattern)
	for i := 0; i < 3; i++ {
		println("========================================")
		fmt.Printf("RunDispatcher ENTERED - addr=%d\n", int(config.Address))
		println("UART: 38400 baud, TX=GP0, RX=GP1")
		println("========================================")
		time.Sleep(200 * time.Millisecond)
	}

	println("Starting Dispatcher Loop")

	// Configure Watchdog (5s timeout)
	println("Configuring watchdog (5s timeout)...")
	machine.Watchdog.Configure(machine.WatchdogConfig{TimeoutMillis: 5000})
	println("Watchdog configured")

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

	// UART Writer Goroutine with recoverable supervisor
	go func() {
		for {
			func() {
				defer func() {
					if r := recover(); r != nil {
						fmt.Printf("[UART WRITER PANIC] %v\n", r)
						time.Sleep(time.Second)
					}
				}()
				for packet := range uartChan {
					uart.Write(packet[:])
					txWritten++
					if debugLog {
						println("TX -> Addr:", packet[1], "Cmd:", packet[2])
					}
				}
			}()
			println("[UART WRITER] Restarting...")
		}
	}()

	// Radio handling goroutine with recoverable supervisor
	go runRadioLogic(radioChan)

	// Current State
	var currentMode byte = 0x00

	// Helper to send UART packet via channel
	sendPacket := func(addr Address, cmd Command, eye, mouth AnimationID) {
		header := byte(0xAA)
		a := byte(addr)
		c := byte(cmd)
		e := byte(eye)
		m := byte(mouth)
		checksum := Crc8([]byte{a, c, e, m})

		select {
		case uartChan <- [6]byte{header, a, c, e, m, checksum}:
			txQueued++
		default:
			txDropped++
			println("[WARN] UART Queue Full!")
		}
	}

	// Keepalive Goroutine - broadcast to all workers
	go func() {
		for {
			time.Sleep(5 * time.Second)
			// Single broadcast packet reaches all workers
			sendPacket(Address_All, Cmd_NoOp, Anim_EyeIdle, Anim_MouthIdle)
		}
	}()

	// Helper for interruptible sleep
	// Returns (newMode, true) if interrupted
	// Returns (0, false) if completed
	sleepWithInterrupt := func(ms int) (byte, bool) {
		// Check immediately before sleeping
		select {
		case code := <-radioChan:
			radioEvents++
			return code, true
		default:
		}

		// Sleep
		select {
		case code := <-radioChan:
			radioEvents++
			return code, true
		case <-time.After(time.Duration(ms) * time.Millisecond):
			return 0, false
		}
	}

	// Initialize RNG
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	// 1Hz LED heartbeat and 10s heartbeat log
	lastLedToggle := time.Now()
	lastHeartbeat := time.Now()
	ledState := false

	println("Entering main dispatcher loop...")

	// TODO: Send Cmd_Ping to Address_All on a chosen radio input for liveness check

	for {
		now := time.Now()

		// 1Hz LED heartbeat - proves loop is running
		if time.Since(lastLedToggle) >= time.Second {
			ledState = !ledState
			if ledState {
				led.High()
			} else {
				led.Low()
			}
			lastLedToggle = now
		}

		// Heartbeat log every 10 seconds with counters
		if time.Since(lastHeartbeat) >= 10*time.Second {
			fmt.Printf("[HB DISP] txQueued=%d txWritten=%d txDropped=%d radio=%d mode=0x%02X\n",
				txQueued, txWritten, txDropped, radioEvents, currentMode)
			machine.Watchdog.Update()
			lastHeartbeat = now
		}

		machine.Watchdog.Update()

		switch currentMode {
		case 0x00: // Standard Idle (Workers 0 & 1)
			workers := []Address{Worker_0, Worker_1}

			// 1. Random Sleep
			sleepTime := 3000 + r.Intn(10)
			if m, changed := sleepWithInterrupt(sleepTime); changed {
				currentMode = m
				println("Mode Change ->", m)
				continue
			}

			// 2. Blink
			for _, w := range workers {
				sendPacket(w, Cmd_DisplayAnim, Anim_EyeBlink, Anim_MouthIdle)
			}

			// 3. Blink Duration
			if m, changed := sleepWithInterrupt(200); changed {
				currentMode = m
				println("Mode Change ->", m)
				continue
			}

			// 4. Return to Idle
			for _, w := range workers {
				sendPacket(w, Cmd_DisplayAnim, Anim_EyeIdle, Anim_MouthIdle)
			}

		case 0x10: // Insignia Spinny (Worker 2)
			sendPacket(Worker_2, Cmd_DisplayAnim, Anim_EyeIdle, Anim_MouthIdle)

			// Just wait and check for interrupt
			if m, changed := sleepWithInterrupt(500); changed {
				currentMode = m
				println("Mode Change ->", m)
				continue
			}

		default:
			println("Unknown Mode, resetting to 0x00")
			currentMode = 0x00
		}
	}
}

// runRadioLogic is a recoverable supervisor for radio input handling
func runRadioLogic(out chan byte) {
	for {
		func() {
			defer func() {
				if r := recover(); r != nil {
					fmt.Printf("[RADIO PANIC] %v\n", r)
					time.Sleep(time.Second)
				}
			}()
			runRadioLogicLoop(out)
		}()
		println("[RADIO] Restarting radio logic...")
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
			result = byte(4 + (p1 << 2) | p2)
		} else {
			result = byte(p1)
		}

		// Non-blocking send if possible
		select {
		case out <- result:
			if debugLog {
				println("Radio ->", result)
			}
		default:
			println("[WARN] Radio channel full, dropping input")
		}
	}
}
