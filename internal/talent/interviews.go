package talent

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"strings"
	"time"
)

// ScorecardInput is one interviewer's feedback.
type ScorecardInput struct {
	InterviewID    string         `json:"interview_id"`
	Ratings        map[string]int `json:"ratings"`
	Overall        int            `json:"overall"`
	Recommendation string         `json:"recommendation"`
	Comment        string         `json:"comment"`
	GuideID        string         `json:"guide_id"`
	Answers        []Answer       `json:"answers"`
}

var recommendations = []string{"strong_no", "no", "yes", "strong_yes"}

// Validate checks the scorecard.
func (s *ScorecardInput) Validate() error {
	if s.Overall < 1 || s.Overall > 5 {
		return invalid("overall rating must be 1 to 5")
	}
	for k, v := range s.Ratings {
		if v < 1 || v > 5 {
			return invalid("rating for %q must be 1 to 5", k)
		}
	}
	if !oneOf(s.Recommendation, recommendations...) {
		return invalid("choose a recommendation")
	}
	if len(s.Comment) > 5000 {
		return invalid("comment is too long")
	}
	return nil
}

// SaveScorecard stores or replaces the caller's own scorecard. Any hiring-team member may score.
func (r *Repository) SaveScorecard(ctx context.Context, a Actor, appID string, in ScorecardInput) (json.RawMessage, error) {
	f, err := r.accessApp(ctx, a, appID)
	if err != nil {
		return nil, err
	}
	if err := in.Validate(); err != nil {
		return nil, err
	}
	if in.Ratings == nil {
		in.Ratings = map[string]int{}
	}
	answers, err := CleanAnswers(in.Answers)
	if err != nil {
		return nil, err
	}
	if in.GuideID != "" {
		if err := r.guideBelongs(ctx, a, in.GuideID); err != nil {
			return nil, err
		}
	}
	rb, _ := json.Marshal(in.Ratings)
	ab, _ := json.Marshal(answers)
	_, err = r.pool.Exec(ctx, `INSERT INTO adrient.talent_scorecards (org_id, application_id, interview_id, reviewer_id, ratings, overall, recommendation, comment, guide_id, answers)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,NULLIF($9,'')::uuid,$10)
		ON CONFLICT (application_id, reviewer_id, COALESCE(interview_id,'00000000-0000-0000-0000-000000000000'::uuid))
		DO UPDATE SET ratings=EXCLUDED.ratings, overall=EXCLUDED.overall, recommendation=EXCLUDED.recommendation, comment=EXCLUDED.comment, guide_id=EXCLUDED.guide_id, answers=EXCLUDED.answers, created_at=now()`,
		a.OrgID, appID, nullUUID(in.InterviewID), a.UserID, rb, in.Overall, in.Recommendation, in.Comment, in.GuideID, ab)
	if err != nil {
		return nil, err
	}
	r.audit(ctx, a, "scorecard.save", "talent_application", appID)
	return r.scorecardsFor(ctx, a, f)
}

// scorecardsFor lists scorecards the actor may read. Leads (recruiter, hiring
// manager, HR) see all. A plain interviewer sees others' feedback only after
// submitting their own, so feedback stays independent.
func (r *Repository) scorecardsFor(ctx context.Context, a Actor, f appFacts) (json.RawMessage, error) {
	q := `SELECT s.id::text AS id, COALESCE(s.interview_id::text,'') AS interview_id, s.reviewer_id::text AS reviewer_id,
		COALESCE(NULLIF(u.display_name,''),u.email,'') AS reviewer_name, s.ratings, s.overall, s.recommendation, s.comment, s.created_at, s.answers
		FROM adrient.talent_scorecards s LEFT JOIN adrient.users u ON u.id=s.reviewer_id WHERE s.application_id=$1`
	if f.Job.leads(a) {
		return r.rows(ctx, q+` ORDER BY s.created_at`, f.ID)
	}
	return r.rows(ctx, q+` AND (s.reviewer_id=$2::uuid OR EXISTS(SELECT 1 FROM adrient.talent_scorecards m WHERE m.application_id=$1 AND m.reviewer_id=$2::uuid))
		ORDER BY s.created_at`, f.ID, a.UserID)
}

// InterviewInput schedules an interview.
type InterviewInput struct {
	Title          string    `json:"title"`
	StartsAt       time.Time `json:"starts_at"`
	EndsAt         time.Time `json:"ends_at"`
	InterviewerIDs []string  `json:"interviewer_ids"`
	Notes          string    `json:"notes"`
}

// Validate checks the schedule.
func (in *InterviewInput) Validate(now time.Time) error {
	in.Title = clean(in.Title)
	if in.Title == "" {
		in.Title = "Interview"
	}
	if in.StartsAt.IsZero() || in.EndsAt.IsZero() {
		return invalid("start and end time are required")
	}
	if !in.EndsAt.After(in.StartsAt) {
		return invalid("the interview must end after it starts")
	}
	if in.EndsAt.Sub(in.StartsAt) > 8*time.Hour {
		return invalid("an interview cannot be longer than 8 hours")
	}
	if in.StartsAt.Before(now.Add(-time.Hour)) {
		return invalid("the interview is in the past")
	}
	in.InterviewerIDs = uuidList(in.InterviewerIDs)
	return nil
}

// ScheduleInterview creates the interview, a Calendar event with a Meet link and mails the candidate.
func (r *Repository) ScheduleInterview(ctx context.Context, a Actor, appID, baseURL string, in InterviewInput) (json.RawMessage, error) {
	f, err := r.leadApp(ctx, a, appID)
	if err != nil {
		return nil, err
	}
	if f.Status != "active" {
		return nil, invalid("this application is %s", f.Status)
	}
	if err := in.Validate(r.now()); err != nil {
		return nil, err
	}
	if r.Deps.Calendar == nil {
		return nil, fmt.Errorf("%w: Calendar integration", ErrUnavailable)
	}
	var candName, candEmail string
	if err := r.pool.QueryRow(ctx, `SELECT c.name, c.email FROM adrient.talent_candidates c WHERE c.id=$1`, f.CandidateID).Scan(&candName, &candEmail); err != nil {
		return nil, mapNoRows(err)
	}
	attendees := []string{candEmail}
	for _, id := range in.InterviewerIDs {
		u, err := r.userBrief(ctx, a.OrgID, id)
		if err != nil {
			return nil, invalid("interviewer is not a member of this organisation")
		}
		attendees = append(attendees, u.Email)
	}
	desc := fmt.Sprintf("Interview for %s with %s.\n%s", f.Job.Title, candName, in.Notes)
	title := fmt.Sprintf("%s: %s (%s)", in.Title, candName, f.Job.Title)
	eventID, code, err := r.Deps.Calendar.CreateMeeting(ctx, a.OrgID, a.UserID, title, desc, in.StartsAt, in.EndsAt, attendees)
	if err != nil {
		return nil, err
	}
	var iid string
	err = r.pool.QueryRow(ctx, `INSERT INTO adrient.talent_interviews (org_id, application_id, title, starts_at, ends_at, interviewer_ids, calendar_event_id, meet_code, created_by)
		VALUES ($1,$2,$3,$4,$5,$6::uuid[],$7,$8,$9) RETURNING id::text`,
		a.OrgID, appID, in.Title, in.StartsAt, in.EndsAt, in.InterviewerIDs, eventID, code, a.UserID).Scan(&iid)
	if err != nil {
		_ = r.Deps.Calendar.DeleteEvent(ctx, a.OrgID, eventID)
		return nil, err
	}
	meetURL := ""
	if code != "" {
		meetURL = strings.TrimRight(baseURL, "/") + "/meet/" + code
	}
	sent := false
	if r.Deps.Mail != nil {
		when := in.StartsAt.Format("Mon 2 Jan 2006, 15:04 MST")
		text := fmt.Sprintf("Hello %s,\n\nWe would like to invite you to an interview for %s.\nWhen: %s\n", candName, f.Job.Title, when)
		h := fmt.Sprintf("<p>Hello %s,</p><p>We would like to invite you to an interview for <b>%s</b>.</p><p>When: <b>%s</b></p>",
			html.EscapeString(candName), html.EscapeString(f.Job.Title), html.EscapeString(when))
		if meetURL != "" {
			text += "Join: " + meetURL + "\n"
			h += fmt.Sprintf(`<p>Join the video call: <a href="%s">%s</a></p>`, html.EscapeString(meetURL), html.EscapeString(meetURL))
		}
		if err := r.Deps.Mail(ctx, []string{candEmail}, "Interview invitation: "+f.Job.Title, h, text); err == nil {
			sent = true
			_, _ = r.pool.Exec(ctx, `UPDATE adrient.talent_interviews SET mail_sent=true WHERE id=$1`, iid)
		}
	}
	r.audit(ctx, a, "interview.schedule", "talent_application", appID)
	_ = sent
	return r.row(ctx, `SELECT id::text AS id, title, starts_at, ends_at, status, meet_code, mail_sent, COALESCE(calendar_event_id::text,'') AS calendar_event_id
		FROM adrient.talent_interviews WHERE id=$1`, iid)
}

// CancelInterview cancels an interview and removes its Calendar event.
func (r *Repository) CancelInterview(ctx context.Context, a Actor, interviewID string) error {
	var appID, ev string
	err := r.pool.QueryRow(ctx, `SELECT application_id::text, COALESCE(calendar_event_id::text,'') FROM adrient.talent_interviews WHERE org_id=$1 AND id=$2 AND status='scheduled'`,
		a.OrgID, interviewID).Scan(&appID, &ev)
	if err != nil {
		return mapNoRows(err)
	}
	if _, err := r.leadApp(ctx, a, appID); err != nil {
		return err
	}
	if ev != "" && r.Deps.Calendar != nil {
		_ = r.Deps.Calendar.DeleteEvent(ctx, a.OrgID, ev)
	}
	_, err = r.pool.Exec(ctx, `UPDATE adrient.talent_interviews SET status='cancelled' WHERE id=$1`, interviewID)
	r.audit(ctx, a, "interview.cancel", "talent_interview", interviewID)
	return err
}
