package gameserver

import "testing"

func TestBattleExpMultiplier(t *testing.T) {
	if battleExpMultiplier(0, 0) != 1 {
		t.Fatal("none")
	}
	if battleExpMultiplier(5, 0) != 2 {
		t.Fatal("dual")
	}
	if battleExpMultiplier(5, 3) != 3 {
		t.Fatal("triple preferred")
	}
}

func TestShouldConsumeAutoFight(t *testing.T) {
	if !shouldConsumeAutoFight(&BattleState{EnemyID: 10, EnemyCatchable: true}) {
		t.Fatal("wild")
	}
	st := &BattleState{EnemyID: 70, OpponentUID: 1}
	if shouldConsumeAutoFight(st) {
		t.Fatal("pvp")
	}
}
