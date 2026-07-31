package gameserver

import (
	"encoding/binary"
	"testing"
)

func TestRareOnlyMapsAlwaysSpawn(t *testing.T) {
	if len(mapWildPool) == 0 {
		t.Skip("map_wild_config not loaded")
	}
	for _, mapID := range []int{8, 323, 405, 494} {
		pool := mapWildPool[mapID]
		if len(pool.Common) != 0 || len(pool.Rare) == 0 {
			t.Fatalf("map %d expected rare-only, common=%d rare=%d", mapID, len(pool.Common), len(pool.Rare))
		}
		got := 0
		for i := 0; i < 20; i++ {
			slots := generateOgreSlots(mapID)
			for _, s := range slots {
				if s.PetID > 0 {
					got++
				}
			}
			if got > 0 {
				break
			}
		}
		if got == 0 {
			t.Fatalf("map %d rare-only should spawn within 20 tries", mapID)
		}
	}
}

func TestBuildAresiaSpacePrize(t *testing.T) {
	b := buildAresiaSpacePrize(5, 0, 0, [][2]uint32{{300001, 2}})
	if len(b) != 24 {
		t.Fatalf("len=%d", len(b))
	}
	if binary.BigEndian.Uint32(b[0:4]) != 5 {
		t.Fatal("bonus")
	}
	if binary.BigEndian.Uint32(b[12:16]) != 1 {
		t.Fatal("n")
	}
	if binary.BigEndian.Uint32(b[16:20]) != 300001 || binary.BigEndian.Uint32(b[20:24]) != 2 {
		t.Fatal("item")
	}
	empty := buildAresiaSpacePrize(0, 0, 0, nil)
	if len(empty) != 16 || binary.BigEndian.Uint32(empty[12:16]) != 0 {
		t.Fatal("empty")
	}
}

func TestMap15HeltokBoss(t *testing.T) {
	pid, lv, name := resolveChallengeBoss(15, 1)
	if pid != 527 || name != "赫尔托克" || lv != 80 {
		t.Fatalf("got pet=%d lv=%d name=%q", pid, lv, name)
	}
	if resolveBossFixedHP(527, 1, 15, 1) != 8000 {
		t.Fatal(resolveBossFixedHP(527, 1, 15, 1))
	}
	rew := sptFirstKillByPetID[527]
	if rew.RewardItemID != 400152 {
		t.Fatal(rew)
	}
}

func TestXioCoralAndWellOreCates(t *testing.T) {
	if miningCateToItemID[17] != 0 {
		// cate 17 在 map325 走 grantMiningCate 时查表；表内可无 17，默认 400001
	}
	if talkXioCoralItem != 400026 || talkCateXioCoralVIP != 2054 {
		t.Fatal("xio coral consts")
	}
	if talkWellOreItemA != 400001 || talkWellOreItemB != 400002 {
		t.Fatal("well ore consts")
	}
	if atresiaPrizeByType[5].ItemID == 0 {
		t.Fatal("atresia type 5")
	}
}
