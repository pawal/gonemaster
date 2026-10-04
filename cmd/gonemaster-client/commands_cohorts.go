package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"codeberg.org/pawal/gonemaster/cmd/internal/cliterm"
)

// adminCohort is one row of GET /analysis/cohorts.
type adminCohort struct {
	ID        int64  `json:"id"`
	SourceTag string `json:"source_tag"`
	Label     string `json:"label"`
}

// cohortSchedule is a cohort snapshot schedule; it is also the PUT body.
type cohortSchedule struct {
	CohortID       int64    `json:"cohort_id,omitempty"`
	SourceTag      string   `json:"source_tag,omitempty"`
	Label          string   `json:"label,omitempty"`
	Enabled        bool     `json:"enabled"`
	Kind           string   `json:"kind"`
	IntervalDays   int      `json:"interval_days"`
	AnchorDate     string   `json:"anchor_date"`
	Weekdays       []string `json:"weekdays"`
	DaysOfMonth    []int    `json:"days_of_month"`
	LastDay        bool     `json:"last_day"`
	TimeOfDay      string   `json:"time_of_day"`
	Timezone       string   `json:"timezone"`
	ProfileID      *int64   `json:"profile_id"`
	PromoteDefault bool     `json:"promote_default"`
	CatchUp        bool     `json:"catch_up"`
	Summary        string   `json:"summary,omitempty"`
	NextRunAt      string   `json:"next_run_at,omitempty"`
	LastRunAt      string   `json:"last_run_at,omitempty"`
	LastBatchID    string   `json:"last_batch_id,omitempty"`
	LastOutcome    string   `json:"last_outcome,omitempty"`
	LastError      string   `json:"last_error,omitempty"`
}

// putBody keeps only the fields PUT accepts.
func (s cohortSchedule) putBody() cohortSchedule {
	return cohortSchedule{
		Enabled: s.Enabled, Kind: s.Kind, IntervalDays: s.IntervalDays, AnchorDate: s.AnchorDate,
		Weekdays: s.Weekdays, DaysOfMonth: s.DaysOfMonth, LastDay: s.LastDay, TimeOfDay: s.TimeOfDay,
		Timezone: s.Timezone, ProfileID: s.ProfileID, PromoteDefault: s.PromoteDefault, CatchUp: s.CatchUp,
	}
}

func runCohorts(ctx context.Context, client *apiClient, opts globalOptions, args []string, out io.Writer, errOut io.Writer) int {
	const subs = "list|snapshots|schedules|schedule"
	if len(args) == 0 {
		fmt.Fprintln(errOut, "cohorts subcommand is required: "+subs)
		return 2
	}
	if isHelpArg(args[0]) {
		return groupUsage(out, "cohorts", subs)
	}
	cmd := args[0]
	args = args[1:]
	switch cmd {
	case "list":
		return runCohortsList(ctx, client, opts, args, out, errOut)
	case "snapshots":
		return runCohortsSnapshots(ctx, client, opts, args, out, errOut)
	case "schedules":
		return runCohortsSchedules(ctx, client, opts, args, out, errOut)
	case "schedule":
		return runCohortsSchedule(ctx, client, opts, args, out, errOut)
	default:
		fmt.Fprintf(errOut, "Unknown cohorts command %q\n", cmd)
		return 2
	}
}

// runCohortsSchedules lists every cohort schedule.
func runCohortsSchedules(ctx context.Context, client *apiClient, opts globalOptions, args []string, out io.Writer, errOut io.Writer) int {
	fs := flag.NewFlagSet("cohorts schedules", flag.ContinueOnError)
	fs.SetOutput(errOut)
	if err := parseWithReorderedFlags(fs, args); err != nil {
		return parseExit(err)
	}
	var list []cohortSchedule
	if err := client.doJSON(ctx, "GET", "/analysis/schedules", nil, &list); err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	if isJSONFormat(opts.format) {
		return writeOrFail(out, errOut, opts.format, list)
	}
	fmt.Fprintf(out, "Schedules: %d\n", len(list))
	for _, s := range list {
		state := "enabled"
		if !s.Enabled {
			state = "paused"
		}
		fmt.Fprintf(out, "  %-24s  %-8s  %s  next=%s%s\n",
			cliterm.Sanitize(s.SourceTag), state, cliterm.Sanitize(s.Summary), s.NextRunAt, lastOutcome(s))
	}
	return 0
}

// runCohortsSchedule shows, sets or removes one cohort's schedule.
func runCohortsSchedule(ctx context.Context, client *apiClient, opts globalOptions, args []string, out io.Writer, errOut io.Writer) int {
	const usage = "Usage: gonemaster-client cohorts schedule <tag> | set <tag> [flags] | remove <tag>"
	if len(args) == 0 {
		fmt.Fprintln(errOut, usage)
		return 2
	}
	if isHelpArg(args[0]) {
		fmt.Fprintln(out, usage)
		return 0
	}
	switch args[0] {
	case "set":
		return runCohortsScheduleSet(ctx, client, opts, args[1:], out, errOut)
	case "remove":
		return runCohortsScheduleRemove(ctx, client, args[1:], out, errOut)
	}
	cohort, err := resolveAdminCohort(ctx, client, args[0])
	if err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	var sched cohortSchedule
	if err := client.doJSON(ctx, "GET", schedulePath(cohort.ID), nil, &sched); err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	return printSchedule(out, errOut, opts, cohort, sched)
}

func runCohortsScheduleSet(ctx context.Context, client *apiClient, opts globalOptions, args []string, out io.Writer, errOut io.Writer) int {
	fs := flag.NewFlagSet("cohorts schedule set", flag.ContinueOnError)
	fs.SetOutput(errOut)
	monthly := fs.String("monthly", "", "Monthly on these days, 1 to 28 and last, e.g. 1,15")
	weekly := fs.String("weekly", "", "Weekly on these days, e.g. mon,thu")
	every := fs.Int("every", 0, "Every N days")
	from := fs.String("from", "", "First date of an --every rule, YYYY-MM-DD (default today)")
	at := fs.String("at", "", "Time of day, HH:MM")
	tz := fs.String("tz", "", "IANA time zone (default UTC)")
	profile := fs.String("profile", "", "Stored profile name or id; empty uses the tag default")
	promote := fs.Bool("promote-default", false, "Set each captured snapshot as cohort default")
	noPromote := fs.Bool("no-promote-default", false, "Do not set the captured snapshot as default")
	catchUp := fs.Bool("catch-up", false, "Run one missed occurrence after downtime")
	noCatchUp := fs.Bool("no-catch-up", false, "Skip an occurrence missed during downtime")
	enable := fs.Bool("enable", false, "Enable the schedule")
	disable := fs.Bool("disable", false, "Pause the schedule")
	if err := parseWithReorderedFlags(fs, args); err != nil {
		return parseExit(err)
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(errOut, "cohorts schedule set requires one cohort tag")
		return 2
	}
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
	if n := countSet(set, "monthly", "weekly", "every"); n > 1 {
		fmt.Fprintln(errOut, "use one of --monthly, --weekly and --every")
		return 2
	}
	for _, pair := range [][2]string{{"promote-default", "no-promote-default"}, {"catch-up", "no-catch-up"}, {"enable", "disable"}} {
		if set[pair[0]] && set[pair[1]] {
			fmt.Fprintf(errOut, "--%s and --%s are mutually exclusive\n", pair[0], pair[1])
			return 2
		}
	}

	cohort, err := resolveAdminCohort(ctx, client, fs.Arg(0))
	if err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	sched, err := currentSchedule(ctx, client, cohort.ID)
	if err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	switch {
	case set["monthly"]:
		days, last, err := parseMonthDays(*monthly)
		if err != nil {
			fmt.Fprintln(errOut, err.Error())
			return 2
		}
		sched.Kind, sched.DaysOfMonth, sched.LastDay = "monthly", days, last
	case set["weekly"]:
		sched.Kind, sched.Weekdays = "weekly", splitCSV(*weekly)
	case set["every"]:
		sched.Kind, sched.IntervalDays = "interval", *every
	}
	if set["from"] {
		sched.AnchorDate = *from
	}
	if sched.Kind == "interval" && sched.AnchorDate == "" {
		sched.AnchorDate = time.Now().Format(time.DateOnly)
	}
	if set["at"] {
		sched.TimeOfDay = *at
	}
	if set["tz"] {
		sched.Timezone = *tz
	}
	if set["profile"] {
		id, err := resolveProfileID(ctx, client, *profile)
		if err != nil {
			fmt.Fprintln(errOut, err.Error())
			return 2
		}
		sched.ProfileID = id
	}
	applyToggle(set, "promote-default", "no-promote-default", *promote, *noPromote, &sched.PromoteDefault)
	applyToggle(set, "catch-up", "no-catch-up", *catchUp, *noCatchUp, &sched.CatchUp)
	applyToggle(set, "enable", "disable", *enable, *disable, &sched.Enabled)

	var stored cohortSchedule
	if err := client.doJSON(ctx, "PUT", schedulePath(cohort.ID), sched.putBody(), &stored); err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	return printSchedule(out, errOut, opts, cohort, stored)
}

func runCohortsScheduleRemove(ctx context.Context, client *apiClient, args []string, out io.Writer, errOut io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(errOut, "cohorts schedule remove requires one cohort tag")
		return 2
	}
	cohort, err := resolveAdminCohort(ctx, client, args[0])
	if err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	if err := client.doJSON(ctx, "DELETE", schedulePath(cohort.ID), nil, nil); err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	fmt.Fprintf(out, "Removed the schedule of %s\n", cliterm.Sanitize(cohort.SourceTag))
	return 0
}

func schedulePath(cohortID int64) string {
	return "/analysis/cohorts/" + strconv.FormatInt(cohortID, 10) + "/schedule"
}

// resolveAdminCohort finds the cohort whose source tag is tag.
func resolveAdminCohort(ctx context.Context, client *apiClient, tag string) (adminCohort, error) {
	tag = strings.TrimSpace(tag)
	var cohorts []adminCohort
	if err := client.doJSON(ctx, "GET", "/analysis/cohorts", nil, &cohorts); err != nil {
		return adminCohort{}, err
	}
	for _, c := range cohorts {
		if c.SourceTag == tag {
			return c, nil
		}
	}
	return adminCohort{}, fmt.Errorf("no cohort has source tag %q", tag)
}

// currentSchedule reads a cohort's schedule, or the defaults of a new one.
func currentSchedule(ctx context.Context, client *apiClient, cohortID int64) (cohortSchedule, error) {
	var list []cohortSchedule
	if err := client.doJSON(ctx, "GET", "/analysis/schedules", nil, &list); err != nil {
		return cohortSchedule{}, err
	}
	for _, s := range list {
		if s.CohortID == cohortID {
			return s, nil
		}
	}
	return cohortSchedule{Enabled: true, IntervalDays: 1, Timezone: "UTC", CatchUp: true}, nil
}

// resolveProfileID maps a profile name or id to an id; empty means none.
func resolveProfileID(ctx context.Context, client *apiClient, value string) (*int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	var profiles []struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}
	if err := client.doJSON(ctx, "GET", "/profiles", nil, &profiles); err != nil {
		return nil, err
	}
	for _, p := range profiles {
		if p.Name == value || strconv.FormatInt(p.ID, 10) == value {
			id := p.ID
			return &id, nil
		}
	}
	return nil, fmt.Errorf("no stored profile named %q", value)
}

// parseMonthDays reads "1,15,last".
func parseMonthDays(value string) ([]int, bool, error) {
	days := []int{}
	last := false
	for _, part := range splitCSV(value) {
		if part == "last" {
			last = true
			continue
		}
		day, err := strconv.Atoi(part)
		if err != nil {
			return nil, false, fmt.Errorf("--monthly takes days 1 to 28 and last, not %q", part)
		}
		days = append(days, day)
	}
	return days, last, nil
}

func splitCSV(value string) []string {
	out := []string{}
	for _, part := range strings.Split(value, ",") {
		if part = strings.ToLower(strings.TrimSpace(part)); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func countSet(set map[string]bool, names ...string) int {
	n := 0
	for _, name := range names {
		if set[name] {
			n++
		}
	}
	return n
}

// applyToggle sets dst from an --x / --no-x flag pair when either was given.
func applyToggle(set map[string]bool, on, off string, onVal, offVal bool, dst *bool) {
	switch {
	case set[on]:
		*dst = onVal
	case set[off]:
		*dst = !offVal
	}
}

func lastOutcome(s cohortSchedule) string {
	if s.LastOutcome == "" {
		return ""
	}
	text := "  last=" + s.LastRunAt + " " + s.LastOutcome
	if s.LastBatchID != "" && s.LastOutcome == "submitted" {
		text += " " + s.LastBatchID
	}
	if s.LastError != "" {
		text += " (" + cliterm.Sanitize(s.LastError) + ")"
	}
	return text
}

func printSchedule(out, errOut io.Writer, opts globalOptions, cohort adminCohort, s cohortSchedule) int {
	if isJSONFormat(opts.format) {
		return writeOrFail(out, errOut, opts.format, s)
	}
	state := "enabled"
	if !s.Enabled {
		state = "paused"
	}
	fmt.Fprintf(out, "Schedule of %s (%s)\n", cliterm.Sanitize(cohort.SourceTag), state)
	fmt.Fprintf(out, "  Rule: %s\n", cliterm.Sanitize(s.Summary))
	if s.Enabled {
		fmt.Fprintf(out, "  Next: %s\n", s.NextRunAt)
	}
	if s.LastOutcome != "" {
		fmt.Fprintf(out, "  Last: %s\n", strings.TrimPrefix(lastOutcome(s), "  last="))
	}
	return 0
}

func writeOrFail(out, errOut io.Writer, format string, payload any) int {
	if err := writeOutput(out, format, payload); err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	return 0
}
