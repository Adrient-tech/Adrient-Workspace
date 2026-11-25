package crm

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// ImportSources are the supported origins. They differ only in the suggested
// column mapping and the external-id namespace; the engine is the same.
var ImportSources = map[string]bool{"zoho": true, "hubspot": true, "salesforce": true, "csv": true}

// ImportEntities are the record kinds an import can create.
var ImportEntities = map[string]bool{"lead": true, "company": true, "deal": true}

// Targets lists the CRM fields a column can map to, per entity.
var Targets = map[string][]string{
	"lead":    {"name", "first_name", "last_name", "email", "phone", "company", "job_title", "status", "source", "tags", "notes", "owner_email", "external_id"},
	"company": {"name", "industry", "website", "phone", "address", "tags", "notes", "owner_email", "external_id"},
	"deal":    {"title", "company", "contact_email", "stage", "value", "currency", "expected_close", "source", "notes", "close_reason", "owner_email", "external_id"},
}

func normHeader(h string) string {
	return strings.ToLower(regexp.MustCompile(`[^a-zA-Z0-9]+`).ReplaceAllString(h, ""))
}

// presets: normalised header -> target, per source then entity. "*" applies to every source.
var presets = map[string]map[string]map[string]string{
	"zoho": {
		"lead":    {"recordid": "external_id", "firstname": "first_name", "lastname": "last_name", "email": "email", "phone": "phone", "mobile": "phone", "company": "company", "designation": "job_title", "leadsource": "source", "leadstatus": "status", "description": "notes", "tag": "tags", "leadowner": "owner_email"},
		"company": {"recordid": "external_id", "accountname": "name", "website": "website", "phone": "phone", "industry": "industry", "billingstreet": "address", "description": "notes"},
		"deal":    {"recordid": "external_id", "dealname": "title", "stage": "stage", "amount": "value", "closingdate": "expected_close", "leadsource": "source", "accountname": "company", "description": "notes", "reasonforloss": "close_reason", "contactemail": "contact_email"},
	},
	"hubspot": {
		"lead":    {"recordid": "external_id", "firstname": "first_name", "lastname": "last_name", "email": "email", "phonenumber": "phone", "mobilephonenumber": "phone", "companyname": "company", "jobtitle": "job_title", "originalsource": "source", "leadstatus": "status", "lifecyclestage": "status", "contactowner": "owner_email"},
		"company": {"recordid": "external_id", "companyname": "name", "name": "name", "companydomainname": "website", "domain": "website", "phonenumber": "phone", "industry": "industry", "streetaddress": "address", "description": "notes"},
		"deal":    {"recordid": "external_id", "dealname": "title", "dealstage": "stage", "amount": "value", "closedate": "expected_close", "originalsource": "source", "associatedcompany": "company", "associatedcontact": "contact_email", "dealowner": "owner_email"},
	},
	"salesforce": {
		"lead":    {"id": "external_id", "firstname": "first_name", "lastname": "last_name", "email": "email", "phone": "phone", "mobilephone": "phone", "company": "company", "title": "job_title", "leadsource": "source", "status": "status", "description": "notes", "ownerid": "owner_email"},
		"company": {"id": "external_id", "name": "name", "website": "website", "phone": "phone", "industry": "industry", "billingstreet": "address", "description": "notes"},
		"deal":    {"id": "external_id", "name": "title", "stagename": "stage", "amount": "value", "closedate": "expected_close", "leadsource": "source", "accountname": "company", "description": "notes", "currencyisocode": "currency"},
	},
	"csv": {
		"lead":    {"id": "external_id", "name": "name", "fullname": "name", "displayname": "name", "firstname": "first_name", "lastname": "last_name", "email": "email", "emailaddress": "email", "phone": "phone", "mobile": "phone", "company": "company", "jobtitle": "job_title", "title": "job_title", "status": "status", "source": "source", "tags": "tags", "notes": "notes"},
		"company": {"id": "external_id", "name": "name", "companyname": "name", "industry": "industry", "website": "website", "phone": "phone", "address": "address", "tags": "tags", "notes": "notes"},
		"deal":    {"id": "external_id", "title": "title", "name": "title", "dealname": "title", "company": "company", "contactemail": "contact_email", "stage": "stage", "value": "value", "amount": "value", "currency": "currency", "expectedclose": "expected_close", "closedate": "expected_close", "source": "source", "notes": "notes"},
	},
}

// SuggestMapping proposes header -> target for a file's headers. Unrecognised headers are omitted.
func SuggestMapping(source, entity string, headers []string) map[string]string {
	out := map[string]string{}
	p := presets[source][entity]
	g := presets["csv"][entity]
	used := map[string]bool{}
	for _, h := range headers {
		n := normHeader(h)
		t := p[n]
		if t == "" {
			t = g[n]
		}
		if t != "" && (!used[t] || t == "phone" || t == "email") {
			out[h] = t
			used[t] = true
		}
	}
	return out
}

// ParseCSV reads a CSV file (BOM and CRLF tolerant) into headers and rows.
func ParseCSV(data []byte, maxRows int) ([]string, [][]string, error) {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	rd := csv.NewReader(bytes.NewReader(data))
	rd.LazyQuotes = true
	rd.FieldsPerRecord = -1
	head, err := rd.Read()
	if err != nil {
		return nil, nil, invalid("the file is empty or is not a CSV")
	}
	for i := range head {
		head[i] = strings.TrimSpace(head[i])
	}
	var rows [][]string
	for {
		rec, err := rd.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, nil, invalid("CSV error: %v", err)
		}
		blank := true
		for _, c := range rec {
			if strings.TrimSpace(c) != "" {
				blank = false
			}
		}
		if blank {
			continue
		}
		if len(rows) >= maxRows {
			return nil, nil, invalid("too many rows (limit %d); split the file", maxRows)
		}
		rows = append(rows, rec)
	}
	return head, rows, nil
}

// MapStage maps a foreign stage name to one of the org's stages.
func MapStage(raw string, stages []Stage) string {
	n := normHeader(raw)
	switch {
	case n == "won" || n == "closedwon" || n == "closewon" || strings.Contains(n, "won") && !strings.Contains(n, "lost"):
		return StageWon
	case n == "lost" || n == "closedlost" || strings.Contains(n, "lost"):
		return StageLost
	}
	for _, s := range stages {
		if normHeader(s.ID) == n || normHeader(s.Name) == n {
			return s.ID
		}
	}
	if len(stages) == 0 {
		return "new"
	}
	return stages[0].ID
}

// MapStatus maps a foreign lead status or lifecycle stage to lead|customer|inactive.
func MapStatus(raw string) string {
	n := normHeader(raw)
	switch {
	case n == "":
		return "lead"
	case strings.Contains(n, "customer") || strings.Contains(n, "converted") || n == "won":
		return "customer"
	case strings.Contains(n, "inactive") || strings.Contains(n, "junk") || strings.Contains(n, "unqualified") || strings.Contains(n, "lost") || strings.Contains(n, "dead") || strings.Contains(n, "unsubscribed"):
		return "inactive"
	}
	return "lead"
}

var dateLayouts = []string{"2006-01-02", time.RFC3339, "2006-01-02 15:04:05", "02/01/2006", "01/02/2006", "2 Jan 2006", "Jan 2, 2006", "January 2, 2006", "02-01-2006", "02-Jan-2006"}

// ParseDate accepts ISO, day-first (31/12/2025) and US (12/31/2025) dates. When a
// slash date is ambiguous (both parts <= 12) it is read day-first.
func ParseDate(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", false
	}
	for _, l := range dateLayouts {
		if t, err := time.Parse(l, s); err == nil {
			return t.Format("2006-01-02"), true
		}
	}
	return "", false
}

var moneyJunk = regexp.MustCompile(`[^0-9.\-]`)

// ParseMoney strips currency symbols and thousands separators.
func ParseMoney(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	if i := strings.IndexAny(s, "0123456789-"); i > 0 {
		s = s[i:]
	}
	f, err := strconv.ParseFloat(moneyJunk.ReplaceAllString(s, ""), 64)
	if err != nil || f < 0 {
		return 0, fmt.Errorf("%q is not a valid amount", s)
	}
	return f, nil
}

func splitMulti(s string) []string {
	var out []string
	for _, p := range regexp.MustCompile(`[;,|]`).Split(s, -1) {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// ImportError is one rejected row.
type ImportError struct {
	Row     int    `json:"row"` // 1-based data row (header excluded)
	Message string `json:"message"`
}

// ImportReport is the outcome of a (dry) run.
type ImportReport struct {
	Source  string        `json:"source"`
	Entity  string        `json:"entity"`
	DryRun  bool          `json:"dry_run"`
	Total   int           `json:"total"`
	Created int           `json:"created"`
	Updated int           `json:"updated"`
	Skipped int           `json:"skipped"`
	Errors  []ImportError `json:"errors"`
	JobID   string        `json:"job_id"`
}

// ErrorsCSV is the downloadable error report.
func (ir ImportReport) ErrorsCSV() []byte {
	rp := Report{Columns: []string{"row", "error"}}
	for _, e := range ir.Errors {
		rp.Rows = append(rp.Rows, []any{e.Row, e.Message})
	}
	return rp.CSV()
}

// ImportRequest is one import run.
type ImportRequest struct {
	Source  string            `json:"source"`
	Entity  string            `json:"entity"`
	CSV     string            `json:"csv"`
	Mapping map[string]string `json:"mapping"` // CSV header -> target field or custom:<key>
	DryRun  bool              `json:"dry_run"`
}

// ImportPreview is what the wizard shows before mapping.
type ImportPreview struct {
	Headers   []string          `json:"headers"`
	Suggested map[string]string `json:"suggested"`
	Sample    [][]string        `json:"sample"`
	Total     int               `json:"total"`
	Targets   []string          `json:"targets"`
	Custom    []FieldDef        `json:"custom_fields"`
}

func (r *Repository) PreviewImport(ctx context.Context, org, source, entity, csvText string) (ImportPreview, error) {
	if !ImportSources[source] || !ImportEntities[entity] {
		return ImportPreview{}, invalid("unknown source or entity")
	}
	head, rows, err := ParseCSV([]byte(csvText), 20000)
	if err != nil {
		return ImportPreview{}, err
	}
	defs, _ := r.ListFieldDefs(ctx, org)
	var custom []FieldDef
	for _, d := range defs {
		if d.Entity == entity || (entity == "lead" && d.Entity == "contact") {
			custom = append(custom, d)
		}
	}
	p := ImportPreview{Headers: head, Suggested: SuggestMapping(source, entity, head), Total: len(rows), Targets: Targets[entity], Custom: custom}
	for i := 0; i < len(rows) && i < 5; i++ {
		p.Sample = append(p.Sample, rows[i])
	}
	return p, nil
}

type importRow map[string]string

func buildRow(head []string, rec []string, mapping map[string]string) importRow {
	row := importRow{}
	for i, h := range head {
		t := mapping[h]
		if t == "" || i >= len(rec) {
			continue
		}
		v := strings.TrimSpace(rec[i])
		if v == "" {
			continue
		}
		if row[t] != "" && (t == "phone" || t == "email" || t == "tags") {
			row[t] += "|" + v
		} else if row[t] == "" {
			row[t] = v
		}
	}
	return row
}

// RunImport maps and loads a CSV. The whole file is processed in one
// transaction with a savepoint per row: bad rows are reported and skipped,
// good rows commit together, and a dry run rolls everything back after
// counting. Rows are matched to existing records by external id (per source),
// then by e-mail (leads), name (companies) or title+company (deals), so running
// the same file again updates instead of duplicating. Imports do not trigger
// automation rules (a bulk load must not fire thousands of follow-ups).
func (r *Repository) RunImport(ctx context.Context, org, user string, req ImportRequest) (ImportReport, error) {
	rep := ImportReport{Source: req.Source, Entity: req.Entity, DryRun: req.DryRun, Errors: []ImportError{}}
	if !ImportSources[req.Source] || !ImportEntities[req.Entity] {
		return rep, invalid("unknown source or entity")
	}
	head, rows, err := ParseCSV([]byte(req.CSV), 20000)
	if err != nil {
		return rep, err
	}
	valid := map[string]bool{}
	for _, t := range Targets[req.Entity] {
		valid[t] = true
	}
	defs, err := r.ListFieldDefs(ctx, org)
	if err != nil {
		return rep, err
	}
	var custom []FieldDef
	for _, d := range defs {
		if d.Entity == req.Entity || (req.Entity == "lead" && (d.Entity == "lead" || d.Entity == "contact")) {
			custom = append(custom, d)
		}
	}
	byKey := map[string]FieldDef{}
	for _, d := range custom {
		byKey[d.Key] = d
	}
	for h, t := range req.Mapping {
		if strings.HasPrefix(t, "custom:") {
			if _, ok := byKey[strings.TrimPrefix(t, "custom:")]; !ok {
				return rep, invalid("column %q maps to unknown custom field %q", h, t)
			}
		} else if t != "" && !valid[t] {
			return rep, invalid("column %q maps to unknown field %q", h, t)
		}
	}
	set, err := r.GetSettings(ctx, org)
	if err != nil {
		return rep, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return rep, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	im := &importer{r: r, tx: tx, org: org, user: user, src: req.Source, set: set, defs: byKey, cache: map[string]string{}}
	var scored []string
	for i, rec := range rows {
		rep.Total++
		row := buildRow(head, rec, req.Mapping)
		sp, err := tx.Begin(ctx)
		if err != nil {
			return rep, err
		}
		im.tx = sp
		var action, id string
		switch req.Entity {
		case "lead":
			action, id, err = im.lead(ctx, row)
		case "company":
			action, id, err = im.company(ctx, row)
		default:
			action, id, err = im.deal(ctx, row)
		}
		if err != nil {
			_ = sp.Rollback(ctx)
			msg := "could not save row"
			if errors.Is(err, ErrInvalid) {
				msg = strings.TrimPrefix(err.Error(), "invalid: ")
			}
			rep.Errors = append(rep.Errors, ImportError{Row: i + 1, Message: msg})
			im.tx = tx
			continue
		}
		if err := sp.Commit(ctx); err != nil {
			return rep, err
		}
		im.tx = tx
		switch action {
		case "created":
			rep.Created++
		case "updated":
			rep.Updated++
		default:
			rep.Skipped++
		}
		if req.Entity == "lead" && id != "" {
			scored = append(scored, id)
		}
	}
	if req.DryRun {
		_ = tx.Rollback(ctx)
	} else {
		if err := tx.Commit(ctx); err != nil {
			return rep, err
		}
		for _, id := range scored {
			_ = r.RescorePerson(ctx, org, id)
		}
	}
	if len(rep.Errors) > 500 {
		rep.Errors = rep.Errors[:500]
	}
	m, _ := json.Marshal(req.Mapping)
	b, _ := json.Marshal(rep)
	_ = r.pool.QueryRow(ctx, `INSERT INTO adrient.crm_import_jobs (org_id,user_id,source,entity,dry_run,mapping,report) VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id::text`,
		org, nz(user), req.Source, req.Entity, req.DryRun, m, b).Scan(&rep.JobID)
	return rep, nil
}

// ImportJob is a past run.
type ImportJob struct {
	ID        string          `json:"id"`
	Source    string          `json:"source"`
	Entity    string          `json:"entity"`
	DryRun    bool            `json:"dry_run"`
	Report    json.RawMessage `json:"report"`
	CreatedAt time.Time       `json:"created_at"`
}

func (r *Repository) ListImportJobs(ctx context.Context, org string) ([]ImportJob, error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text, source, entity, dry_run, report, created_at FROM adrient.crm_import_jobs WHERE org_id=$1 ORDER BY created_at DESC LIMIT 30`, org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ImportJob{}
	for rows.Next() {
		var j ImportJob
		if err := rows.Scan(&j.ID, &j.Source, &j.Entity, &j.DryRun, &j.Report, &j.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

type importer struct {
	r     *Repository
	tx    pgx.Tx
	org   string
	user  string
	src   string
	set   Settings
	defs  map[string]FieldDef
	cache map[string]string // owner email -> user id
}

func (im *importer) owner(ctx context.Context, email string) any {
	email = NormEmail(email)
	if email == "" {
		return nz(im.user)
	}
	if v, ok := im.cache[email]; ok {
		return nz(v)
	}
	var id string
	if err := im.tx.QueryRow(ctx, `SELECT id::text FROM adrient.users WHERE org_id=$1 AND lower(email)=$2`, im.org, email).Scan(&id); err != nil {
		id = im.user
	}
	im.cache[email] = id
	return nz(id)
}

// customFrom converts mapped custom columns to validated values.
func (im *importer) customFrom(row importRow) (map[string]any, error) {
	raw := map[string]any{}
	var defs []FieldDef
	for t, v := range row {
		if !strings.HasPrefix(t, "custom:") {
			continue
		}
		d := im.defs[strings.TrimPrefix(t, "custom:")]
		defs = append(defs, d)
		switch d.Type {
		case "multiselect":
			l := []any{}
			for _, p := range splitMulti(v) {
				l = append(l, p)
			}
			raw[d.Key] = l
		case "date":
			if s, ok := ParseDate(v); ok {
				raw[d.Key] = s
			} else {
				raw[d.Key] = v
			}
		default:
			raw[d.Key] = v
		}
	}
	if len(raw) == 0 {
		return map[string]any{}, nil
	}
	sort.Slice(defs, func(i, j int) bool { return defs[i].Key < defs[j].Key })
	return ValidateCustom(defs, raw, false, nil)
}

func (im *importer) extLookup(ctx context.Context, entity, ext string) string {
	if ext == "" {
		return ""
	}
	var id string
	_ = im.tx.QueryRow(ctx, `SELECT record_id::text FROM adrient.crm_external_ids WHERE org_id=$1 AND source=$2 AND entity=$3 AND external_id=$4`, im.org, im.src, entity, ext).Scan(&id)
	return id
}

func (im *importer) extSave(ctx context.Context, entity, ext, id string) error {
	if ext == "" {
		return nil
	}
	_, err := im.tx.Exec(ctx, `INSERT INTO adrient.crm_external_ids (org_id,source,entity,external_id,record_id) VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (org_id,source,entity,external_id) DO UPDATE SET record_id=EXCLUDED.record_id`, im.org, im.src, entity, ext, id)
	return err
}

func (im *importer) companyID(ctx context.Context, name string, create bool) (any, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, nil
	}
	var id string
	err := im.tx.QueryRow(ctx, `SELECT id::text FROM adrient.crm_companies WHERE org_id=$1 AND lower(name)=lower($2) ORDER BY created_at LIMIT 1`, im.org, name).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		if !create {
			return nil, nil
		}
		err = im.tx.QueryRow(ctx, `INSERT INTO adrient.crm_companies (org_id,name,owner_id,created_by) VALUES ($1,$2,$3,$3) RETURNING id::text`, im.org, name, nz(im.user)).Scan(&id)
	}
	return id, err
}

func (im *importer) company(ctx context.Context, row importRow) (string, string, error) {
	name := row["name"]
	if name == "" {
		return "", "", invalid("name is required")
	}
	custom, err := im.customFrom(row)
	if err != nil {
		return "", "", err
	}
	cj, _ := json.Marshal(custom)
	tags := splitMulti(row["tags"])
	ext := row["external_id"]
	id := im.extLookup(ctx, "company", ext)
	if id == "" {
		_ = im.tx.QueryRow(ctx, `SELECT id::text FROM adrient.crm_companies WHERE org_id=$1 AND lower(name)=lower($2) ORDER BY created_at LIMIT 1`, im.org, name).Scan(&id)
	}
	if id == "" {
		err = im.tx.QueryRow(ctx, `INSERT INTO adrient.crm_companies (org_id,name,industry,website,phone,address,notes,tags,owner_id,created_by,custom)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id::text`, im.org, name, row["industry"], row["website"], row["phone"], row["address"], row["notes"],
			nonNilStrs(tags), im.owner(ctx, row["owner_email"]), nz(im.user), cj).Scan(&id)
		if err != nil {
			return "", "", err
		}
		return "created", id, im.extSave(ctx, "company", ext, id)
	}
	_, err = im.tx.Exec(ctx, `UPDATE adrient.crm_companies SET name=$3, industry=coalesce(nullif($4,''),industry), website=coalesce(nullif($5,''),website),
		phone=coalesce(nullif($6,''),phone), address=coalesce(nullif($7,''),address), notes=coalesce(nullif($8,''),notes),
		tags=(SELECT coalesce(array_agg(DISTINCT t),'{}') FROM unnest(tags || $9::text[]) t), custom=custom || $10 WHERE org_id=$1 AND id=$2`,
		im.org, id, name, row["industry"], row["website"], row["phone"], row["address"], row["notes"], nonNilStrs(tags), cj)
	if err != nil {
		return "", "", err
	}
	return "updated", id, im.extSave(ctx, "company", ext, id)
}

func (im *importer) lead(ctx context.Context, row importRow) (string, string, error) {
	name := row["name"]
	if name == "" {
		name = strings.TrimSpace(row["first_name"] + " " + row["last_name"])
	}
	emails := []string{}
	for _, e := range strings.Split(row["email"], "|") {
		if e = NormEmail(e); e != "" {
			if !ValidEmail(e) {
				return "", "", invalid("%q is not a valid email address", e)
			}
			emails = append(emails, e)
		}
	}
	phones := []string{}
	for _, p := range strings.Split(row["phone"], "|") {
		if p = strings.TrimSpace(p); p != "" {
			phones = append(phones, p)
		}
	}
	if name == "" && len(emails) == 0 {
		return "", "", invalid("a name or an email address is required")
	}
	if name == "" {
		name = emails[0]
	}
	custom, err := im.customFrom(row)
	if err != nil {
		return "", "", err
	}
	cj, _ := json.Marshal(custom)
	company, err := im.companyID(ctx, row["company"], true)
	if err != nil {
		return "", "", err
	}
	status := MapStatus(row["status"])
	tags := nonNilStrs(splitMulti(row["tags"]))
	ext := row["external_id"]
	pid := im.extLookup(ctx, "lead", ext)
	var contact string
	if pid != "" {
		_ = im.tx.QueryRow(ctx, `SELECT contact_id::text FROM adrient.crm_people WHERE org_id=$1 AND id=$2`, im.org, pid).Scan(&contact)
		if contact == "" {
			pid = ""
		}
	}
	if pid == "" && len(emails) > 0 {
		_ = im.tx.QueryRow(ctx, `SELECT c.id::text FROM adrient.contacts c WHERE c.org_id=$1 AND c.trashed_at IS NULL AND
			EXISTS (SELECT 1 FROM jsonb_array_elements_text(c.emails) e WHERE lower(e)=$2) ORDER BY c.created_at LIMIT 1`, im.org, emails[0]).Scan(&contact)
		if contact != "" {
			_ = im.tx.QueryRow(ctx, `SELECT id::text FROM adrient.crm_people WHERE org_id=$1 AND contact_id=$2`, im.org, contact).Scan(&pid)
		}
	}
	ej, _ := json.Marshal(emails)
	pj, _ := json.Marshal(phones)
	if contact == "" {
		first, last := row["first_name"], row["last_name"]
		if first == "" && last == "" {
			first, last, _ = strings.Cut(name, " ")
		}
		if err := im.tx.QueryRow(ctx, `INSERT INTO adrient.contacts (org_id,owner_id,display_name,first_name,last_name,company,job_title,emails,phones)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id::text`, im.org, im.user, name, first, last, row["company"], row["job_title"], ej, pj).Scan(&contact); err != nil {
			return "", "", err
		}
	} else if _, err := im.tx.Exec(ctx, `UPDATE adrient.contacts SET display_name=$3, job_title=coalesce(nullif($4,''),job_title),
		emails=(SELECT coalesce(jsonb_agg(DISTINCT e),'[]') FROM jsonb_array_elements_text(emails || $5::jsonb) e),
		phones=(SELECT coalesce(jsonb_agg(DISTINCT e),'[]') FROM jsonb_array_elements_text(phones || $6::jsonb) e), updated_at=now()
		WHERE org_id=$1 AND id=$2`, im.org, contact, name, row["job_title"], ej, pj); err != nil {
		return "", "", err
	}
	if pid == "" {
		err = im.tx.QueryRow(ctx, `INSERT INTO adrient.crm_people (org_id,contact_id,company_id,status,source,tags,notes,owner_id,created_by,custom)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT (org_id,contact_id) DO UPDATE SET source=EXCLUDED.source RETURNING id::text`,
			im.org, contact, company, status, row["source"], tags, row["notes"], im.owner(ctx, row["owner_email"]), nz(im.user), cj).Scan(&pid)
		if err != nil {
			return "", "", err
		}
		return "created", pid, im.extSave(ctx, "lead", ext, pid)
	}
	_, err = im.tx.Exec(ctx, `UPDATE adrient.crm_people SET company_id=coalesce($3::uuid,company_id), status=$4, source=coalesce(nullif($5,''),source),
		notes=coalesce(nullif($6,''),notes), tags=(SELECT coalesce(array_agg(DISTINCT t),'{}') FROM unnest(tags || $7::text[]) t), custom=custom || $8
		WHERE org_id=$1 AND id=$2`, im.org, pid, company, status, row["source"], row["notes"], tags, cj)
	if err != nil {
		return "", "", err
	}
	return "updated", pid, im.extSave(ctx, "lead", ext, pid)
}

func (im *importer) deal(ctx context.Context, row importRow) (string, string, error) {
	title := row["title"]
	if title == "" {
		return "", "", invalid("deal title is required")
	}
	val, err := ParseMoney(row["value"])
	if err != nil {
		return "", "", invalid("%v", err)
	}
	var closeDate any
	if row["expected_close"] != "" {
		d, ok := ParseDate(row["expected_close"])
		if !ok {
			return "", "", invalid("%q is not a valid date", row["expected_close"])
		}
		closeDate = d
	}
	custom, err := im.customFrom(row)
	if err != nil {
		return "", "", err
	}
	cj, _ := json.Marshal(custom)
	company, err := im.companyID(ctx, row["company"], true)
	if err != nil {
		return "", "", err
	}
	var contact any
	if e := NormEmail(row["contact_email"]); e != "" {
		var cid string
		if im.tx.QueryRow(ctx, `SELECT c.id::text FROM adrient.contacts c WHERE c.org_id=$1 AND c.trashed_at IS NULL AND
			EXISTS (SELECT 1 FROM jsonb_array_elements_text(c.emails) x WHERE lower(x)=$2) LIMIT 1`, im.org, e).Scan(&cid) == nil {
			contact = cid
		}
	}
	stage := MapStage(row["stage"], im.set.Stages)
	if row["stage"] == "" {
		stage = im.set.Stages[0].ID
	}
	cur := strings.ToUpper(firstNonEmpty(row["currency"], im.set.Currency))
	ext := row["external_id"]
	id := im.extLookup(ctx, "deal", ext)
	if id == "" {
		_ = im.tx.QueryRow(ctx, `SELECT id::text FROM adrient.crm_deals WHERE org_id=$1 AND lower(title)=lower($2) AND company_id IS NOT DISTINCT FROM $3::uuid ORDER BY created_at LIMIT 1`, im.org, title, company).Scan(&id)
	}
	if id == "" {
		err = im.tx.QueryRow(ctx, `INSERT INTO adrient.crm_deals (org_id,title,company_id,contact_id,stage,value,currency,expected_close,source,notes,close_reason,closed_at,owner_id,created_by,custom)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,CASE WHEN $5 IN ('won','lost') THEN now() END,$12,$13,$14) RETURNING id::text`,
			im.org, title, company, contact, stage, val, cur, closeDate, row["source"], row["notes"], row["close_reason"], im.owner(ctx, row["owner_email"]), nz(im.user), cj).Scan(&id)
		if err != nil {
			return "", "", err
		}
		if _, err = im.tx.Exec(ctx, `INSERT INTO adrient.crm_stage_history (org_id,deal_id,stage) VALUES ($1,$2,$3)`, im.org, id, stage); err != nil {
			return "", "", err
		}
		return "created", id, im.extSave(ctx, "deal", ext, id)
	}
	_, err = im.tx.Exec(ctx, `UPDATE adrient.crm_deals SET title=$3, company_id=coalesce($4::uuid,company_id), contact_id=coalesce($5::uuid,contact_id),
		value=$6, currency=$7, expected_close=coalesce($8::date,expected_close), source=coalesce(nullif($9,''),source), notes=coalesce(nullif($10,''),notes),
		custom=custom || $11, updated_at=now() WHERE org_id=$1 AND id=$2`, im.org, id, title, company, contact, val, cur, closeDate, row["source"], row["notes"], cj)
	if err != nil {
		return "", "", err
	}
	var curStage string
	_ = im.tx.QueryRow(ctx, `SELECT stage FROM adrient.crm_deals WHERE id=$1`, id).Scan(&curStage)
	if curStage != stage {
		terminal := stage == StageWon || stage == StageLost
		if _, err = im.tx.Exec(ctx, `UPDATE adrient.crm_deals SET stage=$3, stage_entered_at=now(), closed_at=CASE WHEN $4 THEN now() ELSE NULL END WHERE org_id=$1 AND id=$2`, im.org, id, stage, terminal); err != nil {
			return "", "", err
		}
		if _, err = im.tx.Exec(ctx, `INSERT INTO adrient.crm_stage_history (org_id,deal_id,stage) VALUES ($1,$2,$3)`, im.org, id, stage); err != nil {
			return "", "", err
		}
	}
	return "updated", id, im.extSave(ctx, "deal", ext, id)
}
