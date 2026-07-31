package tableloader

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// SoulBeadDef items.xml 元神珠：赋形产物与时长。
type SoulBeadDef struct {
	ItemID       int
	Name         string
	TransmuteTm  int   // 秒
	VipTransmute int   // 秒
	TransmuteMon []int // 米色珠可多只
}

// HatchTaskDef HatchTaskXMLInfo：吸能步数（客户端地图交互写 2353）。
type HatchTaskDef struct {
	ItemID   int
	Name     string
	IsDir    bool
	ProCount int
	Maps     []int
}

// SoulBeadOf 查元神珠定义。
func (c *Catalog) SoulBeadOf(itemID int) (SoulBeadDef, bool) {
	if c == nil {
		return SoulBeadDef{}, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	d, ok := c.SoulBeads[itemID]
	return d, ok
}

// HatchTaskOf 查吸能任务；无配置时 ProCount=1。
func (c *Catalog) HatchTaskOf(itemID int) HatchTaskDef {
	if c == nil {
		return HatchTaskDef{ItemID: itemID, ProCount: 1}
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if d, ok := c.HatchTasks[itemID]; ok {
		return d
	}
	return HatchTaskDef{ItemID: itemID, ProCount: 1}
}

// HatchAbsorbSteps 完成赋形前需要置 true 的步数。
func (c *Catalog) HatchAbsorbSteps(itemID int) int {
	n := c.HatchTaskOf(itemID).ProCount
	if n <= 0 {
		return 1
	}
	if n > 20 {
		return 20
	}
	return n
}

func (c *Catalog) loadHatchTaskXML(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read hatchTask: %w", err)
	}
	var root struct {
		Tasks []struct {
			ID    string `xml:"ID,attr"`
			Name  string `xml:"name,attr"`
			IsDir string `xml:"isDir,attr"`
			Pros  []struct {
				Map string `xml:"map,attr"`
			} `xml:"pro"`
		} `xml:"task"`
	}
	if err := xml.Unmarshal(b, &root); err != nil {
		return fmt.Errorf("parse hatchTask: %w", err)
	}
	if c.HatchTasks == nil {
		c.HatchTasks = make(map[int]HatchTaskDef)
	}
	for _, t := range root.Tasks {
		id, err := strconv.Atoi(t.ID)
		if err != nil || id <= 0 {
			continue
		}
		maps := make([]int, 0, len(t.Pros))
		for _, p := range t.Pros {
			if m, e := strconv.Atoi(p.Map); e == nil && m > 0 {
				maps = append(maps, m)
			}
		}
		c.HatchTasks[id] = HatchTaskDef{
			ItemID: id, Name: t.Name, IsDir: t.IsDir == "1",
			ProCount: len(t.Pros), Maps: maps,
		}
	}
	return nil
}

func parseTransmuteMon(s string) []int {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Fields(s)
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		if n, err := strconv.Atoi(p); err == nil && n > 0 {
			out = append(out, n)
		}
	}
	return out
}

// LoadHatchTaskXML 供单测/外部调用。
func LoadHatchTaskXML(c *Catalog, xmlDir string) error {
	if c == nil {
		return fmt.Errorf("nil catalog")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.loadHatchTaskXML(filepath.Join(xmlDir, "HatchTaskXMLInfo.xml"))
}
