package gameserver

import (
	"encoding/binary"
	"log"

	"niaohao/server/internal/cmdname"
	"niaohao/server/internal/store"
	"niaohao/server/internal/tableloader"
)

// 经典 SPT 精灵 → AchieveXML Branch2 Threshold。
var sptPetToAchieveThreshold = map[int]int{
	47:   301, // 蘑菇怪
	34:   302, // 钢牙鲨
	42:   303, // 里奥斯
	50:   304, // 阿克希亚
	69:   305, // 提亚斯
	70:   306, // 雷伊
	88:   307, // 纳多雷
	113:  308, // 雷纳多
	132:  309, // 尤纳斯
	187:  310, // 魔狮迪露
	216:  311, // 哈莫雷特
	264:  312, // 奈尼芬多
	391:  314, // 塔西亚
	274:  315, // 塔克林
	1337: 315, // 机械塔克林（同「击败塔克林」）
	421:  316, // 厄尔塞拉
	490:  317, // 劳克蒙德
	538:  318, // 克拉尼特
	587:  320, // 墨杜萨
	617:  321, // 肯佩德
	5012: 322, // 亚伦斯（冰之妖兽）
	715:  323, // 德拉萨
}

// 四神兽真身 → Branch21 Threshold。
var fourBeastPetToAchieveThreshold = map[int]int{
	501: 145, // 巴斯特
	502: 146, // 朵拉格
	503: 147, // 泰格尔
}

type sptAchieveHit struct {
	BranchID  int
	Threshold int
}

type achieveBranchView struct {
	BranchID      int
	AchievePoint  int
	Value         int
	CompleteValue int
	TitleIDs      []int
	Status        int
}

// collectSPTAchieveHits 本场击败应点亮的成就（Branch2 / 21 / 31）。
func collectSPTAchieveHits(st *BattleState) []sptAchieveHit {
	if st == nil || st.EnemyID <= 0 {
		return nil
	}
	out := make([]sptAchieveHit, 0, 3)
	if th, ok := sptPetToAchieveThreshold[st.EnemyID]; ok {
		out = append(out, sptAchieveHit{BranchID: 2, Threshold: th})
	}
	// 克拉尼特困难：map435 param2=2 →「再次击败」319
	if st.EnemyID == 538 && st.MapID == 435 && st.BossRegion == 2 {
		out = append(out, sptAchieveHit{BranchID: 2, Threshold: 319})
	}
	if th, ok := fourBeastPetToAchieveThreshold[st.EnemyID]; ok {
		out = append(out, sptAchieveHit{BranchID: 21, Threshold: th})
	}
	// 谱尼真身 → Branch31「战胜谱尼」
	if isPuniSealBoss(st.MapID, st.EnemyID, st.BossRegion) && st.BossRegion == 8 {
		out = append(out, sptAchieveHit{BranchID: 31, Threshold: 1})
	}
	return out
}

func (s *Server) syncSPTAchieveFromDefeated(uid int64) {
	if s.cfg.Store == nil || s.cfg.Catalog == nil {
		return
	}
	keys, err := s.cfg.Store.ListDefeatedSPTKeys(uid)
	if err != nil {
		return
	}
	set := make(map[int]bool, len(keys)*2)
	for _, k := range keys {
		set[k] = true
		// 首杀发奖用 petID 作 key，同步映射到成就 Threshold
		if th, ok := sptPetToAchieveThreshold[k]; ok {
			set[th] = true
		}
		if th, ok := fourBeastPetToAchieveThreshold[k]; ok {
			set[th] = true
		}
	}
	for _, branchID := range []int{2, 21, 31} {
		for _, r := range s.cfg.Catalog.AchieveRulesOfBranch(branchID) {
			if r.Threshold <= 0 || !set[r.Threshold] {
				continue
			}
			_ = s.cfg.Store.UpsertAchieveRule(uid, store.AchieveRuleRow{
				BranchID:  branchID,
				RuleID:    r.RuleID,
				Progress:  1,
				Completed: true,
				Claimed:   r.SpeNameBonus == 0,
			})
		}
	}
}

// onAchieveBattleWin PvE 胜场：SPT / 四神兽 / 谱尼真身写入成就并推 3451。
func (s *Server) onAchieveBattleWin(uid uint32, st *BattleState) {
	if s.cfg.Store == nil || st == nil {
		return
	}
	hits := collectSPTAchieveHits(st)
	if len(hits) == 0 {
		return
	}
	for _, h := range hits {
		s.completeAchieveByThreshold(uid, h.BranchID, h.Threshold, st.EnemyID)
	}
}

func (s *Server) completeAchieveByThreshold(uid uint32, branchID, threshold, enemyPetID int) {
	if s.cfg.Store == nil || s.cfg.Catalog == nil || branchID <= 0 || threshold <= 0 {
		return
	}
	uid64 := int64(uid)
	var matched *tableloader.AchieveRule
	for _, r := range s.cfg.Catalog.AchieveRulesOfBranch(branchID) {
		if r.Threshold != threshold {
			continue
		}
		cp := r
		matched = &cp
		break
	}
	if matched == nil {
		return
	}

	// 先读旧进度（不要先 Mark/sync，否则会误判已完成、漏推 3451）
	already, _ := s.cfg.Store.ListAchieveRulesOfBranch(uid64, branchID)
	wasDone := false
	oldMask := 0
	for _, row := range already {
		if !row.Completed {
			continue
		}
		if row.RuleID == matched.RuleID {
			wasDone = true
		}
		if bit := tableloader.AchieveCompleteBit(branchID, row.RuleID); bit >= 0 && bit < 32 {
			oldMask |= 1 << bit
		}
	}
	if wasDone {
		_ = s.cfg.Store.MarkDefeatedSPT(uid64, threshold)
		return
	}

	_ = s.cfg.Store.MarkDefeatedSPT(uid64, threshold)
	_ = s.cfg.Store.UpsertAchieveRule(uid64, store.AchieveRuleRow{
		BranchID:  branchID,
		RuleID:    matched.RuleID,
		Progress:  1,
		Completed: true,
	})
	if matched.SpeNameBonus > 0 {
		_ = s.cfg.Store.GrantTitle(uid64, matched.SpeNameBonus)
		s.pushAchieveTitle(uid, uint32(matched.SpeNameBonus))
	}
	newMask := oldMask
	if bit := tableloader.AchieveCompleteBit(branchID, matched.RuleID); bit >= 0 && bit < 32 {
		newMask |= 1 << bit
	}
	value := matched.RuleID
	if tableloader.AchieveIsBitmaskBranch(branchID) {
		value = newMask
	}
	s.pushAchieveInform(uid, branchID, matched.AchievementPoint, value, oldMask, newMask)
	log.Printf("[achieve] UID=%d pet=%d branch=%d threshold=%d rule=%d point=%d",
		uid, enemyPetID, branchID, threshold, matched.RuleID, matched.AchievementPoint)
}

// pushAchieveInform CMD 3451：branchId+achievePoint+value+oldValue+completeValue。
func (s *Server) pushAchieveInform(uid uint32, branchID, achievePoint, value, oldValue, completeValue int) {
	body := make([]byte, 20)
	binary.BigEndian.PutUint32(body[0:4], uint32(branchID))
	binary.BigEndian.PutUint32(body[4:8], uint32(achievePoint))
	binary.BigEndian.PutUint32(body[8:12], uint32(value))
	binary.BigEndian.PutUint32(body[12:16], uint32(oldValue))
	binary.BigEndian.PutUint32(body[16:20], uint32(completeValue))
	s.sendToUser(int64(uid), 3451, body)
	log.Printf("[CMD] OK     %s UID=%d branch=%d point=%d value=%#x",
		cmdname.Format(3451), uid, branchID, achievePoint, completeValue)
}

func (s *Server) pushAchieveTitle(uid uint32, titleID uint32) {
	if titleID == 0 {
		return
	}
	out := make([]byte, 4)
	binary.BigEndian.PutUint32(out, titleID)
	s.sendToUser(int64(uid), 70005, out)
}

func (s *Server) buildAchieveBranchView(uid int64, branchID int) achieveBranchView {
	info := achieveBranchView{BranchID: branchID, TitleIDs: []int{}}
	if s.cfg.Store == nil {
		return info
	}
	if branchID == 2 || branchID == 21 || branchID == 31 {
		s.syncSPTAchieveFromDefeated(uid)
	}
	rows, _ := s.cfg.Store.ListAchieveRulesOfBranch(uid, branchID)
	rowByRule := make(map[int]store.AchieveRuleRow, len(rows))
	for _, r := range rows {
		rowByRule[r.RuleID] = r
	}
	_, status, _ := s.cfg.Store.GetAchieveBranchState(uid, branchID)
	info.Status = status

	var rules []tableloader.AchieveRule
	if s.cfg.Catalog != nil {
		rules = s.cfg.Catalog.AchieveRulesOfBranch(branchID)
	}
	points := 0
	for _, rule := range rules {
		rec := rowByRule[rule.RuleID]
		if rec.Progress > info.Value {
			info.Value = rec.Progress
		}
		if !rec.Completed {
			continue
		}
		points += rule.AchievementPoint
		if bit := tableloader.AchieveCompleteBit(branchID, rule.RuleID); bit >= 0 && bit < 32 {
			info.CompleteValue |= 1 << bit
		}
		if rule.SpeNameBonus > 0 {
			info.TitleIDs = append(info.TitleIDs, rule.SpeNameBonus)
			_ = s.cfg.Store.GrantTitle(uid, rule.SpeNameBonus)
			if !rec.Claimed {
				rec.Claimed = true
				_ = s.cfg.Store.UpsertAchieveRule(uid, rec)
			}
		}
	}
	info.AchievePoint = points
	if tableloader.AchieveIsBitmaskBranch(branchID) && info.CompleteValue != 0 {
		info.Value = info.CompleteValue
	}
	return info
}

func (s *Server) achieveCurrentTotals(uid int64) (points, completed int) {
	if s.cfg.Store == nil {
		return 0, 0
	}
	if s.cfg.Catalog != nil {
		s.syncSPTAchieveFromDefeated(uid)
	}
	rows, err := s.cfg.Store.ListAchieveRules(uid)
	if err != nil {
		return 0, 0
	}
	for _, r := range rows {
		if !r.Completed {
			continue
		}
		completed++
		if s.cfg.Catalog != nil {
			if rule, ok := s.cfg.Catalog.AchieveRuleOf(r.BranchID, r.RuleID); ok {
				points += rule.AchievementPoint
			}
		}
	}
	return points, completed
}

func encodeAchieveInfo(v achieveBranchView) []byte {
	n := len(v.TitleIDs)
	out := make([]byte, 20+4*n+4)
	off := 0
	put := func(x int) {
		binary.BigEndian.PutUint32(out[off:off+4], uint32(x))
		off += 4
	}
	put(v.BranchID)
	put(v.AchievePoint)
	put(v.Value)
	put(v.CompleteValue)
	put(n)
	for _, tid := range v.TitleIDs {
		put(tid)
	}
	put(v.Status)
	return out
}

// handleAchieveCurrent CMD 80005：AchieveNewPanel.onGetCurrent（少字节 #2030）：
// tip: branchID0+ruleID0+branchID1+ruleID1（16B），
// 随后 for i=0..6 各读 2×u32，写入 curPanel["txt"+i] 为 "a / b"（再 56B），
// 合计 18×u32 = 72B。首对填 totalPoints/completed，其余 0。
func (s *Server) handleAchieveCurrent(c *Client, uid uint32) {
	const tipBytes = 16
	const statPairs = 7
	out := make([]byte, tipBytes+statPairs*8)
	tips := s.pickAchieveTips(int64(uid), 2)
	for i, t := range tips {
		if i >= 2 {
			break
		}
		binary.BigEndian.PutUint32(out[i*8:i*8+4], uint32(t[0]))
		binary.BigEndian.PutUint32(out[i*8+4:i*8+8], uint32(t[1]))
	}
	points, completed := s.achieveCurrentTotals(int64(uid))
	binary.BigEndian.PutUint32(out[tipBytes:tipBytes+4], uint32(points))
	binary.BigEndian.PutUint32(out[tipBytes+4:tipBytes+8], uint32(completed))
	s.send(c, 80005, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d tip0=%d/%d tip1=%d/%d points=%d done=%d body=%d",
		cmdname.Format(80005), uid,
		binary.BigEndian.Uint32(out[0:4]), binary.BigEndian.Uint32(out[4:8]),
		binary.BigEndian.Uint32(out[8:12]), binary.BigEndian.Uint32(out[12:16]),
		points, completed, len(out))
}

// pickAchieveTips 选 n 个 (branchID, ruleID) 作面板提示；优先未完成。
func (s *Server) pickAchieveTips(uid int64, n int) [][2]int {
	if n <= 0 {
		return nil
	}
	out := make([][2]int, 0, n)
	if s.cfg.Catalog == nil {
		return out
	}
	done := map[[2]int]bool{}
	if s.cfg.Store != nil {
		rows, _ := s.cfg.Store.ListAchieveRules(uid)
		for _, r := range rows {
			if r.Completed {
				done[[2]int{r.BranchID, r.RuleID}] = true
			}
		}
	}
	for _, bid := range s.cfg.Catalog.AchieveBranchOrder {
		for _, rule := range s.cfg.Catalog.AchieveRulesOfBranch(bid) {
			key := [2]int{bid, rule.RuleID}
			if done[key] {
				continue
			}
			out = append(out, key)
			if len(out) >= n {
				return out
			}
		}
	}
	// 不足时用已完成规则补位，保证字段有合法 ID
	if len(out) < n {
		for _, bid := range s.cfg.Catalog.AchieveBranchOrder {
			for _, rule := range s.cfg.Catalog.AchieveRulesOfBranch(bid) {
				key := [2]int{bid, rule.RuleID}
				exists := false
				for _, t := range out {
					if t == key {
						exists = true
						break
					}
				}
				if exists {
					continue
				}
				out = append(out, key)
				if len(out) >= n {
					return out
				}
			}
		}
	}
	return out
}

// handleAchieveInfo CMD 80006：请求 branchId(4) → AchieveInfo。
func (s *Server) handleAchieveInfo(c *Client, uid uint32, body []byte) {
	branchID := 0
	if len(body) >= 4 {
		branchID = int(binary.BigEndian.Uint32(body[0:4]))
	}
	view := s.buildAchieveBranchView(int64(uid), branchID)
	out := encodeAchieveInfo(view)
	s.send(c, 80006, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d branch=%d value=%d complete=%#x titles=%d",
		cmdname.Format(80006), uid, branchID, view.Value, view.CompleteValue, len(view.TitleIDs))
}

// handleActiveAchieve CMD 80003：请求 branchId(4)；应答 ret+branchId+point+icon(16B)。
func (s *Server) handleActiveAchieve(c *Client, uid uint32, body []byte) {
	branchID := 0
	if len(body) >= 4 {
		branchID = int(binary.BigEndian.Uint32(body[0:4]))
	}
	out := make([]byte, 16)
	ret := uint32(1)
	if s.cfg.Catalog != nil && s.cfg.Catalog.AchieveBranchOf(branchID) != nil {
		ret = 0
		if s.cfg.Store != nil {
			val, _, _ := s.cfg.Store.GetAchieveBranchState(int64(uid), branchID)
			_ = s.cfg.Store.SetAchieveBranchState(int64(uid), branchID, val, 1)
		}
		view := s.buildAchieveBranchView(int64(uid), branchID)
		binary.BigEndian.PutUint32(out[4:8], uint32(branchID))
		binary.BigEndian.PutUint32(out[8:12], uint32(view.AchievePoint))
		binary.BigEndian.PutUint32(out[12:16], 1) // icon 占位
	}
	binary.BigEndian.PutUint32(out[0:4], ret)
	s.send(c, 80003, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d branch=%d ret=%d", cmdname.Format(80003), uid, branchID, ret)
}

// handleAchieveList CMD 80004：已完成分支 ID 列表。
func (s *Server) handleAchieveList(c *Client, uid uint32) {
	ids := make([]int, 0)
	if s.cfg.Store != nil {
		if s.cfg.Catalog != nil {
			s.syncSPTAchieveFromDefeated(int64(uid))
		}
		rows, _ := s.cfg.Store.ListAchieveRules(int64(uid))
		seen := map[int]bool{}
		for _, r := range rows {
			if !r.Completed || seen[r.BranchID] {
				continue
			}
			seen[r.BranchID] = true
			ids = append(ids, r.BranchID)
		}
	}
	out := make([]byte, 4+4*len(ids))
	binary.BigEndian.PutUint32(out[0:4], uint32(len(ids)))
	for i, id := range ids {
		binary.BigEndian.PutUint32(out[4+4*i:8+4*i], uint32(id))
	}
	s.send(c, 80004, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d count=%d", cmdname.Format(80004), uid, len(ids))
}
