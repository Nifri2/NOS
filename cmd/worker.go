//go:build tinygo

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
	brightnessChan := make(chan int, 4)

	// Start animation routine in background
	fmt.Printf("[%d] Starting displayAnimation goroutine...\n", Ts())
	go displayAnimation(animChan, brightnessChan, led, config)
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

	// Packet framing/decoding lives in protocol.go (pure Go) so the host loopback
	// tests exercise the exact code the worker runs. The parser handles header
	// sync, CRC, and address filtering; the inter-byte timeout stays here because
	// it depends on wall clock.
	parser := NewPacketParser(config.Address)
	lastByteTime := time.Now()

	// Diagnostic counters
	var rxBytes uint32
	var rxPackets uint32
	lastHeartbeat := time.Now()
	lastLedToggle := time.Now()
	ledState := false

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
			machine.Watchdog.Update()
		}

		// Heartbeat log every 10 seconds
		if time.Since(lastHeartbeat) >= 10*time.Second {
			fmt.Printf("[%d] [HB] rxBytes=%d rxPackets=%d bufIdx=%d\n", Ts(), rxBytes, rxPackets, parser.Pending())
			lastHeartbeat = now
		}

		// Inter-byte timeout: drop a partial frame if >20ms since the last byte.
		if parser.Pending() > 0 && time.Since(lastByteTime) > 20*time.Millisecond {
			partial := parser.Partial()
			fmt.Printf("[%d] [TIMEOUT] bufIdx=%d partial=[", Ts(), len(partial))
			for i := 0; i < len(partial); i++ {
				fmt.Printf("%02X", partial[i])
				if i < len(partial)-1 {
					print(" ")
				}
			}
			fmt.Printf("]\n")
			parser.Reset()
		}

		if uart.Buffered() > 0 {
			b, _ := uart.ReadByte()
			rxBytes++
			lastByteTime = now

			pkt, status := parser.Feed(b)
			switch status {
			case ParseIncomplete:
				// keep buffering

			case ParseBadCRC:
				// Dump full packet for debugging.
				frame := parser.LastFrame()
				calc := Crc8Bytes4(frame[1], frame[2], frame[3], frame[4])
				fmt.Printf("[%d] [CRC FAIL] calc=%02X recv=%02X pkt=[", Ts(), calc, frame[5])
				for i := 0; i < PacketSize; i++ {
					fmt.Printf("%02X", frame[i])
					if i < PacketSize-1 {
						print(" ")
					}
				}
				fmt.Printf("]\n")

			case ParseAccepted, ParseNotForUs:
				// rxPackets counts every CRC-valid frame, even those addressed
				// elsewhere (matches the original diagnostic semantics).
				rxPackets++
				if debugLog {
					fmt.Printf("[%d] [PKT] #%d Addr=%02X Cmd=%02X Eye=%02X Mouth=%02X\n",
						Ts(), rxPackets, pkt.Addr, byte(pkt.Cmd), byte(pkt.Eye), byte(pkt.Mouth))
				}

				if status == ParseAccepted {
					switch pkt.Cmd {
					case Cmd_LedOn:
						led.High()
						ledState = true
					case Cmd_LedOff:
						led.Low()
						ledState = false
					case Cmd_NoOp:
						machine.Watchdog.Update()
					case Cmd_Ping:
						fmt.Printf("[%d] [PING from dispatcher]\n", Ts())
					case Cmd_Reboot:
						fmt.Printf("[%d] [REBOOT] received, hard resetting in 500ms\n", Ts())
						time.Sleep(500 * time.Millisecond)
						HardReset()
					case Cmd_DayMode:
						select {
						case brightnessChan <- DayModeBrightnessPercent:
						default:
						}
						fmt.Printf("[%d] [MODE] day (+%d%%)\n", Ts(), DayModeBrightnessPercent-100)
					case Cmd_NightMode:
						select {
						case brightnessChan <- 100:
						default:
						}
						fmt.Printf("[%d] [MODE] night\n", Ts())

					case Cmd_DisplayAnim:
						eyeIdx := MapAnimation(config.Address, pkt.Eye)
						mouthIdx := MapAnimation(config.Address, pkt.Mouth)
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
			}
		} else {
			// Small sleep to yield
			time.Sleep(time.Millisecond)
		}
	}
}

// displayAnimation is a recoverable supervisor that restarts the animation loop on panic
func displayAnimation(animChan chan animUpdate, brightnessChan chan int, led machine.Pin, config Settings) {
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
			displayAnimationLoop(animChan, brightnessChan, led, config)
		}()
		fmt.Printf("[%d] [ANIM] Restarting animation loop...\n", Ts())
	}
}

// displayAnimationLoop is the actual animation rendering logic
func displayAnimationLoop(animChan chan animUpdate, brightnessChan chan int, led machine.Pin, config Settings) {
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

	// Runtime brightness scale applied per-pixel on top of compiled brightness.
	// 100 = night (default), DayModeBrightnessPercent (e.g. 110) when day mode active.
	// Updated via brightnessChan; captured by the rendering closures below.
	brightnessPercent := 100

	// bytesToRawInto / lerpFrameInto (the per-pixel frame math) live in blend.go
	// as pure-Go package functions so they can be host-unit-tested; they take
	// brightnessPercent explicitly instead of capturing it.

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
	eyeBuffer := make([]uint32, EyeFrameWidth*EyeFrameHeight)      // 256 pixels × 4 bytes = 1024 bytes
	mouthBuffer := make([]uint32, MouthFrameWidth*MouthFrameHeight) // 512 pixels × 4 bytes = 2048 bytes
	fmt.Printf("[%d] [ANIM] eyeBuffer=%d mouthBuffer=%d totalRAM=%d bytes\n",
		Ts(), len(eyeBuffer), len(mouthBuffer), (len(eyeBuffer)+len(mouthBuffer))*4)

	// Transition snapshot buffers - store the "from" frame as raw GRB bytes, allocated once
	eyeTransitionFrom := make([]byte, EyeFrameWidth*EyeFrameHeight*3)       // 768 bytes
	mouthTransitionFrom := make([]byte, MouthFrameWidth*MouthFrameHeight*3)  // 1536 bytes

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

	// Transition state - eye
	const eyeTransitionFrames = 15  // ~250ms at 60fps; tune per animation type if needed
	const mouthTransitionFrames = 6 // ~100ms at 60fps; snappier for speech tracking

	var eyeTransitioning bool
	var eyeTransitionStep int
	var eyeTransitionTarget *Animation

	// Transition state - mouth
	var mouthTransitioning bool
	var mouthTransitionStep int
	var mouthTransitionTarget *Animation

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

		// Check for brightness mode update (non-blocking)
		select {
		case bp := <-brightnessChan:
			brightnessPercent = bp
			if debugLog {
				fmt.Printf("[%d] [ANIM] brightnessPercent=%d\n", Ts(), brightnessPercent)
			}
		default:
		}

		// Eye transition trigger. Three cases:
		//   1. Multi-frame target (e.g. eye_blink): immediate hard switch — the animation IS the effect.
		//   2. Multi-frame source, static target: hold until current animation cycle ends, then blend.
		//      This lets the blink play through completely before returning to idle.
		//   3. Static source, static target: smooth blend immediately (or redirect if mid-blend).
		if queuedEyeAnim != nil {
			if len(queuedEyeAnim.Frames) > 1 {
				// Case 1: animated target — immediate hard switch, cancel any in-progress blend
				if queuedEyeAnim != currentEyeAnim {
					currentEyeAnim = queuedEyeAnim
					eyeFrameCounter = 0
					eyeTransitioning = false
					eyeTransitionTarget = nil
					if debugLog {
						fmt.Printf("[%d] [ANIM] Eye switch (animated): %s\n", Ts(), queuedEyeAnim.Name)
					}
				}
				queuedEyeAnim = nil
			} else if currentEyeAnim != nil && len(currentEyeAnim.Frames) > 1 {
				// Case 2: animated source, static target — wait for cycle end
				cycleLen := int64(len(currentEyeAnim.Frames))
				if eyeFrameCounter > 0 && eyeFrameCounter%cycleLen == 0 {
					// Cycle just completed: snapshot last displayed frame and start blend
					for i := 0; i < EyeFrameWidth*EyeFrameHeight; i++ {
						eyeTransitionFrom[i*3] = byte(eyeBuffer[i] >> 24)
						eyeTransitionFrom[i*3+1] = byte(eyeBuffer[i] >> 16)
						eyeTransitionFrom[i*3+2] = byte(eyeBuffer[i] >> 8)
					}
					eyeTransitionTarget = queuedEyeAnim
					eyeTransitionStep = 0
					eyeTransitioning = true
					queuedEyeAnim = nil
					if debugLog {
						fmt.Printf("[%d] [ANIM] Eye transition start (post-cycle): -> %s\n",
							Ts(), eyeTransitionTarget.Name)
					}
				}
				// Cycle not complete: keep queuedEyeAnim, revisit next frame
			} else {
				// Case 3: static source, static target — blend immediately
				if eyeTransitioning && eyeTransitionTarget != nil && queuedEyeAnim != eyeTransitionTarget {
					// Redirect mid-blend to new static target
					for i := 0; i < EyeFrameWidth*EyeFrameHeight; i++ {
						eyeTransitionFrom[i*3] = byte(eyeBuffer[i] >> 24)
						eyeTransitionFrom[i*3+1] = byte(eyeBuffer[i] >> 16)
						eyeTransitionFrom[i*3+2] = byte(eyeBuffer[i] >> 8)
					}
					eyeTransitionTarget = queuedEyeAnim
					eyeTransitionStep = 0
					if debugLog {
						fmt.Printf("[%d] [ANIM] Eye transition redirect -> %s\n", Ts(), queuedEyeAnim.Name)
					}
				} else if !eyeTransitioning && currentEyeAnim != nil &&
					len(currentEyeAnim.Frames) > 0 && queuedEyeAnim != currentEyeAnim {
					// Start new smooth blend
					frameIdx := eyeFrameCounter % int64(len(currentEyeAnim.Frames))
					copy(eyeTransitionFrom, currentEyeAnim.Frames[frameIdx])
					eyeTransitionTarget = queuedEyeAnim
					eyeTransitionStep = 0
					eyeTransitioning = true
					if debugLog {
						fmt.Printf("[%d] [ANIM] Eye transition start: %s -> %s\n",
							Ts(), currentEyeAnim.Name, queuedEyeAnim.Name)
					}
				}
				queuedEyeAnim = nil
			}
		}

		// Mouth transition trigger — same three-case logic as eye.
		if queuedMouthAnim != nil {
			if len(queuedMouthAnim.Frames) > 1 {
				// Case 1: animated target — immediate hard switch
				if queuedMouthAnim != currentMouthAnim {
					currentMouthAnim = queuedMouthAnim
					mouthFrameCounter = 0
					mouthTransitioning = false
					mouthTransitionTarget = nil
					if debugLog {
						fmt.Printf("[%d] [ANIM] Mouth switch (animated): %s\n", Ts(), queuedMouthAnim.Name)
					}
				}
				queuedMouthAnim = nil
			} else if currentMouthAnim != nil && len(currentMouthAnim.Frames) > 1 {
				// Case 2: animated source, static target — wait for cycle end
				cycleLen := int64(len(currentMouthAnim.Frames))
				if mouthFrameCounter > 0 && mouthFrameCounter%cycleLen == 0 {
					for i := 0; i < MouthFrameWidth*MouthFrameHeight; i++ {
						mouthTransitionFrom[i*3] = byte(mouthBuffer[i] >> 24)
						mouthTransitionFrom[i*3+1] = byte(mouthBuffer[i] >> 16)
						mouthTransitionFrom[i*3+2] = byte(mouthBuffer[i] >> 8)
					}
					mouthTransitionTarget = queuedMouthAnim
					mouthTransitionStep = 0
					mouthTransitioning = true
					queuedMouthAnim = nil
					if debugLog {
						fmt.Printf("[%d] [ANIM] Mouth transition start (post-cycle): -> %s\n",
							Ts(), mouthTransitionTarget.Name)
					}
				}
				// Cycle not complete: keep queuedMouthAnim
			} else {
				// Case 3: static source, static target — blend immediately
				if mouthTransitioning && mouthTransitionTarget != nil && queuedMouthAnim != mouthTransitionTarget {
					for i := 0; i < MouthFrameWidth*MouthFrameHeight; i++ {
						mouthTransitionFrom[i*3] = byte(mouthBuffer[i] >> 24)
						mouthTransitionFrom[i*3+1] = byte(mouthBuffer[i] >> 16)
						mouthTransitionFrom[i*3+2] = byte(mouthBuffer[i] >> 8)
					}
					mouthTransitionTarget = queuedMouthAnim
					mouthTransitionStep = 0
					if debugLog {
						fmt.Printf("[%d] [ANIM] Mouth transition redirect -> %s\n", Ts(), queuedMouthAnim.Name)
					}
				} else if !mouthTransitioning && currentMouthAnim != nil &&
					len(currentMouthAnim.Frames) > 0 && queuedMouthAnim != currentMouthAnim {
					frameIdx := mouthFrameCounter % int64(len(currentMouthAnim.Frames))
					copy(mouthTransitionFrom, currentMouthAnim.Frames[frameIdx])
					mouthTransitionTarget = queuedMouthAnim
					mouthTransitionStep = 0
					mouthTransitioning = true
					if debugLog {
						fmt.Printf("[%d] [ANIM] Mouth transition start: %s -> %s\n",
							Ts(), currentMouthAnim.Name, queuedMouthAnim.Name)
					}
				}
				queuedMouthAnim = nil
			}
		}

		// Eye rendering
		if eyeOK && currentEyeAnim != nil && len(currentEyeAnim.Frames) > 0 {
			if eyeTransitioning {
				// Render smoothstep-blended frame between "from" snapshot and target frame 0
				if eyeTransitionTarget != nil && len(eyeTransitionTarget.Frames) > 0 {
					toFrame := eyeTransitionTarget.Frames[0]
					if len(eyeTransitionFrom)/3 == EyeFrameWidth*EyeFrameHeight &&
						len(toFrame)/3 == EyeFrameWidth*EyeFrameHeight {
						lerpFrameInto(eyeBuffer, eyeTransitionFrom, toFrame,
							eyeTransitionStep, eyeTransitionFrames, EyeFrameWidth*EyeFrameHeight, brightnessPercent)
						strip1.WriteRaw(eyeBuffer)
					}
				}
				eyeTransitionStep++
				if eyeTransitionStep > eyeTransitionFrames {
					currentEyeAnim = eyeTransitionTarget
					eyeFrameCounter = 0
					eyeTransitioning = false
					eyeTransitionTarget = nil
					if debugLog {
						fmt.Printf("[%d] [ANIM] Eye transition complete -> %s\n",
							Ts(), currentEyeAnim.Name)
					}
				}
			} else {
				// Normal frame playback
				frameIdx := eyeFrameCounter % int64(len(currentEyeAnim.Frames))
				frameBytes := currentEyeAnim.Frames[frameIdx]
				if len(frameBytes)/3 == EyeFrameWidth*EyeFrameHeight {
					bytesToRawInto(eyeBuffer, frameBytes, brightnessPercent)
					strip1.WriteRaw(eyeBuffer)
				} else {
					fmt.Printf("[%d] [ANIM SKIP eye] pixels=%d expected=%d name=%s frame=%d\n",
						Ts(), len(frameBytes)/3, EyeFrameWidth*EyeFrameHeight, currentEyeAnim.Name, frameIdx)
				}
			}
		}

		// Mouth rendering
		if mouthOK && currentMouthAnim != nil && len(currentMouthAnim.Frames) > 0 {
			if mouthTransitioning {
				// Render smoothstep-blended frame between "from" snapshot and target frame 0
				if mouthTransitionTarget != nil && len(mouthTransitionTarget.Frames) > 0 {
					toFrame := mouthTransitionTarget.Frames[0]
					if len(mouthTransitionFrom)/3 == MouthFrameWidth*MouthFrameHeight &&
						len(toFrame)/3 == MouthFrameWidth*MouthFrameHeight {
						lerpFrameInto(mouthBuffer, mouthTransitionFrom, toFrame,
							mouthTransitionStep, mouthTransitionFrames, MouthFrameWidth*MouthFrameHeight, brightnessPercent)
						strip2.WriteRaw(mouthBuffer)
					}
				}
				mouthTransitionStep++
				if mouthTransitionStep > mouthTransitionFrames {
					currentMouthAnim = mouthTransitionTarget
					mouthFrameCounter = 0
					mouthTransitioning = false
					mouthTransitionTarget = nil
					if debugLog {
						fmt.Printf("[%d] [ANIM] Mouth transition complete -> %s\n",
							Ts(), currentMouthAnim.Name)
					}
				}
			} else {
				// Normal frame playback
				frameIdx := mouthFrameCounter % int64(len(currentMouthAnim.Frames))
				frameBytes := currentMouthAnim.Frames[frameIdx]
				if len(frameBytes)/3 == MouthFrameWidth*MouthFrameHeight {
					bytesToRawInto(mouthBuffer, frameBytes, brightnessPercent)
					strip2.WriteRaw(mouthBuffer)
				} else {
					fmt.Printf("[%d] [ANIM SKIP mouth] pixels=%d expected=%d name=%s frame=%d\n",
						Ts(), len(frameBytes)/3, MouthFrameWidth*MouthFrameHeight, currentMouthAnim.Name, frameIdx)
				}
			}
		}

		// PIO handles latch timing internally - no manual reset delay needed

		// Only advance frame counters when not transitioning; transition resets counter on completion
		if !eyeTransitioning {
			eyeFrameCounter++
		}
		if !mouthTransitioning {
			mouthFrameCounter++
		}

		// ~60Hz frame rate cap (10ms was too fast, produced redundant WriteRaw calls)
		time.Sleep(16 * time.Millisecond)
	}
}
