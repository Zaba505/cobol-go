// Copyright (c) 2026 Richard Carson Derr
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package cobol

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPrinter(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    *File
		expected string
	}{
		{
			name:     "empty file prints nothing",
			input:    &File{},
			expected: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			err := Print(&buf, tc.input)

			require.NoError(t, err)
			require.Equal(t, tc.expected, buf.String())
		})
	}
}

// TestPrinterRoundTrip pins the Parse -> Print -> Parse -> Equal contract.
// Every printer method added later must have a round-trip case here.
func TestPrinterRoundTrip(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		src  string
	}{
		{
			name: "empty source",
			src:  "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			file1, err := Parse(strings.NewReader(tc.src))
			require.NoError(t, err)

			var buf bytes.Buffer
			err = Print(&buf, file1)
			require.NoError(t, err)

			file2, err := Parse(strings.NewReader(buf.String()))
			require.NoError(t, err)

			require.Equal(t, file1, file2)
		})
	}
}
