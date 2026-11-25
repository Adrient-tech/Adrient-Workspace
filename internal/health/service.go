// Package health implements the HealthService gRPC API.
//
// HealthService reports backend version and uptime so operators and orchestrators
// (e.g. Kubernetes liveness probes, process-compose) can confirm the server is up.
package health

import (
	"context"
	"time"

	adrientv1 "github.com/Adrient-tech/Adrient-Workspace/gen/go/adrient/v1"
)

// Service implements adrientv1.HealthServiceServer.
type Service struct {
	adrientv1.UnimplementedHealthServiceServer

	version   string
	commit    string
	startedAt time.Time
}

// NewService constructs a HealthService with the given build identity and start time.
func NewService(version, commit string, startedAt time.Time) *Service {
	return &Service{
		version:   version,
		commit:    commit,
		startedAt: startedAt,
	}
}

// Check returns version, commit, and uptime.
func (s *Service) Check(_ context.Context, _ *adrientv1.CheckRequest) (*adrientv1.CheckResponse, error) {
	return &adrientv1.CheckResponse{
		Version:       s.version,
		Commit:        s.commit,
		UptimeSeconds: int64(time.Since(s.startedAt).Seconds()),
	}, nil
}
