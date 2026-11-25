package scim

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ---- users -----------------------------------------------------------------

// UserRow is a user as SCIM sees it.
type UserRow struct {
	ID, Email, DisplayName, Title, Phone, EmployeeID, Department, ManagerID, ExternalID, Status string
	Created, Updated                                                                            time.Time
}

const userCols = `u.id::text, u.email, u.display_name, u.job_title, u.phone, u.employee_id,
COALESCE((SELECT d.name FROM adrient.departments d WHERE d.id=u.department_id),''),
COALESCE(u.manager_id::text,''), COALESCE(u.scim_external_id,''), u.status, u.created_at, u.updated_at`

func scanUser(row pgx.Row) (UserRow, error) {
	var u UserRow
	err := row.Scan(&u.ID, &u.Email, &u.DisplayName, &u.Title, &u.Phone, &u.EmployeeID, &u.Department, &u.ManagerID, &u.ExternalID, &u.Status, &u.Created, &u.Updated)
	if errors.Is(err, pgx.ErrNoRows) {
		return u, errNotFound
	}
	return u, err
}

var (
	errNotFound   = errors.New("not found")
	errUniqueness = errors.New("uniqueness")
	errNoSeats    = errors.New("no seats available")
)

const userScope = `u.org_id=$1 AND u.user_type='member' AND u.deleted_at IS NULL`

func isUnique(err error) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && pe.Code == "23505"
}

// GetUser fetches one user of orgID.
func (s *Store) GetUser(ctx context.Context, orgID, id string) (UserRow, error) {
	if !isUUID(id) {
		return UserRow{}, errNotFound
	}
	return scanUser(s.Pool.QueryRow(ctx, `SELECT `+userCols+` FROM adrient.users u WHERE `+userScope+` AND u.id::text=$2`, orgID, id))
}

// ListUsers lists users with an optional filter, 1-based startIndex and count.
func (s *Store) ListUsers(ctx context.Context, orgID string, f *Filter, start, count int) ([]UserRow, int, error) {
	where, args := ` AND TRUE`, []any{orgID}
	if f != nil {
		col := ""
		switch f.Attr {
		case "username", "emails.value", "emails[primary eq true].value":
			col = "u.email"
		case "externalid":
			col = "COALESCE(u.scim_external_id,'')"
		case "id":
			col = "u.id::text"
		case "displayname", "name.formatted":
			col = "u.display_name"
		default:
			return nil, 0, errBadFilter
		}
		cond, arg := sqlCond(col, f)
		where += " AND " + cond
		if arg != "" || f.Op != "pr" {
			args = append(args, arg)
		}
	}
	var total int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM adrient.users u WHERE `+userScope+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, count, start-1)
	rows, err := s.Pool.Query(ctx, `SELECT `+userCols+` FROM adrient.users u WHERE `+userScope+where+
		` ORDER BY u.created_at, u.id LIMIT $`+itoa(len(args)-1)+` OFFSET $`+itoa(len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []UserRow{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, u)
	}
	return out, total, rows.Err()
}

var errBadFilter = errors.New("unsupported filter attribute")

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// sqlCond builds a parameterised condition; the parameter index is filled by
// the caller (it always is the next argument, $2 for the first filter).
func sqlCond(col string, f *Filter) (string, string) {
	esc := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(strings.ToLower(f.Value))
	lc := "lower(" + col + ")"
	switch f.Op {
	case "eq":
		return lc + "=$2", strings.ToLower(f.Value)
	case "ne":
		return lc + "<>$2", strings.ToLower(f.Value)
	case "co":
		return lc + " LIKE $2", "%" + esc + "%"
	case "sw":
		return lc + " LIKE $2", esc + "%"
	case "ew":
		return lc + " LIKE $2", "%" + esc
	}
	return col + " <> ''", "" // pr
}

// NewUser is the input of CreateUser.
type NewUser struct {
	Email, DisplayName, Title, Phone, EmployeeID, Department, ExternalID string
	Active                                                               bool
	Subject, Issuer                                                      string // identity-provider account, when known
}

func (s *Store) departmentID(ctx context.Context, orgID, name string) any {
	if strings.TrimSpace(name) == "" {
		return nil
	}
	var id string
	if err := s.Pool.QueryRow(ctx, `SELECT id::text FROM adrient.departments WHERE org_id=$1 AND lower(name)=lower($2) LIMIT 1`, orgID, name).Scan(&id); err != nil {
		return nil
	}
	return id
}

// CreateUser inserts a SCIM-managed member.
func (s *Store) CreateUser(ctx context.Context, orgID string, in NewUser) (UserRow, error) {
	email := strings.ToLower(strings.TrimSpace(in.Email))
	var exists bool
	if err := s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM adrient.users WHERE org_id=$1 AND lower(email)=$2 AND deleted_at IS NULL)`, orgID, email).Scan(&exists); err != nil {
		return UserRow{}, err
	}
	if exists {
		return UserRow{}, errUniqueness
	}
	status, blockedAt, reason := "active", any(nil), ""
	if !in.Active {
		status, blockedAt, reason = "blocked", time.Now(), "Deactivated by the identity provider"
	}
	issuer, subject := in.Issuer, in.Subject
	if subject == "" {
		issuer = PreprovisionedIssuer
		subject = "scim-" + newID()
	}
	name := strings.TrimSpace(in.DisplayName)
	if name == "" {
		name = email
	}
	var id string
	err := s.Pool.QueryRow(ctx, `INSERT INTO adrient.users (org_id, oidc_issuer, oidc_subject, email, display_name, job_title, phone, employee_id,
department_id, scim_external_id, scim_managed, status, blocked_at, blocked_reason)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9::uuid,NULLIF($10,''),true,$11,$12,$13) RETURNING id::text`,
		orgID, issuer, subject, email, name, in.Title, in.Phone, in.EmployeeID, s.departmentID(ctx, orgID, in.Department),
		in.ExternalID, status, blockedAt, reason).Scan(&id)
	if err != nil {
		if isUnique(err) {
			return UserRow{}, errUniqueness
		}
		return UserRow{}, err
	}
	return s.GetUser(ctx, orgID, id)
}

// UpdateUser applies a full replacement (PUT) or patch to a user. Fields that
// are nil are left alone.
func (s *Store) UpdateUser(ctx context.Context, orgID, id string, p userPatch) (UserRow, error) {
	cur, err := s.GetUser(ctx, orgID, id)
	if err != nil {
		return cur, err
	}
	name := cur.DisplayName
	if p.DisplayName != nil {
		name = *p.DisplayName
	}
	if p.Given != nil || p.Family != nil {
		given, family, _ := strings.Cut(strings.TrimSpace(cur.DisplayName), " ")
		if p.Given != nil {
			given = *p.Given
		}
		if p.Family != nil {
			family = *p.Family
		}
		name = strings.TrimSpace(given + " " + family)
	}
	email := cur.Email
	if p.UserName != nil && *p.UserName != "" {
		email = strings.ToLower(strings.TrimSpace(*p.UserName))
	} else if p.Email != nil && *p.Email != "" {
		email = strings.ToLower(strings.TrimSpace(*p.Email))
	}
	title, phone, emp, ext := cur.Title, cur.Phone, cur.EmployeeID, cur.ExternalID
	if p.Title != nil {
		title = *p.Title
	}
	if p.Phone != nil {
		phone = *p.Phone
	}
	if p.EmployeeID != nil {
		emp = *p.EmployeeID
	}
	if p.ExternalID != nil {
		ext = *p.ExternalID
	}
	dept := any(nil)
	if p.Department != nil {
		dept = s.departmentID(ctx, orgID, *p.Department)
	}
	_, err = s.Pool.Exec(ctx, `UPDATE adrient.users SET email=$3, display_name=$4, job_title=$5, phone=$6, employee_id=$7,
scim_external_id=NULLIF($8,''), scim_managed=true,
department_id = CASE WHEN $10 THEN $9::uuid ELSE department_id END, updated_at=now()
WHERE org_id=$1 AND id::text=$2`, orgID, id, email, name, title, phone, emp, ext, dept, p.Department != nil)
	if err != nil {
		if isUnique(err) {
			return cur, errUniqueness
		}
		return cur, err
	}
	return s.GetUser(ctx, orgID, id)
}

// SetActive blocks or unblocks a user and ends their sessions when blocking.
// It returns the IdP subject so the caller can mirror the state in Zitadel.
func (s *Store) SetActive(ctx context.Context, orgID, id string, active bool) (issuer, subject string, changed bool, err error) {
	var status string
	err = s.Pool.QueryRow(ctx, `SELECT status, oidc_issuer, oidc_subject FROM adrient.users WHERE org_id=$1 AND id::text=$2 AND deleted_at IS NULL`, orgID, id).Scan(&status, &issuer, &subject)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", false, errNotFound
	}
	if err != nil {
		return
	}
	if active == (status == "active") {
		return issuer, subject, false, nil
	}
	if active {
		_, err = s.Pool.Exec(ctx, `UPDATE adrient.users SET status='active', blocked_at=NULL, blocked_reason='', updated_at=now() WHERE org_id=$1 AND id::text=$2`, orgID, id)
		return issuer, subject, true, err
	}
	if _, err = s.Pool.Exec(ctx, `UPDATE adrient.users SET status='blocked', blocked_at=now(), blocked_reason='Deactivated by the identity provider', updated_at=now() WHERE org_id=$1 AND id::text=$2`, orgID, id); err != nil {
		return
	}
	_, err = s.Pool.Exec(ctx, `UPDATE adrient.sessions SET revoked_at=now() WHERE user_id::text=$1 AND revoked_at IS NULL`, id)
	return issuer, subject, true, err
}

// ---- groups ----------------------------------------------------------------

// GroupRow is a group as SCIM sees it.
type GroupRow struct {
	ID, Name, ExternalID string
	Members              []string
	Created, Updated     time.Time
}

func (s *Store) groupFrom(ctx context.Context, row pgx.Row) (GroupRow, error) {
	var g GroupRow
	var raw []byte
	err := row.Scan(&g.ID, &g.Name, &g.ExternalID, &raw, &g.Created, &g.Updated)
	if errors.Is(err, pgx.ErrNoRows) {
		return g, errNotFound
	}
	if err != nil {
		return g, err
	}
	_ = json.Unmarshal(raw, &g.Members)
	if g.Members == nil {
		g.Members = []string{}
	}
	return g, nil
}

const groupCols = `g.id::text, g.name, COALESCE(g.scim_external_id,''), g.member_ids, g.created_at, g.updated_at`

// GetGroup fetches one group of orgID.
func (s *Store) GetGroup(ctx context.Context, orgID, id string) (GroupRow, error) {
	if !isUUID(id) {
		return GroupRow{}, errNotFound
	}
	return s.groupFrom(ctx, s.Pool.QueryRow(ctx, `SELECT `+groupCols+` FROM adrient.groups g WHERE g.org_id=$1 AND g.id::text=$2`, orgID, id))
}

// ListGroups lists groups.
func (s *Store) ListGroups(ctx context.Context, orgID string, f *Filter, start, count int) ([]GroupRow, int, error) {
	where, args := "", []any{orgID}
	if f != nil {
		col := ""
		switch f.Attr {
		case "displayname":
			col = "g.name"
		case "externalid":
			col = "COALESCE(g.scim_external_id,'')"
		case "id":
			col = "g.id::text"
		default:
			return nil, 0, errBadFilter
		}
		cond, arg := sqlCond(col, f)
		where = " AND " + cond
		if arg != "" || f.Op != "pr" {
			args = append(args, arg)
		}
	}
	var total int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM adrient.groups g WHERE g.org_id=$1`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, count, start-1)
	rows, err := s.Pool.Query(ctx, `SELECT `+groupCols+` FROM adrient.groups g WHERE g.org_id=$1`+where+
		` ORDER BY g.created_at, g.id LIMIT $`+itoa(len(args)-1)+` OFFSET $`+itoa(len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []GroupRow{}
	for rows.Next() {
		g, err := s.groupFrom(ctx, rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, g)
	}
	return out, total, rows.Err()
}

// orgUserIDs keeps only the ids that are users of orgID (never another org's).
func (s *Store) orgUserIDs(ctx context.Context, orgID string, ids []string) ([]string, error) {
	clean := []string{}
	for _, id := range ids {
		if isUUID(id) {
			clean = append(clean, id)
		}
	}
	if len(clean) == 0 {
		return []string{}, nil
	}
	rows, err := s.Pool.Query(ctx, `SELECT id::text FROM adrient.users WHERE org_id=$1 AND id::text = ANY($2) AND deleted_at IS NULL`, orgID, clean)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// groupOwner picks the owner for a SCIM-created group (groups need one).
func (s *Store) groupOwner(ctx context.Context, orgID, tokenID string) (string, error) {
	var id string
	err := s.Pool.QueryRow(ctx, `SELECT COALESCE(
 (SELECT created_by::text FROM adrient.scim_tokens WHERE id::text=$2 AND created_by IS NOT NULL),
 (SELECT user_id::text FROM adrient.org_admins WHERE org_id=$1 ORDER BY granted_at LIMIT 1),
 (SELECT id::text FROM adrient.users WHERE org_id=$1 AND user_type='member' ORDER BY created_at LIMIT 1))`, orgID, tokenID).Scan(&id)
	return id, err
}

// CreateGroup inserts a group with members (users of the same org only).
func (s *Store) CreateGroup(ctx context.Context, orgID, tokenID, name, externalID string, members []string) (GroupRow, error) {
	owner, err := s.groupOwner(ctx, orgID, tokenID)
	if err != nil || owner == "" {
		return GroupRow{}, errors.New("organization has no user to own the group")
	}
	ids, err := s.orgUserIDs(ctx, orgID, members)
	if err != nil {
		return GroupRow{}, err
	}
	raw, _ := json.Marshal(ids)
	var id string
	err = s.Pool.QueryRow(ctx, `INSERT INTO adrient.groups (org_id, owner_id, name, member_ids, scim_external_id)
VALUES ($1,$2::uuid,$3,$4,NULLIF($5,'')) RETURNING id::text`, orgID, owner, name, raw, externalID).Scan(&id)
	if err != nil {
		if isUnique(err) {
			return GroupRow{}, errUniqueness
		}
		return GroupRow{}, err
	}
	return s.GetGroup(ctx, orgID, id)
}

// UpdateGroup replaces name/externalId and applies member operations.
func (s *Store) UpdateGroup(ctx context.Context, orgID, id string, p groupPatch) (GroupRow, error) {
	cur, err := s.GetGroup(ctx, orgID, id)
	if err != nil {
		return cur, err
	}
	members := append([]string{}, cur.Members...)
	for _, op := range p.Members {
		valid, err := s.orgUserIDs(ctx, orgID, op.UserIDs)
		if err != nil {
			return cur, err
		}
		switch {
		case op.Op == "replace":
			members = valid
		case op.Op == "add":
			for _, v := range valid {
				if !containsStr(members, v) {
					members = append(members, v)
				}
			}
		case op.Op == "remove":
			if op.All {
				members = []string{}
				continue
			}
			keep := members[:0:0]
			for _, m := range members {
				if !containsStr(op.UserIDs, m) {
					keep = append(keep, m)
				}
			}
			members = keep
		}
	}
	name, ext := cur.Name, cur.ExternalID
	if p.DisplayName != nil && strings.TrimSpace(*p.DisplayName) != "" {
		name = *p.DisplayName
	}
	if p.ExternalID != nil {
		ext = *p.ExternalID
	}
	raw, _ := json.Marshal(members)
	_, err = s.Pool.Exec(ctx, `UPDATE adrient.groups SET name=$3, scim_external_id=NULLIF($4,''), member_ids=$5, updated_at=now() WHERE org_id=$1 AND id::text=$2`,
		orgID, id, name, ext, raw)
	if err != nil {
		if isUnique(err) {
			return cur, errUniqueness
		}
		return cur, err
	}
	return s.GetGroup(ctx, orgID, id)
}

// DeleteGroup removes a group of orgID.
func (s *Store) DeleteGroup(ctx context.Context, orgID, id string) error {
	if !isUUID(id) {
		return errNotFound
	}
	tag, err := s.Pool.Exec(ctx, `DELETE FROM adrient.groups WHERE org_id=$1 AND id::text=$2`, orgID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errNotFound
	}
	return nil
}

func containsStr(l []string, v string) bool {
	for _, x := range l {
		if x == v {
			return true
		}
	}
	return false
}

// EmailTaken reports whether a live user of the org already has the address.
func (s *Store) EmailTaken(ctx context.Context, orgID, email string) (bool, error) {
	var exists bool
	err := s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM adrient.users WHERE org_id::text=$1 AND lower(email)=lower($2) AND deleted_at IS NULL)`, orgID, email).Scan(&exists)
	return exists, err
}
