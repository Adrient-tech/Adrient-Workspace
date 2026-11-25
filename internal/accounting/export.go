package accounting

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"fmt"
	"strings"
)

// Table is the flat shape every report is exported in (CSV, XLSX, PDF).
// Cells are strings or float64; the first column is text, the rest are normally numbers.
type Table struct {
	Title    string
	Subtitle string
	Headers  []string
	Rows     [][]any
}

func cellStr(v any) string {
	switch x := v.(type) {
	case float64:
		return fmt.Sprintf("%.2f", x)
	case int:
		return fmt.Sprintf("%d", x)
	case int64:
		return fmt.Sprintf("%d", x)
	case nil:
		return ""
	}
	return fmt.Sprint(v)
}

// csvSafe neutralises spreadsheet formula injection in text cells.
func csvSafe(s string) string {
	if s != "" && strings.ContainsRune("=+@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}

// CSV renders the table as CSV with a title row.
func (t Table) CSV() []byte {
	var b bytes.Buffer
	w := csv.NewWriter(&b)
	_ = w.Write([]string{csvSafe(t.Title)})
	if t.Subtitle != "" {
		_ = w.Write([]string{csvSafe(t.Subtitle)})
	}
	_ = w.Write(t.Headers)
	for _, r := range t.Rows {
		rec := make([]string, len(r))
		for i, c := range r {
			if s, ok := c.(string); ok {
				rec[i] = csvSafe(s)
			} else {
				rec[i] = cellStr(c)
			}
		}
		_ = w.Write(rec)
	}
	w.Flush()
	return b.Bytes()
}

func xmlEsc(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '&':
			b.WriteString("&amp;")
		case r == '<':
			b.WriteString("&lt;")
		case r == '>':
			b.WriteString("&gt;")
		case r == '"':
			b.WriteString("&quot;")
		case r < 0x20 && r != '\t' && r != '\n' && r != '\r':
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func colName(i int) string {
	s := ""
	for i++; i > 0; i = (i - 1) / 26 {
		s = string(rune('A'+(i-1)%26)) + s
	}
	return s
}

// XLSX renders a minimal but valid single-sheet workbook (inline strings, numeric cells).
func (t Table) XLSX() ([]byte, error) {
	var sheet bytes.Buffer
	sheet.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)
	row := 0
	text := func(cells ...string) {
		row++
		fmt.Fprintf(&sheet, `<row r="%d">`, row)
		for i, c := range cells {
			fmt.Fprintf(&sheet, `<c r="%s%d" t="inlineStr"><is><t xml:space="preserve">%s</t></is></c>`, colName(i), row, xmlEsc(c))
		}
		sheet.WriteString(`</row>`)
	}
	text(t.Title)
	if t.Subtitle != "" {
		text(t.Subtitle)
	}
	text(t.Headers...)
	for _, r := range t.Rows {
		row++
		fmt.Fprintf(&sheet, `<row r="%d">`, row)
		for i, c := range r {
			switch x := c.(type) {
			case float64:
				fmt.Fprintf(&sheet, `<c r="%s%d"><v>%.2f</v></c>`, colName(i), row, x)
			case int:
				fmt.Fprintf(&sheet, `<c r="%s%d"><v>%d</v></c>`, colName(i), row, x)
			default:
				fmt.Fprintf(&sheet, `<c r="%s%d" t="inlineStr"><is><t xml:space="preserve">%s</t></is></c>`, colName(i), row, xmlEsc(cellStr(c)))
			}
		}
		sheet.WriteString(`</row>`)
	}
	sheet.WriteString(`</sheetData></worksheet>`)
	files := []struct{ name, body string }{
		{"[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/></Types>`},
		{"_rels/.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`},
		{"xl/workbook.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Report" sheetId="1" r:id="rId1"/></sheets></workbook>`},
		{"xl/_rels/workbook.xml.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/></Relationships>`},
		{"xl/worksheets/sheet1.xml", sheet.String()},
	}
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	for _, f := range files {
		w, err := zw.Create(f.name)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write([]byte(f.body)); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func pdfText(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\\' || r == '(' || r == ')':
			b.WriteRune('\\')
			b.WriteRune(r)
		case r == '₹':
			b.WriteString("Rs ")
		case r < 0x20:
			b.WriteRune(' ')
		case r > 0xFF:
			b.WriteRune('?')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// PDF renders the table as a simple paginated A4 PDF (Helvetica, right-aligned numbers).
func (t Table) PDF() []byte {
	const pageW, pageH, margin, lineH = 595.0, 842.0, 40.0, 13.0
	ncol := len(t.Headers)
	if ncol == 0 {
		ncol = 1
	}
	usable := pageW - 2*margin
	firstW := usable * 0.38
	if ncol == 1 {
		firstW = usable
	}
	otherW := 0.0
	if ncol > 1 {
		otherW = (usable - firstW) / float64(ncol-1)
	}
	var pages []string
	var cur strings.Builder
	y := pageH - margin
	put := func(x, yy float64, size int, bold bool, s string, right bool, width float64) {
		font := "F1"
		if bold {
			font = "F2"
		}
		s = pdfText(s)
		maxChars := int(width / (float64(size) * 0.5))
		if len(s) > maxChars && maxChars > 3 {
			s = s[:maxChars-1] + "."
		}
		if right {
			x = x + width - float64(len(s))*float64(size)*0.5
		}
		fmt.Fprintf(&cur, "BT /%s %d Tf %.1f %.1f Td (%s) Tj ET\n", font, size, x, yy, s)
	}
	newPage := func() {
		if cur.Len() > 0 {
			pages = append(pages, cur.String())
			cur.Reset()
		}
		y = pageH - margin
	}
	header := func() {
		for i, h := range t.Headers {
			x := margin
			w := firstW
			if i > 0 {
				x = margin + firstW + float64(i-1)*otherW
				w = otherW
			}
			put(x, y, 9, true, h, i > 0, w-4)
		}
		y -= lineH + 2
	}
	put(margin, y, 14, true, t.Title, false, usable)
	y -= 18
	if t.Subtitle != "" {
		put(margin, y, 9, false, t.Subtitle, false, usable)
		y -= 16
	}
	header()
	for _, r := range t.Rows {
		if y < margin+lineH {
			newPage()
			header()
		}
		for i, c := range r {
			x := margin
			w := firstW
			if i > 0 {
				x = margin + firstW + float64(i-1)*otherW
				w = otherW
			}
			_, isNum := c.(float64)
			put(x, y, 9, false, cellStr(c), i > 0 && isNum, w-4)
		}
		y -= lineH
	}
	newPage()
	// objects: 1 catalog, 2 pages, 3 F1, 4 F2, then page/content pairs
	var objs []string
	objs = append(objs, "<< /Type /Catalog /Pages 2 0 R >>")
	kids := ""
	for i := range pages {
		kids += fmt.Sprintf("%d 0 R ", 5+i*2)
	}
	objs = append(objs, fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", kids, len(pages)))
	objs = append(objs, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>")
	objs = append(objs, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold /Encoding /WinAnsiEncoding >>")
	for i, p := range pages {
		objs = append(objs, fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %.0f %.0f] /Resources << /Font << /F1 3 0 R /F2 4 0 R >> >> /Contents %d 0 R >>", pageW, pageH, 6+i*2))
		objs = append(objs, fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", len(p), p))
	}
	var out bytes.Buffer
	out.WriteString("%PDF-1.4\n")
	offs := make([]int, len(objs))
	for i, o := range objs {
		offs[i] = out.Len()
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	x := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, o := range offs {
		fmt.Fprintf(&out, "%010d 00000 n \n", o)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, x)
	return out.Bytes()
}
