// Package cnamelog turns recursor *CNAMEError values into testcase log entries.
package cnamelog

import (
	"context"
	"errors"

	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/recursor"
	"codeberg.org/pawal/gonemaster/engine/util"
)

// Log emits the matching CNAME tag for err. Returns true if err was a *CNAMEError.
func Log(ctx context.Context, results *[]*logger.Entry, module string, testcase string, err error) bool {
	if err == nil {
		return false
	}
	var ce *recursor.CNAMEError
	if !errors.As(err, &ce) {
		return false
	}
	tag, args := tagFor(ce)
	entry, addErr := util.LoggerFromContext(ctx).Add(tag, args, module, testcase)
	if addErr != nil || entry == nil {
		return true
	}
	*results = append(*results, entry)
	return true
}

func tagFor(ce *recursor.CNAMEError) (string, map[string]any) {
	switch ce.Reason {
	case recursor.CNAMETooMany:
		return "CNAME_TOO_MANY_RECORDS", map[string]any{"query_name": ce.Name}
	case recursor.CNAMEChainTooLong:
		return "CNAME_CHAIN_TOO_LONG", map[string]any{"query_name": ce.Name}
	}
	return "CNAME_TARGET_UNRESOLVED", map[string]any{"query_name": ce.Name, "cname_target": ce.Target}
}
