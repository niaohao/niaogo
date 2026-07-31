package gameserver

import (
	"encoding/binary"
	"log"
)

// OnlinePlayer GM 在线列表项。
type OnlinePlayer struct {
	UserID   int64  `json:"userId"`
	MapID    int    `json:"mapId"`
	Remote   string `json:"remote"`
	InBattle bool   `json:"inBattle"`
}

// ListOnlinePlayers 当前已登录连接。
func (s *Server) ListOnlinePlayers() []OnlinePlayer {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]OnlinePlayer, 0, len(s.byUID))
	for uid, c := range s.byUID {
		if c == nil || !c.LoggedIn {
			continue
		}
		remote := ""
		if c.Conn != nil {
			remote = c.Conn.RemoteAddr().String()
		}
		out = append(out, OnlinePlayer{
			UserID:   uid,
			MapID:    c.MapID,
			Remote:   remote,
			InBattle: s.battles.get(uid) != nil,
		})
	}
	return out
}

// KickUser 踢下线（断游戏 TCP）。
func (s *Server) KickUser(uid int64) bool {
	s.mu.Lock()
	c := s.byUID[uid]
	s.mu.Unlock()
	if c == nil {
		return false
	}
	s.ForceDisconnect(uid)
	return true
}

// PushItemGain 在线推 8004 道具获得（GM/邮件发奖共用）。
func (s *Server) PushItemGain(uid int64, itemID, count int) {
	if uid <= 0 || itemID <= 0 || count <= 0 {
		return
	}
	s.sendToUser(uid, 8004, buildBossMonster8004Body(0, 0, 0, uint32(itemID), uint32(count)))
}

// PushPetGain 在线推 8004 精灵获得。
func (s *Server) PushPetGain(uid int64, petID int, catchTime int64) {
	if uid <= 0 || petID <= 0 {
		return
	}
	s.sendToUser(uid, 8004, buildBossMonster8004Body(0, uint32(petID), uint32(catchTime), 0, 0))
}

// PushPetRefresh GM 改宠后热推：2508 NOTE_UPDATE_PROP + 2301 GET_PET_INFO。
func (s *Server) PushPetRefresh(uid, catchTime int64) {
	if uid <= 0 || catchTime <= 0 || s.cfg.Store == nil {
		return
	}
	p, err := s.cfg.Store.GetPetByCatchTime(uid, catchTime)
	if err != nil || p == nil {
		return
	}
	_, lv, _, hp, atk, def, sa, sd, spd := petCombatStats(p)
	prop := buildNoteUpdateProp(uint32(catchTime), p.PetID, lv, p.Exp, p.Exp, petNextLevelExp(p.PetID, lv), hp, atk, def, sa, sd, spd, p.EV)
	s.sendToUser(uid, 2508, prop)
	if info := buildPetInfo(p); len(info) > 0 {
		s.sendToUser(uid, 2301, info)
	}
	log.Printf("[GM] PushPetRefresh UID=%d catch=%d pet=%d ev=%v", uid, catchTime, p.PetID, p.EV)
}

// PushCurrencyBalance 推 1106 金豆*100 + 赛尔豆。
func (s *Server) PushCurrencyBalance(uid int64) {
	if uid <= 0 || s.cfg.Store == nil {
		return
	}
	gold, coins := 0, 0
	if u, err := s.cfg.Store.FindByUserID(uid); err == nil && u != nil {
		gold, coins = u.Gold, u.Coins
	}
	body := make([]byte, 8)
	binary.BigEndian.PutUint32(body[0:4], uint32(gold*100))
	binary.BigEndian.PutUint32(body[4:8], uint32(coins))
	s.sendToUser(uid, 1106, body)
}
