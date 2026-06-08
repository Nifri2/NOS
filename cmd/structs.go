package cmd

import "time"

var bootTime = time.Now()

// Ts returns milliseconds since boot for log timestamps
func Ts() int64 { return time.Since(bootTime).Milliseconds() }

type Role int

const (
	Dispatcher Role = 0x00 + iota
	Worker
)

type Address int

const (
	Dispatch Address = 0x00 + iota
	Worker_0         // 0x01
	Worker_1         // 0x02
	Worker_2         // 0x03
	Worker_3         // 0x04
)

// Broadcast address - all workers accept packets with this address
const Address_All Address = 0xFF

type Command int

const (
	Cmd_NoOp        Command = 0x00 + iota
	Cmd_LedOn               // 0x01
	Cmd_LedOff              // 0x02
	Cmd_DisplayAnim         // 0x03
	Cmd_Ping                // 0x04
	Cmd_Reboot              // 0x05
	Cmd_DayMode             // 0x06
	Cmd_NightMode           // 0x07
)

// DayModeBrightnessPercent is the runtime RGB multiplier applied by workers
// when day mode is active. 100 = compiled brightness unchanged; values >100
// scale GRB bytes up (clamped to 255 per channel) before they reach the LEDs.
// Tweak this to dial day-mode brightness.
const DayModeBrightnessPercent = 110

type AnimationID int

// ANIMID_START
const (
	Anim_EyeIdle AnimationID = 0x00
	Anim_EyeBlink AnimationID = 0x01
	Anim_MouthIdleLeft AnimationID = 0x02
	Anim_MouthIdleRight AnimationID = 0x03
	Anim_EyeHappy AnimationID = 0x04
	Anim_MouthYap1Left AnimationID = 0x05
	Anim_MouthYap1Right AnimationID = 0x06
	Anim_MouthYap2Left AnimationID = 0x07
	Anim_MouthYap2Right AnimationID = 0x08
	Anim_MouthYap3Left AnimationID = 0x09
	Anim_MouthYap3Right AnimationID = 0x0A
	Anim_EyeExcitedLeft AnimationID = 0x0B
	Anim_EyeExcitedRight AnimationID = 0x0C
	Anim_EyeFlushed AnimationID = 0x0D
	Anim_EyeStare AnimationID = 0x0E
	Anim_MouthFlushedLeft AnimationID = 0x0F
	Anim_MouthFlushedRight AnimationID = 0x10
	Anim_MouthStareLeft AnimationID = 0x11
	Anim_MouthStareRight AnimationID = 0x12
	Anim_MouthIdle AnimationID = 0x13
	Anim_MouthYap1 AnimationID = 0x14
	Anim_MouthYap2 AnimationID = 0x15
	Anim_MouthYap3 AnimationID = 0x16
	Anim_EyeExcited AnimationID = 0x17
	Anim_MouthFlushed AnimationID = 0x18
	Anim_MouthStare AnimationID = 0x19
)

// ANIMID_END

// MAPPING_START
var animationMapping = map[Address]map[AnimationID]AnimationID{
	Worker_0: { // Left side
		Anim_MouthIdle: Anim_MouthIdleLeft,
		Anim_MouthYap1: Anim_MouthYap1Left,
		Anim_MouthYap2: Anim_MouthYap2Left,
		Anim_MouthYap3: Anim_MouthYap3Left,
		Anim_EyeExcited: Anim_EyeExcitedLeft,
		Anim_MouthFlushed: Anim_MouthFlushedLeft,
		Anim_MouthStare: Anim_MouthStareLeft,
	},
	Worker_1: { // Right side
		Anim_MouthIdle: Anim_MouthIdleRight,
		Anim_MouthYap1: Anim_MouthYap1Right,
		Anim_MouthYap2: Anim_MouthYap2Right,
		Anim_MouthYap3: Anim_MouthYap3Right,
		Anim_EyeExcited: Anim_EyeExcitedRight,
		Anim_MouthFlushed: Anim_MouthFlushedRight,
		Anim_MouthStare: Anim_MouthStareRight,
	},
}

// MapAnimation translates logical animation IDs to side-specific variants
func MapAnimation(addr Address, id AnimationID) int {
	if mapping, ok := animationMapping[addr]; ok {
		if mapped, ok := mapping[id]; ok {
			return int(mapped)
		}
	}
	return int(id)
}

// MAPPING_END

// Complete Protocol Packet:
// [Header(0xAA), Address, Command, AnimID_Eye, AnimID_Mouth, Checksum]
// Checksum = CRC-8/MAXIM of [Address, Command, AnimID_Eye, AnimID_Mouth]
// Commands: NoOp=0x00 LedOn=0x01 LedOff=0x02 DisplayAnim=0x03 Ping=0x04 Reboot=0x05 DayMode=0x06 NightMode=0x07

type Settings struct {
	Role    Role
	Address Address
}

type Animation struct {
	Frames     [][]byte // slice of frames, each frame is []byte
	FrameCount int
	Name       string
}

var LoadedAnimations []*Animation

const (
	EyeFrameWidth  = 16
	EyeFrameHeight = 16

	MouthFrameWidth  = 32
	MouthFrameHeight = 16
)

type animUpdate struct {
	Eye   *Animation
	Mouth *Animation
}
