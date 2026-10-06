package plant

import "testing"

func TestEmpty(t *testing.T) {
	if Offer(make(chan int), 1) {
		t.Fatal("Offer on a channel with no receiver = true, want false")
	}
}
