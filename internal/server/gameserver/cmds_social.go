package gameserver

import (
	"encoding/binary"
	"log"

	"niaohao/server/internal/cmdname"
)

// handleChat CMD 2102：请求 toID+msgLen+msg；应答 ChatInfo，并广播/私聊推送。
func (s *Server) handleChat(c *Client, uid uint32, body []byte) {
	toID, msgLen := uint32(0), uint32(0)
	var msg []byte
	if len(body) >= 8 {
		toID = binary.BigEndian.Uint32(body[0:4])
		msgLen = binary.BigEndian.Uint32(body[4:8])
		if msgLen > 0 && len(body) >= 8+int(msgLen) {
			msg = append([]byte(nil), body[8:8+int(msgLen)]...)
		} else if len(body) > 8 {
			msg = append([]byte(nil), body[8:]...)
		}
	}
	// ChatAction 末尾写 "0" 占位，回显去掉
	if len(msg) > 0 && msg[len(msg)-1] == '0' {
		msg = msg[:len(msg)-1]
	}
	nick := s.nickOf(uid)
	out := buildChatInfo(uid, nick, toID, msg)
	s.send(c, 2102, uid, 0, out)
	if toID == 0 {
		s.broadcastToMap(c, 2102, out)
	} else if toID != uid {
		s.sendToUser(int64(toID), 2102, out)
	}
	log.Printf("[CMD] OK     %s UID=%d to=%d len=%d", cmdname.Format(2102), uid, toID, len(msg))
}

func buildChatInfo(sender uint32, nick string, toID uint32, msg []byte) []byte {
	var buf []byte
	tmp := make([]byte, 4)
	binary.BigEndian.PutUint32(tmp, sender)
	buf = append(buf, tmp...)
	nb := make([]byte, 16)
	copy(nb, []byte(nick))
	buf = append(buf, nb...)
	binary.BigEndian.PutUint32(tmp, toID)
	buf = append(buf, tmp...)
	binary.BigEndian.PutUint32(tmp, uint32(len(msg)))
	buf = append(buf, tmp...)
	buf = append(buf, msg...)
	return buf
}

func (s *Server) nickOf(uid uint32) string {
	if s.cfg.Store != nil {
		if u, err := s.cfg.Store.FindByUserID(int64(uid)); err == nil && u != nil && u.Nickname != "" {
			return u.Nickname
		}
	}
	return "赛尔"
}

// handleChangeCloth CMD 2604：请求 count+N×clothID；应答 userID+count+(id+lv)×N；落库 user_worn_clothes。
func (s *Server) handleChangeCloth(c *Client, uid uint32, body []byte) {
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
		ids = append(ids, binary.BigEndian.Uint32(body[off:off+4]))
	}
	c.ClothIDs = ids
	if s.cfg.Store != nil {
		_ = s.cfg.Store.SetWornClothes(int64(uid), clothIDsToStore(ids))
	}
	out := buildChangeClothInfo(uid, ids)
	s.send(c, 2604, uid, 0, out)
	s.broadcastToMap(c, 2604, out)
	log.Printf("[CMD] OK     %s UID=%d n=%d", cmdname.Format(2604), uid, len(ids))
}

func buildChangeClothInfo(uid uint32, ids []uint32) []byte {
	out := make([]byte, 8+len(ids)*8)
	binary.BigEndian.PutUint32(out[0:4], uid)
	binary.BigEndian.PutUint32(out[4:8], uint32(len(ids)))
	off := 8
	for _, id := range ids {
		binary.BigEndian.PutUint32(out[off:off+4], id)
		binary.BigEndian.PutUint32(out[off+4:off+8], 0) // level
		off += 8
	}
	return out
}

// handleFriendAdd CMD 2151：申请好友；ACK + 向目标推 INFORM(8001)。
func (s *Server) handleFriendAdd(c *Client, uid uint32, body []byte) {
	target := uint32(0)
	if len(body) >= 4 {
		target = binary.BigEndian.Uint32(body[0:4])
	}
	out := make([]byte, 4)
	binary.BigEndian.PutUint32(out, target)
	if target == 0 || target == uid {
		s.send(c, 2151, uid, 0, out)
		log.Printf("[CMD] OK     %s UID=%d target=%d (skip)", cmdname.Format(2151), uid, target)
		return
	}
	if s.cfg.Store != nil {
		if ok, _ := s.cfg.Store.IsFriend(int64(uid), int64(target)); ok {
			s.send(c, 2151, uid, 0, out)
			log.Printf("[CMD] OK     %s UID=%d target=%d (already friend)", cmdname.Format(2151), uid, target)
			return
		}
	}
	s.send(c, 2151, uid, 0, out)
	s.pushFriendAddInform(uid, s.nickOf(uid), int64(target))
	log.Printf("[CMD] OK     %s UID=%d target=%d", cmdname.Format(2151), uid, target)
}

// handleFriendAnswer CMD 2152：targetID(4)+accept(4)；接受则双向落库并推 8001 给请求方。
func (s *Server) handleFriendAnswer(c *Client, uid uint32, body []byte) {
	target := uint32(0)
	accept := uint32(0)
	if len(body) >= 4 {
		target = binary.BigEndian.Uint32(body[0:4])
	}
	if len(body) >= 8 {
		accept = binary.BigEndian.Uint32(body[4:8])
	}
	if accept == 1 && target > 0 && target != uid && s.cfg.Store != nil {
		_ = s.cfg.Store.AddFriend(int64(uid), int64(target))
		s.pushInform(int64(target), 2152, uid, s.nickOf(uid), 1)
	}
	out := make([]byte, 4)
	binary.BigEndian.PutUint32(out, accept)
	s.send(c, 2152, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d target=%d accept=%d", cmdname.Format(2152), uid, target, accept)
}

// handleFriendRemove CMD 2153：friendID(4)；双向删好友。
func (s *Server) handleFriendRemove(c *Client, uid uint32, body []byte) {
	target := uint32(0)
	if len(body) >= 4 {
		target = binary.BigEndian.Uint32(body[0:4])
	}
	if target > 0 && s.cfg.Store != nil {
		_ = s.cfg.Store.RemoveFriend(int64(uid), int64(target))
	}
	s.send(c, 2153, uid, 0, nil)
	log.Printf("[CMD] OK     %s UID=%d target=%d", cmdname.Format(2153), uid, target)
}

// handleBlackAdd CMD 2154：应答 userID(4)；落库并移除好友。
func (s *Server) handleBlackAdd(c *Client, uid uint32, body []byte) {
	target := uint32(0)
	if len(body) >= 4 {
		target = binary.BigEndian.Uint32(body[0:4])
	}
	if target > 0 && target != uid && s.cfg.Store != nil {
		_ = s.cfg.Store.AddBlacklist(int64(uid), int64(target))
	}
	out := make([]byte, 4)
	binary.BigEndian.PutUint32(out, target)
	s.send(c, 2154, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d target=%d", cmdname.Format(2154), uid, target)
}

// handleBlackRemove CMD 2155：targetID(4)。
func (s *Server) handleBlackRemove(c *Client, uid uint32, body []byte) {
	target := uint32(0)
	if len(body) >= 4 {
		target = binary.BigEndian.Uint32(body[0:4])
	}
	if target > 0 && s.cfg.Store != nil {
		_ = s.cfg.Store.RemoveBlacklist(int64(uid), int64(target))
	}
	s.send(c, 2155, uid, 0, nil)
	log.Printf("[CMD] OK     %s UID=%d target=%d", cmdname.Format(2155), uid, target)
}

// handleSeeOnline CMD 2157：请求 count+ids；应答 count + N×OnLineInfo(16B)。
func (s *Server) handleSeeOnline(c *Client, uid uint32, body []byte) {
	n := uint32(0)
	if len(body) >= 4 {
		n = binary.BigEndian.Uint32(body[0:4])
	}
	if n > 64 {
		n = 64
	}
	var out []byte
	tmp := make([]byte, 4)
	binary.BigEndian.PutUint32(tmp, 0) // placeholder count
	out = append(out, tmp...)
	count := uint32(0)
	for i := uint32(0); i < n; i++ {
		off := 4 + int(i)*4
		if len(body) < off+4 {
			break
		}
		id := binary.BigEndian.Uint32(body[off : off+4])
		online, mapID := s.onlineMapOf(int64(id))
		if !online {
			continue
		}
		entry := make([]byte, 16)
		binary.BigEndian.PutUint32(entry[0:4], id)
		binary.BigEndian.PutUint32(entry[4:8], 1) // serverID stub
		binary.BigEndian.PutUint32(entry[8:12], 0)
		binary.BigEndian.PutUint32(entry[12:16], uint32(mapID))
		out = append(out, entry...)
		count++
	}
	binary.BigEndian.PutUint32(out[0:4], count)
	s.send(c, 2157, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d online=%d/%d", cmdname.Format(2157), uid, count, n)
}

func (s *Server) onlineMapOf(uid int64) (bool, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.byUID[uid]
	if c == nil || !c.LoggedIn {
		return false, 0
	}
	return true, c.MapID
}

// handleRequestOut CMD 2158 / handleRequestAnswer CMD 2159：空 ACK。
func (s *Server) handleRequestOut(c *Client, uid uint32, body []byte) {
	s.send(c, 2158, uid, 0, nil)
	log.Printf("[CMD] OK     %s UID=%d", cmdname.Format(2158), uid)
}

func (s *Server) handleRequestAnswer(c *Client, uid uint32, body []byte) {
	s.send(c, 2159, uid, 0, nil)
	log.Printf("[CMD] OK     %s UID=%d", cmdname.Format(2159), uid)
}

// handleDeleteTask CMD 2205 / 2232：删任务记录，空 ACK。
func (s *Server) handleDeleteTask(c *Client, uid uint32, body []byte, cmd int32) {
	taskID := uint32(0)
	if len(body) >= 4 {
		taskID = binary.BigEndian.Uint32(body[0:4])
	}
	if s.cfg.Store != nil && taskID > 0 {
		_ = s.cfg.Store.DeleteTask(int64(uid), int(taskID))
	}
	s.send(c, cmd, uid, 0, nil)
	log.Printf("[CMD] OK     %s UID=%d task=%d", cmdname.Format(cmd), uid, taskID)
}