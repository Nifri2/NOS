package main

import (
	_ "embed"
	"fmt"
	"machine"
	"time"

	"nifri2/proto-dispatch/cmd"
)

// EMBED_START
//go:embed animations/eye_idle.animbyte
var eyeIdleData []byte

//go:embed animations/eye_blink.animbyte
var eyeBlinkData []byte

//go:embed animations/mouth_idle_left.animbyte
var mouthIdleLeftData []byte

//go:embed animations/mouth_idle_right.animbyte
var mouthIdleRightData []byte

// EMBED_END

// buildRole and buildAddress are set at compile time via -ldflags
// e.g. -ldflags="-X main.buildRole=worker -X main.buildAddress=worker-0"
var (
	buildRole    string
	buildAddress string
)

var config = cmd.Settings{
	Role:    cmd.ParseRole(buildRole),
	Address: cmd.ParseAddress(buildAddress),
}

func main() {

	var uart *machine.UART = machine.UART0

	// correct baud: 38400
	// keep it 38400 otherwise the WS2812 timing gets messed up
	uart.Configure(machine.UARTConfig{
		BaudRate: 38400,
		TX:       machine.GP0,
		RX:       machine.GP1,
	})

	led := machine.LED
	led.Configure(machine.PinConfig{Mode: machine.PinOutput})

	// Load animations
// LOAD_START
	eyeIdleAnim, err := cmd.LoadAnimation(eyeIdleData, cmd.EyeFrameWidth, cmd.EyeFrameHeight, "eye_idle")
	if err != nil {
		fmt.Println("Error loading eye_idle:", err)
	}

	eyeBlinkAnim, err := cmd.LoadAnimation(eyeBlinkData, cmd.EyeFrameWidth, cmd.EyeFrameHeight, "eye_blink")
	if err != nil {
		fmt.Println("Error loading eye_blink:", err)
	}

	mouthIdleLeftAnim, err := cmd.LoadAnimation(mouthIdleLeftData, cmd.MouthFrameWidth, cmd.MouthFrameHeight, "mouth_idle_left")
	if err != nil {
		fmt.Println("Error loading mouth_idle_left:", err)
	}

	mouthIdleRightAnim, err := cmd.LoadAnimation(mouthIdleRightData, cmd.MouthFrameWidth, cmd.MouthFrameHeight, "mouth_idle_right")
	if err != nil {
		fmt.Println("Error loading mouth_idle_right:", err)
	}

	// LOAD_END

	// Populate global array in cmd package
// APPEND_START
	cmd.LoadedAnimations = nil
	if eyeIdleAnim != nil {
		cmd.LoadedAnimations = append(cmd.LoadedAnimations, eyeIdleAnim)
	}
	if eyeBlinkAnim != nil {
		cmd.LoadedAnimations = append(cmd.LoadedAnimations, eyeBlinkAnim)
	}
	if mouthIdleLeftAnim != nil {
		cmd.LoadedAnimations = append(cmd.LoadedAnimations, mouthIdleLeftAnim)
	}
	if mouthIdleRightAnim != nil {
		cmd.LoadedAnimations = append(cmd.LoadedAnimations, mouthIdleRightAnim)
	}
	// APPEND_END

	// blink LED based on role, 2 times, 200ms interval for Dispatcher, 5 times 200ms for Worker

	switch config.Role {
	case cmd.Dispatcher:
		for i := 0; i < 2; i++ {
			led.High()
			time.Sleep(200 * time.Millisecond)
			led.Low()
			time.Sleep(200 * time.Millisecond)
		}
	case cmd.Worker:
		for i := 0; i < 5; i++ {
			led.High()
			time.Sleep(40 * time.Millisecond)
			led.Low()
			time.Sleep(40 * time.Millisecond)
		}
	}

	// Main loop

	switch config.Role {
	case cmd.Dispatcher:
		cmd.RunDispatcher(config, uart, led)

	case cmd.Worker:
		cmd.RunWorker(config, uart, led)
	}

}
