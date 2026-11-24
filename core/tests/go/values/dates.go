package main

import (
	"fmt"

	"github.com/ibanks42/goui-kit/gk"
	"github.com/ibanks42/goui-kit/gk/base"
	"github.com/ibanks42/goui-kit/gk/kit"
)

func init() {
	// Register civil date/time round-trip, invalid-date rejection, and calendar matcher tests.
	modes["val-dates"] = &mode{click: once(dates)}
	modes["val-bad-date"] = &mode{click: once(badDate)}
	modes["c6-matcher"] = &mode{click: once(matcher)}
}

// dates proves CalendarState, DatePickerState, and TimeFieldState round-trip
// leap day (2024-02-29), month grid weeks (5..6 rows of 7 days), date ranges
// (2026-01-05..9), full CivilDateTime (2026-11-11 09:30:15), and CivilTime (23:59:00).
func dates(a *App, cx *gk.Cx) {
	cal := base.NewCalendarState(cx).Build()
	cal.SetDate(cx, base.DateSingle{Value: gk.Some(gk.CivilDate{Year: 2024, Month: 2, Day: 29})})
	got, _ := cal.Date(cx).(base.DateSingle)
	d, _ := got.Value.Get()
	today := cal.Today(cx)
	weeks := cal.Days(cx)
	weekLen := 0
	if len(weeks) > 0 {
		weekLen = len(weeks[0])
	}
	fmt.Printf("values calendar date=%d-%02d-%02d today_ok=%v weeks=%d week_len=%d\n", d.Year, d.Month, d.Day,
		today.Year >= 2024 && today.Month >= 1 && today.Month <= 12, len(weeks), weekLen)
	ok := cal.ApplyDate(cx, base.DateRange{V0: gk.Some(gk.CivilDate{Year: 2026, Month: 1, Day: 5}), V1: gk.Some(gk.CivilDate{Year: 2026, Month: 1, Day: 9})})
	r, _ := cal.Date(cx).(base.DateRange)
	s, _ := r.V0.Get()
	e, _ := r.V1.Get()
	fmt.Printf("values calendar range applied=%v start=%d-%02d-%02d end=%d\n", ok, s.Year, s.Month, s.Day, e.Day)
	dp := kit.NewDatePickerState(cx).Build()
	dp.SetDateTime(cx, kit.DateTimeSingle{Value: gk.Some(gk.CivilDateTime{Date: gk.CivilDate{Year: 2026, Month: 11, Day: 11},
		Time: gk.CivilTime{Hour: 9, Minute: 30, Second: 15}})})
	dts, _ := dp.DateTime(cx).(kit.DateTimeSingle)
	dt, _ := dts.Value.Get()
	fmt.Printf("values picker date_time=%d-%02d-%02d %02d:%02d:%02d\n", dt.Date.Year, dt.Date.Month, dt.Date.Day,
		dt.Time.Hour, dt.Time.Minute, dt.Time.Second)
	tf := base.NewTimeFieldState(cx).Build()
	tf.SetTime(cx, gk.CivilTime{Hour: 23, Minute: 59})
	t := tf.Time(cx)
	fmt.Printf("values time %02d:%02d:%02d\n", t.Hour, t.Minute, t.Second)
	_ = kit.NewDateRangePresetDateTime("Launch", kit.DateTimeSingle{Value: gk.Some(gk.CivilDateTime{Date: gk.CivilDate{Year: 2026, Month: 11, Day: 11},
		Time: gk.CivilTime{Hour: 12}})})
	gk.Release(cx, cal)
	gk.Release(cx, dp)
	gk.Release(cx, tf)
}

// badDate proves non-existent civil dates (2026-02-30) fail wire validation
// immediately before reaching the trailing print statement.
func badDate(a *App, cx *gk.Cx) {
	cal := base.NewCalendarState(cx).Build()
	fmt.Println("values writing February 30")
	cal.SetDate(cx, base.DateSingle{Value: gk.Some(gk.CivilDate{Year: 2026, Month: 2, Day: 30})})
	fmt.Println("values not stopped")
}

// matcher proves all four calendar DisabledMatcher variants (Interval before
// 2026-11-12, Range Jan 1..31, Custom 2-date slice, and 2-element weekday slice)
// encode through the mirror-checked codec and read back identically.
func matcher(a *App, cx *gk.Cx) {
	d := gk.CivilDate{Year: 2026, Month: 11, Day: 12}
	cal := base.NewCalendarState(cx).DisabledMatcher(base.MatcherInterval{Value: gk.IntervalMatcher{Before: gk.Some(d)}}).Build()
	printMatcher("interval", cal.DisabledMatcherRef(cx))
	cal.SetDisabledMatcher(cx, base.MatcherRange{Value: gk.RangeMatcher{From: gk.Some(gk.CivilDate{Year: 2026, Month: 1, Day: 1}),
		To: gk.Some(gk.CivilDate{Year: 2026, Month: 1, Day: 31})}})
	printMatcher("range", cal.DisabledMatcherRef(cx))
	days := []gk.CivilDate{{Year: 2026, Month: 12, Day: 24}, {Year: 2026, Month: 12, Day: 25}}
	cal.SetDisabledMatcher(cx, base.MatcherCustom{Value: days})
	printMatcher("custom", cal.DisabledMatcherRef(cx))
	cal.SetDisabledMatcher(cx, []uint32{0, 6})
	printMatcher("weekdays", cal.DisabledMatcherRef(cx))
}

func printMatcher(what string, m gk.Option[base.Matcher]) {
	v, ok := m.Get()
	if !ok {
		fmt.Printf("values c6 matcher %s none\n", what)
		return
	}
	switch x := v.(type) {
	case base.MatcherInterval:
		b, hb := x.Value.Before.Get()
		_, ha := x.Value.After.Get()
		fmt.Printf("values c6 matcher %s interval before=%d-%d-%d has_before=%v has_after=%v\n", what, b.Year, b.Month, b.Day, hb, ha)
	case base.MatcherRange:
		f, _ := x.Value.From.Get()
		t, _ := x.Value.To.Get()
		fmt.Printf("values c6 matcher %s range from=%d-%d-%d to=%d-%d-%d\n", what, f.Year, f.Month, f.Day, t.Year, t.Month, t.Day)
	case base.MatcherCustom:
		var first gk.CivilDate
		if len(x.Value) > 0 {
			first = x.Value[0]
		}
		fmt.Printf("values c6 matcher %s custom n=%d first=%d-%d-%d\n", what, len(x.Value), first.Year, first.Month, first.Day)
	case base.MatcherDayOfWeek:
		fmt.Printf("values c6 matcher %s days n=%d\n", what, len(x.Value))
	}
}
