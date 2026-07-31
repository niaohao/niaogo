package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestJSONStoreBasic(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "saves")
	db, err := OpenJSON(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if db.Backend() != "json" {
		t.Fatalf("backend=%s", db.Backend())
	}
	u, err := db.CreateUser("test@local", "abcd")
	if err != nil {
		t.Fatal(err)
	}
	got, err := db.FindByEmail("test@local")
	if err != nil || got == nil || got.UserID != u.UserID {
		t.Fatalf("find email: %+v %v", got, err)
	}
	if err := db.AddCoins(u.UserID, 100); err != nil {
		t.Fatal(err)
	}
	coins, err := db.GetCoins(u.UserID)
	if err != nil || coins != 100 {
		t.Fatalf("coins=%d err=%v", coins, err)
	}
	if err := db.AddItem(u.UserID, 300001, 5); err != nil {
		t.Fatal(err)
	}
	n, err := db.GetItemCount(u.UserID, 300001)
	if err != nil || n != 5 {
		t.Fatalf("item=%d err=%v", n, err)
	}
	catch, err := db.GrantPet(u.UserID, 1, "布布种子", 5, 0, 0, []int{10001})
	if err != nil || catch <= 0 {
		t.Fatalf("grant pet: %v %d", err, catch)
	}
	pets, err := db.ListBagPets(u.UserID)
	if err != nil || len(pets) != 1 {
		t.Fatalf("bag=%d err=%v", len(pets), err)
	}
	// 重启后仍可读
	db2, err := OpenJSON(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()
	got2, _ := db2.FindByEmail("test@local")
	if got2 == nil || got2.Coins != 100 {
		t.Fatalf("reload user: %+v", got2)
	}
	if _, err := os.Stat(filepath.Join(dir, "meta.json")); err != nil {
		t.Fatal(err)
	}
}

func TestJSONStorePetGMStats(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "saves")
	db, err := OpenJSON(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	u, err := db.CreateUser("gmstats@local", "abcd")
	if err != nil {
		t.Fatal(err)
	}
	catch, err := db.GrantPet(u.UserID, 1, "布布种子", 50, 31, 0, []int{10001})
	if err != nil || catch <= 0 {
		t.Fatalf("grant pet: %v %d", err, catch)
	}
	stats := [6]int{500, 400, 300, 200, 100, 50}
	if err := db.SetPetGMStats(u.UserID, catch, stats); err != nil {
		t.Fatal(err)
	}
	p, err := db.GetPetByCatchTime(u.UserID, catch)
	if err != nil || p == nil || !p.HasGMStats || p.GMStats != stats {
		t.Fatalf("after set: %+v err=%v", p, err)
	}
	db.Close()
	db2, err := OpenJSON(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()
	p2, err := db2.GetPetByCatchTime(u.UserID, catch)
	if err != nil || p2 == nil || !p2.HasGMStats || p2.GMStats != stats {
		t.Fatalf("reload: %+v err=%v", p2, err)
	}
	if err := db2.ClearPetGMStats(u.UserID, catch); err != nil {
		t.Fatal(err)
	}
	p3, err := db2.GetPetByCatchTime(u.UserID, catch)
	if err != nil || p3 == nil || p3.HasGMStats {
		t.Fatalf("after clear: HasGMStats=%v err=%v", p3 != nil && p3.HasGMStats, err)
	}
}
