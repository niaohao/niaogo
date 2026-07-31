package tableloader

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// GoldProduct 金豆商城商品（GoldProductXMLInfo.xml）。
type GoldProduct struct {
	ProductID uint32
	ItemID    int
	Price     int // 金豆
	GoldBonus int // 赠送金豆
	VipFactor float64 // VIP 折扣倍率；1=无折扣；0=VIP免费
	Name      string
}

// MoneyProduct 米币商城商品（MoneyProductXMLInfo.xml）。
type MoneyProduct struct {
	ProductID uint32
	ItemIDs   []int
	Price     float64 // 米币；扣费按 round(price*10)*count 金豆
	GoldBonus int
	VipFactor float64
	Name      string
}

func (c *Catalog) loadGoldProducts(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	type item struct {
		Name      string `xml:"name,attr"`
		ProductID string `xml:"productID,attr"`
		ItemID    string `xml:"itemID,attr"`
		Price     string `xml:"price,attr"`
		Gold      string `xml:"gold,attr"`
		Vip       string `xml:"vip,attr"`
	}
	type root struct {
		Items []item `xml:"item"`
	}
	var r root
	if err := xml.Unmarshal(b, &r); err != nil {
		return fmt.Errorf("parse gold products: %w", err)
	}
	if c.GoldProducts == nil {
		c.GoldProducts = make(map[uint32]GoldProduct)
	}
	for _, it := range r.Items {
		pid, _ := strconv.Atoi(it.ProductID)
		iid, _ := strconv.Atoi(strings.TrimSpace(it.ItemID))
		price, _ := strconv.Atoi(it.Price)
		bonus, _ := strconv.Atoi(it.Gold)
		if pid <= 0 || iid <= 0 || price < 0 {
			continue
		}
		vip := 1.0
		if it.Vip != "" {
			if v, err := strconv.ParseFloat(it.Vip, 64); err == nil {
				vip = v
			}
		}
		c.GoldProducts[uint32(pid)] = GoldProduct{
			ProductID: uint32(pid), ItemID: iid, Price: price, GoldBonus: bonus, VipFactor: vip, Name: it.Name,
		}
	}
	return nil
}

func (c *Catalog) loadMoneyProducts(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	type item struct {
		Name      string `xml:"name,attr"`
		ProductID string `xml:"productID,attr"`
		ItemID    string `xml:"itemID,attr"`
		Price     string `xml:"price,attr"`
		Gold      string `xml:"gold,attr"`
		Vip       string `xml:"vip,attr"`
	}
	type root struct {
		Items []item `xml:"item"`
	}
	var r root
	if err := xml.Unmarshal(b, &r); err != nil {
		return fmt.Errorf("parse money products: %w", err)
	}
	if c.MoneyProducts == nil {
		c.MoneyProducts = make(map[uint32]MoneyProduct)
	}
	for _, it := range r.Items {
		pid, _ := strconv.Atoi(it.ProductID)
		if pid <= 0 {
			continue
		}
		price, _ := strconv.ParseFloat(it.Price, 64)
		bonus, _ := strconv.Atoi(it.Gold)
		ids := parsePipeIDs(it.ItemID)
		vip := 1.0
		if it.Vip != "" {
			if v, err := strconv.ParseFloat(it.Vip, 64); err == nil {
				vip = v
			}
		}
		c.MoneyProducts[uint32(pid)] = MoneyProduct{
			ProductID: uint32(pid), ItemIDs: ids, Price: price, GoldBonus: bonus, VipFactor: vip, Name: it.Name,
		}
	}
	return nil
}

func parsePipeIDs(s string) []int {
	parts := strings.Split(s, "|")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		id, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil || id <= 0 {
			continue
		}
		out = append(out, id)
	}
	return out
}

// GetGoldProduct 查金豆商品。
func (c *Catalog) GetGoldProduct(productID uint32) (GoldProduct, bool) {
	if c == nil {
		return GoldProduct{}, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	p, ok := c.GoldProducts[productID]
	return p, ok
}

// GetMoneyProduct 查米币商品。
func (c *Catalog) GetMoneyProduct(productID uint32) (MoneyProduct, bool) {
	if c == nil {
		return MoneyProduct{}, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	p, ok := c.MoneyProducts[productID]
	return p, ok
}

// loadEquipUpgrades 读 EquipXmlConfig.xml：sendId → 装备升级。
func (c *Catalog) loadEquipUpgrades(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	type level struct {
		SendID         string `xml:"sendId,attr"`
		LevelID        string `xml:"levelId,attr"`
		NeedCatalystID string `xml:"needCatalystId,attr"`
		NeedMatterID   string `xml:"needMatterId,attr"`
		NeedMatterNum  string `xml:"needMatterNum,attr"`
		Odds           string `xml:"odds,attr"`
	}
	type equip struct {
		ID     string  `xml:"id,attr"`
		Levels []level `xml:"level"`
	}
	type root struct {
		Equips []equip `xml:"equip"`
	}
	var r root
	if err := xml.Unmarshal(b, &r); err != nil {
		return fmt.Errorf("parse equip: %w", err)
	}
	if c.EquipBySendID == nil {
		c.EquipBySendID = make(map[uint32]EquipUpgrade)
	}
	for _, eq := range r.Equips {
		itemID, _ := strconv.Atoi(eq.ID)
		if itemID <= 0 {
			continue
		}
		for _, lv := range eq.Levels {
			sendID, _ := strconv.Atoi(lv.SendID)
			levelID, _ := strconv.Atoi(lv.LevelID)
			if sendID <= 0 || levelID <= 0 {
				continue
			}
			up := EquipUpgrade{SendID: uint32(sendID), ItemID: itemID, LevelID: levelID}
			// catalyst: id|count
			catParts := strings.Split(lv.NeedCatalystID, "|")
			if len(catParts) >= 2 {
				up.CatalystID, _ = strconv.Atoi(catParts[0])
				up.CatalystNum, _ = strconv.Atoi(catParts[1])
			}
			matIDs := parsePipeIDs(lv.NeedMatterID)
			matNums := parsePipeIDs(lv.NeedMatterNum)
			for i, id := range matIDs {
				n := 1
				if i < len(matNums) {
					n = matNums[i]
				}
				up.Matters = append(up.Matters, [2]int{id, n})
			}
			odds := strings.TrimSuffix(strings.TrimSpace(lv.Odds), "%")
			up.Odds, _ = strconv.Atoi(odds)
			if up.Odds <= 0 {
				up.Odds = 100
			}
			c.EquipBySendID[uint32(sendID)] = up
		}
	}
	return nil
}

// EquipUpgrade 装备强化一步。
type EquipUpgrade struct {
	SendID      uint32
	ItemID      int
	LevelID     int
	CatalystID  int
	CatalystNum int
	Matters     [][2]int // itemID, count
	Odds        int
}

func (c *Catalog) GetEquipUpgrade(sendID uint32) (EquipUpgrade, bool) {
	if c == nil {
		return EquipUpgrade{}, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	u, ok := c.EquipBySendID[sendID]
	return u, ok
}

func productXMLPath(xmlDir, name string) string {
	return filepath.Join(xmlDir, name)
}

// loadPetShop 本客户端精灵道具店：moneyType=0→赛尔豆价；moneyType=1→金豆商品(1104)。
func (c *Catalog) loadPetShop(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	type item struct {
		Name      string `xml:"name,attr"`
		ItemID    string `xml:"itemID,attr"`
		ProductID string `xml:"productID,attr"`
		Price     string `xml:"price,attr"`
		MoneyType string `xml:"moneyType,attr"`
	}
	type root struct {
		Items []item `xml:"item"`
	}
	var r root
	if err := xml.Unmarshal(b, &r); err != nil {
		return fmt.Errorf("parse pet shop: %w", err)
	}
	if c.GoldProducts == nil {
		c.GoldProducts = make(map[uint32]GoldProduct)
	}
	if c.ItemPrice == nil {
		c.ItemPrice = make(map[int]int)
	}
	for _, it := range r.Items {
		iid, _ := strconv.Atoi(strings.TrimSpace(it.ItemID))
		price, _ := strconv.Atoi(it.Price)
		mt, _ := strconv.Atoi(it.MoneyType)
		if iid <= 0 || price < 0 {
			continue
		}
		switch mt {
		case 0:
			// 赛尔豆店：覆盖 items.xml Price（与前端 PetShop 一致）
			c.ItemPrice[iid] = price
		case 1:
			pid, _ := strconv.Atoi(strings.TrimSpace(it.ProductID))
			if pid <= 0 {
				continue
			}
			// 仅补 GoldProduct 未收录的条目（本客户端 ProductAction 会回退 PetShop）
			if _, ok := c.GoldProducts[uint32(pid)]; !ok {
				c.GoldProducts[uint32(pid)] = GoldProduct{
					ProductID: uint32(pid), ItemID: iid, Price: price, VipFactor: 1, Name: it.Name,
				}
			}
		}
	}
	return nil
}
