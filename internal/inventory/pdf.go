package inventory

// A small dependency-free PDF writer for purchase orders (A4, Helvetica).

import (
	"bytes"
	"fmt"
	"strings"
)

type pdfDoc struct {
	pages   []*bytes.Buffer
	cur     *bytes.Buffer
	y       float64
	objs    []string
}

const (
	pageW, pageH = 595.0, 842.0
	margin       = 40.0
)

func pdfText(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '₹':
			b.WriteString("Rs.")
		case r == '\\' || r == '(' || r == ')':
			b.WriteRune('\\')
			b.WriteRune(r)
		case r == '\n' || r == '\r' || r == '\t':
			b.WriteRune(' ')
		case r >= 32 && r < 127:
			b.WriteRune(r)
		case r >= 160 && r <= 255:
			fmt.Fprintf(&b, "\\%03o", r)
		default:
			b.WriteRune('?')
		}
	}
	return b.String()
}

func (d *pdfDoc) newPage() {
	d.cur = &bytes.Buffer{}
	d.pages = append(d.pages, d.cur)
	d.y = pageH - margin
}

// text writes one line at x with a font size; bold uses Helvetica-Bold; right aligns at x.
func (d *pdfDoc) text(x float64, size float64, bold, right bool, s string) {
	font := "F1"
	if bold {
		font = "F2"
	}
	if right {
		x -= float64(len(s)) * size * 0.5
	}
	fmt.Fprintf(d.cur, "BT /%s %.1f Tf %.1f %.1f Td (%s) Tj ET\n", font, size, x, d.y, pdfText(s))
}

func (d *pdfDoc) line(x1, x2 float64) {
	fmt.Fprintf(d.cur, "0.7 G %.1f %.1f m %.1f %.1f l S 0 G\n", x1, d.y, x2, d.y)
}

func (d *pdfDoc) advance(h float64) {
	d.y -= h
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n-1]) + "."
	}
	return s
}

func money(f float64) string { return fmt.Sprintf("%.2f", f) }

// wrap splits text into lines of at most n characters.
func wrap(s string, n int) []string {
	var out []string
	for _, para := range strings.Split(s, "\n") {
		words := strings.Fields(para)
		line := ""
		for _, w := range words {
			if line != "" && len(line)+1+len(w) > n {
				out = append(out, line)
				line = w
			} else if line == "" {
				line = w
			} else {
				line += " " + w
			}
		}
		if line != "" || len(words) == 0 {
			out = append(out, line)
		}
	}
	return out
}

// POPDF renders a purchase order as a PDF document.
func POPDF(po PurchaseOrder, st PurchaseSettings) []byte {
	d := &pdfDoc{}
	d.newPage()
	company := st.CompanyName
	if company == "" {
		company = "Purchase Order"
	}
	d.text(margin, 16, true, false, company)
	d.text(pageW-margin, 20, true, true, "PURCHASE ORDER")
	d.advance(16)
	for _, l := range wrap(st.CompanyAddress, 60) {
		d.text(margin, 9, false, false, l)
		d.advance(11)
	}
	if st.CompanyGSTIN != "" {
		d.text(margin, 9, false, false, "GSTIN: "+st.CompanyGSTIN)
		d.advance(11)
	}
	d.advance(8)
	d.line(margin, pageW-margin)
	d.advance(16)
	top := d.y
	d.text(margin, 9, true, false, "Supplier")
	d.advance(12)
	d.text(margin, 11, true, false, po.SupplierName)
	d.advance(13)
	if po.SupplierEmail != "" {
		d.text(margin, 9, false, false, po.SupplierEmail)
		d.advance(11)
	}
	left := d.y
	d.y = top
	rx := 330.0
	for _, kv := range [][2]string{{"PO number", po.Number}, {"Order date", po.OrderDate}, {"Expected", po.ExpectedDate}, {"Ship to", po.ShipToName}, {"Supplier ref", po.SupplierRef}} {
		if kv[1] == "" {
			continue
		}
		d.text(rx, 9, true, false, kv[0])
		d.text(rx+80, 9, false, false, clip(kv[1], 32))
		d.advance(12)
	}
	if d.y > left {
		d.y = left
	}
	d.advance(10)

	header := func() {
		d.line(margin, pageW-margin)
		d.advance(12)
		d.text(margin, 9, true, false, "#")
		d.text(margin+20, 9, true, false, "Item")
		d.text(340, 9, true, true, "Qty")
		d.text(400, 9, true, true, "Rate")
		d.text(450, 9, true, true, "Tax %")
		d.text(pageW-margin, 9, true, true, "Amount")
		d.advance(5)
		d.line(margin, pageW-margin)
		d.advance(13)
	}
	header()
	for i, l := range po.Lines {
		if d.y < 120 {
			d.newPage()
			header()
		}
		name := l.ProductName
		if l.SKU != "" {
			name += " (" + l.SKU + ")"
		}
		d.text(margin, 9, false, false, fmt.Sprint(i+1))
		d.text(margin+20, 9, false, false, clip(name, 52))
		d.text(340, 9, false, true, trimFloat(l.Qty)+" "+clip(l.Unit, 5))
		d.text(400, 9, false, true, money(l.UnitCost))
		d.text(450, 9, false, true, trimFloat(l.TaxPct))
		d.text(pageW-margin, 9, false, true, money(l.LineTotal))
		d.advance(12)
		if l.Description != "" {
			for _, w := range wrap(l.Description, 80) {
				d.text(margin+20, 8, false, false, w)
				d.advance(10)
			}
		}
	}
	d.advance(4)
	d.line(340, pageW-margin)
	d.advance(14)
	for _, kv := range [][2]string{{"Subtotal", money(po.Subtotal)}, {"Tax", money(po.TaxTotal)}} {
		d.text(400, 9, false, true, kv[0])
		d.text(pageW-margin, 9, false, true, kv[1])
		d.advance(13)
	}
	d.text(400, 11, true, true, "Total "+po.Currency)
	d.text(pageW-margin, 11, true, true, money(po.Total))
	d.advance(26)
	if po.Notes != "" {
		d.text(margin, 9, true, false, "Notes")
		d.advance(12)
		for _, w := range wrap(po.Notes, 95) {
			d.text(margin, 9, false, false, w)
			d.advance(11)
		}
		d.advance(8)
	}
	if po.Terms != "" {
		d.text(margin, 9, true, false, "Terms")
		d.advance(12)
		for _, w := range wrap(po.Terms, 95) {
			if d.y < 60 {
				d.newPage()
			}
			d.text(margin, 9, false, false, w)
			d.advance(11)
		}
	}

	// Assemble objects: 1 catalog, 2 pages, 3 font, 4 bold font, then page+content pairs.
	var objs []string
	objs = append(objs, "<< /Type /Catalog /Pages 2 0 R >>")
	kids := make([]string, 0, len(d.pages))
	for i := range d.pages {
		kids = append(kids, fmt.Sprintf("%d 0 R", 5+i*2))
	}
	objs = append(objs, fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), len(d.pages)))
	objs = append(objs, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>")
	objs = append(objs, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold /Encoding /WinAnsiEncoding >>")
	for i, p := range d.pages {
		objs = append(objs, fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %.0f %.0f] /Resources << /Font << /F1 3 0 R /F2 4 0 R >> >> /Contents %d 0 R >>", pageW, pageH, 6+i*2))
		foot := fmt.Sprintf("BT /F1 8 Tf %.1f 24 Td (%s) Tj ET\n", margin, pdfText(fmt.Sprintf("%s  -  page %d of %d", po.Number, i+1, len(d.pages))))
		content := p.String() + foot
		objs = append(objs, fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", len(content), content))
	}
	var out bytes.Buffer
	out.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objs))
	for i, o := range objs {
		offsets[i] = out.Len()
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xref := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, off := range offsets {
		fmt.Fprintf(&out, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, xref)
	return out.Bytes()
}
