package inventory

import (
	"context"
	"errors"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	adrientv1 "github.com/Adrient-tech/Adrient-Workspace/gen/go/adrient/v1"
	"github.com/Adrient-tech/Adrient-Workspace/internal/auth"
)

// Service implements adrientv1.InventoryServiceServer over a Repository.
type Service struct {
	repo *Repository
}

// NewService constructs a Service.
func NewService(repo *Repository) *Service { return &Service{repo: repo} }

func callerCtx(ctx context.Context) (orgID, userID string, err error) {
	u, ok := auth.UserFromContext(ctx)
	if !ok {
		return "", "", status.Error(codes.Unauthenticated, "no session")
	}
	o, ok := auth.OrgFromContext(ctx)
	if !ok {
		return "", "", status.Error(codes.Internal, "missing org context")
	}
	return o.ID, u.ID, nil
}

func rpcErr(err error, what string) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrNotFound):
		return status.Errorf(codes.NotFound, "%s not found", what)
	case errors.Is(err, ErrInvalid):
		return status.Error(codes.InvalidArgument, strings.TrimPrefix(err.Error(), "invalid: "))
	case errors.Is(err, ErrConflict):
		return status.Error(codes.AlreadyExists, strings.TrimPrefix(err.Error(), "conflict: "))
	}
	return status.Errorf(codes.Internal, "%s: %v", what, err)
}

func ts(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func locToProto(l Location) *adrientv1.InvLocation {
	return &adrientv1.InvLocation{Id: l.ID, Name: l.Name, Code: l.Code, IsDefault: l.IsDefault, CreatedAt: ts(l.CreatedAt)}
}

func supplierToProto(s Supplier) *adrientv1.InvSupplier {
	return &adrientv1.InvSupplier{Id: s.ID, Name: s.Name, ContactId: s.ContactID, Email: s.Email,
		Phone: s.Phone, Notes: s.Notes, ProductCount: s.ProductCount, CreatedAt: ts(s.CreatedAt)}
}

func productToProto(p Product) *adrientv1.InvProduct {
	out := &adrientv1.InvProduct{
		Id: p.ID, ParentId: p.ParentID, VariantLabel: p.VariantLabel, Name: p.Name, Sku: p.SKU,
		Barcode: p.Barcode, Description: p.Description, Category: p.Category, Unit: p.Unit,
		Price: p.Price, Cost: p.Cost, MinStock: p.MinStock, ReorderQty: p.ReorderQty,
		PhotoFileId: p.PhotoFileID, Archived: p.Archived, TotalStock: p.TotalStock,
		CreatedAt: ts(p.CreatedAt), UpdatedAt: ts(p.UpdatedAt),
		Stock: []*adrientv1.InvStockLevel{}, Variants: []*adrientv1.InvProduct{}, Suppliers: []*adrientv1.InvProductSupplier{},
	}
	for _, s := range p.Stock {
		out.Stock = append(out.Stock, &adrientv1.InvStockLevel{LocationId: s.LocationID, LocationName: s.LocationName, Qty: s.Qty, ShelfCode: s.ShelfCode})
	}
	for _, v := range p.Variants {
		out.Variants = append(out.Variants, productToProto(v))
	}
	for _, s := range p.Suppliers {
		out.Suppliers = append(out.Suppliers, &adrientv1.InvProductSupplier{
			SupplierId: s.SupplierID, SupplierName: s.SupplierName, SupplierSku: s.SupplierSKU,
			Cost: s.Cost, LeadDays: s.LeadDays, PackSize: s.PackSize, Preferred: s.Preferred})
	}
	return out
}

func fieldsFromProto(r *adrientv1.SaveInvProductRequest) ProductFields {
	return ProductFields{
		ParentID: r.GetParentId(), VariantLabel: r.GetVariantLabel(), Name: r.GetName(), SKU: r.GetSku(),
		Barcode: r.GetBarcode(), Description: r.GetDescription(), Category: r.GetCategory(), Unit: r.GetUnit(),
		Price: r.GetPrice(), Cost: r.GetCost(), MinStock: r.GetMinStock(), ReorderQty: r.GetReorderQty(),
		PhotoFileID: r.GetPhotoFileId(), OpeningQty: r.GetOpeningQty(), OpeningLocationID: r.GetOpeningLocationId(),
	}
}

// ---- products

func (s *Service) ListProducts(ctx context.Context, req *adrientv1.ListInvProductsRequest) (*adrientv1.ListInvProductsResponse, error) {
	orgID, _, err := callerCtx(ctx)
	if err != nil {
		return nil, err
	}
	ps, err := s.repo.ListProducts(ctx, orgID, ProductFilter{
		Query: req.GetQuery(), Barcode: req.GetBarcode(), Category: req.GetCategory(), LocationID: req.GetLocationId(),
		IncludeArchived: req.GetIncludeArchived(), ArchivedOnly: req.GetArchivedOnly(),
		LowStockOnly: req.GetLowStockOnly(), Flat: req.GetFlat(),
	})
	if err != nil {
		return nil, rpcErr(err, "list products")
	}
	resp := &adrientv1.ListInvProductsResponse{Products: make([]*adrientv1.InvProduct, 0, len(ps))}
	for _, p := range ps {
		resp.Products = append(resp.Products, productToProto(p))
	}
	return resp, nil
}

func (s *Service) GetProduct(ctx context.Context, req *adrientv1.GetInvProductRequest) (*adrientv1.InvProduct, error) {
	orgID, _, err := callerCtx(ctx)
	if err != nil {
		return nil, err
	}
	p, err := s.repo.GetProduct(ctx, orgID, req.GetId())
	if err != nil {
		return nil, rpcErr(err, "product")
	}
	return productToProto(p), nil
}

func (s *Service) CreateProduct(ctx context.Context, req *adrientv1.SaveInvProductRequest) (*adrientv1.InvProduct, error) {
	orgID, userID, err := callerCtx(ctx)
	if err != nil {
		return nil, err
	}
	p, err := s.repo.CreateProduct(ctx, orgID, userID, fieldsFromProto(req))
	if err != nil {
		return nil, rpcErr(err, "create product")
	}
	return productToProto(p), nil
}

func (s *Service) UpdateProduct(ctx context.Context, req *adrientv1.SaveInvProductRequest) (*adrientv1.InvProduct, error) {
	orgID, _, err := callerCtx(ctx)
	if err != nil {
		return nil, err
	}
	p, err := s.repo.UpdateProduct(ctx, orgID, req.GetId(), fieldsFromProto(req))
	if err != nil {
		return nil, rpcErr(err, "product")
	}
	return productToProto(p), nil
}

func (s *Service) ArchiveProduct(ctx context.Context, req *adrientv1.ArchiveInvProductRequest) (*adrientv1.InvProduct, error) {
	orgID, _, err := callerCtx(ctx)
	if err != nil {
		return nil, err
	}
	p, err := s.repo.ArchiveProduct(ctx, orgID, req.GetId(), req.GetArchived())
	if err != nil {
		return nil, rpcErr(err, "product")
	}
	return productToProto(p), nil
}

func (s *Service) DeleteProduct(ctx context.Context, req *adrientv1.GetInvProductRequest) (*adrientv1.InvEmpty, error) {
	orgID, _, err := callerCtx(ctx)
	if err != nil {
		return nil, err
	}
	return &adrientv1.InvEmpty{}, rpcErr(s.repo.DeleteProduct(ctx, orgID, req.GetId()), "product")
}

func (s *Service) SetProductSuppliers(ctx context.Context, req *adrientv1.SetProductSuppliersRequest) (*adrientv1.InvProduct, error) {
	orgID, _, err := callerCtx(ctx)
	if err != nil {
		return nil, err
	}
	list := make([]ProductSupplier, 0, len(req.GetSuppliers()))
	for _, x := range req.GetSuppliers() {
		list = append(list, ProductSupplier{SupplierID: x.GetSupplierId(), SupplierSKU: x.GetSupplierSku(),
			Cost: x.GetCost(), LeadDays: x.GetLeadDays(), PackSize: x.GetPackSize(), Preferred: x.GetPreferred()})
	}
	p, err := s.repo.SetProductSuppliers(ctx, orgID, req.GetId(), list)
	if err != nil {
		return nil, rpcErr(err, "product")
	}
	return productToProto(p), nil
}

func (s *Service) ImportProducts(ctx context.Context, req *adrientv1.ImportInvProductsRequest) (*adrientv1.ImportInvProductsResponse, error) {
	orgID, userID, err := callerCtx(ctx)
	if err != nil {
		return nil, err
	}
	if len(req.GetRows()) > 5000 {
		return nil, status.Error(codes.InvalidArgument, "at most 5000 rows per import")
	}
	rows := make([]ProductFields, 0, len(req.GetRows()))
	for _, r := range req.GetRows() {
		f := fieldsFromProto(r)
		f.ParentID = "" // imports never create variants
		rows = append(rows, f)
	}
	res, err := s.repo.ImportProducts(ctx, orgID, userID, req.GetMode(), rows, req.GetLocationNames())
	if err != nil {
		return nil, rpcErr(err, "import")
	}
	return &adrientv1.ImportInvProductsResponse{Created: res.Created, Updated: res.Updated, Skipped: res.Skipped, Errors: res.Errors}, nil
}

// ---- stock

func (s *Service) AdjustStock(ctx context.Context, req *adrientv1.AdjustStockRequest) (*adrientv1.InvProduct, error) {
	orgID, userID, err := callerCtx(ctx)
	if err != nil {
		return nil, err
	}
	p, err := s.repo.AdjustStock(ctx, orgID, userID, req.GetId(), AdjustInput{
		LocationID: req.GetLocationId(), Qty: req.GetQty(), Absolute: req.GetAbsolute(), Reason: req.GetReason(),
		Note: req.GetNote(), Ref: req.GetRef(), ShelfCode: req.GetShelfCode(), UnitCost: req.GetUnitCost(),
	})
	if err != nil {
		return nil, rpcErr(err, "product")
	}
	return productToProto(p), nil
}

func (s *Service) TransferStock(ctx context.Context, req *adrientv1.TransferStockRequest) (*adrientv1.InvProduct, error) {
	orgID, userID, err := callerCtx(ctx)
	if err != nil {
		return nil, err
	}
	p, err := s.repo.TransferStock(ctx, orgID, userID, req.GetId(), req.GetFromLocationId(), req.GetToLocationId(), req.GetQty(), req.GetNote())
	if err != nil {
		return nil, rpcErr(err, "product")
	}
	return productToProto(p), nil
}

func (s *Service) ListHistory(ctx context.Context, req *adrientv1.ListInvHistoryRequest) (*adrientv1.ListInvHistoryResponse, error) {
	orgID, _, err := callerCtx(ctx)
	if err != nil {
		return nil, err
	}
	var since *time.Time
	if req.GetSince() != "" {
		t, err := time.Parse(time.RFC3339, req.GetSince())
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "since must be RFC3339")
		}
		since = &t
	}
	es, err := s.repo.ListHistory(ctx, orgID, req.GetProductId(), req.GetReason(), since, int(req.GetLimit()))
	if err != nil {
		return nil, rpcErr(err, "history")
	}
	resp := &adrientv1.ListInvHistoryResponse{Entries: make([]*adrientv1.InvHistoryEntry, 0, len(es))}
	for _, e := range es {
		resp.Entries = append(resp.Entries, &adrientv1.InvHistoryEntry{
			Id: e.ID, ProductId: e.ProductID, ProductName: e.ProductName, Sku: e.SKU, LocationId: e.LocationID,
			LocationName: e.LocationName, Delta: e.Delta, QtyAfter: e.QtyAfter, Reason: e.Reason, Note: e.Note,
			Ref: e.Ref, UserId: e.UserID, CreatedAt: ts(e.CreatedAt)})
	}
	return resp, nil
}

func (s *Service) ReorderList(ctx context.Context, _ *adrientv1.InvEmpty) (*adrientv1.ReorderListResponse, error) {
	orgID, _, err := callerCtx(ctx)
	if err != nil {
		return nil, err
	}
	ls, err := s.repo.ReorderList(ctx, orgID)
	if err != nil {
		return nil, rpcErr(err, "reorder")
	}
	resp := &adrientv1.ReorderListResponse{Lines: make([]*adrientv1.ReorderLine, 0, len(ls))}
	for _, l := range ls {
		resp.Lines = append(resp.Lines, &adrientv1.ReorderLine{Product: productToProto(l.Product), Stock: l.Stock,
			SuggestedQty: l.SuggestedQty, SupplierId: l.SupplierID, SupplierName: l.SupplierName,
			SupplierSku: l.SupplierSKU, UnitCost: l.UnitCost})
	}
	return resp, nil
}

func rowsToProto(rs []ReportRow) []*adrientv1.InvReportRow {
	out := make([]*adrientv1.InvReportRow, 0, len(rs))
	for _, r := range rs {
		out = append(out, &adrientv1.InvReportRow{Label: r.Label, Units: r.Units, CostValue: r.CostValue, RetailValue: r.RetailValue})
	}
	return out
}

func (s *Service) Report(ctx context.Context, _ *adrientv1.InvEmpty) (*adrientv1.InvReport, error) {
	orgID, _, err := callerCtx(ctx)
	if err != nil {
		return nil, err
	}
	r, err := s.repo.Report(ctx, orgID)
	if err != nil {
		return nil, rpcErr(err, "report")
	}
	return &adrientv1.InvReport{ProductCount: r.ProductCount, Units: r.Units, CostValue: r.CostValue,
		RetailValue: r.RetailValue, LowStockCount: r.LowStockCount, OutOfStockCount: r.OutOfStockCount,
		ByCategory: rowsToProto(r.ByCategory), ByLocation: rowsToProto(r.ByLocation),
		Movement: rowsToProto(r.Movement), TopMovers: rowsToProto(r.TopMovers)}, nil
}

// ---- locations

func (s *Service) ListLocations(ctx context.Context, _ *adrientv1.InvEmpty) (*adrientv1.ListInvLocationsResponse, error) {
	orgID, _, err := callerCtx(ctx)
	if err != nil {
		return nil, err
	}
	ls, err := s.repo.ListLocations(ctx, orgID)
	if err != nil {
		return nil, rpcErr(err, "locations")
	}
	resp := &adrientv1.ListInvLocationsResponse{Locations: make([]*adrientv1.InvLocation, 0, len(ls))}
	for _, l := range ls {
		resp.Locations = append(resp.Locations, locToProto(l))
	}
	return resp, nil
}

func (s *Service) CreateLocation(ctx context.Context, req *adrientv1.SaveInvLocationRequest) (*adrientv1.InvLocation, error) {
	return s.saveLocation(ctx, "", req)
}

func (s *Service) UpdateLocation(ctx context.Context, req *adrientv1.SaveInvLocationRequest) (*adrientv1.InvLocation, error) {
	return s.saveLocation(ctx, req.GetId(), req)
}

func (s *Service) saveLocation(ctx context.Context, id string, req *adrientv1.SaveInvLocationRequest) (*adrientv1.InvLocation, error) {
	orgID, _, err := callerCtx(ctx)
	if err != nil {
		return nil, err
	}
	l, err := s.repo.SaveLocation(ctx, orgID, id, Location{Name: req.GetName(), Code: req.GetCode(), IsDefault: req.GetIsDefault()})
	if err != nil {
		return nil, rpcErr(err, "location")
	}
	return locToProto(l), nil
}

func (s *Service) DeleteLocation(ctx context.Context, req *adrientv1.GetInvProductRequest) (*adrientv1.InvEmpty, error) {
	orgID, _, err := callerCtx(ctx)
	if err != nil {
		return nil, err
	}
	return &adrientv1.InvEmpty{}, rpcErr(s.repo.DeleteLocation(ctx, orgID, req.GetId()), "location")
}

// ---- suppliers

func (s *Service) ListSuppliers(ctx context.Context, _ *adrientv1.InvEmpty) (*adrientv1.ListInvSuppliersResponse, error) {
	orgID, _, err := callerCtx(ctx)
	if err != nil {
		return nil, err
	}
	ss, err := s.repo.ListSuppliers(ctx, orgID)
	if err != nil {
		return nil, rpcErr(err, "suppliers")
	}
	resp := &adrientv1.ListInvSuppliersResponse{Suppliers: make([]*adrientv1.InvSupplier, 0, len(ss))}
	for _, x := range ss {
		resp.Suppliers = append(resp.Suppliers, supplierToProto(x))
	}
	return resp, nil
}

func (s *Service) CreateSupplier(ctx context.Context, req *adrientv1.SaveInvSupplierRequest) (*adrientv1.InvSupplier, error) {
	return s.saveSupplier(ctx, "", req)
}

func (s *Service) UpdateSupplier(ctx context.Context, req *adrientv1.SaveInvSupplierRequest) (*adrientv1.InvSupplier, error) {
	return s.saveSupplier(ctx, req.GetId(), req)
}

func (s *Service) saveSupplier(ctx context.Context, id string, req *adrientv1.SaveInvSupplierRequest) (*adrientv1.InvSupplier, error) {
	orgID, _, err := callerCtx(ctx)
	if err != nil {
		return nil, err
	}
	x, err := s.repo.SaveSupplier(ctx, orgID, id, Supplier{Name: req.GetName(), ContactID: req.GetContactId(),
		Email: req.GetEmail(), Phone: req.GetPhone(), Notes: req.GetNotes()})
	if err != nil {
		return nil, rpcErr(err, "supplier")
	}
	return supplierToProto(x), nil
}

func (s *Service) DeleteSupplier(ctx context.Context, req *adrientv1.GetInvProductRequest) (*adrientv1.InvEmpty, error) {
	orgID, _, err := callerCtx(ctx)
	if err != nil {
		return nil, err
	}
	return &adrientv1.InvEmpty{}, rpcErr(s.repo.DeleteSupplier(ctx, orgID, req.GetId()), "supplier")
}
