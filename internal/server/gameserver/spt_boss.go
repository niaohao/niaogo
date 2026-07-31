package gameserver

// mapBossByParam：(mapID, param2) → BOSS。
// 包体以本客户端 MapProcess / fightWithBoss 为准；经典 SPT 对照 PetBook Foundin。
// 地图 10 必须保持拂晓兔(4150)，勿覆盖成参考服其它 BOSS。

var mapBossByParam = map[int]map[uint32]struct {
	PetID int
	Level int
	Name  string
}{
	// —— 本客户端主链路 ——
	10: {0: {PetID: 4150, Level: 20, Name: "拂晓兔"}},

	// —— 第一星系经典 SPT ——
	12: {
		0: {PetID: 47, Level: 10, Name: "蘑菇怪"},
		1: {PetID: 83, Level: 5, Name: "依依"},
	},
	17: {
		0: {PetID: 42, Level: 35, Name: "里奥斯"},
		1: {PetID: 42, Level: 35, Name: "里奥斯"},
	},
	21:  {0: {PetID: 34, Level: 25, Name: "钢牙鲨"}},
	22:  {0: {PetID: 34, Level: 25, Name: "钢牙鲨"}},
	27: {
		0: {PetID: 69, Level: 50, Name: "提亚斯"},
		1: {PetID: 69, Level: 50, Name: "提亚斯"},
	},
	40: {
		0: {PetID: 50, Level: 65, Name: "阿克希亚"},
		1: {PetID: 50, Level: 65, Name: "阿克希亚"},
	},
	49: {
		0: {PetID: 113, Level: 75, Name: "雷纳多"},
		1: {PetID: 113, Level: 75, Name: "雷纳多"},
	},
	106: {0: {PetID: 88, Level: 70, Name: "纳多雷"}},

	// —— 第二星系及常见 BOSS（等级/param 以本客户端 MapProcess 为准，固定血量对照参考服）——
	5:   {0: {PetID: 299, Level: 75, Name: "猛虎王"}},
	16:  {0: {PetID: 393, Level: 100, Name: "上古炎兽"}},
	15:  {1: {PetID: 527, Level: 80, Name: "赫尔托克"}}, // MapProcess_15 fightWithBoss(...,1)
	34:  {0: {PetID: 74, Level: 17, Name: "果冻鸭"}},
	48:  {0: {PetID: 1337, Level: 120, Name: "机械塔克林"}},
	51:  {0: {PetID: 386, Level: 70, Name: "基维奥拉"}},
	53:  {1: {PetID: 187, Level: 50, Name: "魔狮迪露"}}, // 客户端 fightWithBoss(...,1)
	59:  {0: {PetID: 347, Level: 70, Name: "远古鱼龙"}},
	105: {0: {PetID: 91, Level: 17, Name: "悠悠"}},
	110: {0: {PetID: 169, Level: 45, Name: "卡特斯"}}, // 试炼之门；首胜精元 400109
	26:  {0: {PetID: 153, Level: 15, Name: "小莹蜂"}},

	// —— 暗黑武斗场门内图（本客户端 503–513；首胜精元见 spt_reward）——
	503: { // 第1门
		0: {PetID: 171, Level: 50, Name: "魔牙鲨"},
	},
	504: { // 第2门
		0: {PetID: 174, Level: 60, Name: "贝鲁基德"},
	},
	505: { // 第3门
		0: {PetID: 177, Level: 70, Name: "巴弗洛"},
		1: {PetID: 183, Level: 75, Name: "奇拉塔顿"},
	},
	506: { // 第4门
		0: {PetID: 195, Level: 80, Name: "西萨拉斯"},
		1: {PetID: 192, Level: 85, Name: "克林卡修"},
	},
	507: { // 第5门
		0: {PetID: 222, Level: 90, Name: "卡库"},
		1: {PetID: 224, Level: 90, Name: "赫德卡"},
		2: {PetID: 227, Level: 95, Name: "伊兰罗尼"},
	},
	508: { // 第6门
		0: {PetID: 356, Level: 100, Name: "斯加尔卡"},
		1: {PetID: 297, Level: 100, Name: "艾尔伊洛"},
		2: {PetID: 359, Level: 100, Name: "布林克克"},
	},
	509: { // 第7门
		0: {PetID: 438, Level: 100, Name: "魔花使者"},
		1: {PetID: 441, Level: 100, Name: "莫尔加斯"},
		2: {PetID: 435, Level: 100, Name: "萨诺拉斯"},
	},
	510: { // 第8门
		0: {PetID: 656, Level: 100, Name: "帕多尼"},
		1: {PetID: 659, Level: 100, Name: "加洛德"},
		2: {PetID: 661, Level: 100, Name: "萨多拉尼"},
	},
	511: { // 第9门
		0: {PetID: 779, Level: 100, Name: "迪普利德"},
		1: {PetID: 784, Level: 100, Name: "拜洛亚斯"},
		2: {PetID: 782, Level: 100, Name: "阿克诺亚"},
	},
	512: { // 第10门
		0: {PetID: 1182, Level: 100, Name: "鳞甲魔鱼"},
		1: {PetID: 1185, Level: 100, Name: "艾斯德克"},
		2: {PetID: 1187, Level: 100, Name: "狂狮迪卡"},
	},
	513: { // 第11门
		0: {PetID: 1403, Level: 100, Name: "萨洛奇斯"},
		1: {PetID: 1397, Level: 100, Name: "查迪斯"},
		2: {PetID: 1400, Level: 100, Name: "奈尼狄亚"},
	},
	32:  {0: {PetID: 70, Level: 70, Name: "雷伊"}},
	57:  {0: {PetID: 216, Level: 80, Name: "哈莫雷特"}},
	60:  {0: {PetID: 216, Level: 80, Name: "哈莫雷特"}},
	61: {
		// MapProcess_61：fightWithBoss(..., getDay())，周日=0…周六=6；7=特训
		0: {PetID: 421, Level: 80, Name: "厄尔塞拉"},
		1: {PetID: 421, Level: 80, Name: "厄尔塞拉"},
		2: {PetID: 421, Level: 80, Name: "厄尔塞拉"},
		3: {PetID: 421, Level: 80, Name: "厄尔塞拉"},
		4: {PetID: 421, Level: 80, Name: "厄尔塞拉"},
		5: {PetID: 421, Level: 80, Name: "厄尔塞拉"},
		6: {PetID: 421, Level: 80, Name: "厄尔塞拉"},
		// 特训仍用 421：客户端无 fightResource/pet/swf/5019.swf，用厄尔塞拉本体资源
		7: {PetID: 421, Level: 80, Name: "厄尔塞拉特训"},
	},
	305: {0: {PetID: 102, Level: 40, Name: "奇塔"}},
	306: {0: {PetID: 59, Level: 15, Name: "西塔"}},
	307: {0: {PetID: 59, Level: 15, Name: "西塔"}},
	310: {0: {PetID: 74, Level: 17, Name: "果冻鸭"}},
	314: {0: {PetID: 132, Level: 70, Name: "尤纳斯"}},
	315: {0: {PetID: 128, Level: 15, Name: "波古"}},
	320: {
		0: {PetID: 144, Level: 60, Name: "赫尔卡巨人"}, // 与卡塔同 param2=0，包体无名字
		1: {PetID: 144, Level: 60, Name: "赫卡特"},
	},
	325: {0: {PetID: 264, Level: 60, Name: "奈尼芬多"}},
	329: {0: {PetID: 287, Level: 18, Name: "厄斯"}},
	342: {0: {PetID: 414, Level: 18, Name: "乌普"}},
	348: { // MapProcess_348：塔克林0 / 塔西亚1 / 哈莫2 / 塞维尔3
		0: {PetID: 274, Level: 80, Name: "塔克林"},
		1: {PetID: 391, Level: 70, Name: "塔西亚"},
		2: {PetID: 216, Level: 80, Name: "哈莫雷特"},
		3: {PetID: 413, Level: 75, Name: "塞维尔"},
	},
	404: {0: {PetID: 454, Level: 17, Name: "霹雳兽"}},
	414: {1: {PetID: 471, Level: 15, Name: "伊特"}},
	415: {0: {PetID: 474, Level: 17, Name: "该伊"}},
	419: {0: {PetID: 261, Level: 70, Name: "盖亚"}},
	423: {
		0: {PetID: 490, Level: 80, Name: "劳克蒙德"},
		1: {PetID: 490, Level: 80, Name: "劳克蒙德"},
		2: {PetID: 490, Level: 80, Name: "劳克蒙德"},
		4: {PetID: 70, Level: 70, Name: "雷伊"},
		5: {PetID: 216, Level: 80, Name: "哈莫雷特"},
	},
	425: {1: {PetID: 499, Level: 18, Name: "阿零"}},
	430: {1: {PetID: 5012, Level: 80, Name: "亚伦斯"}},
	435: {
		0: {PetID: 538, Level: 80, Name: "克拉尼特"},
		1: {PetID: 547, Level: 60, Name: "紫炎虫"},
		2: {PetID: 538, Level: 80, Name: "克拉尼特困难模式"},
	},
	438: {0: {PetID: 587, Level: 80, Name: "墨杜萨"}},
	441: {0: {PetID: 617, Level: 80, Name: "肯佩德"}},
	486: {
		0: {PetID: 715, Level: 80, Name: "德拉萨"},
		1: {PetID: 715, Level: 80, Name: "德拉萨"},
	},
	565: {0: {PetID: 462, Level: 80, Name: "阿尔达拉"}},

	// —— 四神兽（以本前端 MapProcess_401/403/483 的 fightWithBoss 为准）——
	// 玄武(401)：守护兽 p0 → 巴斯特真身 p1（非参考服 0..5+6）
	401: {
		0: {PetID: 501, Level: 100, Name: "玄武守护兽"},
		1: {PetID: 501, Level: 120, Name: "巴斯特"},
	},
	// 青龙(403)：守护兽 p0 → 朵拉格真身 p1
	403: {
		0: {PetID: 502, Level: 100, Name: "青龙守护兽"},
		1: {PetID: 502, Level: 120, Name: "朵拉格"},
	},
	// 白虎(483)：p0 守护；p1/4 电虎；p2/3 战虎；p5 泰格尔（p1 亦被战白虎占用，服侧出 5016）
	483: {
		0: {PetID: 490, Level: 100, Name: "白虎守护兽"},
		1: {PetID: 5016, Level: 105, Name: "电虎"},
		2: {PetID: 5015, Level: 105, Name: "战虎"},
		3: {PetID: 5015, Level: 105, Name: "战虎"},
		4: {PetID: 5016, Level: 105, Name: "电虎"},
		5: {PetID: 503, Level: 120, Name: "泰格尔"},
	},

	// —— 谱尼多封印（勇者之塔神秘领域 514）：param2=1..7 封印、8 真身、0 任务首次 ——
	514: {
		0: {PetID: 300, Level: 120, Name: "谱尼"},
		1: {PetID: 300, Level: 120, Name: "谱尼·虚无封印"},
		2: {PetID: 300, Level: 120, Name: "谱尼·元素封印"},
		3: {PetID: 300, Level: 120, Name: "谱尼·能量封印"},
		4: {PetID: 300, Level: 120, Name: "谱尼·生命封印"},
		5: {PetID: 300, Level: 120, Name: "谱尼·轮回封印"},
		6: {PetID: 300, Level: 120, Name: "谱尼·永恒封印"},
		7: {PetID: 300, Level: 120, Name: "谱尼·圣洁封印"},
		8: {PetID: 300, Level: 120, Name: "谱尼真身"},
	},
}

// bossMapAlias 任务/子场景 mapID → 正式 BOSS 地图。
var bossMapAlias = map[int]int{
	500: 514, // 勇者之塔进神秘领域时可能仍为 500
	108: 514, // 太空站左翼：点击封印时 mapID 常为 108
}

const (
	petIDPuni          = 300
	puniEssenceItemID  = 400150
	puniSealDefeatBase = 30000 // Defeated key = 30000 + region
)

// puniFragmentItemIDs region(1..8) → 裂片道具。
var puniFragmentItemIDs = map[uint32]int{
	1: 400651, // 虚无
	2: 400652, // 元素
	3: 400653, // 能量
	4: 400654, // 生命
	5: 400655, // 轮回
	6: 400656, // 永恒
	7: 400657, // 圣洁
	8: 400658, // 真身
}

var puniSealHP = map[uint32]int{
	0: 6500,
	1: 5000,
	2: 6000,
	3: 7000,
	4: 8000,
	5: 10000,
	6: 13000,
	7: 16000,
	8: 10000,
}

func getPuniFragmentItemID(region uint32) int {
	return puniFragmentItemIDs[region]
}

func isPuniSealBoss(mapID int, enemyID int, region uint32) bool {
	if enemyID != petIDPuni {
		return false
	}
	if alias, ok := bossMapAlias[mapID]; ok {
		mapID = alias
	}
	if mapID != 514 {
		return false
	}
	return region <= 8
}

func puniSealMaxHP(region uint32) int {
	if hp, ok := puniSealHP[region]; ok && hp > 0 {
		return hp
	}
	return 0
}

