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
)

type AnimationID int

// ANIMID_START
const (
	Anim_EyeIdle AnimationID = 0x00
	Anim_EyeBlink AnimationID = 0x01
	Anim_MouthIdleLeft AnimationID = 0x02
	Anim_MouthIdleRight AnimationID = 0x03
	Anim_MouthYap1Left AnimationID = 0x04
	Anim_MouthYap1Right AnimationID = 0x05
	Anim_MouthYap2Left AnimationID = 0x06
	Anim_MouthYap2Right AnimationID = 0x07
	Anim_MouthYap3Left AnimationID = 0x08
	Anim_MouthYap3Right AnimationID = 0x09
	Anim_MouthIdle AnimationID = 0x10
	Anim_MouthYap1 AnimationID = 0x11
	Anim_MouthYap2 AnimationID = 0x12
	Anim_MouthYap3 AnimationID = 0x13
)

// ANIMID_END

// Mouth open states for live mic mode (logical IDs, animations pending)
const (
	Anim_MouthOpen1 AnimationID = 0x11
	Anim_MouthOpen2 AnimationID = 0x12
	Anim_MouthOpen3 AnimationID = 0x13
)

// MAPPING_START
var animationMapping = map[Address]map[AnimationID]AnimationID{
	Worker_0: { // Left side
		Anim_MouthIdle: Anim_MouthIdleLeft,
		Anim_MouthYap1: Anim_MouthYap1Left,
		Anim_MouthYap2: Anim_MouthYap2Left,
		Anim_MouthYap3: Anim_MouthYap3Left,
	},
	Worker_1: { // Right side
		Anim_MouthIdle: Anim_MouthIdleRight,
		Anim_MouthYap1: Anim_MouthYap1Right,
		Anim_MouthYap2: Anim_MouthYap2Right,
		Anim_MouthYap3: Anim_MouthYap3Right,
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
// Commands: NoOp=0x00 LedOn=0x01 LedOff=0x02 DisplayAnim=0x03 Ping=0x04 Reboot=0x05

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
