package server

import (
	"context"
	"net/http"

	"github.com/Adrient-tech/Adrient-Workspace/internal/auth"
	"github.com/Adrient-tech/Adrient-Workspace/internal/fieldservice"
	"github.com/Adrient-tech/Adrient-Workspace/internal/fleet"
	"github.com/Adrient-tech/Adrient-Workspace/internal/legal"
	"github.com/Adrient-tech/Adrient-Workspace/internal/opshttp"
	"github.com/Adrient-tech/Adrient-Workspace/internal/opsports"
	"github.com/Adrient-tech/Adrient-Workspace/internal/shifts"
	"github.com/Adrient-tech/Adrient-Workspace/internal/timetracking"
)

// opsRoutes holds the wave-6 operations modules (Shifts, Time tracking, Field
// service, Fleet). Each handler is mounted under /api/v1/<module> behind the
// same auth wall and per-app access check as the other business modules.
type opsRoutes struct {
	handlers []opsHandler
}

type opsHandler struct {
	app   string
	mount string
	h     interface {
		Match(string) bool
		ServeHTTP(http.ResponseWriter, *http.Request)
	}
}

// match returns the handler serving path, if any.
func (o *opsRoutes) match(path string) (opsHandler, bool) {
	if o == nil {
		return opsHandler{}, false
	}
	for _, h := range o.handlers {
		if h.h.Match(path) {
			return h, true
		}
	}
	return opsHandler{}, false
}

func opsAuth(isAdmin func(r *http.Request) bool) opshttp.AuthFuncs {
	return opshttp.AuthFuncs{
		UserID: func(r *http.Request) (string, bool) {
			u, ok := auth.UserFromContext(r.Context())
			return u.ID, ok
		},
		OrgID: func(r *http.Request) (string, bool) {
			o, ok := auth.OrgFromContext(r.Context())
			return o.ID, ok
		},
		IsAdmin: isAdmin,
	}
}

// buildOps wires the ops modules from the server config.
func buildOps(cfg Config, isAdmin func(r *http.Request) bool) *opsRoutes {
	o := &opsRoutes{}
	a := opsAuth(isAdmin)
	if cfg.HRRepo != nil && cfg.Pool != nil {
		o.handlers = append(o.handlers, opsHandler{app: "shifts", mount: "/api/v1/shifts", h: shifts.NewHTTPHandler(shifts.NewRepository(cfg.Pool, cfg.HRRepo), a)})
	}
	var inv opshttp.InvoiceWriter
	if cfg.InvoicingRepo != nil {
		inv = opsports.Invoices{Repo: cfg.InvoicingRepo}
	}
	var dir opshttp.ContactDirectory
	if cfg.ContactsRepo != nil {
		dir = opsports.Contacts{Repo: cfg.ContactsRepo}
	}
	if cfg.HRRepo != nil && cfg.Pool != nil {
		o.handlers = append(o.handlers, opsHandler{app: "timetracking", mount: "/api/v1/timetracking", h: timetracking.NewHTTPHandler(timetracking.NewRepository(cfg.Pool, cfg.HRRepo, inv, dir), a)})
	}
	var cal opshttp.CalendarWriter
	if cfg.CalendarRepo != nil {
		cal = opsports.Calendar{Repo: cfg.CalendarRepo}
	}
	if cfg.Pool != nil {
		o.handlers = append(o.handlers, opsHandler{app: "fieldservice", mount: "/api/v1/fieldservice", h: fieldservice.NewHTTPHandler(fieldservice.NewRepository(cfg.Pool, dir, inv, cal), a)})
	}
	if cfg.Pool != nil {
		var exp opshttp.ExpenseWriter
		if cfg.AccountingRepo != nil {
			exp = opsports.Expenses{Repo: cfg.AccountingRepo}
		}
		o.handlers = append(o.handlers, opsHandler{app: "fleet", mount: "/api/v1/fleet", h: fleet.NewHTTPHandler(fleet.NewRepository(cfg.Pool, cal, exp), a)})
	}
	return o
}

// wireLegalTime lets Legal import entries from Time tracking (read only). Skipped when HR is not configured.
func wireLegalTime(cfg Config) {
	if cfg.Know == nil || cfg.Know.Legal == nil || cfg.HRRepo == nil || cfg.Pool == nil {
		return
	}
	tt := timetracking.NewRepository(cfg.Pool, cfg.HRRepo, nil, nil)
	cfg.Know.Legal.Deps.TimeEntries = func(ctx context.Context, org, user string, admin bool, from, to string) ([]legal.TimeEntry, error) {
		c, err := tt.CallerFor(ctx, org, user, admin)
		if err != nil {
			return nil, err
		}
		list, err := tt.ListEntries(ctx, c, timetracking.EntryFilter{From: from, To: to, UserID: "me", Unbilled: false})
		if err != nil {
			return nil, err
		}
		names := map[string]string{}
		if ps, err := tt.ListProjects(ctx, c, true); err == nil {
			for _, p := range ps {
				names[p.ID] = p.Name
			}
		}
		out := make([]legal.TimeEntry, 0, len(list))
		for _, e := range list {
			out = append(out, legal.TimeEntry{ID: e.ID, UserID: e.UserID, Date: e.Date, Seconds: e.Seconds, Note: e.Note, Project: names[e.ProjectID], Billable: e.Billable})
		}
		return out, nil
	}
}
