package cmd

import "testing"

// crcFor wraps Crc8Bytes4 to make test cases read like protocol packets.
func crcFor(addr, cmd, eye, mouth byte) byte {
	return Crc8Bytes4(addr, cmd, eye, mouth)
}

func TestHudApplyPacket_DisplayAnim(t *testing.T) {
	var st hudState
	addr, cmd := byte(Address_All), byte(Cmd_DisplayAnim)
	eye, mouth := byte(Anim_EyeStare), byte(Anim_MouthStare)
	csum := crcFor(addr, cmd, eye, mouth)

	if !hudApplyPacket(&st, addr, cmd, eye, mouth, csum) {
		t.Fatalf("DisplayAnim: expected accept")
	}
	if st.eyeAnim != Anim_EyeStare || st.mouthAnim != Anim_MouthStare {
		t.Errorf("DisplayAnim: state = (%v, %v), want (Stare, Stare)", st.eyeAnim, st.mouthAnim)
	}
	if st.rxPackets != 1 || st.crcFails != 0 {
		t.Errorf("counters = (rx=%d, fails=%d), want (1, 0)", st.rxPackets, st.crcFails)
	}
}

func TestHudApplyPacket_DayNightToggle(t *testing.T) {
	var st hudState
	day := byte(Cmd_DayMode)
	night := byte(Cmd_NightMode)

	hudApplyPacket(&st, 0xFF, day, 0, 0, crcFor(0xFF, day, 0, 0))
	if !st.dayMode {
		t.Errorf("after DayMode: dayMode = false, want true")
	}
	hudApplyPacket(&st, 0xFF, night, 0, 0, crcFor(0xFF, night, 0, 0))
	if st.dayMode {
		t.Errorf("after NightMode: dayMode = true, want false")
	}
}

func TestHudApplyPacket_Battery(t *testing.T) {
	var st hudState
	cmd := byte(Cmd_Battery)
	deci, pct := byte(74), byte(82)
	hudApplyPacket(&st, 0xFF, cmd, deci, pct, crcFor(0xFF, cmd, deci, pct))
	if st.battDeci != 74 || st.battPct != 82 {
		t.Errorf("battery state = (%d, %d), want (74, 82)", st.battDeci, st.battPct)
	}
}

func TestHudApplyPacket_BadCRC(t *testing.T) {
	var st hudState
	addr, cmd := byte(Address_All), byte(Cmd_DisplayAnim)
	eye, mouth := byte(Anim_EyeHappy), byte(Anim_MouthIdle)
	good := crcFor(addr, cmd, eye, mouth)

	if hudApplyPacket(&st, addr, cmd, eye, mouth, good^0x01) {
		t.Fatalf("bad CRC: expected reject")
	}
	if st.rxPackets != 0 || st.crcFails != 1 {
		t.Errorf("counters = (rx=%d, fails=%d), want (0, 1)", st.rxPackets, st.crcFails)
	}
	if st.eyeAnim != 0 || st.mouthAnim != 0 {
		t.Errorf("rejected packet must not mutate render state, got (%v, %v)", st.eyeAnim, st.mouthAnim)
	}
}

func TestHudApplyPacket_UnknownCommandAcceptedNoState(t *testing.T) {
	var st hudState
	cmd := byte(0xEE) // not a defined Command
	csum := crcFor(0xFF, cmd, 0x42, 0x55)
	if !hudApplyPacket(&st, 0xFF, cmd, 0x42, 0x55, csum) {
		t.Fatalf("valid CRC must be accepted even for unknown command")
	}
	// rxPackets ticks but no visible state moved.
	if st.eyeAnim != 0 || st.mouthAnim != 0 || st.dayMode || st.battDeci != 0 {
		t.Errorf("unknown command must not touch rendered state, got %+v", st)
	}
}

func TestDisplayChanged(t *testing.T) {
	a := hudState{eyeAnim: Anim_EyeHappy, battDeci: 74}
	b := a
	if displayChanged(&a, &b) {
		t.Errorf("identical states should not be flagged as changed")
	}

	// rxPackets/crcFails ticking must NOT trigger a redraw.
	b.rxPackets = 999
	b.crcFails = 7
	if displayChanged(&a, &b) {
		t.Errorf("rxPackets/crcFails should not trigger redraw")
	}

	b.eyeAnim = Anim_EyeStare
	if !displayChanged(&a, &b) {
		t.Errorf("eyeAnim change should trigger redraw")
	}
}
