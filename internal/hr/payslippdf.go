package hr

import (
	"bytes"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// A small dependency-free PDF writer, enough for payslips: Helvetica text,
// lines and filled rectangles on A4. internal/pdf only edits existing PDFs.

type pdfPage struct{ b bytes.Buffer }

func pdfEsc(s string) string {
	s = strings.ReplaceAll(s, "₹", "Rs.")
	var out strings.Builder
	for _, r := range s {
		switch {
		case r == '\\' || r == '(' || r == ')':
			out.WriteByte('\\')
			out.WriteRune(r)
		case r < 32:
			out.WriteByte(' ')
		case r < 256:
			out.WriteByte(byte(r)) // WinAnsi matches Latin-1 for these
		default:
			out.WriteByte('?')
		}
	}
	return out.String()
}

// textWidth approximates the width in points of Helvetica text.
func textWidth(s string, size float64) float64 {
	w := 0.0
	for _, r := range pdfEsc(s) {
		switch {
		case r >= '0' && r <= '9':
			w += 556
		case r == '.' || r == ',' || r == ' ' || r == ':' || r == ';' || r == '/' || r == '|' || r == 'i' || r == 'l' || r == 'I' || r == 'j' || r == '\'':
			w += 278
		case r == 'm' || r == 'w' || r == 'M' || r == 'W':
			w += 833
		case r >= 'A' && r <= 'Z':
			w += 667
		default:
			w += 540
		}
	}
	return w * size / 1000
}

func (p *pdfPage) text(x, y, size float64, bold bool, s string) {
	f := "F1"
	if bold {
		f = "F2"
	}
	fmt.Fprintf(&p.b, "BT /%s %.1f Tf %.2f %.2f Td (%s) Tj ET\n", f, size, x, y, pdfEsc(s))
}

func (p *pdfPage) textRight(xRight, y, size float64, bold bool, s string) {
	p.text(xRight-textWidth(s, size), y, size, bold, s)
}

func (p *pdfPage) line(x1, y1, x2, y2, w float64) {
	fmt.Fprintf(&p.b, "%.2f w %.2f %.2f m %.2f %.2f l S\n", w, x1, y1, x2, y2)
}

func (p *pdfPage) rect(x, y, w, h, gray float64) {
	fmt.Fprintf(&p.b, "%.2f g %.2f %.2f %.2f %.2f re f 0 g\n", gray, x, y, w, h)
}

func (p *pdfPage) fillRGB(x, y, w, h, r, g, b float64) {
	fmt.Fprintf(&p.b, "%.3f %.3f %.3f rg %.2f %.2f %.2f %.2f re f 0 g\n", r, g, b, x, y, w, h)
}

func buildPDF(title string, pages ...*pdfPage) []byte {
	var out bytes.Buffer
	offsets := []int{}
	obj := func(body string) {
		offsets = append(offsets, out.Len())
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", len(offsets), body)
	}
	out.WriteString("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")
	// 1 catalog, 2 pages, 3 F1, 4 F2, 5 info, then page/content pairs.
	obj("<< /Type /Catalog /Pages 2 0 R >>")
	kids := make([]string, len(pages))
	for i := range pages {
		kids[i] = fmt.Sprintf("%d 0 R", 6+i*2)
	}
	obj(fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), len(pages)))
	obj("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>")
	obj("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold /Encoding /WinAnsiEncoding >>")
	obj(fmt.Sprintf("<< /Title (%s) /Producer (Adrient HR) /CreationDate (D:%s) >>", pdfEsc(title), time.Now().UTC().Format("20060102150405")+"Z"))
	for i, pg := range pages {
		obj(fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Resources << /Font << /F1 3 0 R /F2 4 0 R >> >> /Contents %d 0 R >>", 7+i*2))
		obj(fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", pg.b.Len(), pg.b.String()))
	}
	xref := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", len(offsets)+1)
	for _, o := range offsets {
		fmt.Fprintf(&out, "%010d 00000 n \n", o)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R /Info 5 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets)+1, xref)
	return out.Bytes()
}

// Payslip is the document model rendered to PDF.
type Payslip struct {
	ID          string       `json:"id"`
	RunID       string       `json:"run_id"`
	Period      string       `json:"period"`
	RunStatus   string       `json:"run_status"`
	EmployeeID  string       `json:"employee_id"`
	EmpCode     string       `json:"emp_code"`
	EmpName     string       `json:"emp_name"`
	Designation string       `json:"designation"`
	Department  string       `json:"department"`
	PAN         string       `json:"pan"`
	UAN         string       `json:"uan"`
	BankName    string       `json:"bank_name"`
	BankAccount string       `json:"bank_account"`
	BankIFSC    string       `json:"bank_ifsc"`
	DaysInMonth int          `json:"days_in_month"`
	LOPDays     float64      `json:"lop_days"`
	PaidDays    float64      `json:"paid_days"`
	Earnings    []Amt        `json:"earnings"`
	Deductions  []Amt        `json:"deductions"`
	Adjustments []Adjustment `json:"adjustments"`
	Gross       float64      `json:"gross"`
	PFEmployee  float64      `json:"pf_employee"`
	PFEmployer  float64      `json:"pf_employer"`
	ESIEmployee float64      `json:"esi_employee"`
	ESIEmployer float64      `json:"esi_employer"`
	ProfTax     float64      `json:"prof_tax"`
	TDS         float64      `json:"tds"`
	OtherDed    float64      `json:"other_deductions"`
	Reimburse   float64      `json:"reimbursements"`
	Net         float64      `json:"net_pay"`
}

func maskAccount(s string) string {
	if utf8.RuneCountInString(s) <= 4 {
		return s
	}
	r := []rune(s)
	return strings.Repeat("X", len(r)-4) + string(r[len(r)-4:])
}

// PeriodLabel turns 2026-03 into "March 2026".
func PeriodLabel(period string) string {
	t, err := time.Parse("2006-01", period)
	if err != nil {
		return period
	}
	return t.Format("January 2006")
}

// RenderPayslipPDF draws one payslip.
func RenderPayslipPDF(p Payslip, company, address string) []byte {
	pg := &pdfPage{}
	left, right := 40.0, 555.0
	pg.rect(0, 782, 595, 60, 0.96)
	pg.fillRGB(0, 836, 595, 6, 0.071, 0.424, 0.353)
	pg.text(left, 812, 16, true, orDefault(company, "Payslip"))
	if address != "" {
		pg.text(left, 796, 8, false, strings.ReplaceAll(address, "\n", ", "))
	}
	pg.textRight(right, 812, 12, true, "Payslip")
	pg.textRight(right, 796, 10, false, PeriodLabel(p.Period))
	y := 760.0
	info := [][2]string{{"Employee", p.EmpName}, {"Employee code", p.EmpCode}, {"Designation", p.Designation}, {"Department", p.Department}}
	info2 := [][2]string{{"PAN", p.PAN}, {"UAN", p.UAN}, {"Bank", strings.TrimSpace(p.BankName + " " + maskAccount(p.BankAccount))}, {"IFSC", p.BankIFSC}}
	for i := range info {
		pg.text(left, y, 9, false, info[i][0]+":")
		pg.text(left+85, y, 9, true, info[i][1])
		pg.text(300, y, 9, false, info2[i][0]+":")
		pg.text(350, y, 9, true, info2[i][1])
		y -= 15
	}
	y -= 4
	pg.text(left, y, 9, false, fmt.Sprintf("Days in month: %d    Loss of pay: %s    Paid days: %s", p.DaysInMonth, trimZero(p.LOPDays), trimZero(p.PaidDays)))
	y -= 14
	pg.line(left, y, right, y, 0.8)
	y -= 18
	// Two columns.
	pg.text(left, y, 10, true, "Earnings")
	pg.textRight(285, y, 10, true, "Amount (Rs.)")
	pg.text(310, y, 10, true, "Deductions")
	pg.textRight(right, y, 10, true, "Amount (Rs.)")
	y -= 6
	pg.line(left, y, right, y, 0.4)
	y -= 14
	type row struct {
		n string
		a float64
	}
	var ern, ded []row
	for _, e := range p.Earnings {
		ern = append(ern, row{e.Name, e.Amount})
	}
	if p.PFEmployee > 0 {
		ded = append(ded, row{"Provident Fund (employee)", p.PFEmployee})
	}
	if p.ESIEmployee > 0 {
		ded = append(ded, row{"ESI (employee)", p.ESIEmployee})
	}
	if p.ProfTax > 0 {
		ded = append(ded, row{"Professional Tax", p.ProfTax})
	}
	if p.TDS > 0 {
		ded = append(ded, row{"Income Tax (TDS)", p.TDS})
	}
	for _, d := range p.Deductions {
		ded = append(ded, row{d.Name, d.Amount})
	}
	n := len(ern)
	if len(ded) > n {
		n = len(ded)
	}
	for i := 0; i < n; i++ {
		if i < len(ern) {
			pg.text(left, y, 9, false, ern[i].n)
			pg.textRight(285, y, 9, false, money(ern[i].a))
		}
		if i < len(ded) {
			pg.text(310, y, 9, false, ded[i].n)
			pg.textRight(right, y, 9, false, money(ded[i].a))
		}
		y -= 14
	}
	if p.Reimburse > 0 {
		pg.text(left, y, 9, false, "Reimbursements (not taxable)")
		pg.textRight(285, y, 9, false, money(p.Reimburse))
		y -= 14
	}
	y -= 2
	pg.line(left, y, right, y, 0.4)
	y -= 14
	totalDed := round2(p.PFEmployee + p.ESIEmployee + p.ProfTax + p.TDS + p.OtherDed)
	pg.text(left, y, 10, true, "Gross earnings")
	pg.textRight(285, y, 10, true, money(round2(p.Gross+p.Reimburse)))
	pg.text(310, y, 10, true, "Total deductions")
	pg.textRight(right, y, 10, true, money(totalDed))
	y -= 12
	pg.line(left, y, right, y, 0.8)
	y -= 30
	pg.rect(left, y-8, right-left, 30, 0.93)
	pg.text(left+10, y+2, 12, true, "Net pay")
	pg.textRight(right-10, y+2, 14, true, "Rs. "+money(p.Net))
	y -= 26
	pg.text(left, y, 9, false, "In words: "+AmountInWords(p.Net))
	y -= 30
	if p.PFEmployer > 0 || p.ESIEmployer > 0 {
		pg.text(left, y, 8, false, fmt.Sprintf("Employer contributions (not part of take-home): PF Rs. %s, ESI Rs. %s", money(p.PFEmployer), money(p.ESIEmployer)))
		y -= 12
	}
	pg.text(left, 50, 8, false, "This is a computer generated payslip and does not require a signature.")
	return buildPDF("Payslip "+p.EmpName+" "+PeriodLabel(p.Period), pg)
}

func trimZero(f float64) string {
	s := fmt.Sprintf("%.2f", f)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	if s == "" {
		return "0"
	}
	return s
}

var ones = []string{"", "One", "Two", "Three", "Four", "Five", "Six", "Seven", "Eight", "Nine", "Ten", "Eleven", "Twelve", "Thirteen", "Fourteen", "Fifteen", "Sixteen", "Seventeen", "Eighteen", "Nineteen"}
var tens = []string{"", "", "Twenty", "Thirty", "Forty", "Fifty", "Sixty", "Seventy", "Eighty", "Ninety"}

func below100(n int) string {
	if n < 20 {
		return ones[n]
	}
	s := tens[n/10]
	if n%10 > 0 {
		s += " " + ones[n%10]
	}
	return s
}

func below1000(n int) string {
	s := ""
	if n >= 100 {
		s = ones[n/100] + " Hundred"
		n %= 100
		if n > 0 {
			s += " "
		}
	}
	return s + below100(n)
}

// AmountInWords writes a rupee amount in the Indian system (lakh, crore).
func AmountInWords(amount float64) string {
	if amount < 0 {
		amount = 0
	}
	paise := int(amount*100 + 0.5)
	rupees, ps := paise/100, paise%100
	if rupees == 0 && ps == 0 {
		return "Zero Rupees Only"
	}
	parts := []string{}
	if c := rupees / 10000000; c > 0 {
		parts = append(parts, below1000(c)+" Crore")
		rupees %= 10000000
	}
	if l := rupees / 100000; l > 0 {
		parts = append(parts, below100(l)+" Lakh")
		rupees %= 100000
	}
	if t := rupees / 1000; t > 0 {
		parts = append(parts, below100(t)+" Thousand")
		rupees %= 1000
	}
	if rupees > 0 {
		parts = append(parts, below1000(rupees))
	}
	s := strings.Join(parts, " ")
	if s == "" {
		s = "Zero"
	}
	s += " Rupees"
	if ps > 0 {
		s += " and " + below100(ps) + " Paise"
	}
	return s + " Only"
}

func orDefault(s, d string) string {
	if strings.TrimSpace(s) == "" {
		return d
	}
	return s
}
