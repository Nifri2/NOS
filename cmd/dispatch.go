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
	// Buffered so watchdog goroutine can signal without blocking
	rebootTrigger := make(chan struct{}, 1)

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
					// Scheduled reset at 5 minutes - signal main loop to broadcast then reset
					if tick >= 300 {
						fmt.Printf("[%d] [SCHED RESET] 5min uptime, triggering coordinated reboot\n", Ts())
						select {
						case rebootTrigger <- struct{}{}:
						default:
						}
						for {
							time.Sleep(100 * time.Millisecond)
						}
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

	// Current State
	var currentMode byte = 0x00

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

	// Keepalive Goroutine - broadcast to all workers
	go func() {
		for {
			time.Sleep(2 * time.Second)
			// Single broadcast packet reaches all workers
			sendPacket(Address_All, Cmd_NoOp, Anim_EyeIdle, Anim_MouthIdle)
		}
	}()

	// Helper for interruptible sleep that feeds watchdog during long sleeps
	// Sleeps in 1-second chunks, calling Watchdog.Update() between chunks
	// Returns (newMode, true) if interrupted
	// Returns (0, false) if completed
	sleepWithInterrupt := func(ms int) (byte, bool) {
		remaining := ms
		for remaining > 0 {
			chunk := remaining
			if chunk > 1000 {
				chunk = 1000
			}
			select {
			case code := <-radioChan:
				radioEvents++
				return code, true
			case <-time.After(time.Duration(chunk) * time.Millisecond):
			}
			machine.Watchdog.Update()
			remaining -= chunk
		}
		return 0, false
	}

	// 1Hz LED heartbeat and 10s heartbeat log (int64 ms to avoid time.Time allocations)
	lastLedToggleMs := Ts()
	lastHeartbeatMs := Ts()
	ledState := false

	fmt.Printf("[%d] Entering main dispatcher loop...\n", Ts())

	// TODO: Send Cmd_Ping to Address_All on a chosen radio input for liveness check

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
			fmt.Printf("[%d] [HB DISP] txQueued=%d txWritten=%d txDropped=%d radio=%d mode=0x%02X writerStuckSec=%d\n",
				nowMs, txQueued, txWritten, txDropped, radioEvents, currentMode, writerStuckSec)
			if writerStuckSec > 10 {
				fmt.Printf("[%d] [ALERT] UART writer appears stuck for %d seconds!\n", nowMs, writerStuckSec)
			}
			machine.Watchdog.Update()
			lastHeartbeatMs = nowMs
		}

		machine.Watchdog.Update()

		// Coordinated reboot: broadcast Cmd_Reboot, give workers 3s to play anim, then reset
		select {
		case <-rebootTrigger:
			fmt.Printf("[%d] [REBOOT] scheduled, broadcasting Cmd_Reboot\n", Ts())
			sendPacket(Address_All, Cmd_Reboot, Anim_RebootEye, Anim_RebootMouth)
			time.Sleep(3 * time.Second)
			fmt.Printf("[%d] [REBOOT] dispatcher resetting\n", Ts())
			for {
				time.Sleep(100 * time.Millisecond)
			}
		default:
		}

		switch currentMode {
		case 0x00: // Standard Idle (Workers 0 & 1)
			workers := []Address{Worker_0, Worker_1}

			// 1. Sleep between blinks (fixed 3s, no RNG to eliminate allocation)
			if m, changed := sleepWithInterrupt(3000); changed {
				currentMode = m
				fmt.Printf("[%d] Mode Change -> %d\n", Ts(), m)
				continue
			}

			// 2. Blink
			for _, w := range workers {
				sendPacket(w, Cmd_DisplayAnim, Anim_EyeBlink, Anim_MouthIdle)
			}

			// 3. Blink Duration
			if m, changed := sleepWithInterrupt(200); changed {
				currentMode = m
				fmt.Printf("[%d] Mode Change -> %d\n", Ts(), m)
				continue
			}

			// 4. Return to Idle
			for _, w := range workers {
				sendPacket(w, Cmd_DisplayAnim, Anim_EyeIdle, Anim_MouthIdle)
			}

		case 0x10: // Insignia Spinny (Worker 2)
			sendPacket(Worker_2, Cmd_DisplayAnim, Anim_EyeIdle, Anim_MouthIdle)

			// Feed watchdog before sleep
			machine.Watchdog.Update()

			// Just wait and check for interrupt
			if m, changed := sleepWithInterrupt(500); changed {
				currentMode = m
				fmt.Printf("[%d] Mode Change -> %d\n", Ts(), m)
				continue
			}

		case 0x0F: // System reboot (D+D radio input)
			fmt.Printf("[%d] [REBOOT] D+D pressed, broadcasting Cmd_Reboot\n", Ts())
			sendPacket(Address_All, Cmd_Reboot, Anim_RebootEye, Anim_RebootMouth)
			time.Sleep(3 * time.Second)
			fmt.Printf("[%d] [REBOOT] dispatcher resetting\n", Ts())
			for {
				time.Sleep(100 * time.Millisecond)
			}

		default:
			fmt.Printf("[%d] Unknown Mode, resetting to 0x00\n", Ts())
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
				fmt.Printf("[%d] Radio -> %d\n", Ts(), result)
			}
		default:
			fmt.Printf("[%d] [WARN] Radio channel full, dropping input\n", Ts())
		}
	}
}
