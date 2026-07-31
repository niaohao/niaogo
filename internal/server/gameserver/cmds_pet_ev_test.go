package gameserver

import (
	"encoding/binary"
	"testing"

	"niaohao/server/internal/store"
	"niaohao/server/internal/tableloader"
)

func TestClampPetEV(t *testing.T) {
	ev := [6]int{255, 255, 255, 0, 0, 0} // 765
	clampPetEV(&ev)
	sum := 0
	for _, v := range ev {
		if v > 255 {
			t.Fatalf("over 255: %v", ev)
		}
		sum += v
	}
	if sum > 510 {
		t.Fatalf("sum=%d ev=%v", sum, ev)
	}
}

func TestCalcStatWithEV(t *testing.T) {
	base := calcStat(50, 20, 50, 0)
	with := calcStat(50, 20, 50, 252)
	if with <= base {
		t.Fatalf("ev should raise stat base=%d with=%d", base, with)
	}
}

func TestCalcStatWithNature(t *testing.T) {
	base := calcStatWithNature(100, 20, 50, 0, 1.0)
	up := calcStatWithNature(100, 20, 50, 0, 1.1)
	down := calcStatWithNature(100, 20, 50, 0, 0.9)
	if up <= base {
		t.Fatalf("1.1 should raise: base=%d up=%d", base, up)
	}
	if down >= base {
		t.Fatalf("0.9 should lower: base=%d down=%d", base, down)
	}
}

func TestPetSixStatsNatureAffectsAtk(t *testing.T) {
	// nature 0 兜底：攻×1.1 防×0.9
	_, atk0, def0, _, _, _ := petSixStats(1, 50, 20, 0, [6]int{})
	_, atk20, def20, _, _, _ := petSixStats(1, 50, 20, 20, [6]int{}) // 中性
	if atk0 <= atk20 {
		t.Fatalf("nature0 atk should > nature20: %d vs %d", atk0, atk20)
	}
	if def0 >= def20 {
		t.Fatalf("nature0 def should < nature20: %d vs %d", def0, def20)
	}
}

func TestAddEVWithCap(t *testing.T) {
	cur := [6]int{250, 250, 0, 0, 0, 0} // 500
	got := addEVWithCap(cur, [6]int{0, 10, 5, 0, 0, 0})
	if evTotal(got) > 510 {
		t.Fatalf("sum=%d", evTotal(got))
	}
	if got[1] < cur[1] {
		t.Fatalf("should not reduce: %v", got)
	}
}

func TestScaleYieldEV(t *testing.T) {
	y := scaleYieldEV([6]int{0, 1, 0, 0, 0, 0}, 2)
	if y[1] != 2 {
		t.Fatalf("got %v", y)
	}
}

func TestShouldGrantYieldingEV(t *testing.T) {
	if !shouldGrantYieldingEV(&BattleState{EnemyID: 10, EnemyCatchable: true}) {
		t.Fatal("wild should grant")
	}
	if shouldGrantYieldingEV(&BattleState{EnemyID: 70, EnemyCatchable: false}) {
		t.Fatal("SPT雷伊 should not grant")
	}
	if !shouldGrantYieldingEV(&BattleState{EnemyID: 58, EnemyCatchable: false}) {
		t.Fatal("non-SPT boss should grant")
	}
}

func TestBuildBoostTimesBody(t *testing.T) {
	b := buildBoostTimesBody(store.BoostTimes{LearnTimes: 30})
	if len(b) != 20 {
		t.Fatalf("len=%d", len(b))
	}
	if binary.BigEndian.Uint32(b[16:20]) != 30 {
		t.Fatalf("learn=%d", binary.BigEndian.Uint32(b[16:20]))
	}
}

func TestNatureModsFallback(t *testing.T) {
	m := tableloader.NatureModsOf(0)
	if m.Atk != 1.1 || m.Def != 0.9 {
		t.Fatalf("nature0 mods=%+v", m)
	}
	m20 := tableloader.NatureModsOf(20)
	if m20.Atk != 1 || m20.Spd != 1 {
		t.Fatalf("nature20 mods=%+v", m20)
	}
}
