package query

import (
	"context"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Classifier analyzes natural language queries to extract search intent.
type Classifier interface {
	// ClassifyQuery analyzes a query string and returns SearchOptions.
	ClassifyQuery(ctx context.Context, query string) *SearchOptions
}

// KeywordClassifier implements regex-based natural language query classification.
// Uses pattern matching to extract temporal, spatial, and intent information.
type KeywordClassifier struct{}

// NewKeywordClassifier creates a new keyword-based classifier.
func NewKeywordClassifier() *KeywordClassifier {
	return &KeywordClassifier{}
}

// Temporal patterns
var (
	yesterdayPattern  = regexp.MustCompile(`(?i)(?:^|[^-])\byesterday\b`)
	todayPattern      = regexp.MustCompile(`(?i)\btoday\b`)
	lastNHoursPattern = regexp.MustCompile(`(?i)\blast\s+(\d+)\s+hours?\b`)
	lastNDaysPattern  = regexp.MustCompile(`(?i)\blast\s+(\d+)\s+days?\b`)
	lastWeekPattern   = regexp.MustCompile(`(?i)\blast\s+week\b`)
	lastMonthPattern  = regexp.MustCompile(`(?i)\blast\s+month\b`)
	lastYearPattern   = regexp.MustCompile(`(?i)\blast\s+year\b`)
	thisWeekPattern   = regexp.MustCompile(`(?i)\bthis\s+week\b`)
	thisMonthPattern  = regexp.MustCompile(`(?i)\bthis\s+month\b`)
	thisYearPattern   = regexp.MustCompile(`(?i)\bthis\s+year\b`)
)

// Spatial patterns
var (
	inZonePattern = regexp.MustCompile(`(?i)\bin\s+(zone|area)[-\s]([a-zA-Z0-9-]+)\b`)
	zonePattern   = regexp.MustCompile(`(?i)\b(zone|area)[-\s]([a-zA-Z0-9-]+)\b`)
	withinPattern = regexp.MustCompile(`(?i)\bwithin\s+(\d+(?:\.\d+)?)\s*(km|m|miles?)\s+of\s+(-?\d+(?:\.\d+)?)\s*,\s*(-?\d+(?:\.\d+)?)`)
)

// Intent patterns
var (
	similarityPattern = regexp.MustCompile(`(?i)\b(similar|like|resembl|compar)\w*`)
	pathPattern       = regexp.MustCompile(`(?i)\b(connect|relat|path|link|between)\w*`)
)

// Aggregation patterns: the triggers of the count, avg, sum, min and max operations.
// The words are grammar, never domain words; the field after a trigger is the caller's.
// Precedence is count > avg > min > max > sum (extractAggregation).
var (
	// count: "how many", "count", "total number of", "the number of" (not "a number of").
	countPattern = regexp.MustCompile(`(?i)\b(how\s+many|total\s+number\s+of|the\s+number\s+of|count)\b`)
	// average
	avgPattern = regexp.MustCompile(`(?i)\b(average|avg|mean)\b`)
	// sum: "sum" alone, or "total" when NOT followed by "number" (handled by order of checks)
	sumPattern = regexp.MustCompile(`(?i)\b(sum|total)\b`)
	// min / max
	minPattern = regexp.MustCompile(`(?i)\b(minimum|min)\b`)
	maxPattern = regexp.MustCompile(`(?i)\b(maximum|max)\b`)
	// A numeric extremum: an extremum word, one field word, then a value noun, as in
	// "the lowest pressure reading". Group 1 is the extremum word, group 2 the field.
	extremumPattern = regexp.MustCompile(`(?i)\b(smallest|lowest|largest|highest|greatest)\s+(` + fieldWord + `)\s+(?:values?|readings?|measurements?|levels?)\b`)
	// Words that make an extremum phrase pick an entity rather than a number, when they come
	// before it in its clause: "which pump has the highest pressure reading".
	entityChoicePattern = regexp.MustCompile(`(?i)\b(which|who|with)\b`)
)

// clauseBreakPattern matches what ends a clause: one of ; , . : ? ! followed by a space or the end
// of the question, or the word "but". A mark inside a number ("2.5", "1,000") ends nothing.
var clauseBreakPattern = regexp.MustCompile(`(?i)[;,.:?!](?:\s|$)|\bbut\b`)

// fieldWord is one word of a caller's field name: letters and digits of any script, '_' and
// inner hyphens ("température", "flow-rate"), so a caller's own spelling is kept whole.
const fieldWord = `[\p{L}\p{N}_]+(?:-[\p{L}\p{N}_]+)*`

// nextWordPattern reads the word that follows, after at least one space.
var nextWordPattern = regexp.MustCompile(`^\s+(` + fieldWord + `)`)

// Ranking patterns — "top N", "bottom N", "most", "least".
var (
	topNPattern    = regexp.MustCompile(`(?i)\btop\s+(\d+)\b`)
	bottomNPattern = regexp.MustCompile(`(?i)\bbottom\s+(\d+)\b`)
	mostPattern    = regexp.MustCompile(`(?i)\bmost\b`)
	leastPattern   = regexp.MustCompile(`(?i)\bleast\b`)
)

// Entity extraction pattern - matches patterns like "sensor-001", "pump-42", "device-abc"
var entityPattern = regexp.MustCompile(`\b([a-zA-Z]+[-][a-zA-Z0-9-]+)\b`)

// ClassifyQuery analyzes a natural language query and populates SearchOptions.
// Detects temporal, spatial, similarity, path, and aggregation intents.
// Always returns non-nil SearchOptions with the original query preserved.
//
// An excluded operation gives no hint and other hints stand (#95). The seven operations are
// count, avg, sum, min, max, similarity and ranking, each excluded on its own when one of its
// triggers (a) follows not, no, never, don't, do not or without in the same clause, (b) lies
// inside a balanced pair of straight (") or curly (“ ”) double quotes, or (c) is one of two
// different operations joined by "or" in one clause. A clause is the text between ; , . : ? !
// (each followed by a space or the end of the question) or the word "but". These are word rules:
// "not only count but average" reads count as negated.
func (k *KeywordClassifier) ClassifyQuery(_ context.Context, query string) *SearchOptions {
	opts, _ := k.classify(query)
	return opts
}

// classify is ClassifyQuery, also reporting whether it excluded an operation, so the chain can
// keep the exclusion from later tiers.
func (k *KeywordClassifier) classify(query string) (*SearchOptions, bool) {
	opts := &SearchOptions{
		Query: query,
	}

	// Empty query returns empty options
	if query == "" {
		return opts, false
	}

	excluded := excludedOperations(query)

	// Extract temporal information (first match wins)
	if timeRange := k.extractTemporal(query); timeRange != nil {
		opts.TimeRange = timeRange
	}

	// Detect similarity intent
	if !excluded[operationSimilarity] && similarityPattern.MatchString(query) {
		opts.UseEmbeddings = true
	}

	// Detect path intent - "related to", "connected to", "path", "links"
	// This takes precedence over zone patterns for entity extraction
	hasPathIntent := pathPattern.MatchString(query)
	if hasPathIntent {
		opts.PathIntent = true
		if entity := k.extractEntityID(query); entity != "" {
			opts.PathStartNode = entity
		}
	}

	// Detect zone intent - "in zone-A", "zone-cold-storage"
	// A zone or area phrase is a path query from the zone; it adds no predicate filter,
	// because a framework names no domain predicate (design D5).
	// Only set zone as start node if path intent didn't already extract an entity
	if !hasPathIntent || opts.PathStartNode == "" {
		if matches := inZonePattern.FindStringSubmatch(query); matches != nil {
			opts.PathIntent = true
			if len(matches) > 2 {
				opts.PathStartNode = matches[1] + "-" + matches[2] // Reconstruct "zone-A" or "area-north"
			}
		} else if matches := zonePattern.FindStringSubmatch(query); matches != nil {
			// Also match zone patterns without "in" prefix
			opts.PathIntent = true
			if len(matches) > 2 {
				opts.PathStartNode = matches[1] + "-" + matches[2]
			}
		}
	}

	// Detect true spatial intent - "within Xkm of lat,lon"
	if bounds := k.extractSpatialBounds(query); bounds != nil {
		opts.GeoBounds = bounds
	}

	// Detect aggregation intent — count, avg, sum, min, max.
	// Must be checked before ranking because "most" / "least" are also aggregation-adjacent.
	if aggType, aggField := k.extractAggregation(query, excluded); aggType != "" {
		opts.AggregationType = aggType
		opts.AggregationField = aggField
	}

	// Detect ranking intent — "top N", "bottom N", "most", "least".
	// Only set ranking when there is no aggregation type already assigned (ranking
	// queries are a separate intent from numeric aggregations).
	if opts.AggregationType == "" && !excluded[operationRanking] {
		if rankingIntent, limit := k.extractRanking(query); rankingIntent {
			opts.RankingIntent = true
			if limit > 0 {
				opts.Limit = limit
			}
		}
	}

	return opts, len(excluded) > 0
}

// extractTemporal attempts to extract temporal range from query text.
// Returns first match found, prioritizing specific patterns over general ones.
func (k *KeywordClassifier) extractTemporal(query string) *TimeRange {
	// Try yesterday
	if yesterdayPattern.MatchString(query) {
		return k.getYesterdayRange()
	}

	// Try today
	if todayPattern.MatchString(query) {
		return k.getTodayRange()
	}

	// Try "last N hours"
	if matches := lastNHoursPattern.FindStringSubmatch(query); matches != nil {
		if hours, err := strconv.Atoi(matches[1]); err == nil {
			return k.getLastNHoursRange(hours)
		}
	}

	// Try "last N days"
	if matches := lastNDaysPattern.FindStringSubmatch(query); matches != nil {
		if days, err := strconv.Atoi(matches[1]); err == nil {
			return k.getLastNDaysRange(days)
		}
	}

	// Try "last week"
	if lastWeekPattern.MatchString(query) {
		return k.getLastNDaysRange(7)
	}

	// Try "last month"
	if lastMonthPattern.MatchString(query) {
		return k.getLastNDaysRange(30)
	}

	// Try "last year"
	if lastYearPattern.MatchString(query) {
		return k.getLastNDaysRange(365)
	}

	// Try "this week"
	if thisWeekPattern.MatchString(query) {
		return k.getThisWeekRange()
	}

	// Try "this month"
	if thisMonthPattern.MatchString(query) {
		return k.getThisMonthRange()
	}

	// Try "this year"
	if thisYearPattern.MatchString(query) {
		return k.getThisYearRange()
	}

	return nil
}

// getYesterdayRange returns TimeRange for yesterday (00:00:00 to 23:59:59).
func (k *KeywordClassifier) getYesterdayRange() *TimeRange {
	now := time.Now()
	yesterdayStart := time.Date(now.Year(), now.Month(), now.Day()-1, 0, 0, 0, 0, now.Location())
	yesterdayEnd := time.Date(now.Year(), now.Month(), now.Day()-1, 23, 59, 59, 999999999, now.Location())
	return &TimeRange{
		Start: yesterdayStart,
		End:   yesterdayEnd,
	}
}

// getTodayRange returns TimeRange for today (00:00:00 to 23:59:59).
func (k *KeywordClassifier) getTodayRange() *TimeRange {
	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	todayEnd := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 999999999, now.Location())
	return &TimeRange{
		Start: todayStart,
		End:   todayEnd,
	}
}

// getLastNHoursRange returns TimeRange from N hours ago to now.
func (k *KeywordClassifier) getLastNHoursRange(hours int) *TimeRange {
	now := time.Now()
	start := now.Add(-time.Duration(hours) * time.Hour)
	return &TimeRange{
		Start: start,
		End:   now,
	}
}

// getLastNDaysRange returns TimeRange from N days ago to now.
func (k *KeywordClassifier) getLastNDaysRange(days int) *TimeRange {
	now := time.Now()
	start := now.Add(-time.Duration(days) * 24 * time.Hour)
	return &TimeRange{
		Start: start,
		End:   now,
	}
}

// getThisWeekRange returns TimeRange for current week (Sunday to Saturday).
func (k *KeywordClassifier) getThisWeekRange() *TimeRange {
	now := time.Now()
	// Find the start of the week (Sunday)
	weekday := int(now.Weekday())
	weekStart := time.Date(now.Year(), now.Month(), now.Day()-weekday, 0, 0, 0, 0, now.Location())
	// End of week (Saturday 23:59:59)
	weekEnd := time.Date(now.Year(), now.Month(), now.Day()+(6-weekday), 23, 59, 59, 999999999, now.Location())
	return &TimeRange{
		Start: weekStart,
		End:   weekEnd,
	}
}

// getThisMonthRange returns TimeRange for current month.
func (k *KeywordClassifier) getThisMonthRange() *TimeRange {
	now := time.Now()
	// First day of month
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	// Last day of month
	nextMonth := monthStart.AddDate(0, 1, 0)
	monthEnd := nextMonth.Add(-time.Second)
	return &TimeRange{
		Start: monthStart,
		End:   monthEnd,
	}
}

// getThisYearRange returns TimeRange for current year.
func (k *KeywordClassifier) getThisYearRange() *TimeRange {
	now := time.Now()
	// First day of year
	yearStart := time.Date(now.Year(), time.January, 1, 0, 0, 0, 0, now.Location())
	// Last day of year
	yearEnd := time.Date(now.Year(), time.December, 31, 23, 59, 59, 999999999, now.Location())
	return &TimeRange{
		Start: yearStart,
		End:   yearEnd,
	}
}

// extractEntityID attempts to extract an entity ID from the query.
// Looks for patterns like "sensor-001", "pump-42", "device-abc".
// Returns the first matching entity ID found, or empty string if none.
func (k *KeywordClassifier) extractEntityID(query string) string {
	matches := entityPattern.FindStringSubmatch(query)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

// extractSpatialBounds attempts to extract spatial bounds from query text.
// Handles patterns like "within 5km of 40.7128,-74.0060".
// Converts radius-based queries to bounding boxes.
func (k *KeywordClassifier) extractSpatialBounds(query string) *SpatialBounds {
	matches := withinPattern.FindStringSubmatch(query)
	if len(matches) < 5 {
		return nil
	}

	// Parse radius value
	radiusStr := matches[1]
	radius, err := strconv.ParseFloat(radiusStr, 64)
	if err != nil || radius <= 0 {
		return nil
	}

	// Parse unit and convert to kilometers
	unit := matches[2]
	switch unit {
	case "m":
		radius = radius / 1000.0 // meters to km
	case "miles", "mile":
		radius = radius * 1.60934 // miles to km
	}
	// "km" needs no conversion

	// Parse coordinates
	latStr := matches[3]
	lonStr := matches[4]
	lat, err := strconv.ParseFloat(latStr, 64)
	if err != nil {
		return nil
	}
	lon, err := strconv.ParseFloat(lonStr, 64)
	if err != nil {
		return nil
	}

	// Convert radius to approximate degrees (rough approximation)
	// 1 degree latitude ≈ 111 km
	// 1 degree longitude ≈ 111 km * cos(latitude)
	latDelta := radius / 111.0
	lonDelta := radius / (111.0 * cosDegrees(lat))

	return &SpatialBounds{
		North: lat + latDelta,
		South: lat - latDelta,
		East:  lon + lonDelta,
		West:  lon - lonDelta,
	}
}

// extractAggregation detects aggregation intent and returns (aggregationType, aggregationField).
// Patterns are checked in precedence order: count > avg > min > max > sum.
// "total number of" binds to count before "total" alone can bind to sum.
// The field is the first word after the trigger that selected the type, skipping an optional
// "of" and "the" ("mean of pressure" gives "pressure"); count takes no field. An operation in
// excluded is skipped, so the next one in precedence order may answer.
func (k *KeywordClassifier) extractAggregation(query string, excluded map[string]bool) (string, string) {
	// count — "how many", "total number of", "the number of", "count"
	if !excluded[AggregationCount] && countPattern.MatchString(query) {
		return AggregationCount, ""
	}

	// average / mean
	if loc := avgPattern.FindStringIndex(query); loc != nil && !excluded[AggregationAvg] {
		return AggregationAvg, fieldAfter(query[loc[1]:])
	}

	// A numeric extremum ("the lowest pressure reading") is min or max like the words below;
	// each operation keeps its own first phrase, so min wins over max in either order.
	minField, maxField := numericExtrema(query)

	// min / max (check before sum so "minimum" / "maximum" are not partially matched)
	if !excluded[AggregationMin] {
		if loc := minPattern.FindStringIndex(query); loc != nil {
			return AggregationMin, fieldAfter(query[loc[1]:])
		}
		if minField != "" {
			return AggregationMin, minField
		}
	}
	if !excluded[AggregationMax] {
		if loc := maxPattern.FindStringIndex(query); loc != nil {
			return AggregationMax, fieldAfter(query[loc[1]:])
		}
		if maxField != "" {
			return AggregationMax, maxField
		}
	}

	// sum / total (bare "total" without "number of")
	if sums := sumTriggers(query); len(sums) > 0 && !excluded[AggregationSum] {
		return AggregationSum, fieldAfter(query[sums[0][1]:])
	}

	return "", ""
}

// fieldAfter returns the first word of rest, skipping an optional "of" and then an optional
// "the". It returns "" when no word follows: the field is never one of the skipped words.
func fieldAfter(rest string) string {
	word, rest := nextWord(rest)
	if strings.EqualFold(word, "of") {
		word, rest = nextWord(rest)
	}
	if strings.EqualFold(word, "the") {
		word, _ = nextWord(rest)
	}
	return word
}

// nextWord splits the word that follows s (after at least one space) from what comes after it.
func nextWord(s string) (word, rest string) {
	m := nextWordPattern.FindStringSubmatchIndex(s)
	if m == nil {
		return "", s
	}
	return s[m[2]:m[3]], s[m[1]:]
}

// sumTriggers returns the [start, end) of each sum trigger in query, leaving out the "total" of
// "total number of", which is a count trigger.
func sumTriggers(query string) [][]int {
	counts := countPattern.FindAllStringIndex(query, -1)
	var sums [][]int
	for _, loc := range sumPattern.FindAllStringIndex(query, -1) {
		if !insideAny(counts, loc[0]) {
			sums = append(sums, loc)
		}
	}
	return sums
}

// insideAny reports whether pos lies within one of the [start, end) spans.
func insideAny(spans [][]int, pos int) bool {
	for _, s := range spans {
		if s[0] <= pos && pos < s[1] {
			return true
		}
	}
	return false
}

// extremumPhrase is a numeric extremum phrase that selects min or max: its [start, end) in the
// question, and its field.
type extremumPhrase struct {
	loc   []int
	isMin bool
	field string
}

// numericExtremumPhrases returns, in order, the numeric extremum phrases of query whose clause has
// no "which", "who" or "with" before them. A field word of "of" or "the" is no field, so that
// phrase is not an extremum.
func numericExtremumPhrases(query string) []extremumPhrase {
	var phrases []extremumPhrase
	for _, m := range extremumPattern.FindAllStringSubmatchIndex(query, -1) {
		field := query[m[4]:m[5]]
		if strings.EqualFold(field, "of") || strings.EqualFold(field, "the") {
			continue
		}
		if entityChoicePattern.MatchString(clauseBefore(query, m[0])) {
			continue
		}
		word := strings.ToLower(query[m[2]:m[3]])
		phrases = append(phrases, extremumPhrase{
			loc:   m[0:2],
			isMin: word == "smallest" || word == "lowest",
			field: field,
		})
	}
	return phrases
}

// numericExtrema returns the field of the first min phrase ("smallest", "lowest") and of the
// first max phrase ("largest", "highest", "greatest") among numericExtremumPhrases; "" where there
// is none.
func numericExtrema(query string) (minField, maxField string) {
	for _, p := range numericExtremumPhrases(query) {
		if p.isMin && minField == "" {
			minField = p.field
		}
		if !p.isMin && maxField == "" {
			maxField = p.field
		}
	}
	return minField, maxField
}

// clauseStart returns the byte offset where the clause holding pos starts. A clause ends at
// ; , . : ? ! followed by a space or the end of the question, or at the word "but"; the breaks
// are read on the whole question, so a mark directly before pos ("not.count") ends nothing.
func clauseStart(query string, pos int) int {
	start := 0
	for _, loc := range clauseBreakPattern.FindAllStringIndex(query, -1) {
		if loc[1] > pos {
			break
		}
		start = loc[1]
	}
	return start
}

// clauseBefore returns the text of the clause holding byte offset pos, from the clause's start
// up to pos.
func clauseBefore(query string, pos int) string {
	return query[clauseStart(query, pos):pos]
}

// Names of the two operations that are not aggregation types; the other five are the
// Aggregation constants.
const (
	operationSimilarity = "similarity"
	operationRanking    = "ranking"
)

// Exclusion words (#95).
var (
	// negationPattern matches a word that negates a trigger after it in its clause. "do not" is
	// matched by its "not"; "don't" with a straight or a typographic apostrophe.
	negationPattern = regexp.MustCompile(`(?i)\b(?:not|no|never|don['’]t|without)\b`)
	// orPattern matches the word that joins two alternatives.
	orPattern = regexp.MustCompile(`(?i)\bor\b`)
)

// operationTriggers returns the [start, end) of every trigger of each of the seven operations
// in query: every match of the patterns the keyword tier reads for that operation.
func operationTriggers(query string) map[string][][]int {
	triggers := map[string][][]int{
		AggregationCount:    countPattern.FindAllStringIndex(query, -1),
		AggregationAvg:      avgPattern.FindAllStringIndex(query, -1),
		AggregationSum:      sumTriggers(query),
		AggregationMin:      minPattern.FindAllStringIndex(query, -1),
		AggregationMax:      maxPattern.FindAllStringIndex(query, -1),
		operationSimilarity: similarityPattern.FindAllStringIndex(query, -1),
	}
	for _, p := range numericExtremumPhrases(query) {
		op := AggregationMax
		if p.isMin {
			op = AggregationMin
		}
		triggers[op] = append(triggers[op], p.loc)
	}
	for _, pattern := range []*regexp.Regexp{topNPattern, bottomNPattern, mostPattern, leastPattern} {
		triggers[operationRanking] = append(triggers[operationRanking], pattern.FindAllStringIndex(query, -1)...)
	}
	return triggers
}

// excludedOperations returns the operations of query that give no hint, by the rules
// KeywordClassifier.ClassifyQuery states. One excluded trigger excludes its operation, even when
// another trigger of it stands alone: an exclusion fails closed.
func excludedOperations(query string) map[string]bool {
	triggers := operationTriggers(query)
	quoted := quotedSpans(query)
	excluded := make(map[string]bool)

	for op, locs := range triggers {
		for _, loc := range locs {
			if insideAny(quoted, loc[0]) || negationPattern.MatchString(clauseBefore(query, loc[0])) {
				excluded[op] = true
			}
		}
	}

	// Two different operations joined by "or" in one clause: each is undecided.
	for _, or := range orPattern.FindAllStringIndex(query, -1) {
		clause := clauseStart(query, or[0])
		before, after := map[string]bool{}, map[string]bool{}
		for op, locs := range triggers {
			for _, loc := range locs {
				if clauseStart(query, loc[0]) != clause {
					continue
				}
				if loc[1] <= or[0] {
					before[op] = true
				} else if loc[0] >= or[1] {
					after[op] = true
				}
			}
		}
		if len(before) == 0 || len(after) == 0 {
			continue
		}
		joined := make(map[string]bool)
		for op := range before {
			joined[op] = true
		}
		for op := range after {
			joined[op] = true
		}
		if len(joined) > 1 {
			for op := range joined {
				excluded[op] = true
			}
		}
	}
	return excluded
}

// quotedSpans returns the inside of each balanced pair of double quotes in query, as [open, close)
// spans from the byte after the opening mark: straight quotes pair in order (first with second,
// third with fourth), and a curly “ pairs with the next ”. An unpaired mark opens nothing.
func quotedSpans(query string) [][]int {
	var spans [][]int
	open := -1
	for i := 0; i < len(query); i++ {
		if query[i] != '"' {
			continue
		}
		if open < 0 {
			open = i + 1
		} else {
			spans = append(spans, []int{open, i})
			open = -1
		}
	}
	open = -1
	for i, r := range query {
		switch {
		case r == '“' && open < 0:
			open = i + len("“")
		case r == '”' && open >= 0:
			spans = append(spans, []int{open, i})
			open = -1
		}
	}
	return spans
}

// extractRanking detects ranking intent ("top N", "bottom N", "most", "least") and
// returns (rankingIntent, limit). limit is 0 when no explicit N is present.
func (k *KeywordClassifier) extractRanking(query string) (bool, int) {
	// "top N" — explicit limit
	if matches := topNPattern.FindStringSubmatch(query); matches != nil {
		if n, err := strconv.Atoi(matches[1]); err == nil {
			return true, n
		}
	}

	// "bottom N" — explicit limit
	if matches := bottomNPattern.FindStringSubmatch(query); matches != nil {
		if n, err := strconv.Atoi(matches[1]); err == nil {
			return true, n
		}
	}

	// "most" / "least" — no explicit limit
	if mostPattern.MatchString(query) || leastPattern.MatchString(query) {
		return true, 0
	}

	return false, 0
}

// cosDegrees calculates cosine of angle in degrees.
func cosDegrees(degrees float64) float64 {
	return math.Cos(degrees * math.Pi / 180.0)
}
