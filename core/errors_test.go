package core

import (
	"errors"
	"testing"
)

func TestIsNotFoundError_WithExactError(t *testing.T) {
	err := ErrNotFound
	if !IsNotFoundError(err) {
		t.Error("expected true for ErrNotFound")
	}
}

func TestIsNotFoundError_WithWrappedError(t *testing.T) {
	err := errors.New("barry: not found")
	if !IsNotFoundError(err) {
		t.Error("expected true for error with same message as ErrNotFound")
	}
}

func TestIsNotFoundError_WithDifferentError(t *testing.T) {
	err := errors.New("some other error")
	if IsNotFoundError(err) {
		t.Error("expected false for unrelated error")
	}
}

func TestIsNotFoundError_WithNil(t *testing.T) {
	if IsNotFoundError(nil) {
		t.Error("expected false for nil error")
	}
}
