package drive

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Adrient-tech/Adrient-Workspace/internal/auth"
	"github.com/Adrient-tech/Adrient-Workspace/internal/authz"
	"github.com/Adrient-tech/Adrient-Workspace/internal/sharing"
)

// LinkViewer describes the person opening a link.
type LinkViewer struct {
	SignedIn  bool
	UserID    string
	OrgID     string
	IsGrantee bool // holds a per-user grant on the file
}

// LinkAllowed is the single rule for opening a share link:
//
//	anyone    always
//	org_only  signed in, in the file's organization
//	named     signed in and the owner or a person the item is shared with
func LinkAllowed(visibility, fileOrgID, fileOwnerID string, v LinkViewer) error {
	switch visibility {
	case VisAnyone:
		return nil
	case VisOrgOnly:
		if !v.SignedIn {
			return errLinkSignIn
		}
		if v.OrgID != fileOrgID {
			return errLinkDenied
		}
		return nil
	case VisNamed:
		if !v.SignedIn {
			return errLinkSignIn
		}
		if v.UserID == fileOwnerID || v.IsGrantee {
			return nil
		}
		return errLinkDenied
	}
	return errLinkDenied // unknown visibility fails closed
}

var (
	errLinkSignIn = errors.New("sign in to open this link")
	errLinkDenied = errors.New("this link is not available to you")
)

// LinkHandler serves share links: GET /api/v1/drive/link/{token} returns the
// file's details and GET /api/v1/drive/link/{token}/content streams it. The
// link's visibility is enforced here on every request.
func (s *Service) LinkHandler() http.Handler {
	const prefix = "/api/v1/drive/link/"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		rest := strings.TrimPrefix(strings.TrimRight(r.URL.Path, "/"), prefix)
		content := strings.HasSuffix(rest, "/content")
		token := strings.TrimSuffix(rest, "/content")
		if token == "" || strings.Contains(token, "/") || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		sh, err := s.acl.LookupShare(ctx, token)
		if err != nil {
			http.Error(w, "link not found, expired or revoked", http.StatusNotFound)
			return
		}
		f, err := s.repo.GetByID(ctx, sh.FileID)
		if err != nil || f.TrashedAt != nil {
			http.Error(w, "link not found, expired or revoked", http.StatusNotFound)
			return
		}
		viewer := LinkViewer{}
		if u, ok := auth.UserFromContext(ctx); ok {
			viewer = LinkViewer{SignedIn: true, UserID: u.ID, OrgID: u.OrgID}
			if s.grants != nil {
				if _, granted, _ := s.grants.RoleFor(ctx, u.ID, sharing.TypeDriveFile, f.ID); granted {
					viewer.IsGrantee = true
				}
			}
		}
		if lerr := LinkAllowed(sh.Visibility, f.OrgID, f.OwnerID, viewer); lerr != nil {
			code := http.StatusForbidden
			if lerr == errLinkSignIn {
				code = http.StatusUnauthorized
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(code)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": lerr.Error(), "visibility": sh.Visibility, "sign_in": lerr == errLinkSignIn})
			return
		}
		if !content {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": f.Name, "mime_type": f.MimeType, "size": f.SizeBytes, "role": sh.Role, "visibility": sh.Visibility,
				"is_folder": f.StorageKey == nil,
			})
			return
		}
		if f.StorageKey == nil {
			http.Error(w, "not a file", http.StatusBadRequest)
			return
		}
		body, mime, size, gerr := s.blobs.Get(ctx, *f.StorageKey)
		if gerr != nil {
			http.Error(w, "blob get: "+gerr.Error(), http.StatusInternalServerError)
			return
		}
		defer body.Close()
		w.Header().Set("Content-Type", mime)
		if size > 0 {
			w.Header().Set("Content-Length", fmt.Sprintf("%d", size))
		}
		disp := "inline"
		if r.URL.Query().Get("download") == "1" {
			disp = "attachment"
			if !authz.ControlBool(ctx, "app.export_data") {
				authz.WriteControlDenied(w, &authz.ControlDenied{Control: "app.export_data", Reason: "Downloads and exports are turned off for your account."})
				return
			}
		}
		w.Header().Set("Content-Disposition", fmt.Sprintf("%s; filename=\"%s\"", disp, strings.ReplaceAll(f.Name, "\"", "\\\"")))
		_, _ = io.Copy(w, body)
	})
}
