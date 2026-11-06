package today

import (
	"strings"
	"time"
)

// timestampLayout 是固定宽度的 UTC 格式。
// SQLite 的 TEXT 列使用 BINARY collation，只有固定宽度才能保证字符串排序等于时间排序。
const timestampLayout = "2006-01-02T15:04:05.000Z"

const dateLayout = "2006-01-02"

// DayRange 是本地自然日换算成的 UTC 区间，Start 含端点，End 不含端点。
type DayRange struct {
	Start string
	End   string
}

// FormatTimestamp 把时间转成固定 3 位小数的 UTC 文本。
func FormatTimestamp(value time.Time) string {
	return value.UTC().Format(timestampLayout)
}

// ResolveDayRange 把 date（YYYY-MM-DD）和 IANA 时区解析为 UTC 区间。
// date 为空时取 now 在该时区的当天；timezone 为空时使用 now 所在时区。
// 第二个返回值非空表示参数不合法，可直接作为用户提示。
func ResolveDayRange(date, timezone string, now time.Time) (DayRange, string) {
	location := now.Location()
	if value := strings.TrimSpace(timezone); value != "" {
		loaded, err := time.LoadLocation(value)
		if err != nil {
			return DayRange{}, "tz 必须是 IANA 时区名称"
		}
		location = loaded
	}

	day := now.In(location)
	if value := strings.TrimSpace(date); value != "" {
		parsed, err := time.ParseInLocation(dateLayout, value, location)
		if err != nil {
			return DayRange{}, "date 必须是 YYYY-MM-DD"
		}
		day = parsed
	}

	// 每天重新计算本地零点，夏令时切换日会得到 23 或 25 小时的真实长度。
	start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, location)
	end := time.Date(day.Year(), day.Month(), day.Day()+1, 0, 0, 0, 0, location)
	return DayRange{
		Start: FormatTimestamp(start),
		End:   FormatTimestamp(end),
	}, ""
}

// NormalizeTimestamp 校验 ISO 8601 时间并转为固定宽度 UTC。
// nil 或空白表示没有时间，返回 nil。
func NormalizeTimestamp(value *string) (*string, string) {
	raw := trimmedValue(value)
	if raw == "" {
		return nil, ""
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, "不是合法的 ISO 8601 时间"
	}
	normalized := FormatTimestamp(parsed)
	return &normalized, ""
}

// NormalizeOptionalTimestamp 用于更新请求：nil 表示不修改，空白表示清空。
func NormalizeOptionalTimestamp(value *string) (*string, string) {
	if value == nil {
		return nil, ""
	}
	if trimmedValue(value) == "" {
		cleared := ""
		return &cleared, ""
	}
	return NormalizeTimestamp(value)
}

// TimeRangeProblem 返回时间范围的中文问题描述，合法时返回空字符串。
func TimeRangeProblem(startAt, endAt *string) string {
	start := trimmedValue(startAt)
	end := trimmedValue(endAt)
	if end == "" {
		return ""
	}
	if start == "" {
		return "填写结束时间时必须填写开始时间"
	}
	startTime, startErr := time.Parse(time.RFC3339, start)
	endTime, endErr := time.Parse(time.RFC3339, end)
	if startErr != nil || endErr != nil {
		return "不是合法的 ISO 8601 时间"
	}
	if !endTime.After(startTime) {
		return "结束时间必须晚于开始时间"
	}
	return ""
}

func trimmedValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}
