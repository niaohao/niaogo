package gameserver

import (
	"testing"

	"niaohao/server/internal/store"
)

func TestEffectiveDisplayIDAndSkin(t *testing.T) {
	p := &store.Pet{PetID: 303, DisplayFormID: 301}
	if effectiveDisplayID(p) != 301 {
		t.Fatalf("display=%d", effectiveDisplayID(p))
	}
	if petSkinID(p) != 301 {
		t.Fatalf("skin=%d", petSkinID(p))
	}
	p.DisplayFormID = 0
	if petSkinID(p) != 0 {
		t.Fatalf("same form skin should be 0")
	}
}

func TestGoldPromoNeedCoins(t *testing.T) {
	if goldPromoNeedCoins(0) != 50000 {
		t.Fatal(goldPromoNeedCoins(0))
	}
	if goldPromoNeedCoins(1) != 100000 {
		t.Fatal(goldPromoNeedCoins(1))
	}
	if goldPromoNeedCoins(4) != 250000 {
		t.Fatal(goldPromoNeedCoins(4))
	}
	if goldPromoNeedCoins(5) != -1 {
		t.Fatal(goldPromoNeedCoins(5))
	}
}

func TestApplyLockedDisplayForm(t *testing.T) {
	p := &store.Pet{PetID: 303, FormLocked: 1, LockedDisplayFormID: 301, DisplayFormID: 302}
	applyLockedDisplayForm(p)
	if p.DisplayFormID != 301 {
		t.Fatalf("got %d", p.DisplayFormID)
	}
}
