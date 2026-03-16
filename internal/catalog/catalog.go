// Package catalog provides types and lookup functions for the appimage.github.io feed.
package catalog

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/sivel/appimg/internal/cache"
)

// Link is an external URL associated with a catalog entry (e.g. a GitHub repository).
type Link struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

// Author is a project author listed in a catalog entry.
type Author struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// Entry is a single application record from the appimage.github.io catalog.
type Entry struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Categories  []string `json:"categories"`
	Authors     []Author `json:"authors"`
	License     string   `json:"license"`
	Links       []Link   `json:"links"`
	Icons       []string `json:"icons"`
	Screenshots []string `json:"screenshots"`
}

// GitHubRepo returns the "owner/repo" string from the entry's GitHub link,
// and reports whether one was found.
func (e *Entry) GitHubRepo() (string, bool) {
	for _, l := range e.Links {
		if strings.EqualFold(l.Type, "GitHub") {
			return l.URL, true
		}
	}
	return "", false
}

// Feed is the parsed top-level structure of the appimage.github.io feed.json.
type Feed struct {
	Items []Entry `json:"items"`
}

// Load reads and parses the cached feed.json from disk.
func Load() (*Feed, error) {
	path, err := cache.Path()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read feed cache: %w", err)
	}
	var feed Feed
	if err := json.Unmarshal(data, &feed); err != nil {
		return nil, fmt.Errorf("parse feed: %w", err)
	}
	return &feed, nil
}

// Find returns the entry with the given name (case-insensitive).
func Find(feed *Feed, name string) (*Entry, bool) {
	for i := range feed.Items {
		if strings.EqualFold(feed.Items[i].Name, name) {
			return &feed.Items[i], true
		}
	}
	return nil, false
}

// Filter returns all entries whose name or description match the given regex pattern.
// An empty pattern matches all entries.
func Filter(feed *Feed, pattern string) ([]Entry, error) {
	if pattern == "" {
		return feed.Items, nil
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid filter pattern: %w", err)
	}
	var results []Entry
	for _, e := range feed.Items {
		if re.MatchString(e.Name) || re.MatchString(e.Description) {
			results = append(results, e)
		}
	}
	return results, nil
}
