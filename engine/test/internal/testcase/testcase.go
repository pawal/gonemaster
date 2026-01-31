package testcase

import (
	"context"
	"sort"

	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/util"
)

// Case runs a single test case and returns its log entries.
type Case func(context.Context) ([]*logger.Entry, error)

// Run executes a test case with an isolated logger and flushes entries to the parent logger.
func Run(ctx context.Context, fn Case) ([]*logger.Entry, error) {
	if fn == nil {
		return nil, nil
	}

	parent := util.Logger()
	buf := logger.New()
	useSilentAppend := parent != nil && parent.Callback != nil
	if useSilentAppend {
		buf.Callback = parent.Callback
	}

	util.SetLogger(buf)
	entries, err := fn(ctx)
	util.SetLogger(parent)

	bufEntries := buf.Entries()
	var extras []*logger.Entry
	if entries == nil {
		entries = bufEntries
	} else if len(bufEntries) > 0 {
		seen := map[*logger.Entry]bool{}
		for _, entry := range entries {
			if entry == nil {
				continue
			}
			seen[entry] = true
		}
		for _, entry := range bufEntries {
			if entry == nil {
				continue
			}
			if !seen[entry] {
				extras = append(extras, entry)
			}
		}
		if len(extras) > 0 {
			sort.SliceStable(extras, func(i, j int) bool {
				left := extras[i]
				right := extras[j]
				if left == nil {
					return false
				}
				if right == nil {
					return true
				}
				return left.Timestamp < right.Timestamp
			})

			merged := make([]*logger.Entry, 0, len(entries)+len(extras))
			extraIdx := 0
			for _, entry := range entries {
				for extraIdx < len(extras) {
					extra := extras[extraIdx]
					if extra == nil {
						extraIdx++
						continue
					}
					if entry != nil && extra.Timestamp < entry.Timestamp {
						merged = append(merged, extra)
						extraIdx++
						continue
					}
					break
				}
				merged = append(merged, entry)
			}
			for extraIdx < len(extras) {
				merged = append(merged, extras[extraIdx])
				extraIdx++
			}
			entries = merged
		}
	}
	if parent != nil && len(entries) > 0 {
		if !useSilentAppend {
			_ = parent.Append(entries...)
		} else if len(bufEntries) == 0 {
			_ = parent.Append(entries...)
		} else {
			bufSeen := map[*logger.Entry]bool{}
			for _, entry := range bufEntries {
				if entry == nil {
					continue
				}
				bufSeen[entry] = true
			}
			for _, entry := range entries {
				if entry == nil {
					continue
				}
				if bufSeen[entry] {
					_ = parent.AppendWithoutCallback(entry)
				} else {
					_ = parent.Append(entry)
				}
			}
		}
	}

	return entries, err
}
