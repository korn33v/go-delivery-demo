package domain

import (
	"testing"
)

func TestDecision(t *testing.T) {
	for _, tc := range []struct {
		token  string
		total  int64
		status string
		bad    bool
	}{{"tok-success", 700, "Succeeded", false}, {"tok-decline", 700, "Declined", false}, {"tok-success", 0, "", true}, {"unknown", 700, "", true}} {
		status, err := Decide("order-1", tc.total, tc.token)
		if (err != nil) != tc.bad || status != tc.status {
			t.Fatalf("%+v: status %s err %v", tc, status, err)
		}
	}
}
