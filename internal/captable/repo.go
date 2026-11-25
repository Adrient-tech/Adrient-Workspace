package captable

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Adrient-tech/Adrient-Workspace/internal/knowkit"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Deps are optional connections to other apps.
type Deps struct {
	Audit knowkit.Audit
	Now   func() time.Time
	// Contacts looks up Contacts entries by id for a stakeholder's name and email.
	ContactName func(ctx context.Context, org, id string) (name, email string, err error)
}

// Repository is the Postgres store. Every query is org-scoped.
type Repository struct {
	pool *pgxpool.Pool
	Deps Deps
}

// NewRepository constructs a Repository.
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (r *Repository) now() time.Time { return knowkit.Clock(r.Deps.Now) }
func (r *Repository) today() Date    { return r.now().Format(dayLayout) }
func (r *Repository) audit(ctx context.Context, action, typ, id string) {
	if r.Deps.Audit != nil {
		r.Deps.Audit(ctx, action, typ, id)
	}
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return knowkit.ErrNotFound
	}
	return err
}

func tooLong(s string, n int) bool { return utf8.RuneCountInString(s) > n }

func validDate(s string) bool { _, ok := parseDate(s); return ok }

// ---- access --------------------------------------------------------------------------------------------------

// Levels of access to the cap table.
const (
	LevelNone = ""
	LevelView = "view"
	LevelEdit = "edit"
)

// Level reports what the actor may do. Org admins edit; named people have the level they were given; nobody else
// sees anything, not even that the company has equity records.
func (r *Repository) Level(ctx context.Context, a knowkit.Actor) string {
	if a.Admin {
		return LevelEdit
	}
	var l string
	if err := r.pool.QueryRow(ctx, `SELECT level FROM adrient.captable_access WHERE org_id=$1 AND user_id=$2`, a.Org, a.User).Scan(&l); err != nil {
		return LevelNone
	}
	return l
}

// AccessEntry is a person with access.
type AccessEntry struct {
	UserID string `json:"user_id"`
	Name   string `json:"name"`
	Email  string `json:"email"`
	Level  string `json:"level"`
}

// AccessList lists the named people (admins manage it).
func (r *Repository) AccessList(ctx context.Context, a knowkit.Actor) ([]AccessEntry, error) {
	if !a.Admin {
		return nil, knowkit.ErrForbidden
	}
	rows, err := r.pool.Query(ctx, `SELECT x.user_id::text, COALESCE(NULLIF(u.display_name,''),u.email,''), COALESCE(u.email,''), x.level FROM adrient.captable_access x
		LEFT JOIN adrient.users u ON u.id=x.user_id WHERE x.org_id=$1 ORDER BY 2`, a.Org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AccessEntry{}
	for rows.Next() {
		var e AccessEntry
		if err := rows.Scan(&e.UserID, &e.Name, &e.Email, &e.Level); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// GrantAccess names a person who may view or edit the cap table.
func (r *Repository) GrantAccess(ctx context.Context, a knowkit.Actor, userID, level string) error {
	if !a.Admin {
		return knowkit.ErrForbidden
	}
	if !knowkit.IsUUID(userID) || !knowkit.OneOf(level, LevelView, LevelEdit) {
		return knowkit.Invalid("choose a person and a level (view or edit)")
	}
	var ok bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM adrient.users WHERE org_id=$1 AND id=$2)`, a.Org, userID).Scan(&ok); err != nil || !ok {
		return knowkit.Invalid("that person is not in this organization")
	}
	if _, err := r.pool.Exec(ctx, `INSERT INTO adrient.captable_access (org_id, user_id, level, created_by) VALUES ($1,$2,$3,$4)
		ON CONFLICT (org_id, user_id) DO UPDATE SET level=EXCLUDED.level`, a.Org, userID, level, a.User); err != nil {
		return err
	}
	r.audit(ctx, "access_grant", "captable_access", userID)
	return nil
}

// RevokeAccess removes a person's access.
func (r *Repository) RevokeAccess(ctx context.Context, a knowkit.Actor, userID string) error {
	if !a.Admin {
		return knowkit.ErrForbidden
	}
	if !knowkit.IsUUID(userID) {
		return knowkit.ErrNotFound
	}
	_, err := r.pool.Exec(ctx, `DELETE FROM adrient.captable_access WHERE org_id=$1 AND user_id=$2`, a.Org, userID)
	r.audit(ctx, "access_revoke", "captable_access", userID)
	return err
}

// ---- company -------------------------------------------------------------------------------------------------

// Company is the company the cap table belongs to.
type Company struct {
	Name         string `json:"name"`
	Currency     string `json:"currency"`
	Incorporated string `json:"incorporated"`
	Notes        string `json:"notes"`
}

// GetCompany returns the company record (defaults when none is saved).
func (r *Repository) GetCompany(ctx context.Context, org string) (Company, error) {
	c := Company{Currency: "INR"}
	err := r.pool.QueryRow(ctx, `SELECT name, currency, COALESCE(to_char(incorporated,'YYYY-MM-DD'),''), notes FROM adrient.captable_company WHERE org_id=$1`, org).Scan(&c.Name, &c.Currency, &c.Incorporated, &c.Notes)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, nil
	}
	return c, err
}

// SaveCompany saves the company record.
func (r *Repository) SaveCompany(ctx context.Context, a knowkit.Actor, c Company) (Company, error) {
	c.Name, c.Currency = strings.TrimSpace(c.Name), strings.ToUpper(strings.TrimSpace(c.Currency))
	if c.Name == "" || tooLong(c.Name, 200) || tooLong(c.Notes, 5000) {
		return c, knowkit.Invalid("the company needs a name of up to 200 characters")
	}
	if len(c.Currency) != 3 {
		c.Currency = "INR"
	}
	var inc any
	if c.Incorporated != "" {
		if !validDate(c.Incorporated) {
			return c, knowkit.Invalid("the incorporation date must be YYYY-MM-DD")
		}
		inc = c.Incorporated
	}
	_, err := r.pool.Exec(ctx, `INSERT INTO adrient.captable_company (org_id, name, currency, incorporated, notes) VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (org_id) DO UPDATE SET name=EXCLUDED.name, currency=EXCLUDED.currency, incorporated=EXCLUDED.incorporated, notes=EXCLUDED.notes`, a.Org, c.Name, c.Currency, inc, c.Notes)
	r.audit(ctx, "company_save", "captable_company", a.Org)
	return c, err
}

// ---- stakeholders --------------------------------------------------------------------------------------------

// StakeholderRec is a person or entity that holds (or may hold) equity.
type StakeholderRec struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Email     string `json:"email"`
	ContactID string `json:"contact_id,omitempty"`
	UserID    string `json:"user_id,omitempty"`
	Notes     string `json:"notes"`
	Holdings  int    `json:"securities"`
}

// StakeholderInput creates or edits a stakeholder.
type StakeholderInput struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Email     string `json:"email"`
	ContactID string `json:"contact_id"`
	UserID    string `json:"user_id"`
	Notes     string `json:"notes"`
}

func (in *StakeholderInput) clean() error {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || tooLong(in.Name, 200) || tooLong(in.Email, 200) || tooLong(in.Notes, 5000) {
		return knowkit.Invalid("a stakeholder needs a name of up to 200 characters")
	}
	if in.Kind == "" {
		in.Kind = "employee"
	}
	if !knowkit.OneOf(in.Kind, "founder", "employee", "investor", "advisor", "entity") {
		return knowkit.Invalid("kind must be founder, employee, investor, advisor or entity")
	}
	for _, id := range []string{in.ContactID, in.UserID} {
		if id != "" && !knowkit.IsUUID(id) {
			return knowkit.Invalid("bad id")
		}
	}
	return nil
}

func nullUUID(s string) any {
	if s == "" {
		return nil
	}
	return s
}

const holderCols = `s.id::text, s.name, s.kind, s.email, COALESCE(s.contact_id::text,''), COALESCE(s.user_id::text,''), s.notes,
	(SELECT count(*) FROM adrient.captable_securities x WHERE x.stakeholder_id=s.id)`

// Stakeholders lists all stakeholders.
func (r *Repository) Stakeholders(ctx context.Context, org string) ([]StakeholderRec, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+holderCols+` FROM adrient.captable_stakeholders s WHERE s.org_id=$1 ORDER BY lower(s.name)`, org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []StakeholderRec{}
	for rows.Next() {
		var s StakeholderRec
		if err := rows.Scan(&s.ID, &s.Name, &s.Kind, &s.Email, &s.ContactID, &s.UserID, &s.Notes, &s.Holdings); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *Repository) stakeholder(ctx context.Context, org, id string) (StakeholderRec, error) {
	var s StakeholderRec
	if !knowkit.IsUUID(id) {
		return s, knowkit.ErrNotFound
	}
	err := r.pool.QueryRow(ctx, `SELECT `+holderCols+` FROM adrient.captable_stakeholders s WHERE s.org_id=$1 AND s.id=$2`, org, id).
		Scan(&s.ID, &s.Name, &s.Kind, &s.Email, &s.ContactID, &s.UserID, &s.Notes, &s.Holdings)
	return s, notFound(err)
}

// CreateStakeholder adds a stakeholder.
func (r *Repository) CreateStakeholder(ctx context.Context, a knowkit.Actor, in StakeholderInput) (StakeholderRec, error) {
	if err := in.clean(); err != nil {
		return StakeholderRec{}, err
	}
	var id string
	if err := r.pool.QueryRow(ctx, `INSERT INTO adrient.captable_stakeholders (org_id, name, kind, email, contact_id, user_id, notes) VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id::text`,
		a.Org, in.Name, in.Kind, strings.TrimSpace(in.Email), nullUUID(in.ContactID), nullUUID(in.UserID), in.Notes).Scan(&id); err != nil {
		return StakeholderRec{}, err
	}
	r.audit(ctx, "stakeholder_create", "captable_stakeholder", id)
	return r.stakeholder(ctx, a.Org, id)
}

// UpdateStakeholder edits a stakeholder.
func (r *Repository) UpdateStakeholder(ctx context.Context, a knowkit.Actor, id string, in StakeholderInput) (StakeholderRec, error) {
	if err := in.clean(); err != nil {
		return StakeholderRec{}, err
	}
	if _, err := r.stakeholder(ctx, a.Org, id); err != nil {
		return StakeholderRec{}, err
	}
	if _, err := r.pool.Exec(ctx, `UPDATE adrient.captable_stakeholders SET name=$3, kind=$4, email=$5, contact_id=$6, user_id=$7, notes=$8 WHERE org_id=$1 AND id=$2`,
		a.Org, id, in.Name, in.Kind, strings.TrimSpace(in.Email), nullUUID(in.ContactID), nullUUID(in.UserID), in.Notes); err != nil {
		return StakeholderRec{}, err
	}
	r.audit(ctx, "stakeholder_update", "captable_stakeholder", id)
	return r.stakeholder(ctx, a.Org, id)
}

// DeleteStakeholder removes a stakeholder with no securities.
func (r *Repository) DeleteStakeholder(ctx context.Context, a knowkit.Actor, id string) error {
	s, err := r.stakeholder(ctx, a.Org, id)
	if err != nil {
		return err
	}
	if s.Holdings > 0 {
		return knowkit.Invalid("%s has equity records; they cannot be deleted", s.Name)
	}
	_, err = r.pool.Exec(ctx, `DELETE FROM adrient.captable_stakeholders WHERE org_id=$1 AND id=$2`, a.Org, id)
	r.audit(ctx, "stakeholder_delete", "captable_stakeholder", id)
	return err
}

// ---- classes -------------------------------------------------------------------------------------------------

// Class is a share class.
type Class struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Kind              string `json:"kind"`
	Seniority         int    `json:"seniority"`
	PrefMultipleMilli int    `json:"pref_multiple_milli"`
	Participating     bool   `json:"participating"`
	ConvertRatioMilli int    `json:"convert_ratio_milli"`
	IssuePrice        string `json:"issue_price"`
	Authorized        int64  `json:"authorized"`
	Issued            int64  `json:"issued"`
}

// ClassInput creates or edits a class.
type ClassInput struct {
	Name              string `json:"name"`
	Kind              string `json:"kind"`
	Seniority         int    `json:"seniority"`
	PrefMultipleMilli int    `json:"pref_multiple_milli"`
	Participating     bool   `json:"participating"`
	ConvertRatioMilli int    `json:"convert_ratio_milli"`
	IssuePrice        string `json:"issue_price"`
	Authorized        int64  `json:"authorized"`
}

func (in *ClassInput) clean() error {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || tooLong(in.Name, 100) {
		return knowkit.Invalid("a class needs a name of up to 100 characters")
	}
	if in.Kind == "" {
		in.Kind = "common"
	}
	if !knowkit.OneOf(in.Kind, "common", "preferred") {
		return knowkit.Invalid("kind must be common or preferred")
	}
	if in.PrefMultipleMilli == 0 {
		in.PrefMultipleMilli = 1000
	}
	if in.ConvertRatioMilli == 0 {
		in.ConvertRatioMilli = 1000
	}
	if in.PrefMultipleMilli < 0 || in.PrefMultipleMilli > 20000 || in.ConvertRatioMilli < 1 || in.ConvertRatioMilli > 1000000 || in.Seniority < 0 || in.Seniority > 100 || in.Authorized < 0 {
		return knowkit.Invalid("the preference multiple, conversion ratio, seniority or authorised count is out of range")
	}
	p, err := cleanDecimal(in.IssuePrice)
	if err != nil {
		return err
	}
	in.IssuePrice = p
	if in.Kind == "common" {
		in.Participating = false
	}
	return nil
}

// cleanDecimal validates a non-negative decimal string with up to 8 decimals and returns it normalised.
func cleanDecimal(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "0", nil
	}
	x, ok := new(big.Rat).SetString(s)
	if !ok || x.Sign() < 0 || strings.ContainsAny(s, "eE/") {
		return "", knowkit.Invalid("%q is not a valid price", s)
	}
	if i := strings.IndexByte(s, '.'); i >= 0 && len(s)-i-1 > 8 {
		return "", knowkit.Invalid("a price takes at most 8 decimals")
	}
	if x.Cmp(big.NewRat(1_000_000_000_000, 1)) > 0 {
		return "", knowkit.Invalid("the price is out of range")
	}
	return s, nil
}

// priceCents converts a decimal price in currency units to cents per share, exactly.
func priceCents(s string) *big.Rat {
	x, ok := new(big.Rat).SetString(strings.TrimSpace(s))
	if !ok {
		return new(big.Rat)
	}
	return x.Mul(x, big.NewRat(100, 1))
}

const classCols = `c.id::text, c.name, c.kind, c.seniority, c.pref_multiple_milli, c.participating, c.convert_ratio_milli, c.issue_price::text, c.authorized,
	COALESCE((SELECT sum(x.quantity) FROM adrient.captable_securities x WHERE x.class_id=c.id AND x.kind='stock'),0)`

// Classes lists share classes.
func (r *Repository) Classes(ctx context.Context, org string) ([]Class, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+classCols+` FROM adrient.captable_classes c WHERE c.org_id=$1 ORDER BY c.seniority DESC, c.name`, org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Class{}
	for rows.Next() {
		var c Class
		if err := rows.Scan(&c.ID, &c.Name, &c.Kind, &c.Seniority, &c.PrefMultipleMilli, &c.Participating, &c.ConvertRatioMilli, &c.IssuePrice, &c.Authorized, &c.Issued); err != nil {
			return nil, err
		}
		c.IssuePrice = trimDecimal(c.IssuePrice)
		out = append(out, c)
	}
	return out, rows.Err()
}

// trimDecimal turns "1.50000000" into "1.5" and "2.00000000" into "2".
func trimDecimal(s string) string {
	if !strings.Contains(s, ".") {
		return s
	}
	s = strings.TrimRight(s, "0")
	return strings.TrimSuffix(s, ".")
}

// CreateClass adds a share class.
func (r *Repository) CreateClass(ctx context.Context, a knowkit.Actor, in ClassInput) (Class, error) {
	if err := in.clean(); err != nil {
		return Class{}, err
	}
	var id string
	err := r.pool.QueryRow(ctx, `INSERT INTO adrient.captable_classes (org_id, name, kind, seniority, pref_multiple_milli, participating, convert_ratio_milli, issue_price, authorized)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8::numeric,$9) RETURNING id::text`, a.Org, in.Name, in.Kind, in.Seniority, in.PrefMultipleMilli, in.Participating, in.ConvertRatioMilli, in.IssuePrice, in.Authorized).Scan(&id)
	if err != nil {
		if strings.Contains(err.Error(), "23505") {
			return Class{}, knowkit.Invalid("a class called %q already exists", in.Name)
		}
		return Class{}, err
	}
	r.audit(ctx, "class_create", "captable_class", id)
	return r.class(ctx, a.Org, id)
}

func (r *Repository) class(ctx context.Context, org, id string) (Class, error) {
	var c Class
	if !knowkit.IsUUID(id) {
		return c, knowkit.ErrNotFound
	}
	err := r.pool.QueryRow(ctx, `SELECT `+classCols+` FROM adrient.captable_classes c WHERE c.org_id=$1 AND c.id=$2`, org, id).
		Scan(&c.ID, &c.Name, &c.Kind, &c.Seniority, &c.PrefMultipleMilli, &c.Participating, &c.ConvertRatioMilli, &c.IssuePrice, &c.Authorized, &c.Issued)
	c.IssuePrice = trimDecimal(c.IssuePrice)
	return c, notFound(err)
}

// UpdateClass edits a class.
func (r *Repository) UpdateClass(ctx context.Context, a knowkit.Actor, id string, in ClassInput) (Class, error) {
	if err := in.clean(); err != nil {
		return Class{}, err
	}
	cur, err := r.class(ctx, a.Org, id)
	if err != nil {
		return Class{}, err
	}
	if cur.Issued > 0 && cur.Kind != in.Kind {
		return Class{}, knowkit.Invalid("shares of this class are already issued; its kind cannot change")
	}
	if in.Authorized > 0 && in.Authorized < cur.Issued {
		return Class{}, knowkit.Invalid("%d shares are already issued, more than the new authorised count", cur.Issued)
	}
	if _, err := r.pool.Exec(ctx, `UPDATE adrient.captable_classes SET name=$3, kind=$4, seniority=$5, pref_multiple_milli=$6, participating=$7, convert_ratio_milli=$8, issue_price=$9::numeric, authorized=$10 WHERE org_id=$1 AND id=$2`,
		a.Org, id, in.Name, in.Kind, in.Seniority, in.PrefMultipleMilli, in.Participating, in.ConvertRatioMilli, in.IssuePrice, in.Authorized); err != nil {
		return Class{}, err
	}
	r.audit(ctx, "class_update", "captable_class", id)
	return r.class(ctx, a.Org, id)
}

// DeleteClass removes a class with no securities.
func (r *Repository) DeleteClass(ctx context.Context, a knowkit.Actor, id string) error {
	if _, err := r.class(ctx, a.Org, id); err != nil {
		return err
	}
	var n int
	_ = r.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.captable_securities WHERE class_id=$1`, id).Scan(&n)
	if n > 0 {
		return knowkit.Invalid("securities use this class")
	}
	_, err := r.pool.Exec(ctx, `DELETE FROM adrient.captable_classes WHERE org_id=$1 AND id=$2`, a.Org, id)
	r.audit(ctx, "class_delete", "captable_class", id)
	return err
}

// ---- pools ---------------------------------------------------------------------------------------------------

// PoolRec is an option pool (equity incentive plan reserve).
type PoolRec struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	Adopted string `json:"adopted"`
	Notes   string `json:"notes"`
}

// PoolInput creates or edits a pool.
type PoolInput struct {
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	Adopted string `json:"adopted"`
	Notes   string `json:"notes"`
}

func (in *PoolInput) clean() error {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || tooLong(in.Name, 100) || tooLong(in.Notes, 2000) || in.Size < 0 || in.Size > 1e13 {
		return knowkit.Invalid("a pool needs a name and a size of zero or more shares")
	}
	if !validDate(in.Adopted) {
		return knowkit.Invalid("the adoption date must be YYYY-MM-DD")
	}
	return nil
}

// Pools lists the option pools.
func (r *Repository) Pools(ctx context.Context, org string) ([]PoolRec, error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text, name, size, to_char(adopted,'YYYY-MM-DD'), notes FROM adrient.captable_pools WHERE org_id=$1 ORDER BY adopted, name`, org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PoolRec{}
	for rows.Next() {
		var p PoolRec
		if err := rows.Scan(&p.ID, &p.Name, &p.Size, &p.Adopted, &p.Notes); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// CreatePool adds a pool.
func (r *Repository) CreatePool(ctx context.Context, a knowkit.Actor, in PoolInput) (PoolRec, error) {
	if err := in.clean(); err != nil {
		return PoolRec{}, err
	}
	var p PoolRec
	err := r.pool.QueryRow(ctx, `INSERT INTO adrient.captable_pools (org_id, name, size, adopted, notes) VALUES ($1,$2,$3,$4,$5) RETURNING id::text, name, size, to_char(adopted,'YYYY-MM-DD'), notes`,
		a.Org, in.Name, in.Size, in.Adopted, in.Notes).Scan(&p.ID, &p.Name, &p.Size, &p.Adopted, &p.Notes)
	r.audit(ctx, "pool_create", "captable_pool", p.ID)
	return p, err
}

// UpdatePool edits a pool; the size cannot drop below what has been granted from it.
func (r *Repository) UpdatePool(ctx context.Context, a knowkit.Actor, id string, in PoolInput) (PoolRec, error) {
	if err := in.clean(); err != nil {
		return PoolRec{}, err
	}
	if !knowkit.IsUUID(id) {
		return PoolRec{}, knowkit.ErrNotFound
	}
	var drawn int64
	_ = r.pool.QueryRow(ctx, `SELECT COALESCE(sum(x.quantity - COALESCE((SELECT sum(e.quantity) FROM adrient.captable_events e WHERE e.security_id=x.id AND e.kind='cancel'),0)),0)
		FROM adrient.captable_securities x WHERE x.pool_id=$1 AND x.kind='option'`, id).Scan(&drawn)
	if in.Size < drawn {
		return PoolRec{}, knowkit.Invalid("%d shares are already granted from this pool", drawn)
	}
	var p PoolRec
	err := r.pool.QueryRow(ctx, `UPDATE adrient.captable_pools SET name=$3, size=$4, adopted=$5, notes=$6 WHERE org_id=$1 AND id=$2 RETURNING id::text, name, size, to_char(adopted,'YYYY-MM-DD'), notes`,
		a.Org, id, in.Name, in.Size, in.Adopted, in.Notes).Scan(&p.ID, &p.Name, &p.Size, &p.Adopted, &p.Notes)
	r.audit(ctx, "pool_update", "captable_pool", id)
	return p, notFound(err)
}

// DeletePool removes a pool nothing was granted from.
func (r *Repository) DeletePool(ctx context.Context, a knowkit.Actor, id string) error {
	if !knowkit.IsUUID(id) {
		return knowkit.ErrNotFound
	}
	var n int
	_ = r.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.captable_securities WHERE pool_id=$1`, id).Scan(&n)
	if n > 0 {
		return knowkit.Invalid("grants were made from this pool")
	}
	_, err := r.pool.Exec(ctx, `DELETE FROM adrient.captable_pools WHERE org_id=$1 AND id=$2`, a.Org, id)
	r.audit(ctx, "pool_delete", "captable_pool", id)
	return err
}

// ---- rounds --------------------------------------------------------------------------------------------------

// RoundRec is a financing round on record.
type RoundRec struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Kind          string `json:"kind"`
	Date          string `json:"date"`
	PreMoney      int64  `json:"pre_money"`
	Investment    int64  `json:"investment"`
	PricePerShare string `json:"price_per_share"`
	ClassID       string `json:"class_id,omitempty"`
	Class         string `json:"class,omitempty"`
	Notes         string `json:"notes"`
	PostMoney     int64  `json:"post_money"`
}

// RoundInputRec creates or edits a round record.
type RoundInputRec struct {
	Name          string `json:"name"`
	Kind          string `json:"kind"`
	Date          string `json:"date"`
	PreMoney      int64  `json:"pre_money"`
	Investment    int64  `json:"investment"`
	PricePerShare string `json:"price_per_share"`
	ClassID       string `json:"class_id"`
	Notes         string `json:"notes"`
}

func (in *RoundInputRec) clean() error {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || tooLong(in.Name, 100) || tooLong(in.Notes, 2000) {
		return knowkit.Invalid("a round needs a name")
	}
	if in.Kind == "" {
		in.Kind = "priced"
	}
	if !knowkit.OneOf(in.Kind, "founding", "priced", "bridge", "other") || !validDate(in.Date) {
		return knowkit.Invalid("the round kind or date is not valid")
	}
	if in.PreMoney < 0 || in.Investment < 0 || in.PreMoney > 1e15 || in.Investment > 1e15 {
		return knowkit.Invalid("amounts are out of range")
	}
	p, err := cleanDecimal(in.PricePerShare)
	if err != nil {
		return err
	}
	in.PricePerShare = p
	if in.ClassID != "" && !knowkit.IsUUID(in.ClassID) {
		return knowkit.Invalid("bad class")
	}
	return nil
}

const roundCols = `r.id::text, r.name, r.kind, to_char(r.round_date,'YYYY-MM-DD'), r.pre_money, r.investment, r.price_per_share::text, COALESCE(r.class_id::text,''), COALESCE(c.name,''), r.notes`

// Rounds lists financing rounds, oldest first.
func (r *Repository) Rounds(ctx context.Context, org string) ([]RoundRec, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+roundCols+` FROM adrient.captable_rounds r LEFT JOIN adrient.captable_classes c ON c.id=r.class_id WHERE r.org_id=$1 ORDER BY r.round_date, r.created_at`, org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RoundRec{}
	for rows.Next() {
		var x RoundRec
		if err := rows.Scan(&x.ID, &x.Name, &x.Kind, &x.Date, &x.PreMoney, &x.Investment, &x.PricePerShare, &x.ClassID, &x.Class, &x.Notes); err != nil {
			return nil, err
		}
		x.PricePerShare = trimDecimal(x.PricePerShare)
		x.PostMoney = x.PreMoney + x.Investment
		out = append(out, x)
	}
	return out, rows.Err()
}

// CreateRound records a round that already happened (use the round modeller to close a new one).
func (r *Repository) CreateRound(ctx context.Context, a knowkit.Actor, in RoundInputRec) (RoundRec, error) {
	if err := in.clean(); err != nil {
		return RoundRec{}, err
	}
	var id string
	if err := r.pool.QueryRow(ctx, `INSERT INTO adrient.captable_rounds (org_id, name, kind, round_date, pre_money, investment, price_per_share, class_id, notes)
		VALUES ($1,$2,$3,$4,$5,$6,$7::numeric,$8,$9) RETURNING id::text`, a.Org, in.Name, in.Kind, in.Date, in.PreMoney, in.Investment, in.PricePerShare, nullUUID(in.ClassID), in.Notes).Scan(&id); err != nil {
		return RoundRec{}, err
	}
	r.audit(ctx, "round_create", "captable_round", id)
	return r.round(ctx, a.Org, id)
}

func (r *Repository) round(ctx context.Context, org, id string) (RoundRec, error) {
	var x RoundRec
	err := r.pool.QueryRow(ctx, `SELECT `+roundCols+` FROM adrient.captable_rounds r LEFT JOIN adrient.captable_classes c ON c.id=r.class_id WHERE r.org_id=$1 AND r.id=$2`, org, id).
		Scan(&x.ID, &x.Name, &x.Kind, &x.Date, &x.PreMoney, &x.Investment, &x.PricePerShare, &x.ClassID, &x.Class, &x.Notes)
	x.PricePerShare = trimDecimal(x.PricePerShare)
	x.PostMoney = x.PreMoney + x.Investment
	return x, notFound(err)
}

// UpdateRound edits a round record.
func (r *Repository) UpdateRound(ctx context.Context, a knowkit.Actor, id string, in RoundInputRec) (RoundRec, error) {
	if err := in.clean(); err != nil {
		return RoundRec{}, err
	}
	if !knowkit.IsUUID(id) {
		return RoundRec{}, knowkit.ErrNotFound
	}
	if _, err := r.pool.Exec(ctx, `UPDATE adrient.captable_rounds SET name=$3, kind=$4, round_date=$5, pre_money=$6, investment=$7, price_per_share=$8::numeric, class_id=$9, notes=$10 WHERE org_id=$1 AND id=$2`,
		a.Org, id, in.Name, in.Kind, in.Date, in.PreMoney, in.Investment, in.PricePerShare, nullUUID(in.ClassID), in.Notes); err != nil {
		return RoundRec{}, err
	}
	r.audit(ctx, "round_update", "captable_round", id)
	return r.round(ctx, a.Org, id)
}

// DeleteRound removes a round record (securities keep existing, unlinked).
func (r *Repository) DeleteRound(ctx context.Context, a knowkit.Actor, id string) error {
	if !knowkit.IsUUID(id) {
		return knowkit.ErrNotFound
	}
	_, err := r.pool.Exec(ctx, `DELETE FROM adrient.captable_rounds WHERE org_id=$1 AND id=$2`, a.Org, id)
	r.audit(ctx, "round_delete", "captable_round", id)
	return err
}

var _ = fmt.Sprintf
