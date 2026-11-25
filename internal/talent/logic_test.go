package talent

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestValidateStages(t *testing.T) {
	if err := ValidateStages([]Stage{{ID: "a", Name: "A"}}); err == nil {
		t.Error("one stage must fail")
	}
	if err := ValidateStages([]Stage{{ID: "hired", Name: "H"}, {ID: "b", Name: "B"}}); err == nil {
		t.Error("reserved id must fail")
	}
	if err := ValidateStages([]Stage{{ID: "a", Name: "A"}, {ID: "a", Name: "B"}}); err == nil {
		t.Error("duplicate id must fail")
	}
	if err := ValidateStages([]Stage{{ID: "a", Name: "A"}, {ID: "b", Name: ""}}); err == nil {
		t.Error("empty name must fail")
	}
	if err := ValidateStages([]Stage{{ID: "a", Name: "A"}, {ID: "b", Name: "B"}}); err != nil {
		t.Error(err)
	}
}

func TestValidateMove(t *testing.T) {
	st := []Stage{{ID: "applied"}, {ID: "interview"}}
	if err := ValidateMove(st, "active", "interview"); err != nil {
		t.Error(err)
	}
	if err := ValidateMove(st, "active", "hired"); err == nil {
		t.Error("moving to a terminal stage must go through hire/reject")
	}
	if err := ValidateMove(st, "rejected", "interview"); err == nil {
		t.Error("closed applications cannot move")
	}
	if err := ValidateMove(st, "active", "nope"); err == nil {
		t.Error("unknown stage must fail")
	}
}

func TestCheckJobTransition(t *testing.T) {
	cases := []struct {
		hr             bool
		approval, a, b string
		ok             bool
	}{
		{true, ApprovalNone, JobDraft, JobOpen, true},
		{false, ApprovalNone, JobDraft, JobOpen, false},
		{false, ApprovalPending, JobDraft, JobOpen, false},
		{false, ApprovalApproved, JobDraft, JobOpen, true},
		{true, ApprovalRejected, JobDraft, JobOpen, false},
		{true, ApprovalNone, JobDraft, JobClosed, false},
		{true, ApprovalNone, JobOpen, JobClosed, true},
		{true, ApprovalNone, JobClosed, JobOpen, true},
		{true, ApprovalNone, JobClosed, JobDraft, false},
		{true, ApprovalNone, JobOpen, "weird", false},
	}
	for i, c := range cases {
		err := CheckJobTransition(c.hr, c.approval, c.a, c.b)
		if (err == nil) != c.ok {
			t.Errorf("case %d (%+v): err=%v", i, c, err)
		}
	}
}

func TestJobInputValidate(t *testing.T) {
	lo, hi := 10.0, 5.0
	if (&JobInput{Title: " "}).Validate() == nil {
		t.Error("blank title must fail")
	}
	if (&JobInput{Title: "x", SalaryMin: &lo, SalaryMax: &hi}).Validate() == nil {
		t.Error("min above max must fail")
	}
	j := JobInput{Title: " Dev ", Skills: []string{" Go ", ""}}
	if err := j.Validate(); err != nil || j.Title != "Dev" || len(j.Skills) != 1 || j.Openings != 1 || j.Currency != "INR" {
		t.Errorf("normalise: %+v %v", j, err)
	}
}

func TestApplyInputValidate(t *testing.T) {
	bad := []ApplyInput{{Name: "", Email: "a@b.co"}, {Name: "A", Email: "nope"}, {Name: "A", Email: "a@b"}, {Name: "A", Email: "a@b.co", Source: "hack"}}
	for i, in := range bad {
		if in.Validate() == nil {
			t.Errorf("case %d must fail", i)
		}
	}
	in := ApplyInput{Name: " Asha ", Email: " Asha@Example.COM "}
	if err := in.Validate(); err != nil || in.Email != "asha@example.com" || in.Source != "career_page" {
		t.Errorf("normalise: %+v %v", in, err)
	}
}

func TestCheckResume(t *testing.T) {
	pdf := []byte("%PDF-1.4 body")
	if m, err := CheckResume("cv.PDF", pdf); err != nil || m != "application/pdf" {
		t.Errorf("pdf: %v %v", m, err)
	}
	if _, err := CheckResume("cv.pdf", []byte("MZ evil")); err == nil {
		t.Error("exe renamed to pdf must fail")
	}
	if _, err := CheckResume("cv.exe", pdf); err == nil {
		t.Error("exe extension must fail")
	}
	if _, err := CheckResume("cv.html", []byte("<script>")); err == nil {
		t.Error("html must fail")
	}
	if _, err := CheckResume("cv.pdf", nil); err == nil {
		t.Error("empty must fail")
	}
	if _, err := CheckResume("cv.pdf", append([]byte("%PDF-"), make([]byte, MaxResumeBytes)...)); err == nil {
		t.Error("oversize must fail")
	}
	if _, err := CheckResume("a.txt", []byte("hello\x00")); err == nil {
		t.Error("binary txt must fail")
	}
	if _, err := CheckResume("a.docx", []byte("PK\x03\x04zip")); err != nil {
		t.Error(err)
	}
}

func TestAbuseGuardWindow(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	g := NewAbuseGuard(2, time.Hour)
	g.now = func() time.Time { return now }
	if !g.Allow("a") || !g.Allow("a") {
		t.Fatal("first two allowed")
	}
	if g.Allow("a") {
		t.Error("third in the window must be refused")
	}
	if !g.Allow("b") {
		t.Error("other key independent")
	}
	now = now.Add(61 * time.Minute)
	if !g.Allow("a") {
		t.Error("window must slide")
	}
}

func TestGoalProgress(t *testing.T) {
	cases := []struct{ s, t, c, want float64 }{
		{0, 100, 50, 50}, {0, 100, 150, 100}, {0, 100, -5, 0}, {10, 20, 15, 50}, {100, 0, 25, 75}, {5, 5, 5, 100}, {5, 5, 4, 0},
	}
	for _, c := range cases {
		if got := GoalProgress(c.s, c.t, c.c); got != c.want {
			t.Errorf("GoalProgress(%v,%v,%v)=%v want %v", c.s, c.t, c.c, got, c.want)
		}
	}
}

func TestValidateScale(t *testing.T) {
	if ValidateScale([]ScaleLabel{{1, "a"}}) == nil {
		t.Error("one point must fail")
	}
	if ValidateScale([]ScaleLabel{{1, "a"}, {3, "c"}}) == nil {
		t.Error("gap must fail")
	}
	if ValidateScale([]ScaleLabel{{1, "a"}, {2, ""}}) == nil {
		t.Error("unlabelled must fail")
	}
	if err := ValidateScale([]ScaleLabel{{1, "a"}, {2, "b"}, {3, "c"}}); err != nil {
		t.Error(err)
	}
}

func TestSafeLink(t *testing.T) {
	for _, s := range []string{"/docs/abc", "https://x.example/p", "http://x.example"} {
		if _, err := SafeLink(s); err != nil {
			t.Errorf("%q: %v", s, err)
		}
	}
	for _, s := range []string{"javascript:alert(1)", "//evil.example", "data:text/html,x", "ftp://x", "https://"} {
		if _, err := SafeLink(s); err == nil {
			t.Errorf("%q must be refused", s)
		}
	}
}

func TestTemplateValidateAndDue(t *testing.T) {
	ok := TemplateInput{Kind: "onboarding", Name: "N", Tasks: []TemplateTask{{Title: "Laptop", Role: "it", DueOffset: -3}}}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	bad := TemplateInput{Kind: "onboarding", Name: "N", Tasks: []TemplateTask{{Title: "x", Role: "boss"}}}
	if bad.Validate() == nil {
		t.Error("unknown role must fail")
	}
	a := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	if d := DueDate(a, -3); d.Day() != 28 || d.Month() != 11 {
		t.Errorf("due %v", d)
	}
}

func TestOfferRenderAndPDF(t *testing.T) {
	out := RenderOffer("", map[string]string{"name": "Asha", "job_title": "Engineer", "department": "R&D", "salary": "INR 100.00", "start_date": "2026-12-01", "company": "Adrient", "sender": "HR"})
	if strings.Contains(out, "{{") || !strings.Contains(out, "Asha") || !strings.Contains(out, "INR 100.00") {
		t.Errorf("render: %s", out)
	}
	pdf := BuildPDF("Offer (draft) \\ test é", strings.Repeat("Line of text (with parens). ", 400))
	if !bytes.HasPrefix(pdf, []byte("%PDF-1.4")) || !bytes.HasSuffix(pdf, []byte("%%EOF\n")) {
		t.Fatal("not a pdf")
	}
	if bytes.Count(pdf, []byte("/Type /Page ")) < 2 {
		t.Error("long text must flow onto several pages")
	}
	if !bytes.Contains(pdf, []byte("Page 1 of")) {
		t.Error("page footer missing")
	}
	if len(BuildPDF("x", "")) == 0 {
		t.Error("empty body still produces a document")
	}
}

func TestJobFactsAccess(t *testing.T) {
	f := jobFacts{Recruiter: "r", Manager: "m", CreatedBy: "c", Team: []string{"i"}}
	cases := []struct {
		a             Actor
		team, leading bool
	}{
		{Actor{UserID: "r"}, true, true}, {Actor{UserID: "m"}, true, true}, {Actor{UserID: "c"}, true, true},
		{Actor{UserID: "i"}, true, false}, {Actor{UserID: "x"}, false, false}, {Actor{UserID: "x", IsHR: true}, true, true}, {Actor{}, false, false},
	}
	for i, c := range cases {
		if f.onTeam(c.a) != c.team || f.leads(c.a) != c.leading {
			t.Errorf("case %d: team=%v leads=%v", i, f.onTeam(c.a), f.leads(c.a))
		}
	}
}

func TestHTTPErrMapping(t *testing.T) {
	if !errors.Is(invalid("x"), ErrInvalid) {
		t.Error("invalid wraps ErrInvalid")
	}
}
