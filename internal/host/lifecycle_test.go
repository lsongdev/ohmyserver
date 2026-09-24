package host

import (
	"errors"
	"reflect"
	"testing"
)

func TestCloseStepsPreserveDeclaredOrderAndAllErrors(t *testing.T) {
	firstErr := errors.New("first close failed")
	secondErr := errors.New("second close failed")
	var order []int
	steps := []func() error{
		func() error {
			order = append(order, 1)
			return firstErr
		},
		func() error {
			order = append(order, 2)
			return secondErr
		},
	}

	err := closeSteps(steps...)
	if !reflect.DeepEqual(order, []int{1, 2}) {
		t.Fatalf("close step order = %v, want [1 2]", order)
	}
	if !errors.Is(err, firstErr) || !errors.Is(err, secondErr) {
		t.Fatalf("close error = %v, want both hook errors", err)
	}
}
