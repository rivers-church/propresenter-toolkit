package pdftext

import (
	"math"

	"github.com/ledongthuc/pdf"
)

// The pdf library's Page.Content() gives each glyph's text, font and
// position but ignores colour, and its text decoders aren't exported. So we
// re-walk the content stream here purely to learn the fill colour in effect
// at each glyph position. The position maths deliberately mirrors the
// library's (including what it leaves out, like word spacing) so the two
// line up exactly; glyphs are then matched to colours by position.

type matrix [3][3]float64

var ident = matrix{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}

func (x matrix) mul(y matrix) matrix {
	var z matrix
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			for k := 0; k < 3; k++ {
				z[i][j] += x[i][k] * y[k][j]
			}
		}
	}
	return z
}

// rgb is a fill colour with components in 0..1.
type rgb [3]float64

// Black is PDF's default fill colour.
var black = rgb{0, 0, 0}

type posKey struct{ x, y int64 }

func keyAt(x, y float64) posKey {
	return posKey{int64(math.Round(x * 100)), int64(math.Round(y * 100))}
}

type colorState struct {
	Tc, Th, Tl, Tfs, Trise float64
	Tf                     pdf.Font
	Tm, Tlm, CTM           matrix
	fill                   rgb
}

// colorArgs turns the operands of a fill-colour operator into RGB,
// inferring the colour space from the component count (1 gray, 3 RGB,
// 4 CMYK). Pattern colours and the like are ignored.
func colorArgs(args []pdf.Value) (rgb, bool) {
	var n []float64
	for _, a := range args {
		if a.Kind() == pdf.Integer || a.Kind() == pdf.Real {
			n = append(n, a.Float64())
		}
	}
	switch len(n) {
	case 1:
		return rgb{n[0], n[0], n[0]}, true
	case 3:
		return rgb{n[0], n[1], n[2]}, true
	case 4:
		c, m, y, k := n[0], n[1], n[2], n[3]
		return rgb{(1 - c) * (1 - k), (1 - m) * (1 - k), (1 - y) * (1 - k)}, true
	}
	return rgb{}, false
}

func popArgs(stk *pdf.Stack) []pdf.Value {
	n := stk.Len()
	args := make([]pdf.Value, n)
	for i := n - 1; i >= 0; i-- {
		args[i] = stk.Pop()
	}
	return args
}

func floats(args []pdf.Value, n int) ([]float64, bool) {
	if len(args) != n {
		return nil, false
	}
	out := make([]float64, n)
	for i, a := range args {
		out[i] = a.Float64()
	}
	return out, true
}

func (m matrix) apply(x, y float64) (float64, float64) {
	return x*m[0][0] + y*m[1][0] + m[2][0], x*m[0][1] + y*m[1][1] + m[2][1]
}

// bbox is a bounding box that grows as points are added.
type bbox struct {
	set                    bool
	minX, minY, maxX, maxY float64
}

func (b *bbox) add(x, y float64) {
	if !b.set {
		*b = bbox{true, x, y, x, y}
		return
	}
	b.minX, b.maxX = math.Min(b.minX, x), math.Max(b.maxX, x)
	b.minY, b.maxY = math.Min(b.minY, y), math.Max(b.maxY, y)
}

func (b bbox) area() float64 { return (b.maxX - b.minX) * (b.maxY - b.minY) }

// brightness is the strongest channel, 0 (black) to 1: pure red counts as
// bright, dark grey as dark.
func (c rgb) brightness() float64 { return math.Max(c[0], math.Max(c[1], c[2])) }

// pageColors maps each glyph position on the page to its fill colour, and
// reports the page background: the colour of the last filled rectangle
// covering (nearly) the whole page, or white if there is none.
func pageColors(p pdf.Page) (map[posKey]rgb, rgb) {
	colors := map[posKey]rgb{}
	background := rgb{1, 1, 1}
	strm := p.V.Key("Contents")
	if strm.Kind() == pdf.Null {
		return colors, background
	}
	pageArea := 0.0
	if mb := p.V.Key("MediaBox"); mb.Len() == 4 {
		pageArea = (mb.Index(2).Float64() - mb.Index(0).Float64()) * (mb.Index(3).Float64() - mb.Index(1).Float64())
	}
	g := colorState{Th: 1, CTM: ident, Tm: ident, Tlm: ident, fill: black}
	var stack []colorState
	var path bbox // extent of the current path

	show := func(s string) {
		for i := 0; i < len(s); i++ {
			trm := matrix{{g.Tfs * g.Th, 0, 0}, {0, g.Tfs, 0}, {0, g.Trise, 1}}.mul(g.Tm).mul(g.CTM)
			colors[keyAt(trm[2][0], trm[2][1])] = g.fill
			w0 := g.Tf.Width(int(s[i]))
			tx := (w0/1000*g.Tfs + g.Tc) * g.Th
			g.Tm = matrix{{1, 0, 0}, {0, 1, 0}, {tx, 0, 1}}.mul(g.Tm)
		}
	}
	nextLine := func() {
		g.Tlm = matrix{{1, 0, 0}, {0, 1, 0}, {0, -g.Tl, 1}}.mul(g.Tlm)
		g.Tm = g.Tlm
	}

	pdf.Interpret(strm, func(stk *pdf.Stack, op string) {
		args := popArgs(stk)
		switch op {
		case "q":
			stack = append(stack, g)
		case "Q":
			if n := len(stack); n > 0 {
				g, stack = stack[n-1], stack[:n-1]
			}
		case "cm":
			if f, ok := floats(args, 6); ok {
				g.CTM = matrix{{f[0], f[1], 0}, {f[2], f[3], 0}, {f[4], f[5], 1}}.mul(g.CTM)
			}
		case "g", "rg", "k", "sc", "scn": // fill colour; stroking colour doesn't affect text
			if c, ok := colorArgs(args); ok {
				g.fill = c
			}
		// Paths: track the bounding box of the current path, so a fill that
		// covers the page (drawn as a rectangle or as a four-point path)
		// can be recognised as the page background.
		case "re":
			if f, ok := floats(args, 4); ok {
				path.add(g.CTM.apply(f[0], f[1]))
				path.add(g.CTM.apply(f[0]+f[2], f[1]+f[3]))
			}
		case "m", "l":
			if f, ok := floats(args, 2); ok {
				path.add(g.CTM.apply(f[0], f[1]))
			}
		case "c":
			if f, ok := floats(args, 6); ok {
				path.add(g.CTM.apply(f[4], f[5]))
			}
		case "v", "y":
			if f, ok := floats(args, 4); ok {
				path.add(g.CTM.apply(f[2], f[3]))
			}
		case "f", "F", "f*", "B", "B*", "b", "b*":
			if pageArea > 0 && path.area() >= 0.9*pageArea {
				background = g.fill
			}
			path = bbox{}
		case "n", "S", "s":
			path = bbox{}
		case "BT":
			g.Tm, g.Tlm = ident, ident
		case "Tc":
			if f, ok := floats(args, 1); ok {
				g.Tc = f[0]
			}
		case "Tz":
			if f, ok := floats(args, 1); ok {
				g.Th = f[0] / 100
			}
		case "TL":
			if f, ok := floats(args, 1); ok {
				g.Tl = f[0]
			}
		case "Ts":
			if f, ok := floats(args, 1); ok {
				g.Trise = f[0]
			}
		case "Tf":
			if len(args) == 2 {
				g.Tf = p.Font(args[0].Name())
				g.Tfs = args[1].Float64()
			}
		case "TD", "Td":
			if f, ok := floats(args, 2); ok {
				if op == "TD" {
					g.Tl = -f[1]
				}
				g.Tlm = matrix{{1, 0, 0}, {0, 1, 0}, {f[0], f[1], 1}}.mul(g.Tlm)
				g.Tm = g.Tlm
			}
		case "Tm":
			if f, ok := floats(args, 6); ok {
				g.Tm = matrix{{f[0], f[1], 0}, {f[2], f[3], 0}, {f[4], f[5], 1}}
				g.Tlm = g.Tm
			}
		case "T*":
			nextLine()
		case "Tj":
			if len(args) == 1 {
				show(args[0].RawString())
			}
		case "'":
			if len(args) == 1 {
				nextLine()
				show(args[0].RawString())
			}
		case "\"":
			if len(args) == 3 {
				g.Tc = args[1].Float64()
				nextLine()
				show(args[2].RawString())
			}
		case "TJ":
			if len(args) == 1 {
				v := args[0]
				for i := 0; i < v.Len(); i++ {
					x := v.Index(i)
					if x.Kind() == pdf.String {
						show(x.RawString())
					} else {
						tx := -x.Float64() / 1000 * g.Tfs * g.Th
						g.Tm = matrix{{1, 0, 0}, {0, 1, 0}, {tx, 0, 1}}.mul(g.Tm)
					}
				}
			}
		}
	})
	return colors, background
}
