package gameserver

import (
	"path/filepath"
	"testing"

	"niaohao/server/internal/tableloader"
)

func TestPickGrandMeleeTempPets(t *testing.T) {
	xmlDir := filepath.Join("..", "..", "..", "tables", "xml")
	cat := tableloader.New(xmlDir)
	if err := cat.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	s := &Server{cfg: Config{Catalog: cat}}
	pets, ok := s.pickGrandMeleeTempPets(6)
	if !ok {
		t.Fatal("pool too small")
	}
	if len(pets) != 6 {
		t.Fatalf("len=%d", len(pets))
	}
	seen := map[int64]bool{}
	for i, p := range pets {
		if p.PetID <= 0 || p.PetID > grandMeleePetMaxID {
			t.Fatalf("[%d] bad petID=%d", i, p.PetID)
		}
		if p.Level != grandMeleeEnemyLevel {
			t.Fatalf("[%d] level=%d", i, p.Level)
		}
		if p.CatchTime == 0 || seen[p.CatchTime] {
			t.Fatalf("[%d] catch=%d", i, p.CatchTime)
		}
		seen[p.CatchTime] = true
		if len(p.Skills) == 0 {
			t.Fatalf("[%d] no skills", i)
		}
		base := cat.PetBase(p.PetID)
		if base == nil || base.EvolvesTo != 0 {
			t.Fatalf("[%d] not final form id=%d", i, p.PetID)
		}
	}
}

func TestTryGrandMeleeEnemySwitch(t *testing.T) {
	xmlDir := filepath.Join("..", "..", "..", "tables", "xml")
	cat := tableloader.New(xmlDir)
	if err := cat.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	s := &Server{cfg: Config{Catalog: cat}, byUID: map[int64]*Client{}}
	pets, ok := s.pickGrandMeleeTempPets(6)
	if !ok {
		t.Fatal("pool")
	}
	player, enemy := pets[:3], pets[3:]
	s.melee.set(7, player, enemy)
	ids := []int{enemy[0].PetID, enemy[1].PetID, enemy[2].PetID}
	st := &BattleState{
		Active:         true,
		IsGrandMelee:   true,
		EnemyHP:        0,
		EnemyTeamIDs:   ids,
		EnemyTeamIndex: 0,
		EnemyID:        ids[0],
	}
	if !s.tryGrandMeleeEnemySwitch(nil, 7, st) {
		t.Fatal("expected switch")
	}
	if st.EnemyTeamIndex != 1 || st.EnemyID != ids[1] || st.EnemyHP == 0 {
		t.Fatalf("after switch idx=%d id=%d hp=%d", st.EnemyTeamIndex, st.EnemyID, st.EnemyHP)
	}
	st.EnemyHP = 0
	if !s.tryGrandMeleeEnemySwitch(nil, 7, st) {
		t.Fatal("expected 2nd switch")
	}
	st.EnemyHP = 0
	if s.tryGrandMeleeEnemySwitch(nil, 7, st) {
		t.Fatal("no more enemies")
	}
}

func TestBuildNoteReadyToFightSides(t *testing.T) {
	st := &BattleState{
		PlayerPetID: 1, PlayerLevel: 100, PlayerHP: 10, PlayerMaxHP: 10, PlayerCatchTime: 1,
		EnemyID: 2, EnemyLevel: 100, EnemyHP: 20, EnemyMaxHP: 20, EnemyName: "AI",
	}
	mine := [][]byte{buildSimplePetInfo(1, 100, 10, 10, 1, nil, 1, 0, 0)}
	theirs := [][]byte{
		buildSimplePetInfo(2, 100, 20, 20, 0, nil, 2, 0, 0),
		buildSimplePetInfo(3, 100, 20, 20, 0, nil, 3, 0, 0),
		buildSimplePetInfo(4, 100, 20, 20, 0, nil, 4, 0, 0),
	}
	body := buildNoteReadyToFightSides(100, "赛尔", st, mine, theirs)
	if len(body) < 40 {
		t.Fatalf("body too short %d", len(body))
	}
}
