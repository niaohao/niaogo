package gameserver

import (
	"encoding/binary"
	"testing"
)

func TestBuildAttackValueSize(t *testing.T) {
	av := buildAttackValue(10002, 10004, 1, 10, 0, 190, 200, 0, 0, 0, nil)
	if len(av) != 78 {
		t.Fatalf("empty skills len=%d want 78", len(av))
	}
	skills := [][2]uint32{{10004, 34}, {20002, 40}}
	av2 := buildAttackValue(10002, 10004, 1, 10, 0, 190, 200, 0, 1, 0, skills)
	if len(av2) != 78+16 {
		t.Fatalf("with 2 skills len=%d want %d", len(av2), 78+16)
	}
	n := binary.BigEndian.Uint32(av2[32:36])
	if n != 2 {
		t.Fatalf("skillListCount=%d", n)
	}
}

func TestBuildAttackValueStatusBattleLv(t *testing.T) {
	st := &BattleState{}
	st.PlayerStatus.Para = true
	st.PlayerStages[stageAtk] = 2
	st.EnemyStatus.Burn = true
	st.EnemyStages[stageDef] = -1
	pav := buildAttackValueFromState(1, 10001, 1, 10, 0, 100, 100, 0, 0, 0, st, true, nil)
	statusOff := 40 // after isCrit at 36 when skillCount=0
	if pav[statusOff] != 2 {
		t.Fatalf("player para status byte=%d", pav[statusOff])
	}
	if int8(pav[statusOff+20]) != 2 {
		t.Fatalf("player atk stage=%d", int8(pav[statusOff+20]))
	}
	eav := buildAttackValueFromState(0, 10001, 1, 10, 0, 50, 100, 0, 0, 0, st, false, nil)
	if eav[statusOff+2] != 2 {
		t.Fatalf("enemy burn status=%d", eav[statusOff+2])
	}
	if int8(eav[statusOff+21]) != -1 {
		t.Fatalf("enemy def stage=%d", int8(eav[statusOff+21]))
	}
}

func TestEncodeParasiteSeedStatus(t *testing.T) {
	st := &BattleState{}
	st.EnemyBuff.DrainRounds = 5
	ps := encodeFightStatusForSide(st, true)
	if ps[statusIdxDrain] != 5 {
		t.Fatalf("player drain icon=%d want 5", ps[statusIdxDrain])
	}
	es := encodeFightStatusForSide(st, false)
	if es[statusIdxDrained] != 5 {
		t.Fatalf("enemy drained icon=%d want 5", es[statusIdxDrained])
	}
	st.PlayerBuff.ImmuneStatusRounds = 3
	ps = encodeFightStatusForSide(st, true)
	if ps[statusIdxImmuneStatus] != 3 {
		t.Fatalf("immune status icon=%d want 3", ps[statusIdxImmuneStatus])
	}
}

func TestNewlyControlledAfterOpponent(t *testing.T) {
	at := snapshotControlStatus(battleStatus{})
	if !newlyControlledAfterOpponent(battleStatus{Sleep: true}, at) {
		t.Fatal("new sleep should block")
	}
	if newlyControlledAfterOpponent(battleStatus{}, at) {
		t.Fatal("no status should not block")
	}
	at2 := snapshotControlStatus(battleStatus{Sleep: true})
	if newlyControlledAfterOpponent(battleStatus{Sleep: true}, at2) {
		t.Fatal("preexisting sleep is not newly controlled")
	}
}

func TestAttackValueIncludesParasiteBeforeDecrement(t *testing.T) {
	st := &BattleState{PlayerHP: 100, PlayerMaxHP: 100, EnemyHP: 100, EnemyMaxHP: 100}
	st.EnemyBuff.DrainRounds = 5
	tickBattleBuffEffects(st)
	av := buildAttackValueFromState(1, 10001, 1, 10, 0, int32(st.PlayerHP), st.PlayerMaxHP, 0, 0, 0, st, true, nil)
	statusOff := 40
	if av[statusOff+statusIdxDrain] != 5 {
		t.Fatalf("drain rounds in AV=%d want 5 (before decrement)", av[statusOff+statusIdxDrain])
	}
	decrementBattleBuffRounds(st)
	if st.EnemyBuff.DrainRounds != 4 {
		t.Fatalf("after dec DrainRounds=%d want 4", st.EnemyBuff.DrainRounds)
	}
}

// 击杀后：敌方 AV remain=0（npcIsdied）；玩家 AV remain 仍为自己剩余血。
func TestAttackValueRemainIsAttackerOwnHP(t *testing.T) {
	const uid uint32 = 10001
	playerHP, playerMax := uint32(80), uint32(100)
	enemyHP, enemyMax := uint32(0), uint32(200) // 已击杀
	playerAv := buildAttackValue(uid, 10001, 1, 50, 0, int32(playerHP), playerMax, 0, 0, 0, nil)
	enemyAv := buildAttackValue(0, 0, 0, 0, 0, int32(enemyHP), enemyMax, 0, 0, 0, nil)

	// remainHP @ offset 20 (int32), maxHP @ 24
	if got := binary.BigEndian.Uint32(playerAv[20:24]); got != playerHP {
		t.Fatalf("player remain=%d want %d", got, playerHP)
	}
	if got := binary.BigEndian.Uint32(playerAv[24:28]); got != playerMax {
		t.Fatalf("player max=%d want %d", got, playerMax)
	}
	if got := int32(binary.BigEndian.Uint32(enemyAv[20:24])); got != 0 {
		t.Fatalf("enemy remain=%d want 0 (npcIsdied)", got)
	}
	if binary.BigEndian.Uint32(enemyAv[0:4]) != 0 {
		t.Fatal("enemy userID must be 0")
	}
}

// 飘字 lostHP 用公式伤害，可大于目标剩余 HP（参考 playerLostHPFor2505）。
func TestAttackValueLostHPIsFormulaDamage(t *testing.T) {
	const formula uint32 = 80
	av := buildAttackValue(10001, 10001, 1, formula, 0, 150, 200, 0, 0, 0, nil)
	if got := binary.BigEndian.Uint32(av[12:16]); got != formula {
		t.Fatalf("lostHP=%d want formula %d", got, formula)
	}
}

func TestBuildChangePetInfo(t *testing.T) {
	b := buildChangePetInfo(10002, 4, "伊优", 5, 21, 21, 1768449796)
	if len(b) != 40 {
		t.Fatalf("len=%d", len(b))
	}
	if binary.BigEndian.Uint32(b[4:8]) != 4 {
		t.Fatal("petID")
	}
}

func TestBuildNoteUpdateProp(t *testing.T) {
	b := buildNoteUpdateProp(1768449796, 4, 5, 10, 10, 50, 21, 30, 30, 30, 30, 30, [6]int{})
	// header 8 + UpdatePropInfo 18*4 = 8+72 = 80
	if len(b) != 80 {
		t.Fatalf("len=%d want 80", len(b))
	}
}

func TestPotionTables(t *testing.T) {
	if potionHealHP(300011) != 20 || potionRestorePP(300016) != 5 {
		t.Fatal("potion map")
	}
}
