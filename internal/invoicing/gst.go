package invoicing

import (
	"math"
	"regexp"
	"strings"
)

// Indian GST state / union territory codes (the first two digits of a GSTIN).
var gstStates = map[string]string{
	"01": "Jammu and Kashmir", "02": "Himachal Pradesh", "03": "Punjab", "04": "Chandigarh", "05": "Uttarakhand",
	"06": "Haryana", "07": "Delhi", "08": "Rajasthan", "09": "Uttar Pradesh", "10": "Bihar", "11": "Sikkim",
	"12": "Arunachal Pradesh", "13": "Nagaland", "14": "Manipur", "15": "Mizoram", "16": "Tripura", "17": "Meghalaya",
	"18": "Assam", "19": "West Bengal", "20": "Jharkhand", "21": "Odisha", "22": "Chhattisgarh", "23": "Madhya Pradesh",
	"24": "Gujarat", "26": "Dadra and Nagar Haveli and Daman and Diu", "27": "Maharashtra", "29": "Karnataka",
	"30": "Goa", "31": "Lakshadweep", "32": "Kerala", "33": "Tamil Nadu", "34": "Puducherry",
	"35": "Andaman and Nicobar Islands", "36": "Telangana", "37": "Andhra Pradesh", "38": "Ladakh",
	"96": "Other Country", "97": "Other Territory",
}

// utStates are union territories without a legislature: intra-state supply
// there is CGST + UTGST instead of CGST + SGST.
var utStates = map[string]bool{"04": true, "26": true, "31": true, "35": true, "38": true}

var stateAbbr = map[string]string{
	"JK": "01", "HP": "02", "PB": "03", "CH": "04", "UK": "05", "UT": "05", "HR": "06", "DL": "07", "RJ": "08",
	"UP": "09", "BR": "10", "SK": "11", "AR": "12", "NL": "13", "MN": "14", "MZ": "15", "TR": "16", "ML": "17",
	"AS": "18", "WB": "19", "JH": "20", "OD": "21", "OR": "21", "CG": "22", "MP": "23", "GJ": "24", "DD": "26",
	"DN": "26", "MH": "27", "KA": "29", "GA": "30", "LD": "31", "KL": "32", "TN": "33", "PY": "34", "AN": "35",
	"TS": "36", "TG": "36", "AP": "37", "LA": "38",
}

var leadDigits = regexp.MustCompile(`^\s*(\d{2})`)

// StateCode normalises a place-of-supply or state value to a two-digit GST
// code. It accepts "29", "29-Karnataka", a GSTIN, "KA" or a state name.
// It returns "" when the value is empty or not recognised.
func StateCode(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if m := leadDigits.FindStringSubmatch(v); m != nil {
		if _, ok := gstStates[m[1]]; ok {
			return m[1]
		}
		return ""
	}
	u := strings.ToUpper(v)
	if c, ok := stateAbbr[u]; ok {
		return c
	}
	for code, name := range gstStates {
		if strings.EqualFold(name, v) {
			return code
		}
	}
	return ""
}

// StateName returns the display name for a code ("" when unknown).
func StateName(code string) string { return gstStates[code] }

// IsIntraState reports whether a supply from supplier to placeOfSupply is
// within one state (CGST + SGST/UTGST) rather than inter-state (IGST). An
// unknown or empty place of supply is treated as the supplier's own state;
// code 96 (other country, an export) is always IGST.
func IsIntraState(supplier, placeOfSupply string) bool {
	s, p := StateCode(supplier), StateCode(placeOfSupply)
	if p == "" {
		return true
	}
	if p == "96" {
		return false
	}
	if s == "" {
		return true
	}
	return s == p
}

// LineTax is the tax of one line split into its GST components.
type LineTax struct {
	Taxable float64 `json:"taxable"`
	Rate    float64 `json:"rate"`
	CGST    float64 `json:"cgst"`
	SGST    float64 `json:"sgst"`
	IGST    float64 `json:"igst"`
	Tax     float64 `json:"tax"`
}

// Breakup is the GST summary of a document.
type Breakup struct {
	Supplier string    `json:"supplier"`
	POS      string    `json:"place_of_supply"`
	Intra    bool      `json:"intra"`
	UT       bool      `json:"ut"` // intra-state in a union territory: the state half is UTGST
	Lines    []LineTax `json:"lines"`
	Taxable  float64   `json:"taxable"`
	CGST     float64   `json:"cgst"`
	SGST     float64   `json:"sgst"`
	IGST     float64   `json:"igst"`
	TaxTotal float64   `json:"tax_total"`
}

// GST computes per-line taxable value and tax split. The document discount is
// spread over the lines in proportion to their net value, the same way Compute
// does, so the tax total always equals Compute's TaxTotal. For intra-state
// supply each line's tax is split half CGST and the remainder SGST so that
// CGST+SGST equals the line tax to the paisa.
func GST(lines []Line, discountKind string, discountValue float64, supplier, placeOfSupply string) Breakup {
	intra := IsIntraState(supplier, placeOfSupply)
	b := Breakup{Supplier: StateCode(supplier), POS: StateCode(placeOfSupply), Intra: intra, Lines: make([]LineTax, len(lines))}
	if intra && utStates[StateCode(placeOfSupply)] {
		b.UT = true
	}
	nets := make([]float64, len(lines))
	var sub float64
	for i, l := range lines {
		nets[i] = lineNet(l)
		sub += nets[i]
	}
	var disc float64
	if discountKind == "amount" {
		disc = math.Min(math.Max(discountValue, 0), sub)
	} else {
		disc = sub * clampPct(discountValue) / 100
	}
	for i, l := range lines {
		share := nets[i]
		if sub > 0 {
			share = nets[i] - disc*nets[i]/sub
		}
		rate := math.Max(l.TaxRate, 0)
		lt := LineTax{Taxable: Round2(share), Rate: rate}
		tax := Round2(share * rate / 100)
		lt.Tax = tax
		if intra {
			lt.CGST = Round2(tax / 2)
			lt.SGST = Round2(tax - lt.CGST)
		} else {
			lt.IGST = tax
		}
		b.Lines[i] = lt
		b.Taxable += lt.Taxable
		b.CGST += lt.CGST
		b.SGST += lt.SGST
		b.IGST += lt.IGST
	}
	b.Taxable, b.CGST, b.SGST, b.IGST = Round2(b.Taxable), Round2(b.CGST), Round2(b.SGST), Round2(b.IGST)
	b.TaxTotal = Round2(b.CGST + b.SGST + b.IGST)
	return b
}
