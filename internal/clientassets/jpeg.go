package clientassets

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
)

// DecodeJPEG decodes a JPEG exactly as the original client does. Delphi's
// jpeg unit links IJG libjpeg 6, whose integer IDCT (jidctint.c), YCbCr tables
// (jdcolor.c) and "fancy" chroma upsampling (jdsample.c) round differently
// from image/jpeg. Baseline images are decoded here bit-exactly; other
// encodings fall back to image/jpeg with IJG colour conversion.
func DecodeJPEG(data []byte) (*image.RGBA, error) {
	d := &jpegDecoder{data: data}
	err := d.decode()
	if errors.Is(err, errUnsupportedJPEG) {
		return decodeJPEGFallback(data)
	}
	if err != nil {
		return nil, err
	}
	return d.rgba()
}

var errUnsupportedJPEG = errors.New("jpeg: unsupported encoding")

type jpegComponent struct {
	id, h, v, tq    int
	td, ta          int
	bw, bh          int // blocks per line/column, padded to whole MCUs
	pixels          []byte
	stride          int
	pred            int
	dsWidth, dsRows int
}

type huffman struct {
	lookup map[uint32]byte // (length<<16 | code) -> value
	maxLen int
}

type jpegDecoder struct {
	data         []byte
	pos          int
	width        int
	height       int
	comps        []jpegComponent
	quant        [4][64]int
	dc, ac       [4]*huffman
	restart      int
	hmax, vmax   int
	bits, nbits  uint32
	markerHit    bool
	frameStarted bool
}

var zigzag = [64]int{
	0, 1, 8, 16, 9, 2, 3, 10, 17, 24, 32, 25, 18, 11, 4, 5,
	12, 19, 26, 33, 40, 48, 41, 34, 27, 20, 13, 6, 7, 14, 21, 28,
	35, 42, 49, 56, 57, 50, 43, 36, 29, 22, 15, 23, 30, 37, 44, 51,
	58, 59, 52, 45, 38, 31, 39, 46, 53, 60, 61, 54, 47, 55, 62, 63,
}

func (d *jpegDecoder) u16(at int) int { return int(d.data[at])<<8 | int(d.data[at+1]) }

func (d *jpegDecoder) decode() error {
	if len(d.data) < 4 || d.data[0] != 0xff || d.data[1] != 0xd8 {
		return errors.New("jpeg: missing SOI")
	}
	d.pos = 2
	for {
		for d.pos < len(d.data) && d.data[d.pos] != 0xff {
			d.pos++
		}
		for d.pos < len(d.data) && d.data[d.pos] == 0xff {
			d.pos++
		}
		if d.pos >= len(d.data) {
			return errors.New("jpeg: missing EOI")
		}
		marker := d.data[d.pos]
		d.pos++
		if marker == 0xd9 {
			return nil
		}
		if marker >= 0xd0 && marker <= 0xd7 || marker == 0x01 {
			continue
		}
		if d.pos+2 > len(d.data) {
			return errors.New("jpeg: truncated segment")
		}
		n := d.u16(d.pos)
		if n < 2 || d.pos+n > len(d.data) {
			return errors.New("jpeg: bad segment length")
		}
		seg := d.data[d.pos+2 : d.pos+n]
		d.pos += n
		var err error
		switch marker {
		case 0xc0, 0xc1:
			err = d.frame(seg)
		case 0xc2, 0xc3, 0xc5, 0xc6, 0xc7, 0xc9, 0xca, 0xcb, 0xcd, 0xce, 0xcf:
			return errUnsupportedJPEG
		case 0xc4:
			err = d.huffmanTables(seg)
		case 0xdb:
			err = d.quantTables(seg)
		case 0xdd:
			if len(seg) < 2 {
				return errors.New("jpeg: bad DRI")
			}
			d.restart = int(seg[0])<<8 | int(seg[1])
		case 0xda:
			err = d.scan(seg)
		}
		if err != nil {
			return err
		}
	}
}

func (d *jpegDecoder) frame(seg []byte) error {
	if len(seg) < 6 || seg[0] != 8 {
		return errUnsupportedJPEG
	}
	d.height, d.width = int(seg[1])<<8|int(seg[2]), int(seg[3])<<8|int(seg[4])
	n := int(seg[5])
	if d.width == 0 || d.height == 0 || (n != 1 && n != 3) || len(seg) < 6+3*n {
		return errUnsupportedJPEG
	}
	if d.width*d.height > 64<<20 {
		return errors.New("jpeg: image too large")
	}
	d.hmax, d.vmax = 1, 1
	for i := range n {
		c := jpegComponent{id: int(seg[6+3*i]), h: int(seg[7+3*i] >> 4), v: int(seg[7+3*i] & 15), tq: int(seg[8+3*i] & 3)}
		if c.h < 1 || c.h > 2 || c.v < 1 || c.v > 2 {
			return errUnsupportedJPEG
		}
		d.hmax, d.vmax = max(d.hmax, c.h), max(d.vmax, c.v)
		d.comps = append(d.comps, c)
	}
	mcuW, mcuH := (d.width+8*d.hmax-1)/(8*d.hmax), (d.height+8*d.vmax-1)/(8*d.vmax)
	for i := range d.comps {
		c := &d.comps[i]
		if n == 1 {
			c.h, c.v, d.hmax, d.vmax = 1, 1, 1, 1
			mcuW, mcuH = (d.width+7)/8, (d.height+7)/8
		}
		c.bw, c.bh = mcuW*c.h, mcuH*c.v
		c.stride = c.bw * 8
		c.pixels = make([]byte, c.stride*c.bh*8)
		c.dsWidth = (d.width*c.h + d.hmax - 1) / d.hmax
		c.dsRows = (d.height*c.v + d.vmax - 1) / d.vmax
	}
	d.frameStarted = true
	return nil
}

func (d *jpegDecoder) quantTables(seg []byte) error {
	for len(seg) > 0 {
		precision, id := seg[0]>>4, seg[0]&3
		size := 64 << precision
		if len(seg) < 1+size {
			return errors.New("jpeg: truncated DQT")
		}
		for i := range 64 {
			if precision == 0 {
				d.quant[id][zigzag[i]] = int(seg[1+i])
			} else {
				d.quant[id][zigzag[i]] = int(seg[1+2*i])<<8 | int(seg[2+2*i])
			}
		}
		seg = seg[1+size:]
	}
	return nil
}

func (d *jpegDecoder) huffmanTables(seg []byte) error {
	for len(seg) > 0 {
		if len(seg) < 17 {
			return errors.New("jpeg: truncated DHT")
		}
		class, id := seg[0]>>4, seg[0]&3
		counts := seg[1:17]
		total := 0
		for _, c := range counts {
			total += int(c)
		}
		if len(seg) < 17+total {
			return errors.New("jpeg: truncated DHT")
		}
		vals := seg[17 : 17+total]
		h := &huffman{lookup: map[uint32]byte{}}
		code, k := uint32(0), 0
		for l := 1; l <= 16; l++ {
			for range counts[l-1] {
				h.lookup[uint32(l)<<16|code] = vals[k]
				k++
				code++
				h.maxLen = l
			}
			code <<= 1
		}
		if class == 0 {
			d.dc[id] = h
		} else {
			d.ac[id] = h
		}
		seg = seg[17+total:]
	}
	return nil
}

// readBit returns the next entropy-coded bit; past a marker it feeds zeros
// as libjpeg does for corrupt data.
func (d *jpegDecoder) readBit() uint32 {
	if d.nbits == 0 {
		var b byte
		if !d.markerHit && d.pos < len(d.data) {
			b = d.data[d.pos]
			if b == 0xff {
				if d.pos+1 < len(d.data) && d.data[d.pos+1] == 0 {
					d.pos += 2
				} else {
					d.markerHit = true
					b = 0
				}
			} else {
				d.pos++
			}
		}
		d.bits, d.nbits = uint32(b), 8
	}
	d.nbits--
	return d.bits >> d.nbits & 1
}

func (d *jpegDecoder) receive(n int) int {
	v := 0
	for range n {
		v = v<<1 | int(d.readBit())
	}
	return v
}

func extend(v, n int) int {
	if n == 0 {
		return 0
	}
	if v < 1<<(n-1) {
		return v - (1 << n) + 1
	}
	return v
}

func (d *jpegDecoder) symbol(h *huffman) (byte, error) {
	if h == nil {
		return 0, errors.New("jpeg: missing Huffman table")
	}
	code := uint32(0)
	for l := 1; l <= h.maxLen; l++ {
		code = code<<1 | d.readBit()
		if v, ok := h.lookup[uint32(l)<<16|code]; ok {
			return v, nil
		}
	}
	return 0, errors.New("jpeg: bad Huffman code")
}

func (d *jpegDecoder) scan(seg []byte) error {
	if !d.frameStarted || len(seg) < 1 {
		return errors.New("jpeg: SOS before SOF")
	}
	n := int(seg[0])
	if n != len(d.comps) || len(seg) < 1+2*n+3 {
		return errUnsupportedJPEG // non-interleaved multi-scan baseline
	}
	for i := range n {
		id, t := int(seg[1+2*i]), seg[2+2*i]
		found := false
		for j := range d.comps {
			if d.comps[j].id == id {
				d.comps[j].td, d.comps[j].ta = int(t>>4), int(t&3)
				found = true
			}
		}
		if !found {
			return errors.New("jpeg: unknown scan component")
		}
	}
	mcuW, mcuH := d.comps[0].bw/d.comps[0].h, d.comps[0].bh/d.comps[0].v
	var coef [64]int
	var block [64]byte
	mcus := 0
	for my := range mcuH {
		for mx := range mcuW {
			if d.restart > 0 && mcus > 0 && mcus%d.restart == 0 {
				d.resync()
			}
			mcus++
			for ci := range d.comps {
				c := &d.comps[ci]
				for by := range c.v {
					for bx := range c.h {
						if err := d.block(c, &coef); err != nil {
							return err
						}
						idctIslow(&coef, &d.quant[c.tq], &block)
						x0, y0 := (mx*c.h+bx)*8, (my*c.v+by)*8
						for r := range 8 {
							copy(c.pixels[(y0+r)*c.stride+x0:], block[r*8:r*8+8])
						}
					}
				}
			}
		}
	}
	return nil
}

func (d *jpegDecoder) resync() {
	d.nbits = 0
	d.markerHit = false
	for d.pos+1 < len(d.data) && !(d.data[d.pos] == 0xff && d.data[d.pos+1] >= 0xd0 && d.data[d.pos+1] <= 0xd7) {
		d.pos++
	}
	d.pos += 2
	for i := range d.comps {
		d.comps[i].pred = 0
	}
}

func (d *jpegDecoder) block(c *jpegComponent, coef *[64]int) error {
	*coef = [64]int{}
	s, err := d.symbol(d.dc[c.td])
	if err != nil {
		return err
	}
	c.pred += extend(d.receive(int(s)), int(s))
	coef[0] = c.pred
	for k := 1; k < 64; {
		rs, err := d.symbol(d.ac[c.ta])
		if err != nil {
			return err
		}
		r, s := int(rs>>4), int(rs&15)
		if s == 0 {
			if r != 15 {
				break
			}
			k += 16
			continue
		}
		k += r
		if k > 63 {
			break
		}
		coef[zigzag[k]] = extend(d.receive(s), s)
		k++
	}
	return nil
}

// idctIslow is libjpeg 6b jidctint.c jpeg_idct_islow, including its
// range-limit table (outputs wrap modulo 1024 before clamping).
func idctIslow(in *[64]int, q *[64]int, out *[64]byte) {
	const (
		constBits = 13
		pass1Bits = 2
		f0298     = 2446
		f0390     = 3196
		f0541     = 4433
		f0765     = 6270
		f0899     = 7373
		f1175     = 9633
		f1501     = 12299
		f1847     = 15137
		f1961     = 16069
		f2053     = 16819
		f2562     = 20995
		f3072     = 25172
	)
	descale := func(x, n int) int { return (x + 1<<(n-1)) >> n }
	var ws [64]int
	for col := range 8 {
		dq := func(r int) int { return in[r*8+col] * q[r*8+col] }
		if in[8+col] == 0 && in[16+col] == 0 && in[24+col] == 0 && in[32+col] == 0 &&
			in[40+col] == 0 && in[48+col] == 0 && in[56+col] == 0 {
			dc := dq(0) << pass1Bits
			for r := range 8 {
				ws[r*8+col] = dc
			}
			continue
		}
		z2, z3 := dq(2), dq(6)
		z1 := (z2 + z3) * f0541
		tmp2 := z1 + z3*(-f1847)
		tmp3 := z1 + z2*f0765
		z2, z3 = dq(0), dq(4)
		tmp0 := (z2 + z3) << constBits
		tmp1 := (z2 - z3) << constBits
		tmp10, tmp13, tmp11, tmp12 := tmp0+tmp3, tmp0-tmp3, tmp1+tmp2, tmp1-tmp2
		tmp0, tmp1, tmp2, tmp3 = dq(7), dq(5), dq(3), dq(1)
		z1, z2, z3 = tmp0+tmp3, tmp1+tmp2, tmp0+tmp2
		z4 := tmp1 + tmp3
		z5 := (z3 + z4) * f1175
		tmp0 *= f0298
		tmp1 *= f2053
		tmp2 *= f3072
		tmp3 *= f1501
		z1 *= -f0899
		z2 *= -f2562
		z3 = z3*(-f1961) + z5
		z4 = z4*(-f0390) + z5
		tmp0 += z1 + z3
		tmp1 += z2 + z4
		tmp2 += z2 + z3
		tmp3 += z1 + z4
		n := constBits - pass1Bits
		ws[0*8+col] = descale(tmp10+tmp3, n)
		ws[7*8+col] = descale(tmp10-tmp3, n)
		ws[1*8+col] = descale(tmp11+tmp2, n)
		ws[6*8+col] = descale(tmp11-tmp2, n)
		ws[2*8+col] = descale(tmp12+tmp1, n)
		ws[5*8+col] = descale(tmp12-tmp1, n)
		ws[3*8+col] = descale(tmp13+tmp0, n)
		ws[4*8+col] = descale(tmp13-tmp0, n)
	}
	limit := func(v int) byte {
		v &= 1023
		if v >= 512 {
			v -= 1024
		}
		return clamp8(v + 128)
	}
	for row := range 8 {
		w := ws[row*8 : row*8+8]
		z2, z3 := w[2], w[6]
		z1 := (z2 + z3) * f0541
		tmp2 := z1 + z3*(-f1847)
		tmp3 := z1 + z2*f0765
		tmp0 := (w[0] + w[4]) << constBits
		tmp1 := (w[0] - w[4]) << constBits
		tmp10, tmp13, tmp11, tmp12 := tmp0+tmp3, tmp0-tmp3, tmp1+tmp2, tmp1-tmp2
		tmp0, tmp1, tmp2, tmp3 = w[7], w[5], w[3], w[1]
		z1, z2, z3 = tmp0+tmp3, tmp1+tmp2, tmp0+tmp2
		z4 := tmp1 + tmp3
		z5 := (z3 + z4) * f1175
		tmp0 *= f0298
		tmp1 *= f2053
		tmp2 *= f3072
		tmp3 *= f1501
		z1 *= -f0899
		z2 *= -f2562
		z3 = z3*(-f1961) + z5
		z4 = z4*(-f0390) + z5
		tmp0 += z1 + z3
		tmp1 += z2 + z4
		tmp2 += z2 + z3
		tmp3 += z1 + z4
		n := constBits + pass1Bits + 3
		o := out[row*8 : row*8+8]
		o[0] = limit(descale(tmp10+tmp3, n))
		o[7] = limit(descale(tmp10-tmp3, n))
		o[1] = limit(descale(tmp11+tmp2, n))
		o[6] = limit(descale(tmp11-tmp2, n))
		o[2] = limit(descale(tmp12+tmp1, n))
		o[5] = limit(descale(tmp12-tmp1, n))
		o[3] = limit(descale(tmp13+tmp0, n))
		o[4] = limit(descale(tmp13-tmp0, n))
	}
}

func (d *jpegDecoder) rgba() (*image.RGBA, error) {
	w, h := d.width, d.height
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	full := func(c *jpegComponent) ([]byte, error) {
		p := plane(c.pixels, c.stride, c.dsWidth, c.dsRows)
		switch {
		case c.h == d.hmax && c.v == d.vmax:
			return plane(c.pixels, c.stride, w, h), nil
		case 2*c.h == d.hmax && 2*c.v == d.vmax:
			return upsampleH2V2(p, c.dsWidth, c.dsRows, w, h), nil
		case 2*c.h == d.hmax && c.v == d.vmax:
			return upsampleH2V1(p, c.dsWidth, h, w), nil
		}
		return nil, errUnsupportedJPEG
	}
	y := plane(d.comps[0].pixels, d.comps[0].stride, w, h)
	if len(d.comps) == 1 {
		for i, v := range y {
			out.Pix[4*i], out.Pix[4*i+1], out.Pix[4*i+2], out.Pix[4*i+3] = v, v, v, 255
		}
		return out, nil
	}
	cb, err := full(&d.comps[1])
	if err != nil {
		return nil, err
	}
	cr, err := full(&d.comps[2])
	if err != nil {
		return nil, err
	}
	yccToRGBA(out, y, cb, cr)
	return out, nil
}

func yccToRGBA(out *image.RGBA, y, cb, cr []byte) {
	for i := range y {
		yy, b, r := int(y[i]), int(cb[i]), int(cr[i])
		out.Pix[4*i] = clamp8(yy + crR[r])
		out.Pix[4*i+1] = clamp8(yy + (cbG[b]+crG[r])>>16)
		out.Pix[4*i+2] = clamp8(yy + cbB[b])
		out.Pix[4*i+3] = 255
	}
}

func decodeJPEGFallback(data []byte) (*image.RGBA, error) {
	m, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	b := m.Bounds()
	w, h := b.Dx(), b.Dy()
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	src, ok := m.(*image.YCbCr)
	if !ok {
		for y := range h {
			for x := range w {
				out.Set(x, y, m.At(b.Min.X+x, b.Min.Y+y))
			}
		}
		return out, nil
	}
	y := plane(src.Y, src.YStride, w, h)
	var cb, cr []byte
	switch src.SubsampleRatio {
	case image.YCbCrSubsampleRatio444:
		cb, cr = plane(src.Cb, src.CStride, w, h), plane(src.Cr, src.CStride, w, h)
	case image.YCbCrSubsampleRatio420:
		cw, ch := (w+1)/2, (h+1)/2
		cb = upsampleH2V2(plane(src.Cb, src.CStride, cw, ch), cw, ch, w, h)
		cr = upsampleH2V2(plane(src.Cr, src.CStride, cw, ch), cw, ch, w, h)
	case image.YCbCrSubsampleRatio422:
		cw := (w + 1) / 2
		cb = upsampleH2V1(plane(src.Cb, src.CStride, cw, h), cw, h, w)
		cr = upsampleH2V1(plane(src.Cr, src.CStride, cw, h), cw, h, w)
	default:
		return nil, fmt.Errorf("jpeg: unsupported chroma subsampling %v", src.SubsampleRatio)
	}
	yccToRGBA(out, y, cb, cr)
	return out, nil
}

func plane(p []byte, stride, w, h int) []byte {
	out := make([]byte, w*h)
	for y := range h {
		copy(out[y*w:], p[y*stride:y*stride+w])
	}
	return out
}

// jdcolor.c build_ycc_rgb_table.
var crR, cbB, crG, cbG [256]int

func init() {
	fix := func(x float64) int { return int(x*65536 + 0.5) }
	for i := range 256 {
		x := i - 128
		crR[i] = (fix(1.40200)*x + 1<<15) >> 16
		cbB[i] = (fix(1.77200)*x + 1<<15) >> 16
		crG[i] = -fix(0.71414) * x
		cbG[i] = -fix(0.34414)*x + 1<<15
	}
}

func clamp8(v int) byte { return byte(min(max(v, 0), 255)) }

// upsampleRow is jdsample.c's horizontal triangle filter over column sums
// (scaled by 4 for h2v2); out has w samples.
func upsampleRow(sum []int, out []byte, w int, shift uint, bias0, bias1, scale int) {
	n := len(sum)
	put := func(i, v int) {
		if i < w {
			out[i] = byte(v >> shift)
		}
	}
	if n == 1 {
		put(0, sum[0]*scale+bias0)
		put(1, sum[0]*scale+bias1)
		return
	}
	put(0, sum[0]*scale+bias0)
	put(1, sum[0]*3+sum[1]+bias1)
	for i := 1; i < n-1; i++ {
		put(2*i, sum[i]*3+sum[i-1]+bias0)
		put(2*i+1, sum[i]*3+sum[i+1]+bias1)
	}
	put(2*n-2, sum[n-1]*3+sum[n-2]+bias0)
	put(2*n-1, sum[n-1]*scale+bias1)
}

// upsampleH2V2 is h2v2_fancy_upsample; edge rows are replicated.
func upsampleH2V2(p []byte, cw, ch, w, h int) []byte {
	out := make([]byte, w*h)
	sum := make([]int, cw)
	for y := range h {
		row := min(y/2, ch-1)
		near := row - 1 // even output rows blend with the row above
		if y%2 == 1 {
			near = row + 1
		}
		near = min(max(near, 0), ch-1)
		for x := range cw {
			sum[x] = int(p[row*cw+x])*3 + int(p[near*cw+x])
		}
		upsampleRow(sum, out[y*w:(y+1)*w], w, 4, 8, 7, 4)
	}
	return out
}

// upsampleH2V1 is h2v1_fancy_upsample.
func upsampleH2V1(p []byte, cw, h, w int) []byte {
	out := make([]byte, w*h)
	sum := make([]int, cw)
	for y := range h {
		for x := range cw {
			sum[x] = int(p[y*cw+x])
		}
		upsampleRow(sum, out[y*w:(y+1)*w], w, 2, 1, 2, 4)
	}
	return out
}
