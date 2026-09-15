package repository

import "math"

// toInt64 narrows a uint to int64 for Postgres BIGINT columns. uint is 64-bit on
// 64-bit platforms, so a value above math.MaxInt64 would wrap to a negative int64;
// clamp it instead. Application IDs are positive BIGINTs, so a value that large
// cannot match any row (the query simply finds nothing).
//
// The value is widened to uint64 first so the bound check applies directly to the
// operand of the int64 conversion (portable across 32/64-bit builds).
func toInt64(v uint) int64 {
	u := uint64(v)
	if u > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(u)
}
