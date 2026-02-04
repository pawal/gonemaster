package cache

import "testing"

func TestDecideStrategy(t *testing.T) {
	tests := []struct {
		name   string
		result Result
		want   Decision
	}{
		{
			name:   "positive response",
			result: Result{HasResponse: true, ErrorKind: ErrorNone},
			want:   DecisionPositive,
		},
		{
			name:   "response with network error classification still caches positive",
			result: Result{HasResponse: true, ErrorKind: ErrorNetwork},
			want:   DecisionPositive,
		},
		{
			name:   "network error caches negative",
			result: Result{HasResponse: false, ErrorKind: ErrorNetwork},
			want:   DecisionNegative,
		},
		{
			name:   "canceled does not cache",
			result: Result{HasResponse: false, ErrorKind: ErrorCanceled},
			want:   DecisionNone,
		},
		{
			name:   "other error does not cache",
			result: Result{HasResponse: false, ErrorKind: ErrorOther},
			want:   DecisionNone,
		},
		{
			name:   "empty result without error does not cache",
			result: Result{HasResponse: false, ErrorKind: ErrorNone},
			want:   DecisionNone,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Decide(tc.result); got != tc.want {
				t.Fatalf("Decide(%+v) = %v, want %v", tc.result, got, tc.want)
			}
		})
	}
}
