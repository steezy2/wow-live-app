package wcl

import (
	"bufio"
	"cmp"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"wow-live/backend/clients/combatlog"
	"wow-live/backend/models"
)

// ImportDir is the folder, in CacheDir, that ImportFight writes fights' zips into (and caches their pages in).
const ImportDir = "wcl-import"

// FightOptions is what a fight's combat log takes besides Warcraft Logs' data.
type FightOptions struct {
	// Logger is the character the log is written as ("Name-Realm"): it and its pets get the log's own character's
	// unit flag, so a key's Logger is it. Empty: no one.
	Logger string
	// Dungeons is the season's dungeon facts by challenge map ID (mplus-dungeons.json): a key's log starts with its
	// dungeon's, found by name.
	Dungeons map[int]models.DungeonFacts
}

// ImportFight writes a report fight as a combat log zipped at <CacheDir>\wcl-import\<code>-<fightID>.zip (a one-log
// zip like combatlog.Archive's, dated at the fight's end), then deletes its cached pages. It returns the zip's path.
func (c *Client) ImportFight(ctx context.Context, code string, fightID int, opts FightOptions) (string, error) {
	meta, err := c.fightMeta(ctx, code, fightID)
	if err != nil {
		return "", err
	}
	name := fmt.Sprintf("%s-%d", code, fightID)
	path := filepath.Join(c.CacheDir, ImportDir, name+".zip")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("importing fight %d of report %s: %w", fightID, code, err)
	}
	r, w := io.Pipe()
	go func() { w.CloseWithError(c.writeLog(ctx, code, meta, opts, w)) }()
	end := time.UnixMilli(meta.StartTime + meta.Fights[0].EndTime)
	err = combatlog.WriteZip(r, name+".txt", end, path)
	r.CloseWithError(err) // stops the writer when the zip failed first
	if err != nil {
		return "", fmt.Errorf("importing fight %d of report %s: %w", fightID, code, err)
	}
	return path, c.removePages(code, fightID)
}

// WriteFightLog writes a report fight to w as the game's combat log, which combatlog and the tracker read as if the
// game had written it: a key's or raid pull's start and end lines from the fight, then its events (fetched page by
// page, see fightEvents) as the lines they came from. Line times are UTC, so they parse to the same instants.
func (c *Client) WriteFightLog(ctx context.Context, code string, fightID int, opts FightOptions, w io.Writer) error {
	meta, err := c.fightMeta(ctx, code, fightID)
	if err != nil {
		return err
	}
	return c.writeLog(ctx, code, meta, opts, w)
}

// writeLog is WriteFightLog with the fight's metadata.
func (c *Client) writeLog(ctx context.Context, code string, meta fightMeta, opts FightOptions, w io.Writer) error {
	lw, err := newLogWriter(meta, opts, w)
	if err != nil {
		return fmt.Errorf("fight %d of report %s: %w", meta.Fights[0].ID, code, err)
	}
	err = c.fightEvents(ctx, code, meta, func(events []event) error {
		for i := range events {
			lw.event(&events[i])
		}
		return lw.out.Flush()
	})
	if err != nil {
		return err
	}
	lw.marksUntil(maxTime, false)
	if len(lw.skipped) > 0 {
		log.Printf("fight %d of report %s: skipped events of types Warcraft Logs has but the app doesn't read: %v",
			meta.Fights[0].ID, code, lw.skipped)
	}
	return lw.out.Flush()
}

const (
	// maxTime is later than any event.
	maxTime = int64(1) << 62
	// noUnit is the combat log's empty unit: no GUID, no name, no flags.
	noUnit = "0000000000000000,nil,0x80000000,0x80000000"
	// noGUID is the empty GUID, as advanced logging's owner of a unit that has none.
	noGUID = "0000000000000000"
	// mplusDifficulty is the game's difficulty ID for Mythic+ in ZONE_CHANGE and ENCOUNTER lines.
	mplusDifficulty = 8
	// meleeAbility is Warcraft Logs' ability ID for melee swings, which the log writes as SWING_ lines.
	meleeAbility = 1
	// autoShot is a hunter's Auto Shot, which the log writes as RANGE_ lines.
	autoShot = 75
	// maxEnvironmentAbility: Warcraft Logs numbers the environment's damage (Falling is 3) below any spell ID. A
	// sourceless hit with a real spell (a boss's lingering debuff) stays a SPELL_ line.
	maxEnvironmentAbility = 100
	// Unit flags: the game's type, control, reaction and affiliation bits.
	flagMine, flagParty, flagRaid        = 0x1, 0x2, 0x4
	flagFriendly, flagHostile            = 0x10, 0x40
	flagOutsider                         = 0x8
	flagPlayerControl, flagNPCControl    = 0x100, 0x200
	flagPlayerType, flagNPCType, flagPet = 0x400, 0x800, 0x1000
)

// logDifficulties is the game's difficulty ID for each of Warcraft Logs' raid difficulties.
var logDifficulties = map[int]int{1: 17, 3: 14, 4: 15, Mythic: 16}

// missTypes is the combat log's miss type for each of Warcraft Logs' hit types that misses.
var missTypes = map[int]string{0: "MISS", 7: "DODGE", 8: "PARRY", 9: "DEFLECT", 10: "IMMUNE", 12: "REFLECT",
	13: "EVADE", 14: "RESIST"}

// splitAbilities are the spells that move damage taken to another unit (Soul Link, Blessing of Sacrifice): the game
// logs that share as DAMAGE_SPLIT, Warcraft Logs as damage like any other.
var splitAbilities = map[int]bool{108446: true, 6940: true}

// event is one Warcraft Logs event, with the fields the combat log lines take; resources (includeResources) are
// the source's (resourceActor 1) or the target's (2).
type event struct {
	Timestamp          int64   `json:"timestamp"`
	Type               string  `json:"type"`
	SourceID           int     `json:"sourceID"`
	SourceInstance     int     `json:"sourceInstance"`
	TargetID           int     `json:"targetID"`
	TargetInstance     int     `json:"targetInstance"`
	AttackerID         int     `json:"attackerID"`
	AttackerInstance   int     `json:"attackerInstance"`
	AbilityGameID      int     `json:"abilityGameID"`
	ExtraAbilityGameID int     `json:"extraAbilityGameID"`
	HitType            int     `json:"hitType"`
	Amount             int64   `json:"amount"`
	UnmitigatedAmount  int64   `json:"unmitigatedAmount"`
	Absorbed           int64   `json:"absorbed"`
	Overheal           int64   `json:"overheal"`
	Overkill           int64   `json:"overkill"`
	Resisted           int64   `json:"resisted"`
	Blocked            int64   `json:"blocked"`
	Tick               bool    `json:"tick"`
	IsAoE              bool    `json:"isAoE"`
	Melee              bool    `json:"melee"`
	IsBuff             bool    `json:"isBuff"`
	Stack              int     `json:"stack"`
	ResourceChange     float64 `json:"resourceChange"`
	ResourceChangeType int     `json:"resourceChangeType"`
	Waste              float64 `json:"waste"`
	MaxResourceAmount  int64   `json:"maxResourceAmount"`
	resources
	combatant
}

// resources is a unit's state on an event, for advanced logging's block. Warcraft Logs keeps positions as the game's
// y and -x, times 100, and facing times 100.
type resources struct {
	ResourceActor  int             `json:"resourceActor"`
	ClassResources []classResource `json:"classResources"`
	HitPoints      int64           `json:"hitPoints"`
	MaxHitPoints   int64           `json:"maxHitPoints"`
	AttackPower    int64           `json:"attackPower"`
	SpellPower     int64           `json:"spellPower"`
	Armor          int64           `json:"armor"`
	Versatility    int64           `json:"versatility"`
	Avoidance      int64           `json:"avoidance"`
	Absorb         int64           `json:"absorb"`
	X              float64         `json:"x"`
	Y              float64         `json:"y"`
	Facing         float64         `json:"facing"`
	MapID          int             `json:"mapID"`
	ItemLevel      float64         `json:"itemLevel"`
}

// classResource is one of a unit's powers (mana, energy...).
type classResource struct {
	Amount int64 `json:"amount"`
	Max    int64 `json:"max"`
	Type   int   `json:"type"`
	Cost   int64 `json:"cost"`
}

// combatant is a combatantinfo event's player: stats (armor and avoidance are in resources, under the same names),
// spec, talents, gear and auras.
type combatant struct {
	Faction, SpecID                       int
	Strength, Agility, Stamina, Intellect int64
	Dodge, Parry, Block                   int64
	CritMelee, CritRanged, CritSpell      int64
	Speed, Leech                          int64
	HasteMelee, HasteRanged, HasteSpell   int64
	Mastery                               int64
	VersatilityDamageDone                 int64 `json:"versatilityDamageDone"`
	VersatilityHealingDone                int64 `json:"versatilityHealingDone"`
	VersatilityDamageReduction            int64 `json:"versatilityDamageReduction"`
	TalentTree                            []struct{ ID, Rank, NodeID int }
	PvpTalents                            []struct{ ID int }
	Gear                                  []gearItem
	Auras                                 []struct{ Source, Ability, Stacks int }
}

// gearItem is an equipped item as combatantinfo lists it.
type gearItem struct {
	ID, ItemLevel                                    int
	PermanentEnchant, TemporaryEnchant, OnUseEnchant int
	BonusIDs                                         []int
	Gems                                             []struct{ ID, ItemLevel int }
}

// mark is a line the fight's metadata gives (a key's or pull's start or end), written at its time: before the events
// of that ms for a start, after them for an end.
type mark struct {
	at     int64
	before bool
	text   string
}

// logWriter turns one fight's events into combat log lines.
type logWriter struct {
	out     *bufio.Writer
	base    int64 // the report's start, ms since the epoch
	units   map[int]unitInfo
	spells  map[int]ability
	marks   []mark
	skipped map[string]int // events of types the log doesn't get, by type
}

// unitInfo is an actor as the log names it.
type unitInfo struct {
	guid, name string // a creature's GUID ends with its instance
	flags      int
	owner      int // a pet's owner's actor ID
	creature   bool
}

// newLogWriter is a writer for meta's fight, with its start and end lines ready.
func newLogWriter(meta fightMeta, opts FightOptions, w io.Writer) (*logWriter, error) {
	fight := meta.Fights[0]
	lw := &logWriter{out: bufio.NewWriterSize(w, 1<<20), base: meta.StartTime, units: map[int]unitInfo{},
		spells: map[int]ability{}, skipped: map[string]int{}}
	for _, a := range meta.MasterData.Abilities {
		lw.spells[a.GameID] = a
	}
	lw.addUnits(meta, opts.Logger)
	zone := fmt.Sprintf(`ZONE_CHANGE,%d,%q,`, fight.GameZone.ID, fight.GameZone.Name)
	start, end := fight.StartTime, fight.EndTime
	if fight.KeystoneLevel > 0 {
		mapID := mapIDOf(opts.Dungeons, fight.Name)
		if mapID == 0 {
			return nil, fmt.Errorf("no challenge map ID for %q in the dungeon facts", fight.Name)
		}
		affixes := make([]string, len(fight.KeystoneAffixes))
		for i, a := range fight.KeystoneAffixes {
			affixes[i] = strconv.Itoa(a)
		}
		timed := 0
		if fight.KeystoneBonus > 0 {
			timed = 1
		}
		lw.mark(start, true, zone+strconv.Itoa(mplusDifficulty))
		lw.mark(start, true, fmt.Sprintf(`CHALLENGE_MODE_START,%q,%d,%d,%d,[%s]`, fight.Name, fight.GameZone.ID, mapID,
			fight.KeystoneLevel, strings.Join(affixes, ",")))
		for _, p := range fight.DungeonPulls {
			if p.EncounterID != 0 {
				lw.encounter(p, mplusDifficulty, fight.Size, fight.GameZone.ID)
			}
		}
		lw.mark(end, false, fmt.Sprintf(`CHALLENGE_MODE_END,%d,%d,%d,%d,0.000000,0.000000`, fight.GameZone.ID, timed,
			fight.KeystoneLevel, fight.KeystoneTime))
	} else {
		difficulty, ok := logDifficulties[fight.Difficulty]
		if fight.EncounterID == 0 || !ok {
			return nil, fmt.Errorf("fight %d is neither a key nor a raid boss pull", fight.ID)
		}
		lw.mark(start, true, zone+strconv.Itoa(difficulty))
		lw.encounter(pull{EncounterID: fight.EncounterID, Name: fight.Name, StartTime: start, EndTime: end,
			Kill: fight.Kill}, difficulty, fight.Size, fight.GameZone.ID)
	}
	slices.SortStableFunc(lw.marks, func(a, b mark) int { return cmp.Compare(a.at, b.at) })
	lw.out.WriteString(lw.stamp(start) + "  COMBAT_LOG_VERSION,22,ADVANCED_LOG_ENABLED,1,BUILD_VERSION,12.1.5," +
		"PROJECT_ID,1\n")
	return lw, nil
}

// addUnits names every actor of the report: players Player-0-<actor> "Name-Realm-Region", pets and NPCs
// Pet-/Creature-0-0-0-<actor>-<NPC ID>-<instance>, flagged by who they are to the logger.
func (lw *logWriter) addUnits(meta fightMeta, logger string) {
	fight := meta.Fights[0]
	affiliation := flagParty
	if fight.Size > 5 {
		affiliation = flagRaid
	}
	friendly, hostilePet := map[int]bool{}, map[int]bool{}
	for _, a := range fight.FriendlyNPCs {
		friendly[a.ID] = true
	}
	for _, a := range fight.EnemyPets {
		hostilePet[a.ID] = true
	}
	mine := map[int]bool{}
	for _, a := range meta.MasterData.Actors {
		if a.Type == "Player" && logger != "" && sameCharacter(a.Name+"-"+a.Server, logger) {
			mine[a.ID] = true
		}
	}
	for _, a := range meta.MasterData.Actors {
		u := unitInfo{name: a.Name, owner: a.PetOwner}
		who := affiliation
		if mine[a.ID] || mine[a.PetOwner] {
			who = flagMine
		}
		switch a.Type {
		case "Player":
			u.guid = fmt.Sprintf("Player-0-%d", a.ID)
			u.name = a.Name + "-" + strings.ReplaceAll(a.Server, " ", "") + "-" + meta.Region.CompactName
			u.flags = flagPlayerType | flagPlayerControl | flagFriendly | who
		case "Pet":
			u.guid, u.creature = fmt.Sprintf("Pet-0-0-0-%d-%d", a.ID, a.GameID), true
			u.flags = flagPet | flagPlayerControl | flagFriendly | who
			if hostilePet[a.ID] {
				u.flags = flagPet | flagNPCControl | flagHostile | flagOutsider
			}
		default:
			if a.ID < 0 { // the environment
				continue
			}
			u.guid, u.creature = fmt.Sprintf("Creature-0-0-0-%d-%d", a.ID, a.GameID), true
			u.flags = flagNPCType | flagNPCControl | flagHostile | flagOutsider
			if friendly[a.ID] {
				u.flags ^= flagHostile | flagFriendly
			}
		}
		lw.units[a.ID] = u
	}
}

// mapIDOf is the challenge map ID of the dungeon named name, 0 for none.
func mapIDOf(dungeons map[int]models.DungeonFacts, name string) int {
	for id, d := range dungeons {
		if d.Name == name {
			return id
		}
	}
	return 0
}

// sameCharacter is whether two "Name-Realm"s are one character, ignoring case, spaces and apostrophes in the realm.
func sameCharacter(a, b string) bool {
	norm := strings.NewReplacer(" ", "", "'", "").Replace
	return strings.EqualFold(norm(a), norm(b))
}

// encounter marks a boss pull's ENCOUNTER_START and ENCOUNTER_END.
func (lw *logWriter) encounter(p pull, difficulty, size, zone int) {
	head := fmt.Sprintf("%d,%q,%d,%d", p.EncounterID, p.Name, difficulty, size)
	lw.mark(p.StartTime, true, fmt.Sprintf("ENCOUNTER_START,%s,%d", head, zone))
	lw.mark(p.EndTime, false, fmt.Sprintf("ENCOUNTER_END,%s,%d,%d", head, flag(p.Kill), p.EndTime-p.StartTime))
}

// mark keeps a line from the fight's metadata for its time.
func (lw *logWriter) mark(at int64, before bool, text string) {
	lw.marks = append(lw.marks, mark{at: at, before: before, text: text})
}

// marksUntil writes the marks before an event at ms at (with before: those of that ms that go before it).
func (lw *logWriter) marksUntil(at int64, before bool) {
	for len(lw.marks) > 0 && (lw.marks[0].at < at || lw.marks[0].at == at && before && lw.marks[0].before) {
		lw.line(lw.marks[0].at, lw.marks[0].text)
		lw.marks = lw.marks[1:]
	}
}

// stamp is the line time of ms since the report's start, in UTC: "11/8/2026 17:37:53.476+0".
func (lw *logWriter) stamp(ms int64) string {
	return time.UnixMilli(lw.base+ms).UTC().Format("1/2/2006 15:04:05.000") + "+0"
}

// line writes one line at ms since the report's start.
func (lw *logWriter) line(ms int64, text string) {
	lw.out.WriteString(lw.stamp(ms))
	lw.out.WriteString("  ")
	lw.out.WriteString(text)
	lw.out.WriteByte('\n')
}

// event writes the line an event came from, or counts it as skipped when the log's reader doesn't use its type.
func (lw *logWriter) event(e *event) {
	lw.marksUntil(e.Timestamp, true)
	text := lw.text(e)
	if text == "" {
		lw.skipped[e.Type]++
		return
	}
	lw.line(e.Timestamp, text)
}

// text is an event's line after its time, "" for a type the log's reader doesn't use.
func (lw *logWriter) text(e *event) string {
	src, dst := lw.unit(e.SourceID, e.SourceInstance), lw.unit(e.TargetID, e.TargetInstance)
	units, spell := src+","+dst, lw.spell(e.AbilityGameID)
	switch e.Type {
	case "damage":
		return lw.damage(e, src, dst)
	case "heal":
		amount := e.Amount + e.Overheal
		return fmt.Sprintf("%s,%s,%s,%s,%d,%d,%d,%d,%s", spellPrefix(e)+"HEAL", units, spell,
			lw.advanced(e, 2), amount, amount, e.Overheal, e.Absorbed, crit(e.HitType))
	case "absorbed":
		attack := ""
		if e.ExtraAbilityGameID != meleeAbility {
			attack = lw.spell(e.ExtraAbilityGameID) + ","
		}
		return fmt.Sprintf("SPELL_ABSORBED,%s,%s,%s%s,%s,%d,0,nil", lw.unit(e.AttackerID, e.AttackerInstance), dst,
			attack, src, spell, e.Amount)
	case "applybuff", "applydebuff":
		return "SPELL_AURA_APPLIED," + units + "," + spell + "," + auraKind(e.Type)
	case "removebuff", "removedebuff":
		return "SPELL_AURA_REMOVED," + units + "," + spell + "," + auraKind(e.Type)
	case "refreshbuff", "refreshdebuff":
		return "SPELL_AURA_REFRESH," + units + "," + spell + "," + auraKind(e.Type)
	case "applybuffstack", "applydebuffstack":
		return fmt.Sprintf("SPELL_AURA_APPLIED_DOSE,%s,%s,%s,%d", units, spell, auraKind(e.Type), e.Stack)
	case "begincast":
		return "SPELL_CAST_START," + units + "," + spell
	case "cast":
		if e.Melee { // Warcraft Logs' own record of a swing, which the log has as SWING_ lines only
			return ""
		}
		return "SPELL_CAST_SUCCESS," + units + "," + spell + "," + lw.advanced(e, 1)
	case "interrupt":
		return "SPELL_INTERRUPT," + units + "," + spell + "," + lw.extraSpell(e.ExtraAbilityGameID)
	case "dispel", "spellsteal":
		event := map[string]string{"dispel": "SPELL_DISPEL", "spellsteal": "SPELL_STOLEN"}[e.Type]
		kind := "DEBUFF"
		if e.IsBuff {
			kind = "BUFF"
		}
		return event + "," + units + "," + spell + "," + lw.extraSpell(e.ExtraAbilityGameID) + "," + kind
	case "summon":
		return "SPELL_SUMMON," + units + "," + spell
	case "resurrect":
		return "SPELL_RESURRECT," + units + "," + spell
	case "resourcechange":
		return fmt.Sprintf("SPELL_ENERGIZE,%s,%s,%s,%.4f,%.4f,%d,%d", units, spell, lw.advanced(e, 1), e.ResourceChange,
			e.Waste, e.ResourceChangeType, e.MaxResourceAmount)
	case "death":
		return "UNIT_DIED," + noUnit + "," + dst + ",0"
	case "combatantinfo":
		return lw.combatantInfo(e)
	}
	return ""
}

// damage is a damage event's line: SWING_ for melee, ENVIRONMENTAL_DAMAGE for the environment's, DAMAGE_SPLIT for a
// share of another's (see spellPrefix for the rest), _MISSED for a miss or a hit absorbed whole. The log's amount
// includes the overkill; Warcraft Logs' doesn't.
func (lw *logWriter) damage(e *event, src, dst string) string {
	amount := e.Amount + max(e.Overkill, 0)
	base := cmp.Or(e.UnmitigatedAmount, amount+e.Absorbed)
	miss, missed := missTypes[e.HitType]
	missed = missed && e.Amount == 0 && e.Absorbed == 0
	if e.Amount == 0 && e.Absorbed > 0 {
		miss, missed = "ABSORB", true
	}
	hit := fmt.Sprintf("%d,%d,%d,%d,%d,%d,%d,%s,%s,%s", amount, base, cmp.Or(e.Overkill, -1),
		lw.spells[e.AbilityGameID].school(), e.Resisted, e.Blocked, e.Absorbed, crit(e.HitType), flagIf(e.HitType == 6),
		flagIf(e.HitType == 15))
	if _, known := lw.units[e.SourceID]; !known && e.AbilityGameID < maxEnvironmentAbility {
		return fmt.Sprintf("ENVIRONMENTAL_DAMAGE,%s,%s,%s,%s,%s", noUnit, dst, lw.advanced(e, 2),
			lw.spells[e.AbilityGameID].Name, hit)
	}
	missText := miss + ",nil"
	if miss == "ABSORB" {
		missText += fmt.Sprintf(",%d,%d,%s", e.Absorbed, base, crit(e.HitType))
	}
	reach := "ST"
	if e.IsAoE {
		reach = "AOE"
	}
	switch {
	case splitAbilities[e.AbilityGameID]:
		return "DAMAGE_SPLIT," + src + "," + dst + "," + lw.spell(e.AbilityGameID) + "," + lw.advanced(e, 2) + "," + hit +
			"," + reach
	case e.AbilityGameID == meleeAbility && missed:
		return "SWING_MISSED," + src + "," + dst + "," + missText
	case e.AbilityGameID == meleeAbility:
		return "SWING_DAMAGE," + src + "," + dst + "," + lw.advanced(e, 1) + "," + hit
	case missed:
		return spellPrefix(e) + "MISSED," + src + "," + dst + "," + lw.spell(e.AbilityGameID) + "," +
			missText + "," + reach
	}
	return spellPrefix(e) + "DAMAGE," + src + "," + dst + "," + lw.spell(e.AbilityGameID) + "," +
		lw.advanced(e, 2) + "," + hit + "," + reach
}

// spellPrefix is a spell event's prefix: SPELL_PERIODIC_ for a tick, RANGE_ for Auto Shot, else SPELL_.
func spellPrefix(e *event) string {
	switch {
	case e.Tick:
		return "SPELL_PERIODIC_"
	case e.AbilityGameID == autoShot:
		return "RANGE_"
	}
	return "SPELL_"
}

// unit is an actor as the log's four unit fields: GUID, "name", flags, raid flags.
func (lw *logWriter) unit(id, instance int) string {
	u, ok := lw.units[id]
	if !ok {
		return noUnit
	}
	return fmt.Sprintf("%s,%q,0x%x,0x80000000", lw.guid(u, instance), u.name, u.flags)
}

// guid is a unit's GUID; a creature's ends with its instance, so each mob of an NPC has its own.
func (lw *logWriter) guid(u unitInfo, instance int) string {
	if u.creature {
		return fmt.Sprintf("%s-%010X", u.guid, instance)
	}
	return u.guid
}

// spell is an ability as the log's three spell fields: ID, "name", school.
func (lw *logWriter) spell(id int) string {
	a := lw.spells[id]
	return fmt.Sprintf("%d,%q,0x%x", id, cmp.Or(a.Name, "Unknown"), a.school())
}

// extraSpell is the interrupted, dispelled or stolen spell: ID, "name", school (in decimal, as the log has it).
func (lw *logWriter) extraSpell(id int) string {
	a := lw.spells[id]
	return fmt.Sprintf("%d,%q,%d", id, cmp.Or(a.Name, "Unknown"), a.school())
}

// school is an ability's spell school (a bit per school).
func (a ability) school() int {
	v, _ := strconv.Atoi(a.Type)
	return v
}

// advanced is advanced logging's block for the event's resource actor, or for the unit fallback names (1 source, 2
// target) when Warcraft Logs kept no resources: GUID, owner, health, max health, attack and spell power, armor,
// versatility, avoidance, absorb, power type, power, max power, cost, x, y, uiMap, facing, level. Missing values are
// the game's 0.
func (lw *logWriter) advanced(e *event, fallback int) string {
	id, instance := e.TargetID, e.TargetInstance
	if cmp.Or(e.ResourceActor, fallback) == 1 {
		id, instance = e.SourceID, e.SourceInstance
	}
	u, ok := lw.units[id]
	if !ok {
		return noGUID + "," + noGUID + strings.Repeat(",0", 17)
	}
	owner := noGUID
	if o, ok := lw.units[u.owner]; ok {
		owner = lw.guid(o, 0)
	}
	r := e.resources
	var power classResource
	if len(r.ClassResources) > 0 {
		power = r.ClassResources[0]
	}
	return fmt.Sprintf("%s,%s,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%.2f,%.2f,%d,%.4f,%g", lw.guid(u, instance), owner,
		r.HitPoints, r.MaxHitPoints, r.AttackPower, r.SpellPower, r.Armor, r.Versatility, r.Avoidance, r.Absorb,
		power.Type, power.Amount, power.Max, power.Cost, r.Y/100, 0-r.X/100, // 0-: not "-0.00"
		r.MapID, r.Facing/100, r.ItemLevel)
}

// combatantInfo is COMBATANT_INFO in the game's field order: GUID, faction, stats, spec, [talents], (PvP talents),
// [gear], [auras], and four PvP fields.
func (lw *logWriter) combatantInfo(e *event) string {
	p := e.combatant
	var b strings.Builder
	fmt.Fprintf(&b, "COMBATANT_INFO,%s,%d,%d,%d,%d,%d,%d,%d,%d,0,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,",
		lw.guid(lw.units[e.SourceID], 0), p.Faction, p.Strength, p.Agility, p.Stamina, p.Intellect, p.Dodge, p.Parry,
		p.Block, p.CritMelee, p.CritRanged, p.CritSpell, p.Speed, p.Leech, p.HasteMelee, p.HasteRanged, p.HasteSpell,
		e.Avoidance, p.Mastery, p.VersatilityDamageDone, p.VersatilityHealingDone, p.VersatilityDamageReduction,
		e.Armor, p.SpecID)
	talents := make([]string, len(p.TalentTree))
	for i, t := range p.TalentTree {
		talents[i] = fmt.Sprintf("(%d,%d,%d)", t.NodeID, t.ID, t.Rank)
	}
	pvp := make([]string, len(p.PvpTalents))
	for i, t := range p.PvpTalents {
		pvp[i] = strconv.Itoa(t.ID)
	}
	gear := make([]string, len(p.Gear))
	for i, g := range p.Gear {
		gear[i] = g.text()
	}
	auras := make([]string, len(p.Auras))
	for i, a := range p.Auras {
		auras[i] = fmt.Sprintf("%s,%d,%d", lw.guid(lw.units[a.Source], 0), a.Ability, a.Stacks)
	}
	fmt.Fprintf(&b, "[%s],(%s),[%s],[%s],0,0,0,0", strings.Join(talents, ","), strings.Join(pvp, ","),
		strings.Join(gear, ","), strings.Join(auras, ","))
	return b.String()
}

// text is an item as COMBATANT_INFO lists it: (itemID,itemLevel,(enchants),(bonusIDs),(gems)).
func (g gearItem) text() string {
	enchants := ""
	if g.PermanentEnchant != 0 || g.TemporaryEnchant != 0 || g.OnUseEnchant != 0 {
		enchants = fmt.Sprintf("%d,%d,%d", g.PermanentEnchant, g.TemporaryEnchant, g.OnUseEnchant)
	}
	gems := make([]string, 0, 2*len(g.Gems))
	for _, gem := range g.Gems {
		gems = append(gems, strconv.Itoa(gem.ID), strconv.Itoa(gem.ItemLevel))
	}
	return fmt.Sprintf("(%d,%d,(%s),(%s),(%s))", g.ID, g.ItemLevel, enchants, joinInts(g.BonusIDs),
		strings.Join(gems, ","))
}

func joinInts(v []int) string {
	s := make([]string, len(v))
	for i, n := range v {
		s[i] = strconv.Itoa(n)
	}
	return strings.Join(s, ",")
}

// auraKind is BUFF or DEBUFF, from an aura event's type.
func auraKind(eventType string) string {
	if strings.Contains(eventType, "debuff") {
		return "DEBUFF"
	}
	return "BUFF"
}

// crit is the log's critical field for a hit type: 1 for a crit (blocked or resisted ones too), else nil.
func crit(hitType int) string {
	return flagIf(hitType == 2 || hitType == 5 || hitType == 17)
}

func flagIf(on bool) string {
	if on {
		return "1"
	}
	return "nil"
}

func flag(on bool) int {
	if on {
		return 1
	}
	return 0
}
