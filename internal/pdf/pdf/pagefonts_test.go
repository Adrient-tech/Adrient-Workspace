package pdf

import (
	"bytes"
	"fmt"
	"testing"

	"golang.org/x/image/font/gofont/gobolditalic"
)

// pdfWithEmbeddedFont builds a one-page PDF whose font F1 embeds a TrueType
// program (FontFile2) and whose font F2 is a standard font with none.
func pdfWithEmbeddedFont(ttf []byte) []byte {
	content := "BT /F1 12 Tf 72 700 Td (Hi) Tj ET"
	objs := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R /F2 8 0 R >> >> /Contents 4 0 R >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
		"<< /Type /Font /Subtype /TrueType /BaseFont /ABCDEF+GoBoldItalic /FirstChar 32 /LastChar 126 /FontDescriptor 6 0 R >>",
		"<< /Type /FontDescriptor /FontName /ABCDEF+GoBoldItalic /Flags 32 /FontBBox [0 0 1000 1000] /ItalicAngle -12 /Ascent 900 /Descent -200 /CapHeight 700 /StemV 80 /FontFile2 7 0 R >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(ttf), ttf),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
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

func TestPageFontsReturnsEmbeddedProgram(t *testing.T) {
	fonts, err := PageFonts(pdfWithEmbeddedFont(gobolditalic.TTF), 1)
	if err != nil {
		t.Fatal(err)
	}
	byRes := map[string]PageFont{}
	for _, f := range fonts {
		byRes[f.Resource] = f
	}
	f1, f2 := byRes["F1"], byRes["F2"]
	if f1.Format != "truetype" || !bytes.Equal(f1.File, gobolditalic.TTF) {
		t.Fatalf("F1 format %q, %d bytes; want the embedded program", f1.Format, len(f1.File))
	}
	if !f1.Bold || !f1.Italic || f1.BaseFont != "ABCDEF+GoBoldItalic" {
		t.Fatalf("F1 = %+v", f1)
	}
	if f2.BaseFont != "Helvetica" || f2.File != nil || f2.Bold {
		t.Fatalf("F2 = %+v", f2)
	}
}

func TestPageFontsBadPage(t *testing.T) {
	if _, err := PageFonts(pdfWithEmbeddedFont(gobolditalic.TTF), 2); err == nil {
		t.Fatal("want error for page 2")
	}
}
