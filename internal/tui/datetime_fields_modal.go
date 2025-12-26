package tui

import (
        "strconv"
        "strings"
        "time"

        "clarity-cli/internal/model"
)

func parseIntDefault(s string, def int) int {
        s = strings.TrimSpace(s)
        if s == "" {
                return def
        }
        n, err := strconv.Atoi(s)
        if err != nil {
                return def
        }
        return n
}

func (m *appModel) currentDatePartsOrToday() (y int, mo int, d int) {
        now := time.Now().UTC()
        y = parseIntDefault(m.yearInput.Value(), now.Year())
        mo = parseIntDefault(m.monthInput.Value(), int(now.Month()))
        d = parseIntDefault(m.dayInput.Value(), now.Day())
        if mo < 1 {
                mo = 1
        }
        if mo > 12 {
                mo = 12
        }
        d = clampDay(y, time.Month(mo), d)
        return
}

func (m *appModel) currentTimePartsOrZero() (h int, mi int) {
        h = parseIntDefault(m.hourInput.Value(), 0)
        mi = parseIntDefault(m.minuteInput.Value(), 0)
        if h < 0 {
                h = 0
        }
        if h > 23 {
                h = 23
        }
        if mi < 0 {
                mi = 0
        }
        if mi > 59 {
                mi = 59
        }
        return
}

func (m *appModel) bumpDateTimeField(delta int) bool {
        switch m.dateFocus {
        case dateFocusYear:
                y, mo, d := m.currentDatePartsOrToday()
                y += delta
                d = clampDay(y, time.Month(mo), d)
                m.yearInput.SetValue(fmtYear(y))
                m.monthInput.SetValue(fmt2(mo))
                m.dayInput.SetValue(fmt2(d))
                return true
        case dateFocusMonth:
                y, mo, d := m.currentDatePartsOrToday()
                mo += delta
                for mo < 1 {
                        mo += 12
                        y--
                }
                for mo > 12 {
                        mo -= 12
                        y++
                }
                d = clampDay(y, time.Month(mo), d)
                m.yearInput.SetValue(fmtYear(y))
                m.monthInput.SetValue(fmt2(mo))
                m.dayInput.SetValue(fmt2(d))
                return true
        case dateFocusDay:
                y, mo, d := m.currentDatePartsOrToday()
                cur := time.Date(y, time.Month(mo), d, 0, 0, 0, 0, time.UTC)
                next := cur.AddDate(0, 0, delta)
                m.yearInput.SetValue(fmtYear(next.Year()))
                m.monthInput.SetValue(fmt2(int(next.Month())))
                m.dayInput.SetValue(fmt2(next.Day()))
                return true
        case dateFocusHour:
                h, mi := m.currentTimePartsOrZero()
                h += delta
                for h < 0 {
                        h += 24
                }
                for h >= 24 {
                        h -= 24
                }
                m.hourInput.SetValue(fmt2(h))
                m.minuteInput.SetValue(fmt2(mi))
                return true
        case dateFocusMinute:
                h, mi := m.currentTimePartsOrZero()
                mi += delta
                for mi < 0 {
                        mi += 60
                        h--
                }
                for mi >= 60 {
                        mi -= 60
                        h++
                }
                for h < 0 {
                        h += 24
                }
                for h >= 24 {
                        h -= 24
                }
                m.hourInput.SetValue(fmt2(h))
                m.minuteInput.SetValue(fmt2(mi))
                return true
        case dateFocusTimeToggle:
                return false
        default:
                return false
        }
}

func fmt2(n int) string {
        if n < 0 {
                n = 0
        }
        if n > 99 {
                n = 99
        }
        if n < 10 {
                return "0" + strconv.Itoa(n)
        }
        return strconv.Itoa(n)
}

func fmtYear(y int) string {
        if y < 0 {
                y = 0
        }
        s := strconv.Itoa(y)
        for len(s) < 4 {
                s = "0" + s
        }
        return s
}

func (m *appModel) applyDateFieldFocus() {
        m.yearInput.Blur()
        m.monthInput.Blur()
        m.dayInput.Blur()
        m.hourInput.Blur()
        m.minuteInput.Blur()
        switch m.dateFocus {
        case dateFocusYear:
                m.yearInput.Focus()
        case dateFocusMonth:
                m.monthInput.Focus()
        case dateFocusDay:
                m.dayInput.Focus()
        case dateFocusTimeToggle:
                // no input focus
        case dateFocusHour:
                if m.timeEnabled {
                        m.hourInput.Focus()
                }
        case dateFocusMinute:
                if m.timeEnabled {
                        m.minuteInput.Focus()
                }
        }
}

func parseDateTimeInputsFields(year, month, day, hour, minute string) (*model.DateTime, error) {
        year = strings.TrimSpace(year)
        month = strings.TrimSpace(month)
        day = strings.TrimSpace(day)
        if year == "" || month == "" || day == "" {
                return nil, errMissingDate
        }
        y, err := strconv.Atoi(year)
        if err != nil || len(year) != 4 {
                return nil, errInvalidDate
        }
        mo, err := strconv.Atoi(month)
        if err != nil || mo < 1 || mo > 12 {
                return nil, errInvalidDate
        }
        dd, err := strconv.Atoi(day)
        if err != nil || dd < 1 || dd > daysInMonth(y, time.Month(mo)) {
                return nil, errInvalidDate
        }
        date := fmtYear(y) + "-" + fmt2(mo) + "-" + fmt2(dd)

        hour = strings.TrimSpace(hour)
        minute = strings.TrimSpace(minute)
        if hour == "" && minute == "" {
                return &model.DateTime{Date: date, Time: nil}, nil
        }
        if hour == "" || minute == "" {
                return nil, errInvalidTime
        }
        h, err := strconv.Atoi(hour)
        if err != nil || h < 0 || h > 23 {
                return nil, errInvalidTime
        }
        mi, err := strconv.Atoi(minute)
        if err != nil || mi < 0 || mi > 59 {
                return nil, errInvalidTime
        }
        tmp := fmt2(h) + ":" + fmt2(mi)
        return &model.DateTime{Date: date, Time: &tmp}, nil
}

var (
        errMissingDate = &dateTimeParseErr{msg: "missing date (year/month/day)"}
        errInvalidDate = &dateTimeParseErr{msg: "invalid date (expected YYYY-MM-DD fields)"}
        errInvalidTime = &dateTimeParseErr{msg: "invalid time (expected HH and MM, 24h)"}
)

type dateTimeParseErr struct{ msg string }

func (e *dateTimeParseErr) Error() string { return e.msg }
