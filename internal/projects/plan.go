package projects

import (
	"errors"
	"fmt"
	"time"
)

// MaxDeps caps the prerequisites of one issue.
const MaxDeps = 20

// WouldCycle reports whether making `issue` depend on `newDeps` would create a
// loop in the dependency graph. deps maps an issue to what it depends on today.
func WouldCycle(deps map[string][]string, issue string, newDeps []string) bool {
	// Walk from every new prerequisite along "depends on" edges; reaching
	// `issue` means the prerequisite already (transitively) depends on it.
	seen := map[string]bool{}
	var visit func(string) bool
	visit = func(n string) bool {
		if n == issue {
			return true
		}
		if seen[n] {
			return false
		}
		seen[n] = true
		for _, d := range deps[n] {
			if visit(d) {
				return true
			}
		}
		return false
	}
	for _, d := range newDeps {
		if visit(d) {
			return true
		}
	}
	return false
}

// DateRange validates optional ISO dates and their order.
func DateRange(start, due string) (*time.Time, *time.Time, error) {
	parse := func(s string) (*time.Time, error) {
		if s == "" {
			return nil, nil
		}
		t, err := time.Parse("2006-01-02", s)
		if err != nil || t.Year() < 2000 || t.Year() > 2100 {
			return nil, fmt.Errorf("dates must look like 2030-01-31")
		}
		return &t, nil
	}
	a, err := parse(start)
	if err != nil {
		return nil, nil, err
	}
	b, err := parse(due)
	if err != nil {
		return nil, nil, err
	}
	if a != nil && b != nil && b.Before(*a) {
		return nil, nil, errors.New("the due date is before the start date")
	}
	return a, b, nil
}

// TemplateIssue is one issue of a project template. Offsets are in days from
// the project start; Needs lists indexes of earlier issues it depends on.
type TemplateIssue struct {
	Title       string
	Description string
	Priority    int32
	StartDay    int
	Days        int
	Milestone   int // index into Template.Milestones, -1 for none
	Needs       []int
	Hours       float64
}

// TemplateMilestone is a milestone at an offset from the project start.
type TemplateMilestone struct {
	Name string
	Day  int
}

// Template is a ready-made project plan.
type Template struct {
	ID          string
	Name        string
	Description string
	Brief       string
	Milestones  []TemplateMilestone
	Issues      []TemplateIssue
}

// Templates are the built-in project templates.
func Templates() []Template {
	return []Template{
		{
			ID: "software-release", Name: "Software release", Description: "Plan, build, test and ship a release.",
			Brief:      "Goal: ship the release on the target date.\n\nScope:\n- \n\nRisks:\n- \n",
			Milestones: []TemplateMilestone{{"Scope agreed", 5}, {"Code complete", 25}, {"Release", 35}},
			Issues: []TemplateIssue{
				{"Write the release scope", "List what is in and out.", 2, 0, 5, 0, nil, 8},
				{"Design the main changes", "", 2, 5, 7, 1, []int{0}, 24},
				{"Build the features", "", 2, 12, 13, 1, []int{1}, 80},
				{"Test and fix bugs", "", 3, 25, 7, 2, []int{2}, 40},
				{"Write release notes", "", 1, 30, 3, 2, []int{2}, 4},
				{"Ship and announce", "", 3, 35, 1, 2, []int{3, 4}, 4},
			},
		},
		{
			ID: "marketing-campaign", Name: "Marketing campaign", Description: "Brief, create, launch and review a campaign.",
			Brief:      "Audience:\n\nMessage:\n\nBudget:\n\nHow we will measure success:\n",
			Milestones: []TemplateMilestone{{"Brief approved", 3}, {"Launch", 21}, {"Review", 35}},
			Issues: []TemplateIssue{
				{"Write the campaign brief", "", 2, 0, 3, 0, nil, 6},
				{"Create content and designs", "", 2, 3, 12, 1, []int{0}, 40},
				{"Set up channels and tracking", "", 1, 10, 7, 1, []int{0}, 12},
				{"Launch", "", 3, 21, 1, 1, []int{1, 2}, 4},
				{"Report results", "", 1, 30, 5, 2, []int{3}, 8},
			},
		},
		{
			ID: "client-onboarding", Name: "Client onboarding", Description: "Get a new client set up and productive.",
			Brief:      "Client:\n\nGoals:\n\nMain contact:\n",
			Milestones: []TemplateMilestone{{"Kick-off done", 2}, {"Go live", 15}},
			Issues: []TemplateIssue{
				{"Kick-off meeting", "", 2, 0, 2, 0, nil, 2},
				{"Collect access and data", "", 2, 2, 5, 1, []int{0}, 6},
				{"Configure the account", "", 2, 7, 5, 1, []int{1}, 12},
				{"Train the client team", "", 1, 12, 3, 1, []int{2}, 6},
				{"Go-live check", "", 3, 15, 1, 1, []int{3}, 2},
			},
		},
	}
}

// FindTemplate returns a template by id.
func FindTemplate(id string) (Template, bool) {
	for _, t := range Templates() {
		if t.ID == id {
			return t, true
		}
	}
	return Template{}, false
}

// Validate checks a template is self-consistent (used by tests and at startup).
func (t Template) Validate() error {
	for i, is := range t.Issues {
		if is.Title == "" || is.Days < 1 {
			return fmt.Errorf("issue %d needs a title and at least one day", i)
		}
		if is.Milestone >= len(t.Milestones) {
			return fmt.Errorf("issue %d points at a missing milestone", i)
		}
		for _, n := range is.Needs {
			if n < 0 || n >= i {
				return fmt.Errorf("issue %d may only need earlier issues", i)
			}
		}
	}
	return nil
}
