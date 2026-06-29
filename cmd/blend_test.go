package cmd

import "testing"

// pack mirrors the GRB-packed uint32 layout the render functions produce.
func pack(g, r, b uint32) uint32 {
	return g<<24 | r<<16 | b<<8
}

func TestBytesToRawInto_Identity(t *testing.T) {
	// brightness 100 => straight GRB pack, no scaling.
	src := []byte{10, 20, 30, 40, 50, 60} // two pixels
	dst := make([]uint32, 2)
	bytesToRawInto(dst, src, 100)
	if dst[0] != pack(10, 20, 30) || dst[1] != pack(40, 50, 60) {
		t.Errorf("identity pack = %#x, %#x; want %#x, %#x",
			dst[0], dst[1], pack(10, 20, 30), pack(40, 50, 60))
	}
}

func TestBytesToRawInto_BrightnessAndClamp(t *testing.T) {
	src := []byte{100, 200, 50}
	dst := make([]uint32, 1)
	bytesToRawInto(dst, src, 200) // double each channel
	// 100*2=200, 200*2=400->clamp 255, 50*2=100
	if dst[0] != pack(200, 255, 100) {
		t.Errorf("scaled/clamped = %#x, want %#x", dst[0], pack(200, 255, 100))
	}
}

func TestBytesToRawInto_SrcLongerThanDst(t *testing.T) {
	src := []byte{1, 1, 1, 2, 2, 2, 3, 3, 3} // 3 pixels
	dst := make([]uint32, 2)                 // room for only 2
	bytesToRawInto(dst, src, 100)
	if dst[0] != pack(1, 1, 1) || dst[1] != pack(2, 2, 2) {
		t.Errorf("truncated copy = %#x, %#x; want %#x, %#x",
			dst[0], dst[1], pack(1, 1, 1), pack(2, 2, 2))
	}
}

func TestLerpFrameInto_Endpoints(t *testing.T) {
	from := []byte{10, 20, 30}
	to := []byte{200, 100, 0}
	dst := make([]uint32, 1)
	const steps = 10

	// step=0 must render the "from" frame exactly.
	lerpFrameInto(dst, from, to, 0, steps, 1, 100)
	if dst[0] != pack(10, 20, 30) {
		t.Errorf("step=0 = %#x, want pure from %#x", dst[0], pack(10, 20, 30))
	}

	// step=totalSteps must render the "to" frame exactly.
	lerpFrameInto(dst, from, to, steps, steps, 1, 100)
	if dst[0] != pack(200, 100, 0) {
		t.Errorf("step=totalSteps = %#x, want pure to %#x", dst[0], pack(200, 100, 0))
	}
}

func TestLerpFrameInto_Midpoint(t *testing.T) {
	// smoothstep(0.5) == 0.5, so the halfway step is the arithmetic midpoint.
	from := []byte{0, 0, 0}
	to := []byte{100, 200, 80}
	dst := make([]uint32, 1)
	const steps = 10
	lerpFrameInto(dst, from, to, steps/2, steps, 1, 100)
	if dst[0] != pack(50, 100, 40) {
		t.Errorf("midpoint = %#x, want %#x", dst[0], pack(50, 100, 40))
	}
}

func TestLerpFrameInto_BrightnessClamp(t *testing.T) {
	// A constant frame at 200, doubled, must clamp every channel to 255
	// regardless of the interpolation step.
	from := []byte{200, 200, 200}
	to := []byte{200, 200, 200}
	dst := make([]uint32, 1)
	lerpFrameInto(dst, from, to, 5, 10, 1, 200)
	if dst[0] != pack(255, 255, 255) {
		t.Errorf("clamp = %#x, want all channels 255", dst[0])
	}
}
