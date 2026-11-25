// Pages hold TipTap JSON, the editor stack Adrient Docs already uses; databases inside pages are Records tables.

// Package wiki is a team wiki: spaces of nested pages edited with a block
// editor, page databases (backed by internal/records), comments, history,
// favourites, search and Markdown import and export.
package wiki

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Adrient-tech/Adrient-Workspace/internal/knowkit"
	"github.com/Adrient-tech/Adrient-Workspace/internal/records"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Roles.
const (
	RoleNone  = 0
	RoleView  = 1
	RoleEdit  = 2
	RoleAdmin = 3
)

// Deps are optional connections to other modules.
type Deps struct {
	Audit knowkit.Audit
	// Records creates the database behind a database page.
	Records *records.Repository
	// FileURL turns a Drive file id into a link for inserted files (optional).
	Now func() time.Time
}

// Repository is the Postgres store. Every query is org-scoped.
type Repository struct {
	pool *pgxpool.Pool
	Deps Deps
}

// NewRepository constructs a Repository.
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (r *Repository) now() time.Time { return knowkit.Clock(r.Deps.Now) }

func (r *Repository) audit(ctx context.Context, action, typ, id string) {
	if r.Deps.Audit != nil {
		r.Deps.Audit(ctx, action, typ, id)
	}
}

// Space is a collection of pages with its own sharing.
type Space struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Icon      string   `json:"icon"`
	Kind      string   `json:"kind"`
	OwnerID   string   `json:"owner_id"`
	OrgAccess string   `json:"org_access"`
	Role      int      `json:"role"`
	Members   []Member `json:"members,omitempty"`
}

// Member is a user a space is shared with.
type Member struct {
	UserID string `json:"user_id"`
	Name   string `json:"name,omitempty"`
	Role   string `json:"role"`
}

func roleOfString(s string) int {
	switch s {
	case "viewer":
		return RoleView
	case "editor":
		return RoleEdit
	case "admin":
		return RoleAdmin
	}
	return RoleNone
}

// SpaceRole is the caller's role in a space (0 when it does not exist or is hidden).
func (r *Repository) SpaceRole(ctx context.Context, org, user string, admin bool, spaceID string) int {
	if !knowkit.IsUUID(spaceID) {
		return RoleNone
	}
	var owner, access, kind string
	if err := r.pool.QueryRow(ctx, `SELECT owner_id::text, org_access, kind FROM adrient.wiki_spaces WHERE org_id=$1 AND id=$2`, org, spaceID).Scan(&owner, &access, &kind); err != nil {
		return RoleNone
	}
	if owner == user {
		return RoleAdmin
	}
	if kind == "personal" {
		return RoleNone // personal spaces are private to their owner
	}
	if admin {
		return RoleAdmin
	}
	role := RoleNone
	var m string
	_ = r.pool.QueryRow(ctx, `SELECT role FROM adrient.wiki_space_members WHERE space_id=$1 AND user_id=$2`, spaceID, user).Scan(&m)
	role = roleOfString(m)
	switch access {
	case "view":
		role = max(role, RoleView)
	case "edit":
		role = max(role, RoleEdit)
	}
	return role
}

// ExternalRole lets Records ask for a role on databases embedded in wiki pages.
func (r *Repository) ExternalRole(ctx context.Context, org, user string, admin bool, spaceID string) int {
	return r.SpaceRole(ctx, org, user, admin, spaceID)
}

func (r *Repository) needSpace(ctx context.Context, a knowkit.Actor, id string, need int) (int, error) {
	role := r.SpaceRole(ctx, a.Org, a.User, a.Admin, id)
	if role == RoleNone {
		return 0, knowkit.ErrNotFound
	}
	if role < need {
		return role, knowkit.ErrForbidden
	}
	return role, nil
}

// ListSpaces lists the spaces the caller can open. The caller's personal space is created on first use.
func (r *Repository) ListSpaces(ctx context.Context, a knowkit.Actor) ([]Space, error) {
	var n int
	_ = r.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.wiki_spaces WHERE org_id=$1 AND owner_id=$2 AND kind='personal'`, a.Org, a.User).Scan(&n)
	if n == 0 {
		if _, err := r.pool.Exec(ctx, `INSERT INTO adrient.wiki_spaces (org_id, name, icon, kind, owner_id) VALUES ($1,'My pages','📝','personal',$2)`, a.Org, a.User); err != nil {
			return nil, err
		}
	}
	rows, err := r.pool.Query(ctx, `SELECT id::text, name, icon, kind, owner_id::text, org_access FROM adrient.wiki_spaces WHERE org_id=$1 ORDER BY kind DESC, lower(name)`, a.Org)
	if err != nil {
		return nil, err
	}
	var all []Space
	for rows.Next() {
		var s Space
		if err := rows.Scan(&s.ID, &s.Name, &s.Icon, &s.Kind, &s.OwnerID, &s.OrgAccess); err != nil {
			rows.Close()
			return nil, err
		}
		all = append(all, s)
	}
	rows.Close()
	out := []Space{}
	for _, s := range all {
		s.Role = r.SpaceRole(ctx, a.Org, a.User, a.Admin, s.ID)
		if s.Role > RoleNone {
			out = append(out, s)
		}
	}
	return out, nil
}

// SpaceInput creates or edits a space.
type SpaceInput struct {
	Name      string `json:"name"`
	Icon      string `json:"icon"`
	OrgAccess string `json:"org_access"`
}

func cleanSpace(in *SpaceInput) error {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || utf8.RuneCountInString(in.Name) > 80 {
		return knowkit.Invalid("a space needs a name of up to 80 characters")
	}
	if utf8.RuneCountInString(in.Icon) > 8 {
		return knowkit.Invalid("the icon is too long")
	}
	if in.OrgAccess == "" {
		in.OrgAccess = "none"
	}
	if !knowkit.OneOf(in.OrgAccess, "none", "view", "edit") {
		return knowkit.Invalid("org_access must be none, view or edit")
	}
	return nil
}

// CreateSpace makes a shared (team) space and a first page.
func (r *Repository) CreateSpace(ctx context.Context, a knowkit.Actor, in SpaceInput) (*Space, error) {
	if err := cleanSpace(&in); err != nil {
		return nil, err
	}
	s := &Space{Name: in.Name, Icon: in.Icon, Kind: "team", OwnerID: a.User, OrgAccess: in.OrgAccess, Role: RoleAdmin}
	if err := r.pool.QueryRow(ctx, `INSERT INTO adrient.wiki_spaces (org_id, name, icon, kind, owner_id, org_access) VALUES ($1,$2,$3,'team',$4,$5) RETURNING id::text`,
		a.Org, in.Name, in.Icon, a.User, in.OrgAccess).Scan(&s.ID); err != nil {
		return nil, err
	}
	if _, err := r.CreatePage(ctx, a, PageInput{SpaceID: s.ID, Title: "Welcome", Icon: "👋", Template: "welcome"}); err != nil {
		return nil, err
	}
	r.audit(ctx, "space.create", "wiki_space", s.ID)
	return s, nil
}

// UpdateSpace edits a space (admin only).
func (r *Repository) UpdateSpace(ctx context.Context, a knowkit.Actor, id string, in SpaceInput) error {
	if _, err := r.needSpace(ctx, a, id, RoleAdmin); err != nil {
		return err
	}
	if err := cleanSpace(&in); err != nil {
		return err
	}
	_, err := r.pool.Exec(ctx, `UPDATE adrient.wiki_spaces SET name=$3, icon=$4, org_access = CASE WHEN kind='personal' THEN 'none' ELSE $5 END WHERE org_id=$1 AND id=$2`, a.Org, id, in.Name, in.Icon, in.OrgAccess)
	return err
}

// DeleteSpace removes a team space and all its pages (and their databases).
func (r *Repository) DeleteSpace(ctx context.Context, a knowkit.Actor, id string) error {
	if _, err := r.needSpace(ctx, a, id, RoleAdmin); err != nil {
		return err
	}
	var kind string
	_ = r.pool.QueryRow(ctx, `SELECT kind FROM adrient.wiki_spaces WHERE id=$1`, id).Scan(&kind)
	if kind == "personal" {
		return knowkit.Invalid("a personal space cannot be deleted")
	}
	_, _ = r.pool.Exec(ctx, `DELETE FROM adrient.records_bases WHERE org_id=$1 AND source='wiki' AND source_ref=$2`, a.Org, id)
	_, err := r.pool.Exec(ctx, `DELETE FROM adrient.wiki_spaces WHERE org_id=$1 AND id=$2`, a.Org, id)
	r.audit(ctx, "space.delete", "wiki_space", id)
	return err
}

// SpaceMembers returns the sharing list (admins only).
func (r *Repository) SpaceMembers(ctx context.Context, a knowkit.Actor, id string) ([]Member, error) {
	if _, err := r.needSpace(ctx, a, id, RoleAdmin); err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, `SELECT m.user_id::text, COALESCE(NULLIF(u.display_name,''),u.email), m.role FROM adrient.wiki_space_members m JOIN adrient.users u ON u.id=m.user_id WHERE m.space_id=$1 ORDER BY 2`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Member{}
	for rows.Next() {
		var m Member
		if err := rows.Scan(&m.UserID, &m.Name, &m.Role); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// SetMembers replaces the people a space is shared with.
func (r *Repository) SetMembers(ctx context.Context, a knowkit.Actor, id string, members []Member) error {
	if _, err := r.needSpace(ctx, a, id, RoleAdmin); err != nil {
		return err
	}
	var kind, owner string
	_ = r.pool.QueryRow(ctx, `SELECT kind, owner_id::text FROM adrient.wiki_spaces WHERE id=$1`, id).Scan(&kind, &owner)
	if kind == "personal" {
		return knowkit.Invalid("a personal space cannot be shared; move the page to a team space")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx, `DELETE FROM adrient.wiki_space_members WHERE space_id=$1`, id); err != nil {
		return err
	}
	for _, m := range members {
		if !knowkit.IsUUID(m.UserID) || !knowkit.OneOf(m.Role, "viewer", "editor", "admin") || m.UserID == owner {
			continue
		}
		if _, err := tx.Exec(ctx, `INSERT INTO adrient.wiki_space_members (space_id, org_id, user_id, role) SELECT $1,$2,u.id,$4 FROM adrient.users u WHERE u.id=$3 AND u.org_id=$2
			ON CONFLICT (space_id,user_id) DO UPDATE SET role=EXCLUDED.role`, id, a.Org, m.UserID, m.Role); err != nil {
			return err
		}
	}
	r.audit(ctx, "space.share", "wiki_space", id)
	return tx.Commit(ctx)
}

// ---- pages -----------------------------------------------------------------

// Page is a wiki page; Content is omitted from tree listings.
type Page struct {
	ID             string          `json:"id"`
	SpaceID        string          `json:"space_id"`
	ParentID       string          `json:"parent_id,omitempty"`
	Title          string          `json:"title"`
	Icon           string          `json:"icon"`
	Kind           string          `json:"kind"`
	Position       float64         `json:"position"`
	Version        int             `json:"version"`
	Content        json.RawMessage `json:"content,omitempty"`
	RecordsBaseID  string          `json:"records_base_id,omitempty"`
	RecordsTableID string          `json:"records_table_id,omitempty"`
	UpdatedAt      time.Time       `json:"updated_at"`
	UpdatedBy      string          `json:"updated_by,omitempty"`
	TrashedAt      *time.Time      `json:"trashed_at,omitempty"`
	Role           int             `json:"role,omitempty"`
	Favorite       bool            `json:"favorite,omitempty"`
}

// PageInput creates a page.
type PageInput struct {
	SpaceID  string `json:"space_id"`
	ParentID string `json:"parent_id"`
	Title    string `json:"title"`
	Icon     string `json:"icon"`
	Kind     string `json:"kind"`     // page (default) or database
	Template string `json:"template"` // optional starter content
}

const emptyDoc = `{"type":"doc","content":[{"type":"paragraph"}]}`

// MaxPageBytes bounds the stored content of one page.
const MaxPageBytes = 4 << 20

// Tree returns every page of a space without content (trashed ones when asked).
func (r *Repository) Tree(ctx context.Context, a knowkit.Actor, spaceID string, trashed bool) ([]Page, error) {
	role, err := r.needSpace(ctx, a, spaceID, RoleView)
	if err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, `SELECT p.id::text, COALESCE(p.parent_id::text,''), p.title, p.icon, p.kind, p.position, p.version, p.updated_at, p.trashed_at,
		COALESCE(p.records_table_id::text,''), (f.page_id IS NOT NULL)
		FROM adrient.wiki_pages p LEFT JOIN adrient.wiki_favorites f ON f.page_id=p.id AND f.user_id=$3
		WHERE p.org_id=$1 AND p.space_id=$2 AND (p.trashed_at IS NOT NULL) = $4 ORDER BY p.position, p.created_at`, a.Org, spaceID, a.User, trashed)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Page{}
	for rows.Next() {
		p := Page{SpaceID: spaceID, Role: role}
		if err := rows.Scan(&p.ID, &p.ParentID, &p.Title, &p.Icon, &p.Kind, &p.Position, &p.Version, &p.UpdatedAt, &p.TrashedAt, &p.RecordsTableID, &p.Favorite); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *Repository) loadPage(ctx context.Context, a knowkit.Actor, id string, need int, withContent bool) (*Page, error) {
	if !knowkit.IsUUID(id) {
		return nil, knowkit.ErrNotFound
	}
	p := &Page{ID: id}
	var content []byte
	err := r.pool.QueryRow(ctx, `SELECT space_id::text, COALESCE(parent_id::text,''), title, icon, kind, position, version, content, COALESCE(records_base_id::text,''), COALESCE(records_table_id::text,''), updated_at, trashed_at,
		COALESCE((SELECT COALESCE(NULLIF(display_name,''),email) FROM adrient.users WHERE id=updated_by),'')
		FROM adrient.wiki_pages WHERE org_id=$1 AND id=$2`, a.Org, id).
		Scan(&p.SpaceID, &p.ParentID, &p.Title, &p.Icon, &p.Kind, &p.Position, &p.Version, &content, &p.RecordsBaseID, &p.RecordsTableID, &p.UpdatedAt, &p.TrashedAt, &p.UpdatedBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, knowkit.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	role, err := r.needSpace(ctx, a, p.SpaceID, need)
	if err != nil {
		return nil, err
	}
	p.Role = role
	if withContent {
		p.Content = content
	}
	return p, nil
}

// GetPage returns a page with content.
func (r *Repository) GetPage(ctx context.Context, a knowkit.Actor, id string) (*Page, error) {
	p, err := r.loadPage(ctx, a, id, RoleView, true)
	if err != nil {
		return nil, err
	}
	_ = r.pool.QueryRow(ctx, `SELECT true FROM adrient.wiki_favorites WHERE user_id=$1 AND page_id=$2`, a.User, id).Scan(&p.Favorite)
	return p, nil
}

// CreatePage adds a page (or a database page) at the end of its parent.
func (r *Repository) CreatePage(ctx context.Context, a knowkit.Actor, in PageInput) (*Page, error) {
	if _, err := r.needSpace(ctx, a, in.SpaceID, RoleEdit); err != nil {
		return nil, err
	}
	in.Title = strings.TrimSpace(in.Title)
	if utf8.RuneCountInString(in.Title) > 200 {
		return nil, knowkit.Invalid("the title is too long")
	}
	if in.Kind == "" {
		in.Kind = "page"
		if in.Template == "task_board" {
			in.Kind = "database"
		}
	}
	if !knowkit.OneOf(in.Kind, "page", "database") {
		return nil, knowkit.Invalid("kind must be page or database")
	}
	if in.ParentID != "" {
		par, err := r.loadPage(ctx, a, in.ParentID, RoleEdit, false)
		if err != nil {
			return nil, err
		}
		if par.SpaceID != in.SpaceID || par.TrashedAt != nil {
			return nil, knowkit.Invalid("the parent page is not in this space")
		}
		if par.Kind == "database" {
			return nil, knowkit.Invalid("a database page cannot have sub-pages; add rows to it instead")
		}
	}
	content, title, icon, err := templateContent(in.Template)
	if err != nil {
		return nil, err
	}
	if in.Title == "" {
		in.Title = title
	}
	if in.Icon == "" {
		in.Icon = icon
	}
	if in.Kind == "database" {
		content = emptyDoc
	}
	text := extractText([]byte(content))
	var parent any
	if in.ParentID != "" {
		parent = in.ParentID
	}
	p := &Page{SpaceID: in.SpaceID, ParentID: in.ParentID, Title: in.Title, Icon: in.Icon, Kind: in.Kind, Version: 1, Role: RoleEdit}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if err := tx.QueryRow(ctx, `INSERT INTO adrient.wiki_pages (org_id, space_id, parent_id, title, icon, kind, content, text_content, position, created_by, updated_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,(SELECT COALESCE(max(position),0)+1 FROM adrient.wiki_pages WHERE space_id=$2 AND parent_id IS NOT DISTINCT FROM $3::uuid),$9,$9)
		RETURNING id::text, position, updated_at`, a.Org, in.SpaceID, parent, in.Title, in.Icon, in.Kind, []byte(content), text, a.User).Scan(&p.ID, &p.Position, &p.UpdatedAt); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	if in.Kind == "database" {
		if r.Deps.Records == nil {
			_, _ = r.pool.Exec(ctx, `DELETE FROM adrient.wiki_pages WHERE id=$1`, p.ID)
			return nil, knowkit.ErrUnavailable
		}
		baseID, tableID, err := r.Deps.Records.CreateEmbedded(ctx, a, in.Title, "wiki", in.SpaceID, in.Template == "task_board")
		if err != nil {
			_, _ = r.pool.Exec(ctx, `DELETE FROM adrient.wiki_pages WHERE id=$1`, p.ID)
			return nil, err
		}
		if _, err := r.pool.Exec(ctx, `UPDATE adrient.wiki_pages SET records_base_id=$2, records_table_id=$3 WHERE id=$1`, p.ID, baseID, tableID); err != nil {
			return nil, err
		}
		p.RecordsBaseID, p.RecordsTableID = baseID, tableID
	}
	_ = r.indexLinks(ctx, a.Org, p.ID, []byte(content))
	r.audit(ctx, "page.create", "wiki_page", p.ID)
	return p, nil
}

// PageUpdate changes a page. Content is saved only when Version matches the stored one.
type PageUpdate struct {
	Title   *string         `json:"title"`
	Icon    *string         `json:"icon"`
	Content json.RawMessage `json:"content"`
	Version int             `json:"version"`
}

// ErrConflict is returned when the page changed since the editor loaded it.
var ErrConflict = knowkit.ErrConflict

// UpdatePage saves title, icon and content. A snapshot of the previous content is kept at most every 5 minutes.
func (r *Repository) UpdatePage(ctx context.Context, a knowkit.Actor, id string, in PageUpdate) (*Page, error) {
	p, err := r.loadPage(ctx, a, id, RoleEdit, true)
	if err != nil {
		return nil, err
	}
	if p.TrashedAt != nil {
		return nil, knowkit.Invalid("this page is in the trash")
	}
	title, icon := p.Title, p.Icon
	if in.Title != nil {
		title = strings.TrimSpace(*in.Title)
		if utf8.RuneCountInString(title) > 200 {
			return nil, knowkit.Invalid("the title is too long")
		}
	}
	if in.Icon != nil {
		icon = *in.Icon
		if utf8.RuneCountInString(icon) > 8 {
			return nil, knowkit.Invalid("the icon is too long")
		}
	}
	content := []byte(p.Content)
	text := ""
	contentChanged := false
	if len(in.Content) > 0 {
		if len(in.Content) > MaxPageBytes {
			return nil, knowkit.Invalid("the page is larger than %d MB", MaxPageBytes>>20)
		}
		var probe struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(in.Content, &probe) != nil || probe.Type != "doc" {
			return nil, knowkit.Invalid("content must be a document")
		}
		if in.Version != p.Version {
			return nil, fmt.Errorf("%w: this page was changed by someone else; reload it", ErrConflict)
		}
		content, contentChanged = in.Content, string(in.Content) != string(p.Content)
		text = extractText(content)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if contentChanged {
		var recent bool
		_ = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM adrient.wiki_page_versions WHERE page_id=$1 AND saved_at > now() - interval '5 minutes')`, id).Scan(&recent)
		if !recent {
			if _, err := tx.Exec(ctx, `INSERT INTO adrient.wiki_page_versions (org_id, page_id, title, content, saved_by) VALUES ($1,$2,$3,$4,$5)`, a.Org, id, p.Title, []byte(p.Content), a.User); err != nil {
				return nil, err
			}
			_, _ = tx.Exec(ctx, `DELETE FROM adrient.wiki_page_versions WHERE page_id=$1 AND id NOT IN (SELECT id FROM adrient.wiki_page_versions WHERE page_id=$1 ORDER BY saved_at DESC LIMIT 50)`, id)
		}
		if _, err := tx.Exec(ctx, `UPDATE adrient.wiki_pages SET title=$3, icon=$4, content=$5, text_content=$6, version=version+1, updated_by=$7, updated_at=now() WHERE org_id=$1 AND id=$2`,
			a.Org, id, title, icon, content, text, a.User); err != nil {
			return nil, err
		}
	} else if _, err := tx.Exec(ctx, `UPDATE adrient.wiki_pages SET title=$3, icon=$4, updated_by=$5, updated_at=now() WHERE org_id=$1 AND id=$2`, a.Org, id, title, icon, a.User); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	if contentChanged {
		_ = r.indexLinks(ctx, a.Org, id, content)
	}
	return r.GetPage(ctx, a, id)
}

// indexLinks records which pages a page links to (for backlinks).
func (r *Repository) indexLinks(ctx context.Context, org, pageID string, content []byte) error {
	ids := extractPageLinks(content)
	if _, err := r.pool.Exec(ctx, `DELETE FROM adrient.wiki_page_links WHERE page_id=$1`, pageID); err != nil {
		return err
	}
	for _, t := range ids {
		if t == pageID || !knowkit.IsUUID(t) {
			continue
		}
		_, _ = r.pool.Exec(ctx, `INSERT INTO adrient.wiki_page_links (org_id, page_id, target_page_id) SELECT $1,$2,id FROM adrient.wiki_pages WHERE id=$3 AND org_id=$1 ON CONFLICT DO NOTHING`, org, pageID, t)
	}
	return nil
}

// MovePage changes a page's parent (and space) and position (a sibling index).
func (r *Repository) MovePage(ctx context.Context, a knowkit.Actor, id, parentID, spaceID string, index int) error {
	p, err := r.loadPage(ctx, a, id, RoleEdit, false)
	if err != nil {
		return err
	}
	if spaceID == "" {
		spaceID = p.SpaceID
	}
	if spaceID != p.SpaceID {
		if _, err := r.needSpace(ctx, a, spaceID, RoleEdit); err != nil {
			return err
		}
	}
	if parentID != "" {
		par, err := r.loadPage(ctx, a, parentID, RoleEdit, false)
		if err != nil {
			return err
		}
		if par.SpaceID != spaceID || par.Kind == "database" || par.TrashedAt != nil {
			return knowkit.Invalid("that page cannot hold sub-pages here")
		}
		// No cycles: the new parent must not be this page or one of its descendants.
		var cyc bool
		if err := r.pool.QueryRow(ctx, `WITH RECURSIVE d AS (SELECT id FROM adrient.wiki_pages WHERE id=$1 UNION ALL SELECT c.id FROM adrient.wiki_pages c JOIN d ON c.parent_id=d.id)
			SELECT EXISTS(SELECT 1 FROM d WHERE id=$2)`, id, parentID).Scan(&cyc); err != nil {
			return err
		}
		if cyc {
			return knowkit.Invalid("a page cannot move into itself")
		}
	}
	var parent any
	if parentID != "" {
		parent = parentID
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	rows, err := tx.Query(ctx, `SELECT id::text FROM adrient.wiki_pages WHERE space_id=$1 AND parent_id IS NOT DISTINCT FROM $2::uuid AND id<>$3 AND trashed_at IS NULL ORDER BY position, created_at`, spaceID, parent, id)
	if err != nil {
		return err
	}
	var sibs []string
	for rows.Next() {
		var s string
		_ = rows.Scan(&s)
		sibs = append(sibs, s)
	}
	rows.Close()
	index = min(max(index, 0), len(sibs))
	sibs = append(sibs[:index], append([]string{id}, sibs[index:]...)...)
	if _, err := tx.Exec(ctx, `UPDATE adrient.wiki_pages SET parent_id=$2, space_id=$3 WHERE id=$1`, id, parent, spaceID); err != nil {
		return err
	}
	if spaceID != p.SpaceID { // descendants and embedded databases follow
		if _, err := tx.Exec(ctx, `WITH RECURSIVE d AS (SELECT id FROM adrient.wiki_pages WHERE parent_id=$1 UNION ALL SELECT c.id FROM adrient.wiki_pages c JOIN d ON c.parent_id=d.id)
			UPDATE adrient.wiki_pages SET space_id=$2 WHERE id IN (SELECT id FROM d)`, id, spaceID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE adrient.records_bases SET source_ref=$2 WHERE org_id=$3 AND source='wiki' AND id IN (SELECT records_base_id FROM adrient.wiki_pages WHERE space_id=$2)`, id, spaceID, a.Org); err != nil {
			return err
		}
	}
	for i, s := range sibs {
		if _, err := tx.Exec(ctx, `UPDATE adrient.wiki_pages SET position=$2 WHERE id=$1`, s, float64(i+1)); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// TrashPage moves a page and its sub-pages to the trash, or brings them back.
func (r *Repository) TrashPage(ctx context.Context, a knowkit.Actor, id string, trash bool) error {
	if _, err := r.loadPage(ctx, a, id, RoleEdit, false); err != nil {
		return err
	}
	set := "now()"
	if !trash {
		set = "NULL"
	}
	_, err := r.pool.Exec(ctx, `WITH RECURSIVE d AS (SELECT id FROM adrient.wiki_pages WHERE id=$1 UNION ALL SELECT c.id FROM adrient.wiki_pages c JOIN d ON c.parent_id=d.id)
		UPDATE adrient.wiki_pages SET trashed_at=`+set+` WHERE id IN (SELECT id FROM d)`, id)
	if err == nil && !trash {
		// A restored page whose parent is still in the trash moves to the top level.
		_, _ = r.pool.Exec(ctx, `UPDATE adrient.wiki_pages SET parent_id=NULL WHERE id=$1 AND parent_id IN (SELECT id FROM adrient.wiki_pages WHERE trashed_at IS NOT NULL)`, id)
	}
	r.audit(ctx, map[bool]string{true: "page.trash", false: "page.restore"}[trash], "wiki_page", id)
	return err
}

// DeletePage deletes a trashed page and everything below it, with embedded databases.
func (r *Repository) DeletePage(ctx context.Context, a knowkit.Actor, id string) error {
	p, err := r.loadPage(ctx, a, id, RoleEdit, false)
	if err != nil {
		return err
	}
	if p.TrashedAt == nil {
		return knowkit.Invalid("move the page to the trash first")
	}
	_, _ = r.pool.Exec(ctx, `WITH RECURSIVE d AS (SELECT id, records_base_id FROM adrient.wiki_pages WHERE id=$1 UNION ALL SELECT c.id, c.records_base_id FROM adrient.wiki_pages c JOIN d ON c.parent_id=d.id)
		DELETE FROM adrient.records_bases WHERE org_id=$2 AND source='wiki' AND id IN (SELECT records_base_id FROM d WHERE records_base_id IS NOT NULL)`, id, a.Org)
	_, err = r.pool.Exec(ctx, `DELETE FROM adrient.wiki_pages WHERE org_id=$1 AND id=$2`, a.Org, id)
	r.audit(ctx, "page.delete", "wiki_page", id)
	return err
}

// DuplicatePage copies a page (content only; a database page copies its schema and rows through a records backup).
func (r *Repository) DuplicatePage(ctx context.Context, a knowkit.Actor, id string) (*Page, error) {
	p, err := r.loadPage(ctx, a, id, RoleEdit, true)
	if err != nil {
		return nil, err
	}
	np, err := r.CreatePage(ctx, a, PageInput{SpaceID: p.SpaceID, ParentID: p.ParentID, Title: p.Title + " (copy)", Icon: p.Icon, Kind: p.Kind})
	if err != nil {
		return nil, err
	}
	if p.Kind == "page" {
		return r.UpdatePage(ctx, a, np.ID, PageUpdate{Content: p.Content, Version: np.Version})
	}
	return np, nil
}

// ---- favourites, backlinks, search, versions, comments ---------------------

// SetFavorite stars or unstars a page for the caller.
func (r *Repository) SetFavorite(ctx context.Context, a knowkit.Actor, id string, on bool) error {
	if _, err := r.loadPage(ctx, a, id, RoleView, false); err != nil {
		return err
	}
	var err error
	if on {
		_, err = r.pool.Exec(ctx, `INSERT INTO adrient.wiki_favorites (user_id, page_id, org_id) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`, a.User, id, a.Org)
	} else {
		_, err = r.pool.Exec(ctx, `DELETE FROM adrient.wiki_favorites WHERE user_id=$1 AND page_id=$2`, a.User, id)
	}
	return err
}

// PageRef is a short page reference for lists.
type PageRef struct {
	ID        string    `json:"id"`
	SpaceID   string    `json:"space_id"`
	SpaceName string    `json:"space_name"`
	Title     string    `json:"title"`
	Icon      string    `json:"icon"`
	Snippet   string    `json:"snippet,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (r *Repository) visible(ctx context.Context, a knowkit.Actor, rows pgx.Rows) ([]PageRef, error) {
	defer rows.Close()
	roles := map[string]int{}
	out := []PageRef{}
	for rows.Next() {
		var p PageRef
		if err := rows.Scan(&p.ID, &p.SpaceID, &p.SpaceName, &p.Title, &p.Icon, &p.Snippet, &p.UpdatedAt); err != nil {
			return nil, err
		}
		role, ok := roles[p.SpaceID]
		if !ok {
			role = r.SpaceRole(ctx, a.Org, a.User, a.Admin, p.SpaceID)
			roles[p.SpaceID] = role
		}
		if role > RoleNone {
			out = append(out, p)
		}
	}
	return out, rows.Err()
}

// Favorites lists the caller's starred pages.
func (r *Repository) Favorites(ctx context.Context, a knowkit.Actor) ([]PageRef, error) {
	rows, err := r.pool.Query(ctx, `SELECT p.id::text, p.space_id::text, s.name, p.title, p.icon, '', p.updated_at FROM adrient.wiki_favorites f JOIN adrient.wiki_pages p ON p.id=f.page_id JOIN adrient.wiki_spaces s ON s.id=p.space_id
		WHERE f.user_id=$1 AND p.org_id=$2 AND p.trashed_at IS NULL ORDER BY p.title`, a.User, a.Org)
	if err != nil {
		return nil, err
	}
	return r.visible(ctx, a, rows)
}

// Recent lists the pages edited most recently in spaces the caller can open.
func (r *Repository) Recent(ctx context.Context, a knowkit.Actor) ([]PageRef, error) {
	rows, err := r.pool.Query(ctx, `SELECT p.id::text, p.space_id::text, s.name, p.title, p.icon, '', p.updated_at FROM adrient.wiki_pages p JOIN adrient.wiki_spaces s ON s.id=p.space_id
		WHERE p.org_id=$1 AND p.trashed_at IS NULL AND (s.kind='team' OR s.owner_id=$2) ORDER BY p.updated_at DESC LIMIT 60`, a.Org, a.User)
	if err != nil {
		return nil, err
	}
	out, err := r.visible(ctx, a, rows)
	if len(out) > 12 {
		out = out[:12]
	}
	return out, err
}

// Backlinks lists the pages that link to a page.
func (r *Repository) Backlinks(ctx context.Context, a knowkit.Actor, id string) ([]PageRef, error) {
	if _, err := r.loadPage(ctx, a, id, RoleView, false); err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, `SELECT p.id::text, p.space_id::text, s.name, p.title, p.icon, '', p.updated_at FROM adrient.wiki_page_links l JOIN adrient.wiki_pages p ON p.id=l.page_id JOIN adrient.wiki_spaces s ON s.id=p.space_id
		WHERE l.target_page_id=$1 AND p.org_id=$2 AND p.trashed_at IS NULL ORDER BY p.title`, id, a.Org)
	if err != nil {
		return nil, err
	}
	return r.visible(ctx, a, rows)
}

// Search finds pages by title and text across the spaces the caller can open.
func (r *Repository) Search(ctx context.Context, a knowkit.Actor, q string) ([]PageRef, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return []PageRef{}, nil
	}
	if len(q) > 200 {
		q = q[:200]
	}
	rows, err := r.pool.Query(ctx, `SELECT p.id::text, p.space_id::text, s.name, p.title, p.icon,
		ts_headline('simple', p.text_content, plainto_tsquery('simple',$2), 'MaxFragments=1,MaxWords=18,MinWords=6,StartSel=«,StopSel=»'), p.updated_at
		FROM adrient.wiki_pages p JOIN adrient.wiki_spaces s ON s.id=p.space_id
		WHERE p.org_id=$1 AND p.trashed_at IS NULL AND (s.kind='team' OR s.owner_id=$3)
		AND (to_tsvector('simple', p.title || ' ' || p.text_content) @@ plainto_tsquery('simple',$2) OR p.title ILIKE '%' || $4 || '%')
		ORDER BY (p.title ILIKE '%' || $4 || '%') DESC, ts_rank(to_tsvector('simple', p.title || ' ' || p.text_content), plainto_tsquery('simple',$2)) DESC LIMIT 40`,
		a.Org, q, a.User, strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q))
	if err != nil {
		return nil, err
	}
	return r.visible(ctx, a, rows)
}

// Version is a saved snapshot of a page.
type Version struct {
	ID      string    `json:"id"`
	Title   string    `json:"title"`
	SavedAt time.Time `json:"saved_at"`
	SavedBy string    `json:"saved_by"`
}

// Versions lists a page's snapshots.
func (r *Repository) Versions(ctx context.Context, a knowkit.Actor, id string) ([]Version, error) {
	if _, err := r.loadPage(ctx, a, id, RoleView, false); err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, `SELECT v.id::text, v.title, v.saved_at, COALESCE(NULLIF(u.display_name,''),u.email,'') FROM adrient.wiki_page_versions v LEFT JOIN adrient.users u ON u.id=v.saved_by WHERE v.page_id=$1 ORDER BY v.saved_at DESC`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Version{}
	for rows.Next() {
		var v Version
		if err := rows.Scan(&v.ID, &v.Title, &v.SavedAt, &v.SavedBy); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// VersionContent returns a snapshot's content.
func (r *Repository) VersionContent(ctx context.Context, a knowkit.Actor, pageID, versionID string) (json.RawMessage, error) {
	if _, err := r.loadPage(ctx, a, pageID, RoleView, false); err != nil {
		return nil, err
	}
	if !knowkit.IsUUID(versionID) {
		return nil, knowkit.ErrNotFound
	}
	var raw []byte
	if err := r.pool.QueryRow(ctx, `SELECT content FROM adrient.wiki_page_versions WHERE page_id=$1 AND id=$2`, pageID, versionID).Scan(&raw); err != nil {
		return nil, knowkit.ErrNotFound
	}
	return raw, nil
}

// RestoreVersion puts a snapshot back as the page content (the current content is snapshotted first).
func (r *Repository) RestoreVersion(ctx context.Context, a knowkit.Actor, pageID, versionID string) (*Page, error) {
	raw, err := r.VersionContent(ctx, a, pageID, versionID)
	if err != nil {
		return nil, err
	}
	p, err := r.loadPage(ctx, a, pageID, RoleEdit, true)
	if err != nil {
		return nil, err
	}
	if _, err := r.pool.Exec(ctx, `INSERT INTO adrient.wiki_page_versions (org_id, page_id, title, content, saved_by) VALUES ($1,$2,$3,$4,$5)`, a.Org, pageID, p.Title, []byte(p.Content), a.User); err != nil {
		return nil, err
	}
	return r.UpdatePage(ctx, a, pageID, PageUpdate{Content: raw, Version: p.Version})
}

// Comment is a note on a page.
type Comment struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Author    string    `json:"author"`
	Body      string    `json:"body"`
	Resolved  bool      `json:"resolved"`
	CreatedAt time.Time `json:"created_at"`
}

// Comments lists a page's comments.
func (r *Repository) Comments(ctx context.Context, a knowkit.Actor, pageID string) ([]Comment, error) {
	if _, err := r.loadPage(ctx, a, pageID, RoleView, false); err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, `SELECT c.id::text, c.user_id::text, COALESCE(NULLIF(u.display_name,''),u.email,''), c.body, c.resolved, c.created_at FROM adrient.wiki_comments c LEFT JOIN adrient.users u ON u.id=c.user_id WHERE c.page_id=$1 ORDER BY c.created_at`, pageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Comment{}
	for rows.Next() {
		var c Comment
		if err := rows.Scan(&c.ID, &c.UserID, &c.Author, &c.Body, &c.Resolved, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// AddComment comments on a page; viewers may comment.
func (r *Repository) AddComment(ctx context.Context, a knowkit.Actor, pageID, body string) (*Comment, error) {
	if _, err := r.loadPage(ctx, a, pageID, RoleView, false); err != nil {
		return nil, err
	}
	body = strings.TrimSpace(body)
	if body == "" || len(body) > 5000 {
		return nil, knowkit.Invalid("a comment needs 1 to 5000 characters")
	}
	c := &Comment{UserID: a.User, Body: body}
	err := r.pool.QueryRow(ctx, `INSERT INTO adrient.wiki_comments (org_id, page_id, user_id, body) VALUES ($1,$2,$3,$4) RETURNING id::text, created_at`, a.Org, pageID, a.User, body).Scan(&c.ID, &c.CreatedAt)
	return c, err
}

// ResolveComment marks a comment resolved or open (editors, or the author).
func (r *Repository) ResolveComment(ctx context.Context, a knowkit.Actor, commentID string, resolved bool) error {
	if !knowkit.IsUUID(commentID) {
		return knowkit.ErrNotFound
	}
	var pageID, author string
	if err := r.pool.QueryRow(ctx, `SELECT page_id::text, user_id::text FROM adrient.wiki_comments WHERE org_id=$1 AND id=$2`, a.Org, commentID).Scan(&pageID, &author); err != nil {
		return knowkit.ErrNotFound
	}
	need := RoleEdit
	if author == a.User {
		need = RoleView
	}
	if _, err := r.loadPage(ctx, a, pageID, need, false); err != nil {
		return err
	}
	_, err := r.pool.Exec(ctx, `UPDATE adrient.wiki_comments SET resolved=$2 WHERE id=$1`, commentID, resolved)
	return err
}

// DeleteComment removes a comment (its author or an editor).
func (r *Repository) DeleteComment(ctx context.Context, a knowkit.Actor, commentID string) error {
	if !knowkit.IsUUID(commentID) {
		return knowkit.ErrNotFound
	}
	var pageID, author string
	if err := r.pool.QueryRow(ctx, `SELECT page_id::text, user_id::text FROM adrient.wiki_comments WHERE org_id=$1 AND id=$2`, a.Org, commentID).Scan(&pageID, &author); err != nil {
		return knowkit.ErrNotFound
	}
	need := RoleEdit
	if author == a.User {
		need = RoleView
	}
	if _, err := r.loadPage(ctx, a, pageID, need, false); err != nil {
		return err
	}
	_, err := r.pool.Exec(ctx, `DELETE FROM adrient.wiki_comments WHERE id=$1`, commentID)
	return err
}
