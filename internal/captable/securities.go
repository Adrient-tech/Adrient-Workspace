package captable

import (
	"context"
	"fmt"
	"math/big"
	"strings"

	"github.com/Adrient-tech/Adrient-Workspace/internal/knowkit"
	"github.com/jackc/pgx/v5"
)

// SecurityRec is an equity record with its state on a date.
type SecurityRec struct {
	ID            string      `json:"id"`
	Label         string      `json:"label"`
	Kind          string      `json:"kind"`
	StakeholderID string      `json:"stakeholder_id"`
	Stakeholder   string      `json:"stakeholder"`
	ClassID       string      `json:"class_id,omitempty"`
	Class         string      `json:"class,omitempty"`
	PoolID        string      `json:"pool_id,omitempty"`
	RoundID       string      `json:"round_id,omitempty"`
	ParentID      string      `json:"parent_id,omitempty"`
	Quantity      int64       `json:"quantity"`
	Price         string      `json:"price"`
	IssueDate     string      `json:"issue_date"`
	VestMonths    int         `json:"vest_months"`
	CliffMonths   int         `json:"cliff_months"`
	VestFreq      string      `json:"vest_freq"`
	VestStart     string      `json:"vest_start"`
	Expires       string      `json:"expires"`
	Principal     int64       `json:"principal"`
	Cap           int64       `json:"cap"`
	DiscountBp    int         `json:"discount_bp"`
	PostMoney     bool        `json:"post_money"`
	InterestBp    int         `json:"interest_bp"`
	Notes         string      `json:"notes"`
	Outstanding   int64       `json:"outstanding"`
	Vested        int64       `json:"vested"`
	Exercised     int64       `json:"exercised"`
	Cancelled     int64       `json:"cancelled"`
	Transferred   int64       `json:"transferred"`
	Exercisable   int64       `json:"exercisable"`
	Converted     bool        `json:"converted"`
	Value         int64       `json:"value,omitempty"` // convertibles: principal plus interest today
	Events        []EventRec  `json:"events,omitempty"`
	Schedule      []VestPoint `json:"schedule,omitempty"`
}

// EventRec is one exercise, cancellation, transfer or conversion.
type EventRec struct {
	ID            string `json:"id"`
	Kind          string `json:"kind"`
	SecurityID    string `json:"security_id"`
	Label         string `json:"label,omitempty"`
	Date          string `json:"date"`
	Quantity      int64  `json:"quantity"`
	ToStakeholder string `json:"to_stakeholder,omitempty"`
	ToName        string `json:"to_name,omitempty"`
	NewSecurityID string `json:"new_security_id,omitempty"`
	Price         string `json:"price"`
	Note          string `json:"note"`
}

// VestPoint is a point on a vesting schedule.
type VestPoint struct {
	Date   string `json:"date"`
	Vested int64  `json:"vested"`
}

// state is everything the pure functions need.
type state struct {
	holders  []Stakeholder
	classes  map[string]string
	pools    []Pool
	secs     []Security
	events   []Event
	holderNm map[string]string
}

func (r *Repository) loadState(ctx context.Context, org string) (*state, error) {
	st := &state{classes: map[string]string{}, holderNm: map[string]string{}}
	rows, err := r.pool.Query(ctx, `SELECT id::text, name, kind FROM adrient.captable_stakeholders WHERE org_id=$1`, org)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var h Stakeholder
		if err := rows.Scan(&h.ID, &h.Name, &h.Kind); err != nil {
			rows.Close()
			return nil, err
		}
		st.holders = append(st.holders, h)
		st.holderNm[h.ID] = h.Name
	}
	rows.Close()
	crows, err := r.pool.Query(ctx, `SELECT id::text, name FROM adrient.captable_classes WHERE org_id=$1`, org)
	if err != nil {
		return nil, err
	}
	for crows.Next() {
		var id, n string
		if err := crows.Scan(&id, &n); err != nil {
			crows.Close()
			return nil, err
		}
		st.classes[id] = n
	}
	crows.Close()
	prows, err := r.pool.Query(ctx, `SELECT id::text, name, size, to_char(adopted,'YYYY-MM-DD') FROM adrient.captable_pools WHERE org_id=$1 ORDER BY adopted`, org)
	if err != nil {
		return nil, err
	}
	for prows.Next() {
		var p Pool
		if err := prows.Scan(&p.ID, &p.Name, &p.Size, &p.Adopted); err != nil {
			prows.Close()
			return nil, err
		}
		st.pools = append(st.pools, p)
	}
	prows.Close()
	recs, err := r.securities(ctx, org, "", "")
	if err != nil {
		return nil, err
	}
	for _, s := range recs {
		st.secs = append(st.secs, s.toSecurity())
	}
	erows, err := r.pool.Query(ctx, `SELECT kind, security_id::text, to_char(event_date,'YYYY-MM-DD'), quantity FROM adrient.captable_events WHERE org_id=$1`, org)
	if err != nil {
		return nil, err
	}
	defer erows.Close()
	for erows.Next() {
		var e Event
		if err := erows.Scan(&e.Kind, &e.SecurityID, &e.Date, &e.Quantity); err != nil {
			return nil, err
		}
		st.events = append(st.events, e)
	}
	return st, erows.Err()
}

func (s SecurityRec) toSecurity() Security {
	start := s.VestStart
	if start == "" {
		start = s.IssueDate
	}
	return Security{ID: s.ID, Label: s.Label, Kind: s.Kind, StakeholderID: s.StakeholderID, ClassID: s.ClassID, PoolID: s.PoolID, Quantity: s.Quantity,
		PriceC: priceCents(s.Price), IssueDate: s.IssueDate, Vesting: Vesting{Start: start, Months: s.VestMonths, CliffMons: s.CliffMonths, Frequency: s.VestFreq}, Expires: s.Expires,
		Conv: Convertible{ID: s.ID, Name: s.Label, Kind: s.Kind, PrincipalC: s.Principal, CapC: s.Cap, DiscountBp: s.DiscountBp, PostMoney: s.PostMoney, InterestBp: s.InterestBp, IssueDate: s.IssueDate}}
}

const secCols = `x.id::text, x.label, x.kind, x.stakeholder_id::text, h.name, COALESCE(x.class_id::text,''), COALESCE(c.name,''), COALESCE(x.pool_id::text,''), COALESCE(x.round_id::text,''),
	COALESCE(x.parent_id::text,''), x.quantity, x.price::text, to_char(x.issue_date,'YYYY-MM-DD'), x.vest_months, x.cliff_months, x.vest_freq, COALESCE(to_char(x.vest_start,'YYYY-MM-DD'),''),
	COALESCE(to_char(x.expires,'YYYY-MM-DD'),''), x.principal, x.cap, x.discount_bp, x.post_money, x.interest_bp, x.notes`
const secFrom = ` FROM adrient.captable_securities x JOIN adrient.captable_stakeholders h ON h.id=x.stakeholder_id LEFT JOIN adrient.captable_classes c ON c.id=x.class_id `

func scanSec(row pgx.Row) (SecurityRec, error) {
	var s SecurityRec
	err := row.Scan(&s.ID, &s.Label, &s.Kind, &s.StakeholderID, &s.Stakeholder, &s.ClassID, &s.Class, &s.PoolID, &s.RoundID, &s.ParentID, &s.Quantity, &s.Price, &s.IssueDate,
		&s.VestMonths, &s.CliffMonths, &s.VestFreq, &s.VestStart, &s.Expires, &s.Principal, &s.Cap, &s.DiscountBp, &s.PostMoney, &s.InterestBp, &s.Notes)
	s.Price = trimDecimal(s.Price)
	return s, notFound(err)
}

func (r *Repository) securities(ctx context.Context, org, holder, kind string) ([]SecurityRec, error) {
	if holder != "" && !knowkit.IsUUID(holder) {
		return nil, knowkit.ErrNotFound
	}
	rows, err := r.pool.Query(ctx, `SELECT `+secCols+secFrom+`WHERE x.org_id=$1 AND ($2='' OR x.stakeholder_id::text=$2) AND ($3='' OR x.kind=$3) ORDER BY x.issue_date, x.created_at`, org, holder, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SecurityRec{}
	for rows.Next() {
		s, err := scanSec(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// Securities lists equity records with their position on asOf (empty = today).
func (r *Repository) Securities(ctx context.Context, org, holder, kind, asOf string) ([]SecurityRec, error) {
	if asOf == "" {
		asOf = r.today()
	}
	if !validDate(asOf) {
		return nil, knowkit.Invalid("as_of must be YYYY-MM-DD")
	}
	recs, err := r.securities(ctx, org, holder, kind)
	if err != nil {
		return nil, err
	}
	st, err := r.loadState(ctx, org)
	if err != nil {
		return nil, err
	}
	pos := map[string]Position{}
	for _, p := range Positions(st.secs, st.events, asOf) {
		pos[p.ID] = p
	}
	for i := range recs {
		fillPosition(&recs[i], pos[recs[i].ID], asOf)
	}
	return recs, nil
}

func fillPosition(s *SecurityRec, p Position, asOf Date) {
	s.Outstanding, s.Vested, s.Exercised, s.Cancelled, s.Transferred, s.Exercisable, s.Converted = p.Outstanding, p.Vested, p.Exercised, p.Cancelled, p.Transferred, p.Exercisable, p.Converted
	if s.Kind == "safe" || s.Kind == "note" {
		s.Value = p.Conv.Value(asOf)
	}
}

// Security returns one record with its events and vesting schedule.
func (r *Repository) Security(ctx context.Context, org, id string) (SecurityRec, error) {
	if !knowkit.IsUUID(id) {
		return SecurityRec{}, knowkit.ErrNotFound
	}
	s, err := scanSec(r.pool.QueryRow(ctx, `SELECT `+secCols+secFrom+`WHERE x.org_id=$1 AND x.id=$2`, org, id))
	if err != nil {
		return s, err
	}
	st, err := r.loadState(ctx, org)
	if err != nil {
		return s, err
	}
	today := r.today()
	for _, p := range Positions(st.secs, st.events, today) {
		if p.ID == id {
			fillPosition(&s, p, today)
		}
	}
	rows, err := r.pool.Query(ctx, `SELECT e.id::text, e.kind, e.security_id::text, to_char(e.event_date,'YYYY-MM-DD'), e.quantity, COALESCE(e.to_stakeholder::text,''), COALESCE(h.name,''),
		COALESCE(e.new_security_id::text,''), e.price::text, e.note FROM adrient.captable_events e LEFT JOIN adrient.captable_stakeholders h ON h.id=e.to_stakeholder
		WHERE e.org_id=$1 AND e.security_id=$2 ORDER BY e.event_date, e.created_at`, org, id)
	if err != nil {
		return s, err
	}
	defer rows.Close()
	s.Events = []EventRec{}
	for rows.Next() {
		var e EventRec
		if err := rows.Scan(&e.ID, &e.Kind, &e.SecurityID, &e.Date, &e.Quantity, &e.ToStakeholder, &e.ToName, &e.NewSecurityID, &e.Price, &e.Note); err != nil {
			return s, err
		}
		e.Price = trimDecimal(e.Price)
		s.Events = append(s.Events, e)
	}
	s.Schedule = Schedule(s.toSecurity().Vesting, s.Quantity)
	return s, rows.Err()
}

// Schedule lists every date the vested count changes, so a chart can draw the curve exactly.
func Schedule(v Vesting, quantity int64) []VestPoint {
	if v.Months <= 0 || quantity <= 0 || !validDate(v.Start) {
		return nil
	}
	out := []VestPoint{{Date: v.Start, Vested: 0}}
	step := v.step()
	for m := step; ; m += step {
		if m > v.Months {
			m = v.Months
		}
		d := addMonths(v.Start, m)
		out = append(out, VestPoint{Date: d, Vested: v.Vested(quantity, d)})
		if m >= v.Months {
			break
		}
	}
	return out
}

// SecurityInput issues equity.
type SecurityInput struct {
	Kind          string `json:"kind"`
	StakeholderID string `json:"stakeholder_id"`
	ClassID       string `json:"class_id"`
	PoolID        string `json:"pool_id"`
	RoundID       string `json:"round_id"`
	Quantity      int64  `json:"quantity"`
	Price         string `json:"price"`
	IssueDate     string `json:"issue_date"`
	VestMonths    int    `json:"vest_months"`
	CliffMonths   int    `json:"cliff_months"`
	VestFreq      string `json:"vest_freq"`
	VestStart     string `json:"vest_start"`
	Expires       string `json:"expires"`
	Principal     int64  `json:"principal"`
	Cap           int64  `json:"cap"`
	DiscountBp    int    `json:"discount_bp"`
	PostMoney     bool   `json:"post_money"`
	InterestBp    int    `json:"interest_bp"`
	Notes         string `json:"notes"`
	Label         string `json:"label"`
}

func (in *SecurityInput) clean() error {
	if !knowkit.OneOf(in.Kind, "stock", "option", "warrant", "safe", "note") {
		return knowkit.Invalid("kind must be stock, option, warrant, safe or note")
	}
	if !knowkit.IsUUID(in.StakeholderID) {
		return knowkit.Invalid("choose a holder")
	}
	for _, id := range []string{in.ClassID, in.PoolID, in.RoundID} {
		if id != "" && !knowkit.IsUUID(id) {
			return knowkit.Invalid("bad id")
		}
	}
	if !validDate(in.IssueDate) || (in.VestStart != "" && !validDate(in.VestStart)) || (in.Expires != "" && !validDate(in.Expires)) {
		return knowkit.Invalid("dates must be YYYY-MM-DD")
	}
	p, err := cleanDecimal(in.Price)
	if err != nil {
		return err
	}
	in.Price = p
	if in.VestFreq == "" {
		in.VestFreq = "monthly"
	}
	if !knowkit.OneOf(in.VestFreq, "monthly", "quarterly", "annual") || in.VestMonths < 0 || in.VestMonths > 240 || in.CliffMonths < 0 || in.CliffMonths > in.VestMonths && in.VestMonths > 0 {
		return knowkit.Invalid("the vesting schedule is not valid (up to 240 months, the cliff within the schedule)")
	}
	if tooLong(in.Notes, 2000) || tooLong(in.Label, 40) {
		return knowkit.Invalid("a field is too long")
	}
	switch in.Kind {
	case "stock", "option", "warrant":
		if in.Quantity <= 0 || in.Quantity > 1e13 {
			return knowkit.Invalid("the share count must be a whole number above zero")
		}
		in.Principal, in.Cap, in.DiscountBp, in.InterestBp = 0, 0, 0, 0
	default:
		if in.Principal <= 0 || in.Principal > 1e15 || in.Cap < 0 || in.DiscountBp < 0 || in.DiscountBp >= 10000 || in.InterestBp < 0 || in.InterestBp > 10000 {
			return knowkit.Invalid("a SAFE or note needs a principal above zero; the discount must be under 100 percent")
		}
		if in.Kind == "safe" {
			in.InterestBp = 0
		}
		in.Quantity, in.Price, in.VestMonths, in.CliffMonths = 0, "0", 0, 0
	}
	return nil
}

var labelPrefix = map[string]string{"stock": "CS", "option": "OPT", "warrant": "WAR", "safe": "SAFE", "note": "NOTE"}

func (r *Repository) nextLabel(ctx context.Context, tx pgx.Tx, org, kind, className string) (string, error) {
	prefix := labelPrefix[kind]
	if kind == "stock" && className != "" {
		prefix = "CS"
		if strings.Contains(strings.ToLower(className), "series") || strings.Contains(strings.ToLower(className), "preferred") {
			prefix = "PS"
		}
	}
	for n := 1; ; n++ {
		l := fmt.Sprintf("%s-%d", prefix, n)
		var taken bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM adrient.captable_securities WHERE org_id=$1 AND label=$2)`, org, l).Scan(&taken); err != nil {
			return "", err
		}
		if !taken {
			return l, nil
		}
	}
}

func (r *Repository) check(ctx context.Context, org string, in SecurityInput, st *state, selfQty int64) (className string, err error) {
	if _, err := r.stakeholder(ctx, org, in.StakeholderID); err != nil {
		return "", knowkit.Invalid("that holder does not exist")
	}
	if in.Kind == "stock" || in.Kind == "warrant" || (in.Kind == "option" && in.ClassID != "") {
		if in.ClassID == "" {
			return "", knowkit.Invalid("choose a share class")
		}
		c, err := r.class(ctx, org, in.ClassID)
		if err != nil {
			return "", knowkit.Invalid("that share class does not exist")
		}
		className = c.Name
		if in.Kind == "stock" && c.Authorized > 0 && c.Issued-selfQty+in.Quantity > c.Authorized {
			return "", knowkit.Invalid("class %s allows %d shares and %d are issued; this would exceed it", c.Name, c.Authorized, c.Issued-selfQty)
		}
	}
	if in.RoundID != "" {
		if _, err := r.round(ctx, org, in.RoundID); err != nil {
			return "", knowkit.Invalid("that round does not exist")
		}
	}
	if in.Kind == "option" && in.PoolID != "" {
		var found bool
		for _, b := range PoolBalances(st.pools, Positions(st.secs, st.events, in.IssueDate), in.IssueDate) {
			if b.ID == in.PoolID {
				found = true
				if in.Quantity > b.Unallocated {
					return "", knowkit.Invalid("the pool %s has %d shares left on that date; this grant needs %d", b.Name, b.Unallocated, in.Quantity)
				}
			}
		}
		if !found {
			return "", knowkit.Invalid("that pool does not exist yet on the grant date")
		}
	}
	return className, nil
}

// Issue records a new security: shares, an option grant, a warrant, a SAFE or a note.
func (r *Repository) Issue(ctx context.Context, a knowkit.Actor, in SecurityInput) (SecurityRec, error) {
	if err := in.clean(); err != nil {
		return SecurityRec{}, err
	}
	st, err := r.loadState(ctx, a.Org)
	if err != nil {
		return SecurityRec{}, err
	}
	className, err := r.check(ctx, a.Org, in, st, 0)
	if err != nil {
		return SecurityRec{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return SecurityRec{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	id, err := r.insertSecurity(ctx, tx, a, in, className, "")
	if err != nil {
		return SecurityRec{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return SecurityRec{}, err
	}
	r.audit(ctx, "security_issue", "captable_security", id)
	return r.Security(ctx, a.Org, id)
}

func (r *Repository) insertSecurity(ctx context.Context, tx pgx.Tx, a knowkit.Actor, in SecurityInput, className, parent string) (string, error) {
	label := strings.TrimSpace(in.Label)
	if label == "" {
		var err error
		if label, err = r.nextLabel(ctx, tx, a.Org, in.Kind, className); err != nil {
			return "", err
		}
	}
	var id string
	err := tx.QueryRow(ctx, `INSERT INTO adrient.captable_securities (org_id, label, kind, stakeholder_id, class_id, pool_id, round_id, parent_id, quantity, price, issue_date,
		vest_months, cliff_months, vest_freq, vest_start, expires, principal, cap, discount_bp, post_money, interest_bp, notes, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::numeric,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23) RETURNING id::text`,
		a.Org, label, in.Kind, in.StakeholderID, nullUUID(in.ClassID), nullUUID(in.PoolID), nullUUID(in.RoundID), nullUUID(parent), in.Quantity, in.Price, in.IssueDate,
		in.VestMonths, in.CliffMonths, in.VestFreq, nullDate(in.VestStart), nullDate(in.Expires), in.Principal, in.Cap, in.DiscountBp, in.PostMoney, in.InterestBp, in.Notes, a.User).Scan(&id)
	if err != nil && strings.Contains(err.Error(), "23505") {
		return "", knowkit.Invalid("the label %q is already used", label)
	}
	return id, err
}

func nullDate(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// UpdateSecurity edits a security that has no events and no successors (otherwise record a correction as a new event).
func (r *Repository) UpdateSecurity(ctx context.Context, a knowkit.Actor, id string, in SecurityInput) (SecurityRec, error) {
	if err := in.clean(); err != nil {
		return SecurityRec{}, err
	}
	cur, err := r.Security(ctx, a.Org, id)
	if err != nil {
		return SecurityRec{}, err
	}
	if len(cur.Events) > 0 || cur.ParentID != "" {
		return SecurityRec{}, knowkit.Invalid("this record has exercises, transfers or conversions on it; undo them to edit it")
	}
	in.Kind = cur.Kind
	st, err := r.loadState(ctx, a.Org)
	if err != nil {
		return SecurityRec{}, err
	}
	// Check the pool and class as if this record did not exist.
	rest := st.secs[:0:0]
	for _, s := range st.secs {
		if s.ID != id {
			rest = append(rest, s)
		}
	}
	st.secs = rest
	if _, err := r.check(ctx, a.Org, in, st, cur.Quantity); err != nil {
		return SecurityRec{}, err
	}
	label := strings.TrimSpace(in.Label)
	if label == "" {
		label = cur.Label
	}
	if _, err := r.pool.Exec(ctx, `UPDATE adrient.captable_securities SET label=$3, stakeholder_id=$4, class_id=$5, pool_id=$6, round_id=$7, quantity=$8, price=$9::numeric, issue_date=$10,
		vest_months=$11, cliff_months=$12, vest_freq=$13, vest_start=$14, expires=$15, principal=$16, cap=$17, discount_bp=$18, post_money=$19, interest_bp=$20, notes=$21 WHERE org_id=$1 AND id=$2`,
		a.Org, id, label, in.StakeholderID, nullUUID(in.ClassID), nullUUID(in.PoolID), nullUUID(in.RoundID), in.Quantity, in.Price, in.IssueDate, in.VestMonths, in.CliffMonths, in.VestFreq,
		nullDate(in.VestStart), nullDate(in.Expires), in.Principal, in.Cap, in.DiscountBp, in.PostMoney, in.InterestBp, in.Notes); err != nil {
		if strings.Contains(err.Error(), "23505") {
			return SecurityRec{}, knowkit.Invalid("the label %q is already used", label)
		}
		return SecurityRec{}, err
	}
	r.audit(ctx, "security_update", "captable_security", id)
	return r.Security(ctx, a.Org, id)
}

// DeleteSecurity removes a security that has no events and no successors.
func (r *Repository) DeleteSecurity(ctx context.Context, a knowkit.Actor, id string) error {
	cur, err := r.Security(ctx, a.Org, id)
	if err != nil {
		return err
	}
	var kids int
	_ = r.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.captable_securities WHERE parent_id=$1`, id).Scan(&kids)
	if len(cur.Events) > 0 || kids > 0 || cur.ParentID != "" {
		return knowkit.Invalid("this record is part of an exercise, transfer or conversion; undo that first")
	}
	_, err = r.pool.Exec(ctx, `DELETE FROM adrient.captable_securities WHERE org_id=$1 AND id=$2`, a.Org, id)
	r.audit(ctx, "security_delete", "captable_security", id)
	return err
}

// EventInput records an exercise, cancellation, transfer or conversion.
type EventInput struct {
	Date          string `json:"date"`
	Quantity      int64  `json:"quantity"`
	ToStakeholder string `json:"to_stakeholder"`
	Price         string `json:"price"`
	ClassID       string `json:"class_id"`
	Note          string `json:"note"`
}

func (r *Repository) positionOn(ctx context.Context, org, id, date string) (SecurityRec, Position, *state, error) {
	s, err := r.Security(ctx, org, id)
	if err != nil {
		return s, Position{}, nil, err
	}
	st, err := r.loadState(ctx, org)
	if err != nil {
		return s, Position{}, nil, err
	}
	for _, p := range Positions(st.secs, st.events, date) {
		if p.ID == id {
			return s, p, st, nil
		}
	}
	return s, Position{}, st, knowkit.ErrNotFound
}

// Record applies an event (kind is exercise, cancel, transfer or convert) to a security. Every write is checked
// against the records it touches: an exercise cannot outrun vesting, a transfer cannot exceed what is held, and
// nothing may predate the security.
func (r *Repository) Record(ctx context.Context, a knowkit.Actor, kind, id string, in EventInput) (SecurityRec, error) {
	if !validDate(in.Date) || tooLong(in.Note, 1000) {
		return SecurityRec{}, knowkit.Invalid("the date must be YYYY-MM-DD")
	}
	s, p, st, err := r.positionOn(ctx, a.Org, id, in.Date)
	if err != nil {
		return SecurityRec{}, err
	}
	if p.Pending || in.Date < s.IssueDate {
		return SecurityRec{}, knowkit.Invalid("this security was issued on %s; the event cannot come before that", s.IssueDate)
	}
	for _, e := range st.events {
		if e.SecurityID == id && e.Date > in.Date {
			return SecurityRec{}, knowkit.Invalid("a later event is already recorded on %s; record events in date order", e.Date)
		}
	}
	price := in.Price
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return SecurityRec{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var newID string
	toHolder := ""
	switch kind {
	case "exercise":
		if s.Kind != "option" && s.Kind != "warrant" {
			return SecurityRec{}, knowkit.Invalid("only options and warrants can be exercised")
		}
		if in.Quantity <= 0 || in.Quantity > p.Exercisable {
			return SecurityRec{}, knowkit.Invalid("%d are vested and not yet exercised on %s; you asked for %d", p.Exercisable, in.Date, in.Quantity)
		}
		if s.Expires != "" && in.Date > s.Expires {
			return SecurityRec{}, knowkit.Invalid("this grant expired on %s", s.Expires)
		}
		classID := s.ClassID
		if classID == "" {
			for id, n := range st.classes {
				if strings.EqualFold(n, "common") || strings.HasPrefix(strings.ToLower(n), "common") {
					classID = id
				}
			}
		}
		if classID == "" {
			return SecurityRec{}, knowkit.Invalid("add a Common share class first; exercised options become common shares")
		}
		price = s.Price
		n, err := r.insertSecurity(ctx, tx, a, SecurityInput{Kind: "stock", StakeholderID: s.StakeholderID, ClassID: classID, Quantity: in.Quantity, Price: s.Price, IssueDate: in.Date, VestFreq: "monthly"}, st.classes[classID], id)
		if err != nil {
			return SecurityRec{}, err
		}
		newID = n
	case "cancel":
		if s.Kind != "option" && s.Kind != "warrant" {
			return SecurityRec{}, knowkit.Invalid("only options and warrants can be cancelled")
		}
		if in.Quantity <= 0 || in.Quantity > p.Outstanding {
			return SecurityRec{}, knowkit.Invalid("%d are outstanding; you asked to cancel %d", p.Outstanding, in.Quantity)
		}
	case "transfer":
		if s.Kind != "stock" {
			return SecurityRec{}, knowkit.Invalid("only shares can be transferred")
		}
		if in.Quantity <= 0 || in.Quantity > p.Outstanding {
			return SecurityRec{}, knowkit.Invalid("%d shares are held; you asked to transfer %d", p.Outstanding, in.Quantity)
		}
		if in.ToStakeholder == s.StakeholderID || !knowkit.IsUUID(in.ToStakeholder) {
			return SecurityRec{}, knowkit.Invalid("choose a different holder to receive the shares")
		}
		if _, err := r.stakeholder(ctx, a.Org, in.ToStakeholder); err != nil {
			return SecurityRec{}, knowkit.Invalid("the receiving holder does not exist")
		}
		toHolder = in.ToStakeholder
		pr, err := cleanDecimal(in.Price)
		if err != nil {
			return SecurityRec{}, err
		}
		price = pr
		// The shares move to a new record for the receiver; vesting carries over on the same schedule.
		n, err := r.insertSecurity(ctx, tx, a, SecurityInput{Kind: "stock", StakeholderID: in.ToStakeholder, ClassID: s.ClassID, Quantity: in.Quantity, Price: s.Price, IssueDate: in.Date,
			VestMonths: s.VestMonths, CliffMonths: s.CliffMonths, VestFreq: s.VestFreq, VestStart: nonEmptyStr(s.VestStart, s.IssueDate), RoundID: s.RoundID}, s.Class, id)
		if err != nil {
			return SecurityRec{}, err
		}
		newID = n
	case "convert":
		if s.Kind != "safe" && s.Kind != "note" {
			return SecurityRec{}, knowkit.Invalid("only SAFEs and notes convert")
		}
		if p.Converted {
			return SecurityRec{}, knowkit.Invalid("this instrument has already converted")
		}
		if in.Quantity <= 0 || !knowkit.IsUUID(in.ClassID) {
			return SecurityRec{}, knowkit.Invalid("give the number of shares it converts into and their class")
		}
		c, err := r.class(ctx, a.Org, in.ClassID)
		if err != nil {
			return SecurityRec{}, knowkit.Invalid("that share class does not exist")
		}
		if c.Authorized > 0 && c.Issued+in.Quantity > c.Authorized {
			return SecurityRec{}, knowkit.Invalid("class %s would exceed its authorised shares", c.Name)
		}
		pr, err := cleanDecimal(in.Price)
		if err != nil {
			return SecurityRec{}, err
		}
		price = pr
		n, err := r.insertSecurity(ctx, tx, a, SecurityInput{Kind: "stock", StakeholderID: s.StakeholderID, ClassID: in.ClassID, Quantity: in.Quantity, Price: pr, IssueDate: in.Date, VestFreq: "monthly"}, c.Name, id)
		if err != nil {
			return SecurityRec{}, err
		}
		newID = n
	default:
		return SecurityRec{}, knowkit.Invalid("unknown event")
	}
	pr, err := cleanDecimal(price)
	if err != nil {
		return SecurityRec{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO adrient.captable_events (org_id, kind, security_id, event_date, quantity, to_stakeholder, new_security_id, price, note, by_user)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8::numeric,$9,$10)`, a.Org, kind, id, in.Date, in.Quantity, nullUUID(toHolder), nullUUID(newID), pr, in.Note, a.User); err != nil {
		return SecurityRec{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return SecurityRec{}, err
	}
	r.audit(ctx, "security_"+kind, "captable_security", id)
	return r.Security(ctx, a.Org, id)
}

func nonEmptyStr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// UndoEvent removes an event and the record it created, when nothing happened to that record since.
func (r *Repository) UndoEvent(ctx context.Context, a knowkit.Actor, eventID string) error {
	if !knowkit.IsUUID(eventID) {
		return knowkit.ErrNotFound
	}
	var sec string
	var newID *string
	if err := r.pool.QueryRow(ctx, `SELECT security_id::text, new_security_id::text FROM adrient.captable_events WHERE org_id=$1 AND id=$2`, a.Org, eventID).Scan(&sec, &newID); err != nil {
		return notFound(err)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if newID != nil {
		var n int
		_ = tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM adrient.captable_events WHERE security_id=$1) + (SELECT count(*) FROM adrient.captable_securities WHERE parent_id=$1)`, *newID).Scan(&n)
		if n > 0 {
			return knowkit.Invalid("the record this created has had events of its own; undo those first")
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM adrient.captable_events WHERE id=$1`, eventID); err != nil {
		return err
	}
	if newID != nil {
		if _, err := tx.Exec(ctx, `DELETE FROM adrient.captable_securities WHERE id=$1`, *newID); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	r.audit(ctx, "event_undo", "captable_security", sec)
	return nil
}

// Events lists every event across the company, newest first.
func (r *Repository) Events(ctx context.Context, org string) ([]EventRec, error) {
	rows, err := r.pool.Query(ctx, `SELECT e.id::text, e.kind, e.security_id::text, x.label, to_char(e.event_date,'YYYY-MM-DD'), e.quantity, COALESCE(e.to_stakeholder::text,''), COALESCE(h.name,''),
		COALESCE(e.new_security_id::text,''), e.price::text, e.note FROM adrient.captable_events e JOIN adrient.captable_securities x ON x.id=e.security_id
		LEFT JOIN adrient.captable_stakeholders h ON h.id=e.to_stakeholder WHERE e.org_id=$1 ORDER BY e.event_date DESC, e.created_at DESC LIMIT 1000`, org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []EventRec{}
	for rows.Next() {
		var e EventRec
		if err := rows.Scan(&e.ID, &e.Kind, &e.SecurityID, &e.Label, &e.Date, &e.Quantity, &e.ToStakeholder, &e.ToName, &e.NewSecurityID, &e.Price, &e.Note); err != nil {
			return nil, err
		}
		e.Price = trimDecimal(e.Price)
		out = append(out, e)
	}
	return out, rows.Err()
}

var _ = big.NewRat
