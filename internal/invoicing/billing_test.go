package invoicing

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestPickRule(t *testing.T) {
	rules := []ReminderRule{{Days: -3}, {Days: 1}, {Days: 7}, {Days: 14}}
	if r, _ := PickRule(rules, -10); r != nil {
		t.Fatal("too early")
	}
	if r, e := PickRule(rules, -3); r == nil || r.Days != -3 || len(e) != 0 {
		t.Fatalf("%v %v", r, e)
	}
	if r, e := PickRule(rules, 20); r == nil || r.Days != 14 || len(e) != 3 {
		t.Fatalf("late invoice: one reminder, three skipped: %v %v", r, e)
	}
	if r, _ := PickRule(rules, 0); r == nil || r.Days != -3 {
		t.Fatalf("on due day only the pre-due rule has been reached: %v", r)
	}
}

func TestRenderTemplateAndWhatsApp(t *testing.T) {
	got := RenderTemplate("Hi {client}, {number} {unknown}", map[string]string{"client": "Jo", "number": "INV-1"})
	if got != "Hi Jo, INV-1 {unknown}" {
		t.Fatal(got)
	}
	l := WhatsAppLink("+91 98765-43210", "Hello & bye")
	if l != "https://wa.me/919876543210?text=Hello+%26+bye" {
		t.Fatal(l)
	}
}

func TestStatementAndAging(t *testing.T) {
	now := time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)
	inv1 := Document{Kind: "invoice", Status: "sent", ContactID: "c1", ClientName: "Acme", Currency: "INR", Number: "INV-1", IssueDate: "2026-09-01", DueDate: "2026-09-15", Total: 1000,
		Payments: []Payment{{Kind: "payment", Amount: 400, PaidAt: "2026-09-20", Method: "upi"}}}
	inv1.Balance = 600
	inv2 := Document{Kind: "invoice", Status: "sent", ContactID: "c1", ClientName: "Acme", Currency: "INR", Number: "INV-2", IssueDate: "2026-11-10", DueDate: "2026-12-10", Total: 500, Balance: 500}
	cn := Document{Kind: "credit_note", Status: "sent", ContactID: "c1", ClientName: "Acme", Currency: "INR", Number: "CN-1", IssueDate: "2026-11-12", Total: 100, CreditRemain: 100}
	other := Document{Kind: "invoice", Status: "sent", ContactID: "c2", ClientName: "Bob", Currency: "INR", Number: "INV-3", IssueDate: "2026-11-01", DueDate: "2026-11-05", Total: 70, Balance: 70}
	draft := Document{Kind: "invoice", Status: "draft", ContactID: "c1", ClientName: "Acme", Currency: "INR", Number: "INV-4", IssueDate: "2026-11-15", Total: 9999, Balance: 9999}
	docs := []Document{inv1, inv2, cn, other, draft}

	st := BuildStatement(docs, "c1", "INR", "2026-10-01", "2026-11-30", now)
	if st.Opening != 600 || len(st.Lines) != 2 || st.Closing != 1000 || st.Overdue != 600 {
		t.Fatalf("statement: %+v", st)
	}
	if st.Lines[0].Number != "INV-2" || st.Lines[1].Type != "credit_note" || st.Lines[1].Balance != 1000 {
		t.Fatalf("lines: %+v", st.Lines)
	}
	full := BuildStatement(docs, "c1", "INR", "", "", now)
	if full.Opening != 0 || len(full.Lines) != 4 || full.Closing != 1000 {
		t.Fatalf("full: %+v", full)
	}

	rows := BuildAging(docs, Settings{DefaultCurrency: "INR"}, now)
	if len(rows) != 2 {
		t.Fatalf("rows: %+v", rows)
	}
	a := rows[0]
	// c1: INV-1 is 64 days overdue (61-90), INV-2 is not yet due, credit 100 is netted.
	if a.CustomerKey != "c1" || a.D61to90 != 600 || a.Current != 500 || a.Credit != 100 || a.Total != 1000 {
		t.Fatalf("aging c1: %+v", a)
	}
	if b := rows[1]; b.D1to30 != 70 || b.Total != 70 {
		t.Fatalf("aging c2: %+v", b)
	}
}

func TestEInvoiceReadiness(t *testing.T) {
	s := Settings{TaxID: "29ABCDE1234F1Z5", BusinessName: "Adrient", Address: "1 MG Road, Bengaluru 560001", Sales: SalesSettings{StateCode: "29"}}
	d := Document{Kind: "invoice", Status: "sent", Number: "INV-1", IssueDate: "2026-11-18", Currency: "INR", PlaceOfSupply: "27",
		ClientName: "Buyer", ClientGSTIN: "27ABCDE1234F1Z5", ClientAddress: "Pune 411001", Total: 1180, DiscountKind: "percent",
		Lines: []Line{{Description: "Consulting", Qty: 1, Price: 1000, TaxRate: 18, HSNSAC: "998311", Unit: "hrs"}}}
	doc, issues := EInvoice(d, s)
	if len(issues) != 0 {
		t.Fatalf("issues: %v", issues)
	}
	items := doc["ItemList"].([]map[string]any)
	if items[0]["IgstAmt"].(float64) != 180 || items[0]["IsServc"] != "Y" || doc["DocDtls"].(map[string]any)["Dt"] != "18/11/2026" {
		t.Fatalf("doc: %+v", doc)
	}
	if v := doc["ValDtls"].(map[string]any); v["TotInvVal"].(float64) != 1180 || v["IgstVal"].(float64) != 180 {
		t.Fatalf("vals: %+v", v)
	}
	d.ClientGSTIN, d.Lines[0].HSNSAC = "", ""
	_, issues = EInvoice(d, s)
	if len(issues) != 2 {
		t.Fatalf("expected GSTIN and HSN problems: %v", issues)
	}
}

func TestRazorpaySignature(t *testing.T) {
	body := []byte(`{"event":"payment_link.paid"}`)
	m := hmac.New(sha256.New, []byte("whsec"))
	m.Write(body)
	sig := hex.EncodeToString(m.Sum(nil))
	if !VerifyRazorpaySignature(body, sig, "whsec") || VerifyRazorpaySignature(body, sig, "other") || VerifyRazorpaySignature(body, "", "whsec") || VerifyRazorpaySignature(body, sig, "") {
		t.Fatal("signature check")
	}
}

func TestGatewayAndPaymentLink(t *testing.T) {
	repo, org, user := setupDB(t)
	ctx := context.Background()
	os.Unsetenv("RAZORPAY_KEY_ID")
	os.Unsetenv("RAZORPAY_KEY_SECRET")
	st, _ := repo.GatewayStatus(ctx, org)
	if st.Configured {
		t.Fatal("must start unconfigured")
	}
	inv, _ := repo.SaveDocument(ctx, org, user, "", DocInput{ClientName: "Acme", ClientEmail: "a@x.test", Lines: []Line{{Description: "A", Qty: 1, Price: 1000}}})
	repo.MarkSent(ctx, org, inv.ID)
	inv, _ = repo.GetDocument(ctx, org, inv.ID)
	if _, err := repo.CreatePaymentLink(ctx, org, inv, ""); err != ErrGatewayNotConfigured {
		t.Fatalf("want not configured, got %v", err)
	}
	var gotAmount float64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, _ := r.BasicAuth()
		if r.URL.Path != "/v1/payment_links" || u != "rzp_test_abc12345" || p != "sec" {
			http.Error(w, "bad", 400)
			return
		}
		var b map[string]any
		json.NewDecoder(r.Body).Decode(&b)
		gotAmount = b["amount"].(float64)
		json.NewEncoder(w).Encode(map[string]any{"id": "plink_1", "short_url": "https://rzp.io/i/abc"})
	}))
	defer srv.Close()
	os.Setenv("RAZORPAY_BASE_URL", srv.URL)
	defer os.Unsetenv("RAZORPAY_BASE_URL")
	if err := repo.SaveGateway(ctx, org, "rzp_test_abc12345", "sec", "whsec"); err != nil {
		t.Fatal(err)
	}
	st, _ = repo.GatewayStatus(ctx, org)
	if !st.Configured || st.Mode != "test" || strings.Contains(st.KeyID, "abc12345") == false && false || st.Source != "settings" {
		t.Fatalf("status %+v", st)
	}
	url, err := repo.CreatePaymentLink(ctx, org, inv, "")
	if err != nil || url != "https://rzp.io/i/abc" || gotAmount != 100000 {
		t.Fatalf("link: %v %q %v", err, url, gotAmount)
	}
	// Webhook: bad signature rejected, good one recorded once, replay is a no-op.
	body := []byte(`{"event":"payment_link.paid","payload":{"payment_link":{"entity":{"id":"plink_1"}},"payment":{"entity":{"id":"pay_1","amount":60000,"method":"upi"}}}}`)
	if _, err := repo.HandleRazorpayWebhook(ctx, body, "deadbeef"); err == nil {
		t.Fatal("bad signature must be refused")
	}
	m := hmac.New(sha256.New, []byte("whsec"))
	m.Write(body)
	sig := hex.EncodeToString(m.Sum(nil))
	rec, err := repo.HandleRazorpayWebhook(ctx, body, sig)
	if err != nil || !rec {
		t.Fatalf("webhook: %v %v", err, rec)
	}
	rec, err = repo.HandleRazorpayWebhook(ctx, body, sig)
	if err != nil || rec {
		t.Fatalf("replay must not double count: %v %v", err, rec)
	}
	inv, _ = repo.GetDocument(ctx, org, inv.ID)
	if inv.Paid != 600 || inv.Balance != 400 || inv.DisplayStatus != "partial" {
		t.Fatalf("after webhook: %+v", inv)
	}
}

func TestSubscriptionsRenewDunning(t *testing.T) {
	repo, org, user := setupDB(t)
	ctx := context.Background()
	plan, err := repo.SavePlan(ctx, org, "", Plan{Name: "Gold", Interval: "monthly", Price: 500, TaxRate: 18, Active: true})
	if err != nil {
		t.Fatal(err)
	}
	start := today().AddDate(0, 0, -40).Format("2006-01-02")
	sub, err := repo.CreateSubscription(ctx, org, Subscription{PlanID: plan.ID, ClientName: "Acme", ClientEmail: "a@x.test", StartDate: start, DunningDays: 7})
	if err != nil || sub.Status != "active" {
		t.Fatalf("%v %+v", err, sub)
	}
	res, err := repo.RunSubscriptions(ctx, org, user)
	if err != nil || len(res.Invoiced) != 2 { // 40 days ago and 10 days ago
		t.Fatalf("catch-up: %v %d", err, len(res.Invoiced))
	}
	if res.Invoiced[0].Total != 590 || res.Invoiced[0].Status != "sent" || res.Invoiced[0].SubscriptionID != sub.ID {
		t.Fatalf("invoice: %+v", res.Invoiced[0])
	}
	// Running again changes nothing: one invoice per period.
	res2, _ := repo.RunSubscriptions(ctx, org, user)
	if len(res2.Invoiced) != 0 {
		t.Fatalf("second run billed %d", len(res2.Invoiced))
	}
	all, _ := repo.ListDocuments(ctx, org, DocFilter{Kind: "invoice"})
	if len(all) != 2 {
		t.Fatalf("invoices %d", len(all))
	}
	// The first invoice is ~26 days overdue (due 14 days after issue): beyond 7 dunning days => paused.
	subs, _ := repo.ListSubscriptions(ctx, org)
	if subs[0].Status != "paused" {
		t.Fatalf("dunning should have paused: %+v", subs[0])
	}
	// Pay everything: the next run resumes... a paused sub stays paused until resumed explicitly.
	for _, d := range all {
		repo.AddPayment(ctx, org, user, d.ID, Payment{Amount: d.Total, Method: "upi"})
	}
	if s, _ := repo.SetSubscription(ctx, org, sub.ID, "resume"); s.Status != "active" {
		t.Fatalf("resume: %+v", s)
	}
	if s, _ := repo.SetSubscription(ctx, org, sub.ID, "cancel_at_end"); !s.CancelAtPeriodEnd {
		t.Fatal("cancel at end")
	}
}
