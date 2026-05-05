package cmd

import (
	"fmt"
	"machine"
	"time"

	pio "github.com/tinygo-org/pio/rp2-pio"
	"github.com/tinygo-org/pio/rp2-pio/piolib"
)

// Each MCU takes care of 1 mouth and 1 eye WS2812 strip
// There will be a sperate worker that will only display Insignia animations

func RunWorker(config Settings, uart *machine.UART, led machine.Pin) {
	// Listen for commands from Dispatcher
	fmt.Println("Starting Worker Loop")

	// Configure Watchdog (5s timeout)
	machine.Watchdog.Configure(machine.WatchdogConfig{TimeoutMillis: 5000})

	animChan := make(chan animUpdate, 1)

	// Start animation routine in background
	go displayAnimation(animChan)

	const (
		HeaderByte = 0xAA
		PacketSize = 6
	)

	// Buffer to hold incoming packet
	// [Header, Addr, Cmd, Eye, Mouth, Checksum]
	buf := make([]byte, PacketSize)
	bufIdx := 0
	lastByteTime := time.Now()

	var loopCounter int

	for {
		// Feed watchdog approx every second (1000 * 1ms sleep)
		loopCounter++
		if loopCounter > 1000 {
			machine.Watchdog.Update()
			loopCounter = 0
		}

		// Inter-byte timeout: reset buffer if >20ms since last byte
		if bufIdx > 0 && time.Since(lastByteTime) > 20*time.Millisecond {
			bufIdx = 0
		}

		if uart.Buffered() > 0 {
			b, _ := uart.ReadByte()
			lastByteTime = time.Now()

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
						fmt.Printf("Rx: Cmd=%x Eye=%x Mouth=%x\n", cmdByte, eyeByte, mouthByte)

						// Accept if addressed to us or broadcast
						if Address(addrByte) == config.Address || Address(addrByte) == Address_All {
							cmd := Command(cmdByte)
							switch cmd {
							case Cmd_LedOn:
								led.High()
							case Cmd_LedOff:
								led.Low()
							case Cmd_NoOp:
								// NoOp
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
									animChan <- update
								}
							}
						}
					} else {
						fmt.Printf("Checksum mismatch: calc %x != recv %x\n", calculatedChecksum, checksumByte)
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

// TODOS:
// - Implement some sort of Queue for animation updates? The animations abruptly change now and it doesnt look great

func displayAnimation(animChan chan animUpdate) {
	// Wait for the board to stabilize
	time.Sleep(2 * time.Second)

	// Defaults if loading failed (or empty)
	// We assume LoadedAnimations is populated by main before calling RunWorker

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

	// PIO-based WS2812 - immune to UART interrupts
	// Eye strip: PIO0 SM0, GP18
	sm0, err := pio.PIO0.ClaimStateMachine()
	if err != nil {
		println("Failed to claim SM0:", err.Error())
		return
	}
	strip1, err := piolib.NewWS2812B(sm0, machine.GP18)
	if err != nil {
		println("Failed to init eye strip:", err.Error())
		return
	}
	strip1.EnableDMA(true)

	// Mouth strip: PIO0 SM1, GP12
	sm1, err := pio.PIO0.ClaimStateMachine()
	if err != nil {
		println("Failed to claim SM1:", err.Error())
		return
	}
	strip2, err := piolib.NewWS2812B(sm1, machine.GP12)
	if err != nil {
		println("Failed to init mouth strip:", err.Error())
		return
	}
	strip2.EnableDMA(true)

	// Boop Sensor will be connected on GP15

	var currentEyeAnim *Animation
	var currentMouthAnim *Animation
	var queuedEyeAnim *Animation = nil
	var queuedMouthAnim *Animation = nil

	// Initial setup for currentEyeAnim
	if eyeIdleAnim == nil {
		if len(LoadedAnimations) > 0 {
			currentEyeAnim = LoadedAnimations[0]
		} else {
			currentEyeAnim = &Animation{FrameCount: 1, Frames: [][]byte{{}}} // Dummy
		}
	} else {
		currentEyeAnim = eyeIdleAnim
	}

	// Initial setup for currentMouthAnim
	if mouthAnim == nil {
		currentMouthAnim = &Animation{FrameCount: 1, Frames: [][]byte{{}}}
	} else {
		currentMouthAnim = mouthAnim
	}

	var eyeFrameCounter int64
	var mouthFrameCounter int64

	for {
		// Check for new animation command
		select {
		case update := <-animChan:
			if update.Eye != nil {
				queuedEyeAnim = update.Eye
			}
			if update.Mouth != nil {
				queuedMouthAnim = update.Mouth
			}
		default:
		}

		// Safety check for nil animations if load failed
		if currentEyeAnim != nil && len(currentEyeAnim.Frames) > 0 {
			eyeFrame := currentEyeAnim.Frames[eyeFrameCounter%int64(currentEyeAnim.FrameCount)]
			strip1.WriteRaw(bytesToRaw(eyeFrame))
		}

		if currentMouthAnim != nil && len(currentMouthAnim.Frames) > 0 {
			mouthFrame := currentMouthAnim.Frames[mouthFrameCounter%int64(currentMouthAnim.FrameCount)]
			strip2.WriteRaw(bytesToRaw(mouthFrame))
		}

		// PIO handles latch timing internally - no manual reset delay needed

		eyeFrameCounter++
		mouthFrameCounter++

		// Transition at end of cycle
		if queuedEyeAnim != nil && currentEyeAnim != nil && eyeFrameCounter > 0 && (eyeFrameCounter%int64(currentEyeAnim.FrameCount) == 0) {
			if queuedEyeAnim != currentEyeAnim {
				fmt.Printf("Transitioning Eye to: %s\n", queuedEyeAnim.Name)
				currentEyeAnim = queuedEyeAnim
				eyeFrameCounter = 0
			}
			queuedEyeAnim = nil
		}

		if queuedMouthAnim != nil && currentMouthAnim != nil && mouthFrameCounter > 0 && (mouthFrameCounter%int64(currentMouthAnim.FrameCount) == 0) {
			if queuedMouthAnim != currentMouthAnim {
				fmt.Printf("Transitioning Mouth to: %s\n", queuedMouthAnim.Name)
				currentMouthAnim = queuedMouthAnim
				mouthFrameCounter = 0
			}
			queuedMouthAnim = nil
		}

		// Yield to allow other goroutines (like UART) to run if needed
		time.Sleep(10 * time.Millisecond)
	}
}
