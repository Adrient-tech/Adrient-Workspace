package chat

import (
	"context"
	"testing"
)

func TestCallStatusHelpers(t *testing.T) {
	if inStatus("ringing", false) != "missed" || inStatus("ringing", true) != "ringing" ||
		inStatus("accepted", false) != "answered" || inStatus("declined", false) != "declined" {
		t.Fatal("inStatus")
	}
	if outStatus([]string{"declined", "accepted"}, false) != "answered" ||
		outStatus([]string{"declined"}, false) != "declined" ||
		outStatus([]string{"ringing"}, false) != "missed" ||
		outStatus([]string{"ringing"}, true) != "ringing" {
		t.Fatal("outStatus")
	}
}

func TestCallLogRoundTrip(t *testing.T) {
	pool, org, caller := setupDB(t)
	callee := seedSecondUser(t, pool, org)
	st := NewStore(NewRepository(pool))
	ctx := context.Background()
	if err := st.LogRing(ctx, org, caller, "abc-defg-hij", "Call", true, []string{callee}); err != nil {
		t.Fatal(err)
	}
	if err := st.LogRespond(ctx, org, callee, caller, "abc-defg-hij", "accepted"); err != nil {
		t.Fatal(err)
	}
	if err := st.LogEnd(ctx, org, callee, "abc-defg-hij"); err != nil {
		t.Fatal(err)
	}
	// second call: caller cancels before the callee answers -> missed for both views
	_ = st.LogRing(ctx, org, caller, "xyz-abcd-efg", "Call", false, []string{callee})
	_ = st.LogRespond(ctx, org, caller, callee, "xyz-abcd-efg", "cancelled")
	in, err := st.CallHistory(ctx, org, callee, 10)
	if err != nil || len(in) != 2 {
		t.Fatalf("callee history: %v %v", in, err)
	}
	if in[0].Status != "missed" || in[0].Direction != "in" || in[1].Status != "answered" || in[1].EndedAt == "" {
		t.Fatalf("callee rows: %+v", in)
	}
	out, _ := st.CallHistory(ctx, org, caller, 10)
	if len(out) != 2 || out[1].Direction != "out" || out[1].Status != "answered" || out[0].Status != "missed" {
		t.Fatalf("caller rows: %+v", out)
	}
}
