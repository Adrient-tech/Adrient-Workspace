package demoseed

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Adrient-tech/Adrient-Workspace/internal/accounting"
	"github.com/Adrient-tech/Adrient-Workspace/internal/calendar"
	"github.com/Adrient-tech/Adrient-Workspace/internal/contacts"
	"github.com/Adrient-tech/Adrient-Workspace/internal/crm"
	"github.com/Adrient-tech/Adrient-Workspace/internal/docs"
	"github.com/Adrient-tech/Adrient-Workspace/internal/hr"
	"github.com/Adrient-tech/Adrient-Workspace/internal/inventory"
	"github.com/Adrient-tech/Adrient-Workspace/internal/invoicing"
	"github.com/Adrient-tech/Adrient-Workspace/internal/pos"
	"github.com/Adrient-tech/Adrient-Workspace/internal/projects"
	"github.com/Adrient-tech/Adrient-Workspace/internal/sheets"
	"github.com/Adrient-tech/Adrient-Workspace/internal/slides"
	"github.com/Adrient-tech/Adrient-Workspace/internal/talent"
	"github.com/Adrient-tech/Adrient-Workspace/internal/tasks"
)

// Report says what a run did.
type Report struct {
	Org      string         `json:"org"`
	Action   string         `json:"action"` // seeded | already_seeded | removed | nothing_to_remove
	Counts   map[string]int `json:"counts,omitempty"`
	Rows     map[string]int `json:"rows_by_table,omitempty"`
	Skipped  []string       `json:"skipped,omitempty"`
	Warnings []string       `json:"warnings,omitempty"`
	Left     map[string]int `json:"left_behind,omitempty"`
}

type seeder struct {
	opt   Options
	admin string // the account people sign in as
	ctx   context.Context
	pool  *pgxpool.Pool
	org   string
	user  string
	rep   *Report
	r     rng
	today time.Time
}

func (s *seeder) warn(section string, err error) {
	if err != nil {
		slog.Warn("demo seed", "section", section, "err", err)
		s.rep.Warnings = append(s.rep.Warnings, section+": "+err.Error())
	}
}

func (s *seeder) count(k string, n int) { s.rep.Counts[k] += n }

func day(t time.Time) string { return t.Format("2006-01-02") }

// Version is the newest seed step. Seed runs every step an organisation has
// not had yet, so a re-run on an older demo org adds only what is missing.
const Version = 6

// BlobStore stores demo file contents for Drive. It is optional: without one
// the Drive folders are still created but files are skipped.
type BlobStore interface {
	Put(ctx context.Context, key, mimeType string, size int64, body io.Reader) error
	Delete(ctx context.Context, key string) error
}

// Options tune a run.
type Options struct {
	Blobs BlobStore
	// BaseURL is the public origin used in demo links (default http://workspace.localtest.me:8080).
	BaseURL string
	// BulkTickets seeds that many extra desk tickets in batches, for scale tests (default 0).
	BulkTickets int
}

// Seed adds the demo data to the organisation with the given slug. It does
// nothing when the organisation already holds the current version.
func Seed(ctx context.Context, pool *pgxpool.Pool, orgSlug string) (Report, error) {
	return SeedWith(ctx, pool, orgSlug, Options{})
}

// SeedWith is Seed with options.
func SeedWith(ctx context.Context, pool *pgxpool.Pool, orgSlug string, opt Options) (Report, error) {
	rep := Report{Action: "seeded", Counts: map[string]int{}}
	if _, err := pool.Exec(ctx, manifestDDL); err != nil {
		return rep, err
	}
	org, user, err := orgAndAdmin(ctx, pool, orgSlug)
	if err != nil {
		return rep, err
	}
	rep.Org = orgSlug
	have := 0
	if err := pool.QueryRow(ctx, `SELECT COALESCE(max(version),0) FROM adrient.demo_seed_runs WHERE org_id=$1`, org).Scan(&have); err != nil {
		return rep, err
	}
	if have >= Version {
		rep.Action = "already_seeded"
		return rep, nil
	}
	tables, err := trackedTables(ctx, pool)
	if err != nil {
		return rep, err
	}
	s := &seeder{ctx: ctx, pool: pool, org: org, user: user, rep: &rep, r: newRNG(), today: time.Now(), opt: opt}
	s.admin = adminUser(ctx, pool, org, user)
	for ver := have + 1; ver <= Version; ver++ {
		before, err := takeSnapshot(ctx, pool, org, tables)
		if err != nil {
			return rep, err
		}
		switch ver {
		case 1:
			s.run()
		case 2:
			s.runV2()
		case 3:
			s.runV3()
		case 4:
			s.runV4()
		case 5:
			s.runV5()
		case 6:
			s.runV6()
		}
		rows, err := recordNew(ctx, pool, org, ver, before, tables)
		if err != nil {
			return rep, err
		}
		if rep.Rows == nil {
			rep.Rows = map[string]int{}
		}
		for k, v := range rows {
			rep.Rows[k] += v
		}
		sum, _ := json.Marshal(rep.Counts)
		if _, err := pool.Exec(ctx, `INSERT INTO adrient.demo_seed_runs (org_id, summary, version) VALUES ($1,$2,$3)
			ON CONFLICT (org_id) DO UPDATE SET version=EXCLUDED.version, summary=EXCLUDED.summary`, org, sum, ver); err != nil {
			return rep, err
		}
	}
	rep.Skipped = append(rep.Skipped, "Photos: not seeded (needs real image files)",
		"Docs bodies and Slides/Sheets content are written by the seeder; Docs text is typed into the editor by scripts/seed-docs.mjs (Docs keeps live-edit state that cannot be written from outside the editor)")
	if opt.BulkTickets > 0 {
		took, err := BulkTickets(ctx, pool, orgSlug, opt.BulkTickets)
		if err != nil {
			return rep, err
		}
		rep.Skipped = append(rep.Skipped, fmt.Sprintf("BulkTickets: %d desk tickets inserted in %s", opt.BulkTickets, took))
	}
	return rep, nil
}

// adminUser picks the account people sign in as: admin@..., else the first user.
func adminUser(ctx context.Context, pool *pgxpool.Pool, org, fallback string) string {
	var id string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM adrient.users WHERE org_id=$1 AND email LIKE 'admin@%' ORDER BY created_at LIMIT 1`, org).Scan(&id); err == nil {
		return id
	}
	return fallback
}

// Remove deletes everything the seeder added.
func Remove(ctx context.Context, pool *pgxpool.Pool, orgSlug string) (Report, error) {
	return RemoveWith(ctx, pool, orgSlug, Options{}, 1)
}

// RemoveWith deletes the rows added by seed steps >= fromVersion (1 = all).
func RemoveWith(ctx context.Context, pool *pgxpool.Pool, orgSlug string, opt Options, fromVersion int) (Report, error) {
	rep := Report{Action: "removed", Org: orgSlug}
	if _, err := pool.Exec(ctx, manifestDDL); err != nil {
		return rep, err
	}
	org, _, err := orgAndAdmin(ctx, pool, orgSlug)
	if err != nil {
		return rep, err
	}
	var have bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM adrient.demo_seed_runs WHERE org_id=$1)`, org).Scan(&have); err != nil {
		return rep, err
	}
	if !have {
		rep.Action = "nothing_to_remove"
		return rep, nil
	}
	if _, err := trackedTables(ctx, pool); err != nil { // fills the key expressions
		return rep, err
	}
	if opt.Blobs != nil {
		keys, _ := pool.Query(ctx, `SELECT f.storage_key FROM adrient.drive_files f JOIN adrient.demo_seed_rows r ON r.tbl='drive_files' AND r.row_id=f.id::text AND r.org_id=f.org_id
			WHERE f.org_id=$1 AND r.ver>=$2 AND f.storage_key IS NOT NULL`, org, fromVersion)
		if keys != nil {
			var ks []string
			for keys.Next() {
				var k string
				if keys.Scan(&k) == nil {
					ks = append(ks, k)
				}
			}
			keys.Close()
			for _, k := range ks {
				_ = opt.Blobs.Delete(ctx, k)
			}
		}
	}
	n, left, err := removeTracked(ctx, pool, org, fromVersion)
	if err != nil {
		return rep, err
	}
	rep.Counts = map[string]int{"rows_deleted": n}
	if len(left) > 0 {
		rep.Left = left
		rep.Warnings = append(rep.Warnings, "some rows could not be deleted (still referenced by other data); run --remove again after removing what refers to them")
		return rep, nil
	}
	if _, err := pool.Exec(ctx, `DELETE FROM adrient.demo_seed_rows WHERE org_id=$1 AND ver>=$2`, org, fromVersion); err != nil {
		return rep, err
	}
	if fromVersion <= 1 {
		if _, err := pool.Exec(ctx, `DELETE FROM adrient.demo_seed_runs WHERE org_id=$1`, org); err != nil {
			return rep, err
		}
	} else if _, err := pool.Exec(ctx, `UPDATE adrient.demo_seed_runs SET version=$2 WHERE org_id=$1`, org, fromVersion-1); err != nil {
		return rep, err
	}
	return rep, nil
}

func orgAndAdmin(ctx context.Context, pool *pgxpool.Pool, slug string) (org, user string, err error) {
	if slug == "" {
		slug = "default"
	}
	if err = pool.QueryRow(ctx, `SELECT id::text FROM adrient.orgs WHERE slug=$1`, slug).Scan(&org); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", fmt.Errorf("organisation %q not found", slug)
		}
		return "", "", err
	}
	err = pool.QueryRow(ctx, `SELECT id::text FROM adrient.users WHERE org_id=$1 ORDER BY created_at LIMIT 1`, org).Scan(&user)
	if err != nil {
		return "", "", fmt.Errorf("organisation %q has no users yet; sign in once first", slug)
	}
	return org, user, nil
}

func (s *seeder) run() {
	inv := inventory.NewRepository(s.pool)
	acct := accounting.NewRepository(s.pool)
	invc := invoicing.NewRepository(s.pool)
	s.seedInventory(inv)
	s.seedHR(acct)
	s.seedTalent()
	s.seedCRM()
	s.seedSales(invc, inv)
	s.seedAccounting(acct, invc, inv)
	s.seedWork()
}

// ---- HR -----------------------------------------------------------------

func (s *seeder) seedHR(acct *accounting.Repository) {
	repo := hr.NewRepository(s.pool).WithAccounting(hr.AccountingAdapter{Repo: acct})
	a, err := repo.ActorFor(s.ctx, s.org, s.user, true)
	if err != nil {
		s.warn("hr actor", err)
		return
	}
	if err := repo.EnsureSeed(s.ctx, s.org); err != nil { // creates the starter shift, leave types and structure
		s.warn("hr settings", err)
	}
	type emp struct{ id, dept string }
	var emps []emp
	heads := map[string]string{}
	for i := 0; i < 25; i++ {
		dep := deptNames[i%len(deptNames)]
		f, l := s.r.personName()
		f = fmt.Sprintf("%s", f)
		start := s.today.AddDate(0, -(6 + s.r.Intn(40)), -s.r.Intn(25))
		titles := titlesByDep[dep]
		in := hr.NewHireInput{Name: f + " " + l, Email: demoEmail(f+fmt.Sprint(i), l), Phone: s.r.phone(), JobTitle: titles[s.r.Intn(len(titles))],
			Department: dep, StartDate: day(start), SourceRef: fmt.Sprintf("demo:emp:%d", i)}
		if h := heads[dep]; h != "" {
			in.ManagerEmpID = h
		}
		e, _, err := repo.CreateFromNewHire(s.ctx, a, in, nil)
		if err != nil {
			s.warn("hr employee", err)
			continue
		}
		if heads[dep] == "" {
			heads[dep] = e.ID
		}
		emps = append(emps, emp{e.ID, dep})
	}
	s.count("hr_employees", len(emps))
	if len(emps) == 0 {
		return
	}
	// Salaries.
	structs, err := repo.ListStructures(s.ctx, a)
	if err != nil || len(structs) == 0 {
		s.warn("hr structures", errors.Join(err, errors.New("no salary structure")))
		return
	}
	sal := 0
	for i, e := range emps {
		annual := float64(360000 + (i%9)*60000)
		lines, err := repo.PreviewStructure(s.ctx, a, structs[0].Lines, annual)
		if err != nil {
			s.warn("hr preview", err)
			break
		}
		_, err = repo.SaveSalary(s.ctx, a, e.id, hr.EmployeeSalary{EffectiveFrom: day(s.today.AddDate(-1, 0, 0)), StructureID: structs[0].ID, AnnualGross: annual, Lines: lines, PFEnabled: true, PTEnabled: true, TDSEnabled: true})
		if err != nil {
			s.warn("hr salary", err)
			continue
		}
		sal++
	}
	s.count("hr_salaries", sal)
	// Leave: requests by employees, approved by their manager (heads: by HR).
	types, _ := repo.ListLeaveTypes(s.ctx, s.org, false)
	var paid []hr.LeaveType
	for _, t := range types {
		if t.Paid {
			paid = append(paid, t)
		}
	}
	nl := 0
	for i, e := range emps {
		if len(paid) == 0 || i%2 == 1 {
			continue
		}
		from := s.today.AddDate(0, 0, -20+i)
		for from.Weekday() == time.Saturday || from.Weekday() == time.Sunday {
			from = from.AddDate(0, 0, 1)
		}
		req, err := repo.ApplyLeave(s.ctx, a, hr.LeaveInput{EmployeeID: e.id, TypeID: paid[i%len(paid)].ID, From: day(from), To: day(from), Reason: "Family function"})
		if err != nil {
			s.warn("hr leave apply", err)
			continue
		}
		approver := a
		if mgr := heads[e.dept]; mgr != "" && mgr != e.id {
			approver = hr.Actor{OrgID: s.org, UserID: s.user, EmployeeID: mgr, IsManager: true}
		}
		if _, err := repo.DecideLeave(s.ctx, approver, req.ID, i%6 != 0, ""); err != nil {
			s.warn("hr leave decide", err)
			continue
		}
		nl++
	}
	s.count("hr_leave_requests", nl)
	// Attendance: punches for the last 14 working days.
	var csv strings.Builder
	codes := map[string]string{}
	if list, err := repo.ListEmployees(s.ctx, a, hr.EmployeeFilter{}); err == nil {
		for _, e := range list {
			codes[e.ID] = e.EmpCode
		}
	}
	punches := 0
	csv.WriteString("emp_code,timestamp\n")
	for d := 1; d <= 20; d++ {
		t := s.today.AddDate(0, 0, -d)
		if t.Weekday() == time.Saturday || t.Weekday() == time.Sunday {
			continue
		}
		for i, e := range emps {
			if (d+i)%11 == 0 {
				continue // absent or on leave
			}
			code := codes[e.id]
			if code == "" {
				continue
			}
			in := time.Date(t.Year(), t.Month(), t.Day(), 9, 5+(i*7+d)%40, 0, 0, time.Local)
			out := time.Date(t.Year(), t.Month(), t.Day(), 18, (i*5+d)%50, 0, 0, time.Local)
			csv.WriteString(code + "," + in.Format("2006-01-02 15:04:05") + "\n")
			csv.WriteString(code + "," + out.Format("2006-01-02 15:04:05") + "\n")
			punches += 2
		}
	}
	if res, err := repo.ImportPunchesCSV(s.ctx, a, strings.NewReader(csv.String())); err != nil {
		s.warn("hr attendance", err)
	} else {
		s.count("hr_punches", res.Imported)
	}
	// One payroll run for last month, locked (posts the journal to Accounting).
	period := s.today.AddDate(0, -1, 0).Format("2006-01")
	run, err := repo.CreateRun(s.ctx, a, period)
	if err != nil {
		s.warn("hr payroll run", err)
		return
	}
	if run, err = repo.MarkReviewed(s.ctx, a, run.ID); err != nil {
		s.warn("hr payroll review", err)
		return
	}
	if _, err = repo.LockRun(s.ctx, a, run.ID); err != nil {
		s.warn("hr payroll lock", err)
		return
	}
	s.count("hr_payroll_runs", 1)
}

// ---- Talent -------------------------------------------------------------

func (s *seeder) seedTalent() {
	repo := talent.NewRepository(s.pool)
	a := talent.Actor{OrgID: s.org, UserID: s.user, IsHR: true}
	set, err := repo.GetSettings(s.ctx, s.org)
	if err != nil || len(set.Stages) < 3 {
		s.warn("talent settings", err)
		return
	}
	jobs := []talent.JobInput{
		{Title: "Senior Backend Engineer", Department: "Engineering", Location: "Pune", EmploymentType: "full_time", Openings: 2, Description: "Own services written in Go.", Currency: "INR"},
		{Title: "Regional Sales Manager", Department: "Sales", Location: "Indore", EmploymentType: "full_time", Openings: 1, Description: "Grow accounts across central India.", Currency: "INR"},
		{Title: "Accounts Executive", Department: "Finance", Location: "Nashik", EmploymentType: "full_time", Openings: 1, Description: "Daily bookkeeping, GST filing support.", Currency: "INR"},
	}
	n := 0
	for ji, j := range jobs {
		raw, err := repo.SaveJob(s.ctx, a, "", j)
		if err != nil {
			s.warn("talent job", err)
			continue
		}
		var jr struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(raw, &jr)
		if _, err := repo.SetJobStatus(s.ctx, a, jr.ID, "open"); err != nil {
			s.warn("talent job open", err)
		}
		for c := 0; c < 4; c++ {
			f, l := s.r.personName()
			raw, err := repo.AddCandidate(s.ctx, a, talent.ApplyInput{JobID: jr.ID, Name: f + " " + l, Email: demoEmail(f+fmt.Sprint(ji*10+c), l), Phone: s.r.phone(),
				Source: []string{"referral", "linkedin", "job_board", "walk_in"}[c%4], CoverNote: "Demo candidate."})
			if err != nil {
				s.warn("talent candidate", err)
				continue
			}
			var ar struct {
				ID string `json:"id"`
			}
			_ = json.Unmarshal(raw, &ar)
			if st := (ji + c) % len(set.Stages); st > 0 {
				if _, err := repo.MoveApplication(s.ctx, a, ar.ID, set.Stages[st].ID, ""); err != nil {
					s.warn("talent move", err)
				}
			}
			n++
		}
	}
	s.count("talent_jobs", len(jobs))
	s.count("talent_candidates", n)
}

// ---- Inventory ----------------------------------------------------------

type invState struct {
	locs      []string
	products  []string
	prodCost  map[string]float64
	suppliers []string
}

var inventoryState invState

func (s *seeder) seedInventory(inv *inventory.Repository) {
	st := &inventoryState
	*st = invState{prodCost: map[string]float64{}}
	locs, err := inv.ListLocations(s.ctx, s.org)
	if err != nil {
		s.warn("inventory locations", err)
		return
	}
	for _, l := range locs {
		st.locs = append(st.locs, l.ID)
	}
	if len(st.locs) == 0 {
		l, err := inv.SaveLocation(s.ctx, s.org, "", inventory.Location{Name: "Main Warehouse", Code: "MAIN", IsDefault: true})
		if err != nil {
			s.warn("inventory location", err)
			return
		}
		st.locs = append(st.locs, l.ID)
	}
	if len(st.locs) < 2 {
		l, err := inv.SaveLocation(s.ctx, s.org, "", inventory.Location{Name: "Pune Store", Code: "PUNE"})
		if err != nil {
			s.warn("inventory location 2", err)
		} else {
			st.locs = append(st.locs, l.ID)
		}
	}
	for i := 0; i < 6; i++ {
		sup, err := inv.SaveSupplier(s.ctx, s.org, "", inventory.Supplier{Name: s.r.company(), Email: fmt.Sprintf("orders%d@%s", i, demoDomain), Phone: s.r.phone(), Notes: "Demo supplier"})
		if err != nil {
			s.warn("inventory supplier", err)
			continue
		}
		st.suppliers = append(st.suppliers, sup.ID)
	}
	for i := 0; i < 150; i++ {
		kind := prodKinds[i%len(prodKinds)]
		name := fmt.Sprintf("%s %s", kind, prodVariants[(i/len(prodKinds))%len(prodVariants)])
		cost := float64(40 + s.r.Intn(900))
		qty := float64(10 + s.r.Intn(190))
		p, err := inv.CreateProduct(s.ctx, s.org, s.user, inventory.ProductFields{Name: name, SKU: fmt.Sprintf("DEMO-%04d", i+1), Category: categories[i%len(categories)], Unit: "pcs",
			Price: cost * 1.35, Cost: cost, MinStock: 10, ReorderQty: 50, OpeningQty: qty, OpeningLocationID: st.locs[0]})
		if err != nil {
			s.warn("inventory product", err)
			continue
		}
		st.products = append(st.products, p.ID)
		st.prodCost[p.ID] = cost
		if len(st.locs) > 1 && i%2 == 0 {
			if _, err := inv.AdjustStock(s.ctx, s.org, s.user, p.ID, inventory.AdjustInput{LocationID: st.locs[1], Qty: float64(5 + s.r.Intn(60)), Reason: "receive", UnitCost: cost}); err != nil {
				s.warn("inventory stock 2", err)
			}
		}
	}
	s.count("inventory_products", len(st.products))
	// Purchase orders: three received in full, two still open.
	if len(st.suppliers) == 0 || len(st.products) < 6 {
		return
	}
	made := 0
	for i := 0; i < 5; i++ {
		var in inventory.POInput
		in.SupplierID, in.ShipToLocID = st.suppliers[i%len(st.suppliers)], st.locs[0]
		for k := 0; k < 3; k++ {
			pid := st.products[(i*7+k*5)%len(st.products)]
			c := st.prodCost[pid]
			in.Lines = append(in.Lines, struct {
				ProductID   string   `json:"product_id"`
				Description string   `json:"description"`
				Qty         float64  `json:"qty"`
				UnitCost    *float64 `json:"unit_cost"`
				TaxPct      float64  `json:"tax_pct"`
			}{ProductID: pid, Qty: float64(20 + s.r.Intn(40)), UnitCost: &c, TaxPct: 18})
		}
		po, err := inv.CreatePO(s.ctx, s.org, s.user, in)
		if err != nil {
			s.warn("inventory po", err)
			continue
		}
		if _, err := inv.ApprovePO(s.ctx, s.org, s.user, po.ID); err != nil {
			s.warn("inventory po approve", err)
			continue
		}
		made++
		if i >= 3 {
			continue // left open
		}
		full, err := inv.GetPO(s.ctx, s.org, po.ID)
		if err != nil {
			continue
		}
		var g inventory.GRNInput
		g.POID, g.LocationID = po.ID, st.locs[0]
		for _, l := range full.Lines {
			g.Lines = append(g.Lines, struct {
				POLineID  string   `json:"po_line_id"`
				ProductID string   `json:"product_id"`
				Qty       float64  `json:"qty"`
				UnitCost  *float64 `json:"unit_cost"`
				LotNo     string   `json:"lot_no"`
				Expiry    string   `json:"expiry"`
				Serials   []string `json:"serials"`
			}{POLineID: l.ID, ProductID: l.ProductID, Qty: l.Qty})
		}
		if _, err := inv.CreateGRN(s.ctx, s.org, s.user, g); err != nil {
			s.warn("inventory grn", err)
		}
	}
	s.count("purchase_orders", made)
}

// ---- CRM ----------------------------------------------------------------

func (s *seeder) seedCRM() {
	repo := crm.NewRepository(s.pool)
	crepo := contacts.NewRepository(s.pool)
	set, err := repo.GetSettings(s.ctx, s.org)
	if err != nil {
		s.warn("crm settings", err)
		return
	}
	var open []string
	for _, st := range set.Stages {
		if st.ID != crm.StageWon && st.ID != crm.StageLost {
			open = append(open, st.ID)
		}
	}
	type co struct{ id, contact string }
	var cos []co
	for i := 0; i < 40; i++ {
		name := s.r.company()
		raw, err := repo.SaveCompany(s.ctx, s.org, "", crm.Company{Name: name, Industry: []string{"Manufacturing", "Retail", "Distribution", "Services"}[i%4], Phone: s.r.phone(), Address: s.r.pick(cities), Notes: "Demo company"})
		if err != nil {
			s.warn("crm company", err)
			continue
		}
		var cr struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(raw, &cr)
		f, l := s.r.personName()
		c, err := crepo.Create(s.ctx, s.org, s.user, contacts.Fields{DisplayName: f + " " + l, FirstName: f, LastName: l, Company: name, JobTitle: "Purchase Head", Emails: []string{demoEmail(f+fmt.Sprint(i), l)}, Phones: []string{s.r.phone()}})
		if err != nil {
			s.warn("crm contact", err)
			continue
		}
		if _, err := repo.SavePerson(s.ctx, s.org, "", crm.Person{ContactID: c.ID, CompanyID: cr.ID, Status: []string{"lead", "customer"}[i%2], Source: []string{"referral", "web", "event"}[i%3]}); err != nil {
			s.warn("crm person", err)
		}
		cos = append(cos, co{cr.ID, c.ID})
	}
	s.count("crm_companies", len(cos))
	if len(cos) == 0 || len(open) == 0 {
		return
	}
	nd := 0
	for i := 0; i < 30; i++ {
		c := cos[i%len(cos)]
		stage := open[i%len(open)]
		raw, err := repo.SaveDeal(s.ctx, s.org, "", crm.Deal{Title: fmt.Sprintf("%s order %d", []string{"Annual supply", "Pilot", "Renewal", "Bulk"}[i%4], 100+i), CompanyID: c.id, ContactID: c.contact, Stage: stage,
			Value: float64(25000 + s.r.Intn(400000)), Currency: "INR", Billing: "one_time", ExpectedClose: day(s.today.AddDate(0, 0, 10+i*3)), Source: "referral", Notes: "Demo deal"})
		if err != nil {
			s.warn("crm deal", err)
			continue
		}
		var dr struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(raw, &dr)
		nd++
		switch {
		case i%6 == 0:
			_, err = repo.MoveDeal(s.ctx, s.org, dr.ID, crm.StageWon, "Demo: signed")
		case i%9 == 0:
			_, err = repo.MoveDeal(s.ctx, s.org, dr.ID, crm.StageLost, "Demo: went with a competitor")
		}
		if err != nil {
			s.warn("crm move", err)
		}
	}
	s.count("crm_deals", nd)
}

// ---- Sales: invoices and payments --------------------------------------

func (s *seeder) seedSales(invc *invoicing.Repository, inv *inventory.Repository) {
	names := []string{}
	for i := 0; i < 10; i++ {
		names = append(names, s.r.company())
	}
	made := 0
	for m := 2; m >= 0; m-- {
		for k := 0; k < 8; k++ {
			issue := time.Date(s.today.Year(), s.today.Month(), 1, 0, 0, 0, 0, time.Local).AddDate(0, -m, 2+k*3)
			if issue.After(s.today) {
				continue
			}
			client := names[(m*8+k)%len(names)]
			var lines []invoicing.Line
			for l := 0; l < 1+s.r.Intn(3); l++ {
				lines = append(lines, invoicing.Line{Description: prodKinds[s.r.Intn(len(prodKinds))], Qty: float64(2 + s.r.Intn(20)), Unit: "pcs", Price: float64(100 + s.r.Intn(1500)), TaxRate: 18, TaxName: "GST"})
			}
			doc, err := invc.SaveDocument(s.ctx, s.org, s.user, "", invoicing.DocInput{Kind: "invoice", ClientName: client, ClientEmail: fmt.Sprintf("accounts%d@%s", k, demoDomain), Currency: "INR",
				IssueDate: day(issue), DueDate: day(issue.AddDate(0, 0, 30)), Lines: lines, DiscountKind: "percent", Notes: "Demo invoice", Schedule: []invoicing.Schedule{}, Attachments: []invoicing.Attachment{}})
			if err != nil {
				s.warn("sales invoice", err)
				continue
			}
			made++
			if _, _, err := invc.MarkSent(s.ctx, s.org, doc.ID); err != nil {
				s.warn("sales invoice sent", err)
			}
			if (m*8+k)%4 != 3 { // most are paid
				if _, err := invc.AddPayment(s.ctx, s.org, s.user, doc.ID, invoicing.Payment{Amount: doc.Total, Method: "bank_transfer", PaidAt: day(issue.AddDate(0, 0, 10+k)), Note: "Demo payment"}); err != nil {
					s.warn("sales payment", err)
				}
			}
		}
	}
	s.count("invoices", made)
	_ = inv
}

// ---- Accounting ---------------------------------------------------------

func (s *seeder) seedAccounting(acct *accounting.Repository, invc *invoicing.Repository, inv *inventory.Repository) {
	if err := acct.EnsureSeeded(s.ctx, s.org); err != nil {
		s.warn("accounting chart", err)
		return
	}
	accts, err := acct.ListAccounts(s.ctx, s.org)
	if err != nil {
		s.warn("accounting accounts", err)
		return
	}
	var exp []accounting.Account
	bank := ""
	for _, a := range accts {
		if a.Type == "expense" && a.Active && a.SystemKey == "" {
			exp = append(exp, a)
		}
		if a.SystemKey == "bank" {
			bank = a.ID
		}
	}
	nb := 0
	for i := 0; i < 12 && len(exp) > 0; i++ {
		d := s.today.AddDate(0, 0, -5-i*7)
		_, err := acct.SaveBill(s.ctx, s.org, s.user, "", accounting.BillInput{Kind: "bill", VendorName: s.r.company(), BillDate: day(d), DueDate: day(d.AddDate(0, 0, 30)), Currency: "INR",
			Lines: []accounting.BillLineIn{{AccountID: exp[i%len(exp)].ID, Description: "Demo bill", Amount: float64(2000 + s.r.Intn(30000)), TaxRate: 18}}, Notes: "Demo bill"})
		if err != nil {
			s.warn("accounting bill", err)
			continue
		}
		nb++
	}
	s.count("bills", nb)
	ne := 0
	for i := 0; i < 15 && len(exp) > 0; i++ {
		d := s.today.AddDate(0, 0, -2-i*6)
		_, err := acct.SaveExpense(s.ctx, s.org, s.user, "", accounting.Expense{Date: day(d), VendorName: s.r.company(), CategoryID: exp[(i*3)%len(exp)].ID, Amount: float64(300 + s.r.Intn(8000)),
			GSTRate: 18, PaidFromID: bank, Paid: true, Notes: "Demo expense"})
		if err != nil {
			s.warn("accounting expense", err)
			continue
		}
		ne++
	}
	s.count("expenses", ne)
	if bank != "" {
		var csv strings.Builder
		csv.WriteString("Date,Description,Debit,Credit\n")
		for i := 0; i < 36; i++ {
			d := s.today.AddDate(0, 0, -i*2-1)
			if i%3 == 0 {
				fmt.Fprintf(&csv, "%s,UPI/%s receipt,,%d.00\n", d.Format("02/01/2006"), s.r.company(), 5000+s.r.Intn(60000))
			} else {
				fmt.Fprintf(&csv, "%s,NEFT/%s payment,%d.00,\n", d.Format("02/01/2006"), s.r.company(), 1500+s.r.Intn(25000))
			}
		}
		if res, err := acct.ImportCSV(s.ctx, s.org, s.user, bank, csv.String()); err != nil {
			s.warn("accounting bank import", err)
		} else {
			s.count("bank_lines", res.Imported)
		}
	}
	// Post invoices and POS sales into the books, the same sync the app runs.
	posRepo := pos.NewRepository(s.pool, inv)
	if _, err := acct.Sync(s.ctx, s.org, s.user, posRepo, invc, true); err != nil {
		s.warn("accounting sync", err)
	}
}

// ---- Projects, tasks, calendar, files -------------------------------------

func (s *seeder) seedWork() {
	pr := projects.NewRepository(s.pool)
	team, err := pr.CreateTeam(s.ctx, s.org, "Delivery", "DLV", "", "")
	if err != nil {
		s.warn("projects team", err)
	} else {
		np := 0
		for _, name := range []string{"Warehouse automation rollout", "GST filing automation", "Customer onboarding revamp"} {
			p, err := pr.CreateProject(s.ctx, s.org, projects.ProjectFields{Name: name, Description: "Demo project"})
			if err != nil {
				s.warn("projects project", err)
				continue
			}
			np++
			for i, t := range []string{"Scope and plan", "Vendor shortlist", "Pilot at Pune store", "Train the team", "Go live review"} {
				status := []string{"todo", "in_progress", "done"}[i%3]
				if _, err := pr.CreateIssue(s.ctx, s.org, team.ID, s.user, projects.IssueFields{Title: t, ProjectID: p.ID, Status: status}); err != nil {
					s.warn("projects issue", err)
				}
			}
		}
		s.count("projects", np)
	}
	tr := tasks.NewRepository(s.pool)
	list, err := tr.CreateList(s.ctx, s.org, s.user, tasks.ListFields{Name: "Month-end checklist"})
	if err != nil {
		s.warn("tasks list", err)
	} else {
		nt := 0
		for i, t := range []string{"Reconcile bank statement", "Chase overdue invoices", "File GST return", "Review stock counts", "Approve payroll", "Send MIS to management"} {
			due := s.today.AddDate(0, 0, 3+i*2)
			if _, err := tr.CreateTask(s.ctx, s.org, list.ID, s.user, tasks.TaskFields{Title: t, DueAt: &due}); err != nil {
				s.warn("tasks task", err)
				continue
			}
			nt++
		}
		s.count("tasks", nt)
	}
	cal := calendar.NewRepository(s.pool)
	ne := 0
	for i, t := range []string{"Weekly sales review", "Payroll cut-off", "Supplier call: Kaveri Textiles", "Warehouse safety drill", "Quarterly business review", "Customer demo", "Hiring panel: Senior Backend Engineer", "Month-end close", "Team lunch", "GST filing deadline"} {
		start := time.Date(s.today.Year(), s.today.Month(), s.today.Day(), 10+i%6, 0, 0, 0, time.Local).AddDate(0, 0, i-2)
		if _, err := cal.Create(s.ctx, s.org, s.user, calendar.Fields{Title: t, StartAt: start, EndAt: start.Add(time.Hour), Description: "Demo event", ItemType: "event"}); err != nil {
			s.warn("calendar event", err)
			continue
		}
		ne++
	}
	s.count("calendar_events", ne)
	// Files in the editor apps.
	dr, sr, slr := docs.NewRepository(s.pool), sheets.NewRepository(s.pool), slides.NewRepository(s.pool)
	for _, t := range []string{"Quarterly business review notes", "Supplier onboarding policy", "Store opening checklist"} {
		if d, err := dr.Create(s.ctx, s.org, s.user, "Demo: "+t); err != nil {
			s.warn("docs", err)
		} else {
			_ = dr.SetPreview(s.ctx, s.org, d.ID, "<h1>"+t+"</h1><p>Demo document.</p>")
			s.count("docs", 1)
		}
	}
	for _, t := range []string{"Sales by region (demo)", "Stock count sheet (demo)", "Budget FY (demo)"} {
		sh, err := sr.Create(s.ctx, s.org, s.user, t)
		if err != nil {
			s.warn("sheets", err)
			continue
		}
		var cells []map[string]any
		head := []string{"Region", "Q1", "Q2", "Q3", "Q4"}
		for c, h := range head {
			cells = append(cells, map[string]any{"r": 0, "c": c, "v": map[string]any{"v": h, "m": h, "bl": 1}})
		}
		for r, reg := range []string{"West", "North", "South", "East", "Central"} {
			cells = append(cells, map[string]any{"r": r + 1, "c": 0, "v": map[string]any{"v": reg, "m": reg}})
			for c := 1; c <= 4; c++ {
				v := 120000 + s.r.Intn(400000)
				cells = append(cells, map[string]any{"r": r + 1, "c": c, "v": map[string]any{"v": v, "m": fmt.Sprint(v), "ct": map[string]any{"fa": "General", "t": "n"}}})
			}
		}
		data, _ := json.Marshal([]map[string]any{{"name": "Sheet1", "id": "sheet_demo_1", "status": 1, "order": 0, "celldata": cells}})
		if err := sr.Save(s.ctx, s.org, sh.ID, string(data)); err != nil {
			s.warn("sheets save", err)
		}
		s.count("sheets", 1)
	}
	for _, t := range []string{"Company overview (demo)", "Q3 results (demo)"} {
		if _, err := slr.Create(s.ctx, s.org, s.user, t); err != nil {
			s.warn("slides", err)
			continue
		}
		s.count("slides", 1)
	}
}
