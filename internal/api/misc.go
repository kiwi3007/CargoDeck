package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/kiwi3007/playerr/internal/indexer"
	"github.com/kiwi3007/playerr/internal/scorer"
)

// Search handles GET /api/v3/search?query=&categories=
// Fans out to Prowlarr, Jackett, and Hydra indexers in parallel.
func (h *Handler) Search(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("query")
	if query == "" {
		jsonErr(w, 400, "query parameter is required")
		return
	}

	prowlarrCfg := h.cfg.LoadProwlarr()
	jackettCfg := h.cfg.LoadJackett()
	hydraSources := h.cfg.LoadHydra()

	prowlarrConfigured := prowlarrCfg.Url != "" && prowlarrCfg.ApiKey != ""
	jackettConfigured := jackettCfg.Url != "" && jackettCfg.ApiKey != ""
	hydraEnabled := len(hydraSources) > 0

	if !prowlarrConfigured && !jackettConfigured && !hydraEnabled {
		jsonOK(w, []indexer.SearchResult{})
		return
	}

	// Parse optional category filter
	var categories []int
	if cats := r.URL.Query().Get("categories"); cats != "" {
		for _, part := range splitComma(cats) {
			var id int
			if _, err := parseInt(part, &id); err == nil {
				categories = append(categories, id)
			}
		}
	}

	type fanResult struct {
		results []indexer.SearchResult
		err     error
	}

	ctx := r.Context()
	var wg sync.WaitGroup
	ch := make(chan fanResult, 10)

	if prowlarrConfigured {
		wg.Add(1)
		go func() {
			defer wg.Done()
			client := indexer.NewProwlarrClient(prowlarrCfg.Url, prowlarrCfg.ApiKey)
			res, err := client.Search(ctx, query, categories)
			if err != nil {
				log.Printf("[Search] Prowlarr error: %v", err)
				ch <- fanResult{err: err}
				return
			}
			ch <- fanResult{results: res}
		}()
	}

	if jackettConfigured {
		wg.Add(1)
		go func() {
			defer wg.Done()
			client := indexer.NewJackettClient(jackettCfg.Url, jackettCfg.ApiKey)
			res, err := client.Search(ctx, query, categories)
			if err != nil {
				log.Printf("[Search] Jackett error: %v", err)
				ch <- fanResult{err: err}
				return
			}
			ch <- fanResult{results: res}
		}()
	}

	for _, src := range hydraSources {
		if !src.Enable {
			continue
		}
		src := src // capture
		wg.Add(1)
		go func() {
			defer wg.Done()
			client := indexer.NewHydraClient(src.Name, src.Url)
			res, err := client.Search(ctx, query)
			if err != nil {
				log.Printf("[Search] Hydra [%s] error: %v", src.Name, err)
				ch <- fanResult{err: err}
				return
			}
			ch <- fanResult{results: res}
		}()
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	// Stream results as NDJSON: each line is a JSON array of scored results
	// from one indexer batch, flushed immediately so the browser can render
	// results as they arrive rather than waiting for all indexers to finish.
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	flusher, canFlush := w.(http.Flusher)

	// Dedup key: same release cross-posted across indexers is kept once.
	type key struct {
		title   string
		size    int64
		indexer string
	}
	seen := map[key]struct{}{}
	total := 0

	for fr := range ch {
		if fr.err != nil {
			continue
		}

		// Deduplicate against already-flushed results.
		batch := fr.results[:0:0] // new slice, same backing array avoided
		for _, res := range fr.results {
			k := key{res.Title, res.Size, res.IndexerName}
			if _, ok := seen[k]; ok {
				continue
			}
			seen[k] = struct{}{}
			batch = append(batch, res)
		}

		// Category filter.
		if len(categories) > 0 {
			filtered := batch[:0]
			for _, res := range batch {
				if matchesAnyCategory(res.Categories, categories) {
					filtered = append(filtered, res)
				}
			}
			batch = filtered
		}

		if len(batch) == 0 {
			continue
		}

		scorer.ScoreAll(batch, query)
		total += len(batch)

		line, err := json.Marshal(batch)
		if err != nil {
			continue
		}
		fmt.Fprintf(w, "%s\n", line)
		if canFlush {
			flusher.Flush()
		}
	}

	log.Printf("[Search] Streamed %d unique results for %q", total, query)
}

// SearchTest handles POST /api/v3/search/test — tests Prowlarr or Jackett connection.
func (h *Handler) SearchTest(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Type   string `json:"type"` // "prowlarr" or "jackett"
		URL    string `json:"url"`
		ApiKey string `json:"apiKey"`
	}
	if err := decodeBody(r, &req); err != nil {
		jsonErr(w, 400, err.Error())
		return
	}

	var connected bool
	if req.Type == "jackett" {
		client := indexer.NewJackettClient(req.URL, req.ApiKey)
		connected = client.TestConnection()
	} else {
		client := indexer.NewProwlarrClient(req.URL, req.ApiKey)
		connected = client.TestConnection()
	}

	msg := "Connection successful"
	if !connected {
		msg = "Failed to connect. Check URL and API Key."
	}
	jsonOK(w, map[string]any{"connected": connected, "message": msg})
}

// FilesystemList lists files and folders at the given path, including a ".." parent entry.
// Returns [{ name, path, type }] where type is "directory", "drive", or "file".
func (h *Handler) FilesystemList(w http.ResponseWriter, r *http.Request) {
	root := r.URL.Query().Get("path")
	if root == "" {
		root = "/"
	}

	type fsEntry struct {
		Name string `json:"name"`
		Path string `json:"path"`
		Type string `json:"type"`
	}

	var result []fsEntry

	// Add parent entry unless already at root
	parent := filepath.Dir(root)
	if parent != root {
		result = append(result, fsEntry{Name: "..", Path: parent, Type: "directory"})
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		jsonErr(w, 400, err.Error())
		return
	}

	for _, e := range entries {
		t := "file"
		if e.IsDir() {
			t = "directory"
		}
		result = append(result, fsEntry{
			Name: e.Name(),
			Path: filepath.Join(root, e.Name()),
			Type: t,
		})
	}

	if result == nil {
		result = []fsEntry{}
	}
	jsonOK(w, result)
}

// Explore lists folders in a directory.
func (h *Handler) Explore(w http.ResponseWriter, r *http.Request) {
	root := r.URL.Query().Get("path")
	if root == "" {
		home, _ := os.UserHomeDir()
		root = home
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		jsonErr(w, 400, err.Error())
		return
	}

	type dirEntry struct {
		Name  string `json:"name"`
		Path  string `json:"path"`
		IsDir bool   `json:"isDir"`
	}

	var result []dirEntry
	for _, e := range entries {
		p := filepath.Join(root, e.Name())
		result = append(result, dirEntry{
			Name:  e.Name(),
			Path:  p,
			IsDir: e.IsDir(),
		})
	}
	if result == nil {
		result = []dirEntry{}
	}
	jsonOK(w, result)
}

// ListFolder lists sub-folders for the filesystem browser.
func (h *Handler) ListFolder(w http.ResponseWriter, r *http.Request) {
	root := r.URL.Query().Get("path")
	if root == "" {
		home, _ := os.UserHomeDir()
		root = home
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		jsonErr(w, 400, err.Error())
		return
	}

	type entry struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}

	var folders []entry
	for _, e := range entries {
		if e.IsDir() {
			p := filepath.Join(root, e.Name())
			folders = append(folders, entry{Name: e.Name(), Path: p})
		}
	}
	if folders == nil {
		folders = []entry{}
	}
	jsonOK(w, folders)
}

// ---- helpers ----

func splitComma(s string) []string {
	var parts []string
	for _, p := range filepath.SplitList(s) {
		p = trimSpace(p)
		if p != "" {
			parts = append(parts, p)
		}
	}
	if len(parts) == 0 {
		// filepath.SplitList uses OS separator; fall back to comma split
		for _, p := range splitByte(s, ',') {
			p = trimSpace(p)
			if p != "" {
				parts = append(parts, p)
			}
		}
	}
	return parts
}

func splitByte(s string, sep byte) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == sep {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}

func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}

// matchesAnyCategory returns true if any result category shares the same Newznab
// parent (thousands digit) as any of the requested category IDs.
// e.g. requesting 4000 matches result categories 4010, 4050, etc., and vice versa.
//
// Extended Newznab categories (ID >= 100000) are normalised by stripping the
// leading 100000 prefix before parent-matching:
//   104050 → 4050 → parent 4000 (PC)
//   101035 → 1035 → parent 1000 (Console)
// Sub-1000 remainders (e.g. TPB: 100400 → 400) cannot be mapped to a standard
// parent, so they are passed through rather than filtered out.
func matchesAnyCategory(resultCats []indexer.Category, requested []int) bool {
	if len(resultCats) == 0 {
		return true // no category info — don't filter out
	}
	// Treat all-zero IDs (dummy/unset) the same as no category info
	hasRealID := false
	for _, rc := range resultCats {
		if rc.ID != 0 {
			hasRealID = true
			break
		}
	}
	if !hasRealID {
		return true
	}
	for _, req := range requested {
		reqParent := (req / 1000) * 1000
		for _, rc := range resultCats {
			id := rc.ID
			if id >= 100000 {
				id = id % 100000
				if id < 1000 {
					return true // TPB-style sub-1000 ID — can't determine standard parent, allow through
				}
			}
			rcParent := (id / 1000) * 1000
			if reqParent == rcParent {
				return true
			}
		}
	}
	return false
}

func parseInt(s string, out *int) (int, error) {
	n := 0
	neg := false
	i := 0
	if i < len(s) && s[i] == '-' {
		neg = true
		i++
	}
	for ; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			*out = n
			return n, nil
		}
		n = n*10 + int(s[i]-'0')
	}
	if neg {
		n = -n
	}
	*out = n
	return n, nil
}
