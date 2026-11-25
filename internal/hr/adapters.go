package hr

import (
	"context"
	"errors"
	"time"

	"github.com/Adrient-tech/Adrient-Workspace/internal/accounting"
	"github.com/Adrient-tech/Adrient-Workspace/internal/calendar"
	"github.com/Adrient-tech/Adrient-Workspace/internal/drive"
	"github.com/Adrient-tech/Adrient-Workspace/internal/notifications"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AccountingAdapter posts payroll through the Accounting repository.
type AccountingAdapter struct{ Repo *accounting.Repository }

// PostPayroll implements PayrollPoster.
func (a AccountingAdapter) PostPayroll(ctx context.Context, orgID, userID string, in PayrollJournal) (string, error) {
	return a.Repo.PostPayroll(ctx, orgID, userID, accounting.PayrollPosting{RunID: in.RunID, Period: in.Period, Date: in.Date, Gross: in.Gross,
		EmployerContrib: in.EmployerContrib, Reimburse: in.Reimburse, PFPayable: in.PFPayable, ESIPayable: in.ESIPayable, PTPayable: in.PTPayable,
		TDSPayable: in.TDSPayable, OtherDed: in.OtherDed, NetSalaryDue: in.NetSalaryDue, Employees: in.Employees})
}

// ListApprovedReports implements PayrollPoster.
func (a AccountingAdapter) ListApprovedReports(ctx context.Context, orgID, userID string) ([]ExpenseClaim, error) {
	rows, err := a.Repo.ApprovedClaimsFor(ctx, orgID, userID)
	if err != nil {
		return nil, err
	}
	out := make([]ExpenseClaim, 0, len(rows))
	for _, c := range rows {
		out = append(out, ExpenseClaim{ID: c.ID, Title: c.Title, Amount: c.Amount})
	}
	return out, nil
}

// MarkReportsReimbursed implements PayrollPoster.
func (a AccountingAdapter) MarkReportsReimbursed(ctx context.Context, orgID, userID string, ids []string, date string) error {
	return a.Repo.MarkReportsReimbursedByPayroll(ctx, orgID, userID, ids, date)
}

// CalendarAdapter writes out-of-office items through the Calendar repository.
type CalendarAdapter struct{ Repo *calendar.Repository }

// CreateOutOfOffice implements CalendarPort.
func (c CalendarAdapter) CreateOutOfOffice(ctx context.Context, orgID, ownerID, title string, start, end time.Time, allDay bool) (string, error) {
	e, err := c.Repo.Create(ctx, orgID, ownerID, calendar.Fields{Title: title, Description: "Approved leave (Adrient HR)", StartAt: start, EndAt: end, AllDay: allDay,
		Color: "#126C5A", ItemType: calendar.ItemTypeOutOfOffice, Status: calendar.StatusBusy, Visibility: calendar.VisibilityDefault})
	return e.ID, err
}

// DeleteEvent implements CalendarPort.
func (c CalendarAdapter) DeleteEvent(ctx context.Context, orgID, id string) error {
	err := c.Repo.Delete(ctx, orgID, id)
	if errors.Is(err, calendar.ErrNotFound) {
		return nil
	}
	return err
}

// NotificationsAdapter delivers through the in-app notification feed.
type NotificationsAdapter struct{ Repo *notifications.Repository }

// Notify implements Notifier.
func (n NotificationsAdapter) Notify(ctx context.Context, orgID, toUserID, actorUserID, typ, title, body, url string) error {
	_, err := n.Repo.Create(ctx, notifications.CreateParams{OrgID: orgID, UserID: toUserID, Type: typ, ActorUserID: actorUserID, Title: title, Body: body, TargetURL: url})
	return err
}

// DriveAdapter records HR documents as Drive files in an "HR Documents" folder
// owned by the uploader.
type DriveAdapter struct {
	Pool *pgxpool.Pool
	Repo *drive.Repository
}

// CreateHRFile implements DriveFiles.
func (d DriveAdapter) CreateHRFile(ctx context.Context, orgID, ownerID, name, mimeType, storageKey string, size int64) (string, error) {
	var folder string
	err := d.Pool.QueryRow(ctx, `SELECT id::text FROM adrient.drive_files WHERE org_id=$1 AND owner_id=$2::uuid AND parent_id IS NULL AND name='HR Documents'
		AND mime_type=$3 AND trashed_at IS NULL ORDER BY created_at LIMIT 1`, orgID, ownerID, drive.FolderMimeType).Scan(&folder)
	if errors.Is(err, pgx.ErrNoRows) {
		f, err := d.Repo.CreateFolder(ctx, orgID, ownerID, "", "HR Documents")
		if err != nil {
			return "", err
		}
		folder = f.ID
	} else if err != nil {
		return "", err
	}
	f, err := d.Repo.CreateFile(ctx, orgID, ownerID, folder, name, mimeType, storageKey, size)
	return f.ID, err
}
