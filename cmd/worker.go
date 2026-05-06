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
		println("========================================")
		println("RunWorker ENTERED - addr=", int(config.Address))
		println("========================================")
		time.Sleep(200 * time.Millisecond)
	}

	// Now configure watchdog
	println("Configuring watchdog (5s timeout)...")
	machine.Watchdog.Configure(machine.WatchdogConfig{TimeoutMillis: 5000})
	println("Watchdog configured")

	animChan := make(chan animUpdate, 1)

	// Start animation routine in background
	println("Starting displayAnimation goroutine...")
	go displayAnimation(animChan, led)
	println("displayAnimation goroutine started")

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

	println("Entering main UART loop...")

	for {
		now := time.Now()

		// ============================================================
		// PHASE D: 1Hz LED heartbeat - proves loop is running
		// Uses time.Since() for robustness against loop timing changes
		// ============================================================
		if time.Since(lastLedToggle) >= time.Second {
			ledState = !ledState
			if ledState {
				led.High()
			} else {
				led.Low()
			}
			lastLedToggle = now
			machine.Watchdog.Update() // Feed watchdog on LED toggle
		}

		// Heartbeat log every 10 seconds
		if time.Since(lastHeartbeat) >= 10*time.Second {
			fmt.Printf("[HB] rxBytes=%d rxPackets=%d bufIdx=%d\n", rxBytes, rxPackets, bufIdx)
			lastHeartbeat = now
		}

		// Inter-byte timeout: reset buffer if >20ms since last byte
		if bufIdx > 0 && time.Since(lastByteTime) > 20*time.Millisecond {
			// Dump partial buffer for debugging
			fmt.Printf("[TIMEOUT] bufIdx=%d partial=[", bufIdx)
			for i := 0; i < bufIdx; i++ {
				fmt.Printf("%02X", buf[i])
				if i < bufIdx-1 {
					print(" ")
				}
			}
			println("]")
			bufIdx = 0
		}

		if uart.Buffered() > 0 {
			b, _ := uart.ReadByte()
			rxBytes++
			lastByteTime = now

			if debugLog {
				fmt.Printf("[RX] byte=%02X bufIdx=%d\n", b, bufIdx)
			}

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
							fmt.Printf("[PKT] #%d Addr=%02X Cmd=%02X Eye=%02X Mouth=%02X\n",
								rxPackets, addrByte, cmdByte, eyeByte, mouthByte)
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
								// NoOp - just keeps watchdog happy via packet receipt
								machine.Watchdog.Update()
								println("[NoOp Watchdog Update] -> Pubby Habby *woof*")
							case Cmd_Ping:
								println("[PING from dispatcher]")
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
										println("[WARN] animChan full, dropping update")
									}
								}
							}
						}
					} else {
						// CRC fail - dump full packet for debugging
						fmt.Printf("[CRC FAIL] calc=%02X recv=%02X pkt=[", calculatedChecksum, checksumByte)
						for i := 0; i < PacketSize; i++ {
							fmt.Printf("%02X", buf[i])
							if i < PacketSize-1 {
								print(" ")
							}
						}
						println("]")
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
func displayAnimation(animChan chan animUpdate, led machine.Pin) {
	for {
		func() {
			defer func() {
				if r := recover(); r != nil {
					fmt.Printf("[ANIM PANIC] %v\n", r)
					// Blink LED rapidly to signal panic
					for i := 0; i < 10; i++ {
						led.High()
						time.Sleep(50 * time.Millisecond)
						led.Low()
						time.Sleep(50 * time.Millisecond)
					}
					runtime.GC() // Reduce odds of allocator state corruption
					time.Sleep(time.Second)
				}
			}()
			displayAnimationLoop(animChan, led)
		}()
		println("[ANIM] Restarting animation loop...")
	}
}

// displayAnimationLoop is the actual animation rendering logic
func displayAnimationLoop(animChan chan animUpdate, led machine.Pin) {
	println("[ANIM] displayAnimation started, waiting 2s for board stabilization...")

	// Wait for the board to stabilize
	time.Sleep(2 * time.Second)

	println("[ANIM] Stabilization complete, initializing...")

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

	fmt.Printf("[ANIM] Found eye_idle=%v mouth_idle=%v\n", eyeIdleAnim != nil, mouthAnim != nil)

	// Helper to convert []byte (G,R,B triplets) to []uint32 for PIO WS2812
	// Color format: uint32(g)<<24 | uint32(r)<<16 | uint32(b)<<8
	bytesToRaw := func(data []byte) []uint32 {
		pixelCount := len(data) / 3
		raw := make([]uint32, pixelCount)
		for i := 0; i < pixelCount; i++ {
			g := data[i*3]
			r := data[i*3+1]
			b := data[i*3+2]
			raw[i] = uint32(g)<<24 | uint32(r)<<16 | uint32(b)<<8
		}
		return raw
	}

	// Helper to pre-convert all frames of an animation
	preconvert := func(anim *Animation) [][]uint32 {
		if anim == nil || len(anim.Frames) == 0 {
			return nil
		}
		result := make([][]uint32, len(anim.Frames))
		for i, f := range anim.Frames {
			result[i] = bytesToRaw(f)
		}
		return result
	}

	// PIO-based WS2812 - immune to UART interrupts
	var strip1 *piolib.WS2812B
	var strip2 *piolib.WS2812B
	eyeOK := false
	mouthOK := false

	// safeWrite wraps WriteRaw with panic recovery
	safeWrite := func(name string, strip *piolib.WS2812B, raw []uint32) {
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("[ANIM safeWrite panic name=%s len=%d] %v\n", name, len(raw), r)
			}
		}()
		if strip != nil && len(raw) > 0 {
			strip.WriteRaw(raw)
		}
	}

	// Eye strip: PIO0 SM0, GP18
	println("[ANIM] Claiming SM0 for eye strip...")
	sm0, err := pio.PIO0.ClaimStateMachine()
	if err != nil {
		println("[ANIM] ERROR: Failed to claim SM0:", err.Error())
	} else {
		println("[ANIM] SM0 claimed, initializing WS2812B on GP18...")
		strip1, err = piolib.NewWS2812B(sm0, machine.GP18)
		if err != nil {
			println("[ANIM] ERROR: Failed to init eye strip:", err.Error())
		} else {
			println("[ANIM] Eye strip initialized, enabling DMA...")
			strip1.EnableDMA(true)
			eyeOK = true
			println("[ANIM] Eye strip ready")
		}
	}

	// Mouth strip: PIO0 SM1, GP12
	println("[ANIM] Claiming SM1 for mouth strip...")
	sm1, err := pio.PIO0.ClaimStateMachine()
	if err != nil {
		println("[ANIM] ERROR: Failed to claim SM1:", err.Error())
	} else {
		println("[ANIM] SM1 claimed, initializing WS2812B on GP12...")
		strip2, err = piolib.NewWS2812B(sm1, machine.GP12)
		if err != nil {
			println("[ANIM] ERROR: Failed to init mouth strip:", err.Error())
		} else {
			println("[ANIM] Mouth strip initialized, enabling DMA...")
			strip2.EnableDMA(true)
			mouthOK = true
			println("[ANIM] Mouth strip ready")
		}
	}

	fmt.Printf("[ANIM] PIO init complete: eye=%v mouth=%v\n", eyeOK, mouthOK)

	// Self-test: write black frames to verify strips work
	println("[ANIM SELFTEST] Testing strips with black frames...")
	if eyeOK {
		testFrame := make([]uint32, EyeFrameWidth*EyeFrameHeight)
		safeWrite("eye-test", strip1, testFrame)
		println("[ANIM SELFTEST] eye OK")
	}
	if mouthOK {
		testFrame := make([]uint32, MouthFrameWidth*MouthFrameHeight)
		safeWrite("mouth-test", strip2, testFrame)
		println("[ANIM SELFTEST] mouth OK")
	}
	time.Sleep(100 * time.Millisecond)

	// If both strips failed, blink LED rapidly to indicate error but don't exit
	if !eyeOK && !mouthOK {
		println("[ANIM] WARNING: Both strips failed! Blinking error pattern...")
		for i := 0; i < 20; i++ {
			led.High()
			time.Sleep(50 * time.Millisecond)
			led.Low()
			time.Sleep(50 * time.Millisecond)
		}
		println("[ANIM] Continuing anyway (will just consume animChan)...")
	}

	var currentEyeAnim *Animation
	var currentMouthAnim *Animation
	var queuedEyeAnim *Animation = nil
	var queuedMouthAnim *Animation = nil

	// Cached pre-converted frames (avoids per-frame allocation)
	var curEyeRaw [][]uint32
	var curMouthRaw [][]uint32

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
	curEyeRaw = preconvert(currentEyeAnim)

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
	curMouthRaw = preconvert(currentMouthAnim)

	var eyeFrameCounter int64
	var mouthFrameCounter int64

	println("[ANIM] Entering animation loop...")

	for {
		// Check for new animation command (non-blocking)
		select {
		case update := <-animChan:
			if update.Eye != nil {
				queuedEyeAnim = update.Eye
				if debugLog {
					fmt.Printf("[ANIM] Queued eye: %s\n", update.Eye.Name)
				}
			}
			if update.Mouth != nil {
				queuedMouthAnim = update.Mouth
				if debugLog {
					fmt.Printf("[ANIM] Queued mouth: %s\n", update.Mouth.Name)
				}
			}
		default:
		}

		// Write to eye strip if available (using pre-converted cached frames)
		if eyeOK && curEyeRaw != nil && len(curEyeRaw) > 0 {
			frameIdx := eyeFrameCounter % int64(len(curEyeRaw))
			raw := curEyeRaw[frameIdx]
			// Validate frame size before write
			expectedPixels := EyeFrameWidth * EyeFrameHeight
			if len(raw) != expectedPixels {
				fmt.Printf("[ANIM SKIP eye] len=%d expected=%d name=%s frame=%d\n",
					len(raw), expectedPixels, currentEyeAnim.Name, frameIdx)
			} else {
				safeWrite("eye", strip1, raw)
			}
		}

		// Write to mouth strip if available (using pre-converted cached frames)
		if mouthOK && curMouthRaw != nil && len(curMouthRaw) > 0 {
			frameIdx := mouthFrameCounter % int64(len(curMouthRaw))
			raw := curMouthRaw[frameIdx]
			// Validate frame size before write
			expectedPixels := MouthFrameWidth * MouthFrameHeight
			if len(raw) != expectedPixels {
				fmt.Printf("[ANIM SKIP mouth] len=%d expected=%d name=%s frame=%d\n",
					len(raw), expectedPixels, currentMouthAnim.Name, frameIdx)
			} else {
				safeWrite("mouth", strip2, raw)
			}
		}

		// PIO handles latch timing internally - no manual reset delay needed

		eyeFrameCounter++
		mouthFrameCounter++

		// Transition eye at end of cycle
		if queuedEyeAnim != nil && currentEyeAnim != nil && len(curEyeRaw) > 0 {
			if eyeFrameCounter > 0 && (eyeFrameCounter%int64(len(curEyeRaw)) == 0) {
				if queuedEyeAnim != currentEyeAnim {
					if debugLog {
						fmt.Printf("[ANIM] Transitioning Eye to: %s\n", queuedEyeAnim.Name)
					}
					currentEyeAnim = queuedEyeAnim
					curEyeRaw = preconvert(currentEyeAnim)
					eyeFrameCounter = 0
				}
				queuedEyeAnim = nil
			}
		}

		// Transition mouth at end of cycle
		if queuedMouthAnim != nil && currentMouthAnim != nil && len(curMouthRaw) > 0 {
			if mouthFrameCounter > 0 && (mouthFrameCounter%int64(len(curMouthRaw)) == 0) {
				if queuedMouthAnim != currentMouthAnim {
					if debugLog {
						fmt.Printf("[ANIM] Transitioning Mouth to: %s\n", queuedMouthAnim.Name)
					}
					currentMouthAnim = queuedMouthAnim
					curMouthRaw = preconvert(currentMouthAnim)
					mouthFrameCounter = 0
				}
				queuedMouthAnim = nil
			}
		}

		// ~60Hz frame rate cap (10ms was too fast, produced redundant WriteRaw calls)
		time.Sleep(16 * time.Millisecond)
	}
}
