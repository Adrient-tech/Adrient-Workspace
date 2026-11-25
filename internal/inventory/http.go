package inventory

// JSON API for the extended stock and purchasing features, served inside the
// auth wall at /api/v1/stock/* and /api/v1/purchasing/*. The older
// /api/v1/inventory/* contract (products, adjust, transfer, history, reorder,
// report, locations, suppliers) is unchanged.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"net/mail"
	"strconv"
	"strings"

	"github.com/Adrient-tech/Adrient-Workspace/internal/email"
	"github.com/jackc/pgx/v5/pgconn"
)

// AuthFuncs supplies the caller's identity from the request context.
type AuthFuncs struct {
	UserID    func(r *http.Request) (string, bool)
	OrgID     func(r *http.Request) (string, bool)
	UserEmail func(r *http.Request) string
}

// Mailer sends email (satisfied by *email.Sender).
type Mailer interface {
	Configured() bool
	Send(ctx context.Context, m email.Message) error
}

// HTTPHandler serves the stock and purchasing APIs.
type HTTPHandler struct {
	repo *Repository
	auth AuthFuncs
	mail Mailer
}

// NewHTTPHandler constructs the handler. mail may be nil.
func NewHTTPHandler(repo *Repository, auth AuthFuncs, mail Mailer) *HTTPHandler {
	return &HTTPHandler{repo: repo, auth: auth, mail: mail}
}

const (
	stockMount      = "/api/v1/stock"
	purchasingMount = "/api/v1/purchasing"
)

// Match reports whether the path belongs to this handler.
func (h *HTTPHandler) Match(path string) bool {
	for _, m := range []string{stockMount, purchasingMount} {
		if path == m || strings.HasPrefix(path, m+"/") {
			return true
		}
	}
	return false
}

func (h *HTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	org, ok := h.auth.OrgID(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	user, _ := h.auth.UserID(r)
	var seg []string
	var area string
	switch {
	case strings.HasPrefix(r.URL.Path, stockMount):
		area = "stock"
		seg = splitPath(strings.TrimPrefix(r.URL.Path, stockMount))
	default:
		area = "purchasing"
		seg = splitPath(strings.TrimPrefix(r.URL.Path, purchasingMount))
	}
	if len(seg) == 0 {
		http.NotFound(w, r)
		return
	}
	c := &call{h: h, w: w, r: r, ctx: r.Context(), org: org, user: user, seg: seg}
	if area == "stock" {
		c.stock()
	} else {
		c.purchasing()
	}
	if r.Method != http.MethodGet && !c.failed {
		// Hand finished movements and bills to Accounting; failures are kept for retry.
		go func(ctx context.Context) {
			_, _, _ = h.repo.FlushJournals(ctx, org)
			h.repo.RetryBillJournals(ctx, org)
		}(context.WithoutCancel(r.Context()))
	}
}

func splitPath(p string) []string {
	p = strings.Trim(p, "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}

type call struct {
	h      *HTTPHandler
	w      http.ResponseWriter
	r      *http.Request
	ctx    context.Context
	org    string
	user   string
	seg    []string
	failed bool
}

func (c *call) q(name string) string { return strings.TrimSpace(c.r.URL.Query().Get(name)) }

func (c *call) qInt(name string, def int) int {
	if v, err := strconv.Atoi(c.q(name)); err == nil {
		return v
	}
	return def
}

func (c *call) qBool(name string) bool {
	v := strings.ToLower(c.q(name))
	return v == "1" || v == "true" || v == "yes"
}

func (c *call) decode(v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(c.w, c.r.Body, 4<<20))
	if err := dec.Decode(v); err != nil {
		c.failed = true
		http.Error(c.w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return false
	}
	return true
}

func (c *call) err(err error) {
	c.failed = true
	var pe *pgconn.PgError
	switch {
	case errors.Is(err, ErrNotFound):
		http.Error(c.w, "not found", http.StatusNotFound)
	case errors.Is(err, ErrInvalid):
		http.Error(c.w, strings.TrimPrefix(err.Error(), "invalid: "), http.StatusBadRequest)
	case errors.Is(err, ErrConflict):
		http.Error(c.w, strings.TrimPrefix(err.Error(), "conflict: "), http.StatusConflict)
	case errors.As(err, &pe) && (pe.Code == "22P02" || pe.Code == "23503" || pe.Code == "22007" || pe.Code == "22008" || pe.Code == "23514"):
		http.Error(c.w, "invalid reference or value", http.StatusBadRequest)
	case errors.As(err, &pe) && pe.Code == "23505":
		http.Error(c.w, "that already exists", http.StatusConflict)
	default:
		slog.Error("inventory http", "path", c.r.URL.Path, "err", err)
		http.Error(c.w, "internal error", http.StatusInternalServerError)
	}
}

func (c *call) ok(v any, err error) {
	if err != nil {
		c.err(err)
		return
	}
	c.w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(c.w).Encode(v)
}

func (c *call) list(key string, v any, err error) {
	c.ok(map[string]any{key: v}, err)
}

func (c *call) done(err error) { c.ok(map[string]bool{"ok": true}, err) }

func (c *call) notFound() {
	c.failed = true
	http.NotFound(c.w, c.r)
}

// ------------------------------------------------------------------ stock

func (c *call) stock() {
	m, s, n := c.r.Method, c.seg, len(c.seg)
	repo, ctx, org, user := c.h.repo, c.ctx, c.org, c.user
	get, post, put, del := m == http.MethodGet, m == http.MethodPost, m == http.MethodPut, m == http.MethodDelete
	switch s[0] {
	case "locations":
		switch {
		case n == 1 && get:
			v, err := repo.ListLocationTree(ctx, org)
			c.list("locations", v, err)
		case n == 1 && post, n == 2 && put:
			var in Loc
			if !c.decode(&in) {
				return
			}
			id := ""
			if n == 2 {
				id = s[1]
			}
			c.ok(repo.SaveLocationNode(ctx, org, id, in))
		case n == 2 && del:
			c.done(repo.DeleteLocationNode(ctx, org, s[1]))
		default:
			c.notFound()
		}
	case "products":
		if n == 2 && s[1] == "page" && get {
			c.productsPage()
			return
		}
		if n < 3 {
			c.notFound()
			return
		}
		id := s[1]
		switch {
		case s[2] == "shelf" && put:
			var b struct {
				LocationID string `json:"location_id"`
				ShelfCode  string `json:"shelf_code"`
			}
			if c.decode(&b) {
				c.done(repo.SetShelfCode(ctx, org, id, b.LocationID, b.ShelfCode))
			}
		case s[2] == "settings" && get:
			c.ok(repo.GetProductSettings(ctx, org, id))
		case s[2] == "settings" && put:
			var in ProductSettings
			if c.decode(&in) {
				c.ok(repo.SetProductSettings(ctx, org, id, in))
			}
		case s[2] == "locations" && get:
			v, err := repo.ProductLocations(ctx, org, id)
			c.list("locations", v, err)
		case s[2] == "layers" && get:
			v, err := repo.ProductLayers(ctx, org, id)
			c.list("layers", v, err)
		case s[2] == "lots" && get:
			v, err := repo.ListLots(ctx, org, id, c.qBool("only_stock"), 0, "")
			c.list("lots", v, err)
		default:
			c.notFound()
		}
	case "availability":
		if get {
			v, err := repo.ListAvailability(ctx, org, c.q("q"), c.q("location_id"))
			c.list("products", v, err)
		} else {
			c.notFound()
		}
	case "writeoffs":
		if n == 1 && post {
			var in WriteOffInput
			if c.decode(&in) {
				c.ok(repo.WriteOff(ctx, org, user, in))
			}
		} else {
			c.notFound()
		}
	case "reservations":
		switch {
		case n == 1 && get:
			v, err := repo.ListReservations(ctx, org, c.q("state"), c.q("ref"), c.q("product_id"))
			c.list("reservations", v, err)
		case n == 1 && post:
			var in ReserveInput
			if c.decode(&in) {
				v, err := repo.Reserve(ctx, org, user, in)
				c.list("reservations", v, err)
			}
		case n == 2 && s[1] == "release" && post:
			var b struct {
				Ref string `json:"ref"`
			}
			if c.decode(&b) {
				k, err := repo.ReleaseReservations(ctx, org, b.Ref)
				c.ok(map[string]int64{"released": k}, err)
			}
		default:
			c.notFound()
		}
	case "transfers":
		switch {
		case n == 1 && get:
			v, err := repo.ListTransfers(ctx, org, c.q("state"))
			c.list("transfers", v, err)
		case n == 1 && post:
			var in TransferInput
			if c.decode(&in) {
				c.ok(repo.CreateTransfer(ctx, org, user, in))
			}
		case n == 2 && get:
			c.ok(repo.GetTransfer(ctx, org, s[1]))
		case n == 3 && s[2] == "ship" && post:
			c.ok(repo.ShipTransfer(ctx, org, user, s[1]))
		case n == 3 && s[2] == "receive" && post:
			var b struct {
				Lines []ReceiveLine `json:"lines"`
			}
			if c.r.ContentLength != 0 && !c.decode(&b) {
				return
			}
			c.ok(repo.ReceiveTransfer(ctx, org, user, s[1], b.Lines))
		case n == 3 && s[2] == "cancel" && post:
			c.ok(repo.CancelTransfer(ctx, org, user, s[1]))
		default:
			c.notFound()
		}
	case "valuation":
		if get {
			c.ok(repo.ValuationAsOf(ctx, org, c.q("as_of"), c.q("category")))
		} else {
			c.notFound()
		}
	case "ledger":
		if get {
			v, err := repo.ListLedger(ctx, org, c.q("from"), c.q("to"), c.q("reason"), c.q("kind"), c.qInt("limit", 500))
			c.list("rows", v, err)
		} else {
			c.notFound()
		}
	case "accounting":
		switch {
		case n == 2 && s[1] == "status" && get:
			p, f, err := repo.PendingJournalCount(ctx, org)
			c.ok(map[string]any{"connected": currentPoster() != nil, "pending": p, "failed": f}, err)
		case n == 2 && s[1] == "sync" && post:
			posted, failed, err := repo.FlushJournals(ctx, org)
			bills := repo.RetryBillJournals(ctx, org)
			c.ok(map[string]any{"posted": posted, "failed": failed, "bills_retried": bills, "connected": currentPoster() != nil}, err)
		default:
			c.notFound()
		}
	case "lots":
		switch {
		case n == 1 && get:
			v, err := repo.ListLots(ctx, org, c.q("product_id"), c.qBool("only_stock"), c.qInt("expiring_days", 0), c.q("q"))
			c.list("lots", v, err)
		case n == 2 && get:
			c.ok(repo.GetLot(ctx, org, s[1]))
		case n == 2 && put:
			var b struct {
				Expiry   string `json:"expiry"`
				MfgDate  string `json:"mfg_date"`
				Note     string `json:"note"`
				Recalled bool   `json:"recalled"`
			}
			if c.decode(&b) {
				c.ok(repo.UpdateLot(ctx, org, s[1], b.Expiry, b.MfgDate, b.Note, b.Recalled))
			}
		case n == 3 && s[2] == "recall" && get:
			c.ok(repo.LotRecall(ctx, org, s[1]))
		default:
			c.notFound()
		}
	case "serials":
		switch {
		case n == 1 && get:
			v, err := repo.ListSerials(ctx, org, c.q("product_id"), c.q("state"), c.q("q"))
			c.list("serials", v, err)
		case n == 3 && s[2] == "history" && get:
			v, err := repo.SerialHistory(ctx, org, s[1])
			c.list("moves", v, err)
		default:
			c.notFound()
		}
	case "lookup":
		if get {
			c.ok(c.lookup(c.q("code")))
		} else {
			c.notFound()
		}
	case "boms":
		switch {
		case n == 1 && get:
			v, err := repo.ListBOMs(ctx, org)
			c.list("boms", v, err)
		case n == 1 && post:
			var in BOMInput
			if c.decode(&in) {
				c.ok(repo.SaveBOM(ctx, org, in))
			}
		case n == 2 && get:
			c.ok(repo.GetBOM(ctx, org, s[1]))
		case n == 2 && del:
			c.done(repo.DeleteBOM(ctx, org, s[1]))
		default:
			c.notFound()
		}
	case "assemblies":
		switch {
		case n == 1 && get:
			v, err := repo.ListAssemblies(ctx, org)
			c.list("assemblies", v, err)
		case n == 1 && post:
			var in AssembleInput
			if c.decode(&in) {
				c.ok(repo.Assemble(ctx, org, user, in))
			}
		default:
			c.notFound()
		}
	case "counts":
		switch {
		case n == 1 && get:
			v, err := repo.ListCounts(ctx, org, "")
			c.list("counts", v, err)
		case n == 1 && post:
			var in CountInput
			if c.decode(&in) {
				c.ok(repo.StartCount(ctx, org, user, in))
			}
		case n == 2 && get:
			c.ok(repo.GetCount(ctx, org, s[1]))
		case n == 3 && s[2] == "lines" && put:
			var b struct {
				Entries []CountEntry `json:"entries"`
			}
			if c.decode(&b) {
				c.ok(repo.RecordCount(ctx, org, s[1], b.Entries))
			}
		case n == 3 && s[2] == "submit" && post:
			c.ok(repo.SubmitCount(ctx, org, s[1]))
		case n == 3 && s[2] == "approve" && post:
			var b struct {
				Reason string `json:"reason"`
			}
			if !c.decode(&b) {
				return
			}
			c.ok(repo.ApproveCount(ctx, org, user, s[1], b.Reason))
		case n == 3 && s[2] == "cancel" && post:
			c.ok(repo.CancelCount(ctx, org, s[1]))
		default:
			c.notFound()
		}
	case "reports":
		if n != 2 || !get {
			c.notFound()
			return
		}
		switch s[1] {
		case "abc":
			v, err := repo.ABCAnalysis(ctx, org, c.qInt("days", 365))
			c.list("rows", v, err)
		case "cover":
			v, err := repo.DaysOfCoverReport(ctx, org, c.qInt("weeks", 8))
			c.list("rows", v, err)
		case "dead":
			v, err := repo.DeadStock(ctx, org, c.qInt("days", 90))
			c.list("rows", v, err)
		case "shrinkage":
			v, err := repo.Shrinkage(ctx, org, c.qInt("days", 30))
			c.ok(v, err)
		case "margin":
			v, err := repo.MarginReport(ctx, org)
			c.list("rows", v, err)
		default:
			c.notFound()
		}
	default:
		c.notFound()
	}
}

// lookup resolves a scanned code: product barcode/SKU, lot number or serial.
func (c *call) lookup(code string) (any, error) {
	if code == "" {
		return nil, invalid("scan or type a code")
	}
	repo := c.h.repo
	ps, err := repo.ListProducts(c.ctx, c.org, ProductFilter{Barcode: code, Flat: true})
	if err != nil {
		return nil, err
	}
	if len(ps) > 0 {
		p := ps[0]
		return map[string]any{"kind": "product", "product_id": p.ID, "name": p.Name, "sku": p.SKU, "barcode": p.Barcode, "unit": p.Unit, "cost": p.Cost, "price": p.Price, "stock": p.TotalStock}, nil
	}
	rows, err := c.h.repo.pool.Query(c.ctx,
		`SELECT l.id::text, l.product_id::text, p.name, p.sku FROM adrient.inv_lots l JOIN adrient.inv_products p ON p.id=l.product_id
		 WHERE l.org_id=$1 AND lower(l.lot_no)=lower($2) LIMIT 5`, c.org, code)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if rows.Next() {
		var lot, pid, name, sku string
		if err := rows.Scan(&lot, &pid, &name, &sku); err != nil {
			return nil, err
		}
		return map[string]any{"kind": "lot", "lot_id": lot, "product_id": pid, "name": name, "sku": sku}, nil
	}
	rows.Close()
	var sid, pid, name, sku, state string
	err = c.h.repo.pool.QueryRow(c.ctx,
		`SELECT s.id::text, s.product_id::text, p.name, p.sku, s.state FROM adrient.inv_serials s JOIN adrient.inv_products p ON p.id=s.product_id
		 WHERE s.org_id=$1 AND lower(s.serial_no)=lower($2) LIMIT 1`, c.org, code).Scan(&sid, &pid, &name, &sku, &state)
	if err == nil {
		return map[string]any{"kind": "serial", "serial_id": sid, "product_id": pid, "name": name, "sku": sku, "state": state}, nil
	}
	return nil, ErrNotFound
}

// -------------------------------------------------------------- purchasing

func (c *call) purchasing() {
	m, s, n := c.r.Method, c.seg, len(c.seg)
	repo, ctx, org, user := c.h.repo, c.ctx, c.org, c.user
	get, post, put, del := m == http.MethodGet, m == http.MethodPost, m == http.MethodPut, m == http.MethodDelete
	switch s[0] {
	case "settings":
		switch {
		case get:
			c.ok(repo.GetPurchaseSettings(ctx, org))
		case put:
			var in PurchaseSettings
			if c.decode(&in) {
				c.ok(repo.SavePurchaseSettings(ctx, org, in))
			}
		default:
			c.notFound()
		}
	case "prices":
		switch {
		case n == 1 && get:
			v, err := repo.ListSupplierPrices(ctx, org, c.q("supplier_id"), c.q("product_id"))
			c.list("prices", v, err)
		case n == 1 && post, n == 2 && put:
			var in SupplierPrice
			if !c.decode(&in) {
				return
			}
			id := ""
			if n == 2 {
				id = s[1]
			}
			c.ok(repo.SaveSupplierPrice(ctx, org, id, in))
		case n == 2 && del:
			c.done(repo.DeleteSupplierPrice(ctx, org, s[1]))
		default:
			c.notFound()
		}
	case "requests":
		switch {
		case n == 1 && get:
			v, err := repo.ListRequests(ctx, org, "")
			c.list("requests", v, err)
		case n == 1 && post:
			var in RequestInput
			if c.decode(&in) {
				c.ok(repo.CreateRequest(ctx, org, user, in))
			}
		case n == 2 && get:
			c.ok(repo.GetRequest(ctx, org, s[1]))
		case n == 3 && post && (s[2] == "submit" || s[2] == "approve" || s[2] == "reject"):
			c.ok(repo.DecideRequest(ctx, org, user, s[1], s[2]))
		case n == 3 && post && s[2] == "convert":
			v, err := repo.ConvertRequest(ctx, org, user, s[1])
			c.list("orders", v, err)
		default:
			c.notFound()
		}
	case "orders":
		c.orders()
	case "receipts":
		switch {
		case n == 1 && get:
			v, err := repo.ListGRNs(ctx, org, "", c.q("po_id"))
			c.list("receipts", v, err)
		case n == 1 && post:
			var in GRNInput
			if c.decode(&in) {
				c.ok(repo.CreateGRN(ctx, org, user, in))
			}
		case n == 2 && get:
			c.ok(repo.GetGRN(ctx, org, s[1]))
		default:
			c.notFound()
		}
	case "bills":
		switch {
		case n == 1 && get:
			v, err := repo.ListBills(ctx, org, "", c.q("state"), c.q("supplier_id"))
			c.list("bills", v, err)
		case n == 1 && post:
			var in BillInput
			if c.decode(&in) {
				c.ok(repo.CreateBill(ctx, org, user, in))
			}
		case n == 2 && get:
			c.ok(repo.GetBill(ctx, org, s[1]))
		case n == 2 && put:
			var in BillInput
			if c.decode(&in) {
				c.ok(repo.UpdateBill(ctx, org, s[1], in))
			}
		case n == 2 && del:
			c.done(repo.DeleteBill(ctx, org, s[1]))
		case n == 3 && s[2] == "post" && post:
			var b struct {
				Override string `json:"override_reason"`
			}
			if c.r.ContentLength != 0 && !c.decode(&b) {
				return
			}
			c.ok(repo.PostBill(ctx, org, user, s[1], b.Override))
		default:
			c.notFound()
		}
	case "returns":
		switch {
		case n == 1 && get:
			v, err := repo.ListReturns(ctx, org, "")
			c.list("returns", v, err)
		case n == 1 && post:
			var in ReturnInput
			if c.decode(&in) {
				c.ok(repo.CreateReturn(ctx, org, user, in))
			}
		case n == 2 && get:
			c.ok(repo.GetReturn(ctx, org, s[1]))
		default:
			c.notFound()
		}
	case "rules":
		switch {
		case n == 1 && get:
			v, err := repo.ListReorderRules(ctx, org)
			c.list("rules", v, err)
		case n == 1 && post, n == 2 && put:
			var in ReorderRule
			if c.decode(&in) {
				c.ok(repo.SaveReorderRule(ctx, org, in))
			}
		case n == 2 && del:
			c.done(repo.DeleteReorderRule(ctx, org, s[1]))
		default:
			c.notFound()
		}
	case "reorder":
		if n == 2 && s[1] == "run" && post {
			var b struct {
				Create bool `json:"create"`
			}
			if c.r.ContentLength != 0 && !c.decode(&b) {
				return
			}
			c.ok(repo.RunReorder(ctx, org, user, b.Create))
		} else {
			c.notFound()
		}
	case "performance":
		if get {
			v, err := repo.SupplierPerformance(ctx, org, c.q("from"), c.q("to"))
			c.list("suppliers", v, err)
		} else {
			c.notFound()
		}
	default:
		c.notFound()
	}
}

func (c *call) orders() {
	m, s, n := c.r.Method, c.seg, len(c.seg)
	repo, ctx, org, user := c.h.repo, c.ctx, c.org, c.user
	get, post, put := m == http.MethodGet, m == http.MethodPost, m == http.MethodPut
	switch {
	case n == 1 && get:
		v, err := repo.ListPOs(ctx, org, c.q("state"), c.q("supplier_id"), c.q("q"))
		c.list("orders", v, err)
	case n == 1 && post:
		var in POInput
		if c.decode(&in) {
			c.ok(repo.CreatePO(ctx, org, user, in))
		}
	case n == 2 && get:
		c.ok(repo.GetPO(ctx, org, s[1]))
	case n == 2 && put:
		var in POInput
		if c.decode(&in) {
			c.ok(repo.UpdatePO(ctx, org, s[1], in))
		}
	case n == 3 && s[2] == "approve" && post:
		c.ok(repo.ApprovePO(ctx, org, user, s[1]))
	case n == 3 && s[2] == "cancel" && post:
		c.ok(repo.CancelPO(ctx, org, s[1]))
	case n == 3 && s[2] == "close" && post:
		c.ok(repo.ClosePO(ctx, org, s[1]))
	case n == 3 && s[2] == "acknowledge" && post:
		var in AckInput
		if c.decode(&in) {
			c.ok(repo.AcknowledgePO(ctx, org, s[1], in))
		}
	case n == 3 && s[2] == "mark-sent" && post:
		var b struct {
			To string `json:"to"`
		}
		if c.r.ContentLength != 0 && !c.decode(&b) {
			return
		}
		c.ok(repo.MarkPOSent(ctx, org, s[1], b.To))
	case n == 3 && s[2] == "send" && post:
		c.sendPO(s[1])
	case n == 3 && s[2] == "pdf" && get:
		po, err := repo.GetPO(ctx, org, s[1])
		if err != nil {
			c.err(err)
			return
		}
		st, _ := repo.GetPurchaseSettings(ctx, org)
		c.w.Header().Set("Content-Type", "application/pdf")
		c.w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", po.Number+".pdf"))
		_, _ = c.w.Write(POPDF(po, st))
	case n == 3 && s[2] == "bill-draft" && get:
		c.ok(repo.BillDraftFromPO(ctx, org, s[1]))
	case n == 3 && s[2] == "receipts" && get:
		v, err := repo.ListGRNs(ctx, org, "", s[1])
		c.list("receipts", v, err)
	default:
		c.notFound()
	}
}

// sendPO emails the purchase order PDF to the supplier and marks it sent.
func (c *call) sendPO(id string) {
	var b struct {
		To       string `json:"to"`
		Subject  string `json:"subject"`
		Message  string `json:"message"`
		MarkOnly bool   `json:"mark_only"`
	}
	if c.r.ContentLength != 0 && !c.decode(&b) {
		return
	}
	repo := c.h.repo
	po, err := repo.GetPO(c.ctx, c.org, id)
	if err != nil {
		c.err(err)
		return
	}
	if po.State != POapproved && po.State != POsent {
		c.err(invalid("approve the order before sending it (it is %s)", po.State))
		return
	}
	to := strings.TrimSpace(b.To)
	if to == "" {
		to = po.SupplierEmail
	}
	if b.MarkOnly {
		c.ok(repo.MarkPOSent(c.ctx, c.org, id, to))
		return
	}
	if to == "" {
		c.err(invalid("the supplier has no email address; type one or use mark as sent"))
		return
	}
	if _, perr := mail.ParseAddress(to); perr != nil {
		c.err(invalid("that email address is not valid"))
		return
	}
	if c.h.mail == nil || !c.h.mail.Configured() {
		c.err(invalid("email is not set up on this server. Download the PDF and send it yourself, then use mark as sent"))
		return
	}
	st, _ := repo.GetPurchaseSettings(c.ctx, c.org)
	pdf := POPDF(po, st)
	subject := b.Subject
	if strings.TrimSpace(subject) == "" {
		subject = "Purchase order " + po.Number
		if st.CompanyName != "" {
			subject += " from " + st.CompanyName
		}
	}
	msg := b.Message
	if strings.TrimSpace(msg) == "" {
		msg = "Hello,\n\nPlease find purchase order " + po.Number + " attached. Please confirm receipt and your delivery date by replying to this email."
		if po.ExpectedDate != "" {
			msg += "\nWe expect delivery by " + po.ExpectedDate + "."
		}
	}
	m := email.Message{To: []string{to}, Subject: subject, Text: msg,
		HTML:        "<p>" + strings.ReplaceAll(html.EscapeString(msg), "\n", "<br>") + "</p>",
		Attachments: []email.Attachment{{Filename: po.Number + ".pdf", ContentType: "application/pdf", Data: pdf}}}
	if c.h.auth.UserEmail != nil {
		m.ReplyTo = c.h.auth.UserEmail(c.r)
	}
	if err := c.h.mail.Send(c.ctx, m); err != nil {
		slog.Error("purchase order email failed", "po", po.Number, "err", err)
		c.err(invalid("the email could not be sent: %v", err))
		return
	}
	c.ok(repo.MarkPOSent(c.ctx, c.org, id, to))
}
