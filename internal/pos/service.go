package pos

import (
	"context"
	"errors"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	adrientv1 "github.com/Adrient-tech/Adrient-Workspace/gen/go/adrient/v1"
	"github.com/Adrient-tech/Adrient-Workspace/internal/auth"
)

// Service implements adrientv1.PosServiceServer over a Repository.
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
		return status.Error(codes.FailedPrecondition, strings.TrimPrefix(err.Error(), "conflict: "))
	}
	return status.Errorf(codes.Internal, "%s: %v", what, err)
}

// org runs fn with the caller's org and user ids.
func org[T any](ctx context.Context, what string, fn func(orgID, userID string) (T, error)) (T, error) {
	var zero T
	o, u, err := callerCtx(ctx)
	if err != nil {
		return zero, err
	}
	v, err := fn(o, u)
	if err != nil {
		return zero, rpcErr(err, what)
	}
	return v, nil
}

var empty = &adrientv1.PosEmpty{}

func (s *Service) ListRegisters(ctx context.Context, _ *adrientv1.PosEmpty) (*adrientv1.ListPosRegistersResponse, error) {
	return org(ctx, "registers", func(o, _ string) (*adrientv1.ListPosRegistersResponse, error) {
		l, err := s.repo.ListRegisters(ctx, o)
		return &adrientv1.ListPosRegistersResponse{Registers: l}, err
	})
}

func (s *Service) CreateRegister(ctx context.Context, r *adrientv1.SavePosRegisterRequest) (*adrientv1.PosRegister, error) {
	return org(ctx, "register", func(o, _ string) (*adrientv1.PosRegister, error) {
		return s.repo.SaveRegister(ctx, o, "", r.GetName(), r.GetLocationId())
	})
}

func (s *Service) UpdateRegister(ctx context.Context, r *adrientv1.SavePosRegisterRequest) (*adrientv1.PosRegister, error) {
	return org(ctx, "register", func(o, _ string) (*adrientv1.PosRegister, error) {
		return s.repo.SaveRegister(ctx, o, r.GetId(), r.GetName(), r.GetLocationId())
	})
}

func (s *Service) ArchiveRegister(ctx context.Context, r *adrientv1.PosIdRequest) (*adrientv1.PosEmpty, error) {
	return org(ctx, "register", func(o, _ string) (*adrientv1.PosEmpty, error) {
		return empty, s.repo.ArchiveRegister(ctx, o, r.GetId())
	})
}

func (s *Service) OpenShift(ctx context.Context, r *adrientv1.OpenPosShiftRequest) (*adrientv1.PosShift, error) {
	return org(ctx, "register", func(o, u string) (*adrientv1.PosShift, error) {
		return s.repo.OpenShift(ctx, o, u, r.GetRegisterId(), r.GetOpeningCash())
	})
}

func (s *Service) CloseShift(ctx context.Context, r *adrientv1.ClosePosShiftRequest) (*adrientv1.PosShift, error) {
	return org(ctx, "shift", func(o, u string) (*adrientv1.PosShift, error) {
		return s.repo.CloseShift(ctx, o, u, r.GetId(), r.GetCountedCash(), r.GetNote())
	})
}

func (s *Service) CashMove(ctx context.Context, r *adrientv1.PosCashMoveRequest) (*adrientv1.PosShift, error) {
	return org(ctx, "shift", func(o, _ string) (*adrientv1.PosShift, error) {
		return s.repo.CashMove(ctx, o, r.GetId(), r.GetAmount(), r.GetNote())
	})
}

func (s *Service) GetShift(ctx context.Context, r *adrientv1.PosIdRequest) (*adrientv1.PosShift, error) {
	return org(ctx, "shift", func(o, _ string) (*adrientv1.PosShift, error) { return s.repo.GetShift(ctx, o, r.GetId()) })
}

func (s *Service) ListShifts(ctx context.Context, r *adrientv1.ListPosShiftsRequest) (*adrientv1.ListPosShiftsResponse, error) {
	return org(ctx, "shifts", func(o, _ string) (*adrientv1.ListPosShiftsResponse, error) {
		l, err := s.repo.ListShifts(ctx, o, r.GetRegisterId(), int(r.GetLimit()))
		return &adrientv1.ListPosShiftsResponse{Shifts: l}, err
	})
}

func (s *Service) HoldSale(ctx context.Context, r *adrientv1.PosSaleInput) (*adrientv1.PosSale, error) {
	return org(ctx, "sale", func(o, u string) (*adrientv1.PosSale, error) { return s.repo.SaveSale(ctx, o, u, r, false) })
}

func (s *Service) CompleteSale(ctx context.Context, r *adrientv1.PosSaleInput) (*adrientv1.PosSale, error) {
	return org(ctx, "sale", func(o, u string) (*adrientv1.PosSale, error) { return s.repo.SaveSale(ctx, o, u, r, true) })
}

func (s *Service) GetSale(ctx context.Context, r *adrientv1.PosIdRequest) (*adrientv1.PosSale, error) {
	return org(ctx, "sale", func(o, _ string) (*adrientv1.PosSale, error) { return s.repo.GetSale(ctx, o, r.GetId()) })
}

func (s *Service) DeleteHeldSale(ctx context.Context, r *adrientv1.PosIdRequest) (*adrientv1.PosEmpty, error) {
	return org(ctx, "held sale", func(o, _ string) (*adrientv1.PosEmpty, error) {
		return empty, s.repo.DeleteHeldSale(ctx, o, r.GetId())
	})
}

func (s *Service) ListSales(ctx context.Context, r *adrientv1.ListPosSalesRequest) (*adrientv1.ListPosSalesResponse, error) {
	return org(ctx, "sales", func(o, _ string) (*adrientv1.ListPosSalesResponse, error) {
		l, err := s.repo.ListSales(ctx, o, r)
		return &adrientv1.ListPosSalesResponse{Sales: l}, err
	})
}

func (s *Service) RefundSale(ctx context.Context, r *adrientv1.RefundPosSaleRequest) (*adrientv1.PosSale, error) {
	return org(ctx, "sale", func(o, u string) (*adrientv1.PosSale, error) { return s.repo.RefundSale(ctx, o, u, r) })
}

func (s *Service) ListTaxRates(ctx context.Context, _ *adrientv1.PosEmpty) (*adrientv1.ListPosTaxRatesResponse, error) {
	return org(ctx, "taxes", func(o, _ string) (*adrientv1.ListPosTaxRatesResponse, error) {
		l, err := s.repo.ListTaxRates(ctx, o)
		return &adrientv1.ListPosTaxRatesResponse{Rates: l}, err
	})
}

func (s *Service) CreateTaxRate(ctx context.Context, r *adrientv1.SavePosTaxRateRequest) (*adrientv1.PosTaxRate, error) {
	return org(ctx, "tax rate", func(o, _ string) (*adrientv1.PosTaxRate, error) {
		r.Id = ""
		return s.repo.SaveTaxRate(ctx, o, r)
	})
}

func (s *Service) UpdateTaxRate(ctx context.Context, r *adrientv1.SavePosTaxRateRequest) (*adrientv1.PosTaxRate, error) {
	return org(ctx, "tax rate", func(o, _ string) (*adrientv1.PosTaxRate, error) { return s.repo.SaveTaxRate(ctx, o, r) })
}

func (s *Service) DeleteTaxRate(ctx context.Context, r *adrientv1.PosIdRequest) (*adrientv1.PosEmpty, error) {
	return org(ctx, "tax rate", func(o, _ string) (*adrientv1.PosEmpty, error) {
		return empty, s.repo.DeleteTaxRate(ctx, o, r.GetId())
	})
}

func (s *Service) ListDiscountPresets(ctx context.Context, _ *adrientv1.PosEmpty) (*adrientv1.ListPosDiscountPresetsResponse, error) {
	return org(ctx, "discounts", func(o, _ string) (*adrientv1.ListPosDiscountPresetsResponse, error) {
		l, err := s.repo.ListDiscountPresets(ctx, o)
		return &adrientv1.ListPosDiscountPresetsResponse{Presets: l}, err
	})
}

func (s *Service) CreateDiscountPreset(ctx context.Context, r *adrientv1.SavePosDiscountPresetRequest) (*adrientv1.PosDiscountPreset, error) {
	return org(ctx, "discount", func(o, _ string) (*adrientv1.PosDiscountPreset, error) {
		r.Id = ""
		return s.repo.SaveDiscountPreset(ctx, o, r)
	})
}

func (s *Service) UpdateDiscountPreset(ctx context.Context, r *adrientv1.SavePosDiscountPresetRequest) (*adrientv1.PosDiscountPreset, error) {
	return org(ctx, "discount", func(o, _ string) (*adrientv1.PosDiscountPreset, error) { return s.repo.SaveDiscountPreset(ctx, o, r) })
}

func (s *Service) DeleteDiscountPreset(ctx context.Context, r *adrientv1.PosIdRequest) (*adrientv1.PosEmpty, error) {
	return org(ctx, "discount", func(o, _ string) (*adrientv1.PosEmpty, error) {
		return empty, s.repo.DeleteDiscountPreset(ctx, o, r.GetId())
	})
}

func (s *Service) GetSettings(ctx context.Context, _ *adrientv1.PosEmpty) (*adrientv1.PosSettings, error) {
	return org(ctx, "settings", func(o, _ string) (*adrientv1.PosSettings, error) { return s.repo.GetSettings(ctx, o) })
}

func (s *Service) SaveSettings(ctx context.Context, r *adrientv1.PosSettings) (*adrientv1.PosSettings, error) {
	return org(ctx, "settings", func(o, _ string) (*adrientv1.PosSettings, error) { return s.repo.SaveSettings(ctx, o, r) })
}

func (s *Service) ListBalances(ctx context.Context, _ *adrientv1.PosEmpty) (*adrientv1.ListPosBalancesResponse, error) {
	return org(ctx, "balances", func(o, _ string) (*adrientv1.ListPosBalancesResponse, error) {
		l, err := s.repo.ListBalances(ctx, o)
		return &adrientv1.ListPosBalancesResponse{Balances: l}, err
	})
}

func (s *Service) GetBalance(ctx context.Context, r *adrientv1.PosIdRequest) (*adrientv1.PosBalance, error) {
	return org(ctx, "balance", func(o, _ string) (*adrientv1.PosBalance, error) { return s.repo.GetBalance(ctx, o, r.GetId()) })
}

func (s *Service) AdjustBalance(ctx context.Context, r *adrientv1.AdjustPosBalanceRequest) (*adrientv1.PosBalance, error) {
	return org(ctx, "balance", func(o, _ string) (*adrientv1.PosBalance, error) {
		return s.repo.AdjustBalance(ctx, o, r.GetContactId(), r.GetKind(), r.GetDelta(), r.GetNote())
	})
}

func (s *Service) Report(ctx context.Context, r *adrientv1.PosReportRequest) (*adrientv1.PosReport, error) {
	return org(ctx, "report", func(o, _ string) (*adrientv1.PosReport, error) {
		return s.repo.Report(ctx, o, r.GetSince(), r.GetUntil())
	})
}
