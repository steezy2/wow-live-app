package service

import (
	"fmt"
	"testing"

	"wow-live/backend/models"
)

func TestTrackKeepsMobCasts(t *testing.T) {
	keys := newTracker(t, nil)
	run := newRun(models.Key{Start: 1 - keyCountdownMS, Status: "timed"}, "") // the clock is the log time - 1
	keys.current, run.party, keys.names["Player-A"] = run, []string{"Player-A"}, "Testmage"
	const mob = "Creature-0-3882-2859-27547-245336-00006BE4EA"
	cast := func(at int64, event, guid, flags, spell string, failed ...string) {
		keys.trackCast(at, event, append([]string{guid, "x", flags, "0x0", "0000000000000000", "nil", "0x0", "0x0",
			"1", spell, "0x20"}, failed...))
	}
	cast(1_001, "SPELL_CAST_START", mob, "0x10a48", "Void Bolt")
	cast(1_501, "SPELL_CAST_START", "Player-A", "0x511", "Frostbolt")
	cast(2_001, "SPELL_CAST_SUCCESS", mob, "0x10a48", "Void Bolt")
	cast(3_001, "SPELL_CAST_START", mob, "0x10a48", "Void Bolt")
	cast(3_501, "SPELL_CAST_FAILED", mob, "0x10a48", "Void Bolt", "Interrupted")
	cast(4_001, "SPELL_CAST_SUCCESS", mob, "0x10a48", "Shadow Slash")
	cast(4_501, "SPELL_CAST_SUCCESS", "Creature-0-3019-3004-1081-95072-00006A951E", "0x1114", "Pulverize") // a pet
	track := keys.trackOf(run)
	if got := fmt.Sprint(track.MobCasts); got != "map["+mob+":[{1000 Void Bolt 2000 false} {3000 Void Bolt 3500 true} "+
		"{4000 Shadow Slash 0 false}]]" {
		t.Errorf("mob casts = %s; want the bolt from 1 to 2 s, one interrupted at 3.5 s and the instant, not the "+
			"mage's or the pet's", got)
	}
	if got := fmt.Sprint(track.Casts["Testmage"]); got != "[{1500 Frostbolt 0 true}]" {
		t.Errorf("mage's casts = %s; want the Frostbolt the run cut", got)
	}
}
