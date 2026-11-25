package people

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/Adrient-tech/Adrient-Workspace/internal/audit"
)

// Mount prefixes of the two API roots served by Handler.
const (
	PeoplePrefix      = "/api/v1/people"
	DepartmentsPrefix = "/api/v1/departments"
)

// AuthFuncs resolves the caller. IsAdmin reports whether the caller holds the
// org:admin permission (Owner and Admin roles).
type AuthFuncs struct {
	OrgID   func(*http.Request) (string, bool)
	UserID  func(*http.Request) (string, bool)
	IsAdmin func(*http.Request) bool
}

// Handler serves /api/v1/people and /api/v1/departments.
//
//	GET    /api/v1/people?query=          search (any member)
//	GET    /api/v1/people/orgchart        every person + departments (any member)
//	GET    /api/v1/people/{id}            profile, manager, chain, reports, department
//	PATCH  /api/v1/people/{id}            Admin/Owner: any attribute; the user themselves: phone, location
//	GET    /api/v1/departments            department list (any member)
//	POST   /api/v1/departments            Admin/Owner: {name,parent_id,head_user_id}
//	PATCH  /api/v1/departments/{id}       Admin/Owner
//	DELETE /api/v1/departments/{id}       Admin/Owner
type Handler struct {
	store *Store
	auth  AuthFuncs
	// Visible limits what a restricted caller (a guest) can see: when it returns
	// restricted=true only people in ids are listed or opened. nil = no limit.
	Visible func(ctx context.Context) (ids map[string]bool, restricted bool)
}

func (h *Handler) filter(ctx context.Context, list []Person) []Person {
	if h.Visible == nil {
		return list
	}
	ids, restricted := h.Visible(ctx)
	if !restricted {
		return list
	}
	out := make([]Person, 0, len(list))
	for _, p := range list {
		if ids[p.ID] {
			out = append(out, p)
		}
	}
	return out
}

// NewHandler constructs the handler.
func NewHandler(store *Store, auth AuthFuncs) *Handler { return &Handler{store: store, auth: auth} }

// Match reports whether path belongs to this handler.
func Match(path string) bool {
	return path == PeoplePrefix || strings.HasPrefix(path, PeoplePrefix+"/") ||
		path == DepartmentsPrefix || strings.HasPrefix(path, DepartmentsPrefix+"/")
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	orgID, ok := h.auth.OrgID(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	callerID, _ := h.auth.UserID(r)
	path := strings.TrimRight(r.URL.Path, "/")
	switch {
	case path == PeoplePrefix:
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		list, err := h.store.Search(r.Context(), orgID, r.URL.Query().Get("query"), limit)
		h.reply(w, err, map[string]any{"people": h.filter(r.Context(), list)})
	case path == PagePath:
		h.pageHandler(w, r, orgID)
	case path == PeoplePrefix+"/orgchart":
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		list, err := h.store.All(r.Context(), orgID)
		if err != nil {
			h.reply(w, err, nil)
			return
		}
		deps, err := h.store.Departments(r.Context(), orgID)
		h.reply(w, err, map[string]any{"people": h.filter(r.Context(), list), "departments": deps})
	case strings.HasPrefix(path, PeoplePrefix+"/"):
		pid := strings.TrimPrefix(path, PeoplePrefix+"/")
		if h.Visible != nil {
			if ids, restricted := h.Visible(r.Context()); restricted && !ids[pid] {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
		}
		h.person(w, r, orgID, callerID, pid)
	case path == DepartmentsPrefix:
		h.departments(w, r, orgID)
	default:
		h.department(w, r, orgID, strings.TrimPrefix(path, DepartmentsPrefix+"/"))
	}
}

func (h *Handler) person(w http.ResponseWriter, r *http.Request, orgID, callerID, id string) {
	switch r.Method {
	case http.MethodGet:
		p, err := h.store.Get(r.Context(), orgID, id)
		if err != nil {
			h.reply(w, err, nil)
			return
		}
		chain, err := h.store.ManagerChain(r.Context(), orgID, id)
		if err != nil {
			h.reply(w, err, nil)
			return
		}
		reports, err := h.store.DirectReports(r.Context(), orgID, id)
		if err != nil {
			h.reply(w, err, nil)
			return
		}
		out := map[string]any{"person": p, "manager_chain": chain, "direct_reports": reports,
			"avatar_url": "/api/v1/users/" + p.ID + "/avatar"}
		if len(chain) > 0 {
			out["manager"] = chain[0]
		}
		if p.DepartmentID != "" {
			if d, err := h.store.GetDepartment(r.Context(), orgID, p.DepartmentID); err == nil {
				out["department"] = d
			}
		}
		h.reply(w, nil, out)
	case http.MethodPatch:
		var patch Patch
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&patch); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
		admin := h.auth.IsAdmin != nil && h.auth.IsAdmin(r)
		if !admin {
			if callerID == "" || callerID != id {
				http.Error(w, "forbidden: only an admin can edit other people", http.StatusForbidden)
				return
			}
			if patch.JobTitle != nil || patch.EmployeeID != nil || patch.StartDate != nil ||
				patch.DepartmentID != nil || patch.ManagerID != nil {
				http.Error(w, "forbidden: you can only edit your own phone and location", http.StatusForbidden)
				return
			}
		}
		before, err := h.store.Get(r.Context(), orgID, id)
		if err != nil {
			h.reply(w, err, nil)
			return
		}
		p, err := h.store.UpdateUser(r.Context(), orgID, id, patch)
		if err != nil {
			h.reply(w, err, nil)
			return
		}
		audit.Annotate(r.Context(), "person", id, describeChange(before, p))
		h.reply(w, nil, map[string]any{"person": p})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func describeChange(a, b Person) string {
	var ch []string
	add := func(name, from, to string) {
		if from != to {
			ch = append(ch, name+": '"+from+"' -> '"+to+"'")
		}
	}
	add("title", a.JobTitle, b.JobTitle)
	add("department", a.DepartmentName, b.DepartmentName)
	add("manager", a.ManagerName, b.ManagerName)
	add("phone", a.Phone, b.Phone)
	add("location", a.Location, b.Location)
	add("employee_id", a.EmployeeID, b.EmployeeID)
	add("start_date", a.StartDate, b.StartDate)
	if len(ch) == 0 {
		return b.Email + ": no change"
	}
	return b.Email + " " + strings.Join(ch, "; ")
}

func (h *Handler) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	if h.auth.IsAdmin == nil || !h.auth.IsAdmin(r) {
		http.Error(w, "forbidden: admin only", http.StatusForbidden)
		return false
	}
	return true
}

func (h *Handler) departments(w http.ResponseWriter, r *http.Request, orgID string) {
	switch r.Method {
	case http.MethodGet:
		ds, err := h.store.Departments(r.Context(), orgID)
		h.reply(w, err, map[string]any{"departments": ds})
	case http.MethodPost:
		if !h.requireAdmin(w, r) {
			return
		}
		var b struct {
			Name       string `json:"name"`
			ParentID   string `json:"parent_id"`
			HeadUserID string `json:"head_user_id"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&b); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
		d, err := h.store.CreateDepartment(r.Context(), orgID, b.Name, b.ParentID, b.HeadUserID)
		if err == nil {
			audit.Annotate(r.Context(), "department", d.ID, "created department '"+d.Name+"'")
		}
		h.replyStatus(w, http.StatusCreated, err, map[string]any{"department": d})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) department(w http.ResponseWriter, r *http.Request, orgID, id string) {
	if strings.Contains(id, "/") {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodPatch:
		if !h.requireAdmin(w, r) {
			return
		}
		var p DepartmentPatch
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&p); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
		before, _ := h.store.GetDepartment(r.Context(), orgID, id)
		d, err := h.store.UpdateDepartment(r.Context(), orgID, id, p)
		if err == nil {
			audit.Annotate(r.Context(), "department", id, "updated department '"+before.Name+"' -> '"+d.Name+"'")
		}
		h.reply(w, err, map[string]any{"department": d})
	case http.MethodDelete:
		if !h.requireAdmin(w, r) {
			return
		}
		before, _ := h.store.GetDepartment(r.Context(), orgID, id)
		err := h.store.DeleteDepartment(r.Context(), orgID, id)
		if err == nil {
			audit.Annotate(r.Context(), "department", id, "deleted department '"+before.Name+"'")
		}
		h.reply(w, err, map[string]any{"ok": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) reply(w http.ResponseWriter, err error, v any) {
	h.replyStatus(w, http.StatusOK, err, v)
}

func (h *Handler) replyStatus(w http.ResponseWriter, ok int, err error, v any) {
	switch {
	case err == nil:
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(ok)
		_ = json.NewEncoder(w).Encode(v)
	case errors.Is(err, ErrNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case errors.Is(err, ErrCycle):
		http.Error(w, "that change would create a loop in the hierarchy", http.StatusConflict)
	case errors.Is(err, ErrDuplicate):
		http.Error(w, err.Error(), http.StatusConflict)
	case errors.Is(err, ErrInvalid):
		http.Error(w, err.Error(), http.StatusBadRequest)
	default:
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}
