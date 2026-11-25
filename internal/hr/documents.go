package hr

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"path"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
)

var (
	panRe   = regexp.MustCompile(`^[A-Z]{5}[0-9]{4}[A-Z]$`)
	ifscRe  = regexp.MustCompile(`^[A-Z]{4}0[A-Z0-9]{6}$`)
	last4Re = regexp.MustCompile(`^[0-9]{4}$`)
)

// MaxDocumentBytes caps one HR document.
const MaxDocumentBytes = 20 << 20

var docCategories = map[string]bool{"offer_letter": true, "id_proof": true, "address_proof": true, "contract": true, "education": true, "other": true}

// Document is an employee document. The bytes are in Drive blob storage and are
// served only through this module to HR and the employee.
type Document struct {
	ID         string `json:"id"`
	EmployeeID string `json:"employee_id"`
	Category   string `json:"category"`
	Name       string `json:"name"`
	MimeType   string `json:"mime_type"`
	Size       int64  `json:"size_bytes"`
	CreatedAt  string `json:"created_at"`
	DriveFile  string `json:"drive_file_id"`
}

func (a Actor) ownsOrHR(empID string) bool {
	return a.IsHR || (a.EmployeeID != "" && a.EmployeeID == empID)
}

// ListDocuments lists documents for HR and the employee only.
func (r *Repository) ListDocuments(ctx context.Context, a Actor, empID string) ([]Document, error) {
	if !a.ownsOrHR(empID) {
		return nil, forbidden("documents are visible to HR and the employee only")
	}
	if !isUUID(empID) {
		return nil, ErrNotFound
	}
	rows, err := r.pool.Query(ctx, `SELECT id::text, employee_id::text, category, name, mime_type, size_bytes, to_char(created_at,'YYYY-MM-DD'), coalesce(drive_file_id::text,'')
		FROM adrient.hr_documents WHERE org_id=$1 AND employee_id=$2::uuid ORDER BY created_at DESC`, a.OrgID, empID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Document{}
	for rows.Next() {
		var d Document
		if err := rows.Scan(&d.ID, &d.EmployeeID, &d.Category, &d.Name, &d.MimeType, &d.Size, &d.CreatedAt, &d.DriveFile); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// AddDocument stores a document. HR may add for anyone; an employee may add ID
// and address proofs to their own record.
func (r *Repository) AddDocument(ctx context.Context, a Actor, empID, category, filename, mime string, size int64, body io.Reader) (Document, error) {
	var d Document
	if !a.ownsOrHR(empID) {
		return d, forbidden("not allowed")
	}
	if !a.IsHR && category != "id_proof" && category != "address_proof" && category != "education" {
		return d, forbidden("employees can add ID, address and education proofs; HR adds the rest")
	}
	if r.blobs == nil || r.files == nil {
		return d, errors.New("document storage is not configured")
	}
	if !docCategories[category] {
		return d, invalid("unknown document category")
	}
	if size <= 0 || size > MaxDocumentBytes {
		return d, invalid("a document must be between 1 byte and 20 MB")
	}
	name := path.Base(strings.ReplaceAll(strings.TrimSpace(filename), `\`, "/"))
	if name == "" || name == "." || name == "/" {
		return d, invalid("a file name is required")
	}
	if _, err := r.getEmployee(ctx, r.pool, a.OrgID, empID); err != nil {
		return d, err
	}
	if mime == "" {
		mime = "application/octet-stream"
	}
	key, err := newStorageKey()
	if err != nil {
		return d, err
	}
	if err := r.blobs.Put(ctx, key, mime, size, body); err != nil {
		return d, err
	}
	fileID, err := r.files.CreateHRFile(ctx, a.OrgID, a.UserID, name, mime, key, size)
	if err != nil {
		_ = r.blobs.Delete(ctx, key)
		return d, err
	}
	err = r.pool.QueryRow(ctx, `INSERT INTO adrient.hr_documents (org_id, employee_id, category, name, drive_file_id, storage_key, mime_type, size_bytes, uploaded_by)
		VALUES ($1,$2::uuid,$3,$4,NULLIF($5,'')::uuid,$6,$7,$8,$9::uuid) RETURNING id::text, employee_id::text, category, name, mime_type, size_bytes, to_char(created_at,'YYYY-MM-DD'), coalesce(drive_file_id::text,'')`,
		a.OrgID, empID, category, name, fileID, key, mime, size, a.UserID).
		Scan(&d.ID, &d.EmployeeID, &d.Category, &d.Name, &d.MimeType, &d.Size, &d.CreatedAt, &d.DriveFile)
	if err != nil {
		_ = r.blobs.Delete(ctx, key)
		return d, err
	}
	r.audit(ctx, r.pool, a.OrgID, a.UserID, "upload", "document", d.ID, category+" "+name)
	return d, nil
}

// OpenDocument returns the document bytes for HR and the employee only.
func (r *Repository) OpenDocument(ctx context.Context, a Actor, id string) (Document, io.ReadCloser, error) {
	var d Document
	var key string
	if !isUUID(id) {
		return d, nil, ErrNotFound
	}
	err := r.pool.QueryRow(ctx, `SELECT id::text, employee_id::text, category, name, mime_type, size_bytes, to_char(created_at,'YYYY-MM-DD'), storage_key
		FROM adrient.hr_documents WHERE org_id=$1 AND id=$2::uuid`, a.OrgID, id).Scan(&d.ID, &d.EmployeeID, &d.Category, &d.Name, &d.MimeType, &d.Size, &d.CreatedAt, &key)
	if errors.Is(err, pgx.ErrNoRows) {
		return d, nil, ErrNotFound
	}
	if err != nil {
		return d, nil, err
	}
	if !a.ownsOrHR(d.EmployeeID) {
		return Document{}, nil, ErrNotFound // do not reveal that it exists
	}
	if r.blobs == nil {
		return d, nil, errors.New("document storage is not configured")
	}
	rc, _, _, err := r.blobs.Get(ctx, key)
	return d, rc, err
}

// DeleteDocument removes a document (HR, or the employee for their own uploads).
func (r *Repository) DeleteDocument(ctx context.Context, a Actor, id string) error {
	if !isUUID(id) {
		return ErrNotFound
	}
	var empID, key, by string
	err := r.pool.QueryRow(ctx, `SELECT employee_id::text, storage_key, coalesce(uploaded_by::text,'') FROM adrient.hr_documents WHERE org_id=$1 AND id=$2::uuid`, a.OrgID, id).Scan(&empID, &key, &by)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if !a.IsHR && !(a.EmployeeID == empID && by == a.UserID) {
		return forbidden("only HR can delete this document")
	}
	if _, err := r.pool.Exec(ctx, `DELETE FROM adrient.hr_documents WHERE id=$1::uuid`, id); err != nil {
		return err
	}
	_, _ = r.pool.Exec(ctx, `DELETE FROM adrient.drive_files WHERE org_id=$1 AND storage_key=$2`, a.OrgID, key)
	if r.blobs != nil {
		_ = r.blobs.Delete(ctx, key)
	}
	r.audit(ctx, r.pool, a.OrgID, a.UserID, "delete", "document", id, "")
	return nil
}

func newStorageKey() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "blobs/" + hex.EncodeToString(buf[:8]) + "/" + hex.EncodeToString(buf[8:]), nil
}
