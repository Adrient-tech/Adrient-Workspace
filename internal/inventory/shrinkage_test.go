package inventory

import (
	"context"
	"errors"
	"testing"
)

func TestMarginRows(t *testing.T) {
	rows := MarginRows([]Product{
		{ID: "a", Name: "Good", Price: 200, Cost: 100},
		{ID: "b", Name: "Loss", Price: 90, Cost: 100},
		{ID: "c", Name: "No cost", Price: 50},
		{ID: "d", Name: "Gone", Price: 10, Cost: 1, Archived: true},
		{ID: "e", Name: "Thirds", Price: 3, Cost: 2},
	})
	if len(rows) != 3 || rows[0].ProductID != "b" || !rows[0].BelowCost || rows[0].Margin != -10 || rows[0].Percent != -11.1 {
		t.Fatalf("%+v", rows)
	}
	if rows[1].ProductID != "e" || rows[1].Percent != 33.3 || rows[2].Percent != 50 {
		t.Fatalf("%+v", rows)
	}
}

func TestWriteOffAndShrinkageInDB(t *testing.T) {
	pool, org, user := setupDB(t)
	r := NewRepository(pool)
	ctx := context.Background()
	locs, _ := r.ListLocations(ctx, org)
	main := locs[0].ID
	pid := mustProduct(t, r, org, user, "Milk", "MILK-1", 0)
	if _, err := r.AdjustStock(ctx, org, user, pid, AdjustInput{Qty: 20, Reason: "receive", UnitCost: 40, LocationID: main}); err != nil {
		t.Fatal(err)
	}
	for _, in := range []WriteOffInput{
		{ProductID: pid, LocationID: main, Qty: 3, LossReason: "expired"},
		{ProductID: pid, LocationID: main, Qty: 2, LossReason: "damaged", Note: "dropped"},
		{ProductID: pid, LocationID: main, Qty: 1, LossReason: "internal"},
	} {
		if _, err := r.WriteOff(ctx, org, user, in); err != nil {
			t.Fatalf("%+v: %v", in, err)
		}
	}
	if _, err := r.AdjustStock(ctx, org, user, pid, AdjustInput{Qty: 1, Reason: "remove", LocationID: main}); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []WriteOffInput{
		{ProductID: pid, LocationID: main, Qty: 1, LossReason: "stolen"},
		{ProductID: pid, LocationID: main, Qty: 0, LossReason: "lost"},
		{ProductID: pid, LocationID: main, Qty: 500, LossReason: "lost"},
	} {
		if _, err := r.WriteOff(ctx, org, user, bad); !errors.Is(err, ErrInvalid) {
			t.Fatalf("%+v should be refused, got %v", bad, err)
		}
	}
	if got := stockAt(t, r, org, pid, main); got != 13 {
		t.Fatalf("stock %v", got)
	}
	rep, err := r.Shrinkage(ctx, org, 30)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Rows) != 1 || rep.TotalUnits != 5 || rep.TotalValue != 200 || rep.InternalUse != 1 || rep.Unspecified != 1 {
		t.Fatalf("%+v", rep)
	}
	if rep.Rows[0].Reasons["expired"] != 3 || rep.Rows[0].Reasons["damaged"] != 2 {
		t.Fatalf("%+v", rep.Rows[0])
	}
}
