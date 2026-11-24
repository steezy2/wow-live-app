package service

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestCustomLinesAllowlist(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		want    []string
		wantErr string // part of the error; "" for none
	}{
		{"blank and comment lines are dropped", "\n  # a note\r\n\n", nil, ""},
		{"an action list", "actions=/auto_attack\nactions+=/call_action_list,name=aoe,if=active_enemies>2",
			[]string{"actions=/auto_attack", "actions+=/call_action_list,name=aoe,if=active_enemies>2"}, ""},
		{"a named list, keys trimmed and lowercased", " Actions.AoE += /spinning_crane_kick ",
			[]string{"actions.aoe+=/spinning_crane_kick"}, ""},
		{"an empty action list clears it", "actions.precombat=", []string{"actions.precombat="}, ""},
		{"listed options", "optimal_raid=0\nFIXED_TIME = 1\nvary_combat_length=0.2",
			[]string{"optimal_raid=0", "fixed_time=1", "vary_combat_length=0.2"}, ""},
		{"an override", "override.bloodlust=0", []string{"override.bloodlust=0"}, ""},
		{"an APL variable", "apl_variable.pool_energy=1", []string{"apl_variable.pool_energy=1"}, ""},
		{"a word value", "position=back", []string{"position=back"}, ""},
		{"over the size cap", strings.Repeat("#", maxCustomBytes+1), nil, "over 16 KB"},
		{"not key=value", "optimal_raid", nil, "not key=value"},
		{"an unlisted key", "enemy=Boss", nil, "not an option"},
		{"a prefix alone", "override.=1", nil, "not an option"},
		{"input", "input=C:/x.simc", nil, "input is not allowed"},
		{"output", "output=out.txt", nil, "output is not allowed"},
		{"html", "html=report.html", nil, "html is not allowed"},
		{"json", "json=r.json", nil, "json is not allowed"},
		{"json2", "json2=r.json", nil, "json is not allowed"},
		{"xml", "xml=r.xml", nil, "xml is not allowed"},
		{"csv", "reforge_plot_output_file=a.csv", nil, "output is not allowed"},
		{"save", "save=profile.simc", nil, "save is not allowed"},
		{"save_actions", "save_actions=a.simc", nil, "save is not allowed"},
		{"load", "load_default=1", nil, "load is not allowed"},
		{"a profileset", "profileset.x+=head=,id=1", nil, "profileset is not allowed"},
		{"copy", "copy=Other", nil, "copy is not allowed"},
		{"proxy", "proxy=http,host,80", nil, "proxy is not allowed"},
		{"armory", "armory=us,realm,name", nil, "armory is not allowed"},
		{"apikey", "apikey=abc", nil, "apikey is not allowed"},
		{"threads", "threads=64", nil, "threads is not allowed"},
		{"iterations", "iterations=1", nil, "iterations is not allowed"},
		{"target_error", "target_error=0.01", nil, "target_error is not allowed"},
		{"max_time", "max_time=1", nil, "max_time is not allowed"},
		{"fight_style", "fight_style=DungeonSlice", nil, "fight_style is not allowed"},
		{"desired_targets", "desired_targets=5", nil, "desired_targets is not allowed"},
		{"a refused word in a value", "position=output", nil, "output is not allowed"},
		{"a path in a value", "seed=a/b", nil, "a value is"},
		{"a backslash in a value", "seed=a\\b", nil, "a value is"},
		{"dot dot in a value", "seed=..", nil, "a value is"},
		{"a second option after a space", "optimal_raid=0 output=x.txt", nil, "output is not allowed"},
		{"a space in an action list", "actions+=/wait output=x.txt", nil, "no spaces"},
		{"a variable in an action list", "actions+=/$(x)", nil, "no spaces"},
		{"a NUL in an action list", "actions+=/a\x00b", nil, "no spaces"},
		{"a NUL in a key", "optimal_raid\x00=0", nil, "not an option"},
		{"+= on an option", "optimal_raid+=1", nil, "only an action list"},
		{"a bad list name", "actions.a-b=/x", nil, "not an option"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := customLines(tt.text)
			if tt.wantErr != "" {
				if !errors.Is(err, ErrBadSimOptions) || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want ErrBadSimOptions with %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || !slices.Equal(got, tt.want) {
				t.Errorf("customLines = %q, %v; want %q", got, err, tt.want)
			}
		})
	}
}
