package tableloader

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// Catalog 静态表内存索引（GM 与玩法共用）。
type Catalog struct {
	mu             sync.RWMutex
	ItemName       map[int]string // itemID -> 中文名
	ItemCat        map[int]int    // itemID -> Cat
	ItemPrice      map[int]int    // itemID -> Price（赛尔豆）
	ItemCatchBonus map[int]float64 // itemID -> 胶囊 Bonus（items.xml）
	ItemMeta       map[int]ItemMeta
	NonoItem       map[int]NonoItemFx // 700xxx 芯片/玩具
	EssenceBreed   map[int]EssenceBreed // itemID -> 精元孵化
	SoulBeads      map[int]SoulBeadDef  // itemID -> 元神珠
	HatchTasks     map[int]HatchTaskDef // itemID -> 吸能任务步数
	itemNewSe      map[int]int          // itemID -> NewSeIdx
	petEffect      map[int]PetEffectDef
	PetName        map[int]string // petID -> 中文名（pets.xml）
	PetType        map[int]int    // petID -> Type
	PetBaseMap     map[int]PetBaseDef
	FrontendPetName map[int]string // petID -> 前端 PetXMLInfo DefName（发放过滤）
	BreedEggs      map[int]BreedEggDef // eggID -> 配方
	BreedPairEgg   map[uint64]int      // male<<32|female -> eggID
	DualEvTimes    map[int]int // itemID -> 学习力双倍仪次数
	DualExpTimes   map[int]int // itemID -> 双倍经验场数
	TrinalExpTimes map[int]int // itemID -> 三倍经验场数
	EnergyAbsTimes map[int]int // itemID -> 能量吸收器次数
	AutoBtlTimes   map[int]int // itemID -> 自动战斗器回合
	Skills         map[int]SkillDef
	GoldProducts       map[uint32]GoldProduct
	MoneyProducts      map[uint32]MoneyProduct
	EquipBySendID      map[uint32]EquipUpgrade
	AchieveByBranch    map[int]*AchieveBranch
	AchieveRules       map[int]AchieveRule // branch*100+rule
	AchieveBranchOrder []int
	loaded             bool
	xmlDir             string
}

func New(xmlDir string) *Catalog {
	return &Catalog{
		ItemName:       make(map[int]string),
		ItemCat:        make(map[int]int),
		ItemPrice:      make(map[int]int),
		ItemCatchBonus: make(map[int]float64),
		ItemMeta:       make(map[int]ItemMeta),
		NonoItem:      make(map[int]NonoItemFx),
		EssenceBreed:  make(map[int]EssenceBreed),
		SoulBeads:     make(map[int]SoulBeadDef),
		HatchTasks:    make(map[int]HatchTaskDef),
		itemNewSe:     make(map[int]int),
		petEffect:     make(map[int]PetEffectDef),
		PetName:       make(map[int]string),
		PetType:       make(map[int]int),
		PetBaseMap:      make(map[int]PetBaseDef),
		FrontendPetName: make(map[int]string),
		BreedEggs:       make(map[int]BreedEggDef),
		BreedPairEgg:    make(map[uint64]int),
		DualEvTimes:     make(map[int]int),
		DualExpTimes:   make(map[int]int),
		TrinalExpTimes: make(map[int]int),
		EnergyAbsTimes: make(map[int]int),
		AutoBtlTimes:   make(map[int]int),
		Skills:         make(map[int]SkillDef),
		GoldProducts:    make(map[uint32]GoldProduct),
		MoneyProducts:   make(map[uint32]MoneyProduct),
		EquipBySendID:   make(map[uint32]EquipUpgrade),
		AchieveByBranch: make(map[int]*AchieveBranch),
		AchieveRules:    make(map[int]AchieveRule),
		xmlDir:          xmlDir,
	}
}

func (c *Catalog) Load() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.loadItems(filepath.Join(c.xmlDir, "items.xml")); err != nil {
		return err
	}
	if err := c.loadPets(filepath.Join(c.xmlDir, "pets.xml")); err != nil {
		return err
	}
	feLoaded := false
	for _, p := range frontendPetXMLPaths(c.xmlDir) {
		if err := c.loadFrontendPetNames(p); err == nil && len(c.FrontendPetName) > 0 {
			fmt.Printf("[tables] frontend pets: %d named from %s\n", len(c.FrontendPetName), p)
			feLoaded = true
			break
		}
	}
	if !feLoaded {
		fmt.Printf("[tables] frontend PetXMLInfo missing; grant-all will use pets.xml only\n")
	}
	if err := c.loadSkills(skillXMLPath(c.xmlDir)); err != nil {
		fmt.Printf("[tables] skills skip: %v\n", err)
	}
	if err := c.loadPetEffects(petEffectXMLPath(c.xmlDir)); err != nil {
		fmt.Printf("[tables] petEffect skip: %v\n", err)
	}
	if err := c.loadGoldProducts(productXMLPath(c.xmlDir, "GoldProductXMLInfo.xml")); err != nil {
		fmt.Printf("[tables] goldProduct skip: %v\n", err)
	}
	if err := c.loadMoneyProducts(productXMLPath(c.xmlDir, "MoneyProductXMLInfo.xml")); err != nil {
		fmt.Printf("[tables] moneyProduct skip: %v\n", err)
	}
	if err := c.loadEquipUpgrades(productXMLPath(c.xmlDir, "EquipXmlConfig.xml")); err != nil {
		fmt.Printf("[tables] equipConfig skip: %v\n", err)
	}
	if err := c.loadAchieve(achieveXMLPath(c.xmlDir)); err != nil {
		fmt.Printf("[tables] achieve skip: %v\n", err)
	}
	if err := c.loadPetShop(productXMLPath(c.xmlDir, "PetShopXMLInfo.xml")); err != nil {
		fmt.Printf("[tables] petShop skip: %v\n", err)
	}
	if err := LoadEvolveXML(filepath.Join(c.xmlDir, "EvolveXMLInfo.xml")); err != nil {
		fmt.Printf("[tables] evolve skip: %v\n", err)
	}
	if err := c.loadHatchTaskXML(filepath.Join(c.xmlDir, "HatchTaskXMLInfo.xml")); err != nil {
		fmt.Printf("[tables] hatchTask skip: %v\n", err)
	}
	if err := c.loadEggs(filepath.Join(c.xmlDir, "EggsXMLInfo.xml")); err != nil {
		fmt.Printf("[tables] eggs skip: %v\n", err)
	}
	if err := LoadNatureXML(filepath.Join(c.xmlDir, "NatureXMLInfo.xml")); err != nil {
		fmt.Printf("[tables] nature skip: %v\n", err)
	}
	c.loaded = true
	return nil
}

// DualEvTimesOf 学习力双倍仪增加的剩余次数；无配置返回 0。
func (c *Catalog) DualEvTimesOf(itemID int) int {
	if c == nil {
		return 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.DualEvTimes[itemID]
}

// DualExpTimesOf 双倍经验加速器场数。
func (c *Catalog) DualExpTimesOf(itemID int) int {
	if c == nil {
		return 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.DualExpTimes[itemID]
}

// TrinalExpTimesOf 三倍经验加速器场数。
func (c *Catalog) TrinalExpTimesOf(itemID int) int {
	if c == nil {
		return 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.TrinalExpTimes[itemID]
}

// EnergyAbsTimesOf 能量吸收器次数。
func (c *Catalog) EnergyAbsTimesOf(itemID int) int {
	if c == nil {
		return 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.EnergyAbsTimes[itemID]
}

// AutoBtlTimesOf 自动战斗器回合数。
func (c *Catalog) AutoBtlTimesOf(itemID int) int {
	if c == nil {
		return 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.AutoBtlTimes[itemID]
}

func (c *Catalog) ItemLabel(id int) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if n, ok := c.ItemName[id]; ok && n != "" {
		return fmt.Sprintf("%s(%d)", n, id)
	}
	return fmt.Sprintf("未知(%d)", id)
}

func (c *Catalog) PetNameOf(id int) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.PetName[id]
}

func (c *Catalog) PetLabel(id int) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if n, ok := c.PetName[id]; ok && n != "" {
		return fmt.Sprintf("%s(%d)", n, id)
	}
	return fmt.Sprintf("未知(%d)", id)
}

// SearchHit 检索命中（名+ID）。
type SearchHit struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Label string `json:"label"`
}

// SearchItems 按 ID 精确或中文名模糊；最多 limit 条。
func (c *Catalog) SearchItems(q string, limit int) []SearchHit {
	return c.searchMap(c.snapshotItemNames(), q, limit)
}

// SearchPets 按 ID 精确或中文名模糊。
func (c *Catalog) SearchPets(q string, limit int) []SearchHit {
	return c.searchMap(c.snapshotPetNames(), q, limit)
}

func (c *Catalog) snapshotItemNames() map[int]string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	m := make(map[int]string, len(c.ItemName))
	for k, v := range c.ItemName {
		m[k] = v
	}
	return m
}

func (c *Catalog) snapshotPetNames() map[int]string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	m := make(map[int]string, len(c.PetName))
	for k, v := range c.PetName {
		m[k] = v
	}
	return m
}

func (c *Catalog) searchMap(names map[int]string, q string, limit int) []SearchHit {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	q = strings.TrimSpace(q)
	out := make([]SearchHit, 0, limit)
	if q == "" {
		return out
	}
	if id, err := strconv.Atoi(q); err == nil && id > 0 {
		name := names[id]
		label := fmt.Sprintf("未知(%d)", id)
		if name != "" {
			label = fmt.Sprintf("%s(%d)", name, id)
		}
		return []SearchHit{{ID: id, Name: name, Label: label}}
	}
	ql := strings.ToLower(q)
	for id, name := range names {
		if name == "" {
			continue
		}
		if strings.Contains(strings.ToLower(name), ql) {
			out = append(out, SearchHit{ID: id, Name: name, Label: fmt.Sprintf("%s(%d)", name, id)})
			if len(out) >= limit {
				break
			}
		}
	}
	return out
}

func (c *Catalog) PetTypeID(id int) int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.PetType[id]
}

func (c *Catalog) Stats() (items, pets int, loaded bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.ItemName), len(c.PetName), c.loaded
}

func (c *Catalog) StatsFull() (items, pets, skills int, loaded bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.ItemName), len(c.PetName), len(c.Skills), c.loaded
}

type itemsRoot struct {
	Cats []itemCat `xml:"Cat"`
}

type itemCat struct {
	ID    string     `xml:"ID,attr"`
	Name  string     `xml:"Name,attr"`
	Items []itemNode `xml:"Item"`
}

type itemNode struct {
	ID             string `xml:"ID,attr"`
	Name           string `xml:"Name,attr"`
	Price          string `xml:"Price,attr"`
	NewSeIdx       string `xml:"NewSeIdx,attr"`
	HP             string `xml:"HP,attr"`
	PP             string `xml:"PP,attr"`
	IncreMonLv     string `xml:"IncreMonLv,attr"`
	DecreMonLv     string `xml:"DecreMonLv,attr"`
	Exp            string `xml:"Exp,attr"`
	MaxHPUp        string `xml:"MaxHPUp,attr"`
	MonAttrReset   string `xml:"MonAttrReset,attr"`
	MonNatureReset string `xml:"MonNatureReset,attr"`
	RandomDv       string `xml:"RandomDv,attr"`
	Nature         string `xml:"Nature,attr"`
	NatureSet      string `xml:"NatureSet,attr"`
	EvRemove       string `xml:"EvRemove,attr"`
	Color          string `xml:"Color,attr"`
	AddPower       string `xml:"AddPower,attr"`
	AddCloseness   string `xml:"AddCloseness,attr"`
	AddIQ          string `xml:"AddIQ,attr"`
	UseAI          string `xml:"UseAI,attr"`
	UsePower       string `xml:"UsePower,attr"`
	BreedTime      string `xml:"BreedTime,attr"`
	VipBreedTime   string `xml:"VipBreedTime,attr"`
	BreedMonID     string `xml:"BreedMonID,attr"`
	BreedMonLv     string `xml:"BreedMonLv,attr"`
	TransmuteTm    string `xml:"TransmuteTm,attr"`
	VipTransmuteTm string `xml:"VipTransmuteTm,attr"`
	TransmuteMon   string `xml:"TransmuteMon,attr"`
	Bonus              string `xml:"Bonus,attr"`
	NewSeReset         string `xml:"NewSeReset,attr"`
	NonFuseAddNewse    string `xml:"NonFuseAddNewse,attr"`
	NonFuseResetNewse  string `xml:"NonFuseResetNewse,attr"`
	DualEvTimes        string `xml:"DualEvTimes,attr"`
	DualEffectTimes    string `xml:"DualEffectTimes,attr"`
	TrinalEffectTimes  string `xml:"TrinalEffectTimes,attr"`
	EnergyAbsorbTimes  string `xml:"EnergyAbsorbTimes,attr"`
	AutoBtlTimes       string `xml:"AutoBtlTimes,attr"`
	AddDv              string `xml:"AddDv,attr"`
	YuanshenDegrade    string `xml:"YuanshenDegrade,attr"`
}

// NonoItemFx NoNo 芯片/玩具属性（items.xml 700xxx）。
type NonoItemFx struct {
	Color        int // -1=无 Color 属性；0=黑色合法
	HasColor     bool
	AddPower     int
	AddCloseness int
	AddIQ        int
	UseAI        int
	UsePower     int
}

func (c *Catalog) loadItems(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read items: %w", err)
	}
	var root itemsRoot
	if err := xml.Unmarshal(b, &root); err != nil {
		return fmt.Errorf("parse items: %w", err)
	}
	if c.ItemMeta == nil {
		c.ItemMeta = make(map[int]ItemMeta)
	}
	if c.ItemCatchBonus == nil {
		c.ItemCatchBonus = make(map[int]float64)
	}
	if c.NonoItem == nil {
		c.NonoItem = make(map[int]NonoItemFx)
	}
	for _, cat := range root.Cats {
		catID, _ := strconv.Atoi(cat.ID)
		for _, it := range cat.Items {
			id, err := strconv.Atoi(it.ID)
			if err != nil {
				continue
			}
			c.ItemName[id] = it.Name
			c.ItemCat[id] = catID
			if p, err := strconv.Atoi(it.Price); err == nil && p > 0 && p < 100000000 {
				c.ItemPrice[id] = p
			}
			if it.Bonus != "" {
				if b, err := strconv.ParseFloat(it.Bonus, 64); err == nil && b > 0 {
					c.ItemCatchBonus[id] = b
				}
			}
			if ns, err := strconv.Atoi(it.NewSeIdx); err == nil && ns > 0 {
				c.itemNewSe[id] = ns
			}
			meta := ItemMeta{
				ID:             id,
				HP:             atoiDefault(it.HP, 0),
				PP:             atoiDefault(it.PP, 0),
				IncreMonLv:     atoiDefault(it.IncreMonLv, 0),
				DecreMonLv:     it.DecreMonLv == "1",
				ExpGrant:       atoiDefault(it.Exp, 0),
				MaxHPUp:        atoiDefault(it.MaxHPUp, 0),
				MonAttrReset:   it.MonAttrReset == "1",
				MonNatureReset: it.MonNatureReset == "1",
				RandomDv:       it.RandomDv == "1",
				NaturePool:     parseIntListAttr(it.Nature),
				NatureSet:      parseIntListAttr(it.NatureSet),
				NewSeIdx:       atoiDefault(it.NewSeIdx, 0),
				EvRemove:       atoiDefault(it.EvRemove, 0),
				NewSeReset:       it.NewSeReset == "1",
				NonFuseAddNewse:  it.NonFuseAddNewse == "1",
				NonFuseResetNewse: atoiDefault(it.NonFuseResetNewse, 0),
				AddDv:            atoiDefault(it.AddDv, 0),
				YuanshenDegrade:  it.YuanshenDegrade == "1",
			}
			// 天赋平衡药剂Ω：xml 无专用属性，按 ID 识别
			if id == 300790 {
				meta.BalanceDv = true
			}
			if meta.HasEffect() {
				c.ItemMeta[id] = meta
			}
			if n := atoiDefault(it.DualEvTimes, 0); n > 0 {
				if c.DualEvTimes == nil {
					c.DualEvTimes = make(map[int]int)
				}
				c.DualEvTimes[id] = n
			}
			if n := atoiDefault(it.DualEffectTimes, 0); n > 0 {
				if c.DualExpTimes == nil {
					c.DualExpTimes = make(map[int]int)
				}
				c.DualExpTimes[id] = n
			}
			if n := atoiDefault(it.TrinalEffectTimes, 0); n > 0 {
				if c.TrinalExpTimes == nil {
					c.TrinalExpTimes = make(map[int]int)
				}
				c.TrinalExpTimes[id] = n
			}
			if n := atoiDefault(it.EnergyAbsorbTimes, 0); n > 0 {
				if c.EnergyAbsTimes == nil {
					c.EnergyAbsTimes = make(map[int]int)
				}
				c.EnergyAbsTimes[id] = n
			}
			if n := atoiDefault(it.AutoBtlTimes, 0); n > 0 {
				if c.AutoBtlTimes == nil {
					c.AutoBtlTimes = make(map[int]int)
				}
				c.AutoBtlTimes[id] = n
			}
			if id >= 700001 && id <= 700999 {
				fx := NonoItemFx{
					AddPower:     atoiDefault(it.AddPower, 0),
					AddCloseness: atoiDefault(it.AddCloseness, 0),
					AddIQ:        atoiDefault(it.AddIQ, 0),
					UseAI:        atoiDefault(it.UseAI, 0),
					UsePower:     atoiDefault(it.UsePower, 0),
				}
				if strings.TrimSpace(it.Color) != "" {
					fx.HasColor = true
					fx.Color = atoiDefault(it.Color, 0)
				}
				if fx.HasColor || fx.AddPower != 0 || fx.AddCloseness != 0 || fx.AddIQ != 0 || fx.UseAI != 0 || fx.UsePower != 0 {
					c.NonoItem[id] = fx
				}
			}
			bt := atoiDefault(it.BreedTime, 0)
			bm := atoiDefault(it.BreedMonID, 0)
			if bt > 0 && bm > 0 {
				if c.EssenceBreed == nil {
					c.EssenceBreed = make(map[int]EssenceBreed)
				}
				c.EssenceBreed[id] = EssenceBreed{
					ItemID: id, BreedTime: bt, VipBreedTime: atoiDefault(it.VipBreedTime, 0),
					BreedMonID: bm, BreedMonLv: atoiDefault(it.BreedMonLv, 1),
				}
			}
			if mons := parseTransmuteMon(it.TransmuteMon); len(mons) > 0 {
				if c.SoulBeads == nil {
					c.SoulBeads = make(map[int]SoulBeadDef)
				}
				c.SoulBeads[id] = SoulBeadDef{
					ItemID: id, Name: it.Name,
					TransmuteTm:  atoiDefault(it.TransmuteTm, 0),
					VipTransmute: atoiDefault(it.VipTransmuteTm, 0),
					TransmuteMon: mons,
				}
			}
		}
	}
	return nil
}

// EssenceBreed 精元孵化配置（items.xml BreedTime/BreedMonID）。
type EssenceBreed struct {
	ItemID       int
	BreedTime    int
	VipBreedTime int
	BreedMonID   int
	BreedMonLv   int
}

// EssenceBreedOf 查精元；无则 ok=false。
func (c *Catalog) EssenceBreedOf(itemID int) (EssenceBreed, bool) {
	if c == nil {
		return EssenceBreed{}, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.EssenceBreed[itemID]
	return e, ok
}

// NonoItemOf 查 NoNo 芯片/玩具属性。
func (c *Catalog) NonoItemOf(itemID int) (NonoItemFx, bool) {
	if c == nil {
		return NonoItemFx{}, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	fx, ok := c.NonoItem[itemID]
	return fx, ok
}

// ItemCatchBonusOf 胶囊 Bonus（items.xml）；无则 0（非捕捉胶囊）。
func (c *Catalog) ItemCatchBonusOf(itemID int) float64 {
	if c == nil {
		return 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.ItemCatchBonus == nil {
		return 0
	}
	return c.ItemCatchBonus[itemID]
}

// ItemBuyPrice 赛尔豆单价；表无或超大价则默认 100。
func (c *Catalog) ItemBuyPrice(itemID int) int {
	if c == nil {
		return 100
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if p, ok := c.ItemPrice[itemID]; ok && p > 0 && p < 1000000 {
		return p
	}
	return 100
}

// FitmentPrice 家具标价；无价/占位天价返回 0（免费或任务发放）。
func (c *Catalog) FitmentPrice(itemID int) int {
	if c == nil || itemID <= 0 {
		return 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if p, ok := c.ItemPrice[itemID]; ok && p > 0 && p < 999999999 {
		return p
	}
	return 0
}

func (c *Catalog) loadPets(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read pets: %w", err)
	}
	// 前端表为 <Monster ID DefName HP Atk…><LearnableMoves><Move ID LearningLv/>
	type moveNode struct {
		ID  string `xml:"ID,attr"`
		Lv  string `xml:"LearningLv,attr"`
	}
	type monsterList struct {
		List []struct {
			ID             string     `xml:"ID,attr"`
			DefName        string     `xml:"DefName,attr"`
			Name           string     `xml:"Name,attr"`
			Type           string     `xml:"Type,attr"`
			GrowthType     string     `xml:"GrowthType,attr"`
			HP             string     `xml:"HP,attr"`
			Atk            string     `xml:"Atk,attr"`
			Def            string     `xml:"Def,attr"`
			SpAtk          string     `xml:"SpAtk,attr"`
			SpDef          string     `xml:"SpDef,attr"`
			Spd            string     `xml:"Spd,attr"`
			EvolvesFrom    string     `xml:"EvolvesFrom,attr"`
			EvolvesTo      string     `xml:"EvolvesTo,attr"`
			EvolvingLv     string     `xml:"EvolvingLv,attr"`
			EvolvFlag      string     `xml:"EvolvFlag,attr"`
			EvolveBabin    string     `xml:"EvolveBabin,attr"`
			EvolvItem      string     `xml:"EvolvItem,attr"`
			EvolvItemCount string     `xml:"EvolvItemCount,attr"`
			IsFuseMon      string     `xml:"IsFuseMon,attr"`
			IsRareMon      string     `xml:"IsRareMon,attr"`
			FreeForbidden  string     `xml:"FreeForbidden,attr"`
			PetClass       string     `xml:"PetClass,attr"`
			CatchRate      string     `xml:"CatchRate,attr"`
			YieldingExp    string     `xml:"YieldingExp,attr"`
			YieldingEV     string     `xml:"YieldingEV,attr"`
			Gender         string     `xml:"Gender,attr"`
			Moves          []moveNode `xml:"LearnableMoves>Move"`
		} `xml:"Monster"`
	}
	var ml monsterList
	_ = xml.Unmarshal(b, &ml)
	for _, p := range ml.List {
		id, err := strconv.Atoi(p.ID)
		if err != nil {
			continue
		}
		name := p.DefName
		if name == "" {
			name = p.Name
		}
		if name == "" {
			continue
		}
		c.PetName[id] = name
		typeID := 0
		if t, err := strconv.Atoi(p.Type); err == nil && t > 0 {
			typeID = t
			c.PetType[id] = t
		}
		atoi := func(s string) int {
			n, _ := strconv.Atoi(s)
			return n
		}
		var moves []LearnableMove
		for _, m := range p.Moves {
			mid, err := strconv.Atoi(m.ID)
			if err != nil || mid <= 0 {
				continue
			}
			moves = append(moves, LearnableMove{ID: mid, Level: atoi(m.Lv)})
		}
		c.PetBaseMap[id] = PetBaseDef{
			ID: id, Name: name, Type: typeID, GrowthType: atoi(p.GrowthType),
			Gender: atoi(p.Gender),
			HP: atoi(p.HP), Atk: atoi(p.Atk), Def: atoi(p.Def),
			SpAtk: atoi(p.SpAtk), SpDef: atoi(p.SpDef), Spd: atoi(p.Spd),
			EvolvesFrom: atoi(p.EvolvesFrom), EvolvesTo: atoi(p.EvolvesTo),
			EvolvingLv: atoi(p.EvolvingLv), EvolvFlag: atoi(p.EvolvFlag),
			EvolveBabin: atoi(p.EvolveBabin), EvolvItem: atoi(p.EvolvItem),
			EvolvItemCount: atoi(p.EvolvItemCount),
			IsFuseMon:      p.IsFuseMon == "1",
			IsRareMon:      p.IsRareMon == "1",
			FreeForbidden:  p.FreeForbidden == "1",
			PetClass:       atoi(p.PetClass),
			CatchRate:      atoi(p.CatchRate),
			YieldingExp:    atoi(p.YieldingExp),
			YieldingEV:     parseYieldingEVAttr(p.YieldingEV),
			LearnableMoves: moves,
		}
	}
	if len(c.PetName) == 0 {
		return c.loadPetsGeneric(b)
	}
	return nil
}

func (c *Catalog) loadPetsGeneric(b []byte) error {
	dec := xml.NewDecoder(bytes.NewReader(b))
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		var idStr, name string
		for _, a := range se.Attr {
			switch a.Name.Local {
			case "ID", "Id", "id":
				idStr = a.Value
			case "Name", "name", "DefName":
				if name == "" {
					name = a.Value
				}
			}
		}
		if idStr == "" || name == "" {
			continue
		}
		id, err := strconv.Atoi(idStr)
		if err != nil {
			continue
		}
		if _, exists := c.PetName[id]; !exists {
			c.PetName[id] = name
		}
	}
	return nil
}
