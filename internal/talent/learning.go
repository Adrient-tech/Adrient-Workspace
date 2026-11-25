package talent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// SafeLink accepts in-app paths and http(s) URLs only (never javascript: or data:).
func SafeLink(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	if strings.HasPrefix(s, "/") && !strings.HasPrefix(s, "//") {
		return s, nil
	}
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", invalid("links must start with / or http(s)://")
	}
	return s, nil
}

// ---------- courses ----------

// CourseInput creates or updates a course.
type CourseInput struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Link        string `json:"link"`
	DurationMin int    `json:"duration_min"`
	Mandatory   bool   `json:"mandatory"`
	Active      *bool  `json:"active"`
}

// SaveCourse creates (id=="") or updates a course. HR only.
func (r *Repository) SaveCourse(ctx context.Context, a Actor, id string, in CourseInput) (json.RawMessage, error) {
	if !a.IsHR {
		return nil, ErrForbidden
	}
	in.Title = clean(in.Title)
	if in.Title == "" || len(in.Title) > 200 {
		return nil, invalid("title is required (max 200 characters)")
	}
	link, err := SafeLink(in.Link)
	if err != nil {
		return nil, err
	}
	if in.DurationMin < 0 || in.DurationMin > 100000 {
		return nil, invalid("duration is out of range")
	}
	active := true
	if in.Active != nil {
		active = *in.Active
	}
	var cid string
	if id == "" {
		err = r.pool.QueryRow(ctx, `INSERT INTO adrient.talent_courses (org_id, title, description, category, link, duration_min, mandatory, active, created_by)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id::text`, a.OrgID, in.Title, in.Description, clean(in.Category), link, in.DurationMin, in.Mandatory, active, a.UserID).Scan(&cid)
	} else {
		err = r.pool.QueryRow(ctx, `UPDATE adrient.talent_courses SET title=$3, description=$4, category=$5, link=$6, duration_min=$7, mandatory=$8, active=$9
			WHERE org_id=$1 AND id=$2 RETURNING id::text`, a.OrgID, id, in.Title, in.Description, clean(in.Category), link, in.DurationMin, in.Mandatory, active).Scan(&cid)
	}
	if err != nil {
		return nil, mapNoRows(err)
	}
	return r.row(ctx, `SELECT id::text AS id, title, description, category, link, duration_min, mandatory, active FROM adrient.talent_courses WHERE id=$1`, cid)
}

// ListCourses lists the catalogue with the caller's own enrollment; HR also gets totals.
func (r *Repository) ListCourses(ctx context.Context, a Actor) (json.RawMessage, error) {
	return r.rows(ctx, `SELECT c.id::text AS id, c.title, c.description, c.category, c.link, c.duration_min, c.mandatory, c.active,
		e.status AS my_status, e.due_date AS my_due_date, e.completed_at AS my_completed_at,
		CASE WHEN $3::boolean THEN (SELECT count(*) FROM adrient.talent_enrollments x WHERE x.course_id=c.id) END AS enrolled,
		CASE WHEN $3::boolean THEN (SELECT count(*) FROM adrient.talent_enrollments x WHERE x.course_id=c.id AND x.status='completed') END AS completed
		FROM adrient.talent_courses c LEFT JOIN adrient.talent_enrollments e ON e.course_id=c.id AND e.user_id=$2::uuid
		WHERE c.org_id=$1 AND ($3::boolean OR c.active) ORDER BY c.mandatory DESC, c.title`, a.OrgID, a.UserID, a.IsHR)
}

// Enroll enrolls the caller (self-service) or, for HR, the given users or everyone.
func (r *Repository) Enroll(ctx context.Context, a Actor, courseID string, users []string, everyone bool, due string) (int, error) {
	var active bool
	if err := r.pool.QueryRow(ctx, `SELECT active FROM adrient.talent_courses WHERE org_id=$1 AND id=$2`, a.OrgID, courseID).Scan(&active); err != nil {
		return 0, mapNoRows(err)
	}
	if !active && !a.IsHR {
		return 0, ErrNotFound
	}
	var dd any
	if due != "" {
		t, err := time.Parse("2006-01-02", due)
		if err != nil {
			return 0, invalid("due date must be YYYY-MM-DD")
		}
		dd = t
	}
	if len(users) == 0 && !everyone {
		users = []string{a.UserID}
	}
	if (everyone || len(users) > 1 || (len(users) == 1 && users[0] != a.UserID)) && !a.IsHR {
		return 0, ErrForbidden
	}
	var ct int64
	if everyone {
		tag, err := r.pool.Exec(ctx, `INSERT INTO adrient.talent_enrollments (org_id, course_id, user_id, due_date, enrolled_by)
			SELECT org_id, $2, id, $3, $4 FROM adrient.users WHERE org_id=$1 AND COALESCE(status,'active')='active' AND user_type='member' ON CONFLICT DO NOTHING`, a.OrgID, courseID, dd, a.UserID)
		if err != nil {
			return 0, err
		}
		return int(tag.RowsAffected()), nil
	}
	for _, u := range uuidList(users) {
		if _, err := r.userBrief(ctx, a.OrgID, u); err != nil {
			return 0, invalid("a person is not a member of this organisation")
		}
		tag, err := r.pool.Exec(ctx, `INSERT INTO adrient.talent_enrollments (org_id, course_id, user_id, due_date, enrolled_by) VALUES ($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING`,
			a.OrgID, courseID, u, dd, a.UserID)
		if err != nil {
			return 0, err
		}
		ct += tag.RowsAffected()
	}
	return int(ct), nil
}

// SetProgress lets people mark their own enrollment in progress or completed.
func (r *Repository) SetProgress(ctx context.Context, a Actor, courseID, status string) error {
	if !oneOf(status, "enrolled", "in_progress", "completed") {
		return invalid("status must be enrolled, in_progress or completed")
	}
	ct, err := r.pool.Exec(ctx, `UPDATE adrient.talent_enrollments SET status=$4, completed_at=CASE WHEN $4='completed' THEN now() END
		WHERE org_id=$1 AND course_id=$2 AND user_id=$3`, a.OrgID, courseID, a.UserID, status)
	if err == nil && ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// CourseTracking lists everyone's progress on a course. HR only.
func (r *Repository) CourseTracking(ctx context.Context, a Actor, courseID string) (json.RawMessage, error) {
	if !a.IsHR {
		return nil, ErrForbidden
	}
	return r.rows(ctx, `SELECT e.user_id::text AS user_id, COALESCE(NULLIF(u.display_name,''),u.email) AS name, e.status, e.due_date, e.enrolled_at, e.completed_at,
		(e.status<>'completed' AND e.due_date < current_date) AS overdue
		FROM adrient.talent_enrollments e JOIN adrient.users u ON u.id=e.user_id WHERE e.org_id=$1 AND e.course_id=$2 ORDER BY name`, a.OrgID, courseID)
}

// ---------- policies ----------

// PolicyInput creates or updates a policy.
type PolicyInput struct {
	Title      string   `json:"title"`
	Summary    string   `json:"summary"`
	Link       string   `json:"link"`
	Mandatory  bool     `json:"mandatory"`
	Audience   []string `json:"audience"`
	NewVersion bool     `json:"new_version"`
}

// SavePolicy creates (id=="") or updates a policy. NewVersion asks everyone to acknowledge again.
func (r *Repository) SavePolicy(ctx context.Context, a Actor, id string, in PolicyInput) (json.RawMessage, error) {
	if !a.IsHR {
		return nil, ErrForbidden
	}
	in.Title = clean(in.Title)
	if in.Title == "" {
		return nil, invalid("title is required")
	}
	link, err := SafeLink(in.Link)
	if err != nil {
		return nil, err
	}
	if link == "" {
		return nil, invalid("link the policy document (from Docs or Drive)")
	}
	aud := uuidList(in.Audience)
	var pid string
	if id == "" {
		err = r.pool.QueryRow(ctx, `INSERT INTO adrient.talent_policies (org_id, title, summary, link, mandatory, audience, created_by) VALUES ($1,$2,$3,$4,$5,$6::uuid[],$7) RETURNING id::text`,
			a.OrgID, in.Title, in.Summary, link, in.Mandatory, aud, a.UserID).Scan(&pid)
	} else {
		err = r.pool.QueryRow(ctx, `UPDATE adrient.talent_policies SET title=$3, summary=$4, link=$5, mandatory=$6, audience=$7::uuid[], version=version+CASE WHEN $8 THEN 1 ELSE 0 END
			WHERE org_id=$1 AND id=$2 RETURNING id::text`, a.OrgID, id, in.Title, in.Summary, link, in.Mandatory, aud, in.NewVersion).Scan(&pid)
	}
	if err != nil {
		return nil, mapNoRows(err)
	}
	return r.row(ctx, `SELECT id::text AS id, title, summary, link, version, mandatory, COALESCE(audience::text[],'{}') AS audience FROM adrient.talent_policies WHERE id=$1`, pid)
}

// ListPolicies lists the policies that apply to the caller and whether they acknowledged the current version.
func (r *Repository) ListPolicies(ctx context.Context, a Actor) (json.RawMessage, error) {
	return r.rows(ctx, `SELECT p.id::text AS id, p.title, p.summary, p.link, p.version, p.mandatory,
		EXISTS(SELECT 1 FROM adrient.talent_policy_acks k WHERE k.policy_id=p.id AND k.user_id=$2::uuid AND k.version=p.version) AS acknowledged,
		(SELECT k.acked_at FROM adrient.talent_policy_acks k WHERE k.policy_id=p.id AND k.user_id=$2::uuid AND k.version=p.version) AS acked_at,
		CASE WHEN $3::boolean THEN COALESCE(p.audience::text[],'{}') END AS audience
		FROM adrient.talent_policies p WHERE p.org_id=$1 AND p.active AND ($3::boolean OR cardinality(p.audience)=0 OR $2::uuid = ANY(p.audience))
		ORDER BY p.mandatory DESC, p.title`, a.OrgID, a.UserID, a.IsHR)
}

// Acknowledge records the caller's acknowledgement of the current version.
func (r *Repository) Acknowledge(ctx context.Context, a Actor, policyID string) error {
	var ver int
	err := r.pool.QueryRow(ctx, `SELECT version FROM adrient.talent_policies WHERE org_id=$1 AND id=$2 AND active
		AND (cardinality(audience)=0 OR $3::uuid = ANY(audience))`, a.OrgID, policyID, a.UserID).Scan(&ver)
	if err != nil {
		return mapNoRows(err)
	}
	_, err = r.pool.Exec(ctx, `INSERT INTO adrient.talent_policy_acks (policy_id, org_id, user_id, version) VALUES ($1,$2,$3,$4) ON CONFLICT DO NOTHING`, policyID, a.OrgID, a.UserID, ver)
	return err
}

// PolicyTracking lists who has and has not acknowledged the current version. HR only.
func (r *Repository) PolicyTracking(ctx context.Context, a Actor, policyID string) (json.RawMessage, error) {
	if !a.IsHR {
		return nil, ErrForbidden
	}
	return r.rows(ctx, `SELECT u.id::text AS user_id, COALESCE(NULLIF(u.display_name,''),u.email) AS name, u.email,
		k.acked_at, (k.acked_at IS NOT NULL) AS acknowledged
		FROM adrient.talent_policies p JOIN adrient.users u ON u.org_id=p.org_id AND COALESCE(u.status,'active')='active' AND u.user_type='member'
			AND (cardinality(p.audience)=0 OR u.id = ANY(p.audience))
		LEFT JOIN adrient.talent_policy_acks k ON k.policy_id=p.id AND k.user_id=u.id AND k.version=p.version
		WHERE p.org_id=$1 AND p.id=$2 ORDER BY (k.acked_at IS NOT NULL), name`, a.OrgID, policyID)
}

// ---------- announcements, kudos, celebrations ----------

// AnnouncementInput posts a company announcement.
type AnnouncementInput struct {
	Title  string `json:"title"`
	Body   string `json:"body"`
	Pinned bool   `json:"pinned"`
}

// PostAnnouncement publishes an announcement. HR only.
func (r *Repository) PostAnnouncement(ctx context.Context, a Actor, in AnnouncementInput) (json.RawMessage, error) {
	if !a.IsHR {
		return nil, ErrForbidden
	}
	in.Title = clean(in.Title)
	if in.Title == "" || len(in.Title) > 200 || len(in.Body) > 10000 {
		return nil, invalid("a title (max 200) is required and the text is limited to 10000 characters")
	}
	var id string
	if err := r.pool.QueryRow(ctx, `INSERT INTO adrient.talent_announcements (org_id, title, body, pinned, author_id) VALUES ($1,$2,$3,$4,$5) RETURNING id::text`,
		a.OrgID, in.Title, in.Body, in.Pinned, a.UserID).Scan(&id); err != nil {
		return nil, err
	}
	return r.row(ctx, `SELECT id::text AS id, title, body, pinned, created_at FROM adrient.talent_announcements WHERE id=$1`, id)
}

// DeleteAnnouncement removes an announcement. HR only.
func (r *Repository) DeleteAnnouncement(ctx context.Context, a Actor, id string) error {
	if !a.IsHR {
		return ErrForbidden
	}
	ct, err := r.pool.Exec(ctx, `DELETE FROM adrient.talent_announcements WHERE org_id=$1 AND id=$2`, a.OrgID, id)
	if err == nil && ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// Feed returns announcements and kudos, newest first (the company-wide feed).
func (r *Repository) Feed(ctx context.Context, a Actor) (json.RawMessage, error) {
	return r.rows(ctx, `SELECT * FROM (
		SELECT 'announcement' AS type, n.id::text AS id, n.title AS title, n.body AS body, n.pinned AS pinned, n.created_at AS created_at,
			COALESCE(NULLIF(u.display_name,''),u.email,'') AS from_name, '' AS to_name, '' AS badge, ''::text AS from_id
		FROM adrient.talent_announcements n LEFT JOIN adrient.users u ON u.id=n.author_id WHERE n.org_id=$1
		UNION ALL
		SELECT 'kudos', k.id::text, '', k.message, false, k.created_at, COALESCE(NULLIF(f.display_name,''),f.email), COALESCE(NULLIF(t.display_name,''),t.email), k.badge, k.from_id::text
		FROM adrient.talent_kudos k JOIN adrient.users f ON f.id=k.from_id JOIN adrient.users t ON t.id=k.to_id WHERE k.org_id=$1
	) x ORDER BY pinned DESC, created_at DESC LIMIT 100`, a.OrgID)
}

var kudosBadges = []string{"thanks", "teamwork", "great_work", "innovation", "customer_hero", "above_beyond"}

// GiveKudos sends recognition to a colleague.
func (r *Repository) GiveKudos(ctx context.Context, a Actor, to, badge, message string) error {
	message = clean(message)
	if badge == "" {
		badge = "thanks"
	}
	switch {
	case to == "" || to == a.UserID:
		return invalid("choose a colleague (not yourself)")
	case !oneOf(badge, kudosBadges...):
		return invalid("unknown badge")
	case message == "" || len(message) > 500:
		return invalid("write a short message (max 500 characters)")
	}
	if _, err := r.userBrief(ctx, a.OrgID, to); err != nil {
		return invalid("that person is not a member of this organisation")
	}
	_, err := r.pool.Exec(ctx, `INSERT INTO adrient.talent_kudos (org_id, from_id, to_id, badge, message) VALUES ($1,$2,$3,$4,$5)`, a.OrgID, a.UserID, to, badge, message)
	return err
}

// DeleteKudos removes a kudos the caller sent (HR may remove any).
func (r *Repository) DeleteKudos(ctx context.Context, a Actor, id string) error {
	ct, err := r.pool.Exec(ctx, `DELETE FROM adrient.talent_kudos WHERE org_id=$1 AND id=$2 AND (from_id=$3::uuid OR $4::boolean)`, a.OrgID, id, a.UserID, a.IsHR)
	if err == nil && ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// SetBirthday stores the month and day of a birthday (own, or any by HR).
func (r *Repository) SetBirthday(ctx context.Context, a Actor, user string, month, day int) error {
	if user == "" {
		user = a.UserID
	}
	if user != a.UserID && !a.IsHR {
		return ErrForbidden
	}
	if month < 1 || month > 12 || day < 1 || day > 31 || (month == 2 && day > 29) || ((month == 4 || month == 6 || month == 9 || month == 11) && day > 30) {
		return invalid("that is not a valid date")
	}
	_, err := r.pool.Exec(ctx, `INSERT INTO adrient.talent_birthdays (org_id, user_id, month, day) VALUES ($1,$2,$3,$4)
		ON CONFLICT (org_id, user_id) DO UPDATE SET month=EXCLUDED.month, day=EXCLUDED.day`, a.OrgID, user, month, day)
	return err
}

// Celebrations lists birthdays and work anniversaries in the next `days` days.
func (r *Repository) Celebrations(ctx context.Context, a Actor, days int) (json.RawMessage, error) {
	if days <= 0 || days > 60 {
		days = 30
	}
	return r.rows(ctx, `SELECT * FROM (
		SELECT 'birthday' AS type, u.id::text AS user_id, COALESCE(NULLIF(u.display_name,''),u.email) AS name,
			make_date(extract(year FROM current_date)::int + CASE WHEN make_date(extract(year FROM current_date)::int, b.month, least(b.day, 28)) < current_date THEN 1 ELSE 0 END, b.month, least(b.day, 28)) AS on_date,
			0 AS years
		FROM adrient.talent_birthdays b JOIN adrient.users u ON u.id=b.user_id WHERE b.org_id=$1
		UNION ALL
		SELECT 'anniversary', u.id::text, COALESCE(NULLIF(u.display_name,''),u.email),
			(u.start_date + make_interval(years => (extract(year FROM age(current_date + 1, u.start_date))::int + CASE WHEN (u.start_date + make_interval(years => extract(year FROM age(current_date, u.start_date))::int)) < current_date THEN 1 ELSE 0 END)))::date,
			extract(year FROM age(current_date, u.start_date))::int + CASE WHEN (u.start_date + make_interval(years => extract(year FROM age(current_date, u.start_date))::int)) < current_date THEN 1 ELSE 0 END
		FROM adrient.users u WHERE u.org_id=$1 AND u.start_date IS NOT NULL AND u.start_date < current_date - 200
	) x WHERE on_date BETWEEN current_date AND current_date + $2::int AND (type='birthday' OR years >= 1) ORDER BY on_date, name`, a.OrgID, days)
}

func (h *HTTPHandler) learning(rt route) bool {
	w, r, a, ctx := rt.w, rt.r, rt.a, rt.r.Context()
	id := rt.id()
	switch rt.seg[0] {
	case "courses":
		switch {
		case rt.is(1, http.MethodGet):
			raw, err := h.repo.ListCourses(ctx, a)
			replyRaw(w, "courses", raw, err)
		case rt.is(1, http.MethodPost), rt.is(2, http.MethodPut):
			var in CourseInput
			if decode(w, r, &in) {
				raw, err := h.repo.SaveCourse(ctx, a, id, in)
				replyRawBody(w, raw, err)
			}
		case rt.sub("enroll", http.MethodPost):
			var b struct {
				UserIDs  []string `json:"user_ids"`
				Everyone bool     `json:"everyone"`
				DueDate  string   `json:"due_date"`
			}
			if decode(w, r, &b) {
				n, err := h.repo.Enroll(ctx, a, id, b.UserIDs, b.Everyone, b.DueDate)
				reply(w, map[string]int{"enrolled": n}, err)
			}
		case rt.sub("progress", http.MethodPost):
			var b struct {
				Status string `json:"status"`
			}
			if decode(w, r, &b) {
				replyOK(w, h.repo.SetProgress(ctx, a, id, b.Status))
			}
		case rt.sub("tracking", http.MethodGet):
			raw, err := h.repo.CourseTracking(ctx, a, id)
			replyRaw(w, "rows", raw, err)
		default:
			return false
		}
	case "policies":
		switch {
		case rt.is(1, http.MethodGet):
			raw, err := h.repo.ListPolicies(ctx, a)
			replyRaw(w, "policies", raw, err)
		case rt.is(1, http.MethodPost), rt.is(2, http.MethodPut):
			var in PolicyInput
			if decode(w, r, &in) {
				raw, err := h.repo.SavePolicy(ctx, a, id, in)
				replyRawBody(w, raw, err)
			}
		case rt.sub("acknowledge", http.MethodPost):
			replyOK(w, h.repo.Acknowledge(ctx, a, id))
		case rt.sub("tracking", http.MethodGet):
			raw, err := h.repo.PolicyTracking(ctx, a, id)
			replyRaw(w, "rows", raw, err)
		default:
			return false
		}
	case "announcements":
		switch {
		case rt.is(1, http.MethodPost):
			var in AnnouncementInput
			if decode(w, r, &in) {
				raw, err := h.repo.PostAnnouncement(ctx, a, in)
				replyRawBody(w, raw, err)
			}
		case rt.is(2, http.MethodDelete):
			replyOK(w, h.repo.DeleteAnnouncement(ctx, a, id))
		default:
			return false
		}
	case "feed":
		if !rt.is(1, http.MethodGet) {
			return false
		}
		raw, err := h.repo.Feed(ctx, a)
		replyRaw(w, "feed", raw, err)
	case "kudos":
		switch {
		case rt.is(1, http.MethodPost):
			var b struct {
				To      string `json:"to_id"`
				Badge   string `json:"badge"`
				Message string `json:"message"`
			}
			if decode(w, r, &b) {
				replyOK(w, h.repo.GiveKudos(ctx, a, b.To, b.Badge, b.Message))
			}
		case rt.is(2, http.MethodDelete):
			replyOK(w, h.repo.DeleteKudos(ctx, a, id))
		default:
			return false
		}
	case "celebrations":
		switch {
		case rt.is(1, http.MethodGet):
			raw, err := h.repo.Celebrations(ctx, a, 30)
			replyRaw(w, "celebrations", raw, err)
		case rt.is(1, http.MethodPost):
			var b struct {
				UserID string `json:"user_id"`
				Month  int    `json:"month"`
				Day    int    `json:"day"`
			}
			if decode(w, r, &b) {
				replyOK(w, h.repo.SetBirthday(ctx, a, b.UserID, b.Month, b.Day))
			}
		default:
			return false
		}
	default:
		return false
	}
	return true
}
