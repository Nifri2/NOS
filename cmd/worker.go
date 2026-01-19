package cmd

import (
	"fmt"
	"machine"
	"time"

	"tinygo.org/x/drivers/ws2812"
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

	var loopCounter int

	for {
		// Feed watchdog approx every second (1000 * 1ms sleep)
		loopCounter++
		if loopCounter > 1000 {
			machine.Watchdog.Update()
			loopCounter = 0
		}

		if uart.Buffered() > 0 {
			b, _ := uart.ReadByte()

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
					// Packet complete, verify checksum
					// Checksum = Addr + Cmd + Eye + Mouth
					// buf = [AA, Addr, Cmd, Eye, Mouth, Checksum]
					addrByte := buf[1]
					cmdByte := buf[2]
					eyeByte := buf[3]
					mouthByte := buf[4]
					checksumByte := buf[5]

					calculatedChecksum := addrByte + cmdByte + eyeByte + mouthByte

					if calculatedChecksum == checksumByte {
						// Valid packet
						fmt.Printf("Rx: Cmd=%x Eye=%x Mouth=%x\n", cmdByte, eyeByte, mouthByte)

						if Address(addrByte) == config.Address {
							cmd := Command(cmdByte)
							switch cmd {
							case Cmd_LedOn:
								led.High()
							case Cmd_LedOff:
								led.Low()
							case Cmd_NoOp:
								// NoOp
							case Cmd_DisplayAnim:
								eyeIdx := int(eyeByte)
								mouthIdx := int(mouthByte)
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

	// we need to use GPIO far away from UART pins to avoid chatter
	// we use GP18 for eye and GP16 for mouth
	ledPin1 := machine.GP18
	ledPin1.Configure(machine.PinConfig{Mode: machine.PinOutput})
	strip1 := ws2812.New(ledPin1)

	ledPin2 := machine.GP16
	ledPin2.Configure(machine.PinConfig{Mode: machine.PinOutput})
	strip2 := ws2812.New(ledPin2)

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
			_, err := strip1.Write(eyeFrame)
			if err != nil {
				// println(err.Error()) // Optional: reduce spam
			}
		}

		if currentMouthAnim != nil && len(currentMouthAnim.Frames) > 0 {
			mouthFrame := currentMouthAnim.Frames[mouthFrameCounter%int64(currentMouthAnim.FrameCount)]
			_, err := strip2.Write(mouthFrame)
			if err != nil {
				// println(err.Error())
			}
		}

		// Delay for the WS2812 reset pulse
		time.Sleep(300 * time.Microsecond)

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
