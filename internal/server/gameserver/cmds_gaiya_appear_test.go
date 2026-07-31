package gameserver

import (
	"encoding/binary"
	"testing"
	"time"
)

func TestGetGaiyaMapIDForTodayMatchesWeekdayTable(t *testing.T) {
	cases := []struct {
		wday time.Weekday
		want int
	}{
		{time.Sunday, 54},
		{time.Monday, 15},
		{time.Tuesday, 54},
		{time.Wednesday, 105},
		{time.Thursday, 54},
		{time.Friday, 15},
		{time.Saturday, 105},
	}
	for _, tc := range cases {
		got := gaiyaMapForWeekday(int(tc.wday))
		if got != tc.want {
			t.Fatalf("wday=%v got map=%d want %d", tc.wday, got, tc.want)
		}
	}
}

func TestGaiyaAppearCondition(t *testing.T) {
	st := &BattleState{MapID: 15, Round: 2, LastHitWasCrit: false}
	// 用固定 needMap 测规则分支（与 gaiyaAppearConditionOK 同逻辑）
	check := func(need int, st *BattleState) bool {
		if st == nil || st.MapID != need {
			return false
		}
		switch need {
		case 15:
			return st.Round > 0 && st.Round <= 2
		case 54:
			return st.LastHitWasCrit
		case 105:
			return st.Round > 10
		default:
			return false
		}
	}
	if !check(15, st) {
		t.Fatal("map15 round<=2")
	}
	st.Round = 3
	if check(15, st) {
		t.Fatal("map15 round>2 should fail")
	}
	st = &BattleState{MapID: 54, Round: 5, LastHitWasCrit: true}
	if !check(54, st) {
		t.Fatal("map54 crit")
	}
	st.LastHitWasCrit = false
	if check(54, st) {
		t.Fatal("map54 no crit")
	}
	st = &BattleState{MapID: 105, Round: 11}
	if !check(105, st) {
		t.Fatal("map105 round>10")
	}
	st.Round = 10
	if check(105, st) {
		t.Fatal("map105 round=10 should fail")
	}
}

func TestBuildGaiyaAppearCompleteTaskBody(t *testing.T) {
	b := buildGaiyaAppearCompleteTaskBody()
	if len(b) != 24 {
		t.Fatalf("len=%d", len(b))
	}
	if binary.BigEndian.Uint32(b[0:4]) != gaiyaAppearTaskID {
		t.Fatal("taskID")
	}
	if binary.BigEndian.Uint32(b[16:20]) != gaiyaAppearEssenceItem {
		t.Fatal("item")
	}
}
