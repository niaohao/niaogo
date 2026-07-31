package gameserver

import "testing"

func TestCalcFusionDVGemAverage(t *testing.T) {
	// 31=5宝石, 31+31 → 5 → 31
	if got := calcFusionDV(31, 31); got != 31 {
		t.Fatalf("31+31 want 31 got %d", got)
	}
	// 16(1宝石)+8(1宝石) → 1 → 16
	if got := calcFusionDV(16, 8); got != 16 {
		t.Fatalf("16+8 want 16 got %d", got)
	}
	// 31(5)+0(0) → 2 → 24 (16|8)
	if got := calcFusionDV(31, 0); got != 24 {
		t.Fatalf("31+0 want 24 got %d", got)
	}
}

func TestResolveMantisAndLanlan(t *testing.T) {
	pid, _, name, ok := resolveMantisBoss(102, 0)
	if !ok || pid != 124 || name == "" {
		t.Fatalf("mantis light: %v %d %s", ok, pid, name)
	}
	pid2, _, _, ok2 := resolveMantisBoss(102, 1)
	if !ok2 || pid2 != 125 {
		t.Fatalf("mantis dark: %v %d", ok2, pid2)
	}
	_, _, _, h, ok3 := resolveLanlanBoss(108, 12)
	if !ok3 || h != 6 {
		t.Fatalf("lanlan hard honor=%d ok=%v", h, ok3)
	}
}

func TestGemCountOfDV(t *testing.T) {
	if gemCountOfDV(31) != 5 || gemCountOfDV(0) != 0 || gemCountOfDV(16) != 1 {
		t.Fatal("gem count")
	}
}

func TestCalcPetRecycleCoins(t *testing.T) {
	// 80*(20+1)*(1+100/200)=80*21*1.5=2520
	got := calcPetRecycleCoins(80, 20, 100)
	if got != 2520 {
		t.Fatalf("got %d", got)
	}
}

func TestTrainRoomEVYield(t *testing.T) {
	y, ok := trainRoomEVYield(488)
	if !ok || y[0] != 1 {
		t.Fatalf("junior HP: %v %v", ok, y)
	}
	y2, ok2 := trainRoomEVYield(473)
	if !ok2 || y2[3] != 9 {
		t.Fatalf("senior SA: %v %v", ok2, y2)
	}
}

func TestNonoVipSignDay1(t *testing.T) {
	p := nonoVipSignDayRewards(1)
	if len(p) < 3 {
		t.Fatal(p)
	}
}

func TestMatchDailyExpConstants(t *testing.T) {
	if petKingDailyExpEach != 40000 || petKingDailyExpCap != 2 {
		t.Fatal("pet king exp")
	}
	if grandMeleeDailyExpEach != 50000 || grandMeleeDailyExpCap != 2 {
		t.Fatal("grand melee exp")
	}
}
