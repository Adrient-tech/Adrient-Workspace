package legal

import (
	"context"
	"strings"
	"time"

	"github.com/Adrient-tech/Adrient-Workspace/internal/knowkit"
	"github.com/jackc/pgx/v5"
)

// Matter is a legal matter with its money and clock summary.
type Matter struct {
	ID            string    `json:"id"`
	Seq           int       `json:"seq"`
	Number        string    `json:"number"`
	Title         string    `json:"title"`
	Description   string    `json:"description"`
	ClientID      string    `json:"client_id"`
	ClientName    string    `json:"client_name"`
	PracticeArea  string    `json:"practice_area"`
	Stage         string    `json:"stage"`
	Status        string    `json:"status"`
	NextStatuses  []string  `json:"next_statuses"`
	ResponsibleID string    `json:"responsible_id"`
	Responsible   string    `json:"responsible"`
	OriginatingID string    `json:"originating_id"`
	OpenDate      string    `json:"open_date"`
	PendingDate   string    `json:"pending_date"`
	ClosedDate    string    `json:"closed_date"`
	SolDate       string    `json:"sol_date"`
	SolDays       *int      `json:"sol_days,omitempty"`
	BillingMethod string    `json:"billing_method"`
	Rate          int64     `json:"rate"`
	FlatFee       int64     `json:"flat_fee"`
	FlatFeeBilled bool      `json:"flat_fee_billed"`
	ContingencyBp int       `json:"contingency_bp"`
	Budget        int64     `json:"budget"`
	Tags          []string  `json:"tags"`
	Access        string    `json:"access"`
	SharedWith    []string  `json:"shared_with"`
	ConflictAt    string    `json:"conflict_checked_at"`
	ConflictWaiv  string    `json:"conflict_waiver"`
	CreatedBy     string    `json:"created_by"`
	UpdatedAt     time.Time `json:"updated_at"`
	// Money and time, all in minor units and minutes.
	Minutes         int   `json:"minutes"`
	UnbilledMinutes int   `json:"unbilled_minutes"`
	Unbilled        int64 `json:"unbilled"`
	Billed          int64 `json:"billed"`
	CanEdit         bool  `json:"can_edit"`
	// Detail only.
	Parties   []Party     `json:"parties,omitempty"`
	Deadlines []Deadline  `json:"deadlines,omitempty"`
	Documents []Document  `json:"documents,omitempty"`
	Log       []StatusLog `json:"status_log,omitempty"`
}

// Party is someone tied to a matter: opposing party, counsel, judge, witness.
type Party struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Role      string `json:"role"`
	ContactID string `json:"contact_id,omitempty"`
	Email     string `json:"email"`
	Phone     string `json:"phone"`
}

// StatusLog is one move through the status workflow.
type StatusLog struct {
	From string    `json:"from"`
	To   string    `json:"to"`
	Note string    `json:"note"`
	By   string    `json:"by"`
	At   time.Time `json:"at"`
}

// MatterInput creates or edits a matter. Status is changed with Move, not here.
type MatterInput struct {
	Title         string   `json:"title"`
	Description   string   `json:"description"`
	ClientID      string   `json:"client_id"`
	PracticeArea  string   `json:"practice_area"`
	Stage         string   `json:"stage"`
	ResponsibleID string   `json:"responsible_id"`
	OriginatingID string   `json:"originating_id"`
	SolDate       string   `json:"sol_date"`
	BillingMethod string   `json:"billing_method"`
	Rate          int64    `json:"rate"`
	FlatFee       int64    `json:"flat_fee"`
	ContingencyBp int      `json:"contingency_bp"`
	Budget        int64    `json:"budget"`
	Tags          []string `json:"tags"`
	Access        string   `json:"access"`
	SharedWith    []string `json:"shared_with"`
	Parties       []Party  `json:"parties"`
}

func (in *MatterInput) clean() error {
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" || tooLong(in.Title, 200) {
		return knowkit.Invalid("a matter needs a title of up to 200 characters")
	}
	if tooLong(in.Description, 5000) || tooLong(in.PracticeArea, 60) || tooLong(in.Stage, 60) {
		return knowkit.Invalid("a field is too long")
	}
	if in.BillingMethod == "" {
		in.BillingMethod = "hourly"
	}
	if !knowkit.OneOf(in.BillingMethod, "hourly", "flat", "contingency") {
		return knowkit.Invalid("billing must be hourly, flat or contingency")
	}
	if in.Rate < 0 || in.FlatFee < 0 || in.Budget < 0 || in.Rate > 1e12 || in.FlatFee > 1e13 || in.Budget > 1e13 {
		return knowkit.Invalid("money amounts are out of range")
	}
	if in.ContingencyBp < 0 || in.ContingencyBp > 10000 {
		return knowkit.Invalid("the contingency share must be between 0 and 100 percent")
	}
	if in.Access == "" {
		in.Access = "org"
	}
	if !knowkit.OneOf(in.Access, "org", "restricted") {
		return knowkit.Invalid("access must be org or restricted")
	}
	for _, id := range append(append([]string{}, in.SharedWith...), in.ResponsibleID, in.OriginatingID, in.ClientID) {
		if id != "" && !knowkit.IsUUID(id) {
			return knowkit.Invalid("bad id in the request")
		}
	}
	if in.SolDate != "" && !ValidDate(in.SolDate) {
		return knowkit.Invalid("the limitation date must be YYYY-MM-DD")
	}
	tags := []string{}
	for _, t := range in.Tags {
		if t = strings.TrimSpace(t); t != "" && !tooLong(t, 40) {
			tags = append(tags, t)
		}
	}
	in.Tags = tags
	if in.SharedWith == nil {
		in.SharedWith = []string{}
	}
	if len(in.Parties) > 100 {
		return knowkit.Invalid("too many related parties")
	}
	for i := range in.Parties {
		p := &in.Parties[i]
		p.Name = strings.TrimSpace(p.Name)
		if p.Name == "" || tooLong(p.Name, 200) || tooLong(p.Role, 60) || tooLong(p.Email, 200) || tooLong(p.Phone, 60) {
			return knowkit.Invalid("each related party needs a name and sensible field lengths")
		}
		if p.Role == "" {
			p.Role = "Related"
		}
		if p.ContactID != "" && !knowkit.IsUUID(p.ContactID) {
			return knowkit.Invalid("bad contact id")
		}
	}
	return nil
}

const matterCols = `m.id::text, m.seq, m.number, m.title, m.description, COALESCE(m.client_id::text,''), COALESCE(c.name,''), m.practice_area, m.stage, m.status,
	COALESCE(m.responsible_id::text,''), COALESCE(NULLIF(u.display_name,''),u.email,''), COALESCE(m.originating_id::text,''),
	COALESCE(to_char(m.open_date,'YYYY-MM-DD'),''), COALESCE(to_char(m.pending_date,'YYYY-MM-DD'),''), COALESCE(to_char(m.closed_date,'YYYY-MM-DD'),''), COALESCE(to_char(m.sol_date,'YYYY-MM-DD'),''),
	m.billing_method, m.rate, m.flat_fee, m.flat_fee_billed, m.contingency_bp, m.budget, m.tags, m.access, COALESCE(m.shared_with::text[],'{}'),
	COALESCE(to_char(m.conflict_checked_at,'YYYY-MM-DD'),''), m.conflict_waiver, COALESCE(m.created_by::text,''), m.updated_at,
	COALESCE((SELECT sum(minutes) FROM adrient.legal_entries e WHERE e.matter_id=m.id AND e.kind='time'),0),
	COALESCE((SELECT sum(minutes) FROM adrient.legal_entries e WHERE e.matter_id=m.id AND e.kind='time' AND e.billable AND e.bill_id IS NULL),0),
	COALESCE((SELECT sum(amount) FROM adrient.legal_entries e WHERE e.matter_id=m.id AND e.billable AND e.bill_id IS NULL),0),
	COALESCE((SELECT sum(total) FROM adrient.legal_bills b WHERE b.matter_id=m.id AND b.status<>'void'),0)`

const matterFrom = ` FROM adrient.legal_matters m LEFT JOIN adrient.legal_clients c ON c.id=m.client_id LEFT JOIN adrient.users u ON u.id=m.responsible_id `

func (r *Repository) scanMatter(row pgx.Row, a knowkit.Actor) (*Matter, error) {
	var m Matter
	err := row.Scan(&m.ID, &m.Seq, &m.Number, &m.Title, &m.Description, &m.ClientID, &m.ClientName, &m.PracticeArea, &m.Stage, &m.Status,
		&m.ResponsibleID, &m.Responsible, &m.OriginatingID, &m.OpenDate, &m.PendingDate, &m.ClosedDate, &m.SolDate,
		&m.BillingMethod, &m.Rate, &m.FlatFee, &m.FlatFeeBilled, &m.ContingencyBp, &m.Budget, &m.Tags, &m.Access, &m.SharedWith,
		&m.ConflictAt, &m.ConflictWaiv, &m.CreatedBy, &m.UpdatedAt, &m.Minutes, &m.UnbilledMinutes, &m.Unbilled, &m.Billed)
	if err != nil {
		return nil, notFound(err)
	}
	if m.Tags == nil {
		m.Tags = []string{}
	}
	m.NextStatuses = NextStatuses(m.Status)
	m.CanEdit = r.canView(a, m)
	if m.SolDate != "" {
		d := DaysBetween(r.today(), m.SolDate)
		m.SolDays = &d
	}
	return &m, nil
}

// canView reports whether the actor may see (and work on) the matter. Restricted matters (an ethical wall)
// are visible to admins, the people on the matter and those it is shared with.
func (r *Repository) canView(a knowkit.Actor, m Matter) bool {
	if a.Admin || m.Access != "restricted" {
		return true
	}
	if a.User == m.CreatedBy || a.User == m.ResponsibleID || a.User == m.OriginatingID {
		return true
	}
	for _, s := range m.SharedWith {
		if s == a.User {
			return true
		}
	}
	return false
}

func (r *Repository) loadMatter(ctx context.Context, a knowkit.Actor, id string) (*Matter, error) {
	if !knowkit.IsUUID(id) {
		return nil, knowkit.ErrNotFound
	}
	m, err := r.scanMatter(r.pool.QueryRow(ctx, `SELECT `+matterCols+matterFrom+`WHERE m.org_id=$1 AND m.id=$2 AND m.deleted_at IS NULL`, a.Org, id), a)
	if err != nil {
		return nil, err
	}
	if !m.CanEdit {
		return nil, knowkit.ErrNotFound
	}
	return m, nil
}

// MatterFilter narrows the list.
type MatterFilter struct {
	Q, Status, Client, Area, Mine string
}

// Matters lists the matters the actor may see, newest number first.
func (r *Repository) Matters(ctx context.Context, a knowkit.Actor, f MatterFilter) ([]*Matter, error) {
	mine := ""
	if f.Mine == "1" {
		mine = a.User
	}
	clientID := ""
	if knowkit.IsUUID(f.Client) {
		clientID = f.Client
	}
	rows, err := r.pool.Query(ctx, `SELECT `+matterCols+matterFrom+`WHERE m.org_id=$1 AND m.deleted_at IS NULL
		AND ($2='' OR m.status=$2) AND ($3='' OR m.client_id::text=$3) AND ($4='' OR m.practice_area=$4) AND ($5='' OR m.responsible_id::text=$5)
		AND ($6='' OR m.title ILIKE '%'||$6||'%' OR m.number ILIKE '%'||$6||'%' OR c.name ILIKE '%'||$6||'%')
		ORDER BY m.seq DESC LIMIT 2000`, a.Org, f.Status, clientID, f.Area, mine, f.Q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Matter{}
	for rows.Next() {
		m, err := r.scanMatter(rows, a)
		if err != nil {
			return nil, err
		}
		if m.CanEdit {
			out = append(out, m)
		}
	}
	return out, rows.Err()
}

// Matter returns one matter with parties, deadlines, documents and its status history.
func (r *Repository) Matter(ctx context.Context, a knowkit.Actor, id string) (*Matter, error) {
	m, err := r.loadMatter(ctx, a, id)
	if err != nil {
		return nil, err
	}
	if m.Parties, err = r.parties(ctx, a.Org, id); err != nil {
		return nil, err
	}
	if m.Deadlines, err = r.deadlines(ctx, a.Org, id); err != nil {
		return nil, err
	}
	if m.Documents, err = r.documents(ctx, a.Org, id); err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, `SELECT from_status, to_status, note, COALESCE(NULLIF(u.display_name,''),u.email,''), l.at FROM adrient.legal_status_log l
		LEFT JOIN adrient.users u ON u.id=l.by_user WHERE l.matter_id=$1 ORDER BY l.at`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m.Log = []StatusLog{}
	for rows.Next() {
		var l StatusLog
		if err := rows.Scan(&l.From, &l.To, &l.Note, &l.By, &l.At); err != nil {
			return nil, err
		}
		m.Log = append(m.Log, l)
	}
	return m, rows.Err()
}

func (r *Repository) parties(ctx context.Context, org, id string) ([]Party, error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text, name, role, COALESCE(contact_id::text,''), email, phone FROM adrient.legal_parties WHERE org_id=$1 AND matter_id=$2 ORDER BY role, lower(name)`, org, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Party{}
	for rows.Next() {
		var p Party
		if err := rows.Scan(&p.ID, &p.Name, &p.Role, &p.ContactID, &p.Email, &p.Phone); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *Repository) replaceParties(ctx context.Context, tx pgx.Tx, org, matter string, ps []Party) error {
	if _, err := tx.Exec(ctx, `DELETE FROM adrient.legal_parties WHERE matter_id=$1 AND org_id=$2`, matter, org); err != nil {
		return err
	}
	for _, p := range ps {
		cid, err := optUUID(p.ContactID)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO adrient.legal_parties (org_id, matter_id, name, role, contact_id, email, phone) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
			org, matter, p.Name, p.Role, cid, strings.TrimSpace(p.Email), strings.TrimSpace(p.Phone)); err != nil {
			return err
		}
	}
	return nil
}

// checkRefs makes sure the client and the people named belong to this org.
func (r *Repository) checkRefs(ctx context.Context, org string, in MatterInput) error {
	if in.ClientID != "" {
		var ok bool
		if err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM adrient.legal_clients WHERE org_id=$1 AND id=$2)`, org, in.ClientID).Scan(&ok); err != nil || !ok {
			return knowkit.Invalid("that client does not exist")
		}
	}
	ids := append([]string{}, in.SharedWith...)
	for _, id := range []string{in.ResponsibleID, in.OriginatingID} {
		if id != "" {
			ids = append(ids, id)
		}
	}
	for _, id := range ids {
		var ok bool
		if err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM adrient.users WHERE org_id=$1 AND id=$2)`, org, id).Scan(&ok); err != nil || !ok {
			return knowkit.Invalid("a person named on the matter is not in this organization")
		}
	}
	return nil
}

// CreateMatter opens a new matter in the intake status. The number comes from the firm's counter.
func (r *Repository) CreateMatter(ctx context.Context, a knowkit.Actor, in MatterInput) (*Matter, error) {
	if err := in.clean(); err != nil {
		return nil, err
	}
	if err := r.checkRefs(ctx, a.Org, in); err != nil {
		return nil, err
	}
	s, err := r.GetSettings(ctx, a.Org)
	if err != nil {
		return nil, err
	}
	var clientName string
	if in.ClientID != "" {
		_ = r.pool.QueryRow(ctx, `SELECT name FROM adrient.legal_clients WHERE id=$1`, in.ClientID).Scan(&clientName)
	}
	resp, err := optUUID(in.ResponsibleID)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		resp = a.User
	}
	orig, _ := optUUID(in.OriginatingID)
	sol, _ := optDate(in.SolDate)
	cl, _ := optUUID(in.ClientID)
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var seq int
	if err := tx.QueryRow(ctx, `UPDATE adrient.legal_settings SET next_matter_no = next_matter_no + 1 WHERE org_id=$1 RETURNING next_matter_no - 1`, a.Org).Scan(&seq); err != nil {
		return nil, err
	}
	number := DisplayNumber(seq, s.NumberFormat, clientName, "", r.now())
	var id string
	err = tx.QueryRow(ctx, `INSERT INTO adrient.legal_matters (org_id, seq, number, title, description, client_id, practice_area, stage, responsible_id, originating_id, sol_date,
		billing_method, rate, flat_fee, contingency_bp, budget, tags, access, shared_with, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20) RETURNING id::text`,
		a.Org, seq, number, in.Title, in.Description, cl, in.PracticeArea, in.Stage, resp, orig, sol, in.BillingMethod, in.Rate, in.FlatFee, in.ContingencyBp, in.Budget, in.Tags, in.Access, in.SharedWith, a.User).Scan(&id)
	if err != nil {
		return nil, err
	}
	if err := r.replaceParties(ctx, tx, a.Org, id, in.Parties); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO adrient.legal_status_log (org_id, matter_id, from_status, to_status, note, by_user) VALUES ($1,$2,'','intake','Matter created',$3)`, a.Org, id, a.User); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	r.audit(ctx, "matter_create", "legal_matter", id)
	r.syncSol(ctx, a, id)
	return r.Matter(ctx, a, id)
}

// UpdateMatter edits a matter's details (not its status).
func (r *Repository) UpdateMatter(ctx context.Context, a knowkit.Actor, id string, in MatterInput) (*Matter, error) {
	if err := in.clean(); err != nil {
		return nil, err
	}
	if _, err := r.loadMatter(ctx, a, id); err != nil {
		return nil, err
	}
	if err := r.checkRefs(ctx, a.Org, in); err != nil {
		return nil, err
	}
	resp, _ := optUUID(in.ResponsibleID)
	orig, _ := optUUID(in.OriginatingID)
	sol, _ := optDate(in.SolDate)
	cl, _ := optUUID(in.ClientID)
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx, `UPDATE adrient.legal_matters SET title=$3, description=$4, client_id=$5, practice_area=$6, stage=$7, responsible_id=$8, originating_id=$9, sol_date=$10,
		billing_method=$11, rate=$12, flat_fee=$13, contingency_bp=$14, budget=$15, tags=$16, access=$17, shared_with=$18, updated_at=now() WHERE org_id=$1 AND id=$2`,
		a.Org, id, in.Title, in.Description, cl, in.PracticeArea, in.Stage, resp, orig, sol, in.BillingMethod, in.Rate, in.FlatFee, in.ContingencyBp, in.Budget, in.Tags, in.Access, in.SharedWith); err != nil {
		return nil, err
	}
	if in.Parties != nil {
		if err := r.replaceParties(ctx, tx, a.Org, id, in.Parties); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	r.audit(ctx, "matter_update", "legal_matter", id)
	r.syncSol(ctx, a, id)
	return r.Matter(ctx, a, id)
}

// DeleteMatter removes a matter that has no issued bills (admins only).
func (r *Repository) DeleteMatter(ctx context.Context, a knowkit.Actor, id string) error {
	if !a.Admin {
		return knowkit.ErrForbidden
	}
	if _, err := r.loadMatter(ctx, a, id); err != nil {
		return err
	}
	var n int
	_ = r.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.legal_bills WHERE matter_id=$1 AND status='issued'`, id).Scan(&n)
	if n > 0 {
		return knowkit.Invalid("this matter has issued bills; close it instead")
	}
	r.dropReminders(ctx, a.Org, id)
	if _, err := r.pool.Exec(ctx, `UPDATE adrient.legal_matters SET deleted_at=now() WHERE org_id=$1 AND id=$2`, a.Org, id); err != nil {
		return err
	}
	r.audit(ctx, "matter_delete", "legal_matter", id)
	return nil
}

// MoveInput changes a matter's status.
type MoveInput struct {
	Status        string `json:"status"`
	Note          string `json:"note"`
	Force         bool   `json:"force"`          // close despite unbilled work
	WaiveConflict string `json:"waive_conflict"` // reason to open without a conflict check
}

// Move walks a matter through its status workflow. Opening needs a recorded conflict check or a written waiver;
// closing with unbilled work needs force.
func (r *Repository) Move(ctx context.Context, a knowkit.Actor, id string, in MoveInput) (*Matter, error) {
	m, err := r.loadMatter(ctx, a, id)
	if err != nil {
		return nil, err
	}
	if !CanMove(m.Status, in.Status) {
		return nil, knowkit.Invalid("a %s matter cannot move to %s", m.Status, in.Status)
	}
	if tooLong(in.Note, 1000) || tooLong(in.WaiveConflict, 1000) {
		return nil, knowkit.Invalid("the note is too long")
	}
	today := r.today()
	switch in.Status {
	case "open":
		if m.OpenDate == "" && m.ConflictAt == "" && m.ConflictWaiv == "" {
			reason := strings.TrimSpace(in.WaiveConflict)
			if reason == "" {
				return nil, knowkit.Invalid("run a conflict check before opening this matter, or give a reason for waiving it")
			}
			if _, err := r.pool.Exec(ctx, `UPDATE adrient.legal_matters SET conflict_waiver=$3 WHERE org_id=$1 AND id=$2`, a.Org, id, reason); err != nil {
				return nil, err
			}
		}
	case "closed":
		if m.UnbilledMinutes > 0 || m.Unbilled > 0 {
			if !in.Force {
				return nil, knowkit.Invalid("this matter has unbilled work; bill it or close with force")
			}
		}
	}
	q := `UPDATE adrient.legal_matters SET status=$3, updated_at=now()`
	switch in.Status {
	case "open":
		q += `, open_date=COALESCE(open_date,$4::date), closed_date=NULL`
	case "pending":
		q += `, pending_date=$4::date`
	case "closed":
		q += `, closed_date=$4::date`
	default:
		q += `, ` + "open_date=open_date" + ` `
	}
	q += ` WHERE org_id=$1 AND id=$2`
	args := []any{a.Org, id, in.Status}
	if in.Status == "open" || in.Status == "pending" || in.Status == "closed" {
		args = append(args, today)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx, q, args...); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO adrient.legal_status_log (org_id, matter_id, from_status, to_status, note, by_user) VALUES ($1,$2,$3,$4,$5,$6)`,
		a.Org, id, m.Status, in.Status, strings.TrimSpace(in.Note), a.User); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	if in.Status == "closed" {
		r.dropReminders(ctx, a.Org, id)
	} else if m.Status == "closed" {
		r.syncSol(ctx, a, id)
	}
	r.audit(ctx, "matter_status", "legal_matter", id)
	return r.Matter(ctx, a, id)
}
