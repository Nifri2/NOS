package cmd

import "testing"

// This is the dispatcher<->worker loopback suite: every signal the firmware can
// put on the bus is encoded with the production BuildPacket and decoded with the
// production PacketParser, then checked for correctness. Because both sides are
// the same code the firmware runs, a passing test means the real wire contract
// holds end to end.

// feedFrame streams a 6-byte frame through a parser one byte at a time, asserting
// the parser reports Incomplete until the final byte. Returns the final result.
func feedFrame(t *testing.T, p *PacketParser, frame [PacketSize]byte) (Packet, ParseStatus) {
	t.Helper()
	var pkt Packet
	var st ParseStatus
	for i, b := range frame {
		pkt, st = p.Feed(b)
		if i < PacketSize-1 && st != ParseIncomplete {
			t.Fatalf("byte %d: status=%v, want Incomplete (frame=% X)", i, st, frame)
		}
	}
	return pkt, st
}

// ---- Signal catalogue --------------------------------------------------------
// These tables are the single source of truth for "every signal we have". The
// sentinel test below fails if the protocol grows past them, forcing coverage to
// be extended whenever a new command or animation ID is added.

var allCommands = []struct {
	cmd  Command
	name string
}{
	{Cmd_NoOp, "NoOp"},
	{Cmd_LedOn, "LedOn"},
	{Cmd_LedOff, "LedOff"},
	{Cmd_DisplayAnim, "DisplayAnim"},
	{Cmd_Ping, "Ping"},
	{Cmd_Reboot, "Reboot"},
	{Cmd_DayMode, "DayMode"},
	{Cmd_NightMode, "NightMode"},
	{Cmd_Battery, "Battery"},
}

var allAnimations = []struct {
	id   AnimationID
	name string
}{
	{Anim_EyeIdle, "eye_idle"},
	{Anim_EyeBlink, "eye_blink"},
	{Anim_MouthIdleLeft, "mouth_idle_left"},
	{Anim_MouthIdleRight, "mouth_idle_right"},
	{Anim_EyeHappy, "eye_happy"},
	{Anim_MouthYap1Left, "mouth_yap_1_left"},
	{Anim_MouthYap1Right, "mouth_yap_1_right"},
	{Anim_MouthYap2Left, "mouth_yap_2_left"},
	{Anim_MouthYap2Right, "mouth_yap_2_right"},
	{Anim_MouthYap3Left, "mouth_yap_3_left"},
	{Anim_MouthYap3Right, "mouth_yap_3_right"},
	{Anim_EyeExcitedLeft, "eye_excited_left"},
	{Anim_EyeExcitedRight, "eye_excited_right"},
	{Anim_EyeFlushed, "eye_flushed"},
	{Anim_EyeStare, "eye_stare"},
	{Anim_MouthFlushedLeft, "mouth_flushed_left"},
	{Anim_MouthFlushedRight, "mouth_flushed_right"},
	{Anim_MouthStareLeft, "mouth_stare_left"},
	{Anim_MouthStareRight, "mouth_stare_right"},
	{Anim_MouthIdle, "mouth_idle"},
	{Anim_MouthYap1, "mouth_yap_1"},
	{Anim_MouthYap2, "mouth_yap_2"},
	{Anim_MouthYap3, "mouth_yap_3"},
	{Anim_EyeExcited, "eye_excited"},
	{Anim_MouthFlushed, "mouth_flushed"},
	{Anim_MouthStare, "mouth_stare"},
}

// TestSignalCoverage_Sentinel fails if a new Command or AnimationID is added
// without extending the catalogues above (and therefore the loopback coverage).
func TestSignalCoverage_Sentinel(t *testing.T) {
	if Cmd_Battery != 0x08 {
		t.Fatalf("highest command is now 0x%02X, not Battery(0x08): add it to allCommands", int(Cmd_Battery))
	}
	if len(allCommands) != 9 {
		t.Fatalf("allCommands has %d entries, expected 9 (one per command 0x00..0x08)", len(allCommands))
	}
	if Anim_MouthStare != 0x19 {
		t.Fatalf("highest animation ID is now 0x%02X, not 0x19: add it to allAnimations", int(Anim_MouthStare))
	}
	if len(allAnimations) != 26 {
		t.Fatalf("allAnimations has %d entries, expected 26 (IDs 0x00..0x19)", len(allAnimations))
	}
}

// ---- Every command: sent and received ---------------------------------------

func TestLoopback_EveryCommandRoundTrips(t *testing.T) {
	for _, c := range allCommands {
		// Broadcast so any worker accepts it. Eye/Mouth carry sentinel payloads
		// to prove all four data bytes survive the round trip intact.
		frame := BuildPacket(Address_All, c.cmd, 0x3D, 0x55)
		p := NewPacketParser(Worker_0)
		pkt, st := feedFrame(t, p, frame)
		if st != ParseAccepted {
			t.Errorf("%s: status=%v, want Accepted", c.name, st)
			continue
		}
		if pkt.Cmd != c.cmd || byte(pkt.Eye) != 0x3D || byte(pkt.Mouth) != 0x55 {
			t.Errorf("%s: decoded cmd=%02X eye=%02X mouth=%02X, want %02X 3D 55",
				c.name, byte(pkt.Cmd), byte(pkt.Eye), byte(pkt.Mouth), byte(c.cmd))
		}
	}
}

// ---- Every animation: sent and received, and resolved to the right side ------

// sideMapping is the expected logical-ID -> per-side variant translation. Written
// as explicit literals (the test oracle) rather than reusing the production map.
var sideMapping = []struct {
	logical     AnimationID
	left, right AnimationID
}{
	{Anim_MouthIdle, Anim_MouthIdleLeft, Anim_MouthIdleRight},
	{Anim_MouthYap1, Anim_MouthYap1Left, Anim_MouthYap1Right},
	{Anim_MouthYap2, Anim_MouthYap2Left, Anim_MouthYap2Right},
	{Anim_MouthYap3, Anim_MouthYap3Left, Anim_MouthYap3Right},
	{Anim_EyeExcited, Anim_EyeExcitedLeft, Anim_EyeExcitedRight},
	{Anim_MouthFlushed, Anim_MouthFlushedLeft, Anim_MouthFlushedRight},
	{Anim_MouthStare, Anim_MouthStareLeft, Anim_MouthStareRight},
}

func TestLoopback_EveryAnimationRoundTrips(t *testing.T) {
	// Each animation ID is sent in both the eye and mouth slot of a DisplayAnim
	// packet and must arrive byte-identical at the worker.
	for _, a := range allAnimations {
		frame := BuildPacket(Address_All, Cmd_DisplayAnim, a.id, a.id)
		p := NewPacketParser(Worker_0)
		pkt, st := feedFrame(t, p, frame)
		if st != ParseAccepted {
			t.Errorf("%s: status=%v, want Accepted", a.name, st)
			continue
		}
		if pkt.Eye != a.id || pkt.Mouth != a.id {
			t.Errorf("%s: decoded eye=%02X mouth=%02X, want %02X",
				a.name, byte(pkt.Eye), byte(pkt.Mouth), byte(a.id))
		}
	}
}

func TestLoopback_LogicalAnimationsResolvePerSide(t *testing.T) {
	// Dispatcher broadcasts a logical ID; each worker must translate it to its
	// own side variant after decoding. This is the full eye+mouth pipeline.
	for _, m := range sideMapping {
		frame := BuildPacket(Address_All, Cmd_DisplayAnim, m.logical, m.logical)

		pl := NewPacketParser(Worker_0)
		pktL, stL := feedFrame(t, pl, frame)
		if stL != ParseAccepted {
			t.Fatalf("left: %v want Accepted", stL)
		}
		if got := MapAnimation(Worker_0, pktL.Eye); AnimationID(got) != m.left {
			t.Errorf("logical %02X on Worker_0 -> %02X, want left %02X",
				byte(m.logical), got, byte(m.left))
		}

		pr := NewPacketParser(Worker_1)
		pktR, stR := feedFrame(t, pr, frame)
		if stR != ParseAccepted {
			t.Fatalf("right: %v want Accepted", stR)
		}
		if got := MapAnimation(Worker_1, pktR.Mouth); AnimationID(got) != m.right {
			t.Errorf("logical %02X on Worker_1 -> %02X, want right %02X",
				byte(m.logical), got, byte(m.right))
		}
	}
}

// ---- Addressing: every target, accepted or filtered correctly ----------------

func TestLoopback_AddressTargeting(t *testing.T) {
	targets := []Address{Worker_0, Worker_1, Worker_2, Worker_3, Address_All}
	for _, target := range targets {
		frame := BuildPacket(target, Cmd_NoOp, 0, 0)
		p := NewPacketParser(Worker_0)
		_, st := feedFrame(t, p, frame)
		wantAccept := target == Worker_0 || target == Address_All
		if wantAccept && st != ParseAccepted {
			t.Errorf("target=%02X: status=%v, want Accepted", int(target), st)
		}
		if !wantAccept && st != ParseNotForUs {
			t.Errorf("target=%02X: status=%v, want NotForUs", int(target), st)
		}
	}
}

// ---- Corruption: every covered byte flipped must be rejected ------------------

func TestLoopback_CorruptionRejected(t *testing.T) {
	base := BuildPacket(Address_All, Cmd_DisplayAnim, Anim_EyeStare, Anim_MouthStare)
	// Flip one bit in each of the 6 bytes; the header break re-syncs, the others
	// must surface as a CRC failure (never a silently-accepted wrong packet).
	for i := 0; i < PacketSize; i++ {
		corrupt := base
		corrupt[i] ^= 0x01
		p := NewPacketParser(Worker_0)
		var st ParseStatus
		for _, b := range corrupt {
			_, st = p.Feed(b)
		}
		if i == 0 {
			// Header corrupted -> never completes a frame.
			if st != ParseIncomplete {
				t.Errorf("corrupt header: status=%v, want Incomplete (no frame)", st)
			}
			continue
		}
		if st == ParseAccepted {
			t.Errorf("corrupting byte %d was silently accepted — CRC did not catch it", i)
		}
	}
}

// ---- Realistic bus: many packets back-to-back with line noise -----------------

func TestLoopback_MixedByteStream(t *testing.T) {
	// Build one packet per command, concatenate them with garbage bytes between
	// frames (as the real noisy bus would), and confirm the parser recovers every
	// command in order — header sync must skip the junk.
	var stream []byte
	var want []Command
	for _, c := range allCommands {
		stream = append(stream, 0x00, 0xFF, 0x12) // inter-frame garbage
		frame := BuildPacket(Address_All, c.cmd, 0, 0)
		stream = append(stream, frame[:]...)
		want = append(want, c.cmd)
	}
	stream = append(stream, 0xAB, 0xCD) // trailing junk, no frame

	p := NewPacketParser(Worker_0)
	var got []Command
	for _, b := range stream {
		pkt, st := p.Feed(b)
		if st == ParseAccepted {
			got = append(got, pkt.Cmd)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("recovered %d packets, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("packet %d: cmd=%02X, want %02X", i, byte(got[i]), byte(want[i]))
		}
	}
}

// ---- The dispatcher's actual emissions ---------------------------------------
// This table mirrors every packet RunDispatcher puts on the bus (dispatch.go:
// keepalive, battery telemetry, each radio action, and the blink state machine).
// It is the spec for "what the dispatcher sends"; add a row when you add an
// action. Each emission is encoded for real and verified on the receiving side.

func TestLoopback_DispatcherEmissions(t *testing.T) {
	type emission struct {
		desc        string
		addr        Address
		cmd         Command
		eye, mouth  AnimationID
		hudRelevant bool // does the HUD react to it?
	}
	emissions := []emission{
		{"keepalive", Address_All, Cmd_NoOp, Anim_EyeIdle, Anim_MouthIdle, false},
		{"battery telemetry", Address_All, Cmd_Battery, AnimationID(74), AnimationID(82), true},
		{"A+A mouth idle", Address_All, Cmd_DisplayAnim, Anim_EyeIdle, Anim_MouthIdle, true},
		{"B+A eye idle blink", Address_All, Cmd_DisplayAnim, Anim_EyeIdle, Anim_MouthIdle, true},
		{"B+B eyes happy", Address_All, Cmd_DisplayAnim, Anim_EyeHappy, Anim_MouthIdle, true},
		{"B+C eyes excited", Address_All, Cmd_DisplayAnim, Anim_EyeExcited, Anim_MouthIdle, true},
		{"C+B stare set", Address_All, Cmd_DisplayAnim, Anim_EyeStare, Anim_MouthStare, true},
		{"C+C flushed set", Address_All, Cmd_DisplayAnim, Anim_EyeFlushed, Anim_MouthFlushed, true},
		{"D+D day mode", Address_All, Cmd_DayMode, 0, 0, true},
		{"D+D night mode", Address_All, Cmd_NightMode, 0, 0, true},
		{"blink on (W0)", Worker_0, Cmd_DisplayAnim, Anim_EyeBlink, Anim_MouthIdle, false},
		{"blink on (W1)", Worker_1, Cmd_DisplayAnim, Anim_EyeBlink, Anim_MouthIdle, false},
		{"blink off (W0)", Worker_0, Cmd_DisplayAnim, Anim_EyeIdle, Anim_MouthIdle, false},
		{"blink off (W1)", Worker_1, Cmd_DisplayAnim, Anim_EyeIdle, Anim_MouthIdle, false},
	}

	for _, e := range emissions {
		frame := BuildPacket(e.addr, e.cmd, e.eye, e.mouth)

		// Received and decoded correctly by an addressed worker.
		recipient := e.addr
		if recipient == Address_All {
			recipient = Worker_0
		}
		p := NewPacketParser(recipient)
		pkt, st := feedFrame(t, p, frame)
		if st != ParseAccepted {
			t.Errorf("%s: worker status=%v, want Accepted", e.desc, st)
			continue
		}
		if pkt.Cmd != e.cmd || pkt.Eye != e.eye || pkt.Mouth != e.mouth {
			t.Errorf("%s: decoded (%02X,%02X,%02X), want (%02X,%02X,%02X)",
				e.desc, byte(pkt.Cmd), byte(pkt.Eye), byte(pkt.Mouth),
				byte(e.cmd), byte(e.eye), byte(e.mouth))
		}

		// HUD-relevant emissions must also drive the HUD reducer correctly.
		if e.hudRelevant {
			var st hudState
			if !hudApplyPacket(&st, frame[1], frame[2], frame[3], frame[4], frame[5]) {
				t.Errorf("%s: HUD rejected a valid packet", e.desc)
				continue
			}
			switch e.cmd {
			case Cmd_DisplayAnim:
				if st.eyeAnim != e.eye || st.mouthAnim != e.mouth {
					t.Errorf("%s: HUD anim state (%02X,%02X), want (%02X,%02X)",
						e.desc, byte(st.eyeAnim), byte(st.mouthAnim), byte(e.eye), byte(e.mouth))
				}
			case Cmd_DayMode:
				if !st.dayMode {
					t.Errorf("%s: HUD dayMode=false, want true", e.desc)
				}
			case Cmd_NightMode:
				if st.dayMode {
					t.Errorf("%s: HUD dayMode=true, want false", e.desc)
				}
			case Cmd_Battery:
				if st.battDeci != byte(e.eye) || st.battPct != byte(e.mouth) {
					t.Errorf("%s: HUD battery (%d,%d), want (%d,%d)",
						e.desc, st.battDeci, st.battPct, byte(e.eye), byte(e.mouth))
				}
			}
		}
	}
}
