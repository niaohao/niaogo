package gameserver

import (
	"encoding/binary"
	"testing"
)

func TestBuildLevelBossBody(t *testing.T) {
	b := buildLevelBossBody(3, []uint32{10, 11})
	if len(b) != 16 {
		t.Fatalf("len=%d", len(b))
	}
	if binary.BigEndian.Uint32(b[0:4]) != 3 {
		t.Fatal("level")
	}
	if binary.BigEndian.Uint32(b[4:8]) != 2 {
		t.Fatal("count")
	}
	if binary.BigEndian.Uint32(b[8:12]) != 10 || binary.BigEndian.Uint32(b[12:16]) != 11 {
		t.Fatal("ids")
	}
}

func TestBraveTowerBosses(t *testing.T) {
	b := braveTowerBosses(1)
	if len(b) != 1 || b[0] != 10 {
		t.Fatalf("%v", b)
	}
}

func TestDarkPortalParseAndBoss(t *testing.T) {
	d, sub := parseDarkPortalCurDoor(0)
	if d != 0 || sub != 0 {
		t.Fatal(d, sub)
	}
	d, sub = parseDarkPortalCurDoor(7) // 第2门 sub1
	if d != 1 || sub != 1 {
		t.Fatal(d, sub)
	}
	d, sub = parseDarkPortalCurDoor(60) // 第11门
	if d != 10 || sub != 0 {
		t.Fatal(d, sub)
	}

	if darkPortalMapID(0) != 503 || darkPortalMapID(10) != 513 {
		t.Fatal("map ids")
	}
	if darkPortalBoss(0) != 171 {
		t.Fatal(darkPortalBoss(0))
	}
	if darkPortalBoss(99) != 171 {
		t.Fatal("overflow")
	}
	if darkPortalRequiredSuperLevel(0) != 1 || darkPortalRequiredSuperLevel(10) != 11 {
		t.Fatal("dark portal level gate")
	}

	pid, lv := darkPortalBossEntry(0, 0)
	if pid != 171 || lv != 50 {
		t.Fatalf("door0: %d lv%d", pid, lv)
	}
	pid, lv = darkPortalBossEntry(2, 1) // 第3门子门奇拉塔顿
	if pid != 183 || lv != 75 {
		t.Fatalf("door2 sub1: %d lv%d", pid, lv)
	}
	pid, lv = darkPortalBossEntry(10, 2) // 第11门奈尼狄亚
	if pid != 1400 || lv != 100 {
		t.Fatalf("door10 sub2: %d lv%d", pid, lv)
	}
	// 无子门配置时回退主 Boss
	pid, lv = darkPortalBossEntry(0, 3)
	if pid != 171 {
		t.Fatalf("fallback: %d", pid)
	}
}

func TestDarkPortalFirstKillRewards(t *testing.T) {
	cases := map[int]int{
		169:  400109,
		171:  400110,
		174:  400111,
		177:  400112,
		183:  400113,
		195:  400115,
		192:  400116,
		222:  400120,
		356:  400129,
		438:  400142,
		656:  400184,
		779:  400197,
		1182: 400304,
		1187: 400306,
		1403: 400432,
		1397: 400430,
		1400: 400431,
	}
	for pet, item := range cases {
		rew, ok := sptFirstKillByPetID[pet]
		if !ok || rew.RewardItemID != item {
			t.Fatalf("pet %d: got %+v want item %d", pet, rew, item)
		}
	}
}

func TestFreshTowerBoss(t *testing.T) {
	if freshTowerBoss(1) != 10 {
		t.Fatal(freshTowerBoss(1))
	}
	if freshTowerBoss(30) != 39 {
		t.Fatal(freshTowerBoss(30))
	}
}
