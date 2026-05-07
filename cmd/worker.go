package cmd

import (
	"fmt"
	"machine"
	"runtime"
	"time"

	pio "github.com/tinygo-org/pio/rp2-pio"
	"github.com/tinygo-org/pio/rp2-pio/piolib"
)

// Each MCU takes care of 1 mouth and 1 eye WS2812 strip
// There will be a separate worker that will only display Insignia animations

// Expected frame sizes for validation
const (
	expectedEyeBytes   = EyeFrameWidth * EyeFrameHeight * 3     // 768 bytes
	expectedMouthBytes = MouthFrameWidth * MouthFrameHeight * 3 // 1536 bytes
)

func RunWorker(config Settings, uart *machine.UART, led machine.Pin) {
	// ============================================================
	// PHASE C: Visible "RunWorker entered" signal BEFORE anything else
	// Toggle LED 10 times at 100ms intervals (2 seconds visible)
	// ============================================================
	for i := 0; i < 10; i++ {
		led.High()
		time.Sleep(100 * time.Millisecond)
		led.Low()
		time.Sleep(100 * time.Millisecond)
	}

	// Print banner 3 times with gaps - some may land even if USB slow
	for i := 0; i < 3; i++ {
		fmt.Printf("[%d] ========================================\n", Ts())
		fmt.Printf("[%d] RunWorker ENTERED - addr=%d\n", Ts(), int(config.Address))
		fmt.Printf("[%d] ========================================\n", Ts())
		time.Sleep(200 * time.Millisecond)
	}

	// Now configure watchdog
	fmt.Printf("[%d] Configuring watchdog (5s timeout)...\n", Ts())
	machine.Watchdog.Configure(machine.WatchdogConfig{TimeoutMillis: 5000})
	fmt.Printf("[%d] Watchdog configured\n", Ts())

	animChan := make(chan animUpdate, 1)

	// Start animation routine in background
	fmt.Printf("[%d] Starting displayAnimation goroutine...\n", Ts())
	go displayAnimation(animChan, led, config)
	fmt.Printf("[%d] displayAnimation goroutine started\n", Ts())

	// Watchdog logging goroutine - proves scheduler is alive even if other goroutines freeze
	// Also logs heap stats every 30s to monitor GC pressure
	go func() {
		var tick int
		var ms runtime.MemStats
		for {
			time.Sleep(5 * time.Second)
			tick++
			fmt.Printf("[%d] [WD] alive\n", Ts())
			// Log heap stats every 30s (every 6th tick)
			if tick%6 == 0 {
				runtime.ReadMemStats(&ms)
				fmt.Printf("[%d] [MEM] alloc=%d totalAlloc=%d sys=%d\n",
					Ts(), ms.Alloc, ms.TotalAlloc, ms.Sys)
			}
		}
	}()

	const (
		HeaderByte = 0xAA
		PacketSize = 6
	)

	// Buffer to hold incoming packet
	// [Header, Addr, Cmd, Eye, Mouth, Checksum]
	buf := make([]byte, PacketSize)
	bufIdx := 0
	lastByteTime := time.Now()

	// Diagnostic counters
	var rxBytes uint32
	var rxPackets uint32
	lastHeartbeat := time.Now()
	lastLedToggle := time.Now()
	ledState := false
	rebootRequested := false

	fmt.Printf("[%d] Entering main UART loop...\n", Ts())

	for {
		now := time.Now()

		// ============================================================
		// PHASE D: 1Hz LED heartbeat - proves loop is running
		// Uses time.Since() for robustness against loop timing changes
		// The 1Hz LED heartbeat is the primary watchdog feed for workers.
		// This ensures watchdog is fed even if no UART packets arrive.
		// ============================================================
		if time.Since(lastLedToggle) >= time.Second {
			ledState = !ledState
			if ledState {
				led.High()
			} else {
				led.Low()
			}
			lastLedToggle = now
			if !rebootRequested {
				machine.Watchdog.Update()
			}
		}

		// Heartbeat log every 10 seconds
		if time.Since(lastHeartbeat) >= 10*time.Second {
			fmt.Printf("[%d] [HB] rxBytes=%d rxPackets=%d bufIdx=%d\n", Ts(), rxBytes, rxPackets, bufIdx)
			lastHeartbeat = now
		}

		// Inter-byte timeout: reset buffer if >20ms since last byte
		if bufIdx > 0 && time.Since(lastByteTime) > 20*time.Millisecond {
			// Dump partial buffer for debugging
			fmt.Printf("[%d] [TIMEOUT] bufIdx=%d partial=[", Ts(), bufIdx)
			for i := 0; i < bufIdx; i++ {
				fmt.Printf("%02X", buf[i])
				if i < bufIdx-1 {
					print(" ")
				}
			}
			fmt.Printf("]\n")
			bufIdx = 0
		}

		if uart.Buffered() > 0 {
			b, _ := uart.ReadByte()
			rxBytes++
			lastByteTime = now

			// State machine-ish logic
			if bufIdx == 0 {
				// Waiting for Header
				if b == HeaderByte {
					buf[0] = b
					bufIdx++
				}
			} else {
				// Filling buffer
				buf[bufIdx] = b
				bufIdx++

				if bufIdx == PacketSize {
					// Packet complete, verify CRC-8/MAXIM checksum
					// buf = [AA, Addr, Cmd, Eye, Mouth, Checksum]
					addrByte := buf[1]
					cmdByte := buf[2]
					eyeByte := buf[3]
					mouthByte := buf[4]
					checksumByte := buf[5]

					calculatedChecksum := Crc8(buf[1:5])

					if calculatedChecksum == checksumByte {
						// Valid packet
						rxPackets++
						if debugLog {
							fmt.Printf("[%d] [PKT] #%d Addr=%02X Cmd=%02X Eye=%02X Mouth=%02X\n",
								Ts(), rxPackets, addrByte, cmdByte, eyeByte, mouthByte)
						}

						// Accept if addressed to us or broadcast
						if Address(addrByte) == config.Address || Address(addrByte) == Address_All {
							cmd := Command(cmdByte)
							switch cmd {
							case Cmd_LedOn:
								led.High()
								ledState = true
							case Cmd_LedOff:
								led.Low()
								ledState = false
							case Cmd_NoOp:
								if !rebootRequested {
									machine.Watchdog.Update()
								}
							case Cmd_Ping:
								fmt.Printf("[%d] [PING from dispatcher]\n", Ts())
							case Cmd_Reboot:
								fmt.Printf("[%d] [REBOOT] received, awaiting watchdog reset\n", Ts())
								rebootRequested = true

						case Cmd_DisplayAnim:
								eyeIdx := MapAnimation(config.Address, AnimationID(eyeByte))
								mouthIdx := MapAnimation(config.Address, AnimationID(mouthByte))
								var update animUpdate

								if eyeIdx >= 0 && eyeIdx < len(LoadedAnimations) {
									update.Eye = LoadedAnimations[eyeIdx]
								}
								if mouthIdx >= 0 && mouthIdx < len(LoadedAnimations) {
									update.Mouth = LoadedAnimations[mouthIdx]
								}
								if update.Eye != nil || update.Mouth != nil {
									// Non-blocking send - don't stall UART if animation is slow
									select {
									case animChan <- update:
									default:
										fmt.Printf("[%d] [WARN] animChan full, dropping update\n", Ts())
									}
								}
							}
						}
					} else {
						// CRC fail - dump full packet for debugging
						fmt.Printf("[%d] [CRC FAIL] calc=%02X recv=%02X pkt=[", Ts(), calculatedChecksum, checksumByte)
						for i := 0; i < PacketSize; i++ {
							fmt.Printf("%02X", buf[i])
							if i < PacketSize-1 {
								print(" ")
							}
						}
						fmt.Printf("]\n")
					}

					// Reset buffer
					bufIdx = 0
				}
			}
		} else {
			// Small sleep to yield
			time.Sleep(time.Millisecond)
		}
	}
}

// displayAnimation is a recoverable supervisor that restarts the animation loop on panic
func displayAnimation(animChan chan animUpdate, led machine.Pin, config Settings) {
	for {
		func() {
			defer func() {
				if r := recover(); r != nil {
					fmt.Printf("[%d] [ANIM PANIC] %v\n", Ts(), r)
					// Blink LED rapidly to signal panic
					for i := 0; i < 10; i++ {
						led.High()
						time.Sleep(50 * time.Millisecond)
						led.Low()
						time.Sleep(50 * time.Millisecond)
					}
					time.Sleep(time.Second)
				}
			}()
			displayAnimationLoop(animChan, led, config)
		}()
		fmt.Printf("[%d] [ANIM] Restarting animation loop...\n", Ts())
	}
}

// displayAnimationLoop is the actual animation rendering logic
func displayAnimationLoop(animChan chan animUpdate, led machine.Pin, config Settings) {
	fmt.Printf("[%d] [ANIM] displayAnimation started, waiting 2s for board stabilization...\n", Ts())

	// Wait for the board to stabilize
	time.Sleep(2 * time.Second)

	fmt.Printf("[%d] [ANIM] Stabilization complete, initializing...\n", Ts())

	// Helper to find animation by name
	findAnim := func(name string) *Animation {
		for _, a := range LoadedAnimations {
			if a.Name == name {
				return a
			}
		}
		return nil
	}

	eyeIdleAnim := findAnim("eye_idle")
	mouthAnim := findAnim("mouth_idle")

	fmt.Printf("[%d] [ANIM] Found eye_idle=%v mouth_idle=%v\n", Ts(), eyeIdleAnim != nil, mouthAnim != nil)

	// bytesToRawInto converts GRB bytes into dst buffer (zero allocation)
	bytesToRawInto := func(dst []uint32, src []byte) {
		n := len(src) / 3
		if n > len(dst) {
			n = len(dst)
		}
		for i := 0; i < n; i++ {
			g := src[i*3]
			r := src[i*3+1]
			b := src[i*3+2]
			dst[i] = uint32(g)<<24 | uint32(r)<<16 | uint32(b)<<8
		}
	}

	// PIO-based WS2812 - immune to UART interrupts
	var strip1 *piolib.WS2812B
	var strip2 *piolib.WS2812B
	eyeOK := false
	mouthOK := false

	// Eye strip: PIO0 SM0, GP18
	fmt.Printf("[%d] [ANIM] Claiming SM0 for eye strip...\n", Ts())
	sm0, err := pio.PIO0.ClaimStateMachine()
	if err != nil {
		fmt.Printf("[%d] [ANIM] ERROR: Failed to claim SM0: %s\n", Ts(), err.Error())
	} else {
		fmt.Printf("[%d] [ANIM] SM0 claimed, initializing WS2812B on GP18...\n", Ts())
		strip1, err = piolib.NewWS2812B(sm0, machine.GP18)
		if err != nil {
			fmt.Printf("[%d] [ANIM] ERROR: Failed to init eye strip: %s\n", Ts(), err.Error())
		} else {
			fmt.Printf("[%d] [ANIM] Eye strip initialized, enabling DMA...\n", Ts())
			strip1.EnableDMA(true)
			eyeOK = true
			fmt.Printf("[%d] [ANIM] Eye strip ready\n", Ts())
		}
	}

	// Mouth strip: PIO0 SM1, GP12
	fmt.Printf("[%d] [ANIM] Claiming SM1 for mouth strip...\n", Ts())
	sm1, err := pio.PIO0.ClaimStateMachine()
	if err != nil {
		fmt.Printf("[%d] [ANIM] ERROR: Failed to claim SM1: %s\n", Ts(), err.Error())
	} else {
		fmt.Printf("[%d] [ANIM] SM1 claimed, initializing WS2812B on GP12...\n", Ts())
		strip2, err = piolib.NewWS2812B(sm1, machine.GP12)
		if err != nil {
			fmt.Printf("[%d] [ANIM] ERROR: Failed to init mouth strip: %s\n", Ts(), err.Error())
		} else {
			fmt.Printf("[%d] [ANIM] Mouth strip initialized, enabling DMA...\n", Ts())
			strip2.EnableDMA(true)
			mouthOK = true
			fmt.Printf("[%d] [ANIM] Mouth strip ready\n", Ts())
		}
	}

	fmt.Printf("[%d] [ANIM] PIO init complete: eye=%v mouth=%v\n", Ts(), eyeOK, mouthOK)

	// Fixed rendering buffers - allocated once, reused forever
	eyeBuffer := make([]uint32, EyeFrameWidth*EyeFrameHeight)     // 256 pixels
	mouthBuffer := make([]uint32, MouthFrameWidth*MouthFrameHeight) // 512 pixels
	fmt.Printf("[%d] [ANIM] eyeBuffer=%d mouthBuffer=%d totalRAM=%d bytes\n",
		Ts(), len(eyeBuffer), len(mouthBuffer), (len(eyeBuffer)+len(mouthBuffer))*4)

	// Self-test: write black frames to verify strips work
	fmt.Printf("[%d] [ANIM SELFTEST] Testing strips with black frames...\n", Ts())
	if eyeOK {
		testFrame := make([]uint32, EyeFrameWidth*EyeFrameHeight)
		strip1.WriteRaw(testFrame)
		fmt.Printf("[%d] [ANIM SELFTEST] eye OK\n", Ts())
	}
	if mouthOK {
		testFrame := make([]uint32, MouthFrameWidth*MouthFrameHeight)
		strip2.WriteRaw(testFrame)
		fmt.Printf("[%d] [ANIM SELFTEST] mouth OK\n", Ts())
	}
	time.Sleep(100 * time.Millisecond)

	// If both strips failed, blink LED rapidly to indicate error but don't exit
	if !eyeOK && !mouthOK {
		fmt.Printf("[%d] [ANIM] WARNING: Both strips failed! Blinking error pattern...\n", Ts())
		for i := 0; i < 20; i++ {
			led.High()
			time.Sleep(50 * time.Millisecond)
			led.Low()
			time.Sleep(50 * time.Millisecond)
		}
		fmt.Printf("[%d] [ANIM] Continuing anyway (will just consume animChan)...\n", Ts())
	}

	var currentEyeAnim *Animation
	var currentMouthAnim *Animation
	var queuedEyeAnim *Animation = nil
	var queuedMouthAnim *Animation = nil

	// Initial setup for currentEyeAnim with properly-sized black frame fallback
	if eyeIdleAnim == nil {
		if len(LoadedAnimations) > 0 {
			currentEyeAnim = LoadedAnimations[0]
		} else {
			// Properly-sized black frame fallback (not empty!)
			currentEyeAnim = &Animation{
				FrameCount: 1,
				Frames:     [][]byte{make([]byte, EyeFrameWidth*EyeFrameHeight*3)},
				Name:       "fallback_eye_black",
			}
		}
	} else {
		currentEyeAnim = eyeIdleAnim
	}
	fmt.Printf("[%d] [ANIM] Initial eye: %s frames=%d\n", Ts(), currentEyeAnim.Name, len(currentEyeAnim.Frames))

	// Initial setup for currentMouthAnim with properly-sized black frame fallback
	if mouthAnim == nil {
		// Properly-sized black frame fallback (not empty!)
		currentMouthAnim = &Animation{
			FrameCount: 1,
			Frames:     [][]byte{make([]byte, MouthFrameWidth*MouthFrameHeight*3)},
			Name:       "fallback_mouth_black",
		}
	} else {
		currentMouthAnim = mouthAnim
	}
	fmt.Printf("[%d] [ANIM] Initial mouth: %s frames=%d\n", Ts(), currentMouthAnim.Name, len(currentMouthAnim.Frames))

	var eyeFrameCounter int64
	var mouthFrameCounter int64

	fmt.Printf("[%d] [ANIM] Entering animation loop...\n", Ts())

	// Canary goroutine - prints every 2s to prove animation goroutine is alive
	go func() {
		tick := int64(0)
		for {
			time.Sleep(2 * time.Second)
			tick++
			fmt.Printf("[%d] [ANIM TICK] %d\n", Ts(), tick)
		}
	}()

	var loopIter int64
	for {
		loopIter++
		if debugLog && loopIter%100 == 0 {
			fmt.Printf("[%d] [ANIM LOOP] iter=%d eyeFrame=%d mouthFrame=%d\n",
				Ts(), loopIter, eyeFrameCounter, mouthFrameCounter)
		}
		// Check for new animation command (non-blocking)
		select {
		case update := <-animChan:
			if update.Eye != nil {
				queuedEyeAnim = update.Eye
				if debugLog {
					fmt.Printf("[%d] [ANIM] Queued eye: %s\n", Ts(), update.Eye.Name)
				}
			}
			if update.Mouth != nil {
				queuedMouthAnim = update.Mouth
				if debugLog {
					fmt.Printf("[%d] [ANIM] Queued mouth: %s\n", Ts(), update.Mouth.Name)
				}
			}
		default:
		}

		// Write to eye strip if available (convert on demand into fixed buffer)
		if eyeOK && currentEyeAnim != nil && len(currentEyeAnim.Frames) > 0 {
			frameIdx := eyeFrameCounter % int64(len(currentEyeAnim.Frames))
			frameBytes := currentEyeAnim.Frames[frameIdx]
			if len(frameBytes)/3 == EyeFrameWidth*EyeFrameHeight {
				bytesToRawInto(eyeBuffer, frameBytes)
				strip1.WriteRaw(eyeBuffer)
			} else {
				fmt.Printf("[%d] [ANIM SKIP eye] pixels=%d expected=%d name=%s frame=%d\n",
					Ts(), len(frameBytes)/3, EyeFrameWidth*EyeFrameHeight, currentEyeAnim.Name, frameIdx)
			}
		}

		// Write to mouth strip if available (convert on demand into fixed buffer)
		if mouthOK && currentMouthAnim != nil && len(currentMouthAnim.Frames) > 0 {
			frameIdx := mouthFrameCounter % int64(len(currentMouthAnim.Frames))
			frameBytes := currentMouthAnim.Frames[frameIdx]
			if len(frameBytes)/3 == MouthFrameWidth*MouthFrameHeight {
				bytesToRawInto(mouthBuffer, frameBytes)
				strip2.WriteRaw(mouthBuffer)
			} else {
				fmt.Printf("[%d] [ANIM SKIP mouth] pixels=%d expected=%d name=%s frame=%d\n",
					Ts(), len(frameBytes)/3, MouthFrameWidth*MouthFrameHeight, currentMouthAnim.Name, frameIdx)
			}
		}

		// PIO handles latch timing internally - no manual reset delay needed

		eyeFrameCounter++
		mouthFrameCounter++

		// Transition eye at end of cycle
		if queuedEyeAnim != nil && currentEyeAnim != nil && len(currentEyeAnim.Frames) > 0 {
			if eyeFrameCounter > 0 && (eyeFrameCounter%int64(len(currentEyeAnim.Frames)) == 0) {
				if queuedEyeAnim != currentEyeAnim {
					if debugLog {
						fmt.Printf("[%d] [ANIM] Transitioning Eye to: %s\n", Ts(), queuedEyeAnim.Name)
					}
					currentEyeAnim = queuedEyeAnim
					eyeFrameCounter = 0
				}
				queuedEyeAnim = nil
			}
		}

		// Transition mouth at end of cycle
		if queuedMouthAnim != nil && currentMouthAnim != nil && len(currentMouthAnim.Frames) > 0 {
			if mouthFrameCounter > 0 && (mouthFrameCounter%int64(len(currentMouthAnim.Frames)) == 0) {
				if queuedMouthAnim != currentMouthAnim {
					if debugLog {
						fmt.Printf("[%d] [ANIM] Transitioning Mouth to: %s\n", Ts(), queuedMouthAnim.Name)
					}
					currentMouthAnim = queuedMouthAnim
					mouthFrameCounter = 0
				}
				queuedMouthAnim = nil
			}
		}

		// ~60Hz frame rate cap (10ms was too fast, produced redundant WriteRaw calls)
		time.Sleep(16 * time.Millisecond)
	}
}
