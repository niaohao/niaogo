package gameserver

import (
	"encoding/binary"
	"log"
	"time"

	"niaohao/server/internal/cmdname"
)

const (
	teacherDelCoins     = 200
	teacherSevenDaySec  = 7 * 24 * 3600
	teacherExpPondShare = 5 // 教官战后经验入池比例分母（gain/5）
)

func (s *Server) teacherRelOf(uid int64) (teacherID, studentID uint32, graduation, pond int) {
	st := s.loadUserOps(uid)
	return st.TeacherID, st.StudentID, st.GraduationCount, st.TeacherExpPond
}

func (s *Server) setTeacherRel(uid int64, teacherID, studentID uint32) {
	st := s.loadUserOps(uid)
	st.TeacherID = teacherID
	st.StudentID = studentID
	s.saveUserOps(uid, st)
}

func (s *Server) setTeacherExpPond(uid int64, pond int) {
	st := s.loadUserOps(uid)
	if pond < 0 {
		pond = 0
	}
	st.TeacherExpPond = pond
	s.saveUserOps(uid, st)
}

func (s *Server) bumpGraduation(uid int64) int {
	st := s.loadUserOps(uid)
	st.GraduationCount++
	n := st.GraduationCount
	s.saveUserOps(uid, st)
	return n
}

// contributeTeacherExpPond 教官有学员时，战后经验按比例入共享池。
func (s *Server) contributeTeacherExpPond(uid int64, gain int) {
	if gain <= 0 {
		return
	}
	_, studentID, _, pond := s.teacherRelOf(uid)
	if studentID == 0 {
		return
	}
	add := gain / teacherExpPondShare
	if add < 1 {
		add = 1
	}
	s.setTeacherExpPond(uid, pond+add)
}

func (s *Server) sendU32Ack(c *Client, cmd int32, uid, v uint32) {
	out := make([]byte, 4)
	binary.BigEndian.PutUint32(out, v)
	s.send(c, cmd, uid, 0, out)
}

// handleRequestAddTeacher CMD 3001：学员申请教官；推 INFORM(3001) 给目标。
func (s *Server) handleRequestAddTeacher(c *Client, uid uint32, body []byte) {
	target := uint32(0)
	if len(body) >= 4 {
		target = binary.BigEndian.Uint32(body[0:4])
	}
	s.send(c, 3001, uid, 0, nil)
	if target == 0 || target == uid {
		log.Printf("[CMD] OK     %s UID=%d target=%d (skip)", cmdname.Format(3001), uid, target)
		return
	}
	myT, myS, _, _ := s.teacherRelOf(int64(uid))
	if myT != 0 || myS != 0 {
		log.Printf("[CMD] OK     %s UID=%d target=%d (already bonded)", cmdname.Format(3001), uid, target)
		return
	}
	tt, ts, _, _ := s.teacherRelOf(int64(target))
	if tt != 0 || ts != 0 {
		log.Printf("[CMD] OK     %s UID=%d target=%d (target bonded)", cmdname.Format(3001), uid, target)
		return
	}
	s.pushInform(int64(target), 3001, uid, s.nickOf(uid), 0)
	log.Printf("[CMD] OK     %s UID=%d target=%d", cmdname.Format(3001), uid, target)
}

// handleAnswerAddTeacher CMD 3002：教官同意/拒绝；同意后自身 studentID=对方、对方 teacherID=自己。
func (s *Server) handleAnswerAddTeacher(c *Client, uid uint32, body []byte) {
	target, accept := uint32(0), uint32(0)
	if len(body) >= 4 {
		target = binary.BigEndian.Uint32(body[0:4])
	}
	if len(body) >= 8 {
		accept = binary.BigEndian.Uint32(body[4:8])
	}
	ok := uint32(0)
	if accept == 1 && target > 0 && target != uid {
		myT, myS, _, _ := s.teacherRelOf(int64(uid))
		tt, ts, _, _ := s.teacherRelOf(int64(target))
		if myT == 0 && myS == 0 && tt == 0 && ts == 0 {
			s.setTeacherRel(int64(uid), 0, target)   // 我是教官
			s.setTeacherRel(int64(target), uid, 0) // 对方是学员
			if s.cfg.Store != nil {
				_ = s.cfg.Store.AddFriend(int64(uid), int64(target))
			}
			ok = 1
		}
	}
	s.sendU32Ack(c, 3002, uid, ok)
	s.pushInform(int64(target), 3002, uid, s.nickOf(uid), ok)
	log.Printf("[CMD] OK     %s UID=%d target=%d accept=%d ok=%d", cmdname.Format(3002), uid, target, accept, ok)
}

// handleRequestAddStudent CMD 3003：教官招收学员；推 INFORM(3003)。
func (s *Server) handleRequestAddStudent(c *Client, uid uint32, body []byte) {
	target := uint32(0)
	if len(body) >= 4 {
		target = binary.BigEndian.Uint32(body[0:4])
	}
	s.send(c, 3003, uid, 0, nil)
	if target == 0 || target == uid {
		log.Printf("[CMD] OK     %s UID=%d target=%d (skip)", cmdname.Format(3003), uid, target)
		return
	}
	myT, myS, _, _ := s.teacherRelOf(int64(uid))
	if myT != 0 || myS != 0 {
		log.Printf("[CMD] OK     %s UID=%d target=%d (already bonded)", cmdname.Format(3003), uid, target)
		return
	}
	tt, ts, _, _ := s.teacherRelOf(int64(target))
	if tt != 0 || ts != 0 {
		log.Printf("[CMD] OK     %s UID=%d target=%d (target bonded)", cmdname.Format(3003), uid, target)
		return
	}
	s.pushInform(int64(target), 3003, uid, s.nickOf(uid), 0)
	log.Printf("[CMD] OK     %s UID=%d target=%d", cmdname.Format(3003), uid, target)
}

// handleAnswerAddStudent CMD 3004：学员同意/拒绝；同意后自身 teacherID=对方、对方 studentID=自己。
func (s *Server) handleAnswerAddStudent(c *Client, uid uint32, body []byte) {
	target, accept := uint32(0), uint32(0)
	if len(body) >= 4 {
		target = binary.BigEndian.Uint32(body[0:4])
	}
	if len(body) >= 8 {
		accept = binary.BigEndian.Uint32(body[4:8])
	}
	ok := uint32(0)
	if accept == 1 && target > 0 && target != uid {
		myT, myS, _, _ := s.teacherRelOf(int64(uid))
		tt, ts, _, _ := s.teacherRelOf(int64(target))
		if myT == 0 && myS == 0 && tt == 0 && ts == 0 {
			s.setTeacherRel(int64(uid), target, 0) // 我是学员
			s.setTeacherRel(int64(target), 0, uid) // 对方是教官
			if s.cfg.Store != nil {
				_ = s.cfg.Store.AddFriend(int64(uid), int64(target))
			}
			ok = 1
		}
	}
	s.sendU32Ack(c, 3004, uid, ok)
	s.pushInform(int64(target), 3004, uid, s.nickOf(uid), ok)
	log.Printf("[CMD] OK     %s UID=%d target=%d accept=%d ok=%d", cmdname.Format(3004), uid, target, accept, ok)
}

// handleDeleteTeacher CMD 3005：学员解除教官。
func (s *Server) handleDeleteTeacher(c *Client, uid uint32, body []byte) {
	teacherID, _, _, _ := s.teacherRelOf(int64(uid))
	if teacherID != 0 {
		s.setTeacherRel(int64(uid), 0, 0)
		tt, ts, _, _ := s.teacherRelOf(int64(teacherID))
		if ts == uid {
			s.setTeacherRel(int64(teacherID), tt, 0)
		}
		s.pushInform(int64(teacherID), 3005, uid, s.nickOf(uid), 2)
	}
	s.send(c, 3005, uid, 0, nil)
	log.Printf("[CMD] OK     %s UID=%d teacher=%d", cmdname.Format(3005), uid, teacherID)
}

// handleDeleteStudent CMD 3006：教官解除学员；未满 7 日未登则扣 200 豆。
func (s *Server) handleDeleteStudent(c *Client, uid uint32, body []byte) {
	_, studentID, _, _ := s.teacherRelOf(int64(uid))
	if studentID != 0 {
		free := s.studentSevenNoLogin(int64(studentID))
		if !free && s.cfg.Store != nil {
			_, _, _ = s.cfg.Store.TrySpendCoins(int64(uid), teacherDelCoins)
		}
		s.setTeacherRel(int64(uid), 0, 0)
		tt, ts, _, _ := s.teacherRelOf(int64(studentID))
		if tt == uid {
			s.setTeacherRel(int64(studentID), 0, ts)
		}
		s.pushInform(int64(studentID), 3006, uid, s.nickOf(uid), 2)
	}
	s.send(c, 3006, uid, 0, nil)
	log.Printf("[CMD] OK     %s UID=%d student=%d", cmdname.Format(3006), uid, studentID)
}

func (s *Server) studentSevenNoLogin(studentUID int64) bool {
	if s.cfg.Store == nil {
		return false
	}
	u, err := s.cfg.Store.FindByUserID(studentUID)
	if err != nil || u == nil || u.LastOnline <= 0 {
		return true
	}
	return time.Now().Unix()-u.LastOnline >= teacherSevenDaySec
}

// handleExperienceShared CMD 3007：学员领取教官共享经验，均分背包精灵。
func (s *Server) handleExperienceShared(c *Client, uid uint32, body []byte) {
	teacherID, _, _, _ := s.teacherRelOf(int64(uid))
	total := 0
	if teacherID != 0 {
		_, _, _, pond := s.teacherRelOf(int64(teacherID))
		if pond > 0 {
			total = s.distributeSharedExp(int64(uid), pond)
			s.setTeacherExpPond(int64(teacherID), 0)
		}
	}
	s.sendU32Ack(c, 3007, uid, uint32(total))
	log.Printf("[CMD] OK     %s UID=%d total=%d", cmdname.Format(3007), uid, total)
}

func (s *Server) distributeSharedExp(uid int64, pond int) int {
	if s.cfg.Store == nil || pond <= 0 {
		return 0
	}
	bag, err := s.cfg.Store.ListBagPets(uid)
	if err != nil || len(bag) == 0 {
		return 0
	}
	each := pond / len(bag)
	if each < 1 {
		each = 1
	}
	used := 0
	for i := range bag {
		p := &bag[i]
		old := p.Level
		got := applyPetExpGain(p, each)
		if got > 0 {
			used += got
			_ = s.afterPetLevelChange(p, old)
			_ = s.cfg.Store.UpsertPet(p)
		}
	}
	return used
}

// handleTeacherReward CMD 3008：graduationCount + 奖励道具列表（本服暂无道具）。
func (s *Server) handleTeacherReward(c *Client, uid uint32, body []byte) {
	_, _, graduation, _ := s.teacherRelOf(int64(uid))
	out := make([]byte, 8)
	binary.BigEndian.PutUint32(out[0:4], uint32(graduation))
	binary.BigEndian.PutUint32(out[4:8], 0) // item count
	s.send(c, 3008, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d graduation=%d", cmdname.Format(3008), uid, graduation)
}

// handleMyExperiencePond CMD 3009：学员查询教官经验池。
func (s *Server) handleMyExperiencePond(c *Client, uid uint32, body []byte) {
	teacherID, _, _, _ := s.teacherRelOf(int64(uid))
	pond := 0
	if teacherID != 0 {
		_, _, _, pond = s.teacherRelOf(int64(teacherID))
	}
	s.sendU32Ack(c, 3009, uid, uint32(pond))
	log.Printf("[CMD] OK     %s UID=%d pond=%d", cmdname.Format(3009), uid, pond)
}

// handleSevenNoLogin CMD 3010：学员是否连续 7 日未登录（教官免费解除）。
func (s *Server) handleSevenNoLogin(c *Client, uid uint32, body []byte) {
	_, studentID, _, _ := s.teacherRelOf(int64(uid))
	st := uint32(0)
	if studentID != 0 && s.studentSevenNoLogin(int64(studentID)) {
		st = 1
	}
	s.sendU32Ack(c, 3010, uid, st)
	log.Printf("[CMD] OK     %s UID=%d status=%d", cmdname.Format(3010), uid, st)
}

// handleGetMyExperience CMD 3011：教官查询自己积累的共享经验。
func (s *Server) handleGetMyExperience(c *Client, uid uint32, body []byte) {
	_, _, _, pond := s.teacherRelOf(int64(uid))
	s.sendU32Ack(c, 3011, uid, uint32(pond))
	log.Printf("[CMD] OK     %s UID=%d pond=%d", cmdname.Format(3011), uid, pond)
}

// teacherIDsForUser 供 PeopleInfo / 登录 / 2051 使用。
func (s *Server) teacherIDsForUser(uid int64) (teacherID, studentID, graduation uint32) {
	t, st, g, _ := s.teacherRelOf(uid)
	return t, st, uint32(g)
}