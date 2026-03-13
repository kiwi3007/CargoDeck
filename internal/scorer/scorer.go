// Package scorer enriches indexer search results with structured metadata and
// a composite confidence score, without making any network or database calls.
package scorer

import (
	"regexp"
	"sort"
	"strings"

	"github.com/kiwi3007/playerr/internal/indexer"
)

// ── Source catalogue ──────────────────────────────────────────────────────────

type sourceEntry struct {
	name          string
	tier          int
	installMethod string // "DirectExtract" | "RunInstaller"
	patterns      []string
}

// sources is ordered most-specific first so the first match wins.
var sources = []sourceEntry{
	// Tier 2 — reputable repackers that bundle proprietary installers
	{name: "FitGirl", tier: 2, installMethod: "RunInstaller", patterns: []string{"FITGIRL", "FIT-GIRL", "FIT_GIRL"}},
	{name: "DODI", tier: 2, installMethod: "RunInstaller", patterns: []string{"DODI-REPACK", "DODI"}},
	{name: "ElAmigos", tier: 2, installMethod: "RunInstaller", patterns: []string{"ELAMIGOS", "EL-AMIGOS", "EL_AMIGOS"}},
	{name: "KAOS", tier: 2, installMethod: "RunInstaller", patterns: []string{"KAOS"}},
	{name: "Xatab", tier: 2, installMethod: "RunInstaller", patterns: []string{"XATAB", "R.G.MECHANICS", "RG.MECHANICS", "RGMECHANICS"}},
	{name: "KaOs", tier: 2, installMethod: "RunInstaller", patterns: []string{"KAOS"}},

	// Tier 1 — digital store releases (archives, no proprietary installer)
	{name: "GOG", tier: 1, installMethod: "DirectExtract", patterns: []string{"(GOG)", "-GOG", "_GOG", ".GOG"}},
	{name: "Steam", tier: 1, installMethod: "DirectExtract", patterns: []string{"(STEAM)", "-STEAM", "_STEAM"}},

	// Tier 3 — scene groups (typically rar/zip archives)
	{name: "TENOKE", tier: 3, installMethod: "DirectExtract", patterns: []string{"TENOKE"}},
	{name: "FLT", tier: 3, installMethod: "DirectExtract", patterns: []string{"-FLT", ".FLT"}},
	{name: "CODEX", tier: 3, installMethod: "DirectExtract", patterns: []string{"CODEX"}},
	{name: "RUNE", tier: 3, installMethod: "DirectExtract", patterns: []string{"-RUNE"}},
	{name: "SKIDROW", tier: 3, installMethod: "DirectExtract", patterns: []string{"SKIDROW"}},
	{name: "RELOADED", tier: 3, installMethod: "DirectExtract", patterns: []string{"RELOADED"}},
	{name: "PROPHET", tier: 3, installMethod: "DirectExtract", patterns: []string{"PROPHET"}},
	{name: "CPY", tier: 3, installMethod: "DirectExtract", patterns: []string{"-CPY"}},
	{name: "EMPRESS", tier: 3, installMethod: "DirectExtract", patterns: []string{"EMPRESS"}},
	{name: "RAZOR1911", tier: 3, installMethod: "DirectExtract", patterns: []string{"RAZOR1911", "RAZOR"}},
	{name: "GOLDBERG", tier: 3, installMethod: "DirectExtract", patterns: []string{"GOLDBERG"}},
	{name: "DARKSiDERS", tier: 3, installMethod: "DirectExtract", patterns: []string{"DARKSIDERS", "DARKSIDE"}},
	{name: "PLAZA", tier: 3, installMethod: "DirectExtract", patterns: []string{"-PLAZA"}},
	{name: "HOODLUM", tier: 3, installMethod: "DirectExtract", patterns: []string{"HOODLUM"}},
}

// ── Compiled regexes (package init) ───────────────────────────────────────────

var (
	reVersion    = regexp.MustCompile(`(?i)v(\d+\.\d+[\.\d]*)`)
	reBuild      = regexp.MustCompile(`(?i)build[\s._-]?(\d{4,8})`)
	reUpdateNum  = regexp.MustCompile(`(?i)(?:update|patch)[\s._-]?(\d+)`)
	reLangTokens = regexp.MustCompile(`(?i)[\[(]([A-Z]{2}(?:[,+][A-Z]{2})+)[\])]`) // e.g. [EN,FR,DE]
	reYear       = regexp.MustCompile(`(?:19|20)\d\d`)
	reNonAlNum   = regexp.MustCompile(`[^a-z0-9 ]+`)
	reMultiSpace = regexp.MustCompile(`\s{2,}`)
)

// ── Source detection ──────────────────────────────────────────────────────────

func detectSource(title string) (name string, tier int, installMethod string) {
	upper := strings.ToUpper(title)
	for _, s := range sources {
		for _, p := range s.patterns {
			if strings.Contains(upper, p) {
				return s.name, s.tier, s.installMethod
			}
		}
	}
	return "", 4, "Unknown"
}

// ── Version extraction ────────────────────────────────────────────────────────

func extractVersion(title string) string {
	if m := reVersion.FindStringSubmatch(title); len(m) > 1 {
		return "v" + m[1]
	}
	if m := reBuild.FindStringSubmatch(title); len(m) > 1 {
		return "Build " + m[1]
	}
	if m := reUpdateNum.FindStringSubmatch(title); len(m) > 1 {
		return "Update " + m[1]
	}
	return ""
}

// ── Language extraction ───────────────────────────────────────────────────────

var langKeywords = map[string]string{
	"MULTI":      "MULTI",
	"ENGLISH":    "EN",
	"FRENCH":     "FR",
	"GERMAN":     "DE",
	"SPANISH":    "ES",
	"PORTUGUESE": "PT",
	"RUSSIAN":    "RU",
	"ITALIAN":    "IT",
	"JAPANESE":   "JA",
	"CHINESE":    "ZH",
	"KOREAN":     "KO",
	"POLISH":     "PL",
	"CZECH":      "CS",
	"HUNGARIAN":  "HU",
	"DUTCH":      "NL",
	"TURKISH":    "TR",
}

func extractLanguages(title string) []string {
	upper := strings.ToUpper(title)
	seen := make(map[string]bool)

	// Try compact bracket format first: [EN,FR,DE]
	if m := reLangTokens.FindStringSubmatch(upper); len(m) > 1 {
		for _, code := range strings.Split(m[1], ",") {
			seen[strings.TrimSpace(code)] = true
		}
	}

	// Then check for keyword matches
	for kw, code := range langKeywords {
		if strings.Contains(upper, kw) {
			seen[code] = true
		}
	}

	if len(seen) == 0 {
		return nil
	}

	langs := make([]string, 0, len(seen))
	for l := range seen {
		langs = append(langs, l)
	}
	sort.Strings(langs)
	return langs
}

// ── Release type detection ────────────────────────────────────────────────────

func detectReleaseType(title string) string {
	// Analyse the portion after the last hyphen (the scene group / metadata suffix)
	// to avoid false positives from game titles containing these words.
	upper := strings.ToUpper(title)

	parts := strings.Split(upper, "-")
	suffix := ""
	if len(parts) > 1 {
		suffix = strings.Join(parts[1:], "-")
	}
	check := suffix
	if check == "" {
		check = upper
	}

	if containsToken(check, "DLC", "SEASON.PASS", "EXPANSION") {
		return "dlc"
	}
	if containsToken(check, "UPDATE", "PATCH", "HOTFIX", "CRACKFIX") {
		return "update"
	}
	// Version-labelled releases without a base game word are often updates
	if reVersion.MatchString(upper) && containsToken(upper, "UPDATE", "PATCH") {
		return "update"
	}
	if containsToken(check, "DEMO", "TRIAL") {
		return "demo"
	}
	return "game"
}

func containsToken(s string, tokens ...string) bool {
	for _, t := range tokens {
		if strings.Contains(s, t) {
			return true
		}
	}
	return false
}

// ── Size plausibility ─────────────────────────────────────────────────────────

const (
	MB int64 = 1024 * 1024
	GB int64 = 1024 * MB
)

func sizeWarning(sizeBytes int64, releaseType, installMethod string) string {
	if sizeBytes <= 0 {
		return ""
	}
	// Only flag "too_large" with absolute upper bounds — lower-bound "too_small"
	// is intentionally omitted here because some games are genuinely small.
	// The frontend detects outliers relative to the full result set instead.
	var hi int64
	switch releaseType {
	case "update":
		hi = 10 * GB
	case "dlc":
		hi = 30 * GB
	case "demo":
		hi = 15 * GB
	default: // "game"
		if installMethod == "RunInstaller" {
			hi = 100 * GB
		} else {
			hi = 80 * GB
		}
	}
	if sizeBytes > hi {
		return "too_large"
	}
	return ""
}

// ── Title match scoring (0–100) ───────────────────────────────────────────────

func normalise(s string) string {
	s = strings.ToLower(s)
	s = reNonAlNum.ReplaceAllString(s, " ")
	s = reMultiSpace.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// stripReleaseMeta removes the scene/group suffix from a release title so that
// "Game.Title.v1.0-GROUPNAME" reduces to "game title".
func stripReleaseMeta(title string) string {
	// Remove everything after a year token
	if loc := reYear.FindStringIndex(title); loc != nil {
		title = title[:loc[0]]
	}
	// Remove everything after known source/group patterns
	upper := strings.ToUpper(title)
	for _, s := range sources {
		for _, p := range s.patterns {
			if idx := strings.Index(upper, p); idx != -1 {
				title = title[:idx]
				upper = upper[:idx]
			}
		}
	}
	// Remove everything after a version tag
	if m := reVersion.FindStringIndex(title); m != nil {
		title = title[:m[0]]
	}
	return normalise(title)
}

func titleMatch(releaseTitle, gameTitle string) int {
	normRelease := stripReleaseMeta(releaseTitle)
	normGame := normalise(gameTitle)

	if normGame == "" || normRelease == "" {
		return 0
	}

	// Check if game title is a substring of the cleaned release title
	if strings.Contains(normRelease, normGame) {
		return 100
	}

	// Word overlap ratio
	gameWords := strings.Fields(normGame)
	matched := 0
	for _, w := range gameWords {
		if len(w) < 2 {
			continue // skip single chars / articles
		}
		if strings.Contains(normRelease, w) {
			matched++
		}
	}
	total := len(gameWords)
	if total == 0 {
		return 0
	}
	score := (matched * 100) / total

	// Bonus: release title starts with game title
	if strings.HasPrefix(normRelease, normGame) {
		score = min(score+15, 100)
	}
	return score
}

// ── Component → composite score ───────────────────────────────────────────────

func sourceScore(tier int) int {
	switch tier {
	case 1:
		return 40
	case 2:
		return 30
	case 3:
		return 20
	default:
		return 0
	}
}

func seederScore(seeders int, protocol string) int {
	if protocol == "nzb" {
		return 20
	}
	switch {
	case seeders == 0:
		return 0
	case seeders < 5:
		return 5
	case seeders < 20:
		return 10
	case seeders < 100:
		return 15
	default:
		return 20
	}
}

// ── Public API ────────────────────────────────────────────────────────────────

// ScoreResult enriches a single SearchResult in place.
func ScoreResult(r *indexer.SearchResult, gameTitle string) {
	// Source
	r.SourceName, r.SourceTier, r.InstallMethod = detectSource(r.Title)

	// Metadata
	r.DetectedVersion = extractVersion(r.Title)
	r.DetectedLangs = extractLanguages(r.Title)
	r.ReleaseType = detectReleaseType(r.Title)
	r.SizeWarning = sizeWarning(r.Size, r.ReleaseType, r.InstallMethod)

	// Component scores
	ts := titleMatch(r.Title, gameTitle)
	r.TitleScore = ts

	rep := sourceScore(r.SourceTier)
	seed := seederScore(r.Seeders, r.Protocol)
	sizeOK := 10
	if r.SizeWarning != "" {
		sizeOK = 0
	}

	// Weighted composite: title match carries 30%, source rep 40 pts, seeders 20, size 10
	raw := int(float64(ts)*0.30) + rep + seed + sizeOK
	if raw < 0 {
		raw = 0
	}
	if raw > 100 {
		raw = 100
	}
	r.Score = raw
	r.ScorePct = raw
}

// ScoreAll enriches a slice of results in place, then re-sorts by ScorePct descending.
func ScoreAll(results []indexer.SearchResult, gameTitle string) {
	for i := range results {
		ScoreResult(&results[i], gameTitle)
	}
	sort.SliceStable(results, func(i, j int) bool {
		return results[i].ScorePct > results[j].ScorePct
	})
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
