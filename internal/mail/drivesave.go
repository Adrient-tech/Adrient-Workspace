package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/Adrient-tech/Adrient-Workspace/internal/auth"
	"github.com/Adrient-tech/Adrient-Workspace/internal/drive"
)

// MailAttachmentsFolder is the Drive folder that "Open"/"Save to Drive" copies
// mail attachments into when the caller names no folder.
const MailAttachmentsFolder = "Mail attachments"

// SetDrive wires the Drive repository so attachments can move between Mail
// and Drive (save-to-drive, attach-from-drive). Optional.
func (a *Attachments) SetDrive(d *drive.Repository) { a.drive = d }

// saveToDrivePath parses /api/v1/mail/messages/{id}/attachments/{aid}/save-to-drive.
func saveToDrivePath(path string) (msgID, attID string, ok bool) {
	const prefix = "/api/v1/mail/messages/"
	const suffix = "/save-to-drive"
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		return "", "", false
	}
	parts := strings.Split(strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix), "/")
	if len(parts) != 3 || parts[1] != "attachments" || parts[0] == "" || parts[2] == "" {
		return "", "", false
	}
	return parts[0], parts[2], true
}

// Route returns the handler for the Drive-bridge endpoints, or nil when the
// request is not one of them.
func (a *Attachments) Route(path, method string) http.Handler {
	if a.drive == nil || method != http.MethodPost {
		return nil
	}
	path = strings.TrimRight(path, "/")
	if _, _, ok := saveToDrivePath(path); ok {
		return a.SaveToDriveHandler()
	}
	if path == "/api/v1/mail/attachments/from-drive" {
		return a.FromDriveHandler()
	}
	return nil
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// folderFor returns the destination folder id: the requested one, or the
// caller's "Mail attachments" folder (created on first use).
func (a *Attachments) folderFor(ctx context.Context, orgID, userID, requested string) (string, error) {
	if requested == "root" {
		return "", nil
	}
	if requested != "" {
		f, err := a.drive.Get(ctx, orgID, requested)
		if err != nil {
			return "", err
		}
		if f.MimeType != drive.FolderMimeType {
			return "", errors.New("target is not a folder")
		}
		return f.ID, nil
	}
	token := ""
	for {
		items, next, err := a.drive.ListChildren(ctx, orgID, "", false, 200, token)
		if err != nil {
			return "", err
		}
		for _, f := range items {
			if f.MimeType == drive.FolderMimeType && f.OwnerID == userID && f.Name == MailAttachmentsFolder {
				return f.ID, nil
			}
		}
		if next == "" {
			break
		}
		token = next
	}
	f, err := a.drive.CreateFolder(ctx, orgID, userID, "", MailAttachmentsFolder)
	if err != nil {
		return "", err
	}
	return f.ID, nil
}

// SaveToDriveHandler copies a mail attachment into the caller's Drive and
// returns {"file":{id,name,mime_type,size}}. Calling it again for the same
// attachment returns the already-saved file (deduped), unless a different
// folder is requested.
func (a *Attachments) SaveToDriveHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := auth.UserFromContext(r.Context())
		o, ok2 := auth.OrgFromContext(r.Context())
		if !ok || !ok2 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		msgID, attID, ok := saveToDrivePath(strings.TrimRight(r.URL.Path, "/"))
		if !ok {
			http.Error(w, "bad path", http.StatusBadRequest)
			return
		}
		var req struct {
			FolderID string `json:"folder_id"`
		}
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req)

		msg, err := a.repo.Get(r.Context(), u.ID, msgID)
		if err != nil {
			http.Error(w, "message not found", http.StatusNotFound)
			return
		}
		found := false
		for _, at := range msg.Attachments {
			if at.ID == attID {
				found = true
				break
			}
		}
		if !found {
			http.Error(w, "attachment not on message", http.StatusNotFound)
			return
		}
		meta, err := a.repo.GetAttachment(r.Context(), o.ID, attID)
		if err != nil {
			http.Error(w, "attachment not found", http.StatusNotFound)
			return
		}
		if prev, err := a.repo.SavedDriveFile(r.Context(), attID, u.ID); err == nil && prev != "" && req.FolderID == "" {
			if f, err := a.drive.Get(r.Context(), o.ID, prev); err == nil && f.TrashedAt == nil {
				writeJSON(w, map[string]any{"file": driveFileJSON(f), "existing": true})
				return
			}
		}
		folder, err := a.folderFor(r.Context(), o.ID, u.ID, req.FolderID)
		if err != nil {
			http.Error(w, "folder: "+err.Error(), http.StatusBadRequest)
			return
		}
		body, ct, size, err := a.blobs.Get(r.Context(), meta.BlobKey)
		if err != nil {
			http.Error(w, "fetch blob", http.StatusInternalServerError)
			return
		}
		defer body.Close()
		if ct == "" || ct == "application/octet-stream" {
			ct = meta.ContentType
		}
		if size <= 0 {
			size = meta.Size
		}
		key, err := drive.NewStorageKey()
		if err != nil {
			http.Error(w, "key", http.StatusInternalServerError)
			return
		}
		// S3 puts need a seekable body to sign the payload: buffer it.
		buf, err := io.ReadAll(io.LimitReader(body, maxAttachment+1))
		if err != nil || int64(len(buf)) > maxAttachment {
			http.Error(w, "read blob", http.StatusInternalServerError)
			return
		}
		size = int64(len(buf))
		if err := a.blobs.Put(r.Context(), key, ct, size, bytes.NewReader(buf)); err != nil {
			log.Printf("mail save-to-drive: store blob: %v", err)
			http.Error(w, "store blob", http.StatusInternalServerError)
			return
		}
		f, err := a.drive.CreateFile(r.Context(), o.ID, u.ID, folder, meta.Filename, ct, key, size)
		// A different attachment with the same name already sits in the folder
		// (unique per parent): keep both, as "name (2).ext".
		for n := 2; err != nil && strings.Contains(err.Error(), "drive_files_org_id_parent_id_name_key") && n < 50; n++ {
			f, err = a.drive.CreateFile(r.Context(), o.ID, u.ID, folder, numberedName(meta.Filename, n), ct, key, size)
		}
		if err != nil {
			http.Error(w, "create drive file: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if req.FolderID == "" {
			_ = a.repo.SetSavedDriveFile(r.Context(), attID, u.ID, f.ID)
		}
		writeJSON(w, map[string]any{"file": driveFileJSON(f)})
	})
}

// FromDriveHandler copies a Drive file into a mail attachment (a real
// attachment on the next send): body {"file_id"} → {"attachments":[...]}.
func (a *Attachments) FromDriveHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := auth.UserFromContext(r.Context())
		o, ok2 := auth.OrgFromContext(r.Context())
		if !ok || !ok2 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var req struct {
			FileID string `json:"file_id"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil || req.FileID == "" {
			http.Error(w, "file_id required", http.StatusBadRequest)
			return
		}
		f, err := a.drive.Get(r.Context(), o.ID, req.FileID)
		if err != nil || f.TrashedAt != nil {
			http.Error(w, "file not found", http.StatusNotFound)
			return
		}
		if f.StorageKey == nil || f.MimeType == drive.FolderMimeType {
			http.Error(w, "not a file", http.StatusBadRequest)
			return
		}
		if f.SizeBytes > maxAttachment {
			http.Error(w, "file too large to attach", http.StatusRequestEntityTooLarge)
			return
		}
		body, ct, size, err := a.blobs.Get(r.Context(), *f.StorageKey)
		if err != nil {
			http.Error(w, "fetch blob", http.StatusInternalServerError)
			return
		}
		defer body.Close()
		if ct == "" {
			ct = f.MimeType
		}
		if size <= 0 {
			size = f.SizeBytes
		}
		key := randKey()
		buf, err := io.ReadAll(io.LimitReader(body, maxAttachment+1))
		if err != nil || int64(len(buf)) > maxAttachment {
			http.Error(w, "read blob", http.StatusInternalServerError)
			return
		}
		size = int64(len(buf))
		if err := a.blobs.Put(r.Context(), key, ct, size, bytes.NewReader(buf)); err != nil {
			log.Printf("mail from-drive: store blob: %v", err)
			http.Error(w, "store blob", http.StatusInternalServerError)
			return
		}
		meta, err := a.repo.CreateAttachment(r.Context(), AttachmentMeta{
			Attachment: Attachment{Filename: f.Name, ContentType: ct, Size: size},
			OrgID:      o.ID, OwnerID: u.ID, BlobKey: key,
		})
		if err != nil {
			http.Error(w, "save attachment", http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"attachments": []Attachment{meta.Attachment}})
	})
}

func driveFileJSON(f drive.File) map[string]any {
	return map[string]any{"id": f.ID, "name": f.Name, "mime_type": f.MimeType, "size": f.SizeBytes, "parent_id": f.ParentID}
}

// numberedName turns "q3.xlsx" into "q3 (2).xlsx".
func numberedName(name string, n int) string {
	ext := path.Ext(name)
	return strings.TrimSuffix(name, ext) + " (" + strconv.Itoa(n) + ")" + ext
}
