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
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"wow-live/backend/clients/raidbots"
	"wow-live/backend/config"
	"wow-live/backend/models"
)

const (
	// minUpgradeLevel is the item level a track's first level gives at least, which leaves out the older seasons' tracks.
	minUpgradeLevel = 250
	// socketBonus is the bonus ID that adds a socket (one of bonus-sockets.json's, which all add one prismatic socket).
	socketBonus = 10397
	// fetchTimeout bounds each download.
	fetchTimeout = 2 * time.Minute
)

// simcValue is what a consumable's simc name may be: the app writes it into simc's input as it is.
var simcValue = regexp.MustCompile(`^[a-z0-9_]+$`)

// consumableFiles are Raidbots' files of the consumables by the kind they are used as (models.SimConsumableKinds).
var consumableFiles = map[string]string{"flask": "flasks.json", "food": "foods.json", "potion": "potions.json",
	"augment": "augments.json", "weapon": "temp-enchants.json"}

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
	ref, err := build(&http.Client{Timeout: fetchTimeout})
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

// build reads Raidbots' files and makes the reference.
func build(client *http.Client) (models.SimReference, error) {
	ref := models.SimReference{Source: raidbots.Source, Consumables: map[string][]models.SimConsumable{}}
	var meta struct {
		WowBuild string `json:"wowBuild"`
	}
	if err := raidbots.Get(client, "metadata.json", &meta); err != nil {
		return ref, err
	}
	ref.Build = meta.WowBuild
	for kind, file := range consumableFiles {
		var items []rawConsumable
		if err := raidbots.Get(client, file, &items); err != nil {
			return ref, err
		}
		ref.Consumables[kind] = consumablesOf(items)
		for _, c := range ref.Consumables[kind] {
			if !simcValue.MatchString(c.Value) {
				return ref, fmt.Errorf("%s: %q is not a simc name", file, c.Value)
			}
		}
	}
	var enchants []rawEnchant
	if err := raidbots.Get(client, "enchantments.json", &enchants); err != nil {
		return ref, err
	}
	ref.Enchants, ref.Gems = enchantsAndGems(enchants)
	var bonuses map[string]rawBonus
	if err := raidbots.Get(client, "bonuses.json", &bonuses); err != nil {
		return ref, err
	}
	ref.Upgrades = upgradeTracks(bonuses)
	var sockets map[string]int
	if err := raidbots.Get(client, "bonus-sockets.json", &sockets); err != nil {
		return ref, err
	}
	if sockets[fmt.Sprint(socketBonus)] != 1 {
		return ref, fmt.Errorf("bonus %d no longer adds one socket: pick another from bonus-sockets.json", socketBonus)
	}
	ref.SocketBonus = socketBonus
	ref.Sockets = map[int]int{}
	for id, n := range sockets {
		if bonusID, err := strconv.Atoi(id); err == nil {
			ref.Sockets[bonusID] = n
		}
	}
	var seasons []struct {
		Active  bool `json:"active"`
		Sockets []struct {
			Slot  string `json:"slot"`
			Vault int    `json:"vault"`
		} `json:"sockets"`
	}
	if err := raidbots.Get(client, "seasons.json", &seasons); err != nil {
		return ref, err
	}
	ref.SocketSlots = map[string]int{}
	for _, season := range seasons {
		for _, s := range season.Sockets {
			if season.Active && s.Vault > 0 {
				ref.SocketSlots[s.Slot] = s.Vault
			}
		}
	}
	var omnium struct {
		Nodes []raidbots.Node `json:"nodes"`
	}
	if err := raidbots.Get(client, "omnium-talents.json", &omnium); err != nil {
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
			out = append(out, models.SimConsumable{Value: i.Value, Name: i.Name, Icon: wowheadIcon(i.Icon), ItemID: i.ItemID,
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
			gems = append(gems, models.SimGem{ID: r.ItemID, Name: r.ItemName, Stats: statsText(r), Icon: wowheadIcon(r.ItemIcon),
				Quality: r.Quality})
		case r.Requirements != nil:
			if slots := slotsOf(r.Requirements.ItemClass, r.Requirements.InvTypes); len(slots) > 0 {
				enchants = append(enchants, models.SimEnchant{ID: r.ID, Name: r.ItemName, Stats: statsText(r),
					Icon: wowheadIcon(r.ItemIcon), Quality: r.Quality, Slots: slots})
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

// omniumNodes are the Omnium Folio's nodes in the shape the talent tree is drawn from.
func omniumNodes(raw []raidbots.Node) []models.TalentNode {
	nodes := []models.TalentNode{}
	for _, n := range raw {
		nodes = append(nodes, n.TalentNode("omnium"))
	}
	slices.SortFunc(nodes, func(a, b models.TalentNode) int { return a.ID - b.ID })
	return nodes
}

// wowheadIcon is Raidbots' icon name as Wowhead's CDN has it: Raidbots writes the "-" the game's icon files end with
// as "_" ("..._red--" is "..._red__"), which Wowhead has no file for.
func wowheadIcon(name string) string {
	kept := strings.TrimRight(name, "_")
	return kept + strings.Repeat("-", len(name)-len(kept))
}
