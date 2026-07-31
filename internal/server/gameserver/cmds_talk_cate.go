package gameserver

import (
	"encoding/binary"
	"log"
	"math/rand"

	"niaohao/server/internal/cmdname"
)

// 尼尔号日常：船长室 2000 豆 / 发明室 10000 经验（客户端文案为「下周」→ 周限）。
const (
	talkCateCaptainCoins = 2001
	talkCateInventExp    = 2002
	talkCateInventExpNS  = 1004 // 无超能 NoNo 时发明室经验
	talkCateCapsuleVIP   = 2101 // 超能 → 超能胶囊 300007（月限）
	talkCateCapsuleNorm  = 1501 // 普通 → 高级胶囊 300003（月限）
	talkCateBalconyExp   = 1003 // 瞭望露台积累经验（MapProcess_103）
	talkCateNiguDew      = 19   // 尼古滴露（普通 NoNo，map59）
	talkCateNiguDewVIP   = 2055 // 尼古滴露（超能，map59；2701 日限也查此 cate）
	talkCateXioCoral     = 17   // 希欧珊瑚（map56 普通；与 map325 黄晶矿同号，按 MapID 区分）
	talkCateXioCoralVIP  = 2054 // 希欧珊瑚（map56 超能）
	talkCateWellOreA     = 2056 // 云雾气井矿石 A（map64）
	talkCateWellOreB     = 2057 // 云雾气井矿石 B（map64）
	talkCaptainCoinsAmt  = 2000
	talkInventExpAmt     = 10000
	talkBalconyExpAmt    = 10000
	talkNiguDewItem      = 400036
	talkNiguDewDailyKey  = "niguDew"
	talkNiguDewDailyMax  = 5
	talkXioCoralItem     = 400026
	talkXioCoralDailyKey = "xioCoral"
	talkXioCoralDailyMax = 5
	talkWellOreDailyMax  = 5
	talkWellOreItemA     = 400001 // 黄晶矿
	talkWellOreItemB     = 400002 // 甲烷
	talkCapsuleVIPItem   = 300007
	talkCapsuleNormItem  = 300003
	talkGachaCate        = 2051
	talkGachaItemID      = 400501
	talkGachaCount       = 2 // 瞭望露台扭蛋币，攻略写 2
	talkCaptainWeekKey   = "captainCoins"
	talkBalconyExpKey    = "balconyExp1003"
)

var miningCateToItemID = map[uint32]uint32{
	// EnergyController + items.xml：黄晶矿 / 甲烷 / 藤结晶 / 蘑菇结晶 / 纳格 / 豆豆 / 电能石 / 露尼亚…
	1: 400001, 2: 400001, 3: 400001, // 黄晶矿（map10/21/15）
	4: 400002, 5: 400002, 6: 400002, // 甲烷（map20/25/16）
	7: 400009,                       // 藤结晶（map34）
	9: 400010,                       // 蘑菇结晶（map105）
	10: 400011,                      // 纳格晶体（map106）
	11: 400012,                      // 豆豆果实（map106）
	12: 400016,                      // 电能石（map49）
	14: 400023, 15: 400024, 16: 400025, // 露尼亚 / 希罗里 / 欧古德（map54）
	// cate 17：map56=希欧珊瑚（专用分支）；map325 EnergyController=黄晶矿
	18: 400002, // 甲烷（map328）
	// 注意：cate 19 为地图59 尼古滴露，勿当矿
	20: 400001, 21: 400001, 22: 400001,
}

// miningDailyLimit：对照 EnergyController.onCountOK 日限（客户端先 2701 再挖）。
var miningDailyLimit = map[uint32]int{
	1: 5, 2: 5, 3: 5,
	4: 2, 5: 2, 6: 2, 18: 2,
	7: 1,
	9: 3,
	10: 1, 11: 1,
	12: 2,
	14: 1, 15: 1, 16: 1,
	17: 5, // map325 黄晶；map56 希欧珊瑚走独立键
}

// handleTalkCount CMD 2701：cateId → 今日/本周/本月次数（按 cate 语义）。
func (s *Server) handleTalkCount(c *Client, uid uint32, body []byte) {
	cateID := uint32(0)
	if len(body) >= 4 {
		cateID = binary.BigEndian.Uint32(body[0:4])
	}
	key := "talk:" + itoaU32(cateID)
	n := 0
	switch cateID {
	case talkCateCaptainCoins:
		// 船长室：客户端「下周再来」→ 周限
		n = s.weeklyCount(int64(uid), talkCaptainWeekKey)
	case talkCateInventExp, talkCateInventExpNS:
		n = s.weeklyCount(int64(uid), "inventExp")
	case talkCateCapsuleVIP, talkCateCapsuleNorm:
		n = s.monthlyCount(int64(uid), "inventCapsule")
	case talkCateBalconyExp:
		n = s.dailyCount(int64(uid), talkBalconyExpKey)
	case talkCateNiguDew, talkCateNiguDewVIP:
		n = s.dailyCount(int64(uid), talkNiguDewDailyKey)
	case talkCateXioCoralVIP:
		n = s.dailyCount(int64(uid), talkXioCoralDailyKey)
	case talkCateXioCoral:
		// map56 希欧珊瑚与 map325 黄晶矿共用 cate 17，按当前图区分计数键
		if c.MapID == 56 {
			n = s.dailyCount(int64(uid), talkXioCoralDailyKey)
		} else {
			n = s.dailyCount(int64(uid), key)
		}
	case talkCateWellOreA, talkCateWellOreB:
		n = s.dailyCount(int64(uid), key)
	default:
		n = s.dailyCount(int64(uid), key)
	}
	out := make([]byte, 4)
	binary.BigEndian.PutUint32(out, uint32(n))
	s.send(c, 2701, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d cate=%d count=%d", cmdname.Format(2701), uid, cateID, n)
}

// handleTalkCate CMD 2702：对话领取 / 挖矿。
func (s *Server) handleTalkCate(c *Client, uid uint32, body []byte) {
	cateID := uint32(0)
	if len(body) >= 4 {
		cateID = binary.BigEndian.Uint32(body[0:4])
	}
	outItemID, outCount := uint32(0), uint32(0)
	key := "talk:" + itoaU32(cateID)

	switch cateID {
	case talkCateCaptainCoins:
		if s.tryMarkWeekly(int64(uid), talkCaptainWeekKey) && s.cfg.Store != nil {
			_ = s.cfg.Store.AddCoins(int64(uid), talkCaptainCoinsAmt)
			outItemID, outCount = 1, talkCaptainCoinsAmt
			s.sendAlert(int64(uid), "领取赛尔豆 "+itoaU32(talkCaptainCoinsAmt))
		}
	case talkCateInventExp, talkCateInventExpNS:
		// 发明室：客户端提示「下周再来」→ 周限 1 次（超能/非超能共享）
		weekKey := "inventExp"
		if s.tryMarkWeekly(int64(uid), weekKey) && s.cfg.Store != nil {
			_, _ = s.cfg.Store.AddExpPool(int64(uid), talkInventExpAmt)
			outItemID, outCount = 3, talkInventExpAmt
			s.sendAlert(int64(uid), "领取积累经验 "+itoaU32(talkInventExpAmt))
		}
	case talkCateBalconyExp:
		if s.tryMarkDaily(int64(uid), talkBalconyExpKey) && s.cfg.Store != nil {
			_, _ = s.cfg.Store.AddExpPool(int64(uid), talkBalconyExpAmt)
			outItemID, outCount = 3, talkBalconyExpAmt
			s.sendAlert(int64(uid), "领取积累经验 "+itoaU32(talkBalconyExpAmt))
		}
	case talkCateNiguDew, talkCateNiguDewVIP:
		// map59：日限 5；超能 2055×2，普通 19×1（客户端用 2055 查次数）
		have := s.dailyCount(int64(uid), talkNiguDewDailyKey)
		if have < talkNiguDewDailyMax && s.cfg.Store != nil {
			s.bumpDaily(int64(uid), talkNiguDewDailyKey)
			cnt := uint32(1)
			if cateID == talkCateNiguDewVIP {
				cnt = 2
			}
			_ = s.cfg.Store.AddItem(int64(uid), talkNiguDewItem, int(cnt))
			outItemID, outCount = talkNiguDewItem, cnt
		}
	case talkCateXioCoralVIP:
		outItemID, outCount = s.grantXioCoral(uid, true)
	case talkCateXioCoral:
		if c.MapID == 56 {
			outItemID, outCount = s.grantXioCoral(uid, false)
		} else if s.cfg.Store != nil {
			outItemID, outCount = s.grantMiningCate(uid, cateID, key)
		}
	case talkCateWellOreA, talkCateWellOreB:
		outItemID, outCount = s.grantWellOre(uid, cateID, key)
	case talkCateCapsuleVIP, talkCateCapsuleNorm:
		// 分析机月领：超能/普通共享本月 1 次
		monthKey := "inventCapsule"
		itemID := talkCapsuleNormItem
		if cateID == talkCateCapsuleVIP {
			itemID = talkCapsuleVIPItem
		}
		if s.tryMarkMonthly(int64(uid), monthKey) && s.cfg.Store != nil {
			_ = s.cfg.Store.AddItem(int64(uid), itemID, 1)
			outItemID, outCount = uint32(itemID), 1
		}
	case talkGachaCate:
		if s.tryMarkDaily(int64(uid), key) && s.cfg.Store != nil {
			_ = s.cfg.Store.AddItem(int64(uid), talkGachaItemID, talkGachaCount)
			outItemID, outCount = talkGachaItemID, talkGachaCount
		}
	default:
		if cateID >= 1 && cateID <= 22 && cateID != talkCateXioCoral && s.cfg.Store != nil {
			outItemID, outCount = s.grantMiningCate(uid, cateID, key)
		}
	}

	// DayTalkInfo：cateCount(0) + outCount(条目数) + CateInfo(id,count)
	out := make([]byte, 4) // cateCount=0
	tmp := make([]byte, 4)
	if outCount > 0 {
		binary.BigEndian.PutUint32(tmp, 1)
		out = append(out, tmp...)
		idb := make([]byte, 8)
		binary.BigEndian.PutUint32(idb[0:4], outItemID)
		binary.BigEndian.PutUint32(idb[4:8], outCount)
		out = append(out, idb...)
	} else {
		binary.BigEndian.PutUint32(tmp, 0)
		out = append(out, tmp...)
	}
	s.send(c, 2702, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d cate=%d item=%d x%d", cmdname.Format(2702), uid, cateID, outItemID, outCount)
}

func (s *Server) grantXioCoral(uid uint32, vip bool) (itemID, count uint32) {
	if s.cfg.Store == nil {
		return 0, 0
	}
	have := s.dailyCount(int64(uid), talkXioCoralDailyKey)
	if have >= talkXioCoralDailyMax {
		return 0, 0
	}
	s.bumpDaily(int64(uid), talkXioCoralDailyKey)
	cnt := uint32(1)
	if vip {
		cnt = 2
	}
	_ = s.cfg.Store.AddItem(int64(uid), talkXioCoralItem, int(cnt))
	return talkXioCoralItem, cnt
}

func (s *Server) grantWellOre(uid uint32, cateID uint32, key string) (itemID, count uint32) {
	if s.cfg.Store == nil {
		return 0, 0
	}
	ka := "talk:" + itoaU32(talkCateWellOreA)
	kb := "talk:" + itoaU32(talkCateWellOreB)
	total := s.dailyCount(int64(uid), ka) + s.dailyCount(int64(uid), kb)
	if total >= talkWellOreDailyMax {
		return 0, 0
	}
	s.bumpDaily(int64(uid), key)
	item := uint32(talkWellOreItemA)
	if cateID == talkCateWellOreB {
		item = talkWellOreItemB
	}
	n := uint32(rand.Intn(3) + 1)
	_ = s.cfg.Store.AddItem(int64(uid), int(item), int(n))
	return item, n
}

func (s *Server) grantMiningCate(uid uint32, cateID uint32, key string) (itemID, count uint32) {
	limit := miningDailyLimit[cateID]
	if limit <= 0 {
		limit = 5
	}
	have := s.dailyCount(int64(uid), key)
	if have >= limit {
		return 0, 0
	}
	s.bumpDaily(int64(uid), key)
	id := miningCateToItemID[cateID]
	if id == 0 {
		id = 400001
	}
	n := uint32(rand.Intn(4) + 2)
	_ = s.cfg.Store.AddItem(int64(uid), int(id), int(n))
	return id, n
}

func itoaU32(v uint32) string {
	const digits = "0123456789"
	if v == 0 {
		return "0"
	}
	var b [10]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = digits[v%10]
		v /= 10
	}
	return string(b[i:])
}
