package gameserver

import (
	"encoding/binary"
	"log"

	"niaohao/server/internal/cmdname"
)

// handleUsePetItemFullAbilityOfStudy CMD 9278：学习力注入。
// 请求：catchTime(4)+statIndex(4)+itemID(4)+flag(4)
// statIndex：0HP 1Atk 2Def 3SA 4SD 5Spd；成功把该项 EV 设为 255 并裁剪总和≤510。
// 应答：u32(0)；再推 2508 + 2301。
func (s *Server) handleUsePetItemFullAbilityOfStudy(c *Client, uid uint32, body []byte) {
	catch, statIdx, itemID := uint32(0), uint32(0), uint32(0)
	if len(body) >= 12 {
		catch = binary.BigEndian.Uint32(body[0:4])
		statIdx = binary.BigEndian.Uint32(body[4:8])
		itemID = binary.BigEndian.Uint32(body[8:12])
	}
	ack := func() {
		s.send(c, 9278, uid, 0, make([]byte, 4))
	}
	if s.cfg.Store == nil || catch == 0 || itemID == 0 {
		ack()
		log.Printf("[CMD] OK     %s UID=%d (empty)", cmdname.Format(9278), uid)
		return
	}
	p, err := s.cfg.Store.GetPetByCatchTime(int64(uid), int64(catch))
	if err != nil || p == nil {
		ack()
		log.Printf("[CMD] OK     %s UID=%d catch=%d miss", cmdname.Format(9278), uid, catch)
		return
	}
	if err := s.cfg.Store.ConsumeItem(int64(uid), int(itemID), 1); err != nil {
		ack()
		log.Printf("[CMD] OK     %s UID=%d item=%d no stock: %v", cmdname.Format(9278), uid, itemID, err)
		return
	}
	if statIdx <= 5 {
		p.EV[statIdx] = 255
		clampPetEV(&p.EV)
		if err := s.cfg.Store.SetPetEV(int64(uid), int64(catch), p.EV); err != nil {
			log.Printf("[CMD] WARN  %s UID=%d SetPetEV: %v", cmdname.Format(9278), uid, err)
		}
	}
	ack()

	_, lv, _, hp, atk, def, sa, sd, spd := petCombatStats(p)
	prop := buildNoteUpdateProp(catch, p.PetID, lv, p.Exp, p.Exp, petNextLevelExp(p.PetID, lv), hp, atk, def, sa, sd, spd, p.EV)
	s.send(c, 2508, uid, 0, prop)
	if info := buildPetInfo(p); len(info) > 0 {
		s.send(c, 2301, uid, 0, info)
	}
	log.Printf("[CMD] OK     %s UID=%d catch=%d item=%d stat=%d ev=%v",
		cmdname.Format(9278), uid, catch, itemID, statIdx, p.EV)
}
