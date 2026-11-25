package scim

import (
	"encoding/json"
	"fmt"
	"strings"
)

// PatchOp is one RFC 7644 PATCH operation.
type PatchOp struct {
	Op    string          `json:"op"`
	Path  string          `json:"path"`
	Value json.RawMessage `json:"value"`
}

// PatchRequest is a SCIM PatchOp message.
type PatchRequest struct {
	Schemas    []string  `json:"schemas"`
	Operations []PatchOp `json:"Operations"`
}

// boolValue reads a SCIM boolean, which Entra ID sends as the strings "True"/"False".
func boolValue(raw json.RawMessage) (bool, error) {
	var b bool
	if json.Unmarshal(raw, &b) == nil {
		return b, nil
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		switch strings.ToLower(strings.TrimSpace(s)) {
		case "true":
			return true, nil
		case "false":
			return false, nil
		}
	}
	return false, fmt.Errorf("expected a boolean")
}

func stringValue(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	return strings.Trim(string(raw), `"`)
}

// userPatch is the result of folding user PATCH operations into field changes.
type userPatch struct {
	Active      *bool
	UserName    *string
	DisplayName *string
	Given       *string
	Family      *string
	Email       *string
	Title       *string
	Phone       *string
	ExternalID  *string
	EmployeeID  *string
	Department  *string
}

func setStr(dst **string, raw json.RawMessage) { s := stringValue(raw); *dst = &s }

// applyUserAttr applies one (attribute, value) pair; attr is lower-cased.
func (p *userPatch) applyUserAttr(attr string, raw json.RawMessage) error {
	switch {
	case attr == "active":
		b, err := boolValue(raw)
		if err != nil {
			return fmt.Errorf("active: %w", err)
		}
		p.Active = &b
	case attr == "username":
		setStr(&p.UserName, raw)
	case attr == "displayname":
		setStr(&p.DisplayName, raw)
	case attr == "name.givenname":
		setStr(&p.Given, raw)
	case attr == "name.familyname":
		setStr(&p.Family, raw)
	case attr == "name.formatted":
		setStr(&p.DisplayName, raw)
	case attr == "name":
		var n struct {
			Given     string `json:"givenName"`
			Family    string `json:"familyName"`
			Formatted string `json:"formatted"`
		}
		if json.Unmarshal(raw, &n) != nil {
			return fmt.Errorf("name must be an object")
		}
		if n.Given != "" {
			p.Given = &n.Given
		}
		if n.Family != "" {
			p.Family = &n.Family
		}
		if n.Formatted != "" {
			p.DisplayName = &n.Formatted
		}
	case attr == "title":
		setStr(&p.Title, raw)
	case attr == "externalid":
		setStr(&p.ExternalID, raw)
	case strings.HasPrefix(attr, "emails"):
		// emails[type eq "work"].value, emails.value, or emails as a list
		var list []struct {
			Value   string `json:"value"`
			Primary bool   `json:"primary"`
		}
		if json.Unmarshal(raw, &list) == nil && len(list) > 0 {
			e := list[0].Value
			for _, x := range list {
				if x.Primary {
					e = x.Value
				}
			}
			p.Email = &e
		} else if strings.HasSuffix(attr, ".value") || attr == "emails" {
			setStr(&p.Email, raw)
		}
	case strings.HasPrefix(attr, "phonenumbers"):
		var list []struct {
			Value string `json:"value"`
		}
		if json.Unmarshal(raw, &list) == nil && len(list) > 0 {
			p.Phone = &list[0].Value
		} else if strings.HasSuffix(attr, ".value") {
			setStr(&p.Phone, raw)
		}
	case strings.HasSuffix(attr, ":employeenumber") || attr == "employeenumber":
		setStr(&p.EmployeeID, raw)
	case strings.HasSuffix(attr, ":department") || attr == "department":
		setStr(&p.Department, raw)
	default:
		// Attributes we do not store (addresses, roles, photos...) are accepted
		// and ignored, as IdPs expect from a minimal server.
	}
	return nil
}

// ParseUserPatch folds operations into a userPatch. Both the path form
// ({"op":"Replace","path":"active","value":"False"}) and the no-path form
// ({"op":"replace","value":{"active":false}}) are supported.
func ParseUserPatch(req PatchRequest) (userPatch, error) {
	var p userPatch
	for _, op := range req.Operations {
		kind := strings.ToLower(op.Op)
		if kind != "add" && kind != "replace" && kind != "remove" {
			return p, fmt.Errorf("unsupported op %q", op.Op)
		}
		path := strings.ToLower(strings.TrimSpace(op.Path))
		path = strings.TrimPrefix(path, "urn:ietf:params:scim:schemas:core:2.0:user:")
		if kind == "remove" {
			empty := json.RawMessage(`""`)
			if path == "" {
				return p, fmt.Errorf("remove needs a path")
			}
			if path == "active" {
				return p, fmt.Errorf("active cannot be removed")
			}
			if err := p.applyUserAttr(path, empty); err != nil {
				return p, err
			}
			continue
		}
		if path != "" {
			if err := p.applyUserAttr(path, op.Value); err != nil {
				return p, err
			}
			continue
		}
		var obj map[string]json.RawMessage
		if json.Unmarshal(op.Value, &obj) != nil {
			return p, fmt.Errorf("a value without a path must be an object")
		}
		for k, v := range obj {
			key := strings.ToLower(k)
			if strings.HasPrefix(key, "urn:") && strings.Contains(key, "enterprise") {
				var ent map[string]json.RawMessage
				if json.Unmarshal(v, &ent) == nil {
					for ek, ev := range ent {
						if err := p.applyUserAttr(strings.ToLower(ek), ev); err != nil {
							return p, err
						}
					}
				}
				continue
			}
			if err := p.applyUserAttr(key, v); err != nil {
				return p, err
			}
		}
	}
	return p, nil
}

// memberOp is one change to a group's members.
type memberOp struct {
	Op      string // add | remove | replace
	UserIDs []string
	All     bool // remove without a filter (clear)
}

type groupPatch struct {
	DisplayName *string
	ExternalID  *string
	Members     []memberOp
}

type memberRef struct {
	Value string `json:"value"`
}

// ParseGroupPatch folds group PATCH operations: add/remove/replace members
// (`members`, `members[value eq "id"]`) and displayName.
func ParseGroupPatch(req PatchRequest) (groupPatch, error) {
	var g groupPatch
	for _, op := range req.Operations {
		kind := strings.ToLower(op.Op)
		if kind != "add" && kind != "replace" && kind != "remove" {
			return g, fmt.Errorf("unsupported op %q", op.Op)
		}
		path := strings.TrimSpace(op.Path)
		lp := strings.ToLower(path)
		switch {
		case lp == "displayname":
			setStr(&g.DisplayName, op.Value)
		case lp == "externalid":
			setStr(&g.ExternalID, op.Value)
		case lp == "members" || lp == "":
			if lp == "" {
				var obj map[string]json.RawMessage
				if json.Unmarshal(op.Value, &obj) == nil {
					for k, v := range obj {
						if strings.EqualFold(k, "displayName") {
							setStr(&g.DisplayName, v)
						}
						if strings.EqualFold(k, "members") {
							ids, err := memberIDs(v)
							if err != nil {
								return g, err
							}
							g.Members = append(g.Members, memberOp{Op: kind, UserIDs: ids})
						}
					}
					continue
				}
				return g, fmt.Errorf("a value without a path must be an object")
			}
			if kind == "remove" && len(op.Value) == 0 {
				g.Members = append(g.Members, memberOp{Op: "remove", All: true})
				continue
			}
			ids, err := memberIDs(op.Value)
			if err != nil {
				return g, err
			}
			g.Members = append(g.Members, memberOp{Op: kind, UserIDs: ids})
		case strings.HasPrefix(lp, "members[") && kind == "remove":
			// members[value eq "id"]
			start := strings.Index(path, `"`)
			end := strings.LastIndex(path, `"`)
			if start < 0 || end <= start {
				return g, fmt.Errorf("bad members filter %q", path)
			}
			g.Members = append(g.Members, memberOp{Op: "remove", UserIDs: []string{path[start+1 : end]}})
		default:
			return g, fmt.Errorf("unsupported group path %q", path)
		}
	}
	return g, nil
}

func memberIDs(raw json.RawMessage) ([]string, error) {
	var refs []memberRef
	if err := json.Unmarshal(raw, &refs); err != nil {
		var one memberRef
		if json.Unmarshal(raw, &one) != nil {
			return nil, fmt.Errorf("members must be a list of {value}")
		}
		refs = []memberRef{one}
	}
	out := make([]string, 0, len(refs))
	for _, r := range refs {
		if r.Value != "" {
			out = append(out, r.Value)
		}
	}
	return out, nil
}
