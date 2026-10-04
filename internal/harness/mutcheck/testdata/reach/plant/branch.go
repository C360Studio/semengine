package plant

// Sign returns -1 for a negative x and 1 otherwise.
func Sign(x int) int {
	if x < 0 {
		x = -1
	} else {
		x = 1
	}
	return x
}

// Name names 1 and 2.
func Name(n int) string {
	var s string
	switch n {
	case 1:
		s = "one"
	case 2:
		s = "two"
	default:
		s = "many"
	}
	return s
}
