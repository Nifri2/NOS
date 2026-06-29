package cmd

// Per-pixel frame math for the worker render loop, split out of worker.go (which
// is tinygo-tagged and hardware-coupled) so it can be unit-tested on the host.
// Both functions write the PIO's GRB-packed uint32 format: g<<24 | r<<16 | b<<8.
// brightnessPercent scales each channel (100 = unchanged) and every channel is
// clamped to 255.

// bytesToRawInto converts a GRB byte frame into dst (zero allocation), applying
// brightnessPercent and clamping each channel to 255. If src holds more pixels
// than dst, the extra pixels are dropped.
func bytesToRawInto(dst []uint32, src []byte, brightnessPercent int) {
	n := len(src) / 3
	if n > len(dst) {
		n = len(dst)
	}
	scale := brightnessPercent
	for i := 0; i < n; i++ {
		gi := int(src[i*3]) * scale / 100
		ri := int(src[i*3+1]) * scale / 100
		bi := int(src[i*3+2]) * scale / 100
		if gi > 255 {
			gi = 255
		}
		if ri > 255 {
			ri = 255
		}
		if bi > 255 {
			bi = 255
		}
		dst[i] = uint32(gi)<<24 | uint32(ri)<<16 | uint32(bi)<<8
	}
}

// lerpFrameInto blends two GRB byte frames into a uint32 render buffer.
// step ranges from 0 to totalSteps inclusive (step=0 → pure from, step=totalSteps → pure to).
// Uses integer smoothstep easing (no floats, no allocations).
// Smoothstep: s(t) = t²(3-2t) computed in fixed-point with 256 = 1.0.
func lerpFrameInto(dst []uint32, from []byte, to []byte, step, totalSteps, pixelCount, brightnessPercent int) {
	scale := brightnessPercent
	for i := 0; i < pixelCount; i++ {
		// Map step to [0, 256]
		t := step * 256 / totalSteps
		// Smoothstep in fixed-point: t²(768-2t)/65536 stays in [0, 256]
		t = t * t * (768 - 2*t) / 65536

		fg := int(from[i*3])
		fr := int(from[i*3+1])
		fb := int(from[i*3+2])
		tg := int(to[i*3])
		tr := int(to[i*3+1])
		tb := int(to[i*3+2])

		gi := (fg + (tg-fg)*t/256) * scale / 100
		ri := (fr + (tr-fr)*t/256) * scale / 100
		bi := (fb + (tb-fb)*t/256) * scale / 100
		if gi > 255 {
			gi = 255
		}
		if ri > 255 {
			ri = 255
		}
		if bi > 255 {
			bi = 255
		}
		dst[i] = uint32(gi)<<24 | uint32(ri)<<16 | uint32(bi)<<8
	}
}
