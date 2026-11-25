package talent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Adrient-tech/Adrient-Workspace/internal/storage"
	"github.com/jackc/pgx/v5/pgxpool"
)

// fixture creates its own org and users in the database named by
// ADRIENT_TEST_DSN. It never drops or truncates anything and removes only its
// own org at the end.
type fixture struct {
	repo                      *Repository
	pool                      *pgxpool.Pool
	org, slug                 string
	hr, rec, interviewer, out Actor
	mgr, sub, peer, other     Actor
	mu                        sync.Mutex
	mails                     []string
	taskCalls                 []string
	resumes                   map[string][]byte
	events                    []string
}

type fakeResumes struct{ f *fixture }

func (s fakeResumes) Save(_ context.Context, _, _, folder, name, mime string, data []byte) (string, string, string, error) {
	s.f.mu.Lock()
	defer s.f.mu.Unlock()
	key := fmt.Sprintf("k%d", len(s.f.resumes))
	s.f.resumes[key] = data
	return "", key, "", nil
}
func (s fakeResumes) Open(_ context.Context, key string) (io.ReadCloser, string, int64, error) {
	b, ok := s.f.resumes[key]
	if !ok {
		return nil, "", 0, errors.New("missing")
	}
	return io.NopCloser(bytes.NewReader(b)), "application/pdf", int64(len(b)), nil
}

type fakeCal struct{ f *fixture }

func (c fakeCal) CreateMeeting(_ context.Context, _, _, title, _ string, _, _ time.Time, att []string) (string, string, error) {
	c.f.events = append(c.f.events, title+"|"+strings.Join(att, ","))
	return "", "abc-defg-hij", errors.New("no real calendar in this test")
}
func (c fakeCal) DeleteEvent(context.Context, string, string) error { return nil }

func newFixture(t *testing.T) *fixture {
	dsn := os.Getenv("ADRIENT_TEST_DSN")
	if dsn == "" {
		t.Skip("ADRIENT_TEST_DSN not set; skipping integration test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := storage.RunMigrations(ctx, pool); err != nil {
		t.Fatal(err)
	}
	f := &fixture{pool: pool, resumes: map[string][]byte{}}
	f.slug = fmt.Sprintf("talent-test-%d", time.Now().UnixNano())
	if err := pool.QueryRow(ctx, `INSERT INTO adrient.orgs (slug, display_name) VALUES ($1,'Talent Test') RETURNING id::text`, f.slug).Scan(&f.org); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		c := context.Background()
		_, _ = pool.Exec(c, `DELETE FROM adrient.talent_candidates WHERE org_id=$1`, f.org)
		_, _ = pool.Exec(c, `DELETE FROM adrient.talent_jobs WHERE org_id=$1`, f.org)
		_, _ = pool.Exec(c, `DELETE FROM adrient.talent_review_cycles WHERE org_id=$1`, f.org)
		_, _ = pool.Exec(c, `DELETE FROM adrient.talent_checklists WHERE org_id=$1`, f.org)
		_, _ = pool.Exec(c, `DELETE FROM adrient.users WHERE org_id=$1`, f.org)
		_, _ = pool.Exec(c, `DELETE FROM adrient.orgs WHERE id=$1`, f.org)
	})
	mk := func(name, manager string) Actor {
		var id string
		if err := pool.QueryRow(ctx, `INSERT INTO adrient.users (org_id, oidc_issuer, oidc_subject, email, display_name, manager_id)
			VALUES ($1,'test',$2,$3,$4,NULLIF($5,'')::uuid) RETURNING id::text`, f.org, f.slug+name, name+"@"+f.slug+".test", name, manager).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return Actor{OrgID: f.org, UserID: id}
	}
	f.hr = mk("hr", "")
	f.hr.IsHR = true
	f.mgr = mk("mgr", "")
	f.sub = mk("sub", f.mgr.UserID)
	f.peer = mk("peer", f.mgr.UserID)
	f.other = mk("other", "")
	f.rec = mk("rec", "")
	f.interviewer = mk("interviewer", "")
	f.out = mk("outsider", "")
	f.repo = NewRepository(pool)
	f.repo.Deps = Deps{
		Resumes:  fakeResumes{f},
		Calendar: fakeCal{f},
		Mail: func(_ context.Context, to []string, subject, _, _ string) error {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.mails = append(f.mails, strings.Join(to, ",")+"|"+subject)
			return nil
		},
		Tasks: func(_ context.Context, _, assignee, title, _ string, _ *time.Time) (string, error) {
			f.taskCalls = append(f.taskCalls, assignee+"|"+title)
			return "", errors.New("not stored in this test")
		},
	}
	return f
}

func (f *fixture) openJob(t *testing.T, openings int) string {
	t.Helper()
	ctx := context.Background()
	raw, err := f.repo.SaveJob(ctx, f.rec, "", JobInput{Title: "Engineer", Department: "R&D", Openings: openings, RecruiterID: f.rec.UserID, TeamIDs: []string{f.interviewer.UserID}})
	if err != nil {
		t.Fatal(err)
	}
	id := jsonField(raw, "id")
	// A non-HR creator needs approval before opening.
	if _, err := f.repo.SetJobStatus(ctx, f.rec, id, JobOpen); err == nil {
		t.Fatal("non-HR must not open an unapproved job")
	}
	if _, err := f.repo.RequestApproval(ctx, f.rec, id); err != nil {
		t.Fatal(err)
	}
	if _, err := f.repo.SetJobStatus(ctx, f.rec, id, JobOpen); err == nil {
		t.Fatal("pending approval must block opening")
	}
	if _, err := f.repo.DecideApproval(ctx, f.rec, id, true); !errors.Is(err, ErrForbidden) {
		t.Fatalf("non-HR approving: %v", err)
	}
	if _, err := f.repo.DecideApproval(ctx, f.hr, id, true); err != nil {
		t.Fatal(err)
	}
	if _, err := f.repo.SetJobStatus(ctx, f.rec, id, JobOpen); err != nil {
		t.Fatal(err)
	}
	return id
}

func jsonField(raw []byte, key string) string {
	s := string(raw)
	i := strings.Index(s, `"`+key+`":"`)
	if i < 0 {
		return ""
	}
	s = s[i+len(key)+4:]
	return s[:strings.Index(s, `"`)]
}

func TestCandidateDataVisibleOnlyToHiringTeam(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	job := f.openJob(t, 1)
	app, err := f.repo.CreateApplication(ctx, f.org, "", ApplyInput{JobID: job, Name: "Asha", Email: "asha@example.com", Consent: true,
		Resume: &ResumeFile{Name: "cv.pdf", Mime: "application/pdf", Data: []byte("%PDF-1 secret")}})
	if err != nil {
		t.Fatal(err)
	}
	// Hiring team (recruiter, interviewer, HR) sees the application; others do not.
	for name, a := range map[string]Actor{"recruiter": f.rec, "interviewer": f.interviewer, "hr": f.hr} {
		if _, err := f.repo.GetApplication(ctx, a, app); err != nil {
			t.Errorf("%s must see the application: %v", name, err)
		}
	}
	for name, a := range map[string]Actor{"outsider": f.out, "manager of nothing": f.mgr, "other": f.other} {
		if _, err := f.repo.GetApplication(ctx, a, app); !errors.Is(err, ErrNotFound) {
			t.Errorf("%s must get not found, got %v", name, err)
		}
		list, err := f.repo.ListApplications(ctx, a, "")
		if err != nil || strings.Contains(string(list), "Asha") {
			t.Errorf("%s list leaks candidates: %v %s", name, err, list)
		}
		if _, err := f.repo.GetJob(ctx, a, job); !errors.Is(err, ErrNotFound) {
			t.Errorf("%s must not see the job: %v", name, err)
		}
		cand := candidateOf(t, f, app)
		if _, rc, _, err := f.repo.OpenResume(ctx, a, cand); err == nil {
			rc.Close()
			t.Errorf("%s must not download the resume", name)
		}
	}
	cand := candidateOf(t, f, app)
	if _, rc, _, err := f.repo.OpenResume(ctx, f.interviewer, cand); err != nil {
		t.Errorf("interviewer resume: %v", err)
	} else {
		rc.Close()
	}
	// An interviewer cannot move, reject, hire, offer or schedule.
	if _, err := f.repo.MoveApplication(ctx, f.interviewer, app, "interview", ""); !errors.Is(err, ErrForbidden) {
		t.Errorf("interviewer move: %v", err)
	}
	if _, err := f.repo.RejectApplication(ctx, f.interviewer, app, "no"); !errors.Is(err, ErrForbidden) {
		t.Errorf("interviewer reject: %v", err)
	}
	if _, err := f.repo.CreateOffer(ctx, f.interviewer, app, OfferInput{Salary: 1}); !errors.Is(err, ErrForbidden) {
		t.Errorf("interviewer offer: %v", err)
	}
	if _, err := f.repo.ScheduleInterview(ctx, f.interviewer, app, "http://x", InterviewInput{}); !errors.Is(err, ErrForbidden) {
		t.Errorf("interviewer schedule: %v", err)
	}
	// Pool, reports, new-hire, erasure and settings are HR-only.
	if _, err := f.repo.ListPool(ctx, f.rec); !errors.Is(err, ErrForbidden) {
		t.Errorf("pool: %v", err)
	}
	if _, err := f.repo.Reports(ctx, f.rec, ""); !errors.Is(err, ErrForbidden) {
		t.Errorf("reports: %v", err)
	}
	if _, err := f.repo.ConvertToNewHire(ctx, f.rec, app); !errors.Is(err, ErrForbidden) {
		t.Errorf("new hire: %v", err)
	}
	if err := f.repo.DeleteCandidate(ctx, f.rec, cand); !errors.Is(err, ErrForbidden) {
		t.Errorf("erase: %v", err)
	}
	if _, err := f.repo.SaveSettings(ctx, f.rec, Settings{}); !errors.Is(err, ErrForbidden) {
		t.Errorf("settings: %v", err)
	}
	// Another org's HR cannot reach this org's data (org scoping).
	foreign := Actor{OrgID: "00000000-0000-0000-0000-000000000001", UserID: f.hr.UserID, IsHR: true}
	if _, err := f.repo.GetApplication(ctx, foreign, app); !errors.Is(err, ErrNotFound) {
		t.Errorf("cross-org: %v", err)
	}
}

func candidateOf(t *testing.T, f *fixture, app string) string {
	var c string
	if err := f.pool.QueryRow(context.Background(), `SELECT candidate_id::text FROM adrient.talent_applications WHERE id=$1`, app).Scan(&c); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestScorecardsStayIndependent(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	job := f.openJob(t, 1)
	app, _ := f.repo.CreateApplication(ctx, f.org, "", ApplyInput{JobID: job, Name: "B", Email: "b@example.com"})
	if _, err := f.repo.SaveScorecard(ctx, f.rec, app, ScorecardInput{Overall: 5, Recommendation: "strong_yes", Comment: "recruiter view"}); err != nil {
		t.Fatal(err)
	}
	raw, _ := f.repo.GetApplication(ctx, f.interviewer, app)
	if strings.Contains(string(raw), "recruiter view") {
		t.Error("interviewer saw another scorecard before submitting their own")
	}
	if _, err := f.repo.SaveScorecard(ctx, f.interviewer, app, ScorecardInput{Overall: 3, Recommendation: "yes"}); err != nil {
		t.Fatal(err)
	}
	raw, _ = f.repo.GetApplication(ctx, f.interviewer, app)
	if !strings.Contains(string(raw), "recruiter view") {
		t.Error("after submitting, the interviewer should see the others")
	}
	if _, err := f.repo.SaveScorecard(ctx, f.out, app, ScorecardInput{Overall: 3, Recommendation: "yes"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("outsider scorecard: %v", err)
	}
	if _, err := f.repo.SaveScorecard(ctx, f.rec, app, ScorecardInput{Overall: 9, Recommendation: "yes"}); err == nil {
		t.Error("rating 9 must fail")
	}
}

func TestPipelineRules(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	job := f.openJob(t, 1)
	in := ApplyInput{JobID: job, Name: "C", Email: "dup@example.com", Phone: "1", Source: "referral", SourceDetail: "Sam"}
	app, err := f.repo.CreateApplication(ctx, f.org, f.rec.UserID, in)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.repo.CreateApplication(ctx, f.org, f.rec.UserID, ApplyInput{JobID: job, Name: "C again", Email: "DUP@example.com"}); !errors.Is(err, ErrDuplicate) {
		t.Errorf("same email same job: %v", err)
	}
	d, err := f.repo.CheckDuplicate(ctx, f.rec, "Dup@Example.com")
	if err != nil || d == nil || d.Applications != 1 {
		t.Errorf("duplicate check: %+v %v", d, err)
	}
	// A second job: same person reuses the one candidate record.
	job2 := f.openJob(t, 1)
	if _, err := f.repo.CreateApplication(ctx, f.org, "", ApplyInput{JobID: job2, Name: "C", Email: "dup@example.com"}); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = f.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.talent_candidates WHERE org_id=$1 AND lower(email)='dup@example.com'`, f.org).Scan(&n)
	if n != 1 {
		t.Errorf("duplicate candidate rows: %d", n)
	}
	if _, err := f.repo.MoveApplication(ctx, f.rec, app, "screening", "ok"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.repo.MoveApplication(ctx, f.rec, app, "bogus", ""); !errors.Is(err, ErrInvalid) {
		t.Errorf("bogus stage: %v", err)
	}
	if _, err := f.repo.RejectApplication(ctx, f.rec, app, " "); !errors.Is(err, ErrInvalid) {
		t.Errorf("reject needs reason: %v", err)
	}
	if _, err := f.repo.HireApplication(ctx, f.rec, app); err != nil {
		t.Fatal(err)
	}
	var status string
	_ = f.pool.QueryRow(ctx, `SELECT status FROM adrient.talent_jobs WHERE id=$1`, job).Scan(&status)
	if status != JobClosed {
		t.Errorf("filling the only opening must close the job, got %s", status)
	}
	if _, err := f.repo.MoveApplication(ctx, f.rec, app, "screening", ""); !errors.Is(err, ErrInvalid) {
		t.Errorf("closed application cannot move: %v", err)
	}
	// Applying to a closed job through the public path is refused.
	if _, err := f.repo.CreateApplication(ctx, f.org, "", ApplyInput{JobID: job, Name: "D", Email: "d@example.com"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("closed job public apply: %v", err)
	}
	nh, err := f.repo.ConvertToNewHire(ctx, f.hr, app)
	if err != nil || nh.Name != "C" || nh.Email != "dup@example.com" || nh.JobTitle != "Engineer" || nh.Department != "R&D" {
		t.Errorf("new hire: %+v %v", nh, err)
	}
	rep, err := f.repo.Reports(ctx, f.hr, job)
	if err != nil || len(rep.TimeToHire) != 1 || rep.Funnel[len(rep.Funnel)-1].Count != 1 || rep.Sources[0].Source != "referral" {
		t.Errorf("reports: %+v %v", rep, err)
	}
}

func TestInterviewCreatesCalendarEventAndOfferFlow(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	job := f.openJob(t, 2)
	app, _ := f.repo.CreateApplication(ctx, f.org, "", ApplyInput{JobID: job, Name: "E", Email: "e@example.com"})
	_, err := f.repo.ScheduleInterview(ctx, f.rec, app, "http://x", InterviewInput{StartsAt: time.Now().Add(time.Hour), EndsAt: time.Now().Add(2 * time.Hour), InterviewerIDs: []string{f.interviewer.UserID}})
	if err == nil || len(f.events) != 1 || !strings.Contains(f.events[0], "e@example.com") || !strings.Contains(f.events[0], "interviewer@") {
		t.Errorf("calendar must be asked once with candidate and interviewer: %v %v", f.events, err)
	}
	// Past and inverted times are refused before the calendar is touched.
	before := len(f.events)
	if _, err := f.repo.ScheduleInterview(ctx, f.rec, app, "http://x", InterviewInput{StartsAt: time.Now().Add(-48 * time.Hour), EndsAt: time.Now().Add(-47 * time.Hour)}); !errors.Is(err, ErrInvalid) {
		t.Errorf("past interview: %v", err)
	}
	if len(f.events) != before {
		t.Error("calendar called for an invalid interview")
	}
	off, err := f.repo.CreateOffer(ctx, f.rec, app, OfferInput{Salary: 1200000, StartDate: "2027-01-10"})
	if err != nil {
		t.Fatal(err)
	}
	tok, oid := jsonField(off, "accept_token"), jsonField(off, "id")
	if _, _, err := f.repo.GetPublicOffer(ctx, tok); !errors.Is(err, ErrNotFound) {
		t.Errorf("a draft offer must not be public: %v", err)
	}
	if err := f.repo.RespondOffer(ctx, tok, true, "E"); err == nil {
		t.Error("cannot accept a draft")
	}
	if _, err := f.repo.SendOffer(ctx, f.rec, oid, "http://x"); err != nil {
		t.Fatal(err)
	}
	if len(f.mails) == 0 || !strings.Contains(f.mails[len(f.mails)-1], "e@example.com") {
		t.Errorf("offer mail not sent: %v", f.mails)
	}
	if err := f.repo.RespondOffer(ctx, tok, true, " "); !errors.Is(err, ErrInvalid) {
		t.Errorf("accept needs a typed name: %v", err)
	}
	if err := f.repo.RespondOffer(ctx, tok, true, "E Person"); err != nil {
		t.Fatal(err)
	}
	if err := f.repo.RespondOffer(ctx, tok, false, ""); !errors.Is(err, ErrInvalid) {
		t.Errorf("an answered offer cannot be changed: %v", err)
	}
	if _, err := f.repo.ConvertToNewHire(ctx, f.hr, app); err != nil {
		t.Errorf("accepted offer converts: %v", err)
	}
	data, err := f.repo.PublicOfferPDF(ctx, tok)
	if err != nil || !bytes.HasPrefix(data, []byte("%PDF")) {
		t.Errorf("public pdf: %v", err)
	}
}

func multipartApply(t *testing.T, fields map[string]string, file []byte, fname string) (*bytes.Buffer, string) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for k, v := range fields {
		_ = w.WriteField(k, v)
	}
	if file != nil {
		fw, _ := w.CreateFormFile("resume", fname)
		_, _ = fw.Write(file)
	}
	_ = w.Close()
	return &buf, w.FormDataContentType()
}

func TestPublicApplyAbuseLimits(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	job := f.openJob(t, 5)
	h := NewPublicHandler(f.repo)
	url := "/api/v1/public/talent/" + f.slug + "/jobs/" + job + "/apply"
	old := strconv.FormatInt(time.Now().Add(-time.Minute).UnixMilli(), 10)
	post := func(ip string, fields map[string]string, file []byte, fname string) *httptest.ResponseRecorder {
		body, ct := multipartApply(t, fields, file, fname)
		req := httptest.NewRequest(http.MethodPost, url, body)
		req.Header.Set("Content-Type", ct)
		req.RemoteAddr = ip + ":1234"
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		return rr
	}
	good := func(email string) map[string]string {
		return map[string]string{"name": "P", "email": email, "consent": "true", "started_at": old}
	}
	count := func() int {
		var n int
		_ = f.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.talent_applications WHERE org_id=$1`, f.org).Scan(&n)
		return n
	}
	// consent missing
	nc := good("nc@example.com")
	nc["consent"] = ""
	if rr := post("10.1.1.1", nc, nil, ""); rr.Code != 400 {
		t.Errorf("no consent: %d", rr.Code)
	}
	// honeypot filled: looks like success, stores nothing
	hp := good("hp@example.com")
	hp["website"] = "http://spam"
	if rr := post("10.1.1.2", hp, nil, ""); rr.Code != 200 || count() != 0 {
		t.Errorf("honeypot: %d stored=%d", rr.Code, count())
	}
	// submitted instantly: dropped
	fast := good("fast@example.com")
	fast["started_at"] = strconv.FormatInt(time.Now().UnixMilli(), 10)
	if rr := post("10.1.1.3", fast, nil, ""); rr.Code != 200 || count() != 0 {
		t.Errorf("too fast: %d stored=%d", rr.Code, count())
	}
	// missing timer
	nt := good("nt@example.com")
	delete(nt, "started_at")
	if rr := post("10.1.1.4", nt, nil, ""); rr.Code != 400 {
		t.Errorf("no timer: %d", rr.Code)
	}
	// bad resume content / type
	if rr := post("10.1.1.5", good("bad@example.com"), []byte("MZ\x90"), "cv.pdf"); rr.Code != 400 {
		t.Errorf("fake pdf: %d", rr.Code)
	}
	if rr := post("10.1.1.5", good("bad2@example.com"), []byte("x"), "run.exe"); rr.Code != 400 {
		t.Errorf("exe: %d", rr.Code)
	}
	// a real application works and stores the resume
	if rr := post("10.1.1.6", good("ok@example.com"), []byte("%PDF-1.4 ok"), "cv.pdf"); rr.Code != 200 || count() != 1 || len(f.resumes) != 1 {
		t.Errorf("good apply: %d stored=%d resumes=%d", rr.Code, count(), len(f.resumes))
	}
	// the same email again is quiet success but not stored twice
	if rr := post("10.1.1.7", good("ok@example.com"), nil, ""); rr.Code != 200 || count() != 1 {
		t.Errorf("repeat: %d stored=%d", rr.Code, count())
	}
	// per-email daily limit: 3 attempts, the 4th is refused
	for i := 0; i < 3; i++ {
		post(fmt.Sprintf("10.2.2.%d", i), good("many@example.com"), nil, "")
	}
	if rr := post("10.2.2.9", good("many@example.com"), nil, ""); rr.Code != 429 {
		t.Errorf("email limit: %d", rr.Code)
	}
	// per-IP hourly limit: the 6th request from one address is refused
	codes := []int{}
	for i := 0; i < 7; i++ {
		codes = append(codes, post("10.3.3.3", good(fmt.Sprintf("ip%d@example.com", i)), nil, "").Code)
	}
	if codes[4] != 200 || codes[5] != 429 || codes[6] != 429 {
		t.Errorf("ip limit codes: %v", codes)
	}
	// unknown org / job and closed job are plain 404
	req := httptest.NewRequest(http.MethodGet, "/api/v1/public/talent/no-such-org/jobs", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 404 {
		t.Errorf("unknown org: %d", rr.Code)
	}
}

func TestPerformanceVisibility(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	raw, err := f.repo.SaveCycle(ctx, f.hr, CycleInput{Name: "H1", StartDate: "2026-01-01", EndDate: "2026-06-30", Kinds: []string{"self", "manager", "peer"}})
	if err != nil {
		t.Fatal(err)
	}
	cyc := jsonField(raw, "id")
	if _, err := f.repo.SaveCycle(ctx, f.sub, CycleInput{Name: "x"}); !errors.Is(err, ErrForbidden) {
		t.Errorf("non-HR cycle: %v", err)
	}
	if _, err := f.repo.SetCycleStatus(ctx, f.hr, cyc, "published"); !errors.Is(err, ErrInvalid) {
		t.Errorf("cannot skip states: %v", err)
	}
	if _, err := f.repo.SetCycleStatus(ctx, f.hr, cyc, "active"); err != nil {
		t.Fatal(err)
	}
	if err := f.repo.AssignPeers(ctx, f.other, cyc, f.sub.UserID, []string{f.peer.UserID}); !errors.Is(err, ErrNotFound) {
		t.Errorf("a stranger cannot assign peers: %v", err)
	}
	if err := f.repo.AssignPeers(ctx, f.mgr, cyc, f.sub.UserID, []string{f.peer.UserID}); err != nil {
		t.Fatal(err)
	}
	reviewIDs := map[string]string{}
	tasks := func(a Actor) string { r, _ := f.repo.MyReviewTasks(ctx, a); return string(r) }
	for name, a := range map[string]Actor{"sub": f.sub, "mgr": f.mgr, "peer": f.peer} {
		_ = name
		_ = tasks(a)
	}
	rows, _ := f.pool.Query(ctx, `SELECT id::text, kind, reviewer_id::text FROM adrient.talent_reviews WHERE cycle_id=$1 AND subject_id=$2`, cyc, f.sub.UserID)
	for rows.Next() {
		var id, kind, rv string
		_ = rows.Scan(&id, &kind, &rv)
		reviewIDs[kind] = id
	}
	rows.Close()
	if len(reviewIDs) != 3 {
		t.Fatalf("expected self, manager and peer reviews, got %v", reviewIDs)
	}
	// Only the reviewer can write a review.
	if err := f.repo.SubmitReview(ctx, f.sub, reviewIDs["manager"], SubmitInput{Rating: 5}); !errors.Is(err, ErrNotFound) {
		t.Errorf("subject writing the manager review: %v", err)
	}
	if err := f.repo.SubmitReview(ctx, f.mgr, reviewIDs["manager"], SubmitInput{Rating: 9}); !errors.Is(err, ErrInvalid) {
		t.Errorf("rating outside the scale: %v", err)
	}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(f.repo.SubmitReview(ctx, f.sub, reviewIDs["self"], SubmitInput{Rating: 4, Comment: "SELF-TEXT"}))
	must(f.repo.SubmitReview(ctx, f.mgr, reviewIDs["manager"], SubmitInput{Rating: 3, Comment: "MANAGER-TEXT"}))
	must(f.repo.SubmitReview(ctx, f.peer, reviewIDs["peer"], SubmitInput{Rating: 5, Comment: "PEER-TEXT"}))

	res := func(a Actor) (Result, error) { return f.repo.GetResult(ctx, a, cyc, f.sub.UserID) }
	// Before publishing: the subject sees only their own self review.
	r, err := res(f.sub)
	must(err)
	if r.Self == nil || r.Manager == nil || r.Manager.Rating != nil || r.Manager.Comment != "" || len(r.Peers) != 0 || r.Final != nil || r.PeerAvg != nil {
		t.Errorf("subject before publish leaks: %+v", r)
	}
	// The manager sees everything; peers are anonymous.
	r, err = res(f.mgr)
	must(err)
	if r.Self == nil || r.Manager == nil || len(r.Peers) != 1 || r.Peers[0].Reviewer != "" || r.Peers[0].Comment != "PEER-TEXT" {
		t.Errorf("manager view: %+v", r)
	}
	// HR sees reviewer names.
	r, err = res(f.hr)
	must(err)
	if len(r.Peers) != 1 || r.Peers[0].Reviewer != "peer" {
		t.Errorf("hr view: %+v", r)
	}
	// Peer, colleague and outsider see nothing.
	for name, a := range map[string]Actor{"peer": f.peer, "other": f.other, "outsider": f.out} {
		if _, err := res(a); !errors.Is(err, ErrNotFound) {
			t.Errorf("%s must not see the result: %v", name, err)
		}
	}
	// Calibration is HR only and wins as the final rating.
	if err := f.repo.SetCalibrated(ctx, f.mgr, cyc, f.sub.UserID, 4); !errors.Is(err, ErrForbidden) {
		t.Errorf("manager calibrating: %v", err)
	}
	must(f.repo.SetCalibrated(ctx, f.hr, cyc, f.sub.UserID, 4))
	if _, err := f.repo.Calibration(ctx, f.mgr, cyc); !errors.Is(err, ErrForbidden) {
		t.Errorf("calibration table to manager: %v", err)
	}
	r, _ = res(f.sub)
	if r.Calibrated != nil || r.Final != nil {
		t.Errorf("calibrated rating visible before publish: %+v", r)
	}
	for _, s := range []string{"closed", "published"} {
		if _, err := f.repo.SetCycleStatus(ctx, f.hr, cyc, s); err != nil {
			t.Fatal(err)
		}
	}
	r, _ = res(f.sub)
	if r.Manager == nil || r.Manager.Comment != "MANAGER-TEXT" || r.Final == nil || *r.Final != 4 || len(r.Peers) != 1 || r.Peers[0].Reviewer != "" || r.PeerAvg == nil {
		t.Errorf("subject after publish: %+v", r)
	}
	if err := f.repo.SubmitReview(ctx, f.sub, reviewIDs["self"], SubmitInput{Rating: 1}); !errors.Is(err, ErrInvalid) {
		t.Errorf("editing after the cycle closed: %v", err)
	}
}

func TestOneOnOnesAndFeedbackPrivacy(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := f.repo.SaveOneOnOne(ctx, f.other, "", OneOnOneInput{WithID: f.sub.UserID, Date: "2026-03-01", Notes: "x"}); !errors.Is(err, ErrInvalid) {
		t.Errorf("1:1 with a non-report: %v", err)
	}
	raw, err := f.repo.SaveOneOnOne(ctx, f.mgr, "", OneOnOneInput{WithID: f.sub.UserID, Date: "2026-03-01", Notes: "PRIVATE-NOTE"})
	if err != nil {
		t.Fatal(err)
	}
	id := jsonField(raw, "id")
	for name, a := range map[string]Actor{"manager": f.mgr, "report": f.sub} {
		l, _ := f.repo.ListOneOnOnes(ctx, a, "")
		if !strings.Contains(string(l), "PRIVATE-NOTE") {
			t.Errorf("%s must see their 1:1", name)
		}
	}
	for name, a := range map[string]Actor{"hr": f.hr, "peer": f.peer, "other": f.other} {
		l, _ := f.repo.ListOneOnOnes(ctx, a, "")
		if strings.Contains(string(l), "PRIVATE-NOTE") {
			t.Errorf("%s must not read someone else's 1:1", name)
		}
	}
	if _, err := f.repo.SaveOneOnOne(ctx, f.hr, id, OneOnOneInput{Date: "2026-03-02", Notes: "tamper"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("HR editing a 1:1: %v", err)
	}
	// Feedback: manager asks the peer about the report; only the asker (and HR) read the answer.
	if err := f.repo.RequestFeedback(ctx, f.other, f.sub.UserID, []string{f.peer.UserID}, "?"); !errors.Is(err, ErrNotFound) {
		t.Errorf("stranger requesting feedback: %v", err)
	}
	if err := f.repo.RequestFeedback(ctx, f.mgr, f.sub.UserID, []string{f.peer.UserID}, "How is sub doing?"); err != nil {
		t.Fatal(err)
	}
	pending, _ := f.repo.FeedbackToGive(ctx, f.peer)
	fid := jsonField(pending, "id")
	if fid == "" {
		t.Fatal("peer has no pending request")
	}
	if err := f.repo.AnswerFeedback(ctx, f.other, fid, "hijack", false); !errors.Is(err, ErrNotFound) {
		t.Errorf("answering someone else's request: %v", err)
	}
	if err := f.repo.AnswerFeedback(ctx, f.peer, fid, "FB-TEXT", false); err != nil {
		t.Fatal(err)
	}
	for name, a := range map[string]Actor{"asker": f.mgr, "hr": f.hr} {
		l, _ := f.repo.FeedbackReceived(ctx, a)
		if !strings.Contains(string(l), "FB-TEXT") {
			t.Errorf("%s must see the answer", name)
		}
	}
	for name, a := range map[string]Actor{"subject": f.sub, "other": f.other} {
		l, _ := f.repo.FeedbackReceived(ctx, a)
		if strings.Contains(string(l), "FB-TEXT") {
			t.Errorf("%s must not see feedback they did not request", name)
		}
	}
}

func TestGoalVisibility(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	for _, vis := range []string{"private", "team", "company"} {
		if _, err := f.repo.SaveGoal(ctx, f.sub, "", GoalInput{Title: "G-" + vis, Visibility: vis, Target: 10}); err != nil {
			t.Fatal(err)
		}
	}
	titles := func(a Actor) string { r, _ := f.repo.ListGoals(ctx, a, f.sub.UserID); return string(r) }
	has := func(s, vis string) bool { return strings.Contains(s, "G-"+vis) }
	for name, c := range map[string]struct {
		a                Actor
		priv, team, comp bool
	}{
		"owner": {f.sub, true, true, true}, "manager": {f.mgr, true, true, true}, "hr": {f.hr, true, true, true},
		"teammate": {f.peer, false, true, true}, "stranger": {f.other, false, false, true},
	} {
		s := titles(c.a)
		if has(s, "private") != c.priv || has(s, "team") != c.team || has(s, "company") != c.comp {
			t.Errorf("%s sees wrong goals: %s", name, s)
		}
	}
	raw, _ := f.repo.ListGoals(ctx, f.sub, "")
	id := jsonField(raw, "id")
	if _, err := f.repo.SaveGoal(ctx, f.other, id, GoalInput{Title: "hijack", Target: 1}); !errors.Is(err, ErrNotFound) {
		t.Errorf("editing another's goal: %v", err)
	}
	if err := f.repo.DeleteGoal(ctx, f.other, id); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleting another's goal: %v", err)
	}
	// Key result progress.
	obj, _ := f.repo.SaveGoal(ctx, f.sub, "", GoalInput{Title: "Obj", Visibility: "team"})
	cur := 30.0
	kr, err := f.repo.SaveGoal(ctx, f.sub, "", GoalInput{Title: "KR", ParentID: jsonField(obj, "id"), Target: 60, Current: &cur})
	if err != nil || !strings.Contains(string(kr), `"key_result"`) {
		t.Errorf("key result: %v %s", err, kr)
	}
	l, _ := f.repo.ListGoals(ctx, f.sub, "")
	if !strings.Contains(string(l), `"progress":50`) {
		t.Errorf("progress not computed: %s", l)
	}
}

func TestOnboardingChecklist(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	tpl, err := f.repo.SaveTemplate(ctx, f.hr, "", TemplateInput{Kind: "onboarding", Name: "Standard", Tasks: []TemplateTask{
		{Title: "Order laptop", Role: "it", DueOffset: -5},
		{Title: "Welcome chat", Role: "manager", DueOffset: 0},
		{Title: "Sign handbook", Role: "employee", DueOffset: 1},
		{Title: "Contract", Role: "hr", DueOffset: -2},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.repo.SaveTemplate(ctx, f.rec, "", TemplateInput{Kind: "onboarding", Name: "x", Tasks: []TemplateTask{{Title: "a", Role: "hr"}}}); !errors.Is(err, ErrForbidden) {
		t.Errorf("non-HR template: %v", err)
	}
	start := time.Now().AddDate(0, 0, 3).Format("2006-01-02")
	if _, err := f.repo.StartChecklist(ctx, f.rec, StartInput{TemplateID: jsonField(tpl, "id"), SubjectUserID: f.sub.UserID, AnchorDate: start}); !errors.Is(err, ErrForbidden) {
		t.Errorf("non-HR start: %v", err)
	}
	cl, err := f.repo.StartChecklist(ctx, f.hr, StartInput{TemplateID: jsonField(tpl, "id"), SubjectUserID: f.sub.UserID, AnchorDate: start, ITID: f.interviewer.UserID})
	if err != nil {
		t.Fatal(err)
	}
	cid := jsonField(cl, "id")
	// Tasks app is asked for each assigned task, with the manager taken from the directory.
	joined := strings.Join(f.taskCalls, "\n")
	for _, want := range []string{f.interviewer.UserID + "|Order laptop", f.mgr.UserID + "|Welcome chat", f.sub.UserID + "|Sign handbook", f.hr.UserID + "|Contract"} {
		if !strings.Contains(joined, want) {
			t.Errorf("task not created: %s\n%s", want, joined)
		}
	}
	// Visibility: assignees see it, strangers do not.
	for name, a := range map[string]Actor{"employee": f.sub, "manager": f.mgr, "it": f.interviewer} {
		if _, err := f.repo.GetChecklist(ctx, a, cid); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := f.repo.GetChecklist(ctx, f.other, cid); !errors.Is(err, ErrNotFound) {
		t.Errorf("stranger: %v", err)
	}
	// Only the assignee, manager or HR tick a task.
	var laptop string
	_ = f.pool.QueryRow(ctx, `SELECT id::text FROM adrient.talent_checklist_tasks WHERE checklist_id=$1 AND title='Order laptop'`, cid).Scan(&laptop)
	if _, err := f.repo.SetTaskDone(ctx, f.sub, laptop, true); !errors.Is(err, ErrNotFound) {
		t.Errorf("employee ticking the IT task: %v", err)
	}
	if _, err := f.repo.SetTaskDone(ctx, f.interviewer, laptop, true); err != nil {
		t.Errorf("assignee ticking: %v", err)
	}
	// Overdue reminders mail the assignee once.
	_, _ = f.pool.Exec(ctx, `UPDATE adrient.talent_checklist_tasks SET due_date=current_date-2 WHERE checklist_id=$1 AND title='Contract'`, cid)
	n, err := f.repo.RemindOverdue(ctx, f.hr)
	if err != nil || n != 1 {
		t.Errorf("reminders: %d %v", n, err)
	}
	if n, _ := f.repo.RemindOverdue(ctx, f.hr); n != 0 {
		t.Errorf("reminded twice on the same day: %d", n)
	}
	// Offboarding: the IT task deep-links to the admin Users page; exit interview is HR only.
	off, _ := f.repo.SaveTemplate(ctx, f.hr, "", TemplateInput{Kind: "offboarding", Name: "Exit", Tasks: []TemplateTask{{Title: "Remove access", Role: "it"}}})
	oc, err := f.repo.StartChecklist(ctx, f.hr, StartInput{TemplateID: jsonField(off, "id"), SubjectUserID: f.sub.UserID, AnchorDate: start})
	if err != nil || !strings.Contains(string(oc), "/admin/users") {
		t.Errorf("offboarding link: %v %s", err, oc)
	}
	ocid := jsonField(oc, "id")
	if _, err := f.repo.SaveExitInterview(ctx, f.mgr, ocid, ExitInput{Reason: "x"}); !errors.Is(err, ErrForbidden) {
		t.Errorf("manager exit interview: %v", err)
	}
	if _, err := f.repo.SaveExitInterview(ctx, f.hr, ocid, ExitInput{Reason: "better offer"}); err != nil {
		t.Error(err)
	}
	got, _ := f.repo.GetChecklist(ctx, f.mgr, ocid)
	if strings.Contains(string(got), "better offer") {
		// the manager sees the checklist but never the exit interview
		t.Error("exit interview must stay with HR")
	}
	if _, err := f.repo.AddAsset(ctx, f.hr, ocid, AssetInput{Name: "Laptop", Serial: "SN1"}); err != nil {
		t.Error(err)
	}
}

func TestLearningAndPolicies(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	c, err := f.repo.SaveCourse(ctx, f.hr, "", CourseInput{Title: "Security basics", Link: "https://example.com/c", Mandatory: true})
	if err != nil {
		t.Fatal(err)
	}
	cid := jsonField(c, "id")
	if _, err := f.repo.SaveCourse(ctx, f.sub, "", CourseInput{Title: "x"}); !errors.Is(err, ErrForbidden) {
		t.Errorf("non-HR course: %v", err)
	}
	if _, err := f.repo.SaveCourse(ctx, f.hr, "", CourseInput{Title: "bad", Link: "javascript:alert(1)"}); !errors.Is(err, ErrInvalid) {
		t.Errorf("javascript link: %v", err)
	}
	if _, err := f.repo.Enroll(ctx, f.sub, cid, []string{f.peer.UserID}, false, ""); !errors.Is(err, ErrForbidden) {
		t.Errorf("enrolling someone else: %v", err)
	}
	if n, err := f.repo.Enroll(ctx, f.sub, cid, nil, false, ""); err != nil || n != 1 {
		t.Errorf("self enroll: %d %v", n, err)
	}
	if err := f.repo.SetProgress(ctx, f.sub, cid, "completed"); err != nil {
		t.Error(err)
	}
	if err := f.repo.SetProgress(ctx, f.peer, cid, "completed"); !errors.Is(err, ErrNotFound) {
		t.Errorf("completing without enrolment: %v", err)
	}
	tr, err := f.repo.CourseTracking(ctx, f.hr, cid)
	if err != nil || !strings.Contains(string(tr), `"completed"`) {
		t.Errorf("tracking: %v %s", err, tr)
	}
	if _, err := f.repo.CourseTracking(ctx, f.sub, cid); !errors.Is(err, ErrForbidden) {
		t.Errorf("tracking for non-HR: %v", err)
	}
	// Policies: acknowledgement is per version.
	p, err := f.repo.SavePolicy(ctx, f.hr, "", PolicyInput{Title: "Code of conduct", Link: "/docs/abc", Mandatory: true})
	if err != nil {
		t.Fatal(err)
	}
	pid := jsonField(p, "id")
	if err := f.repo.Acknowledge(ctx, f.sub, pid); err != nil {
		t.Fatal(err)
	}
	acked := func(a Actor) bool {
		l, _ := f.repo.ListPolicies(ctx, a)
		return strings.Contains(string(l), `"acknowledged":true`)
	}
	if !acked(f.sub) || acked(f.peer) {
		t.Error("ack must be per person")
	}
	if _, err := f.repo.SavePolicy(ctx, f.hr, pid, PolicyInput{Title: "Code of conduct", Link: "/docs/abc", Mandatory: true, NewVersion: true}); err != nil {
		t.Fatal(err)
	}
	if acked(f.sub) {
		t.Error("a new version must need a new acknowledgement")
	}
	trk, _ := f.repo.PolicyTracking(ctx, f.hr, pid)
	if !strings.Contains(string(trk), `"acknowledged":false`) {
		t.Errorf("tracking: %s", trk)
	}
	if _, err := f.repo.PolicyTracking(ctx, f.sub, pid); !errors.Is(err, ErrForbidden) {
		t.Errorf("tracking for non-HR: %v", err)
	}
	// Audience-limited policy is invisible and not acknowledgeable by others.
	ap, _ := f.repo.SavePolicy(ctx, f.hr, "", PolicyInput{Title: "Managers only", Link: "https://example.com/m", Audience: []string{f.mgr.UserID}})
	if err := f.repo.Acknowledge(ctx, f.other, jsonField(ap, "id")); !errors.Is(err, ErrNotFound) {
		t.Errorf("outside audience: %v", err)
	}
	if l, _ := f.repo.ListPolicies(ctx, f.other); strings.Contains(string(l), "Managers only") {
		t.Error("policy outside audience listed")
	}
	// Announcements and kudos.
	if _, err := f.repo.PostAnnouncement(ctx, f.sub, AnnouncementInput{Title: "x"}); !errors.Is(err, ErrForbidden) {
		t.Errorf("non-HR announcement: %v", err)
	}
	if _, err := f.repo.PostAnnouncement(ctx, f.hr, AnnouncementInput{Title: "Office closed Friday", Pinned: true}); err != nil {
		t.Error(err)
	}
	if err := f.repo.GiveKudos(ctx, f.sub, f.sub.UserID, "thanks", "me"); !errors.Is(err, ErrInvalid) {
		t.Errorf("kudos to self: %v", err)
	}
	if err := f.repo.GiveKudos(ctx, f.sub, f.peer.UserID, "teamwork", "Thanks for the help"); err != nil {
		t.Error(err)
	}
	feed, err := f.repo.Feed(ctx, f.other)
	if err != nil || !strings.Contains(string(feed), "Office closed Friday") || !strings.Contains(string(feed), "Thanks for the help") {
		t.Errorf("feed: %v %s", err, feed)
	}
	if err := f.repo.SetBirthday(ctx, f.sub, f.peer.UserID, 3, 4); !errors.Is(err, ErrForbidden) {
		t.Errorf("setting another's birthday: %v", err)
	}
	soon := time.Now().AddDate(0, 0, 5)
	if err := f.repo.SetBirthday(ctx, f.sub, "", int(soon.Month()), soon.Day()); err != nil {
		t.Error(err)
	}
	if err := f.repo.SetBirthday(ctx, f.sub, "", 2, 31); !errors.Is(err, ErrInvalid) {
		t.Errorf("31 Feb: %v", err)
	}
	_, _ = f.pool.Exec(ctx, `UPDATE adrient.users SET start_date=$2 WHERE id=$1`, f.peer.UserID, soon.AddDate(-3, 0, 0))
	cel, err := f.repo.Celebrations(ctx, f.other, 30)
	if err != nil || !strings.Contains(string(cel), `"birthday"`) || !strings.Contains(string(cel), `"anniversary"`) || !strings.Contains(string(cel), `"years":3`) {
		t.Errorf("celebrations: %v %s", err, cel)
	}
}
