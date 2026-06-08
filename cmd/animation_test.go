package cmd

import (
	"encoding/binary"
	"testing"
)

// buildAnimBytes constructs a synthetic .animbyte payload: 4-byte little-endian
// frame count followed by frameCount * width * height * 3 bytes of pixel data.
func buildAnimBytes(frameCount, w, h int, fill byte) []byte {
	bytesPerFrame := w * h * 3
	buf := make([]byte, 4+frameCount*bytesPerFrame)
	binary.LittleEndian.PutUint32(buf[:4], uint32(frameCount))
	for i := 4; i < len(buf); i++ {
		buf[i] = fill
	}
	return buf
}

func TestLoadAnimation_SingleFrameEye(t *testing.T) {
	data := buildAnimBytes(1, EyeFrameWidth, EyeFrameHeight, 0x42)
	anim, err := LoadAnimation(data, EyeFrameWidth, EyeFrameHeight, "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if anim.FrameCount != 1 || len(anim.Frames) != 1 {
		t.Errorf("frame count = %d / %d, want 1 / 1", anim.FrameCount, len(anim.Frames))
	}
	if anim.Name != "test" {
		t.Errorf("name = %q, want %q", anim.Name, "test")
	}
	expectedSize := EyeFrameWidth * EyeFrameHeight * 3
	if len(anim.Frames[0]) != expectedSize {
		t.Errorf("frame size = %d, want %d", len(anim.Frames[0]), expectedSize)
	}
}

func TestLoadAnimation_MultiFrameMouth(t *testing.T) {
	data := buildAnimBytes(50, MouthFrameWidth, MouthFrameHeight, 0x00)
	anim, err := LoadAnimation(data, MouthFrameWidth, MouthFrameHeight, "anim")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if anim.FrameCount != 50 {
		t.Errorf("FrameCount = %d, want 50", anim.FrameCount)
	}
}

func TestLoadAnimation_TooShort(t *testing.T) {
	if _, err := LoadAnimation([]byte{0, 0, 0}, 16, 16, "x"); err == nil {
		t.Errorf("expected error for <4-byte input")
	}
}

func TestLoadAnimation_WrongSize(t *testing.T) {
	// Claim 2 frames but only ship 1 frame's worth of pixels.
	buf := make([]byte, 4+EyeFrameWidth*EyeFrameHeight*3)
	binary.LittleEndian.PutUint32(buf[:4], 2)
	if _, err := LoadAnimation(buf, EyeFrameWidth, EyeFrameHeight, "bad"); err == nil {
		t.Errorf("expected error when payload size mismatches declared frame count")
	}
}

func TestMapAnimation_LogicalToSideVariant(t *testing.T) {
	cases := []struct {
		addr    Address
		in      AnimationID
		want    AnimationID
		comment string
	}{
		{Worker_0, Anim_MouthIdle, Anim_MouthIdleLeft, "left worker takes the left variant"},
		{Worker_1, Anim_MouthIdle, Anim_MouthIdleRight, "right worker takes the right variant"},
		{Worker_0, Anim_MouthFlushed, Anim_MouthFlushedLeft, "flushed (new set) maps left"},
		{Worker_1, Anim_MouthStare, Anim_MouthStareRight, "stare (new set) maps right"},
		{Worker_0, Anim_EyeExcited, Anim_EyeExcitedLeft, "excited maps left"},
	}
	for _, c := range cases {
		got := MapAnimation(c.addr, c.in)
		if AnimationID(got) != c.want {
			t.Errorf("%s: MapAnimation(%v, %v) = %v, want %v", c.comment, c.addr, c.in, AnimationID(got), c.want)
		}
	}
}

func TestMapAnimation_NoMappingPassesThrough(t *testing.T) {
	// Eye animations are dispatcher-broadcast directly; no per-side translation.
	if got := MapAnimation(Worker_0, Anim_EyeBlink); AnimationID(got) != Anim_EyeBlink {
		t.Errorf("Anim_EyeBlink should pass through unchanged on Worker_0, got %v", AnimationID(got))
	}
	// Address that isn't in the mapping table (e.g. Worker_2) should pass through too.
	if got := MapAnimation(Worker_2, Anim_MouthIdle); AnimationID(got) != Anim_MouthIdle {
		t.Errorf("unmapped address should pass IDs through, got %v", AnimationID(got))
	}
}
