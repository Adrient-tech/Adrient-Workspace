package demoseed

import (
	"time"

	"github.com/Adrient-tech/Adrient-Workspace/internal/captable"
	"github.com/Adrient-tech/Adrient-Workspace/internal/invoicing"
	"github.com/Adrient-tech/Adrient-Workspace/internal/knowkit"
	"github.com/Adrient-tech/Adrient-Workspace/internal/legal"
	"github.com/Adrient-tech/Adrient-Workspace/internal/opsports"
)

// runV5 is seed step 5: Legal (clients, matters, deadlines, time, bills, conflict checks) and the Cap table
// of a fictional Indian startup. All names are made up. Dates are relative to today.
func (s *seeder) runV5() {
	a := knowkit.Actor{Org: s.org, User: s.admin, Admin: true}
	s.v5Legal(a)
	s.v5CapTable(a)
}

func (s *seeder) rel(days int) string { return day(s.today.AddDate(0, 0, days)) }

func (s *seeder) v5Legal(a knowkit.Actor) {
	repo := legal.NewRepository(s.pool)
	repo.Deps.Invoices = opsports.Invoices{Repo: invoicing.NewRepository(s.pool)}
	repo.Deps.Now = func() time.Time { return s.today }
	if _, err := repo.SaveSettings(s.ctx, a, legal.Settings{NumberFormat: "year-number", IncrementMin: 6, DefaultRate: 450000, Currency: "INR", TermsDays: 30, TaxRateBp: 1800, TaxName: "GST",
		PracticeAreas: []string{"Corporate", "Litigation", "Intellectual property", "Employment", "Real estate", "Tax"}}); err != nil {
		s.warn("legal settings", err)
		return
	}
	type cl struct{ kind, name, email, phone string }
	var clients = map[string]string{}
	for _, c := range []cl{
		{"company", "Kaveri Textiles Pvt Ltd", "accounts@kaveritextiles.example", "+91 422 555 0101"},
		{"person", "Dr Meenakshi Sundaram", "meenakshi.s@example.in", "+91 98400 55012"},
		{"company", "Nilgiri Labs Pvt Ltd", "founders@nilgirilabs.example", "+91 80 5550 0142"},
		{"company", "Brahmaputra Logistics LLP", "legal@brahmaputralogistics.example", "+91 361 555 0188"},
		{"company", "Ravi & Sons Traders", "ravi@ravisons.example", "+91 44 5550 0170"},
		{"person", "Anitha Venkatesh", "anitha.v@example.in", "+91 99620 11874"},
	} {
		x, err := repo.CreateClient(s.ctx, a, legal.ClientInput{Kind: c.kind, Name: c.name, Email: c.email, Phone: c.phone})
		if err != nil {
			s.warn("legal client", err)
			return
		}
		clients[c.name] = x.ID
		s.count("legal clients", 1)
	}
	type mt struct {
		title, client, area, stage, method string
		rate, flat                          int64
		sol                                 int
		parties                             []legal.Party
		status                              string
		hours                               []string
	}
	matters := []mt{
		{"Supply agreement dispute with Bharat Dyes", "Kaveri Textiles Pvt Ltd", "Litigation", "Pleadings", "hourly", 0, 0, 94,
			[]legal.Party{{Name: "Bharat Dyes LLP", Role: "Opposing party"}, {Name: "Adv. S. Raghunathan", Role: "Opposing counsel"}}, "open", []string{"2:30", "1.5", "45m", "3h", "1:12"}},
		{"Trademark registration: NILGIRI wordmark", "Nilgiri Labs Pvt Ltd", "Intellectual property", "Filing", "flat", 0, 7500000, 0, nil, "open", []string{"1h", "0:40"}},
		{"Series Seed documentation", "Nilgiri Labs Pvt Ltd", "Corporate", "Drafting", "hourly", 600000, 0, 0,
			[]legal.Party{{Name: "Sahyadri Angel Network", Role: "Investor"}}, "open", []string{"4h", "2.5", "1:30", "3:15"}},
		{"Warehouse lease: Guwahati hub", "Brahmaputra Logistics LLP", "Real estate", "Due diligence", "hourly", 0, 0, 0,
			[]legal.Party{{Name: "Kamakhya Estates", Role: "Landlord"}}, "pending", []string{"2h", "1.2"}},
		{"Wrongful termination claim", "Anitha Venkatesh", "Employment", "Conciliation", "hourly", 0, 0, 160,
			[]legal.Party{{Name: "Coromandel Retail Ltd", Role: "Opposing party"}}, "open", []string{"1:45", "0:50", "2.2"}},
		{"GST notice reply FY 2023-24", "Ravi & Sons Traders", "Tax", "Reply filed", "hourly", 0, 0, 0, nil, "closed", []string{"3h", "1:30"}},
		{"Will and succession planning", "Dr Meenakshi Sundaram", "Corporate", "", "hourly", 0, 0, 0, nil, "on_hold", []string{"1h"}},
		{"Distributor agreement review", "Kaveri Textiles Pvt Ltd", "Corporate", "Intake", "hourly", 0, 0, 0, nil, "intake", nil},
	}
	var open []string
	for i, m := range matters {
		in := legal.MatterInput{Title: m.title, ClientID: clients[m.client], PracticeArea: m.area, Stage: m.stage, BillingMethod: m.method, Rate: m.rate, FlatFee: m.flat, Parties: m.parties}
		if m.sol > 0 {
			in.SolDate = s.rel(m.sol)
		}
		x, err := repo.CreateMatter(s.ctx, a, in)
		if err != nil {
			s.warn("legal matter", err)
			continue
		}
		s.count("legal matters", 1)
		if m.status != "intake" {
			if _, err := repo.RecordConflict(s.ctx, a, x.ID, firstWord(m.client)); err != nil {
				s.warn("legal conflict", err)
			}
			if _, err := repo.Move(s.ctx, a, x.ID, legal.MoveInput{Status: "open"}); err != nil {
				s.warn("legal open", err)
				continue
			}
		}
		for j, h := range m.hours {
			if _, err := repo.AddEntry(s.ctx, a, x.ID, legal.EntryInput{Kind: "time", Duration: h, Date: s.rel(-3 - j*4 - i), Description: legalWork[(i+j)%len(legalWork)]}); err != nil {
				s.warn("legal time", err)
			}
		}
		if i == 0 {
			_, _ = repo.AddEntry(s.ctx, a, x.ID, legal.EntryInput{Kind: "expense", Price: 250000, Qty: "1", Date: s.rel(-10), Description: "Court fee and stamp paper"})
			_, _ = repo.AddEntry(s.ctx, a, x.ID, legal.EntryInput{Kind: "expense", Price: 18500, Qty: "4", Date: s.rel(-8), Description: "Certified copies"})
			for _, d := range []legal.DeadlineInput{
				{Title: "File written statement", Kind: "filing", Due: s.rel(9)},
				{Title: "Hearing before the Commercial Court", Kind: "court_date", Due: s.rel(21)},
				{Title: "Reply to interrogatories", Kind: "deadline", Due: s.rel(-2)},
			} {
				if _, err := repo.AddDeadline(s.ctx, a, x.ID, d); err == nil {
					s.count("legal deadlines", 1)
				}
			}
		}
		if i == 2 {
			_, _ = repo.AddDeadline(s.ctx, a, x.ID, legal.DeadlineInput{Title: "Signing meeting with investors", Kind: "meeting", Due: s.rel(5)})
			_, _ = repo.AddNote(s.ctx, a, x.ID, "meeting", "Term sheet walkthrough with the founders. Liquidation preference is 1x non-participating; the investor asked for a 12-month cliff on founder vesting.")
		}
		if i == 4 {
			_, _ = repo.AddDeadline(s.ctx, a, x.ID, legal.DeadlineInput{Title: "Conciliation hearing", Kind: "court_date", Due: s.rel(14)})
			_, _ = repo.AddNote(s.ctx, a, x.ID, "call", "Client confirmed the last working day and the unpaid bonus of two months.")
		}
		if m.status == "pending" {
			_, _ = repo.Move(s.ctx, a, x.ID, legal.MoveInput{Status: "pending", Note: "Waiting for the landlord's title papers"})
		}
		if m.status == "on_hold" {
			_, _ = repo.Move(s.ctx, a, x.ID, legal.MoveInput{Status: "on_hold", Note: "Client travelling until next month"})
		}
		if m.status == "closed" {
			b, err := repo.DraftBill(s.ctx, a, x.ID, legal.BillInput{})
			if err == nil {
				if b, err = repo.IssueBill(s.ctx, a, b.ID); err != nil {
					s.warn("legal bill", err)
				}
				s.count("legal bills", 1)
			}
			if _, err := repo.Move(s.ctx, a, x.ID, legal.MoveInput{Status: "closed", Note: "Reply accepted by the department", Force: true}); err != nil {
				s.warn("legal close", err)
			}
		}
		if m.status == "open" || m.status == "pending" {
			open = append(open, x.ID)
		}
	}
	// One issued bill for the supply dispute's first period and a draft for the lease.
	if len(open) > 0 {
		if b, err := repo.DraftBill(s.ctx, a, open[0], legal.BillInput{To: s.rel(-14)}); err == nil {
			if _, err := repo.IssueBill(s.ctx, a, b.ID); err != nil {
				s.warn("legal bill", err)
			}
			s.count("legal bills", 1)
		}
	}
	if len(open) > 2 {
		if _, err := repo.DraftBill(s.ctx, a, open[3%len(open)], legal.BillInput{}); err == nil {
			s.count("legal bills", 1)
		}
	}
}

var legalWork = []string{
	"Review of the supply contract and correspondence", "Drafting the reply notice", "Call with client on strategy", "Research on limitation and interest",
	"Meeting with opposing counsel", "Drafting and vetting the agreement", "Review of the title documents", "Preparation of the hearing brief",
}

func firstWord(s string) string {
	for i, r := range s {
		if r == ' ' {
			return s[:i]
		}
	}
	return s
}

func (s *seeder) v5CapTable(a knowkit.Actor) {
	repo := captable.NewRepository(s.pool)
	repo.Deps.Now = func() time.Time { return s.today }
	if _, err := repo.SaveCompany(s.ctx, a, captable.Company{Name: "Nilgiri Labs Pvt Ltd", Currency: "INR", Incorporated: s.rel(-760), Notes: "Fictional demo company. Face value Rs 1 per share."}); err != nil {
		s.warn("captable company", err)
		return
	}
	eq, err := repo.CreateClass(s.ctx, a, captable.ClassInput{Name: "Equity shares", Kind: "common", IssuePrice: "1", Authorized: 20_000_000})
	if err != nil {
		s.warn("captable class", err)
		return
	}
	cc, err := repo.CreateClass(s.ctx, a, captable.ClassInput{Name: "Series Seed CCPS", Kind: "preferred", Seniority: 1, PrefMultipleMilli: 1000, ConvertRatioMilli: 1000, Authorized: 6_000_000})
	if err != nil {
		s.warn("captable class", err)
		return
	}
	mk := func(name, kind string) string {
		h, err := repo.CreateStakeholder(s.ctx, a, captable.StakeholderInput{Name: name, Kind: kind})
		if err != nil {
			s.warn("captable stakeholder", err)
			return ""
		}
		s.count("cap table holders", 1)
		return h.ID
	}
	aarav, diya := mk("Aarav Menon", "founder"), mk("Diya Krishnan", "founder")
	sahyadri, ghats := mk("Sahyadri Angel Network", "investor"), mk("Western Ghats Capital", "investor")
	issue := func(in captable.SecurityInput) string {
		x, err := repo.Issue(s.ctx, a, in)
		if err != nil {
			s.warn("captable issue", err)
			return ""
		}
		s.count("cap table securities", 1)
		return x.ID
	}
	issue(captable.SecurityInput{Kind: "stock", StakeholderID: aarav, ClassID: eq.ID, Quantity: 4_500_000, Price: "1", IssueDate: s.rel(-740), VestMonths: 48, CliffMonths: 12, VestStart: s.rel(-740)})
	issue(captable.SecurityInput{Kind: "stock", StakeholderID: diya, ClassID: eq.ID, Quantity: 4_000_000, Price: "1", IssueDate: s.rel(-740), VestMonths: 48, CliffMonths: 12, VestStart: s.rel(-740)})
	pool, err := repo.CreatePool(s.ctx, a, captable.PoolInput{Name: "ESOP 2024", Size: 1_000_000, Adopted: s.rel(-700)})
	if err != nil {
		s.warn("captable pool", err)
		return
	}
	// A post-money SAFE from the angels, before the round: Rs 50 lakh at a Rs 20 crore post-money cap.
	issue(captable.SecurityInput{Kind: "safe", StakeholderID: sahyadri, Principal: 500_000_000, Cap: 20_000_000_000, PostMoney: true, IssueDate: s.rel(-420)})
	team := []struct {
		name string
		n    int64
		ago  int
	}{{"Ishaan Pillai", 120_000, -600}, {"Kavya Reddy", 90_000, -560}, {"Rohan Das", 60_000, -330}, {"Sneha Joshi", 45_000, -300}, {"Tarun Bhatt", 30_000, -280}}
	var ishaan string
	for i, t := range team {
		h := mk(t.name, "employee")
		if i == 0 {
			ishaan = h
		}
		issue(captable.SecurityInput{Kind: "option", StakeholderID: h, ClassID: eq.ID, PoolID: pool.ID, Quantity: t.n, Price: "12", IssueDate: s.rel(t.ago), VestMonths: 48, CliffMonths: 12, Expires: s.rel(t.ago + 3650)})
	}
	// Seed round: Rs 24 crore pre-money, Rs 4 crore in from two investors; the SAFE converts; the pool tops up to 10%.
	if _, err := repo.CloseRound(s.ctx, a, captable.RoundModelInput{Name: "Seed round", Date: s.rel(-240), PreMoney: 24_000_000_000, Investment: 4_000_000_000, PoolTargetBp: 1000, ClassID: cc.ID,
		Investors: []captable.Investor{{StakeholderID: ghats, Amount: 3_000_000_000}, {StakeholderID: sahyadri, Amount: 1_000_000_000}}}); err != nil {
		s.warn("captable round", err)
	} else {
		s.count("cap table rounds", 1)
	}
	// Later grants from the topped-up pool, one exercise and one cancellation.
	if pools, err := repo.Pools(s.ctx, s.org); err == nil && len(pools) > 1 {
		late := mk("Meera Subramanian", "employee")
		issue(captable.SecurityInput{Kind: "option", StakeholderID: late, ClassID: eq.ID, PoolID: pools[1].ID, Quantity: 40_000, Price: "260", IssueDate: s.rel(-90), VestMonths: 48, CliffMonths: 12})
		adv := mk("Prof. Gopal Narayan", "advisor")
		issue(captable.SecurityInput{Kind: "option", StakeholderID: adv, ClassID: eq.ID, PoolID: pools[1].ID, Quantity: 25_000, Price: "260", IssueDate: s.rel(-60), VestMonths: 24, CliffMonths: 6})
	}
	if recs, err := repo.Securities(s.ctx, s.org, ishaan, "option", ""); err == nil && len(recs) > 0 {
		if _, err := repo.Record(s.ctx, a, "exercise", recs[0].ID, captable.EventInput{Date: s.rel(-45), Quantity: 20_000}); err != nil {
			s.warn("captable exercise", err)
		}
	}
	if _, err := repo.SaveScenario(s.ctx, a, "exit", "Strategic sale at Rs 150 crore", []byte(`{"exits":[1500000000000],"unvested":"participate"}`)); err == nil {
		s.count("cap table scenarios", 1)
	}
}
