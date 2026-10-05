package plant

var hits int

// Lab2 adds 2 to hits, and 1 more unless x is negative.
func Lab2(x int) {
	if x < 0 {
		goto done
	}
	hits++
done:
	hits += 2
}
