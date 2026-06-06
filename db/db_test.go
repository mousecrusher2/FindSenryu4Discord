package db

import (
	"errors"
	"fmt"
	"syscall"
	"testing"
)

func TestIsAlreadyClosedError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "broken pipe",
			err:  syscall.EPIPE,
			want: true,
		},
		{
			name: "wrapped broken pipe",
			err:  fmt.Errorf("close database: %w", syscall.EPIPE),
			want: true,
		},
		{
			name: "other error",
			err:  errors.New("close failed"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isAlreadyClosedError(tt.err); got != tt.want {
				t.Fatalf("isAlreadyClosedError() = %v, want %v", got, tt.want)
			}
		})
	}
}
