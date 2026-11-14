package config

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"wow-live/backend/models"
)

func TestRootFindsTheRulesAbove(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, RulesFile), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	deep := filepath.Join(root, "backend", "cmd")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	if got, err := Root(deep); err != nil || got != root {
		t.Fatalf("Root = %q, %v; want %q", got, err, root)
	}
	if _, err := Root(t.TempDir()); err == nil {
		t.Fatal("want an error outside the repo")
	}
}

func TestLoadEnvSetsMissingKeysOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	text := "# comment\nWOW_TEST_A=\"one\"\nWOW_TEST_B = two\nnot a line\n"
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WOW_TEST_B", "already")
	t.Setenv("WOW_TEST_A", "")
	os.Unsetenv("WOW_TEST_A")
	if err := LoadEnv(path); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("WOW_TEST_A") != "one" || os.Getenv("WOW_TEST_B") != "already" {
		t.Fatalf("A=%q B=%q", os.Getenv("WOW_TEST_A"), os.Getenv("WOW_TEST_B"))
	}
	if err := LoadEnv(filepath.Join(t.TempDir(), "missing")); err != nil {
		t.Fatalf("a missing .env is fine, got %v", err)
	}
}

func TestLoadRaidMechanicsRefusesUnknownKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "raid-mechanics.toml")
	text := "raid = \"venomous-abyss\"\n[bosses.\"Ula'tek\"]\nmust_stops = [1]\n" // must_stop, misspelt
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRaidMechanics(path); err == nil {
		t.Fatal("a misspelt key loaded; want an error naming it")
	}
}

func TestTheRepoRulesFilesHaveNoUnknownKeys(t *testing.T) {
	root, err := RootFromWorkingDir()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRules(root); err != nil {
		t.Error(err)
	}
	if _, err := LoadRaidRules(root); err != nil {
		t.Error(err)
	}
	if _, err := LoadRaidMechanics(RaidMechanicsPath(root)); err != nil {
		t.Error(err)
	}
}

func TestSpecRulesDecodeBySpecWithTheirKinds(t *testing.T) {
	fsys := fstest.MapFS{"spec-rules.toml": {Data: []byte(`
[["Arcane Mage".rules]]
kind = "with"
cast = "Arcane Barrage"
buff = "Arcane Soul"
at_least = 0.9
why = "free damage"

[["Arcane Mage".rules]]
kind = "uptime"
buff = "Arcane Tempo"
at_least = 0.95
`)}}
	var rules models.SpecRules
	if err := DecodeTOML(fsys, "spec-rules.toml", &rules); err != nil {
		t.Fatal(err)
	}
	got := rules["Arcane Mage"].Rules
	if len(got) != 2 || got[0].Kind != "with" || got[0].AtLeast != 0.9 || got[1].Buff != "Arcane Tempo" {
		t.Errorf("rules = %+v, want the two Arcane Mage rules", got)
	}
}

func TestShippedSpecRulesParse(t *testing.T) {
	if _, err := LoadSpecRules(filepath.Join("..", "..")); err != nil {
		t.Errorf("spec-rules.toml: %v", err)
	}
}
