package gameserver

import (
	"bytes"
	"encoding/binary"
	"log"
	"strconv"
	"time"

	"niaohao/server/internal/cmdname"
	"niaohao/server/internal/packet"
	"niaohao/server/internal/store"
)

const topWarRankListLimit = 100

// 本批：46001 真实排行 + curTopLevel 存档；2458/2567/46002 仍 stub。
// 完整巅峰匹配/赛季奖后续再接。

// handleTopFightBeyond CMD 2567：突破模式入队 ACK（监听不读包体；完整 AI 对战未接）。
func (s *Server) handleTopFightBeyond(c *Client, uid uint32, body []byte) {
	cup := uint32(0)
	if len(body) >= 4 {
		cup = binary.BigEndian.Uint32(body[0:4])
	}
	s.send(c, 2567, uid, 0, nil)
	log.Printf("[CMD] OK     %s UID=%d cup=%d (stub ACK, no match)", cmdname.Format(2567), uid, cup)
}

// handleGetTopWarRank CMD 46001 → TopWarRankInfo：selfRank(4)+count(4)+[UTF nick + score(4)]*n
// selfRankIndex：1-based；0=未上榜（分≤0 或不在前 N）。
func (s *Server) handleGetTopWarRank(c *Client, uid uint32) {
	out := buildTopWarRankBody(0, nil)
	selfRank := uint32(0)
	count := 0
	if s.cfg.Store != nil {
		list, err := s.cfg.Store.ListTopWarRanks(topWarRankListLimit)
		if err != nil {
			log.Printf("[CMD] WARN  %s UID=%d ListTopWarRanks: %v", cmdname.Format(46001), uid, err)
		} else {
			for i, e := range list {
				if e.UserID == int64(uid) {
					selfRank = uint32(i + 1)
					break
				}
			}
			out = buildTopWarRankBody(selfRank, list)
			count = len(list)
		}
	}
	s.send(c, 46001, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d selfRank=%d count=%d", cmdname.Format(46001), uid, selfRank, count)
}

// buildTopWarRankBody 组装 TopWarRankInfo 包体。
func buildTopWarRankBody(selfRank uint32, list []store.TopWarRankEntry) []byte {
	buf := new(bytes.Buffer)
	packet.WriteU32(buf, selfRank)
	packet.WriteU32(buf, uint32(len(list)))
	for _, e := range list {
		nick := e.Nickname
		if nick == "" {
			nick = strconv.FormatInt(e.UserID, 10)
		}
		packet.WriteUTF(buf, nick)
		packet.WriteU32(buf, uint32(store.ClampTopLevel(e.Score)))
	}
	return buf.Bytes()
}

// getCurTopLevel 读巅峰积分（已钳制）。
func (s *Server) getCurTopLevel(uid int64) int {
	return store.ClampTopLevel(s.loadUserOps(uid).CurTopLevel)
}

// setCurTopLevel 写入巅峰积分并落库。
func (s *Server) setCurTopLevel(uid int64, score int) int {
	st := s.loadUserOps(uid)
	st.CurTopLevel = store.ClampTopLevel(score)
	s.saveUserOps(uid, st)
	return st.CurTopLevel
}

// handlePvpRankReward CMD 46002：领取赛季奖励（暂不做；空 ACK + 提示）。
func (s *Server) handlePvpRankReward(c *Client, uid uint32) {
	s.send(c, 46002, uid, 0, nil)
	s.sendAlert(int64(uid), "本赛季暂无奖励可领取")
	log.Printf("[CMD] OK     %s UID=%d (stub)", cmdname.Format(46002), uid)
}

// 超 No 每日签到：NonoVipDailySignPanel 字段 firstDay/todaySign/signeddays/thisBee/totalDay。

func (s *Server) loadNonoVipSign(uid int64) store.NonoVipSignState {
	st := store.NonoVipSignState{}
	if s.cfg.Store == nil {
		return store.NormalizeNonoVipSign(st, time.Now())
	}
	raw, _ := s.cfg.Store.GetNonoVipSign(uid)
	return store.NormalizeNonoVipSign(raw, time.Now())
}

func chinaNow(now time.Time) time.Time {
	if loc, err := time.LoadLocation("Asia/Shanghai"); err == nil {
		return now.In(loc)
	}
	return now.In(time.FixedZone("CST", 8*3600))
}

func buildNonoVipSignInfoBody(st store.NonoVipSignState, now time.Time) []byte {
	t := chinaNow(now)
	firstOfMonth := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
	lastOfMonth := firstOfMonth.AddDate(0, 1, -1)
	todaySign := uint32(0)
	if st.HasDay(t.Day()) {
		todaySign = 1
	}
	bee := uint32(0)
	if st.BeeTaken {
		bee = 1
	}
	out := make([]byte, 20)
	binary.BigEndian.PutUint32(out[0:4], uint32(firstOfMonth.Weekday())) // AS Date.day：周日=0
	binary.BigEndian.PutUint32(out[4:8], todaySign)
	binary.BigEndian.PutUint32(out[8:12], uint32(st.SignedCount()))
	binary.BigEndian.PutUint32(out[12:16], bee)
	binary.BigEndian.PutUint32(out[16:20], uint32(lastOfMonth.Day()))
	return out
}

// handleNonoVipDailySign CMD 9297：点签到；空请求；空 ACK（面板 signOver 后再拉 9298）。
func (s *Server) handleNonoVipDailySign(c *Client, uid uint32) {
	now := time.Now()
	st := s.loadNonoVipSign(int64(uid))
	day := chinaNow(now).Day()
	already := st.HasDay(day)
	if !already {
		st.SetDay(day)
		if s.cfg.Store != nil {
			if err := s.cfg.Store.SetNonoVipSign(int64(uid), st); err != nil {
				log.Printf("[CMD] WARN  %s UID=%d SetNonoVipSign: %v", cmdname.Format(9297), uid, err)
			}
			s.grantNonoVipSignRewards(int64(uid), day)
		}
	}
	s.send(c, 9297, uid, 0, nil)
	log.Printf("[CMD] OK     %s UID=%d day=%d already=%v signed=%d",
		cmdname.Format(9297), uid, day, already, st.SignedCount())
}

// handleNonoVipDailySignInfo CMD 9298：面板五字段各 u32。
func (s *Server) handleNonoVipDailySignInfo(c *Client, uid uint32) {
	st := s.loadNonoVipSign(int64(uid))
	out := buildNonoVipSignInfoBody(st, time.Now())
	s.send(c, 9298, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d signed=%d today=%d bee=%v",
		cmdname.Format(9298), uid, st.SignedCount(), binary.BigEndian.Uint32(out[4:8]), st.BeeTaken)
}

// handleNonoVipDailySignBee CMD 9299：满勤领小蜜蜂奖（本服：满当月天数；奖积累经验 stub）。
func (s *Server) handleNonoVipDailySignBee(c *Client, uid uint32) {
	now := time.Now()
	st := s.loadNonoVipSign(int64(uid))
	info := buildNonoVipSignInfoBody(st, now)
	totalDay := int(binary.BigEndian.Uint32(info[16:20]))
	ok := false
	if !st.BeeTaken && st.SignedCount() >= totalDay && totalDay > 0 {
		st.BeeTaken = true
		if s.cfg.Store != nil {
			_ = s.cfg.Store.SetNonoVipSign(int64(uid), st)
			const beeExp = 20000
			_, _ = s.cfg.Store.AddExpPool(int64(uid), beeExp)
			s.sendAlert(int64(uid), "领取小蜜蜂奖，获得积累经验 "+strconv.Itoa(beeExp))
		}
		ok = true
	} else if st.BeeTaken {
		s.sendAlert(int64(uid), "本月小蜜蜂奖已领取")
	} else {
		s.sendAlert(int64(uid), "本月尚未满勤，无法领取小蜜蜂奖")
	}
	s.send(c, 9299, uid, 0, nil)
	log.Printf("[CMD] OK     %s UID=%d ok=%v signed=%d/%d",
		cmdname.Format(9299), uid, ok, st.SignedCount(), totalDay)
}

// handleGetHonorValue CMD 70003。
// FightExchangePanel 只读第 1 个 u32 作为当前荣誉（honor_Txt / honorValue）；
// 原先把上限 9999 放首位，导致面板永远显示 9999、客户端预检永远通过。
func (s *Server) handleGetHonorValue(c *Client, uid uint32) {
	cur := s.getHonor(int64(uid))
	if cur < 0 {
		cur = 0
	}
	out := make([]byte, 8)
	binary.BigEndian.PutUint32(out[0:4], uint32(cur))
	binary.BigEndian.PutUint32(out[4:8], 9999) // 上限（本面板未用）
	s.send(c, 70003, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d cur=%d max=9999", cmdname.Format(70003), uid, cur)
}
