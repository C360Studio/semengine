package plant

// Offer sends v on ch when ch has room, and reports whether it did.
func Offer(ch chan int, v int) bool {
	select {
	case ch <- v:
		return true
	default:
	}
	return false
}
