package gameserver

import (
	"encoding/binary"
	"log"

	"niaohao/server/internal/cmdname"
)

// handleItemBuy CMD 2601：购买。请求 itemID+count；应答 cash+itemID+num+level。
func (s *Server) handleItemBuy(c *Client, uid uint32, body []byte) {
	itemID, count := uint32(0), uint32(1)
	if len(body) >= 4 {
		itemID = binary.BigEndian.Uint32(body[0:4])
	}
	if len(body) >= 8 {
		count = binary.BigEndian.Uint32(body[4:8])
	}
	if count == 0 {
		count = 1
	}
	if count > 99 {
		count = 99
	}
	if s.cfg.Store == nil || itemID == 0 {
		s.send(c, 2601, uid, 0, nil)
		return
	}
	unit := 100
	if s.cfg.Catalog != nil {
		unit = s.cfg.Catalog.ItemBuyPrice(int(itemID))
	}
	cost := unit * int(count)
	bal, ok, err := s.cfg.Store.TrySpendCoins(int64(uid), cost)
	if err != nil || !ok {
		s.send(c, 2601, uid, 0, nil)
		log.Printf("[CMD] OK     %s UID=%d item=%d fail coins cost=%d bal=%d err=%v",
			cmdname.Format(2601), uid, itemID, cost, bal, err)
		return
	}
	if err := s.cfg.Store.AddItem(int64(uid), int(itemID), int(count)); err != nil {
		_ = s.cfg.Store.AddCoins(int64(uid), cost) // 回滚扣款
		s.send(c, 2601, uid, 0, nil)
		log.Printf("[CMD] WARN  %s UID=%d AddItem: %v", cmdname.Format(2601), uid, err)
		return
	}
	s.afterGrantItem(int64(uid), int(itemID))
	lv := uint32(0)
	if el, _ := s.cfg.Store.GetEquipLevel(int64(uid), int(itemID)); el > 0 {
		lv = uint32(el)
	}
	out := make([]byte, 16)
	binary.BigEndian.PutUint32(out[0:4], uint32(bal))
	binary.BigEndian.PutUint32(out[4:8], itemID)
	binary.BigEndian.PutUint32(out[8:12], count)
	binary.BigEndian.PutUint32(out[12:16], lv)
	s.send(c, 2601, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d item=%d x%d cost=%d cash=%d",
		cmdname.Format(2601), uid, itemID, count, cost, bal)
}

// handleItemSale CMD 2602：出售。请求 itemID+count；应答可空。
func (s *Server) handleItemSale(c *Client, uid uint32, body []byte) {
	itemID, count := uint32(0), uint32(1)
	if len(body) >= 4 {
		itemID = binary.BigEndian.Uint32(body[0:4])
	}
	if len(body) >= 8 {
		count = binary.BigEndian.Uint32(body[4:8])
	}
	if count == 0 {
		count = 1
	}
	refund := 0
	if s.cfg.Store != nil && itemID > 0 {
		if err := s.cfg.Store.ConsumeItem(int64(uid), int(itemID), int(count)); err != nil {
			log.Printf("[CMD] WARN  %s UID=%d consume: %v", cmdname.Format(2602), uid, err)
		} else {
			unit := 50
			if s.cfg.Catalog != nil {
				unit = s.cfg.Catalog.ItemBuyPrice(int(itemID)) / 2
				if unit < 1 {
					unit = 1
				}
			}
			refund = unit * int(count)
			_ = s.cfg.Store.AddCoins(int64(uid), refund)
		}
	}
	s.send(c, 2602, uid, 0, nil)
	log.Printf("[CMD] OK     %s UID=%d item=%d x%d refund=%d",
		cmdname.Format(2602), uid, itemID, count, refund)
}

// handleItemExpend CMD 2607：消耗物品。请求 itemID+count；应答空。
func (s *Server) handleItemExpend(c *Client, uid uint32, body []byte) {
	itemID, count := uint32(0), uint32(1)
	if len(body) >= 4 {
		itemID = binary.BigEndian.Uint32(body[0:4])
	}
	if len(body) >= 8 {
		count = binary.BigEndian.Uint32(body[4:8])
	}
	if count == 0 {
		count = 1
	}
	ok := false
	if s.cfg.Store != nil && itemID > 0 {
		if err := s.cfg.Store.ConsumeItem(int64(uid), int(itemID), int(count)); err != nil {
			log.Printf("[CMD] WARN  %s UID=%d: %v", cmdname.Format(2607), uid, err)
		} else {
			ok = true
		}
	}
	s.send(c, 2607, uid, 0, nil)
	log.Printf("[CMD] OK     %s UID=%d item=%d x%d ok=%v",
		cmdname.Format(2607), uid, itemID, count, ok)
}

// handleMultiItemBuy CMD 2606：批量购买。请求 count+N×itemID；应答 cash(4)（BuyMultiItemInfo）。
func (s *Server) handleMultiItemBuy(c *Client, uid uint32, body []byte) {
	n := uint32(0)
	if len(body) >= 4 {
		n = binary.BigEndian.Uint32(body[0:4])
	}
	if n > 32 {
		n = 32
	}
	ids := make([]uint32, 0, n)
	for i := uint32(0); i < n; i++ {
		off := 4 + int(i)*4
		if len(body) < off+4 {
			break
		}
		id := binary.BigEndian.Uint32(body[off : off+4])
		if id > 0 {
			ids = append(ids, id)
		}
	}
	cash := 0
	if s.cfg.Store != nil {
		var err error
		cash, err = s.cfg.Store.GetCoins(int64(uid))
		if err != nil {
			cash = 0
		}
	}
	if s.cfg.Store == nil || len(ids) == 0 {
		out := make([]byte, 4)
		binary.BigEndian.PutUint32(out, uint32(cash))
		s.send(c, 2606, uid, 0, out)
		return
	}
	cost := 0
	for _, id := range ids {
		unit := 100
		if s.cfg.Catalog != nil {
			unit = s.cfg.Catalog.ItemBuyPrice(int(id))
		}
		cost += unit
	}
	bal, ok, err := s.cfg.Store.TrySpendCoins(int64(uid), cost)
	if err != nil || !ok {
		out := make([]byte, 4)
		binary.BigEndian.PutUint32(out, uint32(cash))
		s.send(c, 2606, uid, 0, out)
		log.Printf("[CMD] OK     %s UID=%d fail coins cost=%d bal=%d err=%v",
			cmdname.Format(2606), uid, cost, cash, err)
		return
	}
	for _, id := range ids {
		if err := s.cfg.Store.AddItem(int64(uid), int(id), 1); err != nil {
			log.Printf("[CMD] WARN  %s UID=%d AddItem %d: %v", cmdname.Format(2606), uid, id, err)
		} else {
			s.afterGrantItem(int64(uid), int(id))
		}
	}
	out := make([]byte, 4)
	binary.BigEndian.PutUint32(out, uint32(bal))
	s.send(c, 2606, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d n=%d cost=%d cash=%d",
		cmdname.Format(2606), uid, len(ids), cost, bal)
}

// handleEatSpecialMedicine CMD 2610：特药/能量珠。请求 catchTime+itemID。
// 应答 EatSpecialMedicineInfo：catchTime；若非 0 则 +effectID(u16)+leftCount(u8)。
func (s *Server) handleEatSpecialMedicine(c *Client, uid uint32, body []byte) {
	catch, itemID := uint32(0), uint32(0)
	if len(body) >= 8 {
		catch = binary.BigEndian.Uint32(body[0:4])
		itemID = binary.BigEndian.Uint32(body[4:8])
	}
	fail := func() {
		s.send(c, 2610, uid, 0, make([]byte, 4)) // catchTime=0
	}
	if s.cfg.Store == nil || catch == 0 || itemID == 0 {
		fail()
		log.Printf("[CMD] OK     %s UID=%d (empty)", cmdname.Format(2610), uid)
		return
	}
	p, err := s.cfg.Store.GetPetByCatchTime(int64(uid), int64(catch))
	if err != nil || p == nil {
		fail()
		log.Printf("[CMD] OK     %s UID=%d catch=%d miss", cmdname.Format(2610), uid, catch)
		return
	}

	effID, left, isBall := 0, 0, false
	if s.cfg.Catalog != nil {
		effID, left, isBall = s.cfg.Catalog.EnergyBallForItem(int(itemID))
	}
	// 已有未用完能量珠时不叠用、不扣道具
	if isBall && p.EnergyBallItemID > 0 && p.EnergyBallLeftCount > 0 {
		out := buildEatSpecialMedicineBody(catch, uint16(p.EnergyBallEffectID), byte(p.EnergyBallLeftCount))
		s.send(c, 2610, uid, 0, out)
		log.Printf("[CMD] OK     %s UID=%d catch=%d already ball item=%d left=%d",
			cmdname.Format(2610), uid, catch, p.EnergyBallItemID, p.EnergyBallLeftCount)
		return
	}

	if err := s.cfg.Store.ConsumeItem(int64(uid), int(itemID), 1); err != nil {
		fail()
		log.Printf("[CMD] OK     %s UID=%d item=%d no stock: %v", cmdname.Format(2610), uid, itemID, err)
		return
	}

	if isBall {
		if left <= 0 {
			left = 10
		}
		if err := s.cfg.Store.SetPetEnergyBall(int64(uid), int64(catch), int(itemID), left, effID); err != nil {
			log.Printf("[CMD] WARN  %s UID=%d SetPetEnergyBall: %v", cmdname.Format(2610), uid, err)
		} else {
			p.EnergyBallItemID = int(itemID)
			p.EnergyBallLeftCount = left
			p.EnergyBallEffectID = effID
		}
	}

	out := buildEatSpecialMedicineBody(catch, uint16(effID), byte(left))
	s.send(c, 2610, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d catch=%d item=%d ball=%v eff=%d left=%d",
		cmdname.Format(2610), uid, catch, itemID, isBall, effID, left)
}

func buildEatSpecialMedicineBody(catch uint32, effectID uint16, leftCount byte) []byte {
	if catch == 0 {
		return make([]byte, 4)
	}
	out := make([]byte, 7)
	binary.BigEndian.PutUint32(out[0:4], catch)
	binary.BigEndian.PutUint16(out[4:6], effectID)
	out[6] = leftCount
	return out
}
