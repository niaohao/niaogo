package gameserver

import (
	"encoding/binary"
	"log"

	"niaohao/server/internal/cmdname"
)

// 全能性格转化剂：本客户端 PetPropClass_300136 发包第三字段恒为 1（非 itemId）。
var natureChooseItems = []int{300136, 300649}

// handlePetResetNature CMD 2343：
// - body>=12：PET_RESET_NATURE = catchTime(4)+nature(4)+flag(4)；空 ACK，再推 2301
// - 否则：学习力仪/加速器次数 20B（本客户端偶发查询）
func (s *Server) handlePetResetNature(c *Client, uid uint32, body []byte) {
	if len(body) < 12 {
		out := buildBoostTimesBody(s.boostTimesOf(int64(uid)))
		s.send(c, 2343, uid, 0, out)
		log.Printf("[CMD] OK     %s UID=%d (boost times)", cmdname.Format(2343), uid)
		return
	}
	catch := binary.BigEndian.Uint32(body[0:4])
	nature := binary.BigEndian.Uint32(body[4:8])
	flag := binary.BigEndian.Uint32(body[8:12])

	ack := func() { s.send(c, 2343, uid, 0, nil) }

	if nature > 24 || s.cfg.Store == nil || catch == 0 {
		ack()
		log.Printf("[CMD] OK     %s UID=%d reject nature=%d catch=%d", cmdname.Format(2343), uid, nature, catch)
		return
	}
	p, err := s.cfg.Store.GetPetByCatchTime(int64(uid), int64(catch))
	if err != nil || p == nil {
		ack()
		log.Printf("[CMD] OK     %s UID=%d catch=%d miss", cmdname.Format(2343), uid, catch)
		return
	}

	itemID := 0
	for _, id := range natureChooseItems {
		n, _ := s.cfg.Store.GetItemCount(int64(uid), id)
		if n > 0 {
			itemID = id
			break
		}
	}
	// flag==1 为 300136 面板约定；无库存则拒绝
	if itemID == 0 {
		ack()
		s.sendAlert(int64(uid), "没有可用的全能性格转化剂")
		log.Printf("[CMD] OK     %s UID=%d no item flag=%d", cmdname.Format(2343), uid, flag)
		return
	}
	if err := s.cfg.Store.ConsumeItem(int64(uid), itemID, 1); err != nil {
		ack()
		log.Printf("[CMD] OK     %s UID=%d consume %d: %v", cmdname.Format(2343), uid, itemID, err)
		return
	}

	old := p.Nature
	p.Nature = int(nature)
	if err := s.cfg.Store.UpsertPet(p); err != nil {
		ack()
		log.Printf("[CMD] WARN  %s UID=%d UpsertPet: %v", cmdname.Format(2343), uid, err)
		return
	}
	ack()
	if info := buildPetInfo(p); len(info) > 0 {
		s.send(c, 2301, uid, 0, info)
	}
	log.Printf("[CMD] OK     %s UID=%d catch=%d item=%d nature=%d->%d",
		cmdname.Format(2343), uid, catch, itemID, old, nature)
}
