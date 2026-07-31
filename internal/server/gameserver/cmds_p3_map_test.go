package gameserver

import "testing"

func TestFourGodClientParamMap(t *testing.T) {
	cases := []struct {
		mapID, p2, wantPet int
		wantName           string
	}{
		{401, 0, 501, "玄武守护兽"},
		{401, 1, 501, "巴斯特"},
		{403, 0, 502, "青龙守护兽"},
		{403, 1, 502, "朵拉格"},
		{483, 5, 503, "泰格尔"},
		{483, 2, 5015, "战虎"},
		{483, 4, 5016, "电虎"},
	}
	for _, tc := range cases {
		pid, _, name := resolveChallengeBoss(tc.mapID, uint32(tc.p2))
		if pid != tc.wantPet || name != tc.wantName {
			t.Fatalf("map=%d p2=%d got %d %q want %d %q", tc.mapID, tc.p2, pid, name, tc.wantPet, tc.wantName)
		}
	}
}

func TestFourGodNonTrueFormNoReward(t *testing.T) {
	if !isFourGodNonTrueForm(401, 0) || isFourGodNonTrueForm(401, 1) {
		t.Fatal("401: only p0 is non-true")
	}
	if !isFourGodNonTrueForm(403, 0) || isFourGodNonTrueForm(403, 1) {
		t.Fatal("403: only p0 is non-true")
	}
	if !isFourGodNonTrueForm(483, 3) || isFourGodNonTrueForm(483, 5) {
		t.Fatal("483: only p5 is true form")
	}
	if isFourGodNonTrueForm(17, 0) {
		t.Fatal("non-four-god")
	}
}

func TestPuniDailyResFill(t *testing.T) {
	s := &Server{}
	// no store / ops: zeros
	arr := make([]byte, 50)
	s.fillPuniDailyRes(1, arr)
	for i := 40; i <= 47; i++ {
		if arr[i] != 0 {
			t.Fatalf("index %d = %d", i, arr[i])
		}
	}
}

func TestPuniMapAlias(t *testing.T) {
	if !isPuniChallengeMap(514) || !isPuniChallengeMap(108) || !isPuniChallengeMap(500) {
		t.Fatal("puni maps")
	}
	if isPuniChallengeMap(401) {
		t.Fatal("401 not puni")
	}
}

func TestPuniDailyKey(t *testing.T) {
	if puniDailyKey(1) != "puniSeal_1" || puniDailyKey(8) != "puniSeal_8" {
		t.Fatal(puniDailyKey(1), puniDailyKey(8))
	}
}
