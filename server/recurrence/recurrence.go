// Package recurrence computes occurrences of calendar recurrence rules.
package recurrence

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Kind names a recurrence family.
type Kind string

const (
	KindInterval Kind = "interval"
	KindWeekly   Kind = "weekly"
	KindMonthly  Kind = "monthly"
)

// ParseKind reads a kind name.
func ParseKind(s string) (Kind, error) {
	switch k := Kind(strings.TrimSpace(s)); k {
	case KindInterval, KindWeekly, KindMonthly:
		return k, nil
	}
	return "", ErrKind
}

// MaxIntervalDays bounds an interval rule.
const MaxIntervalDays = 365

// MaxDayOfMonth is the highest selectable day; later days use LastDay.
const MaxDayOfMonth = 28

const validDays DaySet = 1<<(MaxDayOfMonth+1) - 2

// Rule validation errors; the message is fit for an API response.
var (
	ErrKind        = errors.New("kind must be interval, weekly or monthly")
	ErrInterval    = fmt.Errorf("interval must be 1 to %d days", MaxIntervalDays)
	ErrAnchorDate  = errors.New("start date must be a valid YYYY-MM-DD date")
	ErrWeekdays    = errors.New("weekdays must name at least one day of mon, tue, wed, thu, fri, sat, sun")
	ErrDaysOfMonth = fmt.Errorf("days of month must be 1 to %d, at least one day or the last day", MaxDayOfMonth)
	ErrTimeOfDay   = errors.New("time of day must be HH:MM, 00:00 to 23:59")
	ErrZone        = errors.New("unknown time zone")
)

// Date is a calendar date without a zone.
type Date struct {
	Year  int
	Month time.Month
	Day   int
}

// ParseDate reads YYYY-MM-DD.
func ParseDate(s string) (Date, error) {
	t, err := time.Parse(time.DateOnly, strings.TrimSpace(s))
	if err != nil {
		return Date{}, ErrAnchorDate
	}
	return dateOf(t), nil
}

// IsZero reports whether d is unset.
func (d Date) IsZero() bool { return d == Date{} }

// String renders YYYY-MM-DD, or "" when unset.
func (d Date) String() string {
	if d.IsZero() {
		return ""
	}
	return d.midnight().Format(time.DateOnly)
}

func (d Date) valid() bool {
	return !d.IsZero() && dateOf(d.midnight()) == d
}

func (d Date) midnight() time.Time {
	return time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, time.UTC)
}

func (d Date) addDays(n int) Date {
	return dateOf(time.Date(d.Year, d.Month, d.Day+n, 0, 0, 0, 0, time.UTC))
}

// daysSince returns d minus o in days.
func (d Date) daysSince(o Date) int {
	return int(d.midnight().Sub(o.midnight()).Hours() / 24)
}

func dateOf(t time.Time) Date {
	y, m, d := t.Date()
	return Date{Year: y, Month: m, Day: d}
}

// WeekdaySet is a bitmask with bit n set for time.Weekday(n).
type WeekdaySet uint8

var weekdayNames = [7]string{"sun", "mon", "tue", "wed", "thu", "fri", "sat"}

// ParseWeekdays reads lowercase three-letter day names.
func ParseWeekdays(names []string) (WeekdaySet, error) {
	var set WeekdaySet
	for _, name := range names {
		i := slices.Index(weekdayNames[:], strings.ToLower(strings.TrimSpace(name)))
		if i < 0 {
			return 0, ErrWeekdays
		}
		set |= 1 << i
	}
	return set, nil
}

// Has reports whether wd is in the set.
func (s WeekdaySet) Has(wd time.Weekday) bool { return s&(1<<wd) != 0 }

// Names lists the set Monday first.
func (s WeekdaySet) Names() []string {
	out := []string{}
	for _, wd := range mondayFirst() {
		if s.Has(wd) {
			out = append(out, weekdayNames[wd])
		}
	}
	return out
}

func mondayFirst() []time.Weekday {
	return []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday, time.Saturday, time.Sunday}
}

// DaySet is a bitmask with bit n set for day n of the month.
type DaySet uint32

// ParseDays reads days of the month.
func ParseDays(days []int) (DaySet, error) {
	var set DaySet
	for _, d := range days {
		if d < 1 || d > MaxDayOfMonth {
			return 0, ErrDaysOfMonth
		}
		set |= 1 << d
	}
	return set, nil
}

// Has reports whether day is in the set.
func (s DaySet) Has(day int) bool { return day >= 0 && day < 32 && s&(1<<day) != 0 }

// Days lists the set in ascending order.
func (s DaySet) Days() []int {
	out := []int{}
	for d := 1; d <= MaxDayOfMonth; d++ {
		if s.Has(d) {
			out = append(out, d)
		}
	}
	return out
}

// TimeOfDay is a wall clock time.
type TimeOfDay struct {
	Hour   int
	Minute int
}

// ParseTimeOfDay reads HH:MM.
func ParseTimeOfDay(s string) (TimeOfDay, error) {
	h, m, ok := strings.Cut(strings.TrimSpace(s), ":")
	if !ok || len(h) != 2 || len(m) != 2 {
		return TimeOfDay{}, ErrTimeOfDay
	}
	hour, errH := strconv.Atoi(h)
	minute, errM := strconv.Atoi(m)
	tod := TimeOfDay{Hour: hour, Minute: minute}
	if errH != nil || errM != nil || !tod.valid() {
		return TimeOfDay{}, ErrTimeOfDay
	}
	return tod, nil
}

// String renders HH:MM.
func (t TimeOfDay) String() string { return fmt.Sprintf("%02d:%02d", t.Hour, t.Minute) }

func (t TimeOfDay) valid() bool {
	return t.Hour >= 0 && t.Hour < 24 && t.Minute >= 0 && t.Minute < 60
}

// Rule is one recurrence in a named zone.
type Rule struct {
	Kind         Kind
	IntervalDays int        // interval: 1 to MaxIntervalDays
	AnchorDate   Date       // interval: first occurrence date in Zone
	Weekdays     WeekdaySet // weekly
	DaysOfMonth  DaySet     // monthly: days 1 to MaxDayOfMonth
	LastDay      bool       // monthly: the last day of the month
	TimeOfDay    TimeOfDay
	Zone         string // IANA name, "" means UTC
}

// Validate reports the first rule error.
func (r Rule) Validate() error {
	switch r.Kind {
	case KindInterval:
		if r.IntervalDays < 1 || r.IntervalDays > MaxIntervalDays {
			return ErrInterval
		}
		if !r.AnchorDate.valid() {
			return ErrAnchorDate
		}
	case KindWeekly:
		if r.Weekdays == 0 || r.Weekdays >= 1<<7 {
			return ErrWeekdays
		}
	case KindMonthly:
		if r.DaysOfMonth&^validDays != 0 || (r.DaysOfMonth == 0 && !r.LastDay) {
			return ErrDaysOfMonth
		}
	default:
		return ErrKind
	}
	if !r.TimeOfDay.valid() {
		return ErrTimeOfDay
	}
	_, err := r.location()
	return err
}

// Canonical clears the fields the kind does not use and names UTC.
func (r Rule) Canonical() Rule {
	out := Rule{Kind: r.Kind, IntervalDays: 1, TimeOfDay: r.TimeOfDay, Zone: r.Zone}
	switch r.Kind {
	case KindInterval:
		out.IntervalDays, out.AnchorDate = r.IntervalDays, r.AnchorDate
	case KindWeekly:
		out.Weekdays = r.Weekdays
	case KindMonthly:
		out.DaysOfMonth, out.LastDay = r.DaysOfMonth, r.LastDay
	}
	if out.Zone == "" {
		out.Zone = "UTC"
	}
	return out
}

func (r Rule) location() (*time.Location, error) {
	if r.Zone == "" {
		return time.UTC, nil
	}
	// "Local" is the host zone, which differs between servers.
	if r.Zone == "Local" {
		return nil, fmt.Errorf("%w: %q", ErrZone, r.Zone)
	}
	loc, err := time.LoadLocation(r.Zone)
	if err != nil {
		return nil, fmt.Errorf("%w: %q", ErrZone, r.Zone)
	}
	return loc, nil
}

// Next returns the first occurrence strictly after t, in UTC.
func (r Rule) Next(t time.Time) (time.Time, error) {
	if err := r.Validate(); err != nil {
		return time.Time{}, err
	}
	loc, _ := r.location()
	// The day before covers an occurrence a DST gap pushed past midnight.
	start := dateOf(t.In(loc)).addDays(-1)
	if r.Kind == KindInterval {
		k := 0
		if gap := start.daysSince(r.AnchorDate); gap > 0 {
			k = (gap + r.IntervalDays - 1) / r.IntervalDays
		}
		for ; ; k++ {
			if at := r.at(r.AnchorDate.addDays(k*r.IntervalDays), loc); at.After(t) {
				return at, nil
			}
		}
	}
	for i := range 64 {
		d := start.addDays(i)
		if !r.matches(d) {
			continue
		}
		if at := r.at(d, loc); at.After(t) {
			return at, nil
		}
	}
	return time.Time{}, errors.New("no occurrence within 64 days")
}

// Preview returns the n occurrences after t.
func (r Rule) Preview(t time.Time, n int) ([]time.Time, error) {
	out := make([]time.Time, 0, n)
	for range n {
		next, err := r.Next(t)
		if err != nil {
			return nil, err
		}
		out = append(out, next)
		t = next
	}
	return out, nil
}

func (r Rule) matches(d Date) bool {
	switch r.Kind {
	case KindWeekly:
		return r.Weekdays.Has(d.midnight().Weekday())
	case KindMonthly:
		return r.DaysOfMonth.Has(d.Day) || (r.LastDay && d.addDays(1).Day == 1)
	}
	return false
}

// at returns the first instant d shows TimeOfDay in loc, in UTC.
func (r Rule) at(d Date, loc *time.Location) time.Time {
	// A nonexistent wall clock is moved forward by time.Date.
	t := time.Date(d.Year, d.Month, d.Day, r.TimeOfDay.Hour, r.TimeOfDay.Minute, 0, 0, loc)
	_, cur := t.Zone()
	_, prev := t.Add(-3 * time.Hour).Zone()
	if prev > cur {
		if e := t.Add(-time.Duration(prev-cur) * time.Second); sameWall(e, t) {
			t = e
		}
	}
	return t.UTC()
}

func sameWall(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd && a.Hour() == b.Hour() && a.Minute() == b.Minute()
}

// String renders the rule in English.
func (r Rule) String() string {
	var what string
	switch r.Kind {
	case KindInterval:
		if r.IntervalDays == 1 {
			what = "Every day from " + r.AnchorDate.String()
		} else {
			what = fmt.Sprintf("Every %d days from %s", r.IntervalDays, r.AnchorDate)
		}
	case KindWeekly:
		var names []string
		for _, wd := range mondayFirst() {
			if r.Weekdays.Has(wd) {
				names = append(names, wd.String())
			}
		}
		what = "Weekly, " + joinAnd(names)
	case KindMonthly:
		var parts []string
		for _, d := range r.DaysOfMonth.Days() {
			parts = append(parts, strconv.Itoa(d))
		}
		label := "day "
		if len(parts) > 1 {
			label = "days "
		}
		switch {
		case len(parts) == 0:
			what = "Monthly, the last day"
		case r.LastDay:
			what = "Monthly, " + label + joinAnd(append(parts, "the last day"))
		default:
			what = "Monthly, " + label + joinAnd(parts)
		}
	default:
		return string(r.Kind)
	}
	zone := r.Zone
	if zone == "" {
		zone = "UTC"
	}
	return fmt.Sprintf("%s, %s %s", what, r.TimeOfDay, zone)
}

func joinAnd(items []string) string {
	if len(items) < 2 {
		return strings.Join(items, "")
	}
	return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
}
