package cmd

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
