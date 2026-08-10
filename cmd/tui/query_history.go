package tui

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/sahilm/fuzzy"
)

const maxQueryHistoryEntries = 200

type queryHistoryEntry struct {
	Query     string    `json:"query"`
	Timestamp time.Time `json:"timestamp"`
}

type historySearchState struct {
	originalQuery string
	searchTerm    string
	matches       []int
	selected      int
}

type queryHistoryManager struct {
	path            string
	entries         []queryHistoryEntry
	browseIndex     int
	browseDraft     string
	browseDraftSet  bool
	hadInvalidLines bool
	search          *historySearchState
}

func loadQueryHistory(path string) (*queryHistoryManager, error) {
	manager := &queryHistoryManager{
		path:        path,
		browseIndex: -1,
	}

	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return manager, nil
		}
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var entry queryHistoryEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			manager.hadInvalidLines = true
			continue
		}

		entry.Query = strings.TrimSpace(entry.Query)
		if entry.Query == "" {
			manager.hadInvalidLines = true
			continue
		}
		manager.entries = append(manager.entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if manager.hadInvalidLines {
		if err := manager.persist(); err != nil {
			return nil, err
		}
		manager.hadInvalidLines = false
	}

	return manager, nil
}

func (h *queryHistoryManager) remember(query string) error {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}

	if len(h.entries) > 0 && h.entries[len(h.entries)-1].Query == query {
		h.resetNavigation()
		h.cancelSearch()
		return nil
	}

	h.entries = append(h.entries, queryHistoryEntry{
		Query:     query,
		Timestamp: time.Now().UTC(),
	})
	if len(h.entries) > maxQueryHistoryEntries {
		h.entries = append([]queryHistoryEntry(nil), h.entries[len(h.entries)-maxQueryHistoryEntries:]...)
	}

	h.resetNavigation()
	h.cancelSearch()
	return h.persist()
}

func (h *queryHistoryManager) previous(current string) string {
	if len(h.entries) == 0 {
		return current
	}
	if h.browseIndex == -1 {
		if !h.browseDraftSet {
			h.browseDraft = current
			h.browseDraftSet = true
		}
		h.browseIndex = len(h.entries) - 1
		return h.entries[h.browseIndex].Query
	}

	if h.browseIndex > 0 {
		h.browseIndex--
	}
	return h.entries[h.browseIndex].Query
}

func (h *queryHistoryManager) next(current string) string {
	if len(h.entries) == 0 {
		return current
	}
	if h.browseIndex == -1 {
		if h.browseDraft == "" {
			h.browseDraft = current
		}
		return current
	}

	if h.browseIndex < len(h.entries)-1 {
		h.browseIndex++
		return h.entries[h.browseIndex].Query
	}

	h.browseIndex = -1
	if !h.browseDraftSet {
		return current
	}
	return h.browseDraft
}

func (h *queryHistoryManager) resetNavigation() {
	h.browseIndex = -1
	h.browseDraft = ""
	h.browseDraftSet = false
}

func (h *queryHistoryManager) beginSearch(current string) string {
	if h.search == nil {
		h.search = &historySearchState{
			originalQuery: current,
		}
		h.refreshSearchMatches()
		return current
	}

	if len(h.search.matches) > 1 {
		h.search.selected = (h.search.selected + 1) % len(h.search.matches)
		return h.searchValue()
	}
	return current
}

func (h *queryHistoryManager) searchActive() bool {
	return h.search != nil
}

func (h *queryHistoryManager) browsing() bool {
	return h.browseIndex != -1
}

func (h *queryHistoryManager) setDraft(current string) {
	h.browseDraft = current
	h.browseDraftSet = true
}

func (h *queryHistoryManager) searchTerm() string {
	if h.search == nil {
		return ""
	}
	return h.search.searchTerm
}

func (h *queryHistoryManager) searchStatus() string {
	if h.search == nil {
		return ""
	}
	if len(h.search.matches) == 0 {
		if h.search.searchTerm == "" {
			return "History search: type to fuzzy search previous queries"
		}
		return `History search: no matches for "` + h.search.searchTerm + `"`
	}
	return "History search (" + itoa(h.search.selected+1) + "/" + itoa(len(h.search.matches)) + "): " + h.search.searchTerm
}

func (h *queryHistoryManager) updateSearchTerm(term string) string {
	if h.search == nil {
		return ""
	}
	h.search.searchTerm = term
	h.refreshSearchMatches()
	return h.searchValue()
}

func (h *queryHistoryManager) searchOlder() string {
	if h.search == nil || len(h.search.matches) == 0 {
		return h.searchValue()
	}
	h.search.selected--
	if h.search.selected < 0 {
		h.search.selected = len(h.search.matches) - 1
	}
	return h.searchValue()
}

func (h *queryHistoryManager) searchNewer() string {
	if h.search == nil || len(h.search.matches) == 0 {
		return h.searchValue()
	}
	h.search.selected = (h.search.selected + 1) % len(h.search.matches)
	return h.searchValue()
}

func (h *queryHistoryManager) acceptSearch() string {
	value := h.searchValue()
	h.search = nil
	h.resetNavigation()
	return value
}

func (h *queryHistoryManager) cancelSearch() string {
	if h.search == nil {
		return ""
	}
	value := h.search.originalQuery
	h.search = nil
	return value
}

func (h *queryHistoryManager) searchValue() string {
	if h.search == nil {
		return ""
	}
	if len(h.search.matches) == 0 {
		return h.search.originalQuery
	}
	index := h.search.matches[h.search.selected]
	if index < 0 || index >= len(h.entries) {
		return h.search.originalQuery
	}
	return h.entries[index].Query
}

func (h *queryHistoryManager) refreshSearchMatches() {
	if h.search == nil {
		return
	}

	h.search.matches = h.search.matches[:0]
	h.search.selected = 0
	if len(h.entries) == 0 {
		return
	}

	if strings.TrimSpace(h.search.searchTerm) == "" {
		for i := len(h.entries) - 1; i >= 0; i-- {
			h.search.matches = append(h.search.matches, i)
		}
		return
	}

	candidates := make([]string, 0, len(h.entries))
	indices := make([]int, 0, len(h.entries))
	for i := len(h.entries) - 1; i >= 0; i-- {
		candidates = append(candidates, h.entries[i].Query)
		indices = append(indices, i)
	}

	matches := fuzzy.Find(h.search.searchTerm, candidates)
	for _, match := range matches {
		if match.Index >= 0 && match.Index < len(indices) {
			h.search.matches = append(h.search.matches, indices[match.Index])
		}
	}
}

func (h *queryHistoryManager) persist() error {
	if err := os.MkdirAll(filepath.Dir(h.path), 0o755); err != nil {
		return err
	}

	file, err := os.Create(h.path)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	for _, entry := range h.entries {
		if err := encoder.Encode(entry); err != nil {
			return err
		}
	}
	return nil
}

func itoa(v int) string {
	return strconv.Itoa(v)
}
