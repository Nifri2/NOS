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

//go:embed animations/eye_happy.animbyte
var eyeHappyData []byte

//go:embed animations/mouth_yap_1_left.animbyte
var mouthYap1LeftData []byte

//go:embed animations/mouth_yap_1_right.animbyte
var mouthYap1RightData []byte

//go:embed animations/mouth_yap_2_left.animbyte
var mouthYap2LeftData []byte

//go:embed animations/mouth_yap_2_right.animbyte
var mouthYap2RightData []byte

//go:embed animations/mouth_yap_3_left.animbyte
var mouthYap3LeftData []byte

//go:embed animations/mouth_yap_3_right.animbyte
var mouthYap3RightData []byte

//go:embed animations/eye_excited_left.animbyte
var eyeExcitedLeftData []byte

//go:embed animations/eye_excited_right.animbyte
var eyeExcitedRightData []byte

//go:embed animations/eye_flushed.animbyte
var eyeFlushedData []byte

//go:embed animations/eye_stare.animbyte
var eyeStareData []byte

//go:embed animations/mouth_flushed_left.animbyte
var mouthFlushedLeftData []byte

//go:embed animations/mouth_flushed_right.animbyte
var mouthFlushedRightData []byte

//go:embed animations/mouth_stare_left.animbyte
var mouthStareLeftData []byte

//go:embed animations/mouth_stare_right.animbyte
var mouthStareRightData []byte

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

	eyeHappyAnim, err := cmd.LoadAnimation(eyeHappyData, cmd.EyeFrameWidth, cmd.EyeFrameHeight, "eye_happy")
	if err != nil {
		fmt.Println("Error loading eye_happy:", err)
	}

	mouthYap1LeftAnim, err := cmd.LoadAnimation(mouthYap1LeftData, cmd.MouthFrameWidth, cmd.MouthFrameHeight, "mouth_yap_1_left")
	if err != nil {
		fmt.Println("Error loading mouth_yap_1_left:", err)
	}

	mouthYap1RightAnim, err := cmd.LoadAnimation(mouthYap1RightData, cmd.MouthFrameWidth, cmd.MouthFrameHeight, "mouth_yap_1_right")
	if err != nil {
		fmt.Println("Error loading mouth_yap_1_right:", err)
	}

	mouthYap2LeftAnim, err := cmd.LoadAnimation(mouthYap2LeftData, cmd.MouthFrameWidth, cmd.MouthFrameHeight, "mouth_yap_2_left")
	if err != nil {
		fmt.Println("Error loading mouth_yap_2_left:", err)
	}

	mouthYap2RightAnim, err := cmd.LoadAnimation(mouthYap2RightData, cmd.MouthFrameWidth, cmd.MouthFrameHeight, "mouth_yap_2_right")
	if err != nil {
		fmt.Println("Error loading mouth_yap_2_right:", err)
	}

	mouthYap3LeftAnim, err := cmd.LoadAnimation(mouthYap3LeftData, cmd.MouthFrameWidth, cmd.MouthFrameHeight, "mouth_yap_3_left")
	if err != nil {
		fmt.Println("Error loading mouth_yap_3_left:", err)
	}

	mouthYap3RightAnim, err := cmd.LoadAnimation(mouthYap3RightData, cmd.MouthFrameWidth, cmd.MouthFrameHeight, "mouth_yap_3_right")
	if err != nil {
		fmt.Println("Error loading mouth_yap_3_right:", err)
	}

	eyeExcitedLeftAnim, err := cmd.LoadAnimation(eyeExcitedLeftData, cmd.EyeFrameWidth, cmd.EyeFrameHeight, "eye_excited_left")
	if err != nil {
		fmt.Println("Error loading eye_excited_left:", err)
	}

	eyeExcitedRightAnim, err := cmd.LoadAnimation(eyeExcitedRightData, cmd.EyeFrameWidth, cmd.EyeFrameHeight, "eye_excited_right")
	if err != nil {
		fmt.Println("Error loading eye_excited_right:", err)
	}

	eyeFlushedAnim, err := cmd.LoadAnimation(eyeFlushedData, cmd.EyeFrameWidth, cmd.EyeFrameHeight, "eye_flushed")
	if err != nil {
		fmt.Println("Error loading eye_flushed:", err)
	}

	eyeStareAnim, err := cmd.LoadAnimation(eyeStareData, cmd.EyeFrameWidth, cmd.EyeFrameHeight, "eye_stare")
	if err != nil {
		fmt.Println("Error loading eye_stare:", err)
	}

	mouthFlushedLeftAnim, err := cmd.LoadAnimation(mouthFlushedLeftData, cmd.MouthFrameWidth, cmd.MouthFrameHeight, "mouth_flushed_left")
	if err != nil {
		fmt.Println("Error loading mouth_flushed_left:", err)
	}

	mouthFlushedRightAnim, err := cmd.LoadAnimation(mouthFlushedRightData, cmd.MouthFrameWidth, cmd.MouthFrameHeight, "mouth_flushed_right")
	if err != nil {
		fmt.Println("Error loading mouth_flushed_right:", err)
	}

	mouthStareLeftAnim, err := cmd.LoadAnimation(mouthStareLeftData, cmd.MouthFrameWidth, cmd.MouthFrameHeight, "mouth_stare_left")
	if err != nil {
		fmt.Println("Error loading mouth_stare_left:", err)
	}

	mouthStareRightAnim, err := cmd.LoadAnimation(mouthStareRightData, cmd.MouthFrameWidth, cmd.MouthFrameHeight, "mouth_stare_right")
	if err != nil {
		fmt.Println("Error loading mouth_stare_right:", err)
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
	if eyeHappyAnim != nil {
		cmd.LoadedAnimations = append(cmd.LoadedAnimations, eyeHappyAnim)
	}
	if mouthYap1LeftAnim != nil {
		cmd.LoadedAnimations = append(cmd.LoadedAnimations, mouthYap1LeftAnim)
	}
	if mouthYap1RightAnim != nil {
		cmd.LoadedAnimations = append(cmd.LoadedAnimations, mouthYap1RightAnim)
	}
	if mouthYap2LeftAnim != nil {
		cmd.LoadedAnimations = append(cmd.LoadedAnimations, mouthYap2LeftAnim)
	}
	if mouthYap2RightAnim != nil {
		cmd.LoadedAnimations = append(cmd.LoadedAnimations, mouthYap2RightAnim)
	}
	if mouthYap3LeftAnim != nil {
		cmd.LoadedAnimations = append(cmd.LoadedAnimations, mouthYap3LeftAnim)
	}
	if mouthYap3RightAnim != nil {
		cmd.LoadedAnimations = append(cmd.LoadedAnimations, mouthYap3RightAnim)
	}
	if eyeExcitedLeftAnim != nil {
		cmd.LoadedAnimations = append(cmd.LoadedAnimations, eyeExcitedLeftAnim)
	}
	if eyeExcitedRightAnim != nil {
		cmd.LoadedAnimations = append(cmd.LoadedAnimations, eyeExcitedRightAnim)
	}
	if eyeFlushedAnim != nil {
		cmd.LoadedAnimations = append(cmd.LoadedAnimations, eyeFlushedAnim)
	}
	if eyeStareAnim != nil {
		cmd.LoadedAnimations = append(cmd.LoadedAnimations, eyeStareAnim)
	}
	if mouthFlushedLeftAnim != nil {
		cmd.LoadedAnimations = append(cmd.LoadedAnimations, mouthFlushedLeftAnim)
	}
	if mouthFlushedRightAnim != nil {
		cmd.LoadedAnimations = append(cmd.LoadedAnimations, mouthFlushedRightAnim)
	}
	if mouthStareLeftAnim != nil {
		cmd.LoadedAnimations = append(cmd.LoadedAnimations, mouthStareLeftAnim)
	}
	if mouthStareRightAnim != nil {
		cmd.LoadedAnimations = append(cmd.LoadedAnimations, mouthStareRightAnim)
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
