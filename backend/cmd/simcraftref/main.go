// Command simcraftref writes reference/sim-crafting.json: what each crafted item of the current expansion can be
// recrafted with (embellishments, secondary stat pairs) and the season's
// Catalyst conversions, all from Raidbots' public game data (https://www.raidbots.com/static/data/live/). Run it
// each patch and review the diff.
//
// An item's reagents are its recipe's optional crafting slots (equippable-items.json, streamed: it is 53 MB) looked
// up in crafting.json; a reagent's craftingBonusIds say what it is: an effect (bonus-effects.json) is an
// embellishment, a stat pair (bonus-crafted-stats.json) a recraft, an item level (bonuses.json) a level.
//
// An addon export's crafted item can say crafted_stats=36/40 while its bonus IDs carry 8795 (crit/vers): Raidbots'
// "Recraft to" list leaves out crit/vers there, so the pair an item has is the one its bonus IDs give, never
// crafted_stats.
//
// Usage, from backend/:
//
//	go run ./cmd/simcraftref
package main

import (
	"cmp"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"slices"
	"time"

	"wow-live/backend/clients/raidbots"
	"wow-live/backend/config"
	"wow-live/backend/models"
)

// fetchTimeout bounds each download (equippable-items.json is 53 MB).
const fetchTimeout = 5 * time.Minute

// weaponClass and armorClass are the item classes a Top Gear can wear; the others are professions' tools.
const (
	weaponClass = 2
	armorClass  = 4
)

func main() {
	root, err := config.RootFromWorkingDir()
	if err != nil {
		log.Fatal(err)
	}
	client := &http.Client{Timeout: fetchTimeout}
	ref, err := build(func(name string) (io.ReadCloser, error) { return raidbots.Open(client, name) })
	if err != nil {
		log.Fatal(err)
	}
	data, err := json.MarshalIndent(ref, "", " ")
	if err != nil {
		log.Fatal(fmt.Errorf("writing the crafting data: %w", err))
	}
	path := config.SimCraftingPath(root)
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		log.Fatal(fmt.Errorf("writing %s: %w", path, err))
	}
	log.Printf("wrote %s: %d KB, %d crafted items, %d reagents, %d conversions", path, len(data)/1024, len(ref.Items),
		len(ref.Reagents), len(ref.Conversions))
}

// build reads Raidbots' files through open and makes the crafting data.
func build(open func(name string) (io.ReadCloser, error)) (models.SimCrafting, error) {
	ref := models.SimCrafting{Source: raidbots.Source}
	var meta struct {
		WowBuild string `json:"wowBuild"`
	}
	var craft rawCrafting
	var bonuses bonusData
	var conversions map[int]rawConversion
	for name, v := range map[string]any{"metadata.json": &meta, "crafting.json": &craft,
		"bonus-crafted-stats.json": &bonuses.stats, "bonus-effects.json": &bonuses.effects,
		"item-conversions.json": &conversions} {
		if err := read(open, name, v); err != nil {
			return ref, err
		}
	}
	ref.Build = meta.WowBuild
	body, err := open("equippable-items.json")
	if err != nil {
		return ref, err
	}
	defer body.Close()
	items, err := craftedItems(body)
	if err != nil {
		return ref, fmt.Errorf("reading equippable-items.json: %w", err)
	}
	ref.Items, ref.Reagents = craftingOf(items, craft, bonuses)
	ref.Conversions = conversionsOf(conversions)
	return ref, nil
}

// read decodes the file name into v.
func read(open func(name string) (io.ReadCloser, error), name string, v any) error {
	body, err := open(name)
	if err != nil {
		return err
	}
	defer body.Close()
	if err := json.NewDecoder(body).Decode(v); err != nil {
		return fmt.Errorf("reading %s: %w", name, err)
	}
	return nil
}

// rawItem is an entry of Raidbots' equippable-items.json; only a crafted one has Profession.
type rawItem struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	ItemClass  int    `json:"itemClass"`
	Expansion  int    `json:"expansion"`
	Profession *struct {
		Slots []struct {
			ID int `json:"id"`
		} `json:"optionalCraftingSlots"`
	} `json:"profession"`
}

// craftedItems are the crafted gear of equippable-items.json, decoded one entry at a time so the file is never
// held whole.
func craftedItems(r io.Reader) ([]rawItem, error) {
	dec := json.NewDecoder(r)
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	var items []rawItem
	for dec.More() {
		var item rawItem
		if err := dec.Decode(&item); err != nil {
			return nil, err
		}
		if item.Profession != nil && (item.ItemClass == weaponClass || item.ItemClass == armorClass) {
			items = append(items, item)
		}
	}
	return items, nil
}

// rawCrafting is Raidbots' crafting.json: the optional crafting slots by ID and every reagent.
type rawCrafting struct {
	Slots map[int]struct {
		ReagentIDs []int `json:"reagentIds"`
	} `json:"slots"`
	Reagents []rawReagent `json:"reagents"`
}

// rawReagent is a reagent of crafting.json, with the bonus IDs it adds to what it crafts.
type rawReagent struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Icon      string `json:"icon"`
	Quality   int    `json:"craftingQuality"`
	Expansion int    `json:"expansion"`
	BonusIDs  []int  `json:"craftingBonusIds"`
}

// bonusData is what a reagent's bonus IDs mean: bonus-crafted-stats.json's stats and bonus-effects.json's effects.
type bonusData struct {
	stats   map[int][]int
	effects map[int]json.RawMessage
}

// craftingOf are the newest expansion's crafted items that take any reagent the data names, and those reagents. A
// reagent's qualities add the same bonus IDs, so one stands for them all.
func craftingOf(items []rawItem, craft rawCrafting, bonuses bonusData) (map[int]models.CraftedItem,
	map[int]models.CraftingReagent) {
	newest := 0
	for _, i := range items {
		newest = max(newest, i.Expansion)
	}
	kept, known := bestReagents(craft.Reagents, bonuses)
	out, used := map[int]models.CraftedItem{}, map[int]models.CraftingReagent{}
	for _, item := range items {
		if item.Expansion != newest {
			continue
		}
		crafted := models.CraftedItem{Name: item.Name}
		for _, slot := range item.Profession.Slots {
			for _, id := range craft.Slots[slot.ID].ReagentIDs {
				if k, ok := kept[id]; ok && !slices.Contains(crafted.Reagents, k) {
					crafted.Reagents = append(crafted.Reagents, k)
					used[k] = known[k]
				}
			}
		}
		if crafted.Reagents != nil {
			slices.Sort(crafted.Reagents)
			out[item.ID] = crafted
		}
	}
	return out, used
}

// bestReagents maps each reagent the data names to the one kept for its bonus IDs (the newest expansion's, at its
// highest crafting quality: an older expansion's missive adds the same stat pair), and gives the kept ones.
func bestReagents(raw []rawReagent, bonuses bonusData) (map[int]int, map[int]models.CraftingReagent) {
	best, known := map[string]rawReagent{}, map[int]models.CraftingReagent{}
	for _, r := range raw {
		c, ok := reagentOf(r, bonuses)
		if !ok {
			continue
		}
		key := fmt.Sprint(r.BonusIDs)
		had, ok := best[key]
		if !ok || cmp.Or(cmp.Compare(r.Expansion, had.Expansion), cmp.Compare(r.Quality, had.Quality)) > 0 {
			best[key] = r
		}
		known[r.ID] = c
	}
	kept := map[int]int{}
	for _, r := range raw {
		if _, ok := known[r.ID]; ok {
			kept[r.ID] = best[fmt.Sprint(r.BonusIDs)].ID
		}
	}
	return kept, known
}

// reagentOf is r as the sim uses it, by the first of its bonus IDs the data names; false when none is.
func reagentOf(r rawReagent, bonuses bonusData) (models.CraftingReagent, bool) {
	c := models.CraftingReagent{Name: r.Name, Icon: r.Icon, BonusIDs: r.BonusIDs}
	for _, b := range r.BonusIDs {
		switch {
		case len(bonuses.stats[b]) == 2:
			c.Kind, c.Stats = models.ReagentRecraft, bonuses.stats[b]
		case bonuses.effects[b] != nil:
			c.Kind = models.ReagentEmbellishment
		default:
			continue
		}
		return c, true
	}
	return c, false
}

// rawConversion is an entry of Raidbots' item-conversions.json: the bonus IDs that let an item convert and the
// items it converts to.
type rawConversion struct {
	BonusIDs []int `json:"bonusIds"`
	Items    []struct {
		ID            int    `json:"id"`
		Name          string `json:"name"`
		InventoryType int    `json:"inventoryType"`
		Expansion     int    `json:"expansion"`
		Classes       []int  `json:"allowableClasses"`
	} `json:"items"`
}

// conversionsOf are the newest expansion's conversions (one per season), each item with the one class it is for.
func conversionsOf(raw map[int]rawConversion) map[int]models.ItemConversion {
	newest := 0
	for _, c := range raw {
		for _, i := range c.Items {
			newest = max(newest, i.Expansion)
		}
	}
	out := map[int]models.ItemConversion{}
	for id, c := range raw {
		if len(c.BonusIDs) == 0 || len(c.Items) == 0 || c.Items[0].Expansion != newest {
			continue
		}
		conv := models.ItemConversion{BonusIDs: c.BonusIDs}
		for _, i := range c.Items {
			if len(i.Classes) == 1 {
				conv.Items = append(conv.Items, models.CatalystItem{ID: i.ID, Name: i.Name, Class: i.Classes[0],
					InventoryType: i.InventoryType})
			}
		}
		slices.SortFunc(conv.Items, func(a, b models.CatalystItem) int { return cmp.Compare(a.ID, b.ID) })
		out[id] = conv
	}
	return out
}
