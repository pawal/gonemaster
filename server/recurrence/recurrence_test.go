package recurrence

import (
	"errors"
	"slices"
	"testing"
	"time"
)

func instant(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return v
}

func monthly(days DaySet, last bool, hh, mm int, zone string) Rule {
	return Rule{Kind: KindMonthly, DaysOfMonth: days, LastDay: last, TimeOfDay: TimeOfDay{hh, mm}, Zone: zone}
}

func TestNext(t *testing.T) {
	stockholm := "Europe/Stockholm"
	anchor := Date{2026, time.November, 1}
	for _, tc := range []struct {
		name  string
		rule  Rule
		after string
		want  string
	}{
		{"monthly before the 15th", monthly(1<<1|1<<15, false, 2, 0, stockholm), "2026-10-14T12:00:00Z", "2026-10-15T00:00:00Z"},
		{"monthly after the 15th wraps to the 1st", monthly(1<<1|1<<15, false, 2, 0, stockholm), "2026-10-15T01:00:00Z", "2026-11-01T01:00:00Z"},
		{"monthly last day of a leap February", monthly(0, true, 2, 0, ""), "2028-02-15T00:00:00Z", "2028-02-29T02:00:00Z"},
		{"monthly last day of a short February", monthly(0, true, 2, 0, ""), "2027-02-15T00:00:00Z", "2027-02-28T02:00:00Z"},
		{"monthly day before last day", monthly(1<<28, true, 2, 0, ""), "2028-02-28T03:00:00Z", "2028-02-29T02:00:00Z"},
		{"weekly same day later", Rule{Kind: KindWeekly, Weekdays: 1<<time.Monday | 1<<time.Thursday, TimeOfDay: TimeOfDay{23, 30}}, "2026-10-05T12:00:00Z", "2026-10-05T23:30:00Z"},
		{"weekly strictly after", Rule{Kind: KindWeekly, Weekdays: 1<<time.Monday | 1<<time.Thursday, TimeOfDay: TimeOfDay{23, 30}}, "2026-10-05T23:30:00Z", "2026-10-08T23:30:00Z"},
		{"weekly in zone", Rule{Kind: KindWeekly, Weekdays: 1 << time.Sunday, TimeOfDay: TimeOfDay{0, 30}, Zone: stockholm}, "2026-10-03T00:00:00Z", "2026-10-03T22:30:00Z"},
		{"interval anchored in the future", Rule{Kind: KindInterval, IntervalDays: 3, AnchorDate: anchor, TimeOfDay: TimeOfDay{2, 0}}, "2026-10-01T00:00:00Z", "2026-11-01T02:00:00Z"},
		{"interval anchored in the past", Rule{Kind: KindInterval, IntervalDays: 3, AnchorDate: anchor, TimeOfDay: TimeOfDay{2, 0}}, "2026-11-05T00:00:00Z", "2026-11-07T02:00:00Z"},
		{"interval on an occurrence", Rule{Kind: KindInterval, IntervalDays: 3, AnchorDate: anchor, TimeOfDay: TimeOfDay{2, 0}}, "2026-11-04T02:00:00Z", "2026-11-07T02:00:00Z"},
		{"interval fourteen days", Rule{Kind: KindInterval, IntervalDays: 14, AnchorDate: anchor, TimeOfDay: TimeOfDay{2, 0}}, "2026-11-15T03:00:00Z", "2026-11-29T02:00:00Z"},
		{"DST gap runs an hour later", Rule{Kind: KindInterval, IntervalDays: 1, AnchorDate: Date{2026, time.March, 1}, TimeOfDay: TimeOfDay{2, 30}, Zone: stockholm}, "2026-03-28T12:00:00Z", "2026-03-29T01:30:00Z"},
		{"DST overlap takes the first instant", Rule{Kind: KindInterval, IntervalDays: 1, AnchorDate: Date{2026, time.October, 1}, TimeOfDay: TimeOfDay{2, 30}, Zone: stockholm}, "2026-10-24T12:00:00Z", "2026-10-25T00:30:00Z"},
		{"DST overlap never fires twice", Rule{Kind: KindInterval, IntervalDays: 1, AnchorDate: Date{2026, time.October, 1}, TimeOfDay: TimeOfDay{2, 30}, Zone: stockholm}, "2026-10-25T00:30:00Z", "2026-10-26T01:30:00Z"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.rule.Next(instant(t, tc.after))
			if err != nil {
				t.Fatalf("Next: %v", err)
			}
			if want := instant(t, tc.want); !got.Equal(want) {
				t.Errorf("Next = %s, want %s", got.Format(time.RFC3339), tc.want)
			}
			if got.Location() != time.UTC {
				t.Errorf("Next location = %s, want UTC", got.Location())
			}
		})
	}
}

func TestPreview(t *testing.T) {
	rule := monthly(1<<1|1<<15, false, 2, 0, "Europe/Stockholm")
	got, err := rule.Preview(instant(t, "2026-10-02T00:00:00Z"), 3)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	want := []time.Time{
		instant(t, "2026-10-15T00:00:00Z"),
		instant(t, "2026-11-01T01:00:00Z"),
		instant(t, "2026-11-15T01:00:00Z"),
	}
	if !slices.EqualFunc(got, want, time.Time.Equal) {
		t.Errorf("Preview = %v, want %v", got, want)
	}
}

func TestValidate(t *testing.T) {
	tod := TimeOfDay{2, 0}
	for _, tc := range []struct {
		name string
		rule Rule
		want error
	}{
		{"unknown kind", Rule{Kind: "cron", TimeOfDay: tod}, ErrKind},
		{"interval zero", Rule{Kind: KindInterval, IntervalDays: 0, AnchorDate: Date{2026, 11, 1}, TimeOfDay: tod}, ErrInterval},
		{"interval too long", Rule{Kind: KindInterval, IntervalDays: 366, AnchorDate: Date{2026, 11, 1}, TimeOfDay: tod}, ErrInterval},
		{"interval without anchor", Rule{Kind: KindInterval, IntervalDays: 3, TimeOfDay: tod}, ErrAnchorDate},
		{"interval with impossible anchor", Rule{Kind: KindInterval, IntervalDays: 3, AnchorDate: Date{2026, 2, 30}, TimeOfDay: tod}, ErrAnchorDate},
		{"weekly without days", Rule{Kind: KindWeekly, TimeOfDay: tod}, ErrWeekdays},
		{"monthly without days", Rule{Kind: KindMonthly, TimeOfDay: tod}, ErrDaysOfMonth},
		{"monthly day 29", Rule{Kind: KindMonthly, DaysOfMonth: 1 << 29, TimeOfDay: tod}, ErrDaysOfMonth},
		{"monthly day 0", Rule{Kind: KindMonthly, DaysOfMonth: 1, TimeOfDay: tod}, ErrDaysOfMonth},
		{"hour 24", Rule{Kind: KindMonthly, LastDay: true, TimeOfDay: TimeOfDay{24, 0}}, ErrTimeOfDay},
		{"minute 60", Rule{Kind: KindMonthly, LastDay: true, TimeOfDay: TimeOfDay{1, 60}}, ErrTimeOfDay},
		{"unknown zone", Rule{Kind: KindMonthly, LastDay: true, TimeOfDay: tod, Zone: "Mars/Olympus"}, ErrZone},
		{"host zone", Rule{Kind: KindMonthly, LastDay: true, TimeOfDay: tod, Zone: "Local"}, ErrZone},
		{"valid", Rule{Kind: KindMonthly, DaysOfMonth: 1 << 28, TimeOfDay: tod, Zone: "UTC"}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.rule.Validate()
			if !errors.Is(err, tc.want) {
				t.Errorf("Validate = %v, want %v", err, tc.want)
			}
			if _, nextErr := tc.rule.Next(time.Now()); tc.want != nil && !errors.Is(nextErr, tc.want) {
				t.Errorf("Next error = %v, want %v", nextErr, tc.want)
			}
		})
	}
}

func TestParse(t *testing.T) {
	if got, err := ParseKind("weekly"); err != nil || got != KindWeekly {
		t.Errorf("ParseKind(weekly) = %q %v", got, err)
	}
	if _, err := ParseKind("cron"); !errors.Is(err, ErrKind) {
		t.Errorf("ParseKind(cron) = %v, want ErrKind", err)
	}
	if got, err := ParseWeekdays([]string{"thu", "Mon"}); err != nil || !slices.Equal(got.Names(), []string{"mon", "thu"}) {
		t.Errorf("ParseWeekdays = %v %v, want [mon thu]", got.Names(), err)
	}
	if _, err := ParseWeekdays([]string{"monday"}); !errors.Is(err, ErrWeekdays) {
		t.Errorf("ParseWeekdays(monday) = %v, want ErrWeekdays", err)
	}
	if got, err := ParseDays([]int{15, 1}); err != nil || !slices.Equal(got.Days(), []int{1, 15}) {
		t.Errorf("ParseDays = %v %v, want [1 15]", got.Days(), err)
	}
	if _, err := ParseDays([]int{29}); !errors.Is(err, ErrDaysOfMonth) {
		t.Errorf("ParseDays(29) = %v, want ErrDaysOfMonth", err)
	}
	for _, bad := range []string{"2:00", "24:00", "02:60", "0200", "ab:cd"} {
		if _, err := ParseTimeOfDay(bad); !errors.Is(err, ErrTimeOfDay) {
			t.Errorf("ParseTimeOfDay(%q) = %v, want ErrTimeOfDay", bad, err)
		}
	}
	if got, err := ParseTimeOfDay("23:05"); err != nil || got.String() != "23:05" {
		t.Errorf("ParseTimeOfDay(23:05) = %v %v", got, err)
	}
	if got, err := ParseDate("2026-11-01"); err != nil || got.String() != "2026-11-01" {
		t.Errorf("ParseDate = %v %v", got, err)
	}
	if _, err := ParseDate("2026-02-30"); !errors.Is(err, ErrAnchorDate) {
		t.Errorf("ParseDate(2026-02-30) = %v, want ErrAnchorDate", err)
	}
}

func TestCanonical(t *testing.T) {
	got := Rule{Kind: KindWeekly, IntervalDays: 9, AnchorDate: Date{2026, 1, 1}, Weekdays: 2, DaysOfMonth: 4, LastDay: true, TimeOfDay: TimeOfDay{2, 0}}.Canonical()
	want := Rule{Kind: KindWeekly, IntervalDays: 1, Weekdays: 2, TimeOfDay: TimeOfDay{2, 0}, Zone: "UTC"}
	if got != want {
		t.Errorf("Canonical = %+v, want %+v", got, want)
	}
}

func TestString(t *testing.T) {
	tod := TimeOfDay{2, 0}
	for _, tc := range []struct {
		rule Rule
		want string
	}{
		{monthly(1<<1, false, 2, 0, "Europe/Stockholm"), "Monthly, day 1, 02:00 Europe/Stockholm"},
		{monthly(1<<1|1<<15, false, 2, 0, "Europe/Stockholm"), "Monthly, days 1 and 15, 02:00 Europe/Stockholm"},
		{monthly(1<<1|1<<10|1<<20, true, 2, 0, ""), "Monthly, days 1, 10, 20 and the last day, 02:00 UTC"},
		{monthly(0, true, 2, 0, ""), "Monthly, the last day, 02:00 UTC"},
		{Rule{Kind: KindWeekly, Weekdays: 1<<time.Sunday | 1<<time.Monday | 1<<time.Thursday, TimeOfDay: TimeOfDay{23, 30}}, "Weekly, Monday, Thursday and Sunday, 23:30 UTC"},
		{Rule{Kind: KindInterval, IntervalDays: 3, AnchorDate: Date{2026, 11, 1}, TimeOfDay: tod}, "Every 3 days from 2026-11-01, 02:00 UTC"},
		{Rule{Kind: KindInterval, IntervalDays: 1, AnchorDate: Date{2026, 11, 1}, TimeOfDay: tod}, "Every day from 2026-11-01, 02:00 UTC"},
	} {
		if got := tc.rule.String(); got != tc.want {
			t.Errorf("String = %q, want %q", got, tc.want)
		}
	}
}
