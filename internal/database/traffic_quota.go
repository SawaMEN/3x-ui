package database

import "fmt"

// TrafficQuotaExpr compares counters without adding BIGINTs. CASE also avoids
// subtracting a corrupt negative counter: SQL need not short-circuit AND/OR.
// Arguments are trusted SQL column names, never request values.
func TrafficQuotaExpr(up, down, total string) string {
	return fmt.Sprintf(`(%[3]s > 0 AND CASE WHEN %[1]s >= %[3]s OR %[2]s >= %[3]s THEN TRUE WHEN %[1]s > 0 AND %[2]s > 0 THEN %[1]s >= %[3]s - %[2]s ELSE FALSE END)`, up, down, total)
}
