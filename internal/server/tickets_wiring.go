package server

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/Adrient-tech/Adrient-Workspace/internal/ai"
	"github.com/Adrient-tech/Adrient-Workspace/internal/auth"
	"github.com/Adrient-tech/Adrient-Workspace/internal/authz"
	"github.com/Adrient-tech/Adrient-Workspace/internal/notifications"
	"github.com/Adrient-tech/Adrient-Workspace/internal/projects"
	"github.com/Adrient-tech/Adrient-Workspace/internal/tickets"
	"github.com/Adrient-tech/Adrient-Workspace/internal/timetracking"
)

// wireTicketDeps connects the service desk to the other apps: notifications, Tasks (the "create a
// task" automation action), Time tracking and Projects. Each hook is left nil when its app is not
// configured, and the desk then reports that plainly instead of faking it.
func wireTicketDeps(cfg Config, repo *tickets.Repository) {
	if repo == nil {
		return
	}
	if cfg.NotificationsRepo != nil {
		nr := cfg.NotificationsRepo
		repo.Deps.Notify = func(ctx context.Context, org, user, title, body, url string) error {
			_, err := nr.Create(ctx, notifications.CreateParams{OrgID: org, UserID: user, Type: "tickets", Title: title, Body: body, TargetURL: url})
			return err
		}
	}
	if cfg.TasksRepo != nil {
		tr := cfg.TasksRepo
		repo.Deps.CreateTask = func(ctx context.Context, org, assignee, title, description, link string) error {
			if assignee == "" {
				assignee = tickets.ActorID(ctx)
			}
			if assignee == "" {
				return errors.New("no person to give the task to")
			}
			_, err := tr.Quick(ctx, org, assignee, title, description, link, nil)
			return err
		}
	}
	if cfg.HRRepo != nil && cfg.Pool != nil {
		tt := timetracking.NewRepository(cfg.Pool, cfg.HRRepo, nil, nil)
		repo.Deps.LogTime = func(ctx context.Context, org, user string, minutes int, date, note string, billable bool) error {
			c, err := tt.CallerFor(ctx, org, user, false)
			if err != nil {
				return err
			}
			projects, err := tt.ListProjects(ctx, c, false)
			if err != nil {
				return err
			}
			pick := ""
			for _, p := range projects {
				if p.Archived {
					continue
				}
				if pick == "" {
					pick = p.ID
				}
				if strings.EqualFold(p.Name, "support") || strings.EqualFold(p.Name, "helpdesk") || strings.EqualFold(p.Name, "service desk") {
					pick = p.ID
					break
				}
			}
			if pick == "" {
				return errors.New("create a project in Time tracking first")
			}
			_, err = tt.SaveEntry(ctx, c, "", timetracking.Entry{ProjectID: pick, Date: date, Seconds: minutes * 60, Billable: billable, Task: "Ticket", Note: note})
			return err
		}
	}
	if cfg.ProjectsRepo != nil {
		pr := cfg.ProjectsRepo
		repo.Deps.CreateIssue = func(ctx context.Context, org, user, teamID, title, description string) (string, string, error) {
			if teamID == "" {
				teams, err := pr.ListTeams(ctx, org)
				if err != nil {
					return "", "", err
				}
				if len(teams) == 0 {
					return "", "", errors.New("create a team in Projects first")
				}
				teamID = teams[0].ID
			}
			is, err := pr.CreateIssue(ctx, org, teamID, user, projectsIssueFields(title, description))
			if err != nil {
				return "", "", err
			}
			return is.ID, is.TeamKey + "-" + strconv.Itoa(int(is.Number)), nil
		}
	}
}

func projectsIssueFields(title, description string) projects.IssueFields {
	return projects.IssueFields{Title: title, Description: description}
}

// ticketsAI returns the per-request AI runner of the service desk. It goes through the same gateway
// as the assistant panel, so provider keys, redaction, limits and the audit log apply. It returns nil
// when no gateway is configured.
func ticketsAI(cfg Config) func(r *http.Request) tickets.AIRun {
	g := cfg.AIGateway
	if g == nil {
		return nil
	}
	return func(r *http.Request) tickets.AIRun {
		ctx0 := r.Context()
		u, ok := auth.UserFromContext(ctx0)
		o, ok2 := auth.OrgFromContext(ctx0)
		if !ok || !ok2 {
			return nil
		}
		isAdmin := authz.IsAdmin(ctx0)
		caller := ai.Caller{
			OrgID: o.ID, UserID: u.ID, UserType: u.EffectiveType(), IsAdmin: isAdmin,
			Allowed:    authz.ControlBool(ctx0, "ai.assistant"),
			CanOpenApp: func(app string) bool { return authz.CanOpenApp(ctx0, app) },
			Can:        aiCapability(ctx0, cfg, o.ID, u.ID, isAdmin),
		}
		return func(ctx context.Context, system, prompt string) (string, error) {
			var sb strings.Builder
			var failure string
			aerr := g.Chat(ctx, caller, ai.ChatRequest{App: "tickets", SystemHint: system, Messages: []ai.Message{{Role: "user", Content: prompt}}},
				func(e ai.StreamEvent) error {
					switch e.Type {
					case "delta":
						sb.WriteString(e.Text)
					case "error":
						failure = e.Message
					}
					return nil
				})
			if aerr != nil {
				return "", errors.New(aerr.Message)
			}
			if failure != "" {
				return "", errors.New(failure)
			}
			return sb.String(), nil
		}
	}
}
