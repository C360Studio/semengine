package rule

import "testing"

// TestDetectTransition checks each of the four match histories against the
// transition it must give, and that the transitions carry the strings the
// record's last_transition field stores, which operators read by hand.
func TestDetectTransition(t *testing.T) {
	tests := []struct {
		name        string
		wasMatching bool
		nowMatching bool
		want        string
	}{
		{name: "false to true enters", wasMatching: false, nowMatching: true, want: "entered"},
		{name: "true to false exits", wasMatching: true, nowMatching: false, want: "exited"},
		{name: "true to true is no transition", wasMatching: true, nowMatching: true, want: ""},
		{name: "false to false is no transition", wasMatching: false, nowMatching: false, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := detectTransition(tt.wasMatching, tt.nowMatching); string(got) != tt.want {
				t.Errorf("detectTransition(%v, %v) = %q, want %q", tt.wasMatching, tt.nowMatching, got, tt.want)
			}
		})
	}
}
