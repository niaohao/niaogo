package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestJSONListTopWarRanks(t *testing.T) {
	dir := t.TempDir()
	js, err := openJSONStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer js.Close()

	u1, err := js.CreateUser("a@test.local", "md5")
	if err != nil {
		t.Fatal(err)
	}
	u1.Nickname = "TopA"
	_ = js.SaveUser(u1)
	u2, err := js.CreateUser("b@test.local", "md5")
	if err != nil {
		t.Fatal(err)
	}
	u2.Nickname = "TopB"
	_ = js.SaveUser(u2)

	st1 := UserOpsState{CurTopLevel: 2000}
	st2 := UserOpsState{CurTopLevel: 3000}
	if err := js.SetUserOps(u1.UserID, st1); err != nil {
		t.Fatal(err)
	}
	if err := js.SetUserOps(u2.UserID, st2); err != nil {
		t.Fatal(err)
	}
	u3, _ := js.CreateUser("c@test.local", "md5")
	_ = js.SetUserOps(u3.UserID, UserOpsState{CurTopLevel: 0})

	list, err := js.ListTopWarRanks(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("len=%d want 2 (path %s)", len(list), filepath.Join(dir, "users"))
	}
	if list[0].UserID != u2.UserID || list[0].Score != 3000 || list[0].Nickname != "TopB" {
		t.Fatalf("first %+v", list[0])
	}
	if list[1].UserID != u1.UserID || list[1].Score != 2000 {
		t.Fatalf("second %+v", list[1])
	}
}

func TestNormalizeClampsTopLevel(t *testing.T) {
	st := NormalizeUserOps(UserOpsState{CurTopLevel: -5}, time.Now())
	if st.CurTopLevel != 0 {
		t.Fatal(st.CurTopLevel)
	}
	st = NormalizeUserOps(UserOpsState{CurTopLevel: 2_000_000}, time.Now())
	if st.CurTopLevel != TopLevelMax {
		t.Fatal(st.CurTopLevel)
	}
}
