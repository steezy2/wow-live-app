package main

import (
	"cmp"
	"fmt"
	"maps"
	"slices"

	"wow-live/backend/models"
)

// The game's item classes and the armor subclasses simref reads.
const (
	classWeapon   = 2
	classArmor    = 4
	armorShield   = 6
	armorCosmetic = 5
	invCloak      = 16
)

// slots is the slot of each gear InventoryType; the rest (shirts, tabards, bags, tokens, ...) aren't gear.
var slots = map[int]string{1: "head", 2: "neck", 3: "shoulder", 16: "back", 5: "chest", 20: "chest", 9: "wrist",
	10: "hands", 6: "waist", 7: "legs", 8: "feet", 11: "finger", 12: "trinket", 13: "one_hand", 21: "main_hand",
	14: "off_hand", 22: "off_hand", 23: "off_hand", 15: "two_hand", 17: "two_hand", 26: "two_hand"}

var armorTypes = map[int]string{1: "cloth", 2: "leather", 3: "mail", 4: "plate"}

// The game's ItemSpec stats: the primary stats, then the ones that name an item's kind.
const (
	statIntellect = 0
	statAgility   = 1
	statStrength  = 2
	statShield    = 22
	statCloak     = 27
	statNone      = 40
)

// primaryStats is the ItemSpec stats each ItemSparse stat modifier gives (intellect, agility, strength and their
// mixes); secondary stats name no spec.
var primaryStats = map[int][]int{3: {statAgility}, 4: {statStrength}, 5: {statIntellect},
	71: {statAgility, statStrength, statIntellect}, 72: {statAgility, statStrength}, 73: {statAgility, statIntellect},
	74: {statStrength, statIntellect}}

// weaponStats is the ItemSpec stat of each weapon subclass (one-handed axe, two-handed axe, bow, gun, ...).
var weaponStats = map[int]int{0: 7, 1: 8, 2: 16, 3: 15, 4: 11, 5: 12, 6: 19, 7: 9, 8: 10, 9: 28, 10: 18, 13: 14,
	15: 13, 16: 20, 18: 17, 19: 21}

// buildDrops is the season's raid and dungeon gear with its item levels. The raid is each instance whose loot carries
// levels.DisplaySeason, the dungeons the instances on dungeonMaps (combat log instance IDs); loot the journal shows
// in another season only is left out.
func buildDrops(j journal, levels models.SimDropLevels, dungeonMaps []int) (models.SimDrops, error) {
	drops := models.SimDrops{Season: levels.Season, Specs: map[int]models.SimSpec{}, Raids: []models.DropSource{},
		Dungeons: []models.DropSource{}, HeroicDungeon: levels.HeroicDungeon, Keys: levels.Keys, Sources: map[string]string{
			"items": "wago.tools' tables of the live game (JournalInstance, JournalEncounter, JournalEncounterItem, " +
				"Item, ItemSparse): every weapon and armor piece each boss drops",
			"specs": "ItemSpecOverride, else the game's ItemSpec rule over the item's kind and primary stats; " +
				"ChrClasses and ChrSpecialization for the names",
			"levels": "reference/sim-drops.toml (kept by hand, checked against the game)",
		}}
	for id, s := range j.specs {
		drops.Specs[id] = s.SimSpec
	}
	rows, raids := map[int][]loot{}, map[int]bool{}
	for _, l := range j.loot {
		e, ok := j.encounters[l.encounter]
		if !ok || (l.season != 0 && l.season != levels.DisplaySeason) {
			continue
		}
		rows[e.instance] = append(rows[e.instance], l)
		if l.season != 0 && !slices.Contains(dungeonMaps, j.instances[e.instance].mapID) {
			raids[e.instance] = true
		}
	}
	if len(raids) == 0 {
		return drops, fmt.Errorf("no raid loot has DisplaySeasonID %d: check display_season in sim-drops.toml",
			levels.DisplaySeason)
	}
	bossLevels := map[string]models.DifficultyLevels{}
	for _, g := range levels.Raid {
		for _, b := range g.Bosses {
			bossLevels[b] = g.Levels
		}
	}
	for _, id := range slices.Sorted(maps.Keys(raids)) {
		raid := j.source(id, rows[id])
		for i, b := range raid.Bosses {
			l, ok := bossLevels[b.Name]
			if !ok {
				return drops, fmt.Errorf("%s of %s has no item levels in sim-drops.toml", b.Name, raid.Name)
			}
			raid.Bosses[i].Levels = &l
			delete(bossLevels, b.Name)
		}
		drops.Raids = append(drops.Raids, raid)
	}
	if len(bossLevels) > 0 {
		return drops, fmt.Errorf("sim-drops.toml has %v, which drop no gear in the season's raid",
			slices.Sorted(maps.Keys(bossLevels)))
	}
	for _, id := range slices.Sorted(maps.Keys(j.instances)) {
		if slices.Contains(dungeonMaps, j.instances[id].mapID) {
			drops.Dungeons = append(drops.Dungeons, j.source(id, rows[id]))
		}
	}
	if len(drops.Dungeons) != len(dungeonMaps) {
		return drops, fmt.Errorf("%d journal instances for the season's %d dungeons (maps %v)", len(drops.Dungeons),
			len(dungeonMaps), dungeonMaps)
	}
	slices.SortFunc(drops.Dungeons, func(a, b models.DropSource) int { return cmp.Compare(a.Name, b.Name) })
	return drops, nil
}

// source is an instance's bosses in journal order, each with the gear its loot rows name, by item ID.
func (j journal) source(id int, rows []loot) models.DropSource {
	in := j.instances[id]
	src := models.DropSource{InstanceID: id, MapID: in.mapID, Name: in.name, Bosses: []models.DropBoss{}}
	gear := map[int][]models.DropItem{}
	for _, l := range rows {
		d, ok := j.gear(l.item)
		if ok && !slices.ContainsFunc(gear[l.encounter], func(g models.DropItem) bool { return g.ID == d.ID }) {
			gear[l.encounter] = append(gear[l.encounter], d)
		}
	}
	encounters := slices.SortedFunc(maps.Keys(gear), func(a, b int) int {
		return cmp.Or(cmp.Compare(j.encounters[a].order, j.encounters[b].order), cmp.Compare(a, b))
	})
	for _, e := range encounters {
		items := gear[e]
		slices.SortFunc(items, func(a, b models.DropItem) int { return cmp.Compare(a.ID, b.ID) })
		src.Bosses = append(src.Bosses, models.DropBoss{Name: j.encounters[e].name, Items: items})
	}
	return src
}

// gear is the item as the Droptimizer reads it, or false for an item that isn't a weapon or armor piece (tokens,
// decor, mounts, cosmetics) or that ItemSparse has no row for.
func (j journal) gear(id int) (models.DropItem, bool) {
	it := j.items[id]
	slot, ok := slots[it.inventory]
	if !ok || it.name == "" || (it.class != classWeapon && it.class != classArmor) ||
		(it.class == classArmor && it.subclass == armorCosmetic) {
		return models.DropItem{}, false
	}
	d := models.DropItem{ID: id, Name: it.name, Slot: slot, Specs: j.lootSpecs(it)}
	if it.class == classArmor && slot != "back" {
		d.Armor = armorTypes[it.subclass]
	}
	return d, true
}

// lootSpecs is the specs that can loot the item, nil when every spec can: its ItemSpecOverride rows, else the ItemSpec
// rows of its kind whose stats it has. A row with no primary stat fits only an item with none, so a strength axe
// isn't an agility spec's.
func (j journal) lootSpecs(it item) []int {
	fits := j.overrides[it.id]
	if len(fits) == 0 {
		kind, stats := specKind(it)
		primary := slices.ContainsFunc(stats, func(s int) bool { return s <= statStrength })
		for _, s := range j.itemSpecs {
			hasPrimary := slices.Contains(stats, s.primary) || (s.primary == statNone && !primary)
			hasSecondary := s.secondary == statNone || slices.Contains(stats, s.secondary)
			if sp, ok := j.specs[s.spec]; ok && s.kind == kind && hasPrimary && hasSecondary &&
				(it.allowableClasses == -1 || it.allowableClasses&(1<<(sp.class-1)) != 0) {
				fits = append(fits, s.spec)
			}
		}
	}
	fits = slices.DeleteFunc(slices.Clone(fits), func(s int) bool { _, ok := j.specs[s]; return !ok })
	slices.Sort(fits)
	if fits = slices.Compact(fits); len(fits) == len(j.specs) {
		return nil
	}
	return fits
}

// specKind is the item's ItemSpec kind (ItemSpec.ItemType: 0 other, 1 to 4 cloth to plate, 5 weapon, 6 shield) and
// the ItemSpec stats it has: its weapon type, shield or cloak, and its primary stats.
func specKind(it item) (int, []int) {
	kind, stats := 0, []int{}
	switch {
	case it.class == classWeapon:
		kind = 5
		if s, ok := weaponStats[it.subclass]; ok {
			stats = append(stats, s)
		}
	case it.class == classArmor && it.inventory == invCloak:
		stats = append(stats, statCloak)
	case it.class == classArmor && armorTypes[it.subclass] != "":
		kind = it.subclass
	case it.class == classArmor && it.subclass == armorShield:
		kind, stats = 6, append(stats, statShield)
	}
	for _, m := range it.stats {
		stats = append(stats, primaryStats[m]...)
	}
	return kind, stats
}
