package server

import (
	"context"
	"net/http"

	"github.com/Adrient-tech/Adrient-Workspace/internal/ai"
	"github.com/Adrient-tech/Adrient-Workspace/internal/audit"
	"github.com/Adrient-tech/Adrient-Workspace/internal/auth"
	"github.com/Adrient-tech/Adrient-Workspace/internal/authz"
	"github.com/Adrient-tech/Adrient-Workspace/internal/search"
	"github.com/Adrient-tech/Adrient-Workspace/internal/sharing"
)

// aiSearcher adapts the global search (org + user scoped) for Ask Adrient.
type aiSearcher struct{ repo *search.Repository }

func (s aiSearcher) Search(ctx context.Context, orgID, userID, q string, n int) ([]ai.SearchHit, error) {
	res, err := s.repo.Search(ctx, orgID, userID, q, n)
	if err != nil {
		return nil, err
	}
	out := make([]ai.SearchHit, 0, len(res))
	for _, r := range res {
		out = append(out, ai.SearchHit{Type: r.Type, ID: r.ID, Title: r.Title, Snippet: r.Snippet, URL: r.URL})
	}
	return out, nil
}

// newAIHandler wires the AI gateway to auth, app controls, audit, search and
// the same document access checks the editors' collab sockets use.
func newAIHandler(cfg Config, auditRec *audit.Recorder) *ai.HTTPHandler {
	g := cfg.AIGateway
	if g == nil {
		return nil
	}
	if cfg.SearchRepo != nil {
		g.Searcher = aiSearcher{repo: cfg.SearchRepo}
	}
	g.Audit = func(ctx context.Context, action string, d map[string]any) { auditRec.Emit(ctx, "ai", action, d) }
	grants := cfg.SharingRepo
	g.DocAccess = func(ctx context.Context, _ ai.Caller, app, id string) (bool, bool, bool) {
		r, err := http.NewRequestWithContext(ctx, http.MethodGet, "/", nil)
		if err != nil {
			return false, false, true
		}
		switch app {
		case "docs":
			if cfg.DocsRepo == nil {
				return false, false, false
			}
			acc := docsAccessFor(r, id, cfg.DocsRepo, docsGrantLookup(grants))
			return acc.Read, acc.Write, true
		case "slides":
			if cfg.SlidesRepo == nil {
				return false, false, false
			}
			read, write := slidesDeckAccess(r, id, cfg.SlidesRepo, grants)
			return read, write, true
		case "sheets":
			if cfg.SheetsRepo == nil {
				return false, false, false
			}
			u, ok := auth.UserFromContext(ctx)
			if !ok {
				return false, false, true
			}
			if org, ok := auth.OrgFromContext(ctx); ok {
				if _, err := cfg.SheetsRepo.Get(ctx, org.ID, id); err == nil {
					return true, true, true
				}
			}
			if grants != nil {
				if role, ok, err := grants.RoleFor(ctx, u.ID, sharing.TypeSheetsSheet, id); err == nil && ok {
					if _, err := cfg.SheetsRepo.GetByID(ctx, id); err == nil {
						return true, sharing.CanWrite(role), true
					}
				}
			}
			return false, false, true
		}
		return false, false, false
	}
	return ai.NewHTTPHandler(g, ai.AuthFuncs{Caller: func(r *http.Request) (ai.Caller, bool) {
		ctx := r.Context()
		u, ok := auth.UserFromContext(ctx)
		o, ok2 := auth.OrgFromContext(ctx)
		if !ok || !ok2 {
			return ai.Caller{}, false
		}
		isAdmin := authz.IsAdmin(ctx)
		return ai.Caller{
			OrgID: o.ID, UserID: u.ID, UserType: u.EffectiveType(),
			IsAdmin:    isAdmin,
			Allowed:    authz.ControlBool(ctx, "ai.assistant"),
			CanOpenApp: func(app string) bool { return authz.CanOpenApp(ctx, app) },
			Can:        aiCapability(ctx, cfg, o.ID, u.ID, isAdmin),
		}, true
	}})
}

// aiCapability resolves the capabilities in internal/ai/policy.go for one
// request. Platform permissions go through authz; "hr.admin" and
// "talent.recruiter" ask the module's own role tables, the same ones its API uses.
func aiCapability(ctx context.Context, cfg Config, orgID, userID string, isAdmin bool) func(string) bool {
	return func(perm string) bool {
		switch perm {
		case ai.CapOrgAdmin:
			return isAdmin
		case ai.CapHRAdmin:
			if isAdmin {
				return true
			}
			if cfg.HRRepo == nil {
				return false
			}
			a, err := cfg.HRRepo.ActorFor(ctx, orgID, userID, false)
			return err == nil && a.IsHR
		case ai.CapRecruiter:
			if isAdmin {
				return true
			}
			return cfg.TalentRepo != nil && cfg.TalentRepo.IsHRUser(ctx, orgID, userID)
		}
		return authz.Can(ctx, perm)
	}
}
