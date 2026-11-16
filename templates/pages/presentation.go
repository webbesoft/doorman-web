package pages

import (
	"fmt"
	"time"

	"github.com/webbesoft/doorman/internal/types"
)

func chartMaximum(days []types.DailyStats) int64 {
	var maximum int64 = 2
	for _, day := range days {
		if day.PageVisits > maximum {
			maximum = day.PageVisits
		}
		if day.UniqueUsers > maximum {
			maximum = day.UniqueUsers
		}
	}
	// An even scale keeps the midpoint label accurate for small counts.
	if maximum%2 != 0 {
		maximum++
	}
	return maximum
}

func barHeight(value, maximum int64) string {
	return fmt.Sprintf("height: %.2f%%", float64(value)/float64(maximum)*100)
}

func shortDate(value string) string {
	if date, err := time.Parse("2006-01-02", value); err == nil {
		return date.Format("02 Jan")
	}
	return value
}
