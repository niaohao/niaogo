package gameserver

import (
	"bytes"
	"encoding/binary"
	"log"
	"strings"

	"niaohao/server/internal/cmdname"
)

// handleChangeNickName CMD 2061：请求 nick16；应答 ChangeUserNameInfo=userId+nick16；并广播同图。
func (s *Server) handleChangeNickName(c *Client, uid uint32, body []byte) {
	nick := ""
	if len(body) >= 16 {
		nick = string(bytes.TrimRight(body[:16], "\x00"))
	} else if len(body) > 0 {
		nick = string(bytes.TrimRight(body, "\x00"))
	}
	nick = strings.TrimSpace(nick)
	if nick == "" {
		nick = s.nickOf(uid)
	}
	if s.cfg.Store != nil {
		if u, err := s.cfg.Store.FindByUserID(int64(uid)); err == nil && u != nil {
			u.Nickname = nick
			_ = s.cfg.Store.SaveUser(u)
		}
	}
	out := make([]byte, 20)
	binary.BigEndian.PutUint32(out[0:4], uid)
	putFixedNick(out, 4, nick)
	s.send(c, 2061, uid, 0, out)
	s.broadcastToMap(c, 2061, out)
	log.Printf("[CMD] OK     %s UID=%d nick=%q", cmdname.Format(2061), uid, nick)
}

// handleChangeDoodle CMD 2062：请求 doodleItemId；应答 DoodleInfo=userID+color+texture+coins。
func (s *Server) handleChangeDoodle(c *Client, uid uint32, body []byte) {
	texture := uint32(0)
	if len(body) >= 4 {
		texture = binary.BigEndian.Uint32(body[0:4])
	}
	color, coins := uint32(0), uint32(0)
	if s.cfg.Store != nil {
		if u, err := s.cfg.Store.FindByUserID(int64(uid)); err == nil && u != nil {
			color = uint32(u.Color)
			coins = uint32(u.Coins)
		}
	}
	out := make([]byte, 16)
	binary.BigEndian.PutUint32(out[0:4], uid)
	binary.BigEndian.PutUint32(out[4:8], color)
	binary.BigEndian.PutUint32(out[8:12], texture)
	binary.BigEndian.PutUint32(out[12:16], coins)
	s.send(c, 2062, uid, 0, out)
	s.broadcastToMap(c, 2062, out)
	log.Printf("[CMD] OK     %s UID=%d texture=%d", cmdname.Format(2062), uid, texture)
}

// handleChangeColor CMD 2063：请求 color；应答 userId+color+cost+coins。
func (s *Server) handleChangeColor(c *Client, uid uint32, body []byte) {
	color := uint32(0)
	if len(body) >= 4 {
		color = binary.BigEndian.Uint32(body[0:4])
	}
	coins := uint32(0)
	if s.cfg.Store != nil {
		if u, err := s.cfg.Store.FindByUserID(int64(uid)); err == nil && u != nil {
			u.Color = int(color)
			_ = s.cfg.Store.SaveUser(u)
			coins = uint32(u.Coins)
		}
	}
	out := make([]byte, 16)
	binary.BigEndian.PutUint32(out[0:4], uid)
	binary.BigEndian.PutUint32(out[4:8], color)
	binary.BigEndian.PutUint32(out[8:12], 0)
	binary.BigEndian.PutUint32(out[12:16], coins)
	s.send(c, 2063, uid, 0, out)
	s.broadcastToMap(c, 2063, out)
	log.Printf("[CMD] OK     %s UID=%d color=%d", cmdname.Format(2063), uid, color)
}

// handleDanceAction CMD 2103：请求 aid+atype；应答 uid+aid+atype；广播同图。
func (s *Server) handleDanceAction(c *Client, uid uint32, body []byte) {
	aid, atype := uint32(0), uint32(0)
	if len(body) >= 8 {
		aid = binary.BigEndian.Uint32(body[0:4])
		atype = binary.BigEndian.Uint32(body[4:8])
	} else if len(body) >= 4 {
		aid = binary.BigEndian.Uint32(body[0:4])
	}
	out := make([]byte, 12)
	binary.BigEndian.PutUint32(out[0:4], uid)
	binary.BigEndian.PutUint32(out[4:8], aid)
	binary.BigEndian.PutUint32(out[8:12], atype)
	s.send(c, 2103, uid, 0, out)
	s.broadcastToMap(c, 2103, out)
	log.Printf("[CMD] OK     %s UID=%d aid=%d atype=%d", cmdname.Format(2103), uid, aid, atype)
}

// handlePeopleTransform CMD 2111：请求 suitID；应答 TransformInfo=uid+suitID；广播。
func (s *Server) handlePeopleTransform(c *Client, uid uint32, body []byte) {
	suit := uint32(0)
	if len(body) >= 4 {
		suit = binary.BigEndian.Uint32(body[0:4])
	}
	out := make([]byte, 8)
	binary.BigEndian.PutUint32(out[0:4], uid)
	binary.BigEndian.PutUint32(out[4:8], suit)
	s.send(c, 2111, uid, 0, out)
	s.broadcastToMap(c, 2111, out)
	log.Printf("[CMD] OK     %s UID=%d suit=%d", cmdname.Format(2111), uid, suit)
}

// handleRemoveCoins CMD 2113：请求 coins；应答 remain。
func (s *Server) handleRemoveCoins(c *Client, uid uint32, body []byte) {
	need := 0
	if len(body) >= 4 {
		need = int(binary.BigEndian.Uint32(body[0:4]))
	}
	remain := 0
	if s.cfg.Store != nil {
		if need > 0 {
			if bal, ok, err := s.cfg.Store.TrySpendCoins(int64(uid), need); err == nil && ok {
				remain = bal
			} else if u, e := s.cfg.Store.FindByUserID(int64(uid)); e == nil && u != nil {
				remain = u.Coins
			}
		} else if u, e := s.cfg.Store.FindByUserID(int64(uid)); e == nil && u != nil {
			remain = u.Coins
		}
	}
	out := make([]byte, 4)
	binary.BigEndian.PutUint32(out, uint32(remain))
	s.send(c, 2113, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d spend=%d remain=%d", cmdname.Format(2113), uid, need, remain)
}

// handleModifyPetName CMD 2302：请求 catchTime+name；应答 ret+catchTime。
func (s *Server) handleModifyPetName(c *Client, uid uint32, body []byte) {
	ack := make([]byte, 8)
	if len(body) < 4 || s.cfg.Store == nil {
		binary.BigEndian.PutUint32(ack[0:4], 1)
		s.send(c, 2302, uid, 0, ack)
		return
	}
	catch := int64(binary.BigEndian.Uint32(body[0:4]))
	name := strings.TrimSpace(strings.TrimRight(string(body[4:]), "\x00"))
	if name == "" {
		name = "精灵"
	}
	ret := uint32(1)
	if p, err := s.cfg.Store.GetPetByCatchTime(int64(uid), catch); err == nil && p != nil {
		p.Name = name
		if err := s.cfg.Store.UpsertPet(p); err == nil {
			ret = 0
		}
	}
	binary.BigEndian.PutUint32(ack[0:4], ret)
	binary.BigEndian.PutUint32(ack[4:8], uint32(catch))
	s.send(c, 2302, uid, 0, ack)
	log.Printf("[CMD] OK     %s UID=%d catch=%d ret=%d name=%q", cmdname.Format(2302), uid, catch, ret, name)
}

// handleGetImageAddress CMD 1005：ip16+port2+session16。
func (s *Server) handleGetImageAddress(c *Client, uid uint32, body []byte) {
	host := s.cfg.AdvertiseHost
	if host == "" {
		host = "127.0.0.1"
	}
	out := make([]byte, 34)
	hb := []byte(host)
	if len(hb) > 16 {
		hb = hb[:16]
	}
	copy(out[0:16], hb)
	binary.BigEndian.PutUint16(out[16:18], 80)
	s.send(c, 1005, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d host=%s", cmdname.Format(1005), uid, host)
}
