package cmd

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
)

type AnimationID int

// ANIMID_START
const (
	Anim_EyeIdle AnimationID = 0x00
	Anim_EyeBlink AnimationID = 0x01
	Anim_MouthIdleLeft AnimationID = 0x02
	Anim_MouthIdleRight AnimationID = 0x03
	Anim_MouthIdle AnimationID = 0x10
)

// ANIMID_END

// MAPPING_START
var animationMapping = map[Address]map[AnimationID]AnimationID{
	Worker_0: { // Left side
		Anim_MouthIdle: Anim_MouthIdleLeft,
	},
	Worker_1: { // Right side
		Anim_MouthIdle: Anim_MouthIdleRight,
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
// Checksum = Address + Command + AnimID_Eye + AnimID_Mouth

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
