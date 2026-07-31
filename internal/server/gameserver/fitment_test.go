package gameserver

import (
	"testing"

	"niaohao/server/internal/store"
)

func TestSyncOwnerFitmentsAutoPlace(t *testing.T) {
	bag := &fitmentBag{Items: map[int]int{}, Fitments: nil}
	if !syncOwnerFitmentsOnEnter(bag) {
		t.Fatal("bootstrap+place")
	}
	if fitmentPlacedCount(bag, 500503) != 1 {
		t.Fatalf("want 分子转化仪 auto-placed, fitments=%v", bag.Fitments)
	}
	if fitmentPlacedCount(bag, 500502) != 1 {
		t.Fatalf("want 恢复仓 auto-placed, fitments=%v", bag.Fitments)
	}
	if !hasRoomFrame(bag.Fitments) {
		t.Fatal("want default room frame")
	}
	// 摆上后仓库对应数量应扣减
	if fitmentItemCount(bag, 500503) != 0 {
		t.Fatalf("placed 500503 should leave warehouse 0, got %d", fitmentItemCount(bag, 500503))
	}
	// 第二次进入不重复摆放
	n := len(bag.Fitments)
	_ = syncOwnerFitmentsOnEnter(bag)
	if len(bag.Fitments) != n {
		t.Fatalf("no duplicate place: before=%d after=%d", n, len(bag.Fitments))
	}
}

func TestSanitizeAddsDefaultFrame(t *testing.T) {
	bag := &fitmentBag{
		Items:    map[int]int{},
		Fitments: []store.Fitment{{ID: 500503, X: 100, Y: 100}},
	}
	_ = sanitizeUserFitments(bag)
	if !hasRoomFrame(bag.Fitments) {
		t.Fatalf("fitments=%v", bag.Fitments)
	}
}

func TestEnsureStarterWarehouseRestore(t *testing.T) {
	bag := &fitmentBag{
		Items:    map[int]int{500001: 1},
		Fitments: []store.Fitment{},
	}
	if !ensureStarterFitmentWarehouseItems(bag) {
		t.Fatal("restore")
	}
	if fitmentItemCount(bag, 500502) != 1 || fitmentItemCount(bag, 500503) != 1 {
		t.Fatalf("items=%v", bag.Items)
	}
}

func TestReconcileFitmentWarehouse(t *testing.T) {
	bag := &fitmentBag{
		Items:    map[int]int{500510: 2},
		Fitments: []store.Fitment{},
	}
	before := append([]store.Fitment(nil), bag.Fitments...)
	bag.Fitments = append(bag.Fitments, store.Fitment{ID: 500510, X: 100, Y: 100})
	reconcileFitmentWarehouse(bag, before, bag.Fitments)
	if fitmentItemCount(bag, 500510) != 1 || fitmentPlacedCount(bag, 500510) != 1 {
		t.Fatalf("wh=%d pl=%d", fitmentItemCount(bag, 500510), fitmentPlacedCount(bag, 500510))
	}
	before = append([]store.Fitment(nil), bag.Fitments...)
	bag.Fitments = nil
	reconcileFitmentWarehouse(bag, before, bag.Fitments)
	if fitmentItemCount(bag, 500510) != 2 {
		t.Fatalf("return wh=%d", fitmentItemCount(bag, 500510))
	}
}

func TestPlayerPlaceDeductsWarehouse(t *testing.T) {
	bag := &fitmentBag{
		Items:    map[int]int{500655: 1, 500001: 1},
		Fitments: []store.Fitment{},
	}
	before := append([]store.Fitment(nil), bag.Fitments...)
	bag.Fitments = []store.Fitment{
		{ID: 500001},
		{ID: 500655, X: 600, Y: 280},
	}
	reconcileFitmentWarehouse(bag, before, bag.Fitments)
	if fitmentItemCount(bag, 500655) != 0 {
		t.Fatalf("placed should leave warehouse empty, got %d", fitmentItemCount(bag, 500655))
	}
}
