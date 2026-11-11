package intelligence

import "strconv"

func plural(n int) string { return strconv.Itoa(n) }

func itoa(n int) string { return strconv.Itoa(n) }

func format1(f float64) string {
	return strconv.FormatFloat(f, 'f', 1, 64)
}
