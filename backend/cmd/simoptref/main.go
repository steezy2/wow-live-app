// Command simoptref writes reference/sim-options.json: what the Sim tab offers besides the pasted gear. The current
// expansion's flasks, food, potions, augment runes, weapon oils, enchants and gems, the upgrade tracks of the
// current seasons and the Omnium Folio's rune tree, all from Raidbots' public game data
// (https://www.raidbots.com/static/data/live/), which is the game's own tables with simc's names for the
// consumables. Run it each patch and review the diff.
//
// Usage, from backend/:
//
//	go run ./cmd/simoptref
package main

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"wow-live/backend/config"
	"wow-live/backend/models"
)

const (
	source = "https://www.raidbots.com/static/data/live/"
	// minUpgradeLevel is the item level a track's first level gives at least, which leaves out the older seasons' tracks.
	minUpgradeLevel = 250
	// socketBonus is the bonus ID that adds a socket (one of bonus-sockets.json's, which all add one prismatic socket).
	socketBonus = 10397
	// fetchTimeout bounds each download.
	fetchTimeout = 2 * time.Minute
)

// consumableFiles are Raidbots' files of the consumables by the kind they are used as (models.SimConsumableKinds).
var consumableFiles = map[string]string{"flask": "flasks", "food": "foods", "potion": "potions",
	"augment": "augments", "weapon": "temp-enchants"}

// invSlots are the slots an enchant goes on, by the bit its inventory type sets in an enchant's invTypeMask.
var invSlots = map[int][]string{1: {"head"}, 2: {"neck"}, 3: {"shoulder"}, 5: {"chest"}, 20: {"chest"}, 6: {"waist"},
	7: {"legs"}, 8: {"feet"}, 9: {"wrist"}, 10: {"hands"}, 11: {"finger1", "finger2"}, 16: {"back"}}

// weaponClass and armorClass are the item classes an enchant can be for; the others are professions' tools.
const (
	weaponClass = 2
	armorClass  = 4
)

func main() {
	root, err := config.RootFromWorkingDir()
	if err != nil {
		log.Fatal(err)
	}
	ref, err := build(context.Background(), http.DefaultClient)
	if err != nil {
		log.Fatal(err)
	}
	data, err := json.MarshalIndent(ref, "", " ")
	if err != nil {
		log.Fatal(fmt.Errorf("writing the options: %w", err))
	}
	path := config.SimOptionsPath(root)
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		log.Fatal(fmt.Errorf("writing %s: %w", path, err))
	}
	log.Printf("wrote %s: %d enchants, %d gems, %d tracks, %d omnium nodes", path, len(ref.Enchants), len(ref.Gems),
		len(ref.Upgrades), len(ref.Omnium))
}

// fetch reads a Raidbots file into v.
func fetch(ctx context.Context, client *http.Client, name string, v any) error {
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source+name+".json", nil)
	if err != nil {
		return fmt.Errorf("building request for %s: %w", name, err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("reading %s: %w", name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("reading %s: %s", name, resp.Status)
	}
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		return fmt.Errorf("reading %s: %w", name, err)
	}
	return nil
}

// build reads Raidbots' files and makes the reference.
func build(ctx context.Context, client *http.Client) (models.SimReference, error) {
	ref := models.SimReference{Source: source, Consumables: map[string][]models.SimConsumable{}}
	var meta struct {
		WowBuild string `json:"wowBuild"`
	}
	if err := fetch(ctx, client, "metadata", &meta); err != nil {
		return ref, err
	}
	ref.Build = meta.WowBuild
	for kind, file := range consumableFiles {
		var items []rawConsumable
		if err := fetch(ctx, client, file, &items); err != nil {
			return ref, err
		}
		ref.Consumables[kind] = consumablesOf(items)
	}
	var enchants []rawEnchant
	if err := fetch(ctx, client, "enchantments", &enchants); err != nil {
		return ref, err
	}
	ref.Enchants, ref.Gems = enchantsAndGems(enchants)
	var bonuses map[string]rawBonus
	if err := fetch(ctx, client, "bonuses", &bonuses); err != nil {
		return ref, err
	}
	ref.Upgrades = upgradeTracks(bonuses)
	var sockets map[string]int
	if err := fetch(ctx, client, "bonus-sockets", &sockets); err != nil {
		return ref, err
	}
	if sockets[fmt.Sprint(socketBonus)] != 1 {
		return ref, fmt.Errorf("bonus %d no longer adds one socket: pick another from bonus-sockets.json", socketBonus)
	}
	ref.SocketBonus = socketBonus
	var omnium struct {
		Nodes []rawNode `json:"nodes"`
	}
	if err := fetch(ctx, client, "omnium-talents", &omnium); err != nil {
		return ref, err
	}
	ref.Omnium = omniumNodes(omnium.Nodes)
	return ref, nil
}

// rawConsumable is an entry of Raidbots' flasks.json, foods.json, potions.json, augments.json and temp-enchants.json.
type rawConsumable struct {
	Value     string `json:"value"`
	Name      string `json:"name"`
	ItemID    int    `json:"itemId"`
	Icon      string `json:"icon"`
	Expansion int    `json:"expansion"`
	Quality   int    `json:"craftingQuality"`
}

// consumablesOf are the newest expansion's consumables of a file, in its order.
func consumablesOf(items []rawConsumable) []models.SimConsumable {
	newest := 0
	for _, i := range items {
		newest = max(newest, i.Expansion)
	}
	out := []models.SimConsumable{}
	for _, i := range items {
		if i.Expansion == newest {
			out = append(out, models.SimConsumable{Value: i.Value, Name: i.Name, Icon: i.Icon, ItemID: i.ItemID,
				Quality: i.Quality})
		}
	}
	return out
}

// rawEnchant is an entry of Raidbots' enchantments.json: an enchant, or a gem when its slot is "socket".
type rawEnchant struct {
	ID          int    `json:"id"`
	DisplayName string `json:"displayName"`
	ItemID      int    `json:"itemId"`
	ItemName    string `json:"itemName"`
	ItemIcon    string `json:"itemIcon"`
	Expansion   int    `json:"expansion"`
	Quality     int    `json:"craftingQuality"`
	Slot        string `json:"slot"`
	Stats       []struct {
		Type   string `json:"type"`
		Amount int    `json:"amount"`
	} `json:"stats"`
	Requirements *struct {
		ItemClass int `json:"itemClass"`
		InvTypes  int `json:"invTypeMask"`
	} `json:"equipRequirements"`
}

// statNames are the stats whose Raidbots name isn't just capitalized.
var statNames = map[string]string{"stragiint": "Main stat"}

// statsText is a gem's or enchant's stats as "32 Int & 70 Sta".
func statsText(r rawEnchant) string {
	parts := make([]string, len(r.Stats))
	for i, s := range r.Stats {
		parts[i] = fmt.Sprintf("%d %s", s.Amount, cmp.Or(statNames[s.Type], strings.ToUpper(s.Type[:1])+s.Type[1:]))
	}
	return strings.Join(parts, " & ")
}

// slotsOf are the slots an enchant's item class and inventory type mask allow; none for a profession's tool.
func slotsOf(class, invTypes int) []string {
	if class == weaponClass {
		return []string{"main_hand", "off_hand"}
	}
	if class != armorClass {
		return nil
	}
	var slots []string
	for bit, s := range invSlots {
		if invTypes&(1<<bit) != 0 {
			for _, slot := range s {
				if !slices.Contains(slots, slot) {
					slots = append(slots, slot)
				}
			}
		}
	}
	slices.Sort(slots)
	return slots
}

// enchantsAndGems split the newest expansion's entries into enchants (with the slots they go on) and gems, each
// sorted by name and quality.
func enchantsAndGems(raw []rawEnchant) ([]models.SimEnchant, []models.SimGem) {
	newest := 0
	for _, r := range raw {
		newest = max(newest, r.Expansion)
	}
	enchants, gems := []models.SimEnchant{}, []models.SimGem{}
	for _, r := range raw {
		switch {
		case r.Expansion != newest || r.ItemID == 0:
		case r.Slot == "socket":
			gems = append(gems, models.SimGem{ID: r.ItemID, Name: r.ItemName, Stats: statsText(r), Icon: r.ItemIcon,
				Quality: r.Quality})
		case r.Requirements != nil:
			if slots := slotsOf(r.Requirements.ItemClass, r.Requirements.InvTypes); len(slots) > 0 {
				enchants = append(enchants, models.SimEnchant{ID: r.ID, Name: r.ItemName, Stats: statsText(r),
					Icon: r.ItemIcon, Quality: r.Quality, Slots: slots})
			}
		}
	}
	slices.SortFunc(enchants, func(a, b models.SimEnchant) int {
		return cmp.Or(strings.Compare(a.Name, b.Name), cmp.Compare(a.Quality, b.Quality))
	})
	slices.SortFunc(gems, func(a, b models.SimGem) int {
		return cmp.Or(strings.Compare(a.Name, b.Name), cmp.Compare(a.Quality, b.Quality))
	})
	return enchants, gems
}

// rawBonus is an entry of Raidbots' bonuses.json; only an upgrade level has Upgrade.
type rawBonus struct {
	Upgrade *struct {
		Group     int    `json:"group"`
		Level     int    `json:"level"`
		ItemLevel int    `json:"itemLevel"`
		Name      string `json:"name"`
	} `json:"upgrade"`
}

// upgradeTracks are the tracks whose first level gives at least minUpgradeLevel, best first.
func upgradeTracks(bonuses map[string]rawBonus) []models.SimUpgradeTrack {
	tracks := map[int]*models.SimUpgradeTrack{}
	for id, b := range bonuses {
		if b.Upgrade == nil {
			continue
		}
		bonusID, err := strconv.Atoi(id)
		if err != nil {
			continue
		}
		t := tracks[b.Upgrade.Group]
		if t == nil {
			t = &models.SimUpgradeTrack{Group: b.Upgrade.Group, Name: b.Upgrade.Name}
			tracks[b.Upgrade.Group] = t
		}
		t.Levels = append(t.Levels, models.SimUpgradeLevel{BonusID: bonusID, Level: b.Upgrade.Level,
			ItemLevel: b.Upgrade.ItemLevel})
	}
	out := []models.SimUpgradeTrack{}
	for _, t := range tracks {
		slices.SortFunc(t.Levels, func(a, b models.SimUpgradeLevel) int { return a.Level - b.Level })
		if t.Levels[0].ItemLevel >= minUpgradeLevel {
			out = append(out, *t)
		}
	}
	slices.SortFunc(out, func(a, b models.SimUpgradeTrack) int { return b.Group - a.Group })
	return out
}

// rawNode is a node of Raidbots' omnium-talents.json.
type rawNode struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	PosX     int    `json:"posX"`
	PosY     int    `json:"posY"`
	MaxRanks int    `json:"maxRanks"`
	Next     []int  `json:"next"`
	Entries  []struct {
		ID      int    `json:"id"`
		SpellID int    `json:"spellId"`
		Name    string `json:"name"`
		Icon    string `json:"icon"`
	} `json:"entries"`
}

// omniumNodes are the Omnium Folio's nodes in the shape the talent tree is drawn from.
func omniumNodes(raw []rawNode) []models.TalentNode {
	nodes := []models.TalentNode{}
	for _, n := range raw {
		node := models.TalentNode{ID: n.ID, Name: n.Name, Tree: "omnium", Choice: n.Type == "choice", X: n.PosX, Y: n.PosY,
			MaxRanks: n.MaxRanks, Next: append([]int{}, n.Next...), Entries: []models.TalentEntry{}}
		for _, e := range n.Entries {
			node.Entries = append(node.Entries, models.TalentEntry{ID: e.ID, SpellID: e.SpellID, Name: e.Name, Icon: e.Icon})
		}
		nodes = append(nodes, node)
	}
	slices.SortFunc(nodes, func(a, b models.TalentNode) int { return a.ID - b.ID })
	return nodes
}
