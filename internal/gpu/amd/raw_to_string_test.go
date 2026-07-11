package amd

import "testing"

func TestRawToString_returnsError_whenJSONValueIsUnsupported(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
	}{
		{name: "object", raw: []byte(`{"value":42}`)},
		{name: "malformed", raw: []byte(`{"value"`)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// When
			got, err := rawToString(tt.raw)

			// Then
			if got != "" {
				t.Fatalf("rawToString = %q, want empty", got)
			}
			requireError(t, err)
		})
	}
}
