package cron

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ParseCronExpression parses a cron expression and returns the next run time
// Supports standard 5-field cron: minute hour day month weekday
// Also supports special expressions: @yearly, @monthly, @weekly, @daily, @hourly, @minutely, @reboot
func ParseCronExpression(expr string) (*time.Time, error) {
	expr = strings.TrimSpace(expr)

	// Handle special expressions
	switch expr {
	case "@yearly", "@annually":
		return NextYearly(), nil
	case "@monthly":
		return NextMonthly(), nil
	case "@weekly":
		return NextWeekly(), nil
	case "@daily", "@midnight":
		return NextDaily(), nil
	case "@hourly":
		return NextHourly(), nil
	case "@minutely":
		return NextMinutely(), nil
	}

	// Parse standard 5-field cron expression
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return nil, fmt.Errorf("invalid cron expression: expected 5 fields, got %d", len(fields))
	}

	minute, err := parseField(fields[0], 0, 59)
	if err != nil {
		return nil, fmt.Errorf("invalid minute field: %w", err)
	}

	hour, err := parseField(fields[1], 0, 23)
	if err != nil {
		return nil, fmt.Errorf("invalid hour field: %w", err)
	}

	day, err := parseField(fields[2], 1, 31)
	if err != nil {
		return nil, fmt.Errorf("invalid day field: %w", err)
	}

	month, err := parseField(fields[3], 1, 12)
	if err != nil {
		return nil, fmt.Errorf("invalid month field: %w", err)
	}

	weekday, err := parseField(fields[4], 0, 6)
	if err != nil {
		return nil, fmt.Errorf("invalid weekday field: %w", err)
	}

	// Calculate next run time
	next := nextCronTime(minute, hour, day, month, weekday)
	return next, nil
}

// parseField parses a single cron field and returns matched values
func parseField(field string, min, max int) ([]int, error) {
	var values []int

	parts := strings.Split(field, ",")
	for _, part := range parts {
		vals, err := parsePart(part, min, max)
		if err != nil {
			return nil, err
		}
		values = append(values, vals...)
	}

	if len(values) == 0 {
		return nil, fmt.Errorf("no valid values in field")
	}

	// Remove duplicates and sort
	seen := make(map[int]bool)
	var unique []int
	for _, v := range values {
		if !seen[v] {
			seen[v] = true
			unique = append(unique, v)
		}
	}

	return unique, nil
}

// parsePart parses a part of a cron field (handles ranges and steps)
func parsePart(part string, min, max int) ([]int, error) {
	var values []int

	// Handle step values
	step := 1
	if idx := strings.Index(part, "/"); idx != -1 {
		stepStr := part[idx+1:]
		var err error
		step, err = strconv.Atoi(stepStr)
		if err != nil {
			return nil, fmt.Errorf("invalid step value: %s", stepStr)
		}
		part = part[:idx]
	}

	// Handle wildcards
	if part == "*" {
		for i := min; i <= max; i += step {
			values = append(values, i)
		}
		return values, nil
	}

	// Handle ranges
	if idx := strings.Index(part, "-"); idx != -1 {
		start, err := strconv.Atoi(part[:idx])
		if err != nil {
			return nil, fmt.Errorf("invalid range start: %s", part[:idx])
		}
		end, err := strconv.Atoi(part[idx+1:])
		if err != nil {
			return nil, fmt.Errorf("invalid range end: %s", part[idx+1:])
		}
		if start < min || start > max || end < min || end > max || start > end {
			return nil, fmt.Errorf("invalid range: %d-%d", start, end)
		}
		for i := start; i <= end; i += step {
			values = append(values, i)
		}
		return values, nil
	}

	// Handle single values
	val, err := strconv.Atoi(part)
	if err != nil {
		return nil, fmt.Errorf("invalid value: %s", part)
	}
	if val < min || val > max {
		return nil, fmt.Errorf("value out of range: %d (expected %d-%d)", val, min, max)
	}
	return []int{val}, nil
}

// nextCronTime calculates the next run time for given cron fields
func nextCronTime(minutes, hours, days, months, weekdays []int) *time.Time {
	now := time.Now()
	year := now.Year()

	// Start from the next minute
	t := time.Date(year, now.Month(), now.Day(), now.Hour(), now.Minute()+1, 0, 0, now.Location())

	// Search for up to 4 years
	for i := 0; i < 4*366*24*60; i++ {
		if contains(months, int(t.Month())) &&
			contains(days, t.Day()) &&
			contains(weekdays, int(t.Weekday())) &&
			contains(hours, t.Hour()) &&
			contains(minutes, t.Minute()) {
			return &t
		}
		t = t.Add(time.Minute)
	}

	return nil
}

// contains checks if a slice contains a value
func contains(slice []int, val int) bool {
	for _, v := range slice {
		if v == val {
			return true
		}
	}
	return false
}

// Special expression handlers

// NextYearly returns the next yearly run time (Jan 1, 00:00)
func NextYearly() *time.Time {
	now := time.Now()
	t := time.Date(now.Year()+1, 1, 1, 0, 0, 0, 0, now.Location())
	return &t
}

// NextMonthly returns the next monthly run time (1st of month, 00:00)
func NextMonthly() *time.Time {
	now := time.Now()
	year := now.Year()
	month := now.Month() + 1
	if month > 12 {
		month = 1
		year++
	}
	t := time.Date(year, month, 1, 0, 0, 0, 0, now.Location())
	return &t
}

// NextWeekly returns the next weekly run time (next Monday, 00:00)
func NextWeekly() *time.Time {
	now := time.Now()
	daysUntilMonday := int(time.Monday - now.Weekday())
	if daysUntilMonday <= 0 {
		daysUntilMonday += 7
	}
	t := now.AddDate(0, 0, daysUntilMonday)
	t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	return &t
}

// NextDaily returns the next daily run time (tomorrow, 00:00)
func NextDaily() *time.Time {
	now := time.Now()
	t := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
	return &t
}

// NextHourly returns the next hourly run time (next hour, :00)
func NextHourly() *time.Time {
	now := time.Now()
	t := time.Date(now.Year(), now.Month(), now.Day(), now.Hour()+1, 0, 0, 0, now.Location())
	return &t
}

// NextMinutely returns the next minutely run time (next minute, :00)
func NextMinutely() *time.Time {
	now := time.Now()
	t := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute()+1, 0, 0, now.Location())
	return &t
}
