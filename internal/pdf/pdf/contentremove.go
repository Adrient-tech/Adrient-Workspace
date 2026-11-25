package pdf

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/font"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// RemoveRegion is an area of one page whose content must be removed from the
// file, not just painted over. Page is 1-based; X, Y, W, H are in the page's
// default user space (points, origin bottom-left), the space pdf.js reports
// through viewport.convertToPdfPoint.
//
// A redaction region (the default) removes glyphs, vector paths drawn inside
// it and the covered pixels of images. A GlyphsOnly region (an in-place text
// edit) removes only the glyphs and leaves drawings and images alone.
type RemoveRegion struct {
	Page       int     `json:"page"`
	X          float64 `json:"x"`
	Y          float64 `json:"y"`
	W          float64 `json:"w"`
	H          float64 `json:"h"`
	GlyphsOnly bool    `json:"glyphsOnly,omitempty"`
}

// RemovePageStats reports what was removed from one page. Unhandled counts
// form XObjects that overlap a region: their inner content is not rewritten,
// so a caller that needs a guaranteed removal should flatten that page.
type RemovePageStats struct {
	Page          int `json:"page"`
	GlyphsRemoved int `json:"glyphsRemoved"`
	ImagesRemoved int `json:"imagesRemoved"`
	ImagesClipped int `json:"imagesClipped"`
	PathsRemoved  int `json:"pathsRemoved"`
	Unhandled     int `json:"unhandled"`
}

func (s RemovePageStats) changed() bool {
	return s.GlyphsRemoved+s.ImagesRemoved+s.ImagesClipped+s.PathsRemoved > 0
}

// RemoveContent deletes every glyph whose centre lies inside a region and
// every image that overlaps a region, rewriting the page content streams.
// Remaining glyphs keep their exact positions (removed glyphs become
// positioning offsets in a TJ array). Untouched operations are copied
// byte-for-byte.
func RemoveContent(in []byte, regions []RemoveRegion) ([]byte, []RemovePageStats, error) {
	ctx, err := api.ReadContext(bytes.NewReader(in), model.NewDefaultConfiguration())
	if err != nil {
		return nil, nil, fmt.Errorf("read pdf: %w", err)
	}
	if err := ctx.EnsurePageCount(); err != nil {
		return nil, nil, fmt.Errorf("page count: %w", err)
	}
	type pageRegions struct{ glyph, redact []rect }
	byPage := map[int]*pageRegions{}
	for _, r := range regions {
		if r.Page < 1 || r.Page > ctx.PageCount {
			return nil, nil, fmt.Errorf("region page %d out of range 1..%d", r.Page, ctx.PageCount)
		}
		if r.W <= 0 || r.H <= 0 {
			continue
		}
		pr := byPage[r.Page]
		if pr == nil {
			pr = &pageRegions{}
			byPage[r.Page] = pr
		}
		rc := rect{r.X, r.Y, r.X + r.W, r.Y + r.H}
		pr.glyph = append(pr.glyph, rc)
		if !r.GlyphsOnly {
			pr.redact = append(pr.redact, rc)
		}
	}
	var stats []RemovePageStats
	for pageNr := 1; pageNr <= ctx.PageCount; pageNr++ {
		pr := byPage[pageNr]
		if pr == nil {
			continue
		}
		d, _, inh, err := ctx.PageDict(pageNr, false)
		if err != nil || d == nil {
			return nil, nil, fmt.Errorf("page %d: %v", pageNr, err)
		}
		content, err := ctx.PageContent(d, pageNr)
		if err != nil {
			if err == model.ErrNoContent {
				continue
			}
			return nil, nil, fmt.Errorf("page %d content: %w", pageNr, err)
		}
		var res types.Dict
		if o, found := d.Find("Resources"); found {
			res, _ = ctx.DereferenceDict(o)
		} else if inh != nil {
			res = inh.Resources
		}
		rm := &remover{ctx: ctx, res: res, glyph: pr.glyph, redact: pr.redact}
		out, err := rm.run(content)
		if err != nil {
			return nil, nil, fmt.Errorf("page %d: %w", pageNr, err)
		}
		st := rm.st
		st.Page = pageNr
		stats = append(stats, st)
		if !st.changed() {
			continue
		}
		if len(rm.newXObjects) > 0 {
			// Give the page its own copy of the resources so the clipped
			// images do not leak into other pages sharing the dictionary.
			nres := types.NewDict()
			for k, v := range res {
				nres[k] = v
			}
			nx := types.NewDict()
			if old, _ := ctx.DereferenceDict(res["XObject"]); old != nil {
				for k, v := range old {
					nx[k] = v
				}
			}
			for k, v := range rm.newXObjects {
				nx[k] = v
			}
			nres["XObject"] = nx
			d["Resources"] = nres
		}
		sd, err := ctx.NewStreamDictForBuf(out)
		if err != nil {
			return nil, nil, err
		}
		if err := sd.Encode(); err != nil {
			return nil, nil, err
		}
		ir, err := ctx.IndRefForNewObject(*sd)
		if err != nil {
			return nil, nil, err
		}
		d["Contents"] = *ir
	}
	var buf bytes.Buffer
	if err := api.WriteContext(ctx, &buf); err != nil {
		return nil, nil, fmt.Errorf("write pdf: %w", err)
	}
	return buf.Bytes(), stats, nil
}

type rect struct{ x0, y0, x1, y1 float64 }

func (r rect) contains(x, y float64) bool { return x >= r.x0 && x <= r.x1 && y >= r.y0 && y <= r.y1 }
func (r rect) overlaps(o rect) bool {
	return r.x0 < o.x1 && o.x0 < r.x1 && r.y0 < o.y1 && o.y0 < r.y1
}

type matrix [6]float64

var identity = matrix{1, 0, 0, 1, 0, 0}

// mul returns m × n (PDF row-vector convention: apply m first, then n).
func (m matrix) mul(n matrix) matrix {
	return matrix{
		m[0]*n[0] + m[1]*n[2], m[0]*n[1] + m[1]*n[3],
		m[2]*n[0] + m[3]*n[2], m[2]*n[1] + m[3]*n[3],
		m[4]*n[0] + m[5]*n[2] + n[4], m[4]*n[1] + m[5]*n[3] + n[5],
	}
}

func (m matrix) apply(x, y float64) (float64, float64) {
	return x*m[0] + y*m[2] + m[4], x*m[1] + y*m[3] + m[5]
}

func (m matrix) bbox() rect { // image of the unit square
	r := rect{1e18, 1e18, -1e18, -1e18}
	for _, p := range [][2]float64{{0, 0}, {1, 0}, {0, 1}, {1, 1}} {
		x, y := m.apply(p[0], p[1])
		r.x0, r.y0 = min(r.x0, x), min(r.y0, y)
		r.x1, r.y1 = max(r.x1, x), max(r.y1, y)
	}
	return r
}

type fontInfo struct {
	twoByte  bool
	first    int
	widths   []float64
	dw       float64
	cidWidth map[int]float64
	core     string // standard-14 name when the font has no Widths array
}

func (f *fontInfo) width(code int) float64 {
	if f == nil {
		return 500
	}
	if f.twoByte {
		if w, ok := f.cidWidth[code]; ok {
			return w
		}
		return f.dw
	}
	if i := code - f.first; i >= 0 && i < len(f.widths) {
		return f.widths[i]
	}
	if f.core != "" {
		if w := font.CharWidth(f.core, rune(code)); w > 0 {
			return float64(w)
		}
	}
	return f.dw
}

func numOf(ctx *model.Context, o types.Object) (float64, bool) {
	o, _ = ctx.Dereference(o)
	switch v := o.(type) {
	case types.Integer:
		return float64(v), true
	case types.Float:
		return float64(v), true
	}
	return 0, false
}

func loadFont(ctx *model.Context, o types.Object) *fontInfo {
	fd, err := ctx.DereferenceDict(o)
	if err != nil || fd == nil {
		return nil
	}
	fi := &fontInfo{dw: 500}
	if st := fd.NameEntry("Subtype"); st != nil && *st == "Type0" {
		fi.twoByte = true
		fi.dw = 1000
		fi.cidWidth = map[int]float64{}
		da, _ := ctx.DereferenceArray(fd["DescendantFonts"])
		if len(da) == 0 {
			return fi
		}
		cd, _ := ctx.DereferenceDict(da[0])
		if cd == nil {
			return fi
		}
		if v, ok := numOf(ctx, cd["DW"]); ok {
			fi.dw = v
		}
		w, _ := ctx.DereferenceArray(cd["W"])
		for i := 0; i < len(w); {
			c0, ok := numOf(ctx, w[i])
			if !ok || i+1 >= len(w) {
				break
			}
			if arr, err := ctx.DereferenceArray(w[i+1]); err == nil && arr != nil {
				for k, e := range arr {
					if v, ok := numOf(ctx, e); ok {
						fi.cidWidth[int(c0)+k] = v
					}
				}
				i += 2
				continue
			}
			if i+2 >= len(w) {
				break
			}
			c1, _ := numOf(ctx, w[i+1])
			v, _ := numOf(ctx, w[i+2])
			for c := int(c0); c <= int(c1) && c-int(c0) < 65536; c++ {
				fi.cidWidth[c] = v
			}
			i += 3
		}
		return fi
	}
	if v, ok := numOf(ctx, fd["FirstChar"]); ok {
		fi.first = int(v)
	}
	ws, _ := ctx.DereferenceArray(fd["Widths"])
	for _, e := range ws {
		v, _ := numOf(ctx, e)
		fi.widths = append(fi.widths, v)
	}
	if bf := fd.NameEntry("BaseFont"); bf != nil {
		if strings.Contains(*bf, "Courier") {
			fi.dw = 600
		}
		if len(fi.widths) == 0 && font.IsCoreFont(*bf) {
			fi.core = *bf
		}
	}
	return fi
}

type gstate struct {
	ctm matrix
}

type textState struct {
	tc, tw, th, tl, tfs, rise float64
	font                      *fontInfo
}

// remover rewrites one page content stream.
type remover struct {
	ctx         *model.Context
	res         types.Dict
	glyph       []rect // glyphs whose centre falls inside are removed
	redact      []rect // paths and image pixels inside are removed too
	st          RemovePageStats
	newXObjects map[string]types.Object
}

func (rm *remover) run(content []byte) ([]byte, error) {
	ctx, res, regions := rm.ctx, rm.res, rm.glyph
	st := &rm.st
	ops, err := parseContent(content)
	if err != nil {
		return nil, err
	}
	fonts := map[string]*fontInfo{}
	var fontRes, xobjRes types.Dict
	if res != nil {
		fontRes, _ = ctx.DereferenceDict(res["Font"])
		xobjRes, _ = ctx.DereferenceDict(res["XObject"])
	}
	g := gstate{ctm: identity}
	var stack []gstate
	ts := textState{th: 1}
	var tsStack []textState
	tm, tlm := identity, identity

	var out bytes.Buffer
	last := 0
	replaceRange := func(start, end int, s string) {
		out.Write(content[last:start])
		out.WriteString(s)
		last = end
	}
	replace := func(op contentOp, s string) { replaceRange(op.start, op.end, s) }

	// Current path: start offset of its first construction operator, its
	// device-space bounding box, and whether it sets a clip.
	pathStart := -1
	pathBox := rect{1e18, 1e18, -1e18, -1e18}
	pathClip, pathOnlyRect := false, true
	addPt := func(x, y float64) {
		dx, dy := g.ctm.apply(x, y)
		pathBox.x0, pathBox.y0 = min(pathBox.x0, dx), min(pathBox.y0, dy)
		pathBox.x1, pathBox.y1 = max(pathBox.x1, dx), max(pathBox.y1, dy)
	}
	resetPath := func() {
		pathStart = -1
		pathBox = rect{1e18, 1e18, -1e18, -1e18}
		pathClip, pathOnlyRect = false, true
	}

	// show runs one string through the text matrix and returns the TJ
	// elements to emit when at least one glyph was removed.
	show := func(s []byte, emit *[]string, cur *[]byte, removed *bool) {
		step := 1
		if ts.font != nil && ts.font.twoByte {
			step = 2
		}
		for i := 0; i+step <= len(s); i += step {
			code := int(s[i])
			if step == 2 {
				code = int(s[i])<<8 | int(s[i+1])
			}
			w0 := ts.font.width(code)
			trm := matrix{ts.tfs * ts.th, 0, 0, ts.tfs, 0, ts.rise}.mul(tm).mul(g.ctm)
			cx, cy := trm.apply(w0/2000, 0.3)
			wordSpace := 0.0
			if step == 1 && code == 32 {
				wordSpace = ts.tw
			}
			hit := false
			for _, r := range regions {
				if r.contains(cx, cy) {
					hit = true
					break
				}
			}
			if hit {
				*removed = true
				st.GlyphsRemoved++
				if len(*cur) > 0 {
					*emit = append(*emit, hexString(*cur))
					*cur = nil
				}
				adj := w0
				if ts.tfs != 0 {
					adj += (ts.tc + wordSpace) * 1000 / ts.tfs
				}
				*emit = append(*emit, formatNum(-adj))
			} else {
				*cur = append(*cur, s[i:i+step]...)
			}
			tx := (w0/1000*ts.tfs + ts.tc + wordSpace) * ts.th
			tm = matrix{1, 0, 0, 1, tx, 0}.mul(tm)
		}
	}
	nextLine := func() {
		tlm = matrix{1, 0, 0, 1, 0, -ts.tl}.mul(tlm)
		tm = tlm
	}
	nums := func(op contentOp) []float64 {
		v := make([]float64, 0, len(op.operands))
		for _, t := range op.operands {
			if t.kind == tokNumber {
				v = append(v, t.num)
			}
		}
		return v
	}

	for _, op := range ops {
		n := nums(op)
		switch op.op {
		case "m", "l", "c", "v", "y", "re", "h":
			if pathStart < 0 {
				pathStart = op.start
			}
			if op.op != "re" && op.op != "h" {
				pathOnlyRect = false
			}
			if op.op == "re" && len(n) == 4 {
				addPt(n[0], n[1])
				addPt(n[0]+n[2], n[1]+n[3])
				addPt(n[0]+n[2], n[1])
				addPt(n[0], n[1]+n[3])
			} else {
				for i := 0; i+1 < len(n); i += 2 {
					addPt(n[i], n[i+1])
				}
			}
		case "W", "W*":
			pathClip = true
		case "S", "s", "f", "F", "f*", "B", "B*", "b", "b*", "n":
			if pathStart >= 0 && op.op != "n" && !pathClip && len(rm.redact) > 0 {
				if pathRedacted(pathBox, pathOnlyRect, rm.redact) {
					st.PathsRemoved++
					replaceRange(pathStart, op.end, "")
				}
			}
			resetPath()
		case "q":
			stack = append(stack, g)
			tsStack = append(tsStack, ts)
		case "Q":
			if len(stack) > 0 {
				g = stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				ts = tsStack[len(tsStack)-1]
				tsStack = tsStack[:len(tsStack)-1]
			}
		case "cm":
			if len(n) == 6 {
				g.ctm = matrix{n[0], n[1], n[2], n[3], n[4], n[5]}.mul(g.ctm)
			}
		case "BT":
			tm, tlm = identity, identity
		case "Tc":
			if len(n) == 1 {
				ts.tc = n[0]
			}
		case "Tw":
			if len(n) == 1 {
				ts.tw = n[0]
			}
		case "Tz":
			if len(n) == 1 {
				ts.th = n[0] / 100
			}
		case "TL":
			if len(n) == 1 {
				ts.tl = n[0]
			}
		case "Ts":
			if len(n) == 1 {
				ts.rise = n[0]
			}
		case "Tf":
			if len(op.operands) == 2 && op.operands[0].kind == tokName {
				name := string(op.operands[0].str)
				if _, ok := fonts[name]; !ok && fontRes != nil {
					fonts[name] = loadFont(ctx, fontRes[name])
				}
				ts.font = fonts[name]
				ts.tfs = op.operands[1].num
			}
		case "Td", "TD":
			if len(n) == 2 {
				if op.op == "TD" {
					ts.tl = -n[1]
				}
				tlm = matrix{1, 0, 0, 1, n[0], n[1]}.mul(tlm)
				tm = tlm
			}
		case "Tm":
			if len(n) == 6 {
				tlm = matrix{n[0], n[1], n[2], n[3], n[4], n[5]}
				tm = tlm
			}
		case "T*":
			nextLine()
		case "Tj", "'", "\"", "TJ":
			prefix := ""
			if op.op == "'" {
				nextLine()
				prefix = "T* "
			}
			if op.op == "\"" && len(n) >= 2 {
				ts.tw, ts.tc = n[0], n[1]
				nextLine()
				prefix = formatNum(n[0]) + " Tw " + formatNum(n[1]) + " Tc T* "
			}
			var emit []string
			var cur []byte
			removed := false
			for _, t := range op.operands {
				switch t.kind {
				case tokString:
					show(t.str, &emit, &cur, &removed)
				case tokArray:
					for _, e := range t.arr {
						if e.kind == tokString {
							show(e.str, &emit, &cur, &removed)
						} else if e.kind == tokNumber {
							if len(cur) > 0 {
								emit = append(emit, hexString(cur))
								cur = nil
							}
							emit = append(emit, formatNum(e.num))
							tm = matrix{1, 0, 0, 1, -e.num / 1000 * ts.tfs * ts.th, 0}.mul(tm)
						}
					}
				}
			}
			if removed {
				if len(cur) > 0 {
					emit = append(emit, hexString(cur))
				}
				replace(op, prefix+"["+strings.Join(emit, " ")+"] TJ")
			}
		case "Do":
			if len(op.operands) != 1 || op.operands[0].kind != tokName || xobjRes == nil {
				break
			}
			box := g.ctm.bbox()
			var hits []rect
			for _, r := range rm.redact {
				if r.overlaps(box) {
					hits = append(hits, r)
				}
			}
			if len(hits) == 0 {
				break
			}
			name := string(op.operands[0].str)
			sd, _, err := ctx.DereferenceStreamDict(xobjRes[name])
			if err != nil || sd == nil {
				break
			}
			if s := sd.NameEntry("Subtype"); s == nil || *s != "Image" {
				st.Unhandled++ // form XObject: its own content is not rewritten
				break
			}
			if coveredBy(box, hits) {
				st.ImagesRemoved++
				replace(op, "")
				break
			}
			if nsd, ok := clipImage(sd, g.ctm, hits); ok {
				ir, err := ctx.IndRefForNewObject(*nsd)
				if err == nil {
					if rm.newXObjects == nil {
						rm.newXObjects = map[string]types.Object{}
					}
					nn := fmt.Sprintf("%sR%d", name, len(rm.newXObjects)+1)
					rm.newXObjects[nn] = *ir
					st.ImagesClipped++
					replace(op, "/"+nn+" Do")
					break
				}
			}
			// Pixels could not be edited in this image format: drop it whole.
			st.ImagesRemoved++
			replace(op, "")
		case "BI":
			box := g.ctm.bbox()
			for _, r := range rm.redact {
				if r.overlaps(box) {
					st.ImagesRemoved++
					replace(op, "")
					break
				}
			}
		}
	}
	out.Write(content[last:])
	return out.Bytes(), nil
}

// pathRedacted decides whether a painted path goes: when it lies wholly
// inside the redaction, or mostly inside it. A plain filled or stroked
// rectangle that only partly overlaps (a background, a table cell) stays,
// since its geometry carries nothing from under the redaction.
func pathRedacted(box rect, onlyRect bool, regions []rect) bool {
	if box.x0 > box.x1 {
		return false
	}
	if coveredBy(box, regions) {
		return true
	}
	if onlyRect {
		return false
	}
	area := max(box.x1-box.x0, 0.01) * max(box.y1-box.y0, 0.01)
	in := 0.0
	for _, r := range regions {
		w := min(box.x1, r.x1) - max(box.x0, r.x0)
		h := min(box.y1, r.y1) - max(box.y0, r.y0)
		if w > 0 && h > 0 {
			in += max(w, 0.01) * max(h, 0.01)
		}
	}
	return in/area >= 0.5
}

// coveredBy reports whether box lies inside one of the regions.
func coveredBy(box rect, regions []rect) bool {
	for _, r := range regions {
		if box.x0 >= r.x0 && box.x1 <= r.x1 && box.y0 >= r.y0 && box.y1 <= r.y1 {
			return true
		}
	}
	return false
}
