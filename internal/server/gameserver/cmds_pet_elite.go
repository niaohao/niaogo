package gameserver

import (
	"encoding/binary"
	"log"

	"niaohao/server/internal/cmdname"
)

// handlePetEliteCollect CMD 2333：本客户端 PET_ELITE_COLLECT — 仓库宠标记精英。
// 请求 catchTime(4)；空 ACK（客户端已本地更新）。
func (s *Server) handlePetEliteCollect(c *Client, uid uint32, body []byte) {
	catch := uint32(0)
	if len(body) >= 4 {
		catch = binary.BigEndian.Uint32(body[0:4])
	}
	if s.cfg.Store != nil && catch > 0 {
		p, err := s.cfg.Store.GetPetByCatchTime(int64(uid), int64(catch))
		if err == nil && p != nil && !p.InBag {
			_ = s.cfg.Store.SetPetElite(int64(uid), int64(catch), true)
		}
	}
	s.send(c, 2333, uid, 0, nil)
	log.Printf("[CMD] OK     %s UID=%d catch=%d", cmdname.Format(2333), uid, catch)
}

// handlePetEliteUncollect CMD 2334：本客户端 PET_ELITE_UNCOLLECT — 取消精英标记。
func (s *Server) handlePetEliteUncollect(c *Client, uid uint32, body []byte) {
	catch := uint32(0)
	if len(body) >= 4 {
		catch = binary.BigEndian.Uint32(body[0:4])
	}
	if s.cfg.Store != nil && catch > 0 {
		_ = s.cfg.Store.SetPetElite(int64(uid), int64(catch), false)
	}
	s.send(c, 2334, uid, 0, nil)
	log.Printf("[CMD] OK     %s UID=%d catch=%d", cmdname.Format(2334), uid, catch)
}
