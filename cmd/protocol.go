package cmd

// Pure-Go wire protocol: the dispatcher's packet builder and the worker's byte
// framing/decoding state machine. Both the firmware (worker.go, dispatch.go,
// tinygo-tagged) and the host unit tests use these, so the encode and decode
// sides are guaranteed to agree — there is no second copy to drift.
//
// Wire format (6 bytes): [Header(0xAA), Address, Command, EyeAnim, MouthAnim,
// CRC-8 of bytes 1..4].

const (
	PacketHeader = 0xAA
	PacketSize   = 6
)

// BuildPacket assembles a wire packet ready to write to the UART bus.
func BuildPacket(addr Address, cmd Command, eye, mouth AnimationID) [PacketSize]byte {
	a, c, e, m := byte(addr), byte(cmd), byte(eye), byte(mouth)
	return [PacketSize]byte{PacketHeader, a, c, e, m, Crc8Bytes4(a, c, e, m)}
}

// Packet is a decoded, CRC-validated frame.
type Packet struct {
	Addr  byte
	Cmd   Command
	Eye   AnimationID
	Mouth AnimationID
}

// ParseStatus reports what feeding a byte produced.
type ParseStatus int

const (
	ParseIncomplete ParseStatus = iota // still buffering a frame
	ParseAccepted                      // full frame, CRC ok, addressed to this node or broadcast
	ParseNotForUs                      // full frame, CRC ok, addressed elsewhere
	ParseBadCRC                        // full frame, checksum mismatch
)

// PacketParser is the worker-side framing state machine: header sync, fixed
// 6-byte fill, CRC check, and address filtering. It is fed one received byte at
// a time. The inter-byte timeout is intentionally left to the caller (the
// firmware tracks wall-clock between UART reads) and applied via Reset.
type PacketParser struct {
	addr Address
	buf  [PacketSize]byte
	idx  int
}

// NewPacketParser returns a parser that accepts frames addressed to addr or to
// the broadcast address.
func NewPacketParser(addr Address) *PacketParser {
	return &PacketParser{addr: addr}
}

// Pending reports how many bytes of a partial frame are buffered.
func (p *PacketParser) Pending() int { return p.idx }

// Reset discards any partial frame (used on inter-byte timeout).
func (p *PacketParser) Reset() { p.idx = 0 }

// Partial returns the bytes buffered so far, for diagnostic logging on timeout.
func (p *PacketParser) Partial() []byte { return p.buf[:p.idx] }

// LastFrame returns the full 6-byte buffer, valid for diagnostics immediately
// after Feed returns ParseBadCRC / ParseAccepted / ParseNotForUs.
func (p *PacketParser) LastFrame() [PacketSize]byte { return p.buf }

// Feed consumes one received byte. While syncing it ignores everything until the
// header. On a completed frame it validates the CRC, decodes, and address-filters,
// then resets for the next frame.
func (p *PacketParser) Feed(b byte) (Packet, ParseStatus) {
	if p.idx == 0 {
		if b == PacketHeader {
			p.buf[0] = b
			p.idx = 1
		}
		return Packet{}, ParseIncomplete
	}

	p.buf[p.idx] = b
	p.idx++
	if p.idx < PacketSize {
		return Packet{}, ParseIncomplete
	}

	// Frame complete — reset index now so the buffer is ready for the next one
	// (the bytes stay readable via LastFrame until the next Feed overwrites them).
	p.idx = 0

	if Crc8Bytes4(p.buf[1], p.buf[2], p.buf[3], p.buf[4]) != p.buf[5] {
		return Packet{}, ParseBadCRC
	}

	pkt := Packet{
		Addr:  p.buf[1],
		Cmd:   Command(p.buf[2]),
		Eye:   AnimationID(p.buf[3]),
		Mouth: AnimationID(p.buf[4]),
	}
	if Address(pkt.Addr) == p.addr || Address(pkt.Addr) == Address_All {
		return pkt, ParseAccepted
	}
	return pkt, ParseNotForUs
}
