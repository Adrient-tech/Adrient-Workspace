package invoicing

import "testing"

func TestStateCode(t *testing.T) {
	cases := map[string]string{"29": "29", "29-Karnataka": "29", "KA": "29", "ka": "29", "Karnataka": "29",
		"29ABCDE1234F1Z5": "29", "": "", "99": "", "Atlantis": "", "TS": "36"}
	for in, want := range cases {
		if got := StateCode(in); got != want {
			t.Errorf("StateCode(%q)=%q want %q", in, got, want)
		}
	}
}

func TestPlaceOfSupply(t *testing.T) {
	cases := []struct {
		sup, pos string
		intra    bool
	}{
		{"29", "29", true},
		{"29", "Karnataka", true},
		{"29", "27", false},
		{"29", "", true},   // unknown place of supply: the supplier's own state
		{"29", "96", false}, // export
		{"", "27", true},   // supplier state not configured
	}
	for _, c := range cases {
		if got := IsIntraState(c.sup, c.pos); got != c.intra {
			t.Errorf("IsIntraState(%q,%q)=%v want %v", c.sup, c.pos, got, c.intra)
		}
	}
}

func TestGSTSplit(t *testing.T) {
	lines := []Line{{Qty: 1, Price: 100.05, TaxRate: 18}, {Qty: 2, Price: 50, TaxRate: 5}}
	intra := GST(lines, "percent", 0, "29", "29")
	if !intra.Intra || intra.IGST != 0 {
		t.Fatalf("intra: %+v", intra)
	}
	// line 1 tax = 18.009 -> 18.01 = 9.01 + 9.00 (CGST rounded, SGST the remainder), line 2 tax 5.00
	if Round2(intra.Lines[0].CGST+intra.Lines[0].SGST) != intra.Lines[0].Tax {
		t.Fatalf("line split must add up: %+v", intra.Lines[0])
	}
	if Round2(intra.CGST+intra.SGST) != intra.TaxTotal || intra.TaxTotal != 23.01 {
		t.Fatalf("totals: %+v", intra)
	}
	inter := GST(lines, "percent", 0, "29", "27")
	if inter.Intra || inter.CGST != 0 || inter.SGST != 0 || inter.IGST != 23.01 {
		t.Fatalf("inter: %+v", inter)
	}
	// Chandigarh (04) is a union territory without a legislature: UTGST.
	if ut := GST(lines, "percent", 0, "04", "04"); !ut.UT || !ut.Intra {
		t.Fatalf("ut: %+v", ut)
	}
}

func TestGSTDiscountSpread(t *testing.T) {
	lines := []Line{{Qty: 1, Price: 1000, TaxRate: 18}, {Qty: 1, Price: 1000, TaxRate: 5}}
	b := GST(lines, "amount", 200, "29", "27") // 100 off each line
	if b.Taxable != 1800 || b.IGST != 162+45 || b.TaxTotal != 207 {
		t.Fatalf("%+v", b)
	}
	c := Compute(lines, "amount", 200)
	if c.TaxTotal != b.TaxTotal {
		t.Fatalf("Compute and GST disagree: %v vs %v", c.TaxTotal, b.TaxTotal)
	}
}

func TestValidGSTIN(t *testing.T) {
	if !ValidGSTIN("29ABCDE1234F1Z5") || ValidGSTIN("29ABCDE1234F1X5") || ValidGSTIN("123") {
		t.Fatal("gstin shape")
	}
}

func TestStandWithCredits(t *testing.T) {
	s := Stand(1000, []Payment{{Kind: "payment", Amount: 300}, {Kind: "credit", Amount: 200}})
	if s.Paid != 300 || s.Credited != 200 || s.Balance != 500 {
		t.Fatalf("%+v", s)
	}
	s = Stand(1000, []Payment{{Kind: "credit", Amount: 1000}})
	if got := DisplayStatus("invoice", "sent", nil, 1000, s, today()); got != "credited" {
		t.Fatalf("status %s", got)
	}
}
