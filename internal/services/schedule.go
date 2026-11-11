package services

import (
	"strings"
	"time"
)

const weekSeconds = 7 * 24 * 3600

var dayIndex = map[string]int{
	"MON": 0,
	"TUE": 1,
	"WED": 2,
	"THU": 3,
	"FRI": 4,
	"SAT": 5,
	"SUN": 6,
}

type clockWindow struct {
	days  []int
	start int
	end   int
}

func normalizeDays(raw string) (string, []int, string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil, "days_of_week is required"
	}
	parts := strings.Split(raw, ",")
	seen := map[int]bool{}
	days := make([]int, 0, len(parts))
	labels := make([]string, 0, len(parts))
	for _, part := range parts {
		token := strings.ToUpper(strings.TrimSpace(part))
		if token == "" {
			return "", nil, "days_of_week must be a comma-separated list such as MON,TUE,WED"
		}
		day, ok := dayIndex[token]
		if !ok {
			return "", nil, "days_of_week must use MON,TUE,WED,THU,FRI,SAT,SUN"
		}
		if seen[day] {
			return "", nil, "days_of_week repeats " + token
		}
		seen[day] = true
		days = append(days, day)
		labels = append(labels, token)
	}
	stored := strings.Join(labels, ",")
	if len(stored) > 20 {
		return "", nil, "days_of_week must be at most 20 characters"
	}
	return stored, days, ""
}

func parseClock(raw string) (int, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}
	layouts := []string{"15:04:05", "15:04"}
	var parsed time.Time
	var err error
	for _, layout := range layouts {
		parsed, err = time.Parse(layout, raw)
		if err == nil {
			return parsed.Hour()*3600 + parsed.Minute()*60 + parsed.Second(), true
		}
	}
	return 0, false
}

func formatClock(seconds int) string {
	if seconds < 0 {
		seconds = 0
	}
	h := seconds / 3600
	m := (seconds % 3600) / 60
	s := seconds % 60
	return pad2(h) + ":" + pad2(m) + ":" + pad2(s)
}

func pad2(n int) string {
	if n < 10 {
		return "0" + itoa(n)
	}
	return itoa(n)
}

func segments(w clockWindow) [][2]int {
	out := make([][2]int, 0, len(w.days)*2)
	for _, day := range w.days {
		base := day * 86400
		if w.end > w.start {
			out = append(out, [2]int{base + w.start, base + w.end})
			continue
		}
		out = append(out, [2]int{base + w.start, base + 86400})
		if w.end == 0 {
			continue
		}
		next := ((day + 1) % 7) * 86400
		out = append(out, [2]int{next, next + w.end})
	}
	return out
}

func rangesOverlap(a, b [2]int) bool {
	return a[0] < b[1] && b[0] < a[1]
}

func shiftsOverlap(a, b clockWindow) bool {
	for _, left := range segments(a) {
		for _, right := range segments(b) {
			if rangesOverlap(left, right) {
				return true
			}
		}
	}
	return false
}

func weekSecond(t time.Time) int {
	t = t.UTC()
	monday := (int(t.Weekday()) + 6) % 7
	return monday*86400 + t.Hour()*3600 + t.Minute()*60 + t.Second()
}

func absoluteOverlap(aStart time.Time, aEnd *time.Time, bStart time.Time, bEnd *time.Time) bool {
	aStop := openEnd(aEnd)
	bStop := openEnd(bEnd)
	return aStart.Before(bStop) && bStart.Before(aStop)
}

func openEnd(t *time.Time) time.Time {
	if t == nil {
		return time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)
	}
	return t.UTC()
}

func absoluteOverlapsShift(start time.Time, end *time.Time, w clockWindow) bool {
	segs := segments(w)
	if len(segs) == 0 {
		return false
	}
	if end == nil {
		return true
	}
	start = start.UTC()
	stop := end.UTC()
	if !stop.After(start) {
		return false
	}
	if stop.Sub(start) >= 7*24*time.Hour {
		return true
	}
	ws := weekSecond(start)
	dur := int(stop.Sub(start) / time.Second)
	spans := [][2]int{{ws, ws + dur}}
	if ws+dur > weekSeconds {
		spans = [][2]int{{ws, weekSeconds}, {0, ws + dur - weekSeconds}}
	}
	for _, span := range spans {
		for _, seg := range segs {
			if rangesOverlap(span, seg) {
				return true
			}
		}
	}
	return false
}

type occupancy struct {
	weekly *clockWindow
	start  time.Time
	end    *time.Time
}

func occupanciesOverlap(a, b occupancy) bool {
	switch {
	case a.weekly != nil && b.weekly != nil:
		return shiftsOverlap(*a.weekly, *b.weekly)
	case a.weekly != nil:
		return absoluteOverlapsShift(b.start, b.end, *a.weekly)
	case b.weekly != nil:
		return absoluteOverlapsShift(a.start, a.end, *b.weekly)
	default:
		return absoluteOverlap(a.start, a.end, b.start, b.end)
	}
}
