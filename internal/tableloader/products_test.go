package tableloader

import (
	"path/filepath"
	"testing"
)

func TestLoadGoldAndItemMeta(t *testing.T) {
	dir := filepath.Join("..", "..", "tables", "xml")
	c := New(dir)
	if err := c.Load(); err != nil {
		t.Fatal(err)
	}
	gp, ok := c.GetGoldProduct(240000)
	if !ok || gp.ItemID != 300006 || gp.Price != 50 {
		t.Fatalf("gold product 240000: %+v ok=%v", gp, ok)
	}
	mp, ok := c.GetMoneyProduct(200000)
	if !ok || mp.GoldBonus != 10 {
		t.Fatalf("money product 200000: %+v ok=%v", mp, ok)
	}
	up, ok := c.GetEquipUpgrade(1)
	if !ok || up.ItemID != 100266 || up.LevelID != 2 {
		t.Fatalf("equip sendId=1: %+v ok=%v", up, ok)
	}
	if c.ItemHealHP(300011) < 1 {
		t.Fatal("heal 300011")
	}
	// xml HP 道具（若表有）或兜底
	if h := c.ItemHealHP(300012); h != 50 {
		t.Fatalf("300012 heal=%d", h)
	}
	if c.ItemBuyPrice(300011) != 20 {
		t.Fatalf("pet shop coin 300011=%d want 20", c.ItemBuyPrice(300011))
	}
	// 本客户端 PetShop：300651 为金豆商品 240033，不是参考服的赛尔豆 20000
	gp2, ok := c.GetGoldProduct(240033)
	if !ok || gp2.ItemID != 300651 || gp2.Price != 8 {
		t.Fatalf("pet shop gold 240033: %+v ok=%v", gp2, ok)
	}
}
