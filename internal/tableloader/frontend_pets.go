package tableloader

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
)

// FrontendPetName 前端 PetXMLInfo 有 DefName 的种族（客户端能显示名字）。
// 仅用于发放/展示过滤；战斗数值仍以 pets.xml PetBaseMap 为准。
func (c *Catalog) loadFrontendPetNames(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	type monsterList struct {
		List []struct {
			ID      string `xml:"ID,attr"`
			DefName string `xml:"DefName,attr"`
			Name    string `xml:"Name,attr"`
		} `xml:"Monster"`
	}
	var ml monsterList
	if err := xml.Unmarshal(b, &ml); err != nil {
		return fmt.Errorf("parse PetXMLInfo: %w", err)
	}
	if c.FrontendPetName == nil {
		c.FrontendPetName = make(map[int]string)
	}
	for _, p := range ml.List {
		id, err := strconv.Atoi(p.ID)
		if err != nil || id <= 0 {
			continue
		}
		name := p.DefName
		if name == "" {
			name = p.Name
		}
		if name == "" {
			continue
		}
		c.FrontendPetName[id] = name
	}
	return nil
}

func frontendPetXMLPaths(xmlDir string) []string {
	return []string{
		filepath.Join(xmlDir, "..", "bin", "PetXMLInfo.bin"),
		filepath.Join(xmlDir, "PetXMLInfo.xml"),
		filepath.Join(xmlDir, "PetXMLInfo.bin"),
	}
}

// GrantablePetIDs 一键发放用：同时在 pets.xml（有数值）与前端 PetXMLInfo（有名字）中。
// 若前端表未加载，回退为 AllPetIDs（兼容仅有 pets.xml 的环境）。
func (c *Catalog) GrantablePetIDs() []int {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.FrontendPetName) == 0 {
		ids := make([]int, 0, len(c.PetBaseMap))
		for id := range c.PetBaseMap {
			if id > 0 {
				ids = append(ids, id)
			}
		}
		sort.Ints(ids)
		return ids
	}
	ids := make([]int, 0, len(c.FrontendPetName))
	for id, name := range c.FrontendPetName {
		if id <= 0 || name == "" {
			continue
		}
		if _, ok := c.PetBaseMap[id]; !ok {
			continue
		}
		ids = append(ids, id)
	}
	sort.Ints(ids)
	return ids
}

// FrontendPetNameOf 前端表名字；无则空串。
func (c *Catalog) FrontendPetNameOf(id int) string {
	if c == nil {
		return ""
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.FrontendPetName[id]
}
