package monitor

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// DownloadLinkStore maps torrent infohash → library game ID so the
// post-processor can associate a completed download with the game the
// user initiated it from, rather than relying on fuzzy name matching.
type DownloadLinkStore struct {
	mu    sync.RWMutex
	links map[string]int // infohash (lowercase hex) → game ID
	path  string
}

func NewDownloadLinkStore(configDir string) *DownloadLinkStore {
	s := &DownloadLinkStore{
		links: map[string]int{},
		path:  filepath.Join(configDir, "download_links.json"),
	}
	s.load()
	return s
}

func (s *DownloadLinkStore) Add(infoHash string, gameID int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.links[strings.ToLower(infoHash)] = gameID
	s.save()
}

// Get returns the game ID for the given infohash and removes the entry.
func (s *DownloadLinkStore) Get(infoHash string) (int, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.links[strings.ToLower(infoHash)]
	if ok {
		delete(s.links, strings.ToLower(infoHash))
		s.save()
	}
	return id, ok
}

func (s *DownloadLinkStore) save() {
	data, _ := json.MarshalIndent(s.links, "", "  ")
	if err := os.WriteFile(s.path, data, 0644); err != nil {
		log.Printf("[DownloadLinks] Failed to save: %v", err)
	}
}

func (s *DownloadLinkStore) load() {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, &s.links)
}

// ExtractMagnetInfoHash parses the infohash from a magnet URI.
// Supports both btih (hex/base32) and btmh formats.
func ExtractMagnetInfoHash(magnetURL string) string {
	const btih = "xt=urn:btih:"
	const btmh = "xt=urn:btmh:"
	for _, prefix := range []string{btih, btmh} {
		idx := strings.Index(magnetURL, prefix)
		if idx == -1 {
			continue
		}
		rest := magnetURL[idx+len(prefix):]
		end := strings.IndexAny(rest, "&# ")
		if end != -1 {
			rest = rest[:end]
		}
		return strings.ToLower(rest)
	}
	return ""
}

// ExtractTorrentInfoHash computes the SHA-1 infohash of a .torrent file's
// info dictionary by locating the bencoded "info" key and hashing its value.
func ExtractTorrentInfoHash(data []byte) string {
	// Find the "4:info" key in the bencode stream.
	needle := []byte("4:info")
	idx := strings.Index(string(data), "4:info")
	if idx == -1 {
		return ""
	}
	start := idx + len(needle)
	end := bencodeEnd(data, start)
	if end == -1 || end > len(data) {
		return ""
	}
	h := sha1.Sum(data[start:end])
	return hex.EncodeToString(h[:])
}

// bencodeEnd returns the index after the end of the bencode value starting at data[pos].
func bencodeEnd(data []byte, pos int) int {
	if pos >= len(data) {
		return -1
	}
	switch data[pos] {
	case 'i': // integer: i<n>e
		end := strings.IndexByte(string(data[pos:]), 'e')
		if end == -1 {
			return -1
		}
		return pos + end + 1
	case 'l', 'd': // list or dict: l...e / d...e
		pos++
		for pos < len(data) {
			if data[pos] == 'e' {
				return pos + 1
			}
			next := bencodeEnd(data, pos)
			if next == -1 {
				return -1
			}
			pos = next
		}
		return -1
	default: // string: <len>:<bytes>
		colon := strings.IndexByte(string(data[pos:]), ':')
		if colon == -1 {
			return -1
		}
		n, err := strconv.Atoi(string(data[pos : pos+colon]))
		if err != nil || n < 0 {
			return -1
		}
		end := pos + colon + 1 + n
		if end > len(data) {
			return -1
		}
		return end
	}
}
