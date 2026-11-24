// val-dates, val-bad-date and c6-matcher: jiff's calendar values (gk.CivilDate, gk.CivilTime,
// gk.CivilDateTime and the Date, DateTime and Matcher variants that carry them).
package main

import (
	"fmt"

	"gpuikit.dev/gk"
)

func init() {
	modes["val-dates"] = &mode{click: once(dates)}
	modes["val-bad-date"] = &mode{click: once(badDate)}
	modes["c6-matcher"] = &mode{click: once(matcher)}
}

// jiff's calendar values (PROTOCOLS.md 13.13): a calendar's date set and read back, its today and
// weeks, a date picker's date and date with time, a time field's time, a preset of a range.
func dates(a *App, cx *gk.Cx) {
	cal := gk.NewCalendarState(cx).Build()
	cal.SetDate(cx, gk.DateSingle{Value: gk.Some(gk.CivilDate{Year: 2024, Month: 2, Day: 29})})
	got, _ := cal.Date(cx).(gk.DateSingle)
	d, _ := got.Value.Get()
	today := cal.Today(cx)
	weeks := cal.Days(cx)
	weekLen := 0
	if len(weeks) > 0 {
		weekLen = len(weeks[0])
	}
	fmt.Printf("values calendar date=%d-%02d-%02d today_ok=%v weeks=%d week_len=%d\n", d.Year, d.Month, d.Day,
		today.Year >= 2024 && today.Month >= 1 && today.Month <= 12, len(weeks), weekLen)
	ok := cal.ApplyDate(cx, gk.DateRange{V0: gk.Some(gk.CivilDate{Year: 2026, Month: 1, Day: 5}), V1: gk.Some(gk.CivilDate{Year: 2026, Month: 1, Day: 9})})
	r, _ := cal.Date(cx).(gk.DateRange)
	s, _ := r.V0.Get()
	e, _ := r.V1.Get()
	fmt.Printf("values calendar range applied=%v start=%d-%02d-%02d end=%d\n", ok, s.Year, s.Month, s.Day, e.Day)
	dp := gk.NewDatePickerState(cx).Build()
	dp.SetDateTime(cx, gk.DateTimeSingle{Value: gk.Some(gk.CivilDateTime{Date: gk.CivilDate{Year: 2026, Month: 11, Day: 11},
		Time: gk.CivilTime{Hour: 9, Minute: 30, Second: 15}})})
	dts, _ := dp.DateTime(cx).(gk.DateTimeSingle)
	dt, _ := dts.Value.Get()
	fmt.Printf("values picker date_time=%d-%02d-%02d %02d:%02d:%02d\n", dt.Date.Year, dt.Date.Month, dt.Date.Day,
		dt.Time.Hour, dt.Time.Minute, dt.Time.Second)
	tf := gk.NewTimeFieldState(cx).Build()
	tf.SetTime(cx, gk.CivilTime{Hour: 23, Minute: 59}) // the field keeps minutes by default
	t := tf.Time(cx)
	fmt.Printf("values time %02d:%02d:%02d\n", t.Hour, t.Minute, t.Second)
	_ = gk.NewDateRangePresetDateTime("Launch", gk.DateTimeSingle{Value: gk.Some(gk.CivilDateTime{Date: gk.CivilDate{Year: 2026, Month: 11, Day: 11},
		Time: gk.CivilTime{Hour: 12}})})
	gk.Release(cx, cal)
	gk.Release(cx, dp)
	gk.Release(cx, tf)
}

// Misuse: a date that does not exist stops where it is written (exit 2).
func badDate(a *App, cx *gk.Cx) {
	cal := gk.NewCalendarState(cx).Build()
	fmt.Println("values writing February 30")
	cal.SetDate(cx, gk.DateSingle{Value: gk.Some(gk.CivilDate{Year: 2026, Month: 2, Day: 30})})
	fmt.Println("values not stopped")
}

// A calendar's disabled matcher of every variant, given in the state's builder chain and by
// SetDisabledMatcher, read back by DisabledMatcherRef (comp6.odin, PROTOCOLS.md 11.21).
func matcher(a *App, cx *gk.Cx) {
	d := gk.CivilDate{Year: 2026, Month: 11, Day: 12}
	cal := gk.NewCalendarState(cx).DisabledMatcher(gk.MatcherInterval{Value: gk.IntervalMatcher{Before: gk.Some(d)}}).Build()
	printMatcher("interval", cal.DisabledMatcherRef(cx))
	cal.SetDisabledMatcher(cx, gk.MatcherRange{Value: gk.RangeMatcher{From: gk.Some(gk.CivilDate{Year: 2026, Month: 1, Day: 1}),
		To: gk.Some(gk.CivilDate{Year: 2026, Month: 1, Day: 31})}})
	printMatcher("range", cal.DisabledMatcherRef(cx))
	days := []gk.CivilDate{{Year: 2026, Month: 12, Day: 24}, {Year: 2026, Month: 12, Day: 25}}
	cal.SetDisabledMatcher(cx, gk.MatcherCustom{Value: days})
	printMatcher("custom", cal.DisabledMatcherRef(cx))
	cal.SetDisabledMatcher(cx, []uint32{0, 6})
	printMatcher("weekdays", cal.DisabledMatcherRef(cx))
}

func printMatcher(what string, m gk.Option[gk.Matcher]) {
	v, ok := m.Get()
	if !ok {
		fmt.Printf("values c6 matcher %s none\n", what)
		return
	}
	switch x := v.(type) {
	case gk.MatcherInterval:
		b, hb := x.Value.Before.Get()
		_, ha := x.Value.After.Get()
		fmt.Printf("values c6 matcher %s interval before=%d-%d-%d has_before=%v has_after=%v\n", what, b.Year, b.Month, b.Day, hb, ha)
	case gk.MatcherRange:
		f, _ := x.Value.From.Get()
		t, _ := x.Value.To.Get()
		fmt.Printf("values c6 matcher %s range from=%d-%d-%d to=%d-%d-%d\n", what, f.Year, f.Month, f.Day, t.Year, t.Month, t.Day)
	case gk.MatcherCustom:
		var first gk.CivilDate
		if len(x.Value) > 0 {
			first = x.Value[0]
		}
		fmt.Printf("values c6 matcher %s custom n=%d first=%d-%d-%d\n", what, len(x.Value), first.Year, first.Month, first.Day)
	case gk.MatcherDayOfWeek:
		fmt.Printf("values c6 matcher %s days n=%d\n", what, len(x.Value))
	}
}
