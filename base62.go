// Package base62 implements base62 encoding.
package base62

import (
	"encoding/binary"
	"io"
	"strconv"
)

/*
 * Encodings
 */

// An Encoding is a radix 62 encoding/decoding scheme, defined by a
// 62-character alphabet.
type Encoding struct {
	encode    [62]byte
	decodeMap [256]byte
}

const encodeStd = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// NewEncoding returns a new padded Encoding defined by the given alphabet,
// which must be a 62-byte string that does not contain the padding character
// or CR / LF ('\r', '\n').
func NewEncoding(encoder string) *Encoding {
	if len(encoder) != 62 {
		panic("encoding alphabet is not 62-bytes long")
	}
	for i := 0; i < len(encoder); i++ {
		if encoder[i] == '\n' || encoder[i] == '\r' {
			panic("encoding alphabet contains newline character")
		}
	}

	e := new(Encoding)
	copy(e.encode[:], encoder)

	for i := 0; i < len(e.decodeMap); i++ {
		e.decodeMap[i] = 0xFF
	}
	for i := 0; i < len(encoder); i++ {
		e.decodeMap[encoder[i]] = byte(i)
	}
	return e
}

// StdEncoding is the standard base62 encoding, as defined in
// RFC 4648.
var StdEncoding = NewEncoding(encodeStd)

// URLEncoding is the alternate base62 encoding defined in RFC 4648.
// It is typically used in URLs and file names.
var URLEncoding = NewEncoding(encodeStd)

/*
 * Encoder
 */

// Encode encodes src using the encoding enc, writing
// EncodedLen(len(src)) bytes to dst.
//
// The encoding pads the output to a multiple of 4 bytes,
// so Encode is not appropriate for use on individual blocks
// of a large data stream. Use NewEncoder() instead.
func (enc *Encoding) Encode(dst, src []byte) {
	if len(src) == 0 {
		return
	}
	// enc is a pointer receiver, so the use of enc.encode within the hot
	// loop below means a nil check at every operation. Lift that nil check
	// outside of the loop to speed up the encoder.
	_ = enc.encode

	di, si := 0, 0
	n := (len(src) / 3) * 3
	for si < n {
		// Convert 3x 8bit source bytes into 4 bytes
		val := uint(src[si+0])<<16 | uint(src[si+1])<<8 | uint(src[si+2])

		dst[di+0] = enc.encode[val>>18&0x3F]
		dst[di+1] = enc.encode[val>>12&0x3F]
		dst[di+2] = enc.encode[val>>6&0x3F]
		dst[di+3] = enc.encode[val&0x3F]

		si += 3
		di += 4
	}

	remain := len(src) - si
	if remain == 0 {
		return
	}
	// Add the remaining small block
	val := uint(src[si+0]) << 16
	if remain == 2 {
		val |= uint(src[si+1]) << 8
	}

	dst[di+0] = enc.encode[val>>18&0x3F]
	dst[di+1] = enc.encode[val>>12&0x3F]

	switch remain {
	case 2:
		dst[di+2] = enc.encode[val>>6&0x3F]
	case 1:
	}
}

// EncodeToString returns the base62 encoding of src.
func (enc *Encoding) EncodeToString(src []byte) string {
	buf := make([]byte, enc.EncodedLen(len(src)))
	enc.Encode(buf, src)
	return string(buf)
}

type encoder struct {
	err  error
	enc  *Encoding
	w    io.Writer
	buf  [3]byte    // buffered data waiting to be encoded
	nbuf int        // number of bytes in buf
	out  [1024]byte // output buffer
}

func (e *encoder) Write(p []byte) (n int, err error) {
	if e.err != nil {
		return 0, e.err
	}

	// Leading fringe.
	if e.nbuf > 0 {
		var i int
		for i = 0; i < len(p) && e.nbuf < 3; i++ {
			e.buf[e.nbuf] = p[i]
			e.nbuf++
		}
		n += i
		p = p[i:]
		if e.nbuf < 3 {
			return
		}
		e.enc.Encode(e.out[:], e.buf[:])
		if _, e.err = e.w.Write(e.out[:4]); e.err != nil {
			return n, e.err
		}
		e.nbuf = 0
	}

	// Large interior chunks.
	for len(p) >= 3 {
		nn := len(e.out) / 4 * 3
		if nn > len(p) {
			nn = len(p)
			nn -= nn % 3
		}
		e.enc.Encode(e.out[:], p[:nn])
		if _, e.err = e.w.Write(e.out[0 : nn/3*4]); e.err != nil {
			return n, e.err
		}
		n += nn
		p = p[nn:]
	}

	// Trailing fringe.
	copy(e.buf[:], p)
	e.nbuf = len(p)
	n += len(p)
	return
}

// Close flushes any pending output from the encoder.
// It is an error to call Write after calling Close.
func (e *encoder) Close() error {
	// If there's anything left in the buffer, flush it out
	if e.err == nil && e.nbuf > 0 {
		e.enc.Encode(e.out[:], e.buf[:e.nbuf])
		_, e.err = e.w.Write(e.out[:e.enc.EncodedLen(e.nbuf)])
		e.nbuf = 0
	}
	return e.err
}

// NewEncoder returns a new base62 stream encoder. Data written to
// the returned writer will be encoded using enc and then written to w.
// base62 encodings operate in 4-byte blocks; when finished
// writing, the caller must Close the returned encoder to flush any
// partially written blocks.
func NewEncoder(enc *Encoding, w io.Writer) io.WriteCloser {
	return &encoder{enc: enc, w: w}
}

// EncodedLen returns the length in bytes of the base62 encoding
// of an input buffer of length n.
func (enc *Encoding) EncodedLen(n int) int {
	return (n*8 + 5) / 6 // minimum # chars at 6 bits per char
}

/*
 * Decoder
 */

type CorruptInputError int64

func (e CorruptInputError) Error() string {
	return "illegal base62 data at input byte " + strconv.FormatInt(int64(e), 10)
}

// decodeQuantum decodes up to 4 base62 bytes. The received parameters are
// the destination buffer dst, the source buffer src and an index in the
// source buffer si.
// It returns the number of bytes read from src, the number of bytes written
// to dst, and an error, if any.
func (enc *Encoding) decodeQuantum(dst, src []byte, si int) (nsi, n int, err error) {
	// Decode quantum using the base62 alphabet
	var dbuf [4]byte
	dlen := 4

	// Lift the nil check outside of the loop.
	_ = enc.decodeMap

	for j := 0; j < len(dbuf); j++ {
		if len(src) == si {
			switch {
			case j == 0:
				return si, 0, nil
			case j == 1, enc.padChar != NoPadding:
				return si, 0, CorruptInputError(si - j)
			}
			dlen = j
			break
		}
		in := src[si]
		si++

		out := enc.decodeMap[in]
		if out != 0xff {
			dbuf[j] = out
			continue
		}

		if in == '\n' || in == '\r' {
			j--
			continue
		}

		if rune(in) != enc.padChar {
			return si, 0, CorruptInputError(si - 1)
		}

		// We've reached the end and there's padding
		switch j {
		case 0, 1:
			// incorrect padding
			return si, 0, CorruptInputError(si - 1)
		case 2:
			// "==" is expected, the first "=" is already consumed.
			// skip over newlines
			for si < len(src) && (src[si] == '\n' || src[si] == '\r') {
				si++
			}
			if si == len(src) {
				// not enough padding
				return si, 0, CorruptInputError(len(src))
			}
			if rune(src[si]) != enc.padChar {
				// incorrect padding
				return si, 0, CorruptInputError(si - 1)
			}

			si++
		}

		// skip over newlines
		for si < len(src) && (src[si] == '\n' || src[si] == '\r') {
			si++
		}
		if si < len(src) {
			// trailing garbage
			err = CorruptInputError(si)
		}
		dlen = j
		break
	}

	// Convert 4x 6bit source bytes into 3 bytes
	val := uint(dbuf[0])<<18 | uint(dbuf[1])<<12 | uint(dbuf[2])<<6 | uint(dbuf[3])
	dbuf[2], dbuf[1], dbuf[0] = byte(val>>0), byte(val>>8), byte(val>>16)
	switch dlen {
	case 4:
		dst[2] = dbuf[2]
		dbuf[2] = 0
		fallthrough
	case 3:
		dst[1] = dbuf[1]
		if enc.strict && dbuf[2] != 0 {
			return si, 0, CorruptInputError(si - 1)
		}
		dbuf[1] = 0
		fallthrough
	case 2:
		dst[0] = dbuf[0]
		if enc.strict && (dbuf[1] != 0 || dbuf[2] != 0) {
			return si, 0, CorruptInputError(si - 2)
		}
	}

	return si, dlen - 1, err
}

// DecodeString returns the bytes represented by the base62 string s.
func (enc *Encoding) DecodeString(s string) ([]byte, error) {
	dbuf := make([]byte, enc.DecodedLen(len(s)))
	n, err := enc.Decode(dbuf, []byte(s))
	return dbuf[:n], err
}

type decoder struct {
	err     error
	readErr error // error from r.Read
	enc     *Encoding
	r       io.Reader
	buf     [1024]byte // leftover input
	nbuf    int
	out     []byte // leftover decoded output
	outbuf  [1024 / 4 * 3]byte
}

func (d *decoder) Read(p []byte) (n int, err error) {
	// Use leftover decoded output from last read.
	if len(d.out) > 0 {
		n = copy(p, d.out)
		d.out = d.out[n:]
		return n, nil
	}

	if d.err != nil {
		return 0, d.err
	}

	// This code assumes that d.r strips supported whitespace ('\r' and '\n').

	// Refill buffer.
	for d.nbuf < 4 && d.readErr == nil {
		nn := len(p) / 3 * 4
		if nn < 4 {
			nn = 4
		}
		if nn > len(d.buf) {
			nn = len(d.buf)
		}
		nn, d.readErr = d.r.Read(d.buf[d.nbuf:nn])
		d.nbuf += nn
	}

	if d.nbuf < 4 {
		if d.enc.padChar == NoPadding && d.nbuf > 0 {
			// Decode final fragment, without padding.
			var nw int
			nw, d.err = d.enc.Decode(d.outbuf[:], d.buf[:d.nbuf])
			d.nbuf = 0
			d.out = d.outbuf[:nw]
			n = copy(p, d.out)
			d.out = d.out[n:]
			if n > 0 || len(p) == 0 && len(d.out) > 0 {
				return n, nil
			}
			if d.err != nil {
				return 0, d.err
			}
		}
		d.err = d.readErr
		if d.err == io.EOF && d.nbuf > 0 {
			d.err = io.ErrUnexpectedEOF
		}
		return 0, d.err
	}

	// Decode chunk into p, or d.out and then p if p is too small.
	nr := d.nbuf / 4 * 4
	nw := d.nbuf / 4 * 3
	if nw > len(p) {
		nw, d.err = d.enc.Decode(d.outbuf[:], d.buf[:nr])
		d.out = d.outbuf[:nw]
		n = copy(p, d.out)
		d.out = d.out[n:]
	} else {
		n, d.err = d.enc.Decode(p, d.buf[:nr])
	}
	d.nbuf -= nr
	copy(d.buf[:d.nbuf], d.buf[nr:])
	return n, d.err
}

// Decode decodes src using the encoding enc. It writes at most
// DecodedLen(len(src)) bytes to dst and returns the number of bytes
// written. If src contains invalid base62 data, it will return the
// number of bytes successfully written and CorruptInputError.
// New line characters (\r and \n) are ignored.
func (enc *Encoding) Decode(dst, src []byte) (n int, err error) {
	if len(src) == 0 {
		return 0, nil
	}

	// Lift the nil check outside of the loop. enc.decodeMap is directly
	// used later in this function, to let the compiler know that the
	// receiver can't be nil.
	_ = enc.decodeMap

	si := 0
	for strconv.IntSize >= 64 && len(src)-si >= 8 && len(dst)-n >= 8 {
		src2 := src[si : si+8]
		if dn, ok := assemble64(
			enc.decodeMap[src2[0]],
			enc.decodeMap[src2[1]],
			enc.decodeMap[src2[2]],
			enc.decodeMap[src2[3]],
			enc.decodeMap[src2[4]],
			enc.decodeMap[src2[5]],
			enc.decodeMap[src2[6]],
			enc.decodeMap[src2[7]],
		); ok {
			binary.BigEndian.PutUint64(dst[n:], dn)
			n += 6
			si += 8
		} else {
			var ninc int
			si, ninc, err = enc.decodeQuantum(dst[n:], src, si)
			n += ninc
			if err != nil {
				return n, err
			}
		}
	}

	for len(src)-si >= 4 && len(dst)-n >= 4 {
		src2 := src[si : si+4]
		if dn, ok := assemble32(
			enc.decodeMap[src2[0]],
			enc.decodeMap[src2[1]],
			enc.decodeMap[src2[2]],
			enc.decodeMap[src2[3]],
		); ok {
			binary.BigEndian.PutUint32(dst[n:], dn)
			n += 3
			si += 4
		} else {
			var ninc int
			si, ninc, err = enc.decodeQuantum(dst[n:], src, si)
			n += ninc
			if err != nil {
				return n, err
			}
		}
	}

	for si < len(src) {
		var ninc int
		si, ninc, err = enc.decodeQuantum(dst[n:], src, si)
		n += ninc
		if err != nil {
			return n, err
		}
	}
	return n, err
}

// assemble32 assembles 4 base62 digits into 3 bytes.
// Each digit comes from the decode map, and will be 0xff
// if it came from an invalid character.
func assemble32(n1, n2, n3, n4 byte) (dn uint32, ok bool) {
	// Check that all the digits are valid. If any of them was 0xff, their
	// bitwise OR will be 0xff.
	if n1|n2|n3|n4 == 0xff {
		return 0, false
	}
	return uint32(n1)<<26 |
			uint32(n2)<<20 |
			uint32(n3)<<14 |
			uint32(n4)<<8,
		true
}

// assemble64 assembles 8 base62 digits into 6 bytes.
// Each digit comes from the decode map, and will be 0xff
// if it came from an invalid character.
func assemble64(n1, n2, n3, n4, n5, n6, n7, n8 byte) (dn uint64, ok bool) {
	// Check that all the digits are valid. If any of them was 0xff, their
	// bitwise OR will be 0xff.
	if n1|n2|n3|n4|n5|n6|n7|n8 == 0xff {
		return 0, false
	}
	return uint64(n1)<<58 |
			uint64(n2)<<52 |
			uint64(n3)<<46 |
			uint64(n4)<<40 |
			uint64(n5)<<34 |
			uint64(n6)<<28 |
			uint64(n7)<<22 |
			uint64(n8)<<16,
		true
}

type newlineFilteringReader struct {
	wrapped io.Reader
}

func (r *newlineFilteringReader) Read(p []byte) (int, error) {
	n, err := r.wrapped.Read(p)
	for n > 0 {
		offset := 0
		for i, b := range p[:n] {
			if b != '\r' && b != '\n' {
				if i != offset {
					p[offset] = b
				}
				offset++
			}
		}
		if offset > 0 {
			return offset, err
		}
		// Previous buffer entirely whitespace, read again
		n, err = r.wrapped.Read(p)
	}
	return n, err
}

// NewDecoder constructs a new base62 stream decoder.
func NewDecoder(enc *Encoding, r io.Reader) io.Reader {
	return &decoder{enc: enc, r: &newlineFilteringReader{r}}
}

// DecodedLen returns the maximum length in bytes of the decoded data
// corresponding to n bytes of base62-encoded data.
func (enc *Encoding) DecodedLen(n int) int {
	return n * 6 / 8
}
