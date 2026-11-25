# Accounting + Expenses

## Data sharing (auto-posting)

"Sync from apps" (button, and automatically when the module opens) reads the other modules' repositories on the server and posts balanced journals. Each source row owns one journal keyed by (source, source id), so a sync never double-posts. A changed source (hash differs) rewrites its journal in place; a source that disappears (draft, trashed, deleted payment) voids it.

| Source | Journal |
| --- | --- |
| POS completed/refunded sale (`pos_sale`) | Dr Cash / Bank / Store credit per payment, Cr Sales (net of GST), Cr GST Output |
| POS refund (`pos_refund`) | Dr Sales Returns, Dr GST Output (pro rata), Cr Cash / Bank / Store credit |
| Invoice, not draft/trashed (`invoice`) | Dr Accounts Receivable, Cr Sales, Cr GST Output; due date kept for aging |
| Invoice payment / refund / write-off (`invoice_payment`) | Dr Cash/Bank Cr AR; reverse; Dr Bad Debts Cr AR |

GST is split CGST/SGST (intrastate, the default) or IGST (setting). Customers and vendors are Contacts ids with a name snapshot. Posting into a closed period is skipped and counted.

## Posting API for other modules (Go)

`accounting.Repository` exposes an idempotent posting API; call it from HR, Inventory/Purchasing, Sales/POS.

```go
j, created, err := repo.PostSourceJournal(ctx, orgID, userID, accounting.SourcePost{
    Source:   accounting.SourcePayroll, // payroll | purchase_bill | stock_cogs | stock_adjustment | bill | bill_payment | vendor_credit | credit_memo | deposit | depreciation | recurring | revaluation
    SourceID: runID,                    // required; (org, source, source_id) owns exactly one journal
    Date:     "2026-11-30",
    Memo:     "Payroll Nov 2026",
    Lines: []accounting.PostLine{
        {Account: "salaries_expense", Debit: 100000},
        {Account: "payroll_payable", Credit: 100000},
    },
})
ok, err := repo.VoidSourceJournal(ctx, orgID, userID, accounting.SourcePayroll, runID)
```

- Idempotent: the same source id with identical content is a no-op (`created=false`); changed content rewrites that journal in place; void keeps it for audit and excludes it from reports.
- `Account` is a system key, an account code or an account id. System keys: `cash bank ar ap inventory cogs sales sales_returns bad_debt round_off capital opening gst_in_cgst gst_in_sgst gst_in_igst gst_out_cgst gst_out_sgst gst_out_igst tds_receivable tds_payable undeposited_funds accum_depreciation depreciation_expense payroll_payable payroll_deductions salaries_expense employer_contrib stock_adjustment purchases fx_gain_loss fx_unrealised write_off`. Missing system accounts are created on first use.
- Lines must balance; closed periods return `ErrLocked`; unknown source/account returns `ErrInvalid`.
- `Currency` (and optional `Rate`) on `SourcePost` means the line amounts are in that foreign currency; they are converted to base at the rate on the date (rate table) and the foreign amounts are kept on the lines.
- `Class` on a line tags a cost center / class for P&L by class.

## Features

Done:
- Chart of accounts (India default, ~40 accounts), add/edit/inactivate, per-account register with running balance.
- Journals: manual journal entries (must balance), list/search/filter by source, view, edit, delete (manual only).
- Closing date (lock period) enforced on every write path, including sync.
- Expenses: category, vendor (Contact), GST rate, paid from bank/cash or left as a bill (Accounts Payable), pay bill, receipt photos/PDFs uploaded to Drive (no OCR).
- Expense reports: draft, submit, approve/reject (with note), reimburse from a bank/cash account. Out-of-pocket claims post to Employee Reimbursements Payable on approval.
- Banking: CSV statement import (header auto-detect, debit/credit or signed amount, Indian date formats, `Dr`/`Cr`), de-duplication on re-import, review lines (add to books with a category, exclude, undo), bank rules (text contains, in/out, optional bank account, priority, auto-add).
- Reconcile: tick cleared lines against a statement ending balance, must match to finish; history; undo latest; reconciled entries are protected from edits.
- Reports: Trial Balance, Profit and Loss, Balance Sheet (current-year and retained earnings), Cash Flow (direct, classified operating/investing/financing, ties to cash+bank), Aged Receivables, Aged Payables (oldest-first allocation per contact), GST summary (output vs input, net payable); CSV export and print.
- Search (journals, accounts, expenses) and audit log.

## Who posts here
| Source key | Poster | When |
| --- | --- | --- |
| `payroll` | HR (`hr.AccountingAdapter`) | locking a payroll run; idempotent per run |
| `purchase_bill` | Inventory purchasing | posting a supplier bill: Dr Inventory and GST input, Cr Accounts Payable (goods enter the ledger at the bill, not at receipt) |
| `stock_cogs`, `stock_adjustment` | Inventory (`stockPoster`, flushed every 30 s) | sales, deliveries, counts |
| `pos_sale`, `invoice`, `invoice_payment` | the sync described above | on open and "Sync from apps" |
Expense reports: approved out-of-pocket claims can be added to a draft payslip and are marked reimbursed when the run locks.

**Stock value vs ledger:** `GET /api/v1/bridge/stock/reconciliation` returns valuation total, Inventory account balance, goods received but not billed yet, and any other difference. Received-but-unbilled goods are the normal reason the valuation is larger than the ledger.

## More features
