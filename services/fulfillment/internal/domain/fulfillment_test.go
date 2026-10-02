package domain

import (
	"errors"
	"testing"
)

func TestLifecycle(t *testing.T) {
	status := "Accepted"
	for _, want := range []string{"Cooking", "Ready", "Delivering", "Completed"} {
		next, err := Next(status)
		if err != nil || next != want {
			t.Fatalf("%s => %s err=%v", status, next, err)
		}
		status = next
	}
	if _, err := Next(status); !errors.Is(err, ErrCompleted) {
		t.Fatal("completed advanced")
	}
}
