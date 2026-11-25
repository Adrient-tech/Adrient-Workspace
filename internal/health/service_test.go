package health

import (
	"context"
	"testing"
	"time"

	adrientv1 "github.com/Adrient-tech/Adrient-Workspace/gen/go/adrient/v1"
)

func TestCheck_ReturnsVersionAndCommit(t *testing.T) {
	startedAt := time.Now().Add(-3 * time.Second)
	svc := NewService("1.2.3", "abc1234", startedAt)

	resp, err := svc.Check(context.Background(), &adrientv1.CheckRequest{})
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}

	if resp.GetVersion() != "1.2.3" {
		t.Errorf("version: got %q, want %q", resp.GetVersion(), "1.2.3")
	}
	if resp.GetCommit() != "abc1234" {
		t.Errorf("commit: got %q, want %q", resp.GetCommit(), "abc1234")
	}
	if resp.GetUptimeSeconds() < 3 || resp.GetUptimeSeconds() > 5 {
		t.Errorf("uptime_seconds: got %d, want roughly 3", resp.GetUptimeSeconds())
	}
}
