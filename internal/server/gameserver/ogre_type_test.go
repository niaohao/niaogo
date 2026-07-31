package gameserver

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	dir := findTestDataDir()
	if dir != "" {
		_ = LoadBattleData(dir)
	}
	os.Exit(m.Run())
}

func findTestDataDir() string {
	candidates := []string{
		filepath.Join("..", "..", "..", "data"), // internal/server/gameserver -> server/data
		filepath.Join("..", "..", "data"),
		"data",
	}
	for _, c := range candidates {
		p := filepath.Join(c, "type_chart.json")
		if _, err := os.Stat(p); err == nil {
			abs, _ := filepath.Abs(c)
			return abs
		}
	}
	return ""
}

func TestBuildMapOgreList36(t *testing.T) {
	slots := make([]OgreSlot, 9)
	slots[2] = OgreSlot{PetID: 10, Level: 2, CanCatch: true}
	slots[5] = OgreSlot{PetID: 13, Level: 5, CanCatch: true}
	body := buildMapOgreList(slots)
	if len(body) != 36 {
		t.Fatalf("len=%d", len(body))
	}
	if binary.BigEndian.Uint32(body[0:4]) != 10 {
		t.Fatalf("slot0=%d", binary.BigEndian.Uint32(body[0:4]))
	}
	if binary.BigEndian.Uint32(body[4:8]) != 13 {
		t.Fatalf("slot1=%d", binary.BigEndian.Uint32(body[4:8]))
	}
}

func TestTypeMultiplierBasics(t *testing.T) {
	if typeMultiplier(3, 1) != 2 { // 火克草
		t.Fatalf("fire vs grass got %v", typeMultiplier(3, 1))
	}
	if typeMultiplier(2, 3) != 2 { // 水克火
		t.Fatalf("water vs fire got %v", typeMultiplier(2, 3))
	}
	if typeMultiplier(5, 7) != 0 { // 电无效地
		t.Fatalf("electric vs ground got %v", typeMultiplier(5, 7))
	}
	if stabBonus(2, 2) != 1.5 {
		t.Fatal("stab")
	}
}

func TestTypeChartLoadedAndCompositeSTAB(t *testing.T) {
	if !battleTypes.loaded {
		t.Skip("type_chart.json not loaded")
	}
	if typeMultiplier(3, 1) != 2 {
		t.Fatal("chart fire vs grass")
	}
	// 27=火飞行：火技能应 STAB
	if stabBonus(3, 27) != 1.5 {
		t.Fatal("composite STAB fire on 火飞行")
	}
	if stabBonus(4, 27) != 1.5 {
		t.Fatal("composite STAB flying on 火飞行")
	}
}

func TestOgreEnterClearsAndDelays(t *testing.T) {
	h := &ogreHub{}
	h.set(1, 10, []OgreSlot{{PetID: 10, Level: 2}})
	h.setEnterMap(1, 10)
	h.clear(1, 10)
	if slots := h.get(1, 10); slots != nil {
		t.Fatal("enter should clear slots")
	}
	st, ok := h.snapshotState(1)
	if !ok || st.MapID != 10 || !st.LastRefreshTime.IsZero() {
		t.Fatalf("enter state=%+v ok=%v", st, ok)
	}
	h.setFightEnd(1)
	st, _ = h.snapshotState(1)
	if st.LastFightEndTime.IsZero() {
		t.Fatal("fight end time missing")
	}
}

func TestMapHasOgreConfig(t *testing.T) {
	if len(mapWildPool) == 0 {
		t.Skip("no pool")
	}
	if !mapHasOgreConfig(10) {
		t.Fatal("map10 should have config")
	}
	if mapHasOgreConfig(999999) {
		t.Fatal("unknown map")
	}
}

func TestGenerateOgreMap10(t *testing.T) {
	if len(mapWildPool) == 0 {
		t.Skip("map_wild_config not loaded")
	}
	has := false
	for i := 0; i < 80; i++ {
		slots := generateOgreSlots(10)
		for _, s := range slots {
			if s.PetID == 10 || s.PetID == 164 {
				has = true
			}
		}
		if has {
			break
		}
	}
	if !has {
		t.Fatal("map10 should spawn 皮皮")
	}
}

func TestWildPoolExpanded(t *testing.T) {
	if findTestDataDir() == "" {
		t.Skip("no data dir")
	}
	if len(mapWildPool) < 50 {
		t.Fatalf("expected reference-scale map pools, got %d", len(mapWildPool))
	}
	p10 := mapWildPool[10]
	if len(p10.Common) == 0 || p10.Common[0].PetID != 10 {
		t.Fatalf("map10 common=%v", p10.Common)
	}
	p11 := mapWildPool[11]
	if len(p11.Rare) < 1 {
		t.Fatalf("map11 rare empty")
	}
}

func TestOgreCatchableEvolvedForm(t *testing.T) {
	// 依丁丝(84) EvolvesFrom>0 → 不可捕
	e := pickWildEntry([]wildEntry{{84, 1, 6, true}})
	if e.PetID == 84 && e.CanCatch {
		// catalog 未加载时无法判定，跳过
		if petBaseFromCatalog(84) != nil {
			t.Fatal("evolved form should not be catchable")
		}
	}
}
