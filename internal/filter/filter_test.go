package filter

import "testing"

func TestMatch(t *testing.T) {
	cases := []struct {
		name         string
		file         string
		rules        []Rule
		matchAll, cs bool
		want         bool
	}{
		{"no rules", "a.txt", nil, true, false, true},
		{"empty value matches", "a.txt", []Rule{{Mode: Contains, Value: " "}}, true, false, true},
		{"contains ci", "The Whale.mp3", []Rule{{Mode: Contains, Value: "whale"}}, true, false, true},
		{"contains cs", "The Whale.mp3", []Rule{{Mode: Contains, Value: "whale"}}, true, true, false},
		{"starts", "The Whale.mp3", []Rule{{Mode: StartsWith, Value: "the"}}, true, false, true},
		{"ends", "The Whale.mp3", []Rule{{Mode: EndsWith, Value: ".MP3"}}, true, false, true},
		{"ext plain", "a.PNG", []Rule{{Mode: Extension, Value: "png"}}, true, false, true},
		{"ext dotted", "a.png", []Rule{{Mode: Extension, Value: ".png"}}, true, false, true},
		{"ext compound", "a.tar.gz", []Rule{{Mode: Extension, Value: "tar.gz"}}, true, false, true},
		{"ext matches last part", "a.tar.gz", []Rule{{Mode: Extension, Value: "gz"}}, true, false, true},
		{"ext not a dotfile", ".png", []Rule{{Mode: Extension, Value: "png"}}, true, false, false},
		{"ext cs", "a.PNG", []Rule{{Mode: Extension, Value: "png"}}, true, true, false},
		{"AND fails", "a.png", []Rule{{Mode: Contains, Value: "a"}, {Mode: Extension, Value: "jpg"}}, true, false, false},
		{"OR passes", "a.png", []Rule{{Mode: Contains, Value: "zzz"}, {Mode: Extension, Value: "png"}}, false, false, true},
		{"OR none", "a.png", []Rule{{Mode: Contains, Value: "zzz"}, {Mode: Extension, Value: "jpg"}}, false, false, false},
	}
	for _, c := range cases {
		if got := Match(c.file, c.rules, c.matchAll, c.cs); got != c.want {
			t.Errorf("%s: Match = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestFiles(t *testing.T) {
	all := []string{"/d/a.png", "/d/b.jpg", "/d/c.png"}
	got := Files(all, []Rule{{Mode: Extension, Value: "png"}}, true, false)
	if len(got) != 2 || got[0] != "/d/a.png" || got[1] != "/d/c.png" {
		t.Fatalf("got %v", got)
	}
}

func TestRegexMode(t *testing.T) {
	rule := []Rule{{Mode: Regex, Value: `^img_\d{3}\.jpe?g$`}}
	if !Match("IMG_042.JPG", rule, true, false) {
		t.Error("case-insensitive regex should match")
	}
	if Match("IMG_042.JPG", rule, true, true) {
		t.Error("case-sensitive regex should not match")
	}
	if Match("IMG_42.JPG", rule, true, false) {
		t.Error("should not match 2 digits")
	}
	bad := Rule{Mode: Regex, Value: "("}
	if Match("x", []Rule{bad}, true, false) {
		t.Error("invalid regex matches nothing")
	}
	if Validate(bad) == "" || Validate(rule[0]) != "" || Validate(Rule{Mode: Contains, Value: "("}) != "" {
		t.Error("Validate misbehaves")
	}
}
