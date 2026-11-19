package main

import (
	"errors"
	"fmt"
	"time"
)

// The native calendar converter uses this explicit fixed-offset list, including
// for DST regions. Other identifiers (including UTC) fall back to browser local
// time. Preserve that distinction instead of treating every label as IANA time.
var phase3NativeCalendarOffsets = map[string]int{
	"Etc/GMT+12": -12, "Pacific/Pago_Pago": -11, "Pacific/Honolulu": -10, "America/Anchorage": -9,
	"America/Los_Angeles": -8, "America/Denver": -7, "America/Mexico_City": -6, "America/New_York": -5,
	"America/Santiago": -4, "America/Sao_Paulo": -3, "Atlantic/South_Georgia": -2, "Atlantic/Cape_Verde": -1,
	"Europe/London": 0, "Europe/Lisbon": 0, "Europe/Paris": 1, "Europe/Berlin": 1, "Europe/Rome": 1, "Europe/Madrid": 1,
	"Africa/Cairo": 2, "Europe/Moscow": 3, "Asia/Riyadh": 3, "Asia/Dubai": 4, "Asia/Karachi": 5, "Asia/Dhaka": 6,
	"Asia/Bangkok": 7, "Asia/Jakarta": 7, "Asia/Ho_Chi_Minh": 7, "Asia/Shanghai": 8, "Asia/Kuala_Lumpur": 8,
	"Asia/Tokyo": 9, "Asia/Seoul": 9, "Australia/Sydney": 10, "Pacific/Guadalcanal": 11, "Pacific/Auckland": 12,
}

func phase3CalendarDisplayZone(document, browser string) (*time.Location, map[string]any, error) {
	if document == "" || browser == "" {
		return nil, nil, errors.New("日历文档或浏览器时区缺失，无法验证实际日期")
	}
	local, err := time.LoadLocation(browser)
	if err != nil {
		return nil, nil, errors.New("浏览器时区无法解析")
	}
	context := map[string]any{"documentTimezone": document, "browserTimezone": browser}
	if offset, ok := phase3NativeCalendarOffsets[document]; ok {
		context["effectiveCalendarTimezone"] = fmt.Sprintf("UTC%+d (native fixed offset: %s)", offset, document)
		context["calendarTimezoneSource"] = "native-fixed-offset"
		return time.FixedZone(document, offset*3600), context, nil
	}
	context["effectiveCalendarTimezone"] = browser
	context["calendarTimezoneSource"] = "browser-fallback"
	context["calendarTimezoneFallbackReason"] = "document timezone is absent from the native calendar fixed-offset list"
	return local, context, nil
}

func (g *phase3TablePresence) BrowserTimezone() (string, error) {
	raw, err := g.browser.EvalReadOnly(`Intl.DateTimeFormat().resolvedOptions().timeZone`)
	if err != nil {
		return "", err
	}
	timezone, ok := raw.(string)
	if !ok || timezone == "" {
		return "", errors.New("无法读回浏览器时区")
	}
	return timezone, nil
}
