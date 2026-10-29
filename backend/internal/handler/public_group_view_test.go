package handler

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestPublicGroupPlatform(t *testing.T) {
	cases := []struct {
		name     string
		platform string
		want     string
	}{
		{"internal pool label is neutralised", "yunwu", "standard"},
		{"internal pool label is case insensitive", "YunWu", "standard"},
		{"surrounding whitespace is ignored", "  yunwu  ", "standard"},
		{"first-class platforms are published unchanged", "anthropic", "anthropic"},
		{"openai is published unchanged", "openai", "openai"},
		{"empty stays empty", "", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := publicGroupPlatform(tc.platform); got != tc.want {
				t.Fatalf("publicGroupPlatform(%q) = %q, want %q", tc.platform, got, tc.want)
			}
		})
	}
}

func TestPublicGroupName(t *testing.T) {
	cases := []struct {
		name     string
		platform string
		group    string
		want     string
	}{
		{"pool label as the whole name is neutralised", "yunwu", "yunwu", "standard"},
		{"pool label inside the name is neutralised", "yunwu", "Yunwu Pool", "standard"},
		{"pool label suffix is neutralised", "yunwu", "yunwu-smart", "standard"},
		{"case insensitive", "yunwu", "YUNWU", "standard"},
		{"billing tier names survive", "yunwu", "smart", "smart"},
		{"billing tier names survive (fast)", "yunwu", "fast", "fast"},
		{"admin-chosen names survive", "yunwu", "Premium Claude", "Premium Claude"},
		{"other platforms are never rewritten", "anthropic", "yunwu", "yunwu"},
		{"empty name survives", "yunwu", "", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := publicGroupName(tc.platform, tc.group); got != tc.want {
				t.Fatalf("publicGroupName(%q, %q) = %q, want %q", tc.platform, tc.group, got, tc.want)
			}
		})
	}
}

// A user-facing channel response must never carry the internal "yunwu" label —
// not on the platform section, not on the groups inside it and not on the
// supported models.
func TestBuildPlatformSections_ProjectsInternalPoolLabel(t *testing.T) {
	channel := service.AvailableChannel{
		SupportedModels: []service.SupportedModel{
			{Name: "claude-sonnet-5", Platform: "yunwu"},
			{Name: "gpt-4o", Platform: "openai"},
		},
	}
	groups := []userAvailableGroup{
		{ID: 1, Name: "yunwu", Platform: "yunwu"},
		{ID: 2, Name: "smart", Platform: "yunwu"},
		{ID: 3, Name: "Default", Platform: "openai"},
	}

	sections := buildPlatformSections(channel, groups)
	if len(sections) != 2 {
		t.Fatalf("expected 2 platform sections, got %d (%+v)", len(sections), sections)
	}

	byPlatform := map[string]userChannelPlatformSection{}
	for _, s := range sections {
		if s.Platform == "yunwu" {
			t.Fatalf("platform section still publishes the internal pool label: %+v", s)
		}
		byPlatform[s.Platform] = s
	}

	pool, ok := byPlatform[neutralPoolPlatform]
	if !ok {
		t.Fatalf("expected a %q section, got %+v", neutralPoolPlatform, byPlatform)
	}
	if len(pool.Groups) != 2 {
		t.Fatalf("expected both pool groups in the section, got %+v", pool.Groups)
	}
	names := map[int64]string{}
	for _, g := range pool.Groups {
		if g.Platform != neutralPoolPlatform {
			t.Fatalf("group %d still publishes platform %q", g.ID, g.Platform)
		}
		names[g.ID] = g.Name
	}
	if names[1] != neutralPoolGroupName {
		t.Fatalf("pool-named group was not neutralised: %q", names[1])
	}
	if names[2] != "smart" {
		t.Fatalf("billing tier group name must survive, got %q", names[2])
	}

	// The pool models must still be attached to the pool section (the neutral
	// label is a presentation concern, not a filter).
	if len(pool.SupportedModels) != 1 || pool.SupportedModels[0].Name != "claude-sonnet-5" {
		t.Fatalf("unexpected pool models: %+v", pool.SupportedModels)
	}
	if pool.SupportedModels[0].Platform != neutralPoolPlatform {
		t.Fatalf("model row still publishes platform %q", pool.SupportedModels[0].Platform)
	}

	// The unrelated platform is untouched and keeps only its own models.
	openai, ok := byPlatform["openai"]
	if !ok {
		t.Fatalf("expected the openai section, got %+v", byPlatform)
	}
	if len(openai.SupportedModels) != 1 || openai.SupportedModels[0].Name != "gpt-4o" {
		t.Fatalf("unexpected openai models: %+v", openai.SupportedModels)
	}
}

func TestIsInternalPoolPlatform(t *testing.T) {
	if !isInternalPoolPlatform("yunwu") || !isInternalPoolPlatform(" Yunwu ") {
		t.Fatal("expected the internal pool label to be recognised")
	}
	if isInternalPoolPlatform("anthropic") || isInternalPoolPlatform("") {
		t.Fatal("expected a first-class platform not to be treated as the pool")
	}
}
