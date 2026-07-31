package gameserver

import "testing"

func TestResolveChallengeBossSPT(t *testing.T) {
	cases := []struct {
		mapID, param2 int
		wantPet       int
		wantName      string
	}{
		{10, 0, 4150, "拂晓兔"},
		{17, 0, 42, "里奥斯"},
		{17, 1, 42, "里奥斯"},
		{27, 1, 69, "提亚斯"},
		{16, 0, 393, "上古炎兽"},
		{61, 7, 421, "厄尔塞拉特训"},
		{314, 0, 132, "尤纳斯"},
		{53, 1, 187, "魔狮迪露"},
		{348, 0, 274, "塔克林"},
		{348, 2, 216, "哈莫雷特"},
		{320, 1, 144, "赫卡特"},
		{430, 1, 5012, "亚伦斯"},
		{423, 0, 490, "劳克蒙德"},
		{401, 0, 501, "玄武守护兽"},
		{401, 1, 501, "巴斯特"},
		{403, 0, 502, "青龙守护兽"},
		{403, 1, 502, "朵拉格"},
		{483, 0, 490, "白虎守护兽"},
		{483, 1, 5016, "电虎"},
		{483, 3, 5015, "战虎"},
		{483, 5, 503, "泰格尔"},
		{514, 1, 300, "谱尼·虚无封印"},
		{514, 8, 300, "谱尼真身"},
		{108, 5, 300, "谱尼·轮回封印"}, // alias → 514
		{500, 8, 300, "谱尼真身"},       // alias → 514
		{9999, 0, fallbackEnemyPetID, fallbackEnemyName},
	}
	for _, tc := range cases {
		pid, _, name := resolveChallengeBoss(tc.mapID, uint32(tc.param2))
		if pid != tc.wantPet || name != tc.wantName {
			t.Fatalf("map=%d p2=%d got pet=%d name=%q want %d %q", tc.mapID, tc.param2, pid, name, tc.wantPet, tc.wantName)
		}
	}
}

func TestResolveBossFixedHP(t *testing.T) {
	cases := []struct {
		petID, orig, mapID int
		region             uint32
		want               int
	}{
		{47, 999, 12, 0, 100},
		{70, 999, 32, 0, 2000},
		{187, 1, 53, 1, 3000000},
		{300, 1, 514, 1, 5000},
		{300, 1, 514, 8, 10000},
		{501, 1, 401, 1, 50000},
		{501, 1, 401, 0, 5000},
		{502, 1, 403, 1, 50000},
		{502, 1, 403, 0, 10000},
		{503, 1, 483, 5, 50000},
		{5015, 1, 483, 3, 1500},
		{490, 1, 483, 0, 5000},
		{490, 1, 423, 0, 2500},
		{216, 1, 423, 5, 1900},
		{4150, 888, 10, 0, 888}, // 拂晓兔无固定表，保留公式血
	}
	for _, tc := range cases {
		got := resolveBossFixedHP(tc.petID, tc.orig, tc.mapID, tc.region)
		if got != tc.want {
			t.Fatalf("pet=%d map=%d r=%d got %d want %d", tc.petID, tc.mapID, tc.region, got, tc.want)
		}
	}
}

func TestBossTraits(t *testing.T) {
	if !isStatusImmuneBoss(70) || !isStatusImmuneBoss(261) {
		t.Fatal("雷伊/盖亚应免疫异常")
	}
	if isControlImmuneBoss(50) {
		t.Fatal("阿克希亚不应免疫控制（雷伊特训）")
	}
	if !isControlImmuneBoss(47) {
		t.Fatal("蘑菇怪应免疫控制")
	}
	if !isStatDropImmuneBoss(88) || !isInfinitePPBoss(70) {
		t.Fatal("纳多雷降能力免疫 / 雷伊无限PP")
	}
	if bossPriorityBonus(70, 100, 2000) != 6 {
		t.Fatal("雷伊先制+6")
	}
	if bossPriorityBonus(187, 2000000, 3000000) != 0 {
		t.Fatal("魔狮满血无先制加成")
	}
	if bossPriorityBonus(187, 1000000, 3000000) != 6 {
		t.Fatal("魔狮半血先制+6")
	}
	if bossDamageTakenMultiplier(187) != 10 {
		t.Fatal("魔狮受伤×10")
	}
	if canApplyEnemyStatus(70, 10) || canApplyEnemyStatus(47, 16) {
		t.Fatal("状态/控制免疫应拦截")
	}
	if !isStatusImmuneBoss(393) || !isStatDropImmuneBoss(393) {
		t.Fatal("上古炎兽应免疫异常+降能力")
	}
	if !isControlImmuneBoss(299) || !isControlImmuneBoss(386) || !isControlImmuneBoss(462) {
		t.Fatal("猛虎王/基维奥拉/阿尔达拉应免疫控制")
	}
}

func TestBossInnateAtkPlus2(t *testing.T) {
	if !shouldBossInnateAtkPlus2(70) || !shouldBossInnateAtkPlus2(261) || !shouldBossInnateAtkPlus2(393) {
		t.Fatal("雷伊/盖亚/上古炎兽应天生攻击+2")
	}
	if shouldBossInnateAtkPlus2(7) {
		t.Fatal("普通精灵不应天生攻击+2")
	}
	st := &BattleState{EnemyID: 70}
	applyBossInnateStages(st)
	if st.EnemyStages[stageAtk] != 2 {
		t.Fatalf("雷伊 EnemyStages atk=%d want 2", st.EnemyStages[stageAtk])
	}
	st = &BattleState{EnemyID: 1403}
	applyBossInnateStages(st)
	if st.EnemyStages[stageAtk] != 0 || st.EnemyStages[stageDef] != 2 {
		t.Fatalf("萨洛奇斯 want def+2 got atk=%d def=%d", st.EnemyStages[stageAtk], st.EnemyStages[stageDef])
	}
	st = &BattleState{EnemyID: 1397}
	applyBossInnateStages(st)
	if st.EnemyStages[stageAtk] != 2 {
		t.Fatalf("查迪斯 atk=%d want 2", st.EnemyStages[stageAtk])
	}
	info := buildFightPetInfo(0, 70, "雷伊", 0, 2000, 2000, 70, 0, encodeBattleLv(st.EnemyStages))
	if len(info) != 50 {
		t.Fatalf("FightPetInfo len=%d want 50", len(info))
	}
	// 上一场是查迪斯；单独测雷伊 battleLv 字节
	info = buildFightPetInfo(0, 70, "雷伊", 0, 2000, 2000, 70, 0, encodeBattleLv([5]int8{2, 0, 0, 0, 0}))
	if info[44] != 2 || info[45] != 0 {
		t.Fatalf("battleLv bytes atk=%d def=%d want 2,0", int8(info[44]), int8(info[45]))
	}
}

func TestSPTFirstKillExtras(t *testing.T) {
	cases := []struct {
		pet, item, rewardPet int
	}{
		{393, 400133, 0},
		{299, 400119, 0},
		{462, 400147, 0},
		{386, 0, 385},
	}
	for _, tc := range cases {
		rew, ok := sptFirstKillByPetID[tc.pet]
		if !ok || rew.RewardItemID != tc.item || rew.RewardPetID != tc.rewardPet {
			t.Fatalf("pet=%d got %+v want item=%d pet=%d", tc.pet, rew, tc.item, tc.rewardPet)
		}
	}
	if _, ok := sptFirstKillByPetID[4150]; ok {
		t.Fatal("拂晓兔不应有首杀表")
	}
}

func TestApplyBossFixedHPState(t *testing.T) {
	st := &BattleState{EnemyID: 70, MapID: 32, BossRegion: 0, EnemyHP: 99, EnemyMaxHP: 99}
	applyBossFixedHP(st)
	if st.EnemyHP != 2000 || st.EnemyMaxHP != 2000 {
		t.Fatalf("雷伊固定血 got %d/%d", st.EnemyHP, st.EnemyMaxHP)
	}
}

func TestApplyBossHalfHPOneShot(t *testing.T) {
	st := &BattleState{EnemyID: 187, EnemyHP: 100, EnemyMaxHP: 300, PlayerHP: 500}
	if got := applyBossHalfHPOneShot(st, 10, true); got != 500 {
		t.Fatalf("oneshot got %d", got)
	}
	st.EnemyHP = 200
	if got := applyBossHalfHPOneShot(st, 10, true); got != 10 {
		t.Fatalf("above half got %d", got)
	}
}
