package tui

import "time"

func daysInMonth(y int, m time.Month) int {
        // Day 0 of next month is last day of this month.
        return time.Date(y, m+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

func clampDay(y int, m time.Month, d int) int {
        if d < 1 {
                return 1
        }
        max := daysInMonth(y, m)
        if d > max {
                return max
        }
        return d
}
