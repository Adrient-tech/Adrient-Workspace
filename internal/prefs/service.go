package prefs

import (
	"context"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	adrientv1 "github.com/Adrient-tech/Adrient-Workspace/gen/go/adrient/v1"
	"github.com/Adrient-tech/Adrient-Workspace/internal/auth"
)

// Service implements adrientv1.PreferencesServiceServer over a Repository.
type Service struct {
	repo *Repository
}

// NewService constructs a Service.
func NewService(repo *Repository) *Service { return &Service{repo: repo} }

func callerIDs(ctx context.Context) (userID, orgID string, err error) {
	u, ok := auth.UserFromContext(ctx)
	if !ok {
		return "", "", status.Error(codes.Unauthenticated, "no session")
	}
	o, ok := auth.OrgFromContext(ctx)
	if !ok {
		return "", "", status.Error(codes.Internal, "missing org context")
	}
	return u.ID, o.ID, nil
}

func toProto(p Preferences) *adrientv1.UserPreferences {
	return &adrientv1.UserPreferences{
		UserId:             p.UserID,
		OrgId:              p.OrgID,
		Language:           p.Language,
		Density:            p.Density,
		DefaultApp:         p.DefaultApp,
		DateFormat:         p.DateFormat,
		TimeFormat:         p.TimeFormat,
		WeekStart:          p.WeekStart,
		EmailNotifications: p.EmailNotifications,
		Extra:              p.Extra,
		UpdatedAt:          p.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func (s *Service) GetPreferences(ctx context.Context, _ *adrientv1.GetPreferencesRequest) (*adrientv1.UserPreferences, error) {
	userID, orgID, err := callerIDs(ctx)
	if err != nil {
		return nil, err
	}
	p, err := s.repo.GetOrDefault(ctx, orgID, userID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get preferences: %v", err)
	}
	return toProto(p), nil
}

func (s *Service) UpdatePreferences(ctx context.Context, req *adrientv1.UpdatePreferencesRequest) (*adrientv1.UserPreferences, error) {
	userID, orgID, err := callerIDs(ctx)
	if err != nil {
		return nil, err
	}
	p, err := s.repo.UpdatePreferences(ctx, orgID, userID, UpdateFields{
		Language:           req.GetLanguage(),
		Density:            req.GetDensity(),
		DefaultApp:         req.GetDefaultApp(),
		DateFormat:         req.GetDateFormat(),
		TimeFormat:         req.GetTimeFormat(),
		WeekStart:          req.GetWeekStart(),
		EmailNotifications: req.GetEmailNotifications(),
		Extra:              req.GetExtra(),
		Mask:               req.GetUpdateMask(),
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "update preferences: %v", err)
	}
	return toProto(p), nil
}
