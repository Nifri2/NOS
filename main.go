package main

import (
	_ "embed"
	"fmt"
	"machine"
	"time"

	"nifri2/proto-dispatch/cmd"
)

//go:embed animations/eye_blink.animbyte
var eyeBlinkData []byte

//go:embed animations/eye_idle.animbyte
var eyeIdleData []byte

//go:embed animations/mouth_idle.animbyte
var mouthIdleData []byte

//go:embed animations/nifri.animbyte
var nifriData []byte

//go:embed animations/spinnylambda.animbyte
var spinnylambdaData []byte

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
	eyeBlinkAnim, err := cmd.LoadAnimation(eyeBlinkData, cmd.EyeFrameWidth, cmd.EyeFrameHeight, "eye_blink")
	if err != nil {
		fmt.Println("Error loading eye_blink:", err)
	}

	eyeIdleAnim, err := cmd.LoadAnimation(eyeIdleData, cmd.EyeFrameWidth, cmd.EyeFrameHeight, "eye_idle")
	if err != nil {
		fmt.Println("Error loading eye_idle:", err)
	}

	mouthAnim, err := cmd.LoadAnimation(mouthIdleData, cmd.MouthFrameWidth, cmd.MouthFrameHeight, "mouth_idle")
	if err != nil {
		fmt.Println("Error loading mouth_idle:", err)
	}

	nifriAnim, err := cmd.LoadAnimation(nifriData, cmd.EyeFrameWidth, cmd.EyeFrameHeight, "nifri")
	if err != nil {
		fmt.Println("Error loading nifri:", err)
	}

	spinnyAnim, err := cmd.LoadAnimation(spinnylambdaData, cmd.EyeFrameWidth, cmd.EyeFrameHeight, "spinnylambda")
	if err != nil {
		fmt.Println("Error loading spinnylambda:", err)
	}

	// Populate global array in cmd package
	cmd.LoadedAnimations = nil
	if eyeIdleAnim != nil {
		cmd.LoadedAnimations = append(cmd.LoadedAnimations, eyeIdleAnim)
	}
	if eyeBlinkAnim != nil {
		cmd.LoadedAnimations = append(cmd.LoadedAnimations, eyeBlinkAnim)
	}
	if mouthAnim != nil {
		cmd.LoadedAnimations = append(cmd.LoadedAnimations, mouthAnim)
	}
	if nifriAnim != nil {
		cmd.LoadedAnimations = append(cmd.LoadedAnimations, nifriAnim)
	}
	if spinnyAnim != nil {
		cmd.LoadedAnimations = append(cmd.LoadedAnimations, spinnyAnim)
	}

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
