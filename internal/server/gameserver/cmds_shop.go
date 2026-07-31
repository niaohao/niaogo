package gameserver

import (
	"encoding/binary"
	"log"
	"math"
	"math/rand"

	"niaohao/server/internal/cmdname"
)

// handleGoldCheckRemain CMD 1105：应答 gold×100（4B）。金豆购前校验。
func (s *Server) handleGoldCheckRemain(c *Client, uid uint32) {
	gold := 0
	if s.cfg.Store != nil {
		if g, err := s.cfg.Store.GetGold(int64(uid)); err == nil {
			gold = g
		}
	}
	out := make([]byte, 4)
	binary.BigEndian.PutUint32(out, uint32(gold*100))
	s.send(c, 1105, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d gold=%d", cmdname.Format(1105), uid, gold)
}

// handleGoldBuyProduct CMD 1104：请求 productID(4)+count(2 BE)；应答 skip(4)+payGold×100+gold×100。
func (s *Server) handleGoldBuyProduct(c *Client, uid uint32, body []byte) {
	if len(body) < 6 || s.cfg.Store == nil || s.cfg.Catalog == nil {
		s.send(c, 1104, uid, 0, nil)
		return
	}
	productID := binary.BigEndian.Uint32(body[0:4])
	count := int(binary.BigEndian.Uint16(body[4:6]))
	if count <= 0 {
		count = 1
	}
	if count > 99 {
		count = 99
	}
	entry, ok := s.cfg.Catalog.GetGoldProduct(productID)
	if !ok {
		s.send(c, 1104, uid, 0, nil)
		log.Printf("[CMD] WARN  %s UID=%d unknown product=%d", cmdname.Format(1104), uid, productID)
		return
	}
	cost := entry.Price * count
	if s.userIsVip(int64(uid)) && entry.VipFactor >= 0 && entry.VipFactor < 1 {
		cost = int(math.Round(float64(entry.Price) * entry.VipFactor * float64(count)))
		if cost < 0 {
			cost = 0
		}
	}
	bal, okSpend, err := s.cfg.Store.TrySpendGold(int64(uid), cost)
	if err != nil || !okSpend {
		s.send(c, 1104, uid, 0, nil)
		log.Printf("[CMD] OK     %s UID=%d fail gold cost=%d bal=%d err=%v",
			cmdname.Format(1104), uid, cost, bal, err)
		return
	}
	if err := s.cfg.Store.AddItem(int64(uid), entry.ItemID, count); err != nil {
		_ = s.cfg.Store.AddGold(int64(uid), cost)
		s.send(c, 1104, uid, 0, nil)
		log.Printf("[CMD] WARN  %s UID=%d AddItem: %v", cmdname.Format(1104), uid, err)
		return
	}
	s.afterGrantItem(int64(uid), entry.ItemID)
	if entry.GoldBonus > 0 {
		_ = s.cfg.Store.AddGold(int64(uid), entry.GoldBonus*count)
		bal, _ = s.cfg.Store.GetGold(int64(uid))
	}
	out := make([]byte, 12)
	binary.BigEndian.PutUint32(out[0:4], 0)
	binary.BigEndian.PutUint32(out[4:8], uint32(cost*100))
	binary.BigEndian.PutUint32(out[8:12], uint32(bal*100))
	s.send(c, 1104, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d product=%d item=%d x%d cost=%d gold=%d",
		cmdname.Format(1104), uid, productID, entry.ItemID, count, cost, bal)
}

// handleMoneyBuyProduct CMD 1102：请求 productID(4)+count(2)+…；扣金豆（米币×10）发物品。
func (s *Server) handleMoneyBuyProduct(c *Client, uid uint32, body []byte) {
	fail := func(gold int) {
		out := make([]byte, 12)
		binary.BigEndian.PutUint32(out[0:4], 1)
		binary.BigEndian.PutUint32(out[8:12], uint32(gold*100))
		s.send(c, 1102, uid, 0, out)
	}
	if len(body) < 6 || s.cfg.Store == nil || s.cfg.Catalog == nil {
		fail(0)
		return
	}
	productID := binary.BigEndian.Uint32(body[0:4])
	count := int(binary.BigEndian.Uint16(body[4:6]))
	if count <= 0 {
		count = 1
	}
	if count > 99 {
		count = 99
	}
	entry, ok := s.cfg.Catalog.GetMoneyProduct(productID)
	if !ok {
		g, _ := s.cfg.Store.GetGold(int64(uid))
		fail(g)
		log.Printf("[CMD] WARN  %s UID=%d unknown product=%d", cmdname.Format(1102), uid, productID)
		return
	}
	needGold := int(math.Round(entry.Price * float64(count) * 10))
	if s.userIsVip(int64(uid)) && entry.VipFactor >= 0 && entry.VipFactor < 1 {
		needGold = int(math.Round(entry.Price * entry.VipFactor * float64(count) * 10))
	}
	if needGold < 0 {
		needGold = 0
	}
	bal, okSpend, err := s.cfg.Store.TrySpendGold(int64(uid), needGold)
	if err != nil || !okSpend {
		fail(bal)
		log.Printf("[CMD] OK     %s UID=%d fail gold need=%d bal=%d", cmdname.Format(1102), uid, needGold, bal)
		return
	}
	if entry.GoldBonus > 0 {
		_ = s.cfg.Store.AddGold(int64(uid), entry.GoldBonus*count)
	}
	for _, itemID := range entry.ItemIDs {
		if itemID <= 0 || itemID == 5 {
			continue
		}
		if err := s.cfg.Store.AddItem(int64(uid), itemID, count); err != nil {
			log.Printf("[CMD] WARN  %s UID=%d AddItem %d: %v", cmdname.Format(1102), uid, itemID, err)
			continue
		}
		s.afterGrantItem(int64(uid), itemID)
	}
	bal, _ = s.cfg.Store.GetGold(int64(uid))
	payMoney := uint32(math.Round(entry.Price * float64(count) * 100))
	out := make([]byte, 12)
	binary.BigEndian.PutUint32(out[0:4], 0)
	binary.BigEndian.PutUint32(out[4:8], payMoney)
	binary.BigEndian.PutUint32(out[8:12], uint32(bal*100))
	s.send(c, 1102, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d product=%d items=%v costGold=%d gold=%d",
		cmdname.Format(1102), uid, productID, entry.ItemIDs, needGold, bal)
}

// afterGrantItem 可强化装备写入 equips。
func (s *Server) afterGrantItem(uid int64, itemID int) {
	if s.cfg.Store == nil {
		return
	}
	switch itemID {
	case 100266, 100267, 100268:
		_ = s.cfg.Store.EnsureEquip(uid, itemID, 1)
	}
}

// handleItemRepair CMD 2603：本客户端无独立解析；空 ACK。
func (s *Server) handleItemRepair(c *Client, uid uint32) {
	s.send(c, 2603, uid, 0, nil)
	log.Printf("[CMD] OK     %s UID=%d", cmdname.Format(2603), uid)
}

// handleEquipUpdate CMD 2609：请求 sendId(4)；成功应答 result=1。
func (s *Server) handleEquipUpdate(c *Client, uid uint32, body []byte) {
	sendID := uint32(0)
	if len(body) >= 4 {
		sendID = binary.BigEndian.Uint32(body[0:4])
	}
	fail := func() {
		out := make([]byte, 4)
		s.send(c, 2609, uid, 0, out)
	}
	if s.cfg.Store == nil || s.cfg.Catalog == nil || sendID == 0 {
		fail()
		return
	}
	up, ok := s.cfg.Catalog.GetEquipUpgrade(sendID)
	if !ok {
		fail()
		log.Printf("[CMD] WARN  %s UID=%d unknown sendId=%d", cmdname.Format(2609), uid, sendID)
		return
	}
	// 须已拥有该装备
	n, _ := s.cfg.Store.GetItemCount(int64(uid), up.ItemID)
	if n < 1 {
		fail()
		log.Printf("[CMD] OK     %s UID=%d no equip item=%d", cmdname.Format(2609), uid, up.ItemID)
		return
	}
	cur, _ := s.cfg.Store.GetEquipLevel(int64(uid), up.ItemID)
	if cur < 1 {
		cur = 1
	}
	if up.LevelID <= cur {
		fail()
		log.Printf("[CMD] OK     %s UID=%d already lv=%d want=%d", cmdname.Format(2609), uid, cur, up.LevelID)
		return
	}
	// 扣材料
	if up.CatalystID > 0 && up.CatalystNum > 0 {
		if err := s.cfg.Store.ConsumeItem(int64(uid), up.CatalystID, up.CatalystNum); err != nil {
			fail()
			log.Printf("[CMD] OK     %s UID=%d no catalyst: %v", cmdname.Format(2609), uid, err)
			return
		}
	}
	for _, m := range up.Matters {
		if err := s.cfg.Store.ConsumeItem(int64(uid), m[0], m[1]); err != nil {
			fail()
			log.Printf("[CMD] OK     %s UID=%d no matter %d: %v", cmdname.Format(2609), uid, m[0], err)
			return
		}
	}
	if up.Odds < 100 && rand.Intn(100) >= up.Odds {
		fail()
		log.Printf("[CMD] OK     %s UID=%d odds fail item=%d", cmdname.Format(2609), uid, up.ItemID)
		return
	}
	if err := s.cfg.Store.SetEquipLevel(int64(uid), up.ItemID, up.LevelID); err != nil {
		fail()
		log.Printf("[CMD] WARN  %s UID=%d SetEquipLevel: %v", cmdname.Format(2609), uid, err)
		return
	}
	out := make([]byte, 4)
	binary.BigEndian.PutUint32(out, 1)
	s.send(c, 2609, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d item=%d lv=%d", cmdname.Format(2609), uid, up.ItemID, up.LevelID)
}
