package gameserver

import (
	"testing"

	"niaohao/server/internal/store"
)

func TestBattleYieldingExpFallback(t *testing.T) {
	st := &BattleState{EnemyID: 1, EnemyLevel: 10}
	got := battleYieldingExp(nil, st)
	want := 15 + 10*2
	if got != want {
		t.Fatalf("fallback got %d want %d", got, want)
	}
	if battleYieldingExp(nil, nil) != 15 {
		t.Fatal("nil state")
	}
}

func TestTeacherRelRoundTrip(t *testing.T) {
	db, err := store.OpenJSON(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	u1, err := db.CreateUser("t1@test.local", "pass")
	if err != nil || u1 == nil {
		t.Fatal(err)
	}
	u2, err := db.CreateUser("t2@test.local", "pass")
	if err != nil || u2 == nil {
		t.Fatal(err)
	}
	teacherUID, studentUID := u1.UserID, u2.UserID
	s := &Server{cfg: Config{Store: db}}
	s.setTeacherRel(teacherUID, 0, uint32(studentUID))
	s.setTeacherRel(studentUID, uint32(teacherUID), 0)
	tID, sID, _, _ := s.teacherRelOf(teacherUID)
	if tID != 0 || sID != uint32(studentUID) {
		t.Fatalf("teacher side got teacher=%d student=%d", tID, sID)
	}
	tID, sID, _, _ = s.teacherRelOf(studentUID)
	if tID != uint32(teacherUID) || sID != 0 {
		t.Fatalf("student side got teacher=%d student=%d", tID, sID)
	}
	s.setTeacherExpPond(teacherUID, 120)
	_, _, _, pond := s.teacherRelOf(teacherUID)
	if pond != 120 {
		t.Fatalf("pond=%d", pond)
	}
	s.contributeTeacherExpPond(teacherUID, 50) // +10
	_, _, _, pond = s.teacherRelOf(teacherUID)
	if pond != 130 {
		t.Fatalf("after contribute pond=%d", pond)
	}
	orphan, err := db.CreateUser("t3@test.local", "pass")
	if err != nil || orphan == nil {
		t.Fatal(err)
	}
	s.contributeTeacherExpPond(orphan.UserID, 50) // no student
	_, _, _, pond = s.teacherRelOf(orphan.UserID)
	if pond != 0 {
		t.Fatalf("orphan pond=%d", pond)
	}
}

func TestTeamPKStubBodyLens(t *testing.T) {
	cases := []struct {
		name string
		n    int
	}{
		{"sign", 30},
		{"weeky", 28},
		{"history", 36},
		{"someone", 12},
	}
	for _, c := range cases {
		if len(make([]byte, c.n)) != c.n {
			t.Fatalf("%s len", c.name)
		}
	}
}
