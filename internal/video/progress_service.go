package video

import (
	"context"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	adrientv1 "github.com/Adrient-tech/Adrient-Workspace/gen/go/adrient/v1"
	"github.com/Adrient-tech/Adrient-Workspace/internal/auth"
)

func toProgressProto(p Progress) *adrientv1.VideoProgress {
	return &adrientv1.VideoProgress{
		VideoId:         p.VideoID,
		PositionSeconds: p.PositionSeconds,
		Percent:         p.Percent,
		Watched:         p.Watched,
		UpdatedAt:       p.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func (s *Service) SetVideoProgress(ctx context.Context, req *adrientv1.SetVideoProgressRequest) (*adrientv1.SetVideoProgressResponse, error) {
	u, ok := auth.UserFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "no session")
	}
	if s.progress == nil {
		return nil, status.Error(codes.Unimplemented, "progress not configured")
	}
	p, err := s.progress.Upsert(ctx, u.ID, req.GetVideoId(), req.GetPositionSeconds(), req.GetPercent())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "set progress: %v", err)
	}
	return &adrientv1.SetVideoProgressResponse{Progress: toProgressProto(p)}, nil
}

func (s *Service) GetVideoProgress(ctx context.Context, req *adrientv1.GetVideoProgressRequest) (*adrientv1.VideoProgress, error) {
	u, ok := auth.UserFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "no session")
	}
	if s.progress == nil {
		return nil, status.Error(codes.Unimplemented, "progress not configured")
	}
	p, err := s.progress.Get(ctx, u.ID, req.GetVideoId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get progress: %v", err)
	}
	return toProgressProto(p), nil
}
