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

//go:embed animations/boot_eye_left.animbyte
var bootEyeLeftData []byte

//go:embed animations/boot_eye_right.animbyte
var bootEyeRightData []byte

//go:embed animations/boot_mouth_left.animbyte
var bootMouthLeftData []byte

//go:embed animations/boot_mouth_right.animbyte
var bootMouthRightData []byte

//go:embed animations/reboot_eye_left.animbyte
var rebootEyeLeftData []byte

//go:embed animations/reboot_eye_right.animbyte
var rebootEyeRightData []byte

//go:embed animations/reboot_mouth_left.animbyte
var rebootMouthLeftData []byte

//go:embed animations/reboot_mouth_right.animbyte
var rebootMouthRightData []byte

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

	bootEyeLeftAnim, err := cmd.LoadAnimation(bootEyeLeftData, cmd.EyeFrameWidth, cmd.EyeFrameHeight, "boot_eye_left")
	if err != nil {
		fmt.Println("Error loading boot_eye_left:", err)
	}

	bootEyeRightAnim, err := cmd.LoadAnimation(bootEyeRightData, cmd.EyeFrameWidth, cmd.EyeFrameHeight, "boot_eye_right")
	if err != nil {
		fmt.Println("Error loading boot_eye_right:", err)
	}

	bootMouthLeftAnim, err := cmd.LoadAnimation(bootMouthLeftData, cmd.MouthFrameWidth, cmd.MouthFrameHeight, "boot_mouth_left")
	if err != nil {
		fmt.Println("Error loading boot_mouth_left:", err)
	}

	bootMouthRightAnim, err := cmd.LoadAnimation(bootMouthRightData, cmd.MouthFrameWidth, cmd.MouthFrameHeight, "boot_mouth_right")
	if err != nil {
		fmt.Println("Error loading boot_mouth_right:", err)
	}

	rebootEyeLeftAnim, err := cmd.LoadAnimation(rebootEyeLeftData, cmd.EyeFrameWidth, cmd.EyeFrameHeight, "reboot_eye_left")
	if err != nil {
		fmt.Println("Error loading reboot_eye_left:", err)
	}

	rebootEyeRightAnim, err := cmd.LoadAnimation(rebootEyeRightData, cmd.EyeFrameWidth, cmd.EyeFrameHeight, "reboot_eye_right")
	if err != nil {
		fmt.Println("Error loading reboot_eye_right:", err)
	}

	rebootMouthLeftAnim, err := cmd.LoadAnimation(rebootMouthLeftData, cmd.MouthFrameWidth, cmd.MouthFrameHeight, "reboot_mouth_left")
	if err != nil {
		fmt.Println("Error loading reboot_mouth_left:", err)
	}

	rebootMouthRightAnim, err := cmd.LoadAnimation(rebootMouthRightData, cmd.MouthFrameWidth, cmd.MouthFrameHeight, "reboot_mouth_right")
	if err != nil {
		fmt.Println("Error loading reboot_mouth_right:", err)
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
	if bootEyeLeftAnim != nil {
		cmd.LoadedAnimations = append(cmd.LoadedAnimations, bootEyeLeftAnim)
	}
	if bootEyeRightAnim != nil {
		cmd.LoadedAnimations = append(cmd.LoadedAnimations, bootEyeRightAnim)
	}
	if bootMouthLeftAnim != nil {
		cmd.LoadedAnimations = append(cmd.LoadedAnimations, bootMouthLeftAnim)
	}
	if bootMouthRightAnim != nil {
		cmd.LoadedAnimations = append(cmd.LoadedAnimations, bootMouthRightAnim)
	}
	if rebootEyeLeftAnim != nil {
		cmd.LoadedAnimations = append(cmd.LoadedAnimations, rebootEyeLeftAnim)
	}
	if rebootEyeRightAnim != nil {
		cmd.LoadedAnimations = append(cmd.LoadedAnimations, rebootEyeRightAnim)
	}
	if rebootMouthLeftAnim != nil {
		cmd.LoadedAnimations = append(cmd.LoadedAnimations, rebootMouthLeftAnim)
	}
	if rebootMouthRightAnim != nil {
		cmd.LoadedAnimations = append(cmd.LoadedAnimations, rebootMouthRightAnim)
	}
	// APPEND_END

	// Blink LED based on role - visible "I am alive" indicator BEFORE serial
	// Phase 1: Quick role identification blinks
	switch config.Role {
	case cmd.Dispatcher:
		// Fast boot - no patterns, dispatcher resets need to be invisible
	case cmd.Worker:
		// 5 fast blinks for worker
		for i := 0; i < 5; i++ {
			led.High()
			time.Sleep(40 * time.Millisecond)
			led.Low()
			time.Sleep(40 * time.Millisecond)
		}
	}

	// Phase 2: Role-specific pattern to confirm main() reached role switch
	switch config.Role {
	case cmd.Dispatcher:
		// Fast boot - no patterns, dispatcher resets need to be invisible
	case cmd.Worker:
		// 3 long blinks (500ms on / 500ms off)
		for i := 0; i < 3; i++ {
			led.High()
			time.Sleep(500 * time.Millisecond)
			led.Low()
			time.Sleep(500 * time.Millisecond)
		}

		// USB warmup delay - let USB-CDC enumerate before serial output
		time.Sleep(2 * time.Second)
		fmt.Printf("[%d] BOOT main: role=worker addr=%s\n", cmd.Ts(), buildAddress)
		time.Sleep(500 * time.Millisecond) // let terminal attach
	}

	// Main loop
	switch config.Role {
	case cmd.Dispatcher:
		cmd.RunDispatcher(config, uart, led)

	case cmd.Worker:
		cmd.RunWorker(config, uart, led)
	}

}
