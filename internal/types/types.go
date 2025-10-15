package types

type DashboardMetrics struct {
	TotalPageVisits  int64
	UniqueVisitors   int64
	TotalAnalytics   int64
	AvgDwellTime     float64
	AvgScrollDepth   float64
	BotPercentage    float64
}

type TopPage struct {
	URL          string
	Visits       int64
	AvgDwellTime int
	AvgScroll    int
}

type TopReferrer struct {
	Referrer string
	Count    int64
}

type DailyStats struct {
	Date         string
	PageVisits   int64
	UniqueUsers  int64
	AvgDwellTime float64
}

type CountryStats struct {
	Country string
	Count   int64
}