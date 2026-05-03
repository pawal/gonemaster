package main

import (
	"fmt"
	"maps"
	"sort"
	"strings"
	"sync"

	"codeberg.org/pawal/gonemaster/engine/logger"
)

var countLevelOrder = []string{"CRITICAL", "ERROR", "WARNING", "NOTICE", "INFO", "DEBUG", "DEBUG2", "DEBUG3"}

type countReporter struct {
	mu         sync.Mutex
	levelCount map[string]int
	tagCount   map[string]map[string]int
}

func newCountReporter() *countReporter {
	return &countReporter{
		levelCount: map[string]int{},
		tagCount:   map[string]map[string]int{},
	}
}

// Callback records counts for each emitted log entry by level and message tag.
func (r *countReporter) Callback(entry *logger.Entry) error {
	if r == nil || entry == nil {
		return nil
	}
	level := strings.ToUpper(strings.TrimSpace(entry.Level()))
	if level == "" {
		level = "UNKNOWN"
	}
	tag := strings.ToUpper(strings.TrimSpace(entry.Tag))
	if tag == "" {
		tag = "UNKNOWN"
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.levelCount[level]++
	levelTags := r.tagCount[level]
	if levelTags == nil {
		levelTags = map[string]int{}
		r.tagCount[level] = levelTags
	}
	levelTags[tag]++
	return nil
}

func (r *countReporter) SummaryLines() []string {
	if r == nil {
		return nil
	}
	levelCount, tagCount := r.snapshot()
	levels := orderedLevels(levelCount)

	numberHeader := "Number of log entries"
	numberWidth := len(numberHeader)
	lines := []string{
		"",
		"",
		fmt.Sprintf("%7s\t%*s", "Level", numberWidth, numberHeader),
		fmt.Sprintf("=======\t%s", strings.Repeat("=", numberWidth)),
	}
	for _, level := range levels {
		lines = append(lines, fmt.Sprintf("%7s\t%*d", level, numberWidth, levelCount[level]))
	}

	tagHeader := "Message tag"
	tagWidth := len(tagHeader)
	for _, level := range levels {
		for tag := range tagCount[level] {
			if len(tag) > tagWidth {
				tagWidth = len(tag)
			}
		}
	}

	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("%7s\t%-*s\t%5s", "Level", tagWidth, tagHeader, "Count"))
	lines = append(lines, fmt.Sprintf("=======\t%s\t=====", strings.Repeat("=", tagWidth)))
	for _, level := range levels {
		levelTags := tagCount[level]
		tags := make([]string, 0, len(levelTags))
		for tag := range levelTags {
			tags = append(tags, tag)
		}
		sort.Strings(tags)
		for _, tag := range tags {
			lines = append(lines, fmt.Sprintf("%7s\t%-*s\t%5d", level, tagWidth, tag, levelTags[tag]))
		}
	}
	return lines
}

func (r *countReporter) snapshot() (map[string]int, map[string]map[string]int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	levelCount := make(map[string]int, len(r.levelCount))
	maps.Copy(levelCount, r.levelCount)

	tagCount := make(map[string]map[string]int, len(r.tagCount))
	for level, levelTags := range r.tagCount {
		cloned := make(map[string]int, len(levelTags))
		maps.Copy(cloned, levelTags)
		tagCount[level] = cloned
	}
	return levelCount, tagCount
}

func orderedLevels(levelCount map[string]int) []string {
	known := make(map[string]struct{}, len(countLevelOrder))
	out := make([]string, 0, len(levelCount))
	for _, level := range countLevelOrder {
		known[level] = struct{}{}
		if count, ok := levelCount[level]; ok && count > 0 {
			out = append(out, level)
		}
	}
	extra := make([]string, 0)
	for level, count := range levelCount {
		if count <= 0 {
			continue
		}
		if _, ok := known[level]; ok {
			continue
		}
		extra = append(extra, level)
	}
	sort.Strings(extra)
	return append(out, extra...)
}

func composeCallbacks(callbacks ...func(*logger.Entry) error) func(*logger.Entry) error {
	valid := make([]func(*logger.Entry) error, 0, len(callbacks))
	for _, cb := range callbacks {
		if cb != nil {
			valid = append(valid, cb)
		}
	}
	if len(valid) == 0 {
		return nil
	}
	if len(valid) == 1 {
		return valid[0]
	}
	return func(entry *logger.Entry) error {
		for _, cb := range valid {
			if err := cb(entry); err != nil {
				return err
			}
		}
		return nil
	}
}
