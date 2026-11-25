package talent

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// ---------- goals / OKRs ----------

// GoalInput creates or updates a goal or key result.
type GoalInput struct {
	ParentID    string   `json:"parent_id"`
	Kind        string   `json:"kind"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Unit        string   `json:"unit"`
	StartValue  float64  `json:"start_value"`
	Target      float64  `json:"target"`
	Current     *float64 `json:"current"`
	DueDate     string   `json:"due_date"`
	Visibility  string   `json:"visibility"`
	Status      string   `json:"status"`
}

// GoalProgress returns 0..100 progress, clamped, for a start/target/current triple.
func GoalProgress(start, target, current float64) float64 {
	if target == start {
		if current >= target {
			return 100
		}
		return 0
	}
	p := (current - start) / (target - start) * 100
	if p < 0 {
		return 0
	}
	if p > 100 {
		return 100
	}
	return float64(int(p*10+0.5)) / 10
}

// Validate checks the goal.
func (g *GoalInput) Validate() error {
	g.Title = clean(g.Title)
	if g.Title == "" || len(g.Title) > 200 {
		return invalid("title is required (max 200 characters)")
	}
	if g.Kind == "" {
		g.Kind = "objective"
		if g.ParentID != "" {
			g.Kind = "key_result"
		}
	}
	if !oneOf(g.Kind, "objective", "key_result") {
		return invalid("kind must be objective or key_result")
	}
	if g.Kind == "key_result" && g.ParentID == "" {
		return invalid("a key result needs an objective")
	}
	if g.Visibility == "" {
		g.Visibility = "team"
	}
	if !oneOf(g.Visibility, "private", "team", "company") {
		return invalid("visibility must be private, team or company")
	}
	if g.Target == 0 && g.StartValue == 0 {
		g.Target = 100
	}
	if g.Unit == "" {
		g.Unit = "%"
	}
	if g.Status == "" {
		g.Status = "active"
	}
	if !oneOf(g.Status, "active", "done", "dropped") {
		return invalid("status must be active, done or dropped")
	}
	return nil
}

// canViewUserGoal decides whether viewer may see a goal of owner with visibility vis.
func (r *Repository) canViewUserGoal(ctx context.Context, a Actor, owner, vis string) bool {
	switch {
	case a.IsHR || a.UserID == owner || vis == "company":
		return true
	case r.managesChain(ctx, a.OrgID, a.UserID, owner):
		return true
	case vis == "team":
		om, vm := r.managerOf(ctx, a.OrgID, owner), r.managerOf(ctx, a.OrgID, a.UserID)
		return om != "" && om == vm
	}
	return false
}

const goalCols = `g.id::text AS id, g.owner_id::text AS owner_id, COALESCE(g.parent_id::text,'') AS parent_id, g.kind, g.title, g.description, g.unit,
	g.start_value::float8 AS start_value, g.target::float8 AS target, g.current::float8 AS current, g.due_date, g.visibility, g.status, g.updated_at`

// ListGoals lists the goals of owner ("" = the actor) that the actor may see, with progress.
func (r *Repository) ListGoals(ctx context.Context, a Actor, owner string) (json.RawMessage, error) {
	if owner == "" {
		owner = a.UserID
	}
	raw, err := r.rows(ctx, `SELECT `+goalCols+`, COALESCE(NULLIF(u.display_name,''),u.email) AS owner_name
		FROM adrient.talent_goals g JOIN adrient.users u ON u.id=g.owner_id WHERE g.org_id=$1 AND g.owner_id=$2 ORDER BY g.created_at`, a.OrgID, owner)
	if err != nil {
		return nil, err
	}
	var all []map[string]any
	_ = json.Unmarshal(raw, &all)
	out := []map[string]any{}
	for _, g := range all {
		vis, _ := g["visibility"].(string)
		if !r.canViewUserGoal(ctx, a, owner, vis) {
			continue
		}
		s, _ := g["start_value"].(float64)
		t, _ := g["target"].(float64)
		c, _ := g["current"].(float64)
		g["progress"] = GoalProgress(s, t, c)
		out = append(out, g)
	}
	b, _ := json.Marshal(out)
	return b, nil
}

// CompanyGoals lists goals shared company-wide.
func (r *Repository) CompanyGoals(ctx context.Context, a Actor) (json.RawMessage, error) {
	return r.rows(ctx, `SELECT `+goalCols+`, COALESCE(NULLIF(u.display_name,''),u.email) AS owner_name,
		CASE WHEN g.target=g.start_value THEN (CASE WHEN g.current>=g.target THEN 100 ELSE 0 END)
		ELSE least(100, greatest(0, round(((g.current-g.start_value)/(g.target-g.start_value)*100)::numeric,1)))::float8 END AS progress
		FROM adrient.talent_goals g JOIN adrient.users u ON u.id=g.owner_id WHERE g.org_id=$1 AND g.visibility='company' AND g.status<>'dropped' ORDER BY g.kind, g.created_at`, a.OrgID)
}

// SaveGoal creates (id=="") or updates a goal. Only the owner (or HR) may change it.
func (r *Repository) SaveGoal(ctx context.Context, a Actor, id string, in GoalInput) (json.RawMessage, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}
	if in.Current == nil {
		z := in.StartValue
		in.Current = &z
	}
	var due any
	if in.DueDate != "" {
		t, err := time.Parse("2006-01-02", in.DueDate)
		if err != nil {
			return nil, invalid("due date must be YYYY-MM-DD")
		}
		due = t
	}
	if in.ParentID != "" {
		var po string
		if err := r.pool.QueryRow(ctx, `SELECT owner_id::text FROM adrient.talent_goals WHERE org_id=$1 AND id=$2`, a.OrgID, in.ParentID).Scan(&po); err != nil {
			return nil, invalid("the objective does not exist")
		}
		if po != a.UserID && !a.IsHR {
			return nil, ErrForbidden
		}
	}
	var gid string
	var err error
	if id == "" {
		err = r.pool.QueryRow(ctx, `INSERT INTO adrient.talent_goals (org_id, owner_id, parent_id, kind, title, description, unit, start_value, target, current, due_date, visibility, status)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) RETURNING id::text`, a.OrgID, a.UserID, nullUUID(in.ParentID), in.Kind, in.Title, in.Description,
			in.Unit, in.StartValue, in.Target, *in.Current, due, in.Visibility, in.Status).Scan(&gid)
	} else {
		err = r.pool.QueryRow(ctx, `UPDATE adrient.talent_goals SET title=$3, description=$4, unit=$5, start_value=$6, target=$7, current=$8, due_date=$9,
			visibility=$10, status=$11, updated_at=now() WHERE org_id=$1 AND id=$2 AND (owner_id=$12::uuid OR $13::boolean) RETURNING id::text`,
			a.OrgID, id, in.Title, in.Description, in.Unit, in.StartValue, in.Target, *in.Current, due, in.Visibility, in.Status, a.UserID, a.IsHR).Scan(&gid)
	}
	if err != nil {
		return nil, mapNoRows(err)
	}
	return r.row(ctx, `SELECT `+goalCols+` FROM adrient.talent_goals g WHERE g.id=$1`, gid)
}

// DeleteGoal removes a goal (owner or HR).
func (r *Repository) DeleteGoal(ctx context.Context, a Actor, id string) error {
	ct, err := r.pool.Exec(ctx, `DELETE FROM adrient.talent_goals WHERE org_id=$1 AND id=$2 AND (owner_id=$3::uuid OR $4::boolean)`, a.OrgID, id, a.UserID, a.IsHR)
	if err == nil && ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// ---------- review cycles ----------

// CycleInput creates a review cycle.
type CycleInput struct {
	Name      string          `json:"name"`
	StartDate string          `json:"start_date"`
	EndDate   string          `json:"end_date"`
	Kinds     []string        `json:"kinds"`
	Scale     json.RawMessage `json:"scale"`
	Questions []string        `json:"questions"`
}

// ScaleLabel is one point on a rating scale.
type ScaleLabel struct {
	V     int    `json:"v"`
	Label string `json:"label"`
}

// ValidateScale checks a rating scale: 2..10 consecutive ascending values from 1.
func ValidateScale(sc []ScaleLabel) error {
	if len(sc) < 2 || len(sc) > 10 {
		return invalid("a rating scale needs 2 to 10 points")
	}
	for i, s := range sc {
		if s.V != i+1 || clean(s.Label) == "" {
			return invalid("scale points must be numbered 1..n and labelled")
		}
	}
	return nil
}

// SaveCycle creates a draft cycle. HR only.
func (r *Repository) SaveCycle(ctx context.Context, a Actor, in CycleInput) (json.RawMessage, error) {
	if !a.IsHR {
		return nil, ErrForbidden
	}
	in.Name = clean(in.Name)
	if in.Name == "" {
		return nil, invalid("name is required")
	}
	s, err1 := time.Parse("2006-01-02", in.StartDate)
	e, err2 := time.Parse("2006-01-02", in.EndDate)
	if err1 != nil || err2 != nil || e.Before(s) {
		return nil, invalid("dates must be YYYY-MM-DD with the end on or after the start")
	}
	if len(in.Kinds) == 0 {
		in.Kinds = []string{"self", "manager"}
	}
	for _, k := range in.Kinds {
		if !oneOf(k, "self", "manager", "peer") {
			return nil, invalid("unknown review kind %q", k)
		}
	}
	scale := []byte(in.Scale)
	if len(scale) == 0 || string(scale) == "null" {
		scale = []byte(`[{"v":1,"label":"Needs improvement"},{"v":2,"label":"Partially meets"},{"v":3,"label":"Meets expectations"},{"v":4,"label":"Exceeds"},{"v":5,"label":"Outstanding"}]`)
	} else {
		var sc []ScaleLabel
		if err := json.Unmarshal(scale, &sc); err != nil {
			return nil, invalid("scale is not valid")
		}
		if err := ValidateScale(sc); err != nil {
			return nil, err
		}
	}
	if len(in.Questions) == 0 {
		in.Questions = []string{"What went well this period?", "What could be improved?"}
	}
	qb, _ := json.Marshal(in.Questions)
	var id string
	if err := r.pool.QueryRow(ctx, `INSERT INTO adrient.talent_review_cycles (org_id, name, start_date, end_date, kinds, scale, questions, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id::text`, a.OrgID, in.Name, s, e, in.Kinds, scale, qb, a.UserID).Scan(&id); err != nil {
		return nil, err
	}
	return r.row(ctx, `SELECT id::text AS id, name, start_date, end_date, status, kinds, scale, questions FROM adrient.talent_review_cycles WHERE id=$1`, id)
}

// ListCycles lists cycles. HR sees all; others see active/closed/published ones they take part in.
func (r *Repository) ListCycles(ctx context.Context, a Actor) (json.RawMessage, error) {
	return r.rows(ctx, `SELECT c.id::text AS id, c.name, c.start_date, c.end_date, c.status, c.kinds, c.scale, c.questions,
		(SELECT count(*) FROM adrient.talent_reviews x WHERE x.cycle_id=c.id) AS reviews,
		(SELECT count(*) FROM adrient.talent_reviews x WHERE x.cycle_id=c.id AND x.submitted_at IS NOT NULL) AS submitted
		FROM adrient.talent_review_cycles c WHERE c.org_id=$1 AND ($3::boolean OR (c.status<>'draft' AND EXISTS(SELECT 1 FROM adrient.talent_reviews x
			WHERE x.cycle_id=c.id AND (x.subject_id=$2::uuid OR x.reviewer_id=$2::uuid)) OR EXISTS(SELECT 1 FROM adrient.users m WHERE m.org_id=$1 AND m.manager_id=$2::uuid)))
		ORDER BY c.start_date DESC`, a.OrgID, a.UserID, a.IsHR)
}

// SetCycleStatus moves a cycle draft -> active -> closed -> published. Activating
// creates self and manager review tasks for everyone with an active account.
func (r *Repository) SetCycleStatus(ctx context.Context, a Actor, id, to string) (json.RawMessage, error) {
	if !a.IsHR {
		return nil, ErrForbidden
	}
	var cur string
	var kinds []string
	if err := r.pool.QueryRow(ctx, `SELECT status, kinds FROM adrient.talent_review_cycles WHERE org_id=$1 AND id=$2`, a.OrgID, id).Scan(&cur, &kinds); err != nil {
		return nil, mapNoRows(err)
	}
	order := map[string]int{"draft": 0, "active": 1, "closed": 2, "published": 3}
	if to == "active" && cur == "closed" {
		// allow reopening
	} else if order[to] != order[cur]+1 {
		return nil, invalid("a cycle goes draft, active, closed, published; it is %s", cur)
	}
	if to == "active" && cur == "draft" {
		if contains(kinds, "self") {
			if _, err := r.pool.Exec(ctx, `INSERT INTO adrient.talent_reviews (org_id, cycle_id, subject_id, reviewer_id, kind)
				SELECT org_id, $2, id, id, 'self' FROM adrient.users WHERE org_id=$1 AND COALESCE(status,'active')='active' AND user_type IN ('member','internal','')
				ON CONFLICT DO NOTHING`, a.OrgID, id); err != nil {
				return nil, err
			}
		}
		if contains(kinds, "manager") {
			if _, err := r.pool.Exec(ctx, `INSERT INTO adrient.talent_reviews (org_id, cycle_id, subject_id, reviewer_id, kind)
				SELECT org_id, $2, id, manager_id, 'manager' FROM adrient.users WHERE org_id=$1 AND manager_id IS NOT NULL AND COALESCE(status,'active')='active'
				ON CONFLICT DO NOTHING`, a.OrgID, id); err != nil {
				return nil, err
			}
		}
	}
	if _, err := r.pool.Exec(ctx, `UPDATE adrient.talent_review_cycles SET status=$3 WHERE org_id=$1 AND id=$2`, a.OrgID, id, to); err != nil {
		return nil, err
	}
	r.audit(ctx, a, "cycle."+to, "talent_cycle", id)
	return r.row(ctx, `SELECT id::text AS id, name, status FROM adrient.talent_review_cycles WHERE id=$1`, id)
}

// AssignPeers adds peer reviewers for a subject (HR or the subject's manager).
func (r *Repository) AssignPeers(ctx context.Context, a Actor, cycleID, subject string, peers []string) error {
	var status string
	var kinds []string
	if err := r.pool.QueryRow(ctx, `SELECT status, kinds FROM adrient.talent_review_cycles WHERE org_id=$1 AND id=$2`, a.OrgID, cycleID).Scan(&status, &kinds); err != nil {
		return mapNoRows(err)
	}
	if !a.IsHR && !r.managesChain(ctx, a.OrgID, a.UserID, subject) {
		return ErrNotFound
	}
	if !contains(kinds, "peer") {
		return invalid("this cycle has no peer (360) reviews")
	}
	if status != "active" {
		return invalid("peers can be assigned while the cycle is active")
	}
	for _, p := range uuidList(peers) {
		if p == subject {
			continue
		}
		if _, err := r.userBrief(ctx, a.OrgID, p); err != nil {
			return invalid("a peer is not a member of this organisation")
		}
		if _, err := r.pool.Exec(ctx, `INSERT INTO adrient.talent_reviews (org_id, cycle_id, subject_id, reviewer_id, kind) VALUES ($1,$2,$3,$4,'peer') ON CONFLICT DO NOTHING`,
			a.OrgID, cycleID, subject, p); err != nil {
			return err
		}
	}
	return nil
}

// MyReviewTasks lists reviews the actor must write in active cycles.
func (r *Repository) MyReviewTasks(ctx context.Context, a Actor) (json.RawMessage, error) {
	return r.rows(ctx, `SELECT x.id::text AS id, x.cycle_id::text AS cycle_id, c.name AS cycle_name, x.kind, x.subject_id::text AS subject_id,
		COALESCE(NULLIF(u.display_name,''),u.email) AS subject_name, x.rating, x.answers, x.comment, (x.submitted_at IS NOT NULL) AS submitted, c.scale, c.questions, c.end_date
		FROM adrient.talent_reviews x JOIN adrient.talent_review_cycles c ON c.id=x.cycle_id JOIN adrient.users u ON u.id=x.subject_id
		WHERE x.org_id=$1 AND x.reviewer_id=$2 AND c.status='active' ORDER BY (x.submitted_at IS NOT NULL), c.end_date`, a.OrgID, a.UserID)
}

// SubmitInput is a written review.
type SubmitInput struct {
	Rating  int      `json:"rating"`
	Answers []string `json:"answers"`
	Comment string   `json:"comment"`
}

// SubmitReview saves a review. Only its reviewer may, and only while the cycle is active.
func (r *Repository) SubmitReview(ctx context.Context, a Actor, reviewID string, in SubmitInput) error {
	var reviewer, status string
	var scale []byte
	err := r.pool.QueryRow(ctx, `SELECT x.reviewer_id::text, c.status, c.scale FROM adrient.talent_reviews x JOIN adrient.talent_review_cycles c ON c.id=x.cycle_id
		WHERE x.org_id=$1 AND x.id=$2`, a.OrgID, reviewID).Scan(&reviewer, &status, &scale)
	if err != nil || reviewer != a.UserID {
		return ErrNotFound
	}
	if status != "active" {
		return invalid("this review cycle is %s", status)
	}
	var sc []ScaleLabel
	_ = json.Unmarshal(scale, &sc)
	if in.Rating < 1 || in.Rating > len(sc) {
		return invalid("rating must be between 1 and %d", len(sc))
	}
	if len(in.Comment) > 5000 {
		return invalid("comment is too long")
	}
	ab, _ := json.Marshal(in.Answers)
	_, err = r.pool.Exec(ctx, `UPDATE adrient.talent_reviews SET rating=$2, answers=$3, comment=$4, submitted_at=now() WHERE id=$1`, reviewID, in.Rating, ab, in.Comment)
	return err
}

// Result is the review outcome of one person in a cycle, filtered to what the viewer may see.
type Result struct {
	SubjectID  string       `json:"subject_id"`
	Subject    string       `json:"subject_name"`
	Published  bool         `json:"published"`
	Self       *ReviewView  `json:"self"`
	Manager    *ReviewView  `json:"manager"`
	Peers      []ReviewView `json:"peers"`
	PeerAvg    *float64     `json:"peer_avg"`
	Calibrated *int         `json:"calibrated_rating"`
	Final      *int         `json:"final_rating"`
	Viewer     string       `json:"viewer_role"`
}

// ReviewView is one review as shown to a viewer.
type ReviewView struct {
	Reviewer  string   `json:"reviewer,omitempty"`
	Rating    *int     `json:"rating"`
	Answers   []string `json:"answers"`
	Comment   string   `json:"comment"`
	Submitted bool     `json:"submitted"`
}

// GetResult returns a person's review result with server-side visibility rules:
// HR sees everything; a manager (direct or above) sees all reviews with peers
// anonymous; the subject sees their own self review at any time and the manager
// review, peer feedback and final rating only after the cycle is published; a
// peer sees nothing of the result.
func (r *Repository) GetResult(ctx context.Context, a Actor, cycleID, subject string) (Result, error) {
	var res Result
	var status string
	if err := r.pool.QueryRow(ctx, `SELECT status FROM adrient.talent_review_cycles WHERE org_id=$1 AND id=$2`, a.OrgID, cycleID).Scan(&status); err != nil {
		return res, mapNoRows(err)
	}
	role := ""
	switch {
	case a.IsHR:
		role = "hr"
	case r.managesChain(ctx, a.OrgID, a.UserID, subject):
		role = "manager"
	case a.UserID == subject:
		role = "subject"
	default:
		return res, ErrNotFound
	}
	if status == "draft" && role != "hr" {
		return res, ErrNotFound
	}
	u, err := r.userBrief(ctx, a.OrgID, subject)
	if err != nil {
		return res, err
	}
	res.SubjectID, res.Subject, res.Published, res.Viewer = subject, u.Name, status == "published", role
	rows, err := r.pool.Query(ctx, `SELECT x.kind, COALESCE(NULLIF(rv.display_name,''),rv.email), x.rating, x.answers, x.comment, x.submitted_at IS NOT NULL, x.calibrated_rating
		FROM adrient.talent_reviews x JOIN adrient.users rv ON rv.id=x.reviewer_id WHERE x.org_id=$1 AND x.cycle_id=$2 AND x.subject_id=$3 ORDER BY x.kind`, a.OrgID, cycleID, subject)
	if err != nil {
		return res, err
	}
	defer rows.Close()
	var peerSum, peerN int
	for rows.Next() {
		var kind, who, comment string
		var rating, cal *int
		var ans []byte
		var sub bool
		if err := rows.Scan(&kind, &who, &rating, &ans, &comment, &sub, &cal); err != nil {
			return res, err
		}
		var answers []string
		_ = json.Unmarshal(ans, &answers)
		v := ReviewView{Reviewer: who, Rating: rating, Answers: answers, Comment: comment, Submitted: sub}
		if cal != nil {
			res.Calibrated = cal
		}
		switch kind {
		case "self":
			res.Self = &v
		case "manager":
			if role == "subject" && !res.Published {
				v = ReviewView{Submitted: sub}
			}
			res.Manager = &v
		case "peer":
			if role == "subject" && !res.Published {
				continue
			}
			if sub && rating != nil {
				peerSum += *rating
				peerN++
			}
			if role != "hr" {
				v.Reviewer = "" // peers stay anonymous to managers and the subject
			}
			res.Peers = append(res.Peers, v)
		}
	}
	if peerN > 0 {
		avg := float64(int(float64(peerSum)/float64(peerN)*10+0.5)) / 10
		res.PeerAvg = &avg
	}
	if role == "subject" && !res.Published {
		res.Calibrated = nil
	}
	if res.Calibrated != nil {
		res.Final = res.Calibrated
	} else if res.Manager != nil && res.Manager.Rating != nil && (role != "subject" || res.Published) {
		res.Final = res.Manager.Rating
	}
	return res, rows.Err()
}

// Calibration is the HR calibration table of a cycle.
func (r *Repository) Calibration(ctx context.Context, a Actor, cycleID string) (json.RawMessage, error) {
	if !a.IsHR {
		return nil, ErrForbidden
	}
	return r.rows(ctx, `SELECT u.id::text AS subject_id, COALESCE(NULLIF(u.display_name,''),u.email) AS name, COALESCE(d.name,'') AS department,
		(SELECT x.rating FROM adrient.talent_reviews x WHERE x.cycle_id=$2 AND x.subject_id=u.id AND x.kind='self') AS self_rating,
		(SELECT x.rating FROM adrient.talent_reviews x WHERE x.cycle_id=$2 AND x.subject_id=u.id AND x.kind='manager') AS manager_rating,
		(SELECT round(avg(x.rating)::numeric,1)::float8 FROM adrient.talent_reviews x WHERE x.cycle_id=$2 AND x.subject_id=u.id AND x.kind='peer' AND x.rating IS NOT NULL) AS peer_avg,
		(SELECT max(x.calibrated_rating) FROM adrient.talent_reviews x WHERE x.cycle_id=$2 AND x.subject_id=u.id) AS calibrated_rating
		FROM adrient.users u LEFT JOIN adrient.departments d ON d.id=u.department_id
		WHERE u.org_id=$1 AND EXISTS(SELECT 1 FROM adrient.talent_reviews x WHERE x.cycle_id=$2 AND x.subject_id=u.id) ORDER BY 2`, a.OrgID, cycleID)
}

// SetCalibrated stores the calibrated (final) rating for a person. HR only.
func (r *Repository) SetCalibrated(ctx context.Context, a Actor, cycleID, subject string, rating int) error {
	if !a.IsHR {
		return ErrForbidden
	}
	if rating < 1 || rating > 10 {
		return invalid("rating is out of range")
	}
	ct, err := r.pool.Exec(ctx, `UPDATE adrient.talent_reviews SET calibrated_rating=$4 WHERE org_id=$1 AND cycle_id=$2 AND subject_id=$3 AND kind='manager'`, a.OrgID, cycleID, subject, rating)
	if err == nil && ct.RowsAffected() == 0 {
		return invalid("there is no manager review to calibrate for this person")
	}
	r.audit(ctx, a, "cycle.calibrate", "talent_cycle", cycleID)
	return err
}

// ---------- 1:1 notes ----------

// OneOnOneInput records a 1:1 meeting.
type OneOnOneInput struct {
	WithID  string          `json:"with_id"`
	Date    string          `json:"meeting_date"`
	Agenda  string          `json:"agenda"`
	Notes   string          `json:"notes"`
	Actions json.RawMessage `json:"actions"`
}

// SaveOneOnOne creates (id=="") or updates a 1:1. Only a manager and their direct report take part.
func (r *Repository) SaveOneOnOne(ctx context.Context, a Actor, id string, in OneOnOneInput) (json.RawMessage, error) {
	d, err := time.Parse("2006-01-02", in.Date)
	if err != nil {
		return nil, invalid("date must be YYYY-MM-DD")
	}
	actions := []byte(in.Actions)
	if len(actions) == 0 || string(actions) == "null" {
		actions = []byte("[]")
	}
	if id != "" {
		var m, rp string
		if err := r.pool.QueryRow(ctx, `SELECT manager_id::text, report_id::text FROM adrient.talent_one_on_ones WHERE org_id=$1 AND id=$2`, a.OrgID, id).Scan(&m, &rp); err != nil {
			return nil, mapNoRows(err)
		}
		if a.UserID != m && a.UserID != rp {
			return nil, ErrNotFound
		}
		if _, err := r.pool.Exec(ctx, `UPDATE adrient.talent_one_on_ones SET meeting_date=$2, agenda=$3, notes=$4, actions=$5 WHERE id=$1`, id, d, in.Agenda, in.Notes, actions); err != nil {
			return nil, err
		}
		return r.row(ctx, `SELECT id::text AS id, manager_id::text AS manager_id, report_id::text AS report_id, meeting_date, agenda, notes, actions FROM adrient.talent_one_on_ones WHERE id=$1`, id)
	}
	mgr, rep := "", ""
	switch {
	case r.managerOf(ctx, a.OrgID, in.WithID) == a.UserID:
		mgr, rep = a.UserID, in.WithID
	case r.managerOf(ctx, a.OrgID, a.UserID) == in.WithID && in.WithID != "":
		mgr, rep = in.WithID, a.UserID
	default:
		return nil, invalid("1:1s are between a manager and a direct report")
	}
	var nid string
	if err := r.pool.QueryRow(ctx, `INSERT INTO adrient.talent_one_on_ones (org_id, manager_id, report_id, meeting_date, agenda, notes, actions, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id::text`, a.OrgID, mgr, rep, d, in.Agenda, in.Notes, actions, a.UserID).Scan(&nid); err != nil {
		return nil, err
	}
	return r.row(ctx, `SELECT id::text AS id, manager_id::text AS manager_id, report_id::text AS report_id, meeting_date, agenda, notes, actions FROM adrient.talent_one_on_ones WHERE id=$1`, nid)
}

// ListOneOnOnes lists the actor's own 1:1s, optionally with one person. Private to the two people: HR cannot read them.
func (r *Repository) ListOneOnOnes(ctx context.Context, a Actor, with string) (json.RawMessage, error) {
	return r.rows(ctx, `SELECT o.id::text AS id, o.manager_id::text AS manager_id, o.report_id::text AS report_id, o.meeting_date, o.agenda, o.notes, o.actions,
		COALESCE(NULLIF(m.display_name,''),m.email) AS manager_name, COALESCE(NULLIF(p.display_name,''),p.email) AS report_name
		FROM adrient.talent_one_on_ones o JOIN adrient.users m ON m.id=o.manager_id JOIN adrient.users p ON p.id=o.report_id
		WHERE o.org_id=$1 AND (o.manager_id=$2::uuid OR o.report_id=$2::uuid) AND ($3='' OR o.manager_id::text=$3 OR o.report_id::text=$3) ORDER BY o.meeting_date DESC`,
		a.OrgID, a.UserID, with)
}

// ---------- feedback requests ----------

// RequestFeedback asks people for feedback about someone (the person themself, their manager, or HR).
func (r *Repository) RequestFeedback(ctx context.Context, a Actor, about string, from []string, prompt string) error {
	if about == "" {
		about = a.UserID
	}
	if about != a.UserID && !a.IsHR && !r.managesChain(ctx, a.OrgID, a.UserID, about) {
		return ErrNotFound
	}
	from = uuidList(from)
	if len(from) == 0 || len(from) > 20 {
		return invalid("choose between 1 and 20 people to ask")
	}
	for _, f := range from {
		if f == about {
			continue
		}
		if _, err := r.userBrief(ctx, a.OrgID, f); err != nil {
			return invalid("someone you asked is not a member of this organisation")
		}
		if _, err := r.pool.Exec(ctx, `INSERT INTO adrient.talent_feedback (org_id, about_id, from_id, requested_by, prompt) VALUES ($1,$2,$3,$4,$5)`,
			a.OrgID, about, f, a.UserID, clean(prompt)); err != nil {
			return err
		}
	}
	return nil
}

// FeedbackToGive lists pending requests addressed to the actor.
func (r *Repository) FeedbackToGive(ctx context.Context, a Actor) (json.RawMessage, error) {
	return r.rows(ctx, `SELECT f.id::text AS id, f.about_id::text AS about_id, COALESCE(NULLIF(u.display_name,''),u.email) AS about_name, f.prompt, f.status, f.created_at
		FROM adrient.talent_feedback f JOIN adrient.users u ON u.id=f.about_id WHERE f.org_id=$1 AND f.from_id=$2 AND f.status='requested' ORDER BY f.created_at DESC`, a.OrgID, a.UserID)
}

// FeedbackReceived lists answered feedback the actor asked for. HR sees all.
func (r *Repository) FeedbackReceived(ctx context.Context, a Actor) (json.RawMessage, error) {
	return r.rows(ctx, `SELECT f.id::text AS id, f.about_id::text AS about_id, COALESCE(NULLIF(u.display_name,''),u.email) AS about_name,
		COALESCE(NULLIF(g.display_name,''),g.email) AS from_name, f.prompt, f.response, f.status, f.answered_at
		FROM adrient.talent_feedback f JOIN adrient.users u ON u.id=f.about_id JOIN adrient.users g ON g.id=f.from_id
		WHERE f.org_id=$1 AND f.status<>'requested' AND ($3::boolean OR f.requested_by=$2::uuid) ORDER BY f.answered_at DESC`, a.OrgID, a.UserID, a.IsHR)
}

// AnswerFeedback records the response (or a decline) of the person asked.
func (r *Repository) AnswerFeedback(ctx context.Context, a Actor, id, response string, decline bool) error {
	response = clean(response)
	if !decline && response == "" {
		return invalid("write your feedback or decline")
	}
	if len(response) > 5000 {
		return invalid("feedback is too long")
	}
	st := "answered"
	if decline {
		st = "declined"
	}
	ct, err := r.pool.Exec(ctx, `UPDATE adrient.talent_feedback SET response=$4, status=$5, answered_at=now() WHERE org_id=$1 AND id=$2 AND from_id=$3 AND status='requested'`,
		a.OrgID, id, a.UserID, response, st)
	if err == nil && ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func (h *HTTPHandler) performance(rt route) bool {
	w, r, a, ctx := rt.w, rt.r, rt.a, rt.r.Context()
	id := rt.id()
	q := r.URL.Query()
	switch rt.seg[0] {
	case "goals":
		switch {
		case rt.is(1, http.MethodGet):
			var raw json.RawMessage
			var err error
			if q.Get("scope") == "company" {
				raw, err = h.repo.CompanyGoals(ctx, a)
			} else {
				raw, err = h.repo.ListGoals(ctx, a, q.Get("owner"))
			}
			replyRaw(w, "goals", raw, err)
		case rt.is(1, http.MethodPost), rt.is(2, http.MethodPut):
			var in GoalInput
			if decode(w, r, &in) {
				raw, err := h.repo.SaveGoal(ctx, a, id, in)
				replyRawBody(w, raw, err)
			}
		case rt.is(2, http.MethodDelete):
			replyOK(w, h.repo.DeleteGoal(ctx, a, id))
		default:
			return false
		}
	case "cycles":
		switch {
		case rt.is(1, http.MethodGet):
			raw, err := h.repo.ListCycles(ctx, a)
			replyRaw(w, "cycles", raw, err)
		case rt.is(1, http.MethodPost):
			var in CycleInput
			if decode(w, r, &in) {
				raw, err := h.repo.SaveCycle(ctx, a, in)
				replyRawBody(w, raw, err)
			}
		case rt.sub("status", http.MethodPost):
			var b struct {
				Status string `json:"status"`
			}
			if decode(w, r, &b) {
				raw, err := h.repo.SetCycleStatus(ctx, a, id, b.Status)
				replyRawBody(w, raw, err)
			}
		case rt.sub("calibration", http.MethodGet):
			raw, err := h.repo.Calibration(ctx, a, id)
			replyRaw(w, "rows", raw, err)
		case len(rt.seg) == 4 && rt.seg[2] == "result" && rt.method == http.MethodGet:
			res, err := h.repo.GetResult(ctx, a, id, rt.seg[3])
			reply(w, res, err)
		case len(rt.seg) == 4 && rt.seg[2] == "calibrate" && rt.method == http.MethodPost:
			var b struct {
				Rating int `json:"rating"`
			}
			if decode(w, r, &b) {
				replyOK(w, h.repo.SetCalibrated(ctx, a, id, rt.seg[3], b.Rating))
			}
		case len(rt.seg) == 4 && rt.seg[2] == "peers" && rt.method == http.MethodPost:
			var b struct {
				PeerIDs []string `json:"peer_ids"`
			}
			if decode(w, r, &b) {
				replyOK(w, h.repo.AssignPeers(ctx, a, id, rt.seg[3], b.PeerIDs))
			}
		default:
			return false
		}
	case "reviews":
		switch {
		case rt.is(1, http.MethodGet):
			raw, err := h.repo.MyReviewTasks(ctx, a)
			replyRaw(w, "reviews", raw, err)
		case rt.sub("submit", http.MethodPost):
			var in SubmitInput
			if decode(w, r, &in) {
				replyOK(w, h.repo.SubmitReview(ctx, a, id, in))
			}
		default:
			return false
		}
	case "one-on-ones":
		switch {
		case rt.is(1, http.MethodGet):
			raw, err := h.repo.ListOneOnOnes(ctx, a, q.Get("with"))
			replyRaw(w, "one_on_ones", raw, err)
		case rt.is(1, http.MethodPost), rt.is(2, http.MethodPut):
			var in OneOnOneInput
			if decode(w, r, &in) {
				raw, err := h.repo.SaveOneOnOne(ctx, a, id, in)
				replyRawBody(w, raw, err)
			}
		default:
			return false
		}
	case "feedback":
		switch {
		case rt.is(1, http.MethodGet) && q.Get("box") == "received":
			raw, err := h.repo.FeedbackReceived(ctx, a)
			replyRaw(w, "feedback", raw, err)
		case rt.is(1, http.MethodGet):
			raw, err := h.repo.FeedbackToGive(ctx, a)
			replyRaw(w, "feedback", raw, err)
		case rt.is(1, http.MethodPost):
			var b struct {
				About  string   `json:"about_id"`
				From   []string `json:"from_ids"`
				Prompt string   `json:"prompt"`
			}
			if decode(w, r, &b) {
				replyOK(w, h.repo.RequestFeedback(ctx, a, b.About, b.From, b.Prompt))
			}
		case rt.sub("answer", http.MethodPost):
			var b struct {
				Response string `json:"response"`
				Decline  bool   `json:"decline"`
			}
			if decode(w, r, &b) {
				replyOK(w, h.repo.AnswerFeedback(ctx, a, id, b.Response, b.Decline))
			}
		default:
			return false
		}
	default:
		return false
	}
	return true
}
