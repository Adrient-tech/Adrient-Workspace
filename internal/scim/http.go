package scim

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Mount is the public base path of the SCIM service.
const Mount = "/scim/v2"

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func isUUID(s string) bool { return uuidRe.MatchString(s) }

func newID() string {
	b := make([]byte, 10)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// IdP mirrors user state into the identity provider (Zitadel). Optional: a nil
// IdP leaves users "pre-provisioned" until their first sign-in.
type IdP interface {
	CreateUser(ctx context.Context, email, displayName string) (issuer, subject string, err error)
	SetActive(ctx context.Context, issuer, subject string, active bool) error
}

// Offboarder removes a user the way an admin delete does (legal holds and the
// restore window apply). nil falls back to blocking the user.
type Offboarder interface {
	SoftDelete(ctx context.Context, orgID, userID, actor string) error
}

// Server is the SCIM HTTP handler.
type Server struct {
	Store *Store
	IdP   IdP
	// SeatCheck refuses a new active member when the plan has no seat left.
	SeatCheck func(ctx context.Context, orgID string) error
	Offboard  Offboarder
	Audit     func(ctx context.Context, orgID, action string, detail map[string]any)
	// BaseURL overrides the public origin used in "meta.location" links.
	BaseURL string
}

// Match reports whether path is a SCIM path.
func Match(path string) bool { return path == Mount || strings.HasPrefix(path, Mount+"/") }

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/scim+json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, scimType, detail string) {
	body := map[string]any{"schemas": []string{SchemaError}, "status": strconv.Itoa(code), "detail": detail}
	if scimType != "" {
		body["scimType"] = scimType
	}
	writeJSON(w, code, body)
}

type statusWriter struct {
	http.ResponseWriter
	code int
}

func (s *statusWriter) WriteHeader(c int) { s.code = c; s.ResponseWriter.WriteHeader(c) }

func (h *Server) base(r *http.Request) string {
	if h.BaseURL != "" {
		return strings.TrimRight(h.BaseURL, "/") + Mount
	}
	proto := r.Header.Get("X-Forwarded-Proto")
	if proto == "" {
		proto = "http"
		if r.TLS != nil {
			proto = "https"
		}
	}
	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Host
	}
	return proto + "://" + host + Mount
}

func (h *Server) audit(ctx context.Context, orgID, action string, d map[string]any) {
	if h.Audit != nil {
		h.Audit(ctx, orgID, action, d)
	}
}

// ServeHTTP authenticates the bearer token, applies the token's rate limit and
// dispatches to the resource handlers.
func (h *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	hdr := r.Header.Get("Authorization")
	plain := ""
	if len(hdr) > 7 && strings.EqualFold(hdr[:7], "bearer ") {
		plain = strings.TrimSpace(hdr[7:])
	}
	tok, err := h.Store.Authenticate(r.Context(), plain)
	if err != nil {
		w.Header().Set("WWW-Authenticate", `Bearer realm="scim"`)
		writeError(w, http.StatusUnauthorized, "", "A valid SCIM bearer token is required.")
		return
	}
	if ok, wait := h.Store.Allow(tok); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
		writeError(w, http.StatusTooManyRequests, "", "Rate limit of "+strconv.Itoa(tok.RatePerMin)+" requests per minute exceeded.")
		return
	}
	sw := &statusWriter{ResponseWriter: w, code: http.StatusOK}
	path := strings.Trim(strings.TrimPrefix(strings.TrimRight(r.URL.Path, "/"), Mount), "/")
	h.route(sw, r, tok, path)
	h.Store.LogEvent(r.Context(), tok.OrgID, tok.ID, r.Method, "/scim/v2/"+path, sw.code, "")
}

func (h *Server) route(w http.ResponseWriter, r *http.Request, tok Token, path string) {
	seg := strings.Split(path, "/")
	switch {
	case path == "ServiceProviderConfig" && r.Method == http.MethodGet:
		h.serviceProviderConfig(w, r)
	case path == "ResourceTypes" && r.Method == http.MethodGet:
		h.resourceTypes(w, r)
	case path == "Schemas" && r.Method == http.MethodGet:
		h.schemas(w, r)
	case seg[0] == "Users" && len(seg) <= 2:
		h.users(w, r, tok, seg)
	case seg[0] == "Groups" && len(seg) <= 2:
		h.groups(w, r, tok, seg)
	default:
		writeError(w, http.StatusNotFound, "", "Unknown SCIM resource.")
	}
}

func decode(w http.ResponseWriter, r *http.Request, v any) error {
	return json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(v)
}

func pageParams(r *http.Request) (start, count int) {
	start, count = 1, 100
	if v, err := strconv.Atoi(r.URL.Query().Get("startIndex")); err == nil && v > 0 {
		start = v
	}
	if v, err := strconv.Atoi(r.URL.Query().Get("count")); err == nil && v >= 0 {
		count = v
	}
	if count > 200 {
		count = 200
	}
	return
}

// ---- users -----------------------------------------------------------------

func (h *Server) userJSON(r *http.Request, u UserRow) map[string]any {
	given, family, _ := strings.Cut(strings.TrimSpace(u.DisplayName), " ")
	out := map[string]any{
		"schemas":     []string{SchemaUser, SchemaEnterprise},
		"id":          u.ID,
		"userName":    u.Email,
		"name":        map[string]any{"formatted": u.DisplayName, "givenName": given, "familyName": family},
		"displayName": u.DisplayName,
		"emails":      []map[string]any{{"value": u.Email, "type": "work", "primary": true}},
		"active":      u.Status == "active",
		"meta": map[string]any{"resourceType": "User", "created": u.Created.UTC().Format(time.RFC3339),
			"lastModified": u.Updated.UTC().Format(time.RFC3339), "location": h.base(r) + "/Users/" + u.ID},
	}
	if u.ExternalID != "" {
		out["externalId"] = u.ExternalID
	}
	if u.Title != "" {
		out["title"] = u.Title
	}
	if u.Phone != "" {
		out["phoneNumbers"] = []map[string]any{{"value": u.Phone, "type": "work", "primary": true}}
	}
	ent := map[string]any{}
	if u.EmployeeID != "" {
		ent["employeeNumber"] = u.EmployeeID
	}
	if u.Department != "" {
		ent["department"] = u.Department
	}
	if u.ManagerID != "" {
		ent["manager"] = map[string]any{"value": u.ManagerID}
	}
	if len(ent) > 0 {
		out[SchemaEnterprise] = ent
	}
	return out
}

// scimUserBody is the subset of the User resource we read.
type scimUserBody struct {
	UserName    string `json:"userName"`
	ExternalID  string `json:"externalId"`
	DisplayName string `json:"displayName"`
	Title       string `json:"title"`
	Active      *bool  `json:"active"`
	RawName     struct {
		GivenName  string `json:"givenName"`
		FamilyName string `json:"familyName"`
		Formatted  string `json:"formatted"`
	} `json:"name"`
	Emails []struct {
		Value   string `json:"value"`
		Primary bool   `json:"primary"`
	} `json:"emails"`
	Phones []struct {
		Value string `json:"value"`
	} `json:"phoneNumbers"`
	Enterprise struct {
		EmployeeNumber string `json:"employeeNumber"`
		Department     string `json:"department"`
	} `json:"urn:ietf:params:scim:schemas:extension:enterprise:2.0:User"`
}

func (b scimUserBody) email() string {
	e := strings.TrimSpace(b.UserName)
	if !strings.Contains(e, "@") {
		for _, x := range b.Emails {
			if x.Primary && x.Value != "" {
				e = x.Value
			}
		}
		if !strings.Contains(e, "@") && len(b.Emails) > 0 {
			e = b.Emails[0].Value
		}
	}
	return strings.ToLower(strings.TrimSpace(e))
}

func (b scimUserBody) displayName() string {
	switch {
	case strings.TrimSpace(b.RawName.Formatted) != "":
		return strings.TrimSpace(b.RawName.Formatted)
	case strings.TrimSpace(b.RawName.GivenName+" "+b.RawName.FamilyName) != "":
		return strings.TrimSpace(b.RawName.GivenName + " " + b.RawName.FamilyName)
	}
	return strings.TrimSpace(b.DisplayName)
}

func validEmail(e string) bool {
	at := strings.LastIndexByte(e, '@')
	return at > 0 && at < len(e)-3 && strings.Contains(e[at:], ".") && !strings.ContainsAny(e, " <>\"")
}

func (h *Server) users(w http.ResponseWriter, r *http.Request, tok Token, seg []string) {
	ctx := r.Context()
	if len(seg) == 1 {
		switch r.Method {
		case http.MethodGet:
			f, err := ParseFilter(r.URL.Query().Get("filter"))
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalidFilter", err.Error())
				return
			}
			start, count := pageParams(r)
			list, total, err := h.Store.ListUsers(ctx, tok.OrgID, f, start, count)
			if err == errBadFilter {
				writeError(w, http.StatusBadRequest, "invalidFilter", "Filtering on that attribute is not supported (userName, externalId, id, displayName are).")
				return
			}
			if err != nil {
				writeError(w, http.StatusInternalServerError, "", "Could not list users.")
				return
			}
			res := make([]map[string]any, 0, len(list))
			for _, u := range list {
				res = append(res, h.userJSON(r, u))
			}
			writeJSON(w, http.StatusOK, map[string]any{"schemas": []string{SchemaList}, "totalResults": total,
				"startIndex": start, "itemsPerPage": len(res), "Resources": res})
		case http.MethodPost:
			h.createUser(w, r, tok)
		default:
			writeError(w, http.StatusMethodNotAllowed, "", "Method not allowed.")
		}
		return
	}
	id := seg[1]
	switch r.Method {
	case http.MethodGet:
		u, err := h.Store.GetUser(ctx, tok.OrgID, id)
		if err != nil {
			writeError(w, http.StatusNotFound, "", "User not found.")
			return
		}
		writeJSON(w, http.StatusOK, h.userJSON(r, u))
	case http.MethodPut:
		var b scimUserBody
		if decode(w, r, &b) != nil {
			writeError(w, http.StatusBadRequest, "invalidSyntax", "Body is not valid JSON.")
			return
		}
		p := userPatch{}
		if e := b.email(); e != "" {
			if !validEmail(e) {
				writeError(w, http.StatusBadRequest, "invalidValue", "userName must be an e-mail address.")
				return
			}
			p.UserName = &e
		}
		dn := b.displayName()
		p.DisplayName = &dn
		p.Title = &b.Title
		p.ExternalID = &b.ExternalID
		if len(b.Phones) > 0 {
			p.Phone = &b.Phones[0].Value
		}
		p.EmployeeID = &b.Enterprise.EmployeeNumber
		p.Department = &b.Enterprise.Department
		p.Active = b.Active
		h.applyUser(w, r, tok, id, p)
	case http.MethodPatch:
		var req PatchRequest
		if decode(w, r, &req) != nil {
			writeError(w, http.StatusBadRequest, "invalidSyntax", "Body is not valid JSON.")
			return
		}
		p, err := ParseUserPatch(req)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalidValue", err.Error())
			return
		}
		h.applyUser(w, r, tok, id, p)
	case http.MethodDelete:
		if _, err := h.Store.GetUser(ctx, tok.OrgID, id); err != nil {
			writeError(w, http.StatusNotFound, "", "User not found.")
			return
		}
		if h.Offboard != nil {
			if err := h.Offboard.SoftDelete(ctx, tok.OrgID, id, ""); err != nil {
				writeError(w, http.StatusForbidden, "", err.Error())
				return
			}
		} else if _, _, _, err := h.Store.SetActive(ctx, tok.OrgID, id, false); err != nil {
			writeError(w, http.StatusInternalServerError, "", "Could not deactivate the user.")
			return
		}
		h.audit(ctx, tok.OrgID, "scim.user.delete", map[string]any{"user_id": id, "token": tok.Name})
		w.WriteHeader(http.StatusNoContent)
	default:
		writeError(w, http.StatusMethodNotAllowed, "", "Method not allowed.")
	}
}

func (h *Server) createUser(w http.ResponseWriter, r *http.Request, tok Token) {
	ctx := r.Context()
	var b scimUserBody
	if decode(w, r, &b) != nil {
		writeError(w, http.StatusBadRequest, "invalidSyntax", "Body is not valid JSON.")
		return
	}
	email := b.email()
	if !validEmail(email) {
		writeError(w, http.StatusBadRequest, "invalidValue", "userName must be an e-mail address.")
		return
	}
	active := b.Active == nil || *b.Active
	if active && h.SeatCheck != nil {
		if err := h.SeatCheck(ctx, tok.OrgID); err != nil {
			writeError(w, http.StatusForbidden, "", err.Error())
			return
		}
	}
	in := NewUser{Email: email, DisplayName: b.displayName(), Title: b.Title, ExternalID: b.ExternalID, Active: active,
		EmployeeID: b.Enterprise.EmployeeNumber, Department: b.Enterprise.Department}
	if len(b.Phones) > 0 {
		in.Phone = b.Phones[0].Value
	}
	if taken, _ := h.Store.EmailTaken(ctx, tok.OrgID, email); taken {
		writeError(w, http.StatusConflict, "uniqueness", "A user with that userName or externalId already exists.")
		return
	}
	if h.IdP != nil {
		iss, sub, err := h.IdP.CreateUser(ctx, email, in.DisplayName)
		if err != nil {
			if strings.Contains(err.Error(), "409") {
				writeError(w, http.StatusConflict, "uniqueness", "The identity provider already has an account for that address. Ask an admin to restore or remove it.")
				return
			}
			writeError(w, http.StatusBadGateway, "", "The identity provider refused the user: "+err.Error())
			return
		}
		in.Issuer, in.Subject = iss, sub
	}
	u, err := h.Store.CreateUser(ctx, tok.OrgID, in)
	if errors.Is(err, errUniqueness) {
		writeError(w, http.StatusConflict, "uniqueness", "A user with that userName or externalId already exists.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "", "Could not create the user.")
		return
	}
	h.audit(ctx, tok.OrgID, "scim.user.create", map[string]any{"user_id": u.ID, "email": u.Email, "token": tok.Name})
	w.Header().Set("Location", h.base(r)+"/Users/"+u.ID)
	writeJSON(w, http.StatusCreated, h.userJSON(r, u))
}

func (h *Server) applyUser(w http.ResponseWriter, r *http.Request, tok Token, id string, p userPatch) {
	ctx := r.Context()
	if p.UserName != nil && !validEmail(strings.ToLower(strings.TrimSpace(*p.UserName))) {
		writeError(w, http.StatusBadRequest, "invalidValue", "userName must be an e-mail address.")
		return
	}
	if p.Active != nil && *p.Active && h.SeatCheck != nil {
		if cur, err := h.Store.GetUser(ctx, tok.OrgID, id); err == nil && cur.Status != "active" {
			if err := h.SeatCheck(ctx, tok.OrgID); err != nil {
				writeError(w, http.StatusForbidden, "", err.Error())
				return
			}
		}
	}
	u, err := h.Store.UpdateUser(ctx, tok.OrgID, id, p)
	switch {
	case errors.Is(err, errNotFound):
		writeError(w, http.StatusNotFound, "", "User not found.")
		return
	case errors.Is(err, errUniqueness):
		writeError(w, http.StatusConflict, "uniqueness", "That userName or externalId is used by another user.")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "", "Could not update the user.")
		return
	}
	if p.Active != nil {
		iss, sub, changed, err := h.Store.SetActive(ctx, tok.OrgID, id, *p.Active)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "", "Could not change the user's active state.")
			return
		}
		if changed {
			if h.IdP != nil && iss != PreprovisionedIssuer {
				_ = h.IdP.SetActive(ctx, iss, sub, *p.Active)
			}
			h.audit(ctx, tok.OrgID, "scim.user.active", map[string]any{"user_id": id, "active": *p.Active, "token": tok.Name})
		}
		if u, err = h.Store.GetUser(ctx, tok.OrgID, id); err != nil {
			writeError(w, http.StatusInternalServerError, "", "Could not read the user.")
			return
		}
	}
	h.audit(ctx, tok.OrgID, "scim.user.update", map[string]any{"user_id": id, "token": tok.Name})
	writeJSON(w, http.StatusOK, h.userJSON(r, u))
}

// ---- groups ----------------------------------------------------------------

func (h *Server) groupJSON(r *http.Request, g GroupRow) map[string]any {
	members := make([]map[string]any, 0, len(g.Members))
	for _, m := range g.Members {
		members = append(members, map[string]any{"value": m, "$ref": h.base(r) + "/Users/" + m, "type": "User"})
	}
	out := map[string]any{"schemas": []string{SchemaGroup}, "id": g.ID, "displayName": g.Name, "members": members,
		"meta": map[string]any{"resourceType": "Group", "created": g.Created.UTC().Format(time.RFC3339),
			"lastModified": g.Updated.UTC().Format(time.RFC3339), "location": h.base(r) + "/Groups/" + g.ID}}
	if g.ExternalID != "" {
		out["externalId"] = g.ExternalID
	}
	return out
}

type groupBody struct {
	DisplayName string      `json:"displayName"`
	ExternalID  string      `json:"externalId"`
	Members     []memberRef `json:"members"`
}

func (b groupBody) ids() []string {
	ids := []string{}
	for _, m := range b.Members {
		ids = append(ids, m.Value)
	}
	return ids
}

func (h *Server) groups(w http.ResponseWriter, r *http.Request, tok Token, seg []string) {
	ctx := r.Context()
	if len(seg) == 1 {
		switch r.Method {
		case http.MethodGet:
			f, err := ParseFilter(r.URL.Query().Get("filter"))
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalidFilter", err.Error())
				return
			}
			start, count := pageParams(r)
			list, total, err := h.Store.ListGroups(ctx, tok.OrgID, f, start, count)
			if err == errBadFilter {
				writeError(w, http.StatusBadRequest, "invalidFilter", "Filtering on that attribute is not supported (displayName, externalId, id are).")
				return
			}
			if err != nil {
				writeError(w, http.StatusInternalServerError, "", "Could not list groups.")
				return
			}
			res := make([]map[string]any, 0, len(list))
			for _, g := range list {
				res = append(res, h.groupJSON(r, g))
			}
			writeJSON(w, http.StatusOK, map[string]any{"schemas": []string{SchemaList}, "totalResults": total,
				"startIndex": start, "itemsPerPage": len(res), "Resources": res})
		case http.MethodPost:
			var b groupBody
			if decode(w, r, &b) != nil || strings.TrimSpace(b.DisplayName) == "" {
				writeError(w, http.StatusBadRequest, "invalidValue", "displayName is required.")
				return
			}
			g, err := h.Store.CreateGroup(ctx, tok.OrgID, tok.ID, strings.TrimSpace(b.DisplayName), b.ExternalID, b.ids())
			if errors.Is(err, errUniqueness) {
				writeError(w, http.StatusConflict, "uniqueness", "A group with that externalId already exists.")
				return
			}
			if err != nil {
				writeError(w, http.StatusInternalServerError, "", err.Error())
				return
			}
			h.audit(ctx, tok.OrgID, "scim.group.create", map[string]any{"group_id": g.ID, "name": g.Name, "token": tok.Name})
			w.Header().Set("Location", h.base(r)+"/Groups/"+g.ID)
			writeJSON(w, http.StatusCreated, h.groupJSON(r, g))
		default:
			writeError(w, http.StatusMethodNotAllowed, "", "Method not allowed.")
		}
		return
	}
	id := seg[1]
	switch r.Method {
	case http.MethodGet:
		g, err := h.Store.GetGroup(ctx, tok.OrgID, id)
		if err != nil {
			writeError(w, http.StatusNotFound, "", "Group not found.")
			return
		}
		writeJSON(w, http.StatusOK, h.groupJSON(r, g))
	case http.MethodPut:
		var b groupBody
		if decode(w, r, &b) != nil {
			writeError(w, http.StatusBadRequest, "invalidSyntax", "Body is not valid JSON.")
			return
		}
		p := groupPatch{DisplayName: &b.DisplayName, ExternalID: &b.ExternalID, Members: []memberOp{{Op: "replace", UserIDs: b.ids()}}}
		h.applyGroup(w, r, tok, id, p)
	case http.MethodPatch:
		var req PatchRequest
		if decode(w, r, &req) != nil {
			writeError(w, http.StatusBadRequest, "invalidSyntax", "Body is not valid JSON.")
			return
		}
		p, err := ParseGroupPatch(req)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalidValue", err.Error())
			return
		}
		h.applyGroup(w, r, tok, id, p)
	case http.MethodDelete:
		if err := h.Store.DeleteGroup(ctx, tok.OrgID, id); err != nil {
			if errors.Is(err, errNotFound) {
				writeError(w, http.StatusNotFound, "", "Group not found.")
				return
			}
			writeError(w, http.StatusInternalServerError, "", "Could not delete the group.")
			return
		}
		h.audit(ctx, tok.OrgID, "scim.group.delete", map[string]any{"group_id": id, "token": tok.Name})
		w.WriteHeader(http.StatusNoContent)
	default:
		writeError(w, http.StatusMethodNotAllowed, "", "Method not allowed.")
	}
}

func (h *Server) applyGroup(w http.ResponseWriter, r *http.Request, tok Token, id string, p groupPatch) {
	g, err := h.Store.UpdateGroup(r.Context(), tok.OrgID, id, p)
	switch {
	case errors.Is(err, errNotFound):
		writeError(w, http.StatusNotFound, "", "Group not found.")
		return
	case errors.Is(err, errUniqueness):
		writeError(w, http.StatusConflict, "uniqueness", "That externalId is used by another group.")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "", "Could not update the group.")
		return
	}
	h.audit(r.Context(), tok.OrgID, "scim.group.update", map[string]any{"group_id": id, "members": len(g.Members), "token": tok.Name})
	writeJSON(w, http.StatusOK, h.groupJSON(r, g))
}

// ---- discovery -------------------------------------------------------------

func (h *Server) serviceProviderConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"schemas":        []string{"urn:ietf:params:scim:schemas:core:2.0:ServiceProviderConfig"},
		"patch":          map[string]any{"supported": true},
		"bulk":           map[string]any{"supported": false, "maxOperations": 0, "maxPayloadSize": 0},
		"filter":         map[string]any{"supported": true, "maxResults": 200},
		"changePassword": map[string]any{"supported": false},
		"sort":           map[string]any{"supported": false},
		"etag":           map[string]any{"supported": false},
		"authenticationSchemes": []map[string]any{{"type": "oauthbearertoken", "name": "Bearer token",
			"description": "Org-scoped bearer token created in Admin > Provisioning", "primary": true}},
		"meta": map[string]any{"resourceType": "ServiceProviderConfig", "location": h.base(r) + "/ServiceProviderConfig"},
	})
}

func (h *Server) resourceTypes(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"schemas": []string{SchemaList}, "totalResults": 2, "Resources": []map[string]any{
		{"schemas": []string{"urn:ietf:params:scim:schemas:core:2.0:ResourceType"}, "id": "User", "name": "User", "endpoint": "/Users", "schema": SchemaUser,
			"schemaExtensions": []map[string]any{{"schema": SchemaEnterprise, "required": false}}},
		{"schemas": []string{"urn:ietf:params:scim:schemas:core:2.0:ResourceType"}, "id": "Group", "name": "Group", "endpoint": "/Groups", "schema": SchemaGroup},
	}})
}

func (h *Server) schemas(w http.ResponseWriter, r *http.Request) {
	attr := func(name, typ string, req bool) map[string]any {
		return map[string]any{"name": name, "type": typ, "multiValued": false, "required": req, "mutability": "readWrite", "returned": "default", "uniqueness": "none"}
	}
	writeJSON(w, http.StatusOK, map[string]any{"schemas": []string{SchemaList}, "totalResults": 2, "Resources": []map[string]any{
		{"id": SchemaUser, "name": "User", "description": "User", "attributes": []map[string]any{
			attr("userName", "string", true), attr("displayName", "string", false), attr("title", "string", false),
			attr("externalId", "string", false), attr("active", "boolean", false)}},
		{"id": SchemaGroup, "name": "Group", "description": "Group", "attributes": []map[string]any{
			attr("displayName", "string", true), attr("externalId", "string", false)}},
	}})
}
