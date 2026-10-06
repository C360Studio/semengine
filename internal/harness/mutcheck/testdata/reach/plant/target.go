package plant

import "strconv"

// Limit is the largest value Clamp returns.
const Limit = 10

type counter int

var calls counter

var label string

func unused() int {
	calls++
	return 1
}

// Clamp returns x, at most Limit.
func Clamp(x int) int {
	label = strconv.Itoa(x)
	calls++
	calls++
	calls++
	if x > Limit {
		calls++
		calls++
		calls++
		calls++
		return Limit
	}
	return x
}
