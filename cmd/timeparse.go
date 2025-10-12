package cmd

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Flexible time parsing utilities to support relative dates, weekdays, durations,
// ranges with dash, time-only ranges, now-anchors and duration anchors.
//
// These helpers are intentionally pure-ish (no global state mutation) and rely on
// the package-level providers `Now()` and configuration via Viper (timezone, first_weekday).
//
// Common usage (examples):
//   ParseFlexibleRange([]string{"yesterday", "09:00", "10:30"}, Now())
//   ParseFlexibleRange([]string{"mon", "14:00", "15:00"}, Now())
//   ParseFlexibleRange([]string{"9-12"}, Now())
//   ParseFlexibleRange([]string{"13:00", "+45m"}, Now())
//   ParseFlexibleRange([]string{"now-30m"}, Now())
//
// Returned `consumed` is how many tokens were consumed from the input slice to form the range.
// Callers can use it to advance through positional args (e.g., customer/project following the range).

// ParseFlexibleRange tries to construct a start and end time from a slice of command-line tokens.
// It accepts a variety of patterns described above. The `anchor` parameter is used as the reference
// time (usually Now()) when resolving "today"/"yesterday"/weekday/durations without explicit date.
func ParseFlexibleRange(tokens []string, anchor time.Time) (start time.Time, end time.Time, consumed int, err error) {
	if len(tokens) == 0 {
		return time.Time{}, time.Time{}, 0, fmt.Errorf("no time tokens provided")
	}

	// normalize anchor into configured timezone
	loc := parserLocation()
	anchor = anchor.In(loc)

	// helper to consume n tokens and return error-wrapped
	retErr := func(e error) (time.Time, time.Time, int, error) { return time.Time{}, time.Time{}, 0, e }

	// Pattern 1: single token that contains a dash (range shorthand) e.g., "9-12", "now-30m", "2h-now"
	if strings.Contains(tokens[0], "-") {
		s := tokens[0]
		left, right := splitDashRange(s)
		st, en, err := resolveDashRangeSides(left, right, anchor, loc)
		if err != nil {
			return retErr(err)
		}
		return st, en, 1, nil
	}

	// Pattern 2: three tokens like "yesterday 09:00 10:30" or "mon 14:00 15:00"
	if len(tokens) >= 3 && looksLikeDateWord(tokens[0]) && looksLikeTime(tokens[1]) && looksLikeTime(tokens[2]) {
		date, err := resolveDateWord(tokens[0], anchor, loc)
		if err != nil {
			return retErr(err)
		}
		st, err := parseTimeOfDay(tokens[1], date, loc)
		if err != nil {
			return retErr(fmt.Errorf("start time: %w", err))
		}
		en, err := parseTimeOfDay(tokens[2], date, loc)
		if err != nil {
			return retErr(fmt.Errorf("end time: %w", err))
		}
		if !en.After(st) {
			return retErr(fmt.Errorf("end time must be after start time"))
		}
		return st, en, 3, nil
	}

	// Pattern 3: two tokens that are both times (assume same day: today or date word preceding in shell)
	if len(tokens) >= 2 && looksLikeTime(tokens[0]) && looksLikeTime(tokens[1]) {
		// use anchor's date
		date := anchor
		st, err := parseTimeOfDay(tokens[0], date, loc)
		if err != nil {
			return retErr(fmt.Errorf("start time: %w", err))
		}
		en, err := parseTimeOfDay(tokens[1], date, loc)
		if err != nil {
			return retErr(fmt.Errorf("end time: %w", err))
		}
		if !en.After(st) {
			return retErr(fmt.Errorf("end time must be after start time"))
		}
		return st, en, 2, nil
	}

	// Pattern 4: two tokens where second is a duration like "13:00 +45m"
	if len(tokens) >= 2 && (looksLikeTime(tokens[0]) || isAbsoluteDate(tokens[0])) && looksLikeDuration(tokens[1]) {
		var st time.Time
		var err error
		if looksLikeTime(tokens[0]) {
			st, err = parseTimeOfDay(tokens[0], anchor, loc)
		} else {
			st, err = mustParseTimeFlexible(tokens[0], loc)
		}
		if err != nil {
			return retErr(err)
		}
		dur, err := parseDuration(tokens[1])
		if err != nil {
			return retErr(err)
		}
		en := st.Add(dur)
		return st, en, 2, nil
	}

	// Pattern 5: two tokens where first is a duration and second is absolute anchor: "+45m 13:00" -> start = anchor - dur
	if len(tokens) >= 2 && looksLikeDuration(tokens[0]) && (looksLikeTime(tokens[1]) || tokens[1] == "now" || isAbsoluteDate(tokens[1])) {
		dur, err := parseDuration(tokens[0])
		if err != nil {
			return retErr(err)
		}
		var anchorPoint time.Time
		if tokens[1] == "now" {
			anchorPoint = Now().In(loc)
		} else if looksLikeTime(tokens[1]) {
			anchorPoint, err = parseTimeOfDay(tokens[1], anchor, loc)
			if err != nil {
				return retErr(err)
			}
		} else {
			anchorPoint, err = mustParseTimeFlexible(tokens[1], loc)
			if err != nil {
				return retErr(err)
			}
		}
		st := anchorPoint.Add(-dur)
		en := anchorPoint
		return st, en, 2, nil
	}

	// Pattern 6: single token absolute time (interpret as start and require caller to provide end separately)
	// We'll parse it and return consumed=1 with end zero; callers should detect missing end.
	if looksLikeTime(tokens[0]) || tokens[0] == "now" || looksLikeDate(tokens[0]) || isAbsoluteDate(tokens[0]) {
		var st time.Time
		var err error
		if tokens[0] == "now" {
			st = Now().In(loc)
		} else if looksLikeTime(tokens[0]) {
			st, err = parseTimeOfDay(tokens[0], anchor, loc)
			if err != nil {
				return retErr(err)
			}
		} else {
			st, err = mustParseTimeFlexible(tokens[0], loc)
			if err != nil {
				return retErr(err)
			}
		}
		return st, time.Time{}, 1, nil
	}

	return retErr(fmt.Errorf("unrecognized time tokens: %v", tokens))
}

// mustParseTimeFlexible attempts to parse common date/time formats, using the configured timezone (loc).
// It mirrors the behaviour of existing mustParseTimeLocal but accepts a timezone location as parameter
// and returns an error instead of exiting.
func mustParseTimeFlexible(s string, loc *time.Location) (time.Time, error) {
	// Try RFC3339 first (accepts explicit timezone)
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.In(loc), nil
	}

	// Accept full date+time without timezone (T separator)
	if t, err := time.ParseInLocation("2006-01-02T15:04:05", s, loc); err == nil {
		return t, nil
	}
	if t, err := time.ParseInLocation("2006-01-02T15:04", s, loc); err == nil {
		return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), 0, 0, loc), nil
	}

	// Accept space-separated date+time (with and without seconds)
	if t, err := time.ParseInLocation("2006-01-02 15:04:05", s, loc); err == nil {
		return t, nil
	}
	if t, err := time.ParseInLocation("2006-01-02 15:04", s, loc); err == nil {
		return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), 0, 0, loc), nil
	}

	// Accept date-only "2006-01-02"
	if t, err := time.ParseInLocation("2006-01-02", s, loc); err == nil {
		// return midnight of that day
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc), nil
	}

	return time.Time{}, fmt.Errorf("cannot parse absolute time/date: %s", s)
}

// resolveDashRangeSides takes left/right substrings (trimmed) and returns start/end.
// Left or right may be durations, times, weekdays, "now" or empty.
func resolveDashRangeSides(left, right string, anchor time.Time, loc *time.Location) (time.Time, time.Time, error) {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)

	// helper to parse a side which can be: "now", time of day, duration, absolute date/time, weekday, dateword
	parseSide := func(tok string) (isDuration bool, t time.Time, dur time.Duration, err error) {
		if tok == "" {
			return false, time.Time{}, 0, nil
		}
		if tok == "now" {
			return false, Now().In(loc), 0, nil
		}
		// Prefer interpreting tokens that look like times as time-of-day (handles "9", "09", "09:00")
		if looksLikeTime(tok) {
			tt, e := parseTimeOfDay(tok, anchor, loc)
			return false, tt, 0, e
		}
		// Date-words / weekdays next
		if looksLikeDateWord(tok) || looksLikeWeekday(tok) {
			date, e := resolveDateWord(tok, anchor, loc)
			if e != nil {
				return false, time.Time{}, 0, e
			}
			// date without time -> midnight
			return false, time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, loc), 0, nil
		}
		// Durations (\"+45m\", \"2h\", or plain integer minutes) after time/date heuristics
		if looksLikeDuration(tok) {
			d, e := parseDuration(tok)
			return true, time.Time{}, d, e
		}
		// fallback to absolute parse (date or datetime)
		tt, e := mustParseTimeFlexible(tok, loc)
		if e != nil {
			return false, time.Time{}, 0, e
		}
		return false, tt, 0, nil
	}

	leftDur, leftT, leftD, err := parseSide(left)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("left side parse: %w", err)
	}
	rightDur, rightT, rightD, err := parseSide(right)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("right side parse: %w", err)
	}

	// Cases:
	// - both absolute times -> start=leftT, end=rightT
	// - left duration + right absolute -> start = right - dur, end = right
	// - left absolute + right duration -> start = left, end = left + dur
	// - left absolute + right empty -> treat as start only (caller must handle missing end)
	// - left duration + right empty -> start = anchor - dur, end = anchor
	// - both durations -> unavailable
	// - any "now" resolved already as absolute

	// both duration -> error
	if leftDur && rightDur {
		return time.Time{}, time.Time{}, fmt.Errorf("both sides are durations; ambiguous")
	}

	if leftDur && !rightDur {
		// start = rightT - leftD
		if rightT.IsZero() {
			// if right is zero -> use anchor
			rightT = Now().In(loc)
		}
		start := rightT.Add(-leftD)
		return start, rightT, nil
	}
	if !leftDur && rightDur {
		if leftT.IsZero() {
			// if left missing, use anchor as start and end = start + duration
			leftT = Now().In(loc)
		}
		end := leftT.Add(rightD)
		return leftT, end, nil
	}
	// neither is duration: treat them as times
	// if right is zero -> return start-only
	if rightT.IsZero() {
		return leftT, time.Time{}, nil
	}
	if !rightT.After(leftT) {
		return time.Time{}, time.Time{}, fmt.Errorf("end must be after start in range: %s-%s", left, right)
	}
	return leftT, rightT, nil
}

// splitDashRange splits on the first dash not inside an ISO date (i.e., 2006-01-02).
// A simple heuristic: if token contains 'T' or contains two '-' and looks like ISO date, we avoid splitting inside date.
func splitDashRange(s string) (string, string) {
	// Attempt to find a split position where both sides parse as absolute datetimes.
	// This solves cases like "2025-10-10T09:00:00Z-2025-10-10T11:00:00Z".
	// We'll try each '-' as a candidate separator and prefer the first candidate where both sides parse.
	for i := 0; i < len(s); i++ {
		if s[i] != '-' {
			continue
		}
		left := strings.TrimSpace(s[:i])
		right := strings.TrimSpace(s[i+1:])
		// Try parsing both sides as absolute date/time using the flexible parser.
		if _, errL := mustParseTimeFlexible(left, parserLocation()); errL == nil {
			if _, errR := mustParseTimeFlexible(right, parserLocation()); errR == nil {
				return left, right
			}
		}
	}
	// Fallback: split on the first dash (preserves simple user patterns like \"9-12\").
	parts := strings.SplitN(s, "-", 2)
	if len(parts) == 2 {
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	return s, ""
}

// parserLocation returns the *time.Location configured via viper "timezone" or time.Local fallback.
func parserLocation() *time.Location {
	if tz := viper.GetString("timezone"); tz != "" {
		if l, err := time.LoadLocation(tz); err == nil {
			return l
		}
	}
	return time.Local
}

// looksLikeTime returns true if the token resembles HH:MM or HH:MM:SS or H or H: suffixes.
var timeOnlyRe = regexp.MustCompile(`^\d{1,2}(:\d{2}(:\d{2})?)?$`)

func looksLikeTime(s string) bool {
	return timeOnlyRe.MatchString(s)
}

// looksLikeDuration returns true if token is parseable as time.ParseDuration after trimming optional leading '+'.
func looksLikeDuration(s string) bool {
	_, err := parseDuration(s)
	return err == nil
}

// parseDuration accepts strings like "+45m", "45m", "2h" and returns time.Duration.
func parseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty duration")
	}
	if strings.HasPrefix(s, "+") {
		s = s[1:]
	}
	// time.ParseDuration accepts "1h", "30m", etc. Also accept plain integer minutes like "45" -> treat as minutes.
	if d, err := time.ParseDuration(s); err == nil {
		return d, nil
	}
	// fallback: plain integer -> minutes
	if re := regexp.MustCompile(`^\d+$`); re.MatchString(s) {
		// interpret as minutes
		mins := s
		d, err := time.ParseDuration(mins + "m")
		if err != nil {
			return 0, err
		}
		return d, nil
	}
	return 0, fmt.Errorf("invalid duration: %s", s)
}

// parseTimeOfDay takes a time-of-day token like "14:30" and a reference date (anchor) and returns a time.Time in loc.
func parseTimeOfDay(tok string, reference time.Time, loc *time.Location) (time.Time, error) {
	tok = strings.TrimSpace(tok)
	if tok == "now" {
		return Now().In(loc), nil
	}
	if !looksLikeTime(tok) {
		return time.Time{}, fmt.Errorf("not a time of day: %s", tok)
	}
	// Support "H", "H:MM", "HH:MM", "HH:MM:SS"
	parts := strings.Split(tok, ":")
	h := 0
	m := 0
	s := 0
	var err error
	if len(parts) >= 1 && parts[0] != "" {
		h, err = parseInt(parts[0])
		if err != nil {
			return time.Time{}, err
		}
	}
	if len(parts) >= 2 && parts[1] != "" {
		m, err = parseInt(parts[1])
		if err != nil {
			return time.Time{}, err
		}
	}
	if len(parts) >= 3 && parts[2] != "" {
		s, err = parseInt(parts[2])
		if err != nil {
			return time.Time{}, err
		}
	}
	// Build time on the same date as reference (in loc)
	ref := reference.In(loc)
	return time.Date(ref.Year(), ref.Month(), ref.Day(), h, m, s, 0, loc), nil
}

func parseInt(s string) (int, error) {
	return strconvAtoi(strings.TrimSpace(s))
}

// strconvAtoi is a thin wrapper to avoid importing strconv in many places — defined here to keep tests easier.
func strconvAtoi(s string) (int, error) {
	// Using strconv directly (helper)
	return strconv.Atoi(s)
}

//
// Date words & weekdays
//

// looksLikeDateWord detects "yesterday", "today", "tomorrow" or weekday abbreviations.
func looksLikeDateWord(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "yesterday", "today", "tomorrow":
		return true
	}
	return looksLikeWeekday(s)
}

func looksLikeWeekday(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	wd := weekdayFromString(s)
	return wd != nil
}

// resolveDateWord resolves "yesterday"/"today"/"tomorrow"/weekday into a date (time.Time at midnight in loc)
// using anchor as reference.
func resolveDateWord(s string, anchor time.Time, loc *time.Location) (time.Time, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	a := anchor.In(loc)
	switch s {
	case "today":
		return time.Date(a.Year(), a.Month(), a.Day(), 0, 0, 0, 0, loc), nil
	case "yesterday":
		d := a.AddDate(0, 0, -1)
		return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, loc), nil
	case "tomorrow":
		d := a.AddDate(0, 0, 1)
		return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, loc), nil
	default:
		// weekday resolution: choose the most recent occurrence of that weekday on or before anchor.
		wd := weekdayFromString(s)
		if wd == nil {
			return time.Time{}, fmt.Errorf("unknown date/weekday: %s", s)
		}
		// Determine first weekday config (optional)
		// We interpret weekday tokens as "the most recent such weekday <= anchor date" to be convenient for retro-add.
		offset := (int(a.Weekday()) - int(*wd) + 7) % 7
		d := a.AddDate(0, 0, -offset)
		return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, loc), nil
	}
}

// weekdayFromString returns *time.Weekday for strings like "mon", "monday", "tue", etc.
// Returns nil when unrecognized.
func weekdayFromString(s string) *time.Weekday {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "mon", "monday":
		w := time.Monday
		return &w
	case "tue", "tues", "tuesday":
		w := time.Tuesday
		return &w
	case "wed", "wednesday":
		w := time.Wednesday
		return &w
	case "thu", "thurs", "thursday":
		w := time.Thursday
		return &w
	case "fri", "friday":
		w := time.Friday
		return &w
	case "sat", "saturday":
		w := time.Saturday
		return &w
	case "sun", "sunday":
		w := time.Sunday
		return &w
	default:
		return nil
	}
}

//
// Helpers to detect absolute date/datetime tokens
//

func looksLikeDate(s string) bool {
	// quick heuristic: contains '-' and looks like yyyy-mm-dd prefix
	return regexp.MustCompile(`^\d{4}-\d{2}-\d{2}`).MatchString(s)
}

func isAbsoluteDate(s string) bool {
	// try a few common formats
	if looksLikeDate(s) {
		return true
	}
	// RFC3339 full datetime includes 'T'
	if strings.Contains(s, "T") {
		return true
	}
	return false
}

//
// Small imports used by helpers
//
