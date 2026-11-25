package opshttp

import (
	"context"
	"time"
)

// The ports below are what the ops modules need from other Adrient modules.
// internal/server/ops_wiring.go implements them with the real repositories
// (Contacts, Invoicing, Calendar, Accounting), so the ops packages stay free of
// import cycles and can be tested with fakes.

// ContactRef is a customer or person from Contacts.
type ContactRef struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Email   string `json:"email"`
	Phone   string `json:"phone"`
	Company string `json:"company"`
}

// ContactDirectory reads and creates Contacts.
type ContactDirectory interface {
	Search(ctx context.Context, org, q string, limit int) ([]ContactRef, error)
	Get(ctx context.Context, org, id string) (ContactRef, error)
	Create(ctx context.Context, org, user string, c ContactRef) (ContactRef, error)
}

// InvLine is one line of an Invoicing document.
type InvLine struct {
	Description string  `json:"description"`
	Qty         float64 `json:"qty"`
	Unit        string  `json:"unit"`
	Price       float64 `json:"price"`
	TaxRate     float64 `json:"tax_rate"`
}

// InvoiceDraft creates an estimate (quote) or invoice in Invoicing.
type InvoiceDraft struct {
	Kind        string // "estimate" or "invoice"
	ContactID   string
	ClientName  string
	ClientEmail string
	Currency    string
	IssueDate   string
	DueDate     string
	Lines       []InvLine
	Notes       string
	SourceRef   string
	// DiscountKind is "percent" or "amount" (default percent).
	DiscountKind  string
	DiscountValue float64
}

// DocRef is the part of an Invoicing document the ops modules show.
type DocRef struct {
	ID      string  `json:"id"`
	Kind    string  `json:"kind"`
	Number  string  `json:"number"`
	Status  string  `json:"status"`
	Total   float64 `json:"total"`
	Paid    float64 `json:"paid"`
	Balance float64 `json:"balance"`
}

// InvoiceWriter creates and reads Invoicing documents.
type InvoiceWriter interface {
	Create(ctx context.Context, org, user string, d InvoiceDraft) (DocRef, error)
	Get(ctx context.Context, org, id string) (DocRef, error)
	ConvertEstimate(ctx context.Context, org, user, estimateID string) (DocRef, error)
}

// CalEvent is a Calendar item written by an ops module.
type CalEvent struct {
	Title       string
	Description string
	Location    string
	Start, End  time.Time
	AllDay      bool
	Color       string
}

// CalendarWriter creates, moves and removes Calendar items owned by a user.
type CalendarWriter interface {
	Create(ctx context.Context, org, owner string, e CalEvent) (string, error)
	Update(ctx context.Context, org, id string, e CalEvent) error
	Delete(ctx context.Context, org, id string) error
}

// ExpenseAccount is an Accounting expense category.
type ExpenseAccount struct {
	ID   string `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
}

// ExpenseDraft is a bill or paid expense posted to Accounting.
type ExpenseDraft struct {
	Date       string
	Vendor     string
	CategoryID string
	Amount     float64 // before tax
	GSTRate    float64
	Notes      string
}

// ExpenseWriter lists expense categories and records expenses in Accounting.
type ExpenseWriter interface {
	Categories(ctx context.Context, org string) ([]ExpenseAccount, error)
	Record(ctx context.Context, org, user string, e ExpenseDraft) (id string, err error)
}
