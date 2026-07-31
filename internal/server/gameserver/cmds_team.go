package gameserver

import (
	"encoding/binary"
	"log"
	"sort"
	"strings"

	"niaohao/server/internal/cmdname"
)

func teamPrivCanAdmin(priv uint32) bool {
	return priv <= teamPrivVice
}

// teamResolveRuntimeLocked 调用方须已持有 h.mu。
func teamResolveRuntimeLocked(h *teamHub, uid int64, reqTeamID uint32) (teamID uint32, t *teamRuntime) {
	tid := h.uidIndex[uid]
	if reqTeamID == 0 {
		reqTeamID = tid
	}
	if reqTeamID != 0 {
		t = h.teams[reqTeamID]
		if t != nil {
			return reqTeamID, t
		}
	}
	if tid != 0 {
		return tid, h.teams[tid]
	}
	return 0, nil
}

func (s *Server) handleTeamCreate(c *Client, uid uint32, body []byte) {
	s.initTeamHub()
	h := s.teams
	iuid := int64(uid)

	var createdTeamID, ret uint32
	h.mu.Lock()
	if h.uidIndex[iuid] != 0 {
		ret = 2
	} else {
		name, slogan, interest, joinFlag, bg, icon, color, txt, word := parseTeamCreateBody(body)
		teamID := h.nextID
		if teamID < teamMinID {
			teamID = teamMinID
		}
		h.nextID = teamID + 1
		t := &teamRuntime{
			ID:       teamID,
			LeaderID: iuid,
			Name:     name,
			Slogan:   slogan,
			Interest: interest,
			JoinFlag: joinFlag,
			VisitFlag: 1,
			LogoBg:   bg,
			LogoIcon: icon,
			LogoColor: color,
			TxtColor: txt,
			LogoWord: word,
			Members: map[int64]*teamMember{
				iuid: {
					UserID:  iuid,
					Priv:    teamPrivLeader,
					IsShow:  true,
				},
			},
		}
		h.teams[teamID] = t
		h.uidIndex[iuid] = teamID
		createdTeamID = teamID
		h.saveLocked()
	}
	h.mu.Unlock()

	out := make([]byte, 8)
	binary.BigEndian.PutUint32(out[0:4], createdTeamID)
	binary.BigEndian.PutUint32(out[4:8], ret)
	s.send(c, 2910, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d teamID=%d ret=%d", cmdname.Format(2910), uid, createdTeamID, ret)
}

func (s *Server) handleTeamAdd(c *Client, uid uint32, body []byte) {
	var reqTeamID uint32
	if len(body) >= 4 {
		reqTeamID = binary.BigEndian.Uint32(body[0:4])
	}
	iuid := int64(uid)

	ret := uint32(2)
	respTeamID := uint32(0)
	var leaderNotify int64

	s.initTeamHub()
	h := s.teams
	h.mu.Lock()
	if reqTeamID != 0 && h.uidIndex[iuid] == 0 {
		t := h.teams[reqTeamID]
		if t == nil {
			ret = 2
		} else if t.JoinFlag == 0 {
			if t.Members == nil {
				t.Members = make(map[int64]*teamMember)
			}
			t.Members[iuid] = &teamMember{
				UserID: iuid,
				Priv:   teamPrivMember,
				IsShow: true,
			}
			h.uidIndex[iuid] = reqTeamID
			ret = 0
			respTeamID = reqTeamID
			h.saveLocked()
		} else if t.JoinFlag == 1 {
			h.pendingByUser[iuid] = reqTeamID
			ret = 1
			respTeamID = reqTeamID
			leaderNotify = t.LeaderID
			h.saveLocked()
		} else {
			ret = 2
		}
	}
	h.mu.Unlock()

	out := make([]byte, 8)
	binary.BigEndian.PutUint32(out[0:4], ret)
	binary.BigEndian.PutUint32(out[4:8], respTeamID)
	s.send(c, 2911, uid, 0, out)
	if leaderNotify != 0 {
		s.pushTeamInform(leaderNotify, 2911, uid, s.teamNick(iuid), 0, 0)
	}
	log.Printf("[CMD] OK     %s UID=%d ret=%d teamID=%d", cmdname.Format(2911), uid, ret, respTeamID)
}

func (s *Server) handleTeamAnswer(c *Client, uid uint32, body []byte) {
	if len(body) < 8 {
		out := make([]byte, 4)
		binary.BigEndian.PutUint32(out, 1)
		s.send(c, 2912, uid, 0, out)
		return
	}
	targetUID := int64(binary.BigEndian.Uint32(body[0:4]))
	agree := binary.BigEndian.Uint32(body[4:8]) == 1
	iuid := int64(uid)

	ret := uint32(0)
	var notifyData1, notifyTeamID uint32
	var notifyTarget int64

	s.initTeamHub()
	h := s.teams
	h.mu.Lock()
	tid := h.uidIndex[iuid]
	if tid == 0 {
		ret = 1
	} else {
		t := h.teams[tid]
		m := t.Members[iuid]
		if t == nil || m == nil || !teamPrivCanAdmin(m.Priv) || h.pendingByUser[targetUID] != tid {
			ret = 2
		} else {
			notifyTeamID = tid
			if agree && h.uidIndex[targetUID] == 0 {
				if t.Members == nil {
					t.Members = make(map[int64]*teamMember)
				}
				t.Members[targetUID] = &teamMember{
					UserID: targetUID,
					Priv:   teamPrivMember,
					IsShow: true,
				}
				h.uidIndex[targetUID] = tid
				notifyData1 = 1
			}
			delete(h.pendingByUser, targetUID)
			notifyTarget = targetUID
			h.saveLocked()
		}
	}
	h.mu.Unlock()

	out := make([]byte, 4)
	binary.BigEndian.PutUint32(out, ret)
	s.send(c, 2912, uid, 0, out)
	if ret == 0 && notifyTarget != 0 {
		s.pushTeamInform(notifyTarget, 2912, uid, s.teamNick(iuid), notifyData1, notifyTeamID)
	}
	log.Printf("[CMD] OK     %s UID=%d ret=%d target=%d", cmdname.Format(2912), uid, ret, targetUID)
}

func (s *Server) handleTeamInformPull(c *Client, uid uint32, body []byte) {
	out := make([]byte, teamInformBodyLen)
	s.send(c, 2913, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d", cmdname.Format(2913), uid)
}

func (s *Server) handleTeamQuit(c *Client, uid uint32, body []byte) {
	iuid := int64(uid)
	var respTeamID uint32

	s.initTeamHub()
	h := s.teams
	h.mu.Lock()
	tid := h.uidIndex[iuid]
	respTeamID = tid
	if tid != 0 {
		t := h.teams[tid]
		if t != nil {
			delete(t.Members, iuid)
			delete(h.uidIndex, iuid)
			delete(h.pendingByUser, iuid)
			if t.LeaderID == iuid {
				var newLeader int64
				for memberUID, member := range t.Members {
					if newLeader == 0 || memberUID < newLeader {
						newLeader = memberUID
					}
					if member != nil {
						member.Priv = teamPrivMember
					}
				}
				t.LeaderID = newLeader
				if newLeader != 0 {
					if nm := t.Members[newLeader]; nm != nil {
						nm.Priv = teamPrivLeader
					}
				}
			}
			if len(t.Members) == 0 {
				delete(h.teams, tid)
			}
			h.saveLocked()
		}
	}
	h.mu.Unlock()

	out := make([]byte, 4)
	binary.BigEndian.PutUint32(out, respTeamID)
	s.send(c, 2914, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d teamID=%d", cmdname.Format(2914), uid, respTeamID)
}

func (s *Server) handleTeamChangeAdmin(c *Client, uid uint32, body []byte) {
	if len(body) < 8 {
		out := make([]byte, 4)
		binary.BigEndian.PutUint32(out, 1)
		s.send(c, 2915, uid, 0, out)
		return
	}
	targetUID := int64(binary.BigEndian.Uint32(body[0:4]))
	newPriv := binary.BigEndian.Uint32(body[4:8])
	iuid := int64(uid)

	ret := uint32(0)
	var notifyTarget int64
	var notifyPriv uint32

	s.initTeamHub()
	h := s.teams
	h.mu.Lock()
	tid := h.uidIndex[iuid]
	if tid == 0 {
		ret = 1
	} else {
		t := h.teams[tid]
		selfM := t.Members[iuid]
		if t == nil || selfM == nil || !teamPrivCanAdmin(selfM.Priv) {
			ret = 1
		} else if targetM := t.Members[targetUID]; targetM == nil {
			ret = 3
		} else {
			targetM.Priv = newPriv
			notifyTarget = targetUID
			notifyPriv = newPriv
			h.saveLocked()
		}
	}
	h.mu.Unlock()

	out := make([]byte, 4)
	binary.BigEndian.PutUint32(out, ret)
	s.send(c, 2915, uid, 0, out)
	if ret == 0 && notifyTarget != 0 {
		s.pushTeamInform(notifyTarget, 2915, uid, s.teamNick(iuid), notifyPriv, 0)
	}
	log.Printf("[CMD] OK     %s UID=%d ret=%d", cmdname.Format(2915), uid, ret)
}

func (s *Server) handleTeamDeleteMember(c *Client, uid uint32, body []byte) {
	if len(body) < 4 {
		out := make([]byte, 4)
		binary.BigEndian.PutUint32(out, 1)
		s.send(c, 2916, uid, 0, out)
		return
	}
	targetUID := int64(binary.BigEndian.Uint32(body[0:4]))
	iuid := int64(uid)

	ret := uint32(0)
	var notifyTarget int64

	s.initTeamHub()
	h := s.teams
	h.mu.Lock()
	tid := h.uidIndex[iuid]
	if tid == 0 {
		ret = 1
	} else {
		t := h.teams[tid]
		selfM := t.Members[iuid]
		if t == nil || selfM == nil || !teamPrivCanAdmin(selfM.Priv) {
			ret = 1
		} else if targetUID == iuid {
			ret = 2
		} else if _, ok := t.Members[targetUID]; !ok {
			ret = 3
		} else {
			delete(t.Members, targetUID)
			delete(h.uidIndex, targetUID)
			delete(h.pendingByUser, targetUID)
			notifyTarget = targetUID
			h.saveLocked()
		}
	}
	h.mu.Unlock()

	out := make([]byte, 4)
	binary.BigEndian.PutUint32(out, ret)
	s.send(c, 2916, uid, 0, out)
	if ret == 0 && notifyTarget != 0 {
		s.pushTeamInform(notifyTarget, 2916, uid, s.teamNick(iuid), 0, 0)
	}
	log.Printf("[CMD] OK     %s UID=%d ret=%d", cmdname.Format(2916), uid, ret)
}

func (s *Server) handleTeamGetInfo(c *Client, uid uint32, body []byte) {
	var reqTeamID uint32
	if len(body) >= 4 {
		reqTeamID = binary.BigEndian.Uint32(body[0:4])
	}

	s.initTeamHub()
	h := s.teams
	h.mu.Lock()
	teamID, t := teamResolveRuntimeLocked(h, int64(uid), reqTeamID)
	out := teamBuildSimpleInfoBody(t)
	h.mu.Unlock()

	s.send(c, 2917, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d teamID=%d", cmdname.Format(2917), uid, teamID)
}

func (s *Server) handleTeamGetMemberList(c *Client, uid uint32, body []byte) {
	var reqTeamID uint32
	if len(body) >= 4 {
		reqTeamID = binary.BigEndian.Uint32(body[0:4])
	}

	s.initTeamHub()
	h := s.teams
	h.mu.Lock()
	teamID, t := teamResolveRuntimeLocked(h, int64(uid), reqTeamID)
	memberCount := 0
	if t != nil && t.Members != nil {
		memberCount = len(t.Members)
	}
	out := make([]byte, 12+memberCount*teamMemberEntryLen)
	binary.BigEndian.PutUint32(out[0:4], teamID)
	if t != nil {
		binary.BigEndian.PutUint32(out[4:8], t.SuperCoreNum)
	}
	binary.BigEndian.PutUint32(out[8:12], uint32(memberCount))
	off := 12
	if t != nil && t.Members != nil {
		uids := make([]int64, 0, len(t.Members))
		for memberUID := range t.Members {
			uids = append(uids, memberUID)
		}
		sort.Slice(uids, func(i, j int) bool { return uids[i] < uids[j] })
		for _, memberUID := range uids {
			m := t.Members[memberUID]
			priv := uint32(teamPrivMember)
			contrib := uint32(0)
			if m != nil {
				priv = m.Priv
				contrib = m.Contribute
			}
			binary.BigEndian.PutUint32(out[off:off+4], uint32(memberUID))
			binary.BigEndian.PutUint32(out[off+4:off+8], priv)
			binary.BigEndian.PutUint32(out[off+8:off+12], contrib)
			off += teamMemberEntryLen
		}
	}
	h.mu.Unlock()

	s.send(c, 2918, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d teamID=%d count=%d", cmdname.Format(2918), uid, teamID, memberCount)
}

func (s *Server) handleTeamSetJoinFlag(c *Client, uid uint32, body []byte) {
	iuid := int64(uid)
	ret := uint32(0)

	s.initTeamHub()
	h := s.teams
	h.mu.Lock()
	tid := h.uidIndex[iuid]
	if tid == 0 {
		ret = 1
	} else {
		t := h.teams[tid]
		m := t.Members[iuid]
		if t == nil || m == nil || !teamPrivCanAdmin(m.Priv) {
			ret = 1
		} else if len(body) >= 4 {
			t.JoinFlag = binary.BigEndian.Uint32(body[0:4])
			h.saveLocked()
		}
	}
	h.mu.Unlock()

	out := make([]byte, 4)
	binary.BigEndian.PutUint32(out, ret)
	s.send(c, 2920, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d ret=%d", cmdname.Format(2920), uid, ret)
}

func (s *Server) handleTeamSetSlogan(c *Client, uid uint32, body []byte) {
	iuid := int64(uid)
	ret := uint32(0)
	slogan := strings.TrimSpace(strings.Trim(string(body), "\x00"))

	s.initTeamHub()
	h := s.teams
	h.mu.Lock()
	tid := h.uidIndex[iuid]
	if tid == 0 {
		ret = 1
	} else {
		t := h.teams[tid]
		m := t.Members[iuid]
		if t == nil || m == nil || !teamPrivCanAdmin(m.Priv) {
			ret = 1
		} else {
			t.Slogan = slogan
			h.saveLocked()
		}
	}
	h.mu.Unlock()

	out := make([]byte, 4)
	binary.BigEndian.PutUint32(out, ret)
	s.send(c, 2921, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d ret=%d", cmdname.Format(2921), uid, ret)
}

func (s *Server) handleTeamModifyLogo(c *Client, uid uint32, body []byte) {
	iuid := int64(uid)
	ret := uint32(0)

	s.initTeamHub()
	h := s.teams
	h.mu.Lock()
	tid := h.uidIndex[iuid]
	if tid == 0 {
		ret = 1
	} else {
		t := h.teams[tid]
		m := t.Members[iuid]
		if t == nil || m == nil || !teamPrivCanAdmin(m.Priv) {
			ret = 1
		} else {
			applyTeamModifyLogoBody(body, t)
			h.saveLocked()
		}
	}
	h.mu.Unlock()

	out := make([]byte, 4)
	binary.BigEndian.PutUint32(out, ret)
	s.send(c, 2922, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d ret=%d", cmdname.Format(2922), uid, ret)
}

func (s *Server) handleTeamShowLogo(c *Client, uid uint32, body []byte) {
	iuid := int64(uid)
	ret := uint32(0)
	isShow := false
	if len(body) >= 4 {
		isShow = binary.BigEndian.Uint32(body[0:4]) != 0
	}

	s.initTeamHub()
	h := s.teams
	h.mu.Lock()
	if tid := h.uidIndex[iuid]; tid != 0 {
		if t := h.teams[tid]; t != nil {
			if m := t.Members[iuid]; m != nil {
				m.IsShow = isShow
				h.saveLocked()
			}
		}
	}
	h.mu.Unlock()

	out := make([]byte, 4)
	binary.BigEndian.PutUint32(out, ret)
	s.send(c, 2927, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d ret=%d show=%v", cmdname.Format(2927), uid, ret, isShow)
}

func (s *Server) handleTeamGetLogoInfo(c *Client, uid uint32, body []byte) {
	var reqID uint32
	if len(body) >= 4 {
		reqID = binary.BigEndian.Uint32(body[0:4])
	}

	s.initTeamHub()
	h := s.teams
	h.mu.Lock()
	var t *teamRuntime
	var logoID uint32
	if reqID >= teamMinID {
		t = h.teams[reqID]
		logoID = reqID
	} else {
		lookupUID := int64(reqID)
		if lookupUID <= 0 {
			lookupUID = int64(uid)
		}
		if tid := h.uidIndex[lookupUID]; tid != 0 {
			t = h.teams[tid]
			if t != nil {
				logoID = t.ID
			}
		}
	}
	if logoID == 0 && t != nil {
		logoID = t.ID
	}
	out := teamBuildLogoInfoBody(logoID, t)
	h.mu.Unlock()

	s.send(c, 2928, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d req=%d", cmdname.Format(2928), uid, reqID)
}

func (s *Server) handleTeamInviteToJoin(c *Client, uid uint32, body []byte) {
	var targetUID int64
	if len(body) >= 4 {
		targetUID = int64(binary.BigEndian.Uint32(body[0:4]))
	}
	iuid := int64(uid)

	ret := uint32(0)
	var teamID uint32

	s.initTeamHub()
	h := s.teams
	h.mu.Lock()
	tid := h.uidIndex[iuid]
	if tid == 0 {
		ret = 1
	} else {
		teamID = tid
	}
	h.mu.Unlock()

	if ret == 0 && targetUID != 0 {
		s.pushTeamInform(targetUID, 2930, uid, s.teamNick(iuid), 0, teamID)
	}

	out := make([]byte, 4)
	binary.BigEndian.PutUint32(out, ret)
	s.send(c, 2930, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d ret=%d target=%d teamID=%d", cmdname.Format(2930), uid, ret, targetUID, teamID)
}

func (s *Server) handleTeamSetNotice(c *Client, uid uint32, body []byte) {
	iuid := int64(uid)
	ret := uint32(0)
	notice := strings.TrimSpace(strings.Trim(string(body), "\x00"))

	s.initTeamHub()
	h := s.teams
	h.mu.Lock()
	tid := h.uidIndex[iuid]
	if tid == 0 {
		ret = 1
	} else {
		t := h.teams[tid]
		m := t.Members[iuid]
		if t == nil || m == nil || !teamPrivCanAdmin(m.Priv) {
			ret = 1
		} else {
			t.Notice = notice
			h.saveLocked()
		}
	}
	h.mu.Unlock()

	out := make([]byte, 4)
	binary.BigEndian.PutUint32(out, ret)
	s.send(c, 2931, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d ret=%d", cmdname.Format(2931), uid, ret)
}

func (s *Server) handleTeamChat(c *Client, uid uint32, body []byte) {
	iuid := int64(uid)

	s.initTeamHub()
	h := s.teams
	h.mu.Lock()
	tid := h.uidIndex[iuid]
	if tid == 0 {
		h.mu.Unlock()
		s.send(c, 2929, uid, 0, nil)
		return
	}
	defaultTeamID := tid
	teamID, msg := teamParseChatBody(body, defaultTeamID)
	if teamID == 0 {
		teamID = defaultTeamID
	}
	t := h.teams[teamID]
	if t == nil || t.Members == nil || msg == "" {
		h.mu.Unlock()
		s.send(c, 2929, uid, 0, nil)
		return
	}
	memberUIDs := make([]int64, 0, len(t.Members))
	for memberUID := range t.Members {
		memberUIDs = append(memberUIDs, memberUID)
	}
	h.mu.Unlock()

	nick := s.teamNick(iuid)
	msgBytes := []byte(msg)
	chatBody := make([]byte, 28+len(msgBytes))
	binary.BigEndian.PutUint32(chatBody[0:4], uid)
	teamPutFixed(chatBody, 4, nick, 16)
	binary.BigEndian.PutUint32(chatBody[20:24], teamID)
	binary.BigEndian.PutUint32(chatBody[24:28], uint32(len(msgBytes)))
	copy(chatBody[28:], msgBytes)

	for _, memberUID := range memberUIDs {
		if mc := s.clientOf(memberUID); mc != nil && mc.LoggedIn {
			s.send(mc, 2929, uint32(memberUID), 0, chatBody)
		}
	}
	log.Printf("[CMD] OK     %s UID=%d teamID=%d msgLen=%d", cmdname.Format(2929), uid, teamID, len(msgBytes))
}

func (s *Server) handleContributeBounds(c *Client, uid uint32, body []byte) {
	out := make([]byte, 16)
	s.send(c, 2932, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d", cmdname.Format(2932), uid)
}

func (s *Server) handleTeamSelectSuperCore(c *Client, uid uint32, body []byte) {
	iuid := int64(uid)
	var num uint32

	s.initTeamHub()
	h := s.teams
	h.mu.Lock()
	if tid := h.uidIndex[iuid]; tid != 0 {
		if t := h.teams[tid]; t != nil {
			num = t.SuperCoreNum
		}
	}
	h.mu.Unlock()

	out := make([]byte, 4)
	binary.BigEndian.PutUint32(out, num)
	s.send(c, 2925, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d num=%d", cmdname.Format(2925), uid, num)
}

func (s *Server) handleTeamGiveSuperCore(c *Client, uid uint32, body []byte) {
	out := make([]byte, 4)
	s.send(c, 2923, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d", cmdname.Format(2923), uid)
}

func (s *Server) handleTeamGetSuperCore(c *Client, uid uint32, body []byte) {
	out := make([]byte, 4)
	s.send(c, 2924, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d", cmdname.Format(2924), uid)
}

func (s *Server) handleTeamCreatItem(c *Client, uid uint32, body []byte) {
	s.send(c, 2926, uid, 0, nil)
	log.Printf("[CMD] OK     %s UID=%d", cmdname.Format(2926), uid)
}
