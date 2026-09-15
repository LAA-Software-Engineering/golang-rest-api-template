package repository

import "math"

// toInt64 narrows a uint to int64 for Postgres BIGINT columns. uint is 64-bit on
// 64-bit platforms, so a value above math.MaxInt64 would wrap to a negative int64;
// clamp it instead. Application IDs are positive BIGINTs, so a value that large
// cannot match any row (the query simply finds nothing).
func toInt64(v uint) int64 {
	if uint64(v) > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(v)
}
