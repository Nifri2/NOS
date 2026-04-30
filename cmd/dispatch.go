package cmd

import (
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
	println("Starting Dispatcher Loop (Simple)")

	// Configure Watchdog (5s timeout)
	machine.Watchdog.Configure(machine.WatchdogConfig{TimeoutMillis: 5000})

	// Radio Channel
	radioChan := make(chan byte, 10)
	
	// UART Output Channel to ensure atomic packet writes
	uartChan := make(chan [6]byte, 20)

	// UART Writer Goroutine
	go func() {
		for packet := range uartChan {
			uart.Write(packet[:])
			// Log TX for debugging
			println("TX -> Addr:", packet[1], "Cmd:", packet[2])
		}
	}()

	// Radio handling goroutine
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
		checksum := a + c + e + m
		
		select {
		case uartChan <- [6]byte{header, a, c, e, m, checksum}:
		default:
			println("UART Queue Full!")
		}
	}

	// Keepalive Goroutine
	go func() {
		allWorkers := []Address{Worker_0, Worker_1, Worker_2, Worker_3}
		for {
			time.Sleep(1 * time.Second)
			for _, w := range allWorkers {
				// Send NoOp as keepalive
				sendPacket(w, Cmd_NoOp, Anim_EyeIdle, Anim_MouthIdle)
			}
		}
	}()

	// Helper for interruptible sleep
	// Returns (newMode, true) if interrupted
	// Returns (0, false) if completed
	sleepWithInterrupt := func(ms int) (byte, bool) {
		// Check immediately before sleeping
		select {
		case code := <-radioChan:
			return code, true
		default:
		}

		// Sleep
		select {
		case code := <-radioChan:
			return code, true
		case <-time.After(time.Duration(ms) * time.Millisecond):
			return 0, false
		}
	}

	// Initialize RNG
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	for {
		machine.Watchdog.Update()

		switch currentMode {
		case 0x00: // Standard Idle (Workers 0 & 1)
			// println("Mode: Standard")
			
			workers := []Address{Worker_0, Worker_1}
			
			// 1. Random Sleep
			sleepTime := 2000 + r.Intn(4001)
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
			// println("Mode: Insignia")
			
			// Ensure animation is set
			sendPacket(Worker_2, Cmd_DisplayAnim, Anim_SpinnyLambda, Anim_MouthIdle)
			
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

func runRadioLogic(out chan byte) {
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

		// Non-blocking send if possible, else blocking is fine here
		select {
		case out <- result:
			println("Radio ->", result)
		default:
		}
	}
}
