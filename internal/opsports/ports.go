// Package opsports implements the opshttp ports (Contacts, Invoicing, Calendar, Accounting)
// with the real repositories. The server wires them into the ops modules and the demo seeder reuses them.
package opsports

import (
	"context"
	"strings"

	"github.com/Adrient-tech/Adrient-Workspace/internal/accounting"
	"github.com/Adrient-tech/Adrient-Workspace/internal/calendar"
	"github.com/Adrient-tech/Adrient-Workspace/internal/contacts"
	"github.com/Adrient-tech/Adrient-Workspace/internal/invoicing"
	"github.com/Adrient-tech/Adrient-Workspace/internal/opshttp"
)

// Adapters that give the ops modules (opshttp ports) access to Contacts,
// Invoicing, Calendar and Accounting through their repositories.

type Contacts struct{ Repo *contacts.Repository }

func toRef(c contacts.Contact) opshttp.ContactRef {
	r := opshttp.ContactRef{ID: c.ID, Name: c.DisplayName, Company: c.Company}
	if len(c.Emails) > 0 {
		r.Email = c.Emails[0]
	}
	if len(c.Phones) > 0 {
		r.Phone = c.Phones[0]
	}
	return r
}

func (p Contacts) Search(ctx context.Context, org, q string, limit int) ([]opshttp.ContactRef, error) {
	all, err := p.Repo.List(ctx, org, contacts.ListFilter{})
	if err != nil {
		return nil, err
	}
	q = strings.ToLower(strings.TrimSpace(q))
	out := []opshttp.ContactRef{}
	for _, c := range all {
		hay := strings.ToLower(c.DisplayName + " " + c.Company + " " + strings.Join(c.Emails, " ") + " " + strings.Join(c.Phones, " "))
		if q == "" || strings.Contains(hay, q) {
			out = append(out, toRef(c))
			if limit > 0 && len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

func (p Contacts) Get(ctx context.Context, org, id string) (opshttp.ContactRef, error) {
	c, err := p.Repo.Get(ctx, org, id)
	if err != nil {
		return opshttp.ContactRef{}, opshttp.ErrNotFound
	}
	return toRef(c), nil
}

func (p Contacts) Create(ctx context.Context, org, user string, c opshttp.ContactRef) (opshttp.ContactRef, error) {
	name := strings.TrimSpace(c.Name)
	if name == "" {
		return c, opshttp.Invalid("a contact needs a name")
	}
	f := contacts.Fields{DisplayName: name, Company: strings.TrimSpace(c.Company)}
	if c.Email != "" {
		f.Emails = []string{strings.TrimSpace(c.Email)}
	}
	if c.Phone != "" {
		f.Phones = []string{strings.TrimSpace(c.Phone)}
	}
	made, err := p.Repo.Create(ctx, org, user, f)
	if err != nil {
		return c, err
	}
	return toRef(made), nil
}

type Invoices struct{ Repo *invoicing.Repository }

func toDocRef(d invoicing.Document) opshttp.DocRef {
	return opshttp.DocRef{ID: d.ID, Kind: d.Kind, Number: d.Number, Status: d.DisplayStatus, Total: d.Total, Paid: d.Paid, Balance: d.Balance}
}

func (p Invoices) Create(ctx context.Context, org, user string, d opshttp.InvoiceDraft) (opshttp.DocRef, error) {
	lines := make([]invoicing.Line, 0, len(d.Lines))
	for _, l := range d.Lines {
		lines = append(lines, invoicing.Line{Description: l.Description, Qty: l.Qty, Unit: l.Unit, Price: l.Price, TaxRate: l.TaxRate, TaxName: "GST"})
	}
	in := invoicing.DocInput{Kind: d.Kind, ContactID: d.ContactID, ClientName: d.ClientName, ClientEmail: d.ClientEmail, Currency: d.Currency,
		IssueDate: d.IssueDate, DueDate: d.DueDate, Lines: lines, Notes: d.Notes, SourceRef: d.SourceRef, DiscountKind: d.DiscountKind, DiscountValue: d.DiscountValue,
		Schedule: []invoicing.Schedule{}, Attachments: []invoicing.Attachment{}}
	doc, err := p.Repo.SaveDocument(ctx, org, user, "", in)
	if err != nil {
		return opshttp.DocRef{}, err
	}
	return toDocRef(doc), nil
}

func (p Invoices) Get(ctx context.Context, org, id string) (opshttp.DocRef, error) {
	d, err := p.Repo.GetDocument(ctx, org, id)
	if err != nil {
		return opshttp.DocRef{}, opshttp.ErrNotFound
	}
	return toDocRef(d), nil
}

func (p Invoices) ConvertEstimate(ctx context.Context, org, user, id string) (opshttp.DocRef, error) {
	d, err := p.Repo.ConvertEstimate(ctx, org, user, id)
	if err != nil {
		return opshttp.DocRef{}, err
	}
	return toDocRef(d), nil
}

type Calendar struct{ Repo *calendar.Repository }

func calFields(e opshttp.CalEvent) calendar.Fields {
	return calendar.Fields{Title: e.Title, Description: e.Description, Location: e.Location, StartAt: e.Start, EndAt: e.End, AllDay: e.AllDay,
		Color: e.Color, ItemType: calendar.ItemTypeEvent, Status: calendar.StatusBusy, Visibility: calendar.VisibilityDefault}
}

func (p Calendar) Create(ctx context.Context, org, owner string, e opshttp.CalEvent) (string, error) {
	ev, err := p.Repo.Create(ctx, org, owner, calFields(e))
	return ev.ID, err
}

func (p Calendar) Update(ctx context.Context, org, id string, e opshttp.CalEvent) error {
	_, err := p.Repo.Update(ctx, org, id, calFields(e))
	return err
}

func (p Calendar) Delete(ctx context.Context, org, id string) error {
	return p.Repo.Delete(ctx, org, id)
}

type Expenses struct{ Repo *accounting.Repository }

func (p Expenses) Categories(ctx context.Context, org string) ([]opshttp.ExpenseAccount, error) {
	accts, err := p.Repo.ListAccounts(ctx, org)
	if err != nil {
		return nil, err
	}
	out := []opshttp.ExpenseAccount{}
	for _, a := range accts {
		if a.Type == "expense" && a.Active {
			out = append(out, opshttp.ExpenseAccount{ID: a.ID, Code: a.Code, Name: a.Name})
		}
	}
	return out, nil
}

func (p Expenses) Record(ctx context.Context, org, user string, e opshttp.ExpenseDraft) (string, error) {
	x, err := p.Repo.SaveExpense(ctx, org, user, "", accounting.Expense{Date: e.Date, VendorName: e.Vendor, CategoryID: e.CategoryID, Amount: e.Amount, GSTRate: e.GSTRate, Notes: e.Notes})
	if err != nil {
		return "", err
	}
	return x.ID, nil
}
