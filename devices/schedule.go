package devices

import (
	"strconv"
	"strings"
	"time"
)

// ScheduleRule is one rule of an event scheduler (spec §6.3.13). A rule in
// the CRON subset the myGrenton app uses has Supported set, with its time
// and its Days ascending from Sunday; any other rule keeps only its text.
type ScheduleRule struct {
	ID        int64
	Enabled   bool
	Cron      string
	Supported bool
	Hour      int
	Minute    int
	Days      []time.Weekday
}

// scheduleSpace is the whitespace allowed around the parts of a rule list.
const scheduleSpace = " \t\r\n\v\f"

// ParseScheduleRules reads a scheduler's ruleList, nextRule or currentRule
// text, the rules in the CLU's order. It returns nil when no rules are
// known: N/A, an empty table, or a table with a row of another shape.
func ParseScheduleRules(text string) []ScheduleRule {
	t := strings.Trim(text, scheduleSpace)
	if !strings.HasPrefix(t, "{") || !strings.HasSuffix(t, "}") || len(t) < 2 {
		return nil
	}
	inner := strings.Trim(t[1:len(t)-1], scheduleSpace)
	if !strings.HasPrefix(inner, "{") {
		if r, ok := parseRuleRow(t); ok {
			return []ScheduleRule{r}
		}
		return nil
	}
	rows, ok := splitRows(inner)
	if !ok {
		return nil
	}
	rules := make([]ScheduleRule, 0, len(rows))
	for _, row := range rows {
		r, ok := parseRuleRow(row)
		if !ok {
			return nil
		}
		rules = append(rules, r)
	}
	return rules
}

// splitRows splits a table's content on commas outside text and braces.
func splitRows(inner string) ([]string, bool) {
	var rows []string
	depth, inText, start := 0, false, 0
	for i := 0; i < len(inner); i++ {
		switch c := inner[i]; {
		case c == '"':
			inText = !inText
		case inText:
		case c == '{':
			depth++
		case c == '}':
			depth--
		case c == ',' && depth == 0:
			rows = append(rows, inner[start:i])
			start = i + 1
		}
	}
	if depth != 0 || inText {
		return nil, false
	}
	return append(rows, inner[start:]), true
}

// parseRuleRow reads {id, enabled, "cron"}: two integers and a quoted text,
// comma-separated, whitespace allowed outside the quotes.
func parseRuleRow(row string) (ScheduleRule, bool) {
	row = strings.Trim(row, scheduleSpace)
	if len(row) < 2 || row[0] != '{' || row[len(row)-1] != '}' {
		return ScheduleRule{}, false
	}
	inner := row[1 : len(row)-1]
	// Exactly three fields: the text ends at the next quote, and only
	// whitespace may follow it (an extra or embedded quote is another shape).
	q1 := strings.IndexByte(inner, '"')
	q2 := -1
	if q1 >= 0 {
		if n := strings.IndexByte(inner[q1+1:], '"'); n >= 0 {
			q2 = q1 + 1 + n
		}
	}
	if q1 < 0 || q2 < 0 || strings.Trim(inner[q2+1:], scheduleSpace) != "" {
		return ScheduleRule{}, false
	}
	head := strings.Split(inner[:q1], ",")
	if len(head) != 3 || strings.Trim(head[2], scheduleSpace) != "" {
		return ScheduleRule{}, false
	}
	id, ok1 := parseRowInteger(head[0])
	enabled, ok2 := parseRowInteger(head[1])
	if !ok1 || !ok2 {
		return ScheduleRule{}, false
	}
	r := ScheduleRule{ID: id, Enabled: enabled == 1, Cron: inner[q1+1 : q2]}
	r.Hour, r.Minute, r.Days, r.Supported = parseCron(r.Cron)
	return r, true
}

// parseRowInteger matches -?[0-9]+ within Int64.
func parseRowInteger(s string) (int64, bool) {
	s = strings.Trim(s, scheduleSpace)
	digits := strings.TrimPrefix(s, "-")
	if digits == "" || strings.Trim(digits, "0123456789") != "" {
		return 0, false
	}
	i, err := strconv.ParseInt(s, 10, 64)
	return i, err == nil
}

// parseCron reads the subset "minute hour * * days" (spec §6.3.13): days *
// or a list of 0–7 (7 = Sunday) with ranges a-b.
func parseCron(cron string) (hour, minute int, days []time.Weekday, ok bool) {
	fields := strings.Fields(cron)
	if len(fields) != 5 || fields[2] != "*" || fields[3] != "*" {
		return 0, 0, nil, false
	}
	m, okM := smallNumber(fields[0])
	h, okH := smallNumber(fields[1])
	if !okM || !okH || m > 59 || h > 23 {
		return 0, 0, nil, false
	}
	if fields[4] == "*" {
		return h, m, allDays(), true
	}
	var set [7]bool
	for _, item := range strings.Split(fields[4], ",") {
		bounds := strings.Split(item, "-")
		if len(bounds) > 2 {
			return 0, 0, nil, false
		}
		lo, okLo := smallNumber(bounds[0])
		hi, okHi := smallNumber(bounds[len(bounds)-1])
		if !okLo || !okHi || lo > 7 || hi > 7 || lo > hi {
			return 0, 0, nil, false
		}
		for d := lo; d <= hi; d++ {
			set[d%7] = true
		}
	}
	for d, in := range set {
		if in {
			days = append(days, time.Weekday(d))
		}
	}
	return h, m, days, true
}

// smallNumber is a non-empty run of ASCII digits, at most 0–99 in practice.
func smallNumber(s string) (int, bool) {
	if s == "" || len(s) > 9 || strings.Trim(s, "0123456789") != "" {
		return 0, false
	}
	n, err := strconv.Atoi(s)
	return n, err == nil
}

func allDays() []time.Weekday {
	return []time.Weekday{time.Sunday, time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday, time.Saturday}
}

// CronText is the CRON text addRule sends (spec §6.3.13): days in week
// order Monday to Sunday, all seven as *. It returns false for an hour
// outside 0–23, a minute outside 0–59, a day outside Sunday–Saturday or no
// days.
func CronText(hour, minute int, days []time.Weekday) (string, bool) {
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 || len(days) == 0 {
		return "", false
	}
	var set [7]bool
	for _, d := range days {
		if d < time.Sunday || d > time.Saturday {
			return "", false
		}
		set[d] = true
	}
	field := "*"
	var order []string
	for _, d := range []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday,
		time.Saturday, time.Sunday} {
		if set[d] {
			order = append(order, strconv.Itoa(int(d)))
		}
	}
	if len(order) < 7 {
		field = strings.Join(order, ",")
	}
	return strconv.Itoa(minute) + " " + strconv.Itoa(hour) + " * * " + field, true
}
