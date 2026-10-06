package plant

// Pick returns 1 for an x between 0 and 100, and 0 otherwise.
func Pick(x int) int {
	if x > 0 &&
		x < 100 {
		return 1
	}
	return 0
}

// Lab returns x, increased by one unless it is negative.
func Lab(x int) int {
	if x < 0 {
		goto done
	}
	x++
done:
	return x
}
