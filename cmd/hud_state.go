package cmd

import "strconv"

// hudState is everything the HUD reconstructs from the bus.
type hudState struct {
	eyeAnim   AnimationID
	mouthAnim AnimationID
	dayMode   bool
	battDeci  uint8 // pack voltage in deci-volts (74 = 7.4V)
	battPct   uint8 // charge percent 0-100
	rxPackets uint32
	crcFails  uint32
}

// displayChanged reports whether anything the HUD actually renders differs.
// rxPackets/crcFails deliberately don't count: they tick every packet and would
// force a redraw on every frame.
func displayChanged(a, b *hudState) bool {
	return a.eyeAnim != b.eyeAnim ||
		a.mouthAnim != b.mouthAnim ||
		a.dayMode != b.dayMode ||
		a.battDeci != b.battDeci ||
		a.battPct != b.battPct
}

// hudApplyPacket validates the CRC of a 4-byte payload and, on match, applies
// its effect to st. Returns true if the packet was accepted. Mutates st in
// place; rxPackets ticks on accept, crcFails ticks on reject. Pure-Go (no
// hardware deps) so unit tests can exercise every command without a board.
func hudApplyPacket(st *hudState, addr, cmdByte, eye, mouth, checksum byte) bool {
	if Crc8Bytes4(addr, cmdByte, eye, mouth) != checksum {
		st.crcFails++
		return false
	}
	st.rxPackets++
	switch Command(cmdByte) {
	case Cmd_DisplayAnim:
		st.eyeAnim = AnimationID(eye)
		st.mouthAnim = AnimationID(mouth)
	case Cmd_DayMode:
		st.dayMode = true
	case Cmd_NightMode:
		st.dayMode = false
	case Cmd_Battery:
		st.battDeci = eye
		st.battPct = mouth
	}
	return true
}

// formatVolts turns deci-volts into e.g. "7.4V" without float formatting.
func formatVolts(deci uint8) string {
	return strconv.Itoa(int(deci)/10) + "." + strconv.Itoa(int(deci)%10) + "V"
}

// animName maps an animation ID to a short label for the HUD. The dispatcher
// broadcasts logical IDs; the side-specific variants are included for safety.
func animName(id AnimationID) string {
	switch id {
	case Anim_EyeIdle:
		return "Idle"
	case Anim_EyeBlink:
		return "Blink"
	case Anim_EyeHappy:
		return "Happy"
	case Anim_EyeExcited, Anim_EyeExcitedLeft, Anim_EyeExcitedRight:
		return "Excited"
	case Anim_EyeStare, Anim_MouthStare, Anim_MouthStareLeft, Anim_MouthStareRight:
		return "Stare"
	case Anim_EyeFlushed, Anim_MouthFlushed, Anim_MouthFlushedLeft, Anim_MouthFlushedRight:
		return "Flushed"
	case Anim_MouthIdle, Anim_MouthIdleLeft, Anim_MouthIdleRight:
		return "Idle"
	case Anim_MouthYap1, Anim_MouthYap1Left, Anim_MouthYap1Right:
		return "Yap1"
	case Anim_MouthYap2, Anim_MouthYap2Left, Anim_MouthYap2Right:
		return "Yap2"
	case Anim_MouthYap3, Anim_MouthYap3Left, Anim_MouthYap3Right:
		return "Yap3"
	default:
		return "?"
	}
}

// expressionSet reconstructs which expression "set" is active from the eye anim.
// The HUD can't see the dispatcher's internal eyeMode, so it infers it. Stare and
// Flushed are full eye+mouth sets; Happy/Excited are eye-only modes.
func expressionSet(eye AnimationID) string {
	switch eye {
	case Anim_EyeHappy:
		return "Happy"
	case Anim_EyeExcited, Anim_EyeExcitedLeft, Anim_EyeExcitedRight:
		return "Excited"
	case Anim_EyeStare:
		return "Stare"
	case Anim_EyeFlushed:
		return "Flushed"
	default:
		return "None"
	}
}
