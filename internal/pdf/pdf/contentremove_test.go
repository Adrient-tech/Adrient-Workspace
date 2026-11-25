package pdf

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// buildPDF writes a one-page PDF with the given content stream, a Helvetica
// font F1 (no Widths: standard metrics apply) and a 2x2 image Im1.
func buildPDF(content string) []byte {
	objs := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> /XObject << /Im1 6 0 R >> >> /Contents 4 0 R >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content)+0, content),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		"<< /Type /XObject /Subtype /Image /Width 2 /Height 2 /ColorSpace /DeviceGray /BitsPerComponent 8 /Length 4 >>\nstream\n\x00\xff\xff\x00\nendstream",
	}
	var b bytes.Buffer
	b.WriteString("%PDF-1.7\n")
	offs := make([]int, len(objs))
	for i, o := range objs {
		offs[i] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	x := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, o := range offs {
		fmt.Fprintf(&b, "%010d 00000 n \n", o)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, x)
	return b.Bytes()
}

// pageText returns the shown bytes and the operator list of page 1.
func pageText(t *testing.T, pdf []byte) (string, []string) {
	t.Helper()
	ctx, err := api.ReadContext(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("re-read: %v", err)
	}
	if err := ctx.EnsurePageCount(); err != nil {
		t.Fatal(err)
	}
	d, _, _, err := ctx.PageDict(1, false)
	if err != nil {
		t.Fatal(err)
	}
	c, err := ctx.PageContent(d, 1)
	if err != nil {
		t.Fatal(err)
	}
	ops, err := parseContent(c)
	if err != nil {
		t.Fatal(err)
	}
	var text strings.Builder
	var names []string
	for _, op := range ops {
		names = append(names, op.op)
		for _, o := range op.operands {
			if o.kind == tokString {
				text.Write(o.str)
			}
			for _, e := range o.arr {
				if e.kind == tokString {
					text.Write(e.str)
				}
			}
		}
	}
	return text.String(), names
}

const sample = `BT /F1 12 Tf 72 700 Td (Hello World) Tj ET
BT /F1 12 Tf 72 600 Td [(Keep) -250 (Me)] TJ ET
q 100 0 0 50 300 300 cm /Im1 Do Q`

func TestRemoveContentDropsGlyphsInRegion(t *testing.T) {
	in := buildPDF(sample)
	// At 12pt Helvetica "Hello " is 30.7pt wide, so "World" spans about 102.7..134.
	out, stats, err := RemoveContent(in, []RemoveRegion{{Page: 1, X: 106, Y: 695, W: 40, H: 20}})
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 1 || stats[0].GlyphsRemoved != 5 || stats[0].ImagesRemoved != 0 {
		t.Fatalf("stats = %+v", stats)
	}
	text, _ := pageText(t, out)
	if strings.Contains(text, "World") || !strings.Contains(text, "Hello ") || !strings.Contains(text, "Keep") {
		t.Fatalf("text after removal = %q", text)
	}
}

func TestRemoveContentKeepsFollowingGlyphPositions(t *testing.T) {
	in := buildPDF(`BT /F1 10 Tf 0 0 Td (ABC) Tj ET`)
	out, _, err := RemoveContent(in, []RemoveRegion{{Page: 1, X: 9, Y: -1, W: 2, H: 10}}) // Helvetica B spans 6.67..13.34, centre 10
	if err == nil {
		text, _ := pageText(t, out)
		if text != "AC" {
			t.Fatalf("text = %q, want AC", text)
		}
	} else {
		t.Fatal(err)
	}
	// The removed glyph becomes a -667 offset (its width), so C stays at x=13.34.
	ctx, _ := api.ReadContext(bytes.NewReader(out), model.NewDefaultConfiguration())
	_ = ctx.EnsurePageCount()
	d, _, _, _ := ctx.PageDict(1, false)
	c, _ := ctx.PageContent(d, 1)
	if !bytes.Contains(c, []byte("[<41> -667 <43>] TJ")) {
		t.Fatalf("content = %s", c)
	}
}

func TestRemoveContentDropsCoveredImage(t *testing.T) {
	in := buildPDF(sample)
	out, stats, err := RemoveContent(in, []RemoveRegion{{Page: 1, X: 290, Y: 290, W: 120, H: 70}})
	if err != nil {
		t.Fatal(err)
	}
	if stats[0].ImagesRemoved != 1 || stats[0].GlyphsRemoved != 0 {
		t.Fatalf("stats = %+v", stats)
	}
	_, ops := pageText(t, out)
	for _, o := range ops {
		if o == "Do" {
			t.Fatal("image draw still present")
		}
	}
}

func TestRemoveContentNoHitLeavesStream(t *testing.T) {
	in := buildPDF(sample)
	out, stats, err := RemoveContent(in, []RemoveRegion{{Page: 1, X: 500, Y: 50, W: 10, H: 10}})
	if err != nil {
		t.Fatal(err)
	}
	if stats[0].GlyphsRemoved+stats[0].ImagesRemoved != 0 {
		t.Fatalf("stats = %+v", stats)
	}
	text, _ := pageText(t, out)
	if text != "Hello WorldKeepMe" {
		t.Fatalf("text = %q", text)
	}
}

func TestRemoveContentRejectsBadPage(t *testing.T) {
	if _, _, err := RemoveContent(buildPDF(sample), []RemoveRegion{{Page: 3, W: 1, H: 1}}); err == nil {
		t.Fatal("want error for page 3")
	}
}

func TestParseContentStrings(t *testing.T) {
	ops, err := parseContent([]byte(`BT (a\(b\)\101) Tj <4142> Tj [(x) 10 <7a>] TJ ET`))
	if err != nil {
		t.Fatal(err)
	}
	if string(ops[1].operands[0].str) != "a(b)A" || string(ops[2].operands[0].str) != "AB" || len(ops[3].operands[0].arr) != 3 {
		t.Fatalf("ops = %+v", ops)
	}
}

// imagePixels returns the decoded samples of the image drawn on page 1.
func imagePixels(t *testing.T, pdf []byte) []byte {
	t.Helper()
	ctx, err := api.ReadContext(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	_ = ctx.EnsurePageCount()
	d, _, _, _ := ctx.PageDict(1, false)
	c, _ := ctx.PageContent(d, 1)
	ops, _ := parseContent(c)
	res, _ := ctx.DereferenceDict(d["Resources"])
	xo, _ := ctx.DereferenceDict(res["XObject"])
	for _, op := range ops {
		if op.op == "Do" {
			sd, _, err := ctx.DereferenceStreamDict(xo[string(op.operands[0].str)])
			if err != nil || sd == nil {
				t.Fatalf("image %s missing", op.operands[0].str)
			}
			if err := sd.Decode(); err != nil {
				t.Fatal(err)
			}
			return sd.Content
		}
	}
	t.Fatal("no image drawn")
	return nil
}

func TestRemoveContentClipsPartlyCoveredImage(t *testing.T) {
	in := buildPDF(sample) // 2x2 gray image [0 255 / 255 0] at 300,300 sized 100x50
	if got := imagePixels(t, in); !bytes.Equal(got, []byte{0, 255, 255, 0}) {
		t.Fatalf("fixture pixels %v", got)
	}
	// Cover the top-right quarter: pixel (row 0, col 1) turns black.
	out, stats, err := RemoveContent(in, []RemoveRegion{{Page: 1, X: 360, Y: 330, W: 60, H: 30}})
	if err != nil {
		t.Fatal(err)
	}
	if stats[0].ImagesClipped != 1 || stats[0].ImagesRemoved != 0 {
		t.Fatalf("stats = %+v", stats)
	}
	if got := imagePixels(t, out); !bytes.Equal(got, []byte{0, 0, 255, 0}) {
		t.Fatalf("pixels after clip %v, want [0 0 255 0]", got)
	}
}

func TestRemoveContentGlyphsOnlyKeepsImagesAndPaths(t *testing.T) {
	in := buildPDF(sample + "\n0 0 1 rg 310 310 20 20 re f\n")
	_, stats, err := RemoveContent(in, []RemoveRegion{{Page: 1, X: 290, Y: 290, W: 120, H: 70, GlyphsOnly: true}})
	if err != nil {
		t.Fatal(err)
	}
	if s := stats[0]; s.ImagesRemoved+s.ImagesClipped+s.PathsRemoved != 0 {
		t.Fatalf("stats = %+v", stats)
	}
}

func TestRemoveContentRemovesPathsUnderRedaction(t *testing.T) {
	content := "BT /F1 12 Tf 72 700 Td (Keep) Tj ET\n" +
		"0 0 0 RG 100 100 m 150 120 l 140 160 l S\n" + // a scribble inside the region
		"1 0 0 rg 0 0 612 792 re f\n" + // page background: partly overlaps, stays
		"q 120 120 30 30 re W n 0 g 0 0 612 792 re f Q\n" // clip path: never removed
	out, stats, err := RemoveContent(buildPDF(content), []RemoveRegion{{Page: 1, X: 90, Y: 90, W: 80, H: 80}})
	if err != nil {
		t.Fatal(err)
	}
	if stats[0].PathsRemoved != 1 {
		t.Fatalf("stats = %+v", stats)
	}
	_, ops := pageText(t, out)
	got := strings.Join(ops, " ")
	if strings.Contains(got, "m l l S") || !strings.Contains(got, "re W n") || strings.Count(got, "re f") != 2 {
		t.Fatalf("ops after removal: %s", got)
	}
}
