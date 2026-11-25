package timetracking

import "testing"

func TestParseDuration(t *testing.T) {
	cases := map[string]int{
		"1:30": 5400, "90m": 5400, "1.5h": 5400, "1h 20m": 4800, "45": 162000 / 60 * 60 / 60 * 0, "2h30": 9000,
		"2": 7200, "0.25": 900, ",5": 1800, ":15": 900, "1h": 3600, "30 min": 1800, "90s": 90, "1:02:03": 3723,
	}
	cases["45"] = 45 * 60 // a bare integer above 24 means minutes
	for in, want := range cases {
		got, ok := ParseDuration(in)
		if !ok || got != want {
			t.Errorf("ParseDuration(%q) = %d,%v want %d", in, got, ok, want)
		}
	}
	for _, bad := range []string{"", "abc", "1:99", "h", "-5"} {
		if _, ok := ParseDuration(bad); ok {
			t.Errorf("ParseDuration(%q) must fail", bad)
		}
	}
}

func TestRoundSeconds(t *testing.T) {
	if RoundSeconds(61, 15) != 900 || RoundSeconds(900, 15) != 900 || RoundSeconds(901, 15) != 1800 || RoundSeconds(500, 0) != 500 {
		t.Fatal("rounding up to the increment")
	}
}

func f(v float64) *float64 { return &v }
func i(v int) *int         { return &v }

func TestRateCascadeAndBilling(t *testing.T) {
	res := Resolver{
		Settings: Settings{DefaultRate: 50, Rounding: 0},
		Projects: map[string]Project{"p1": {ID: "p1", ClientID: "c1", Billable: true}, "p2": {ID: "p2", ClientID: "c1", Billable: true, Rate: f(120)}, "p3": {ID: "p3", Billable: false}},
		Clients:  map[string]Client{"c1": {ID: "c1", Rate: f(80), Rounding: i(15), MinMinutes: i(30)}},
	}
	e := func(p string, sec int) Entry { return Entry{ProjectID: p, Seconds: sec, Billable: true} }
	if res.Rate(e("p1", 60)) != 80 || res.Rate(e("p2", 60)) != 120 || res.Rate(Entry{ProjectID: "p2", Rate: f(200)}) != 200 || res.Rate(e("p3", 60)) != 50 {
		t.Fatal("rate cascade: entry > project > client > default")
	}
	// 20 minutes rounds up to 30 (15 min steps); the 30 minute minimum is the same here.
	if got := res.BilledSeconds(e("p1", 1200)); got != 1800 {
		t.Fatalf("billed = %d", got)
	}
	// 5 minutes: rounds to 15, minimum lifts it to 30.
	if got := res.BilledSeconds(e("p1", 300)); got != 1800 {
		t.Fatalf("minimum billed = %d", got)
	}
	if res.BilledSeconds(e("p3", 3600)) != 0 {
		t.Fatal("non-billable project never bills")
	}
	if res.Amount(e("p2", 3600)) != 120 {
		t.Fatalf("amount = %v", res.Amount(e("p2", 3600)))
	}
	// Invoiced entries keep the frozen rate even if the client rate changes.
	locked := Entry{ProjectID: "p1", Seconds: 3600, Billable: true, InvoiceID: "d1", LockedRate: f(60), LockedRound: i(0), LockedMinimum: i(0)}
	if res.Rate(locked) != 60 || res.BilledSeconds(locked) != 3600 || res.Amount(locked) != 60 {
		t.Fatal("locked values must win after invoicing")
	}
}
