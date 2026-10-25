package handlers

import "testing"

func TestPublicationByteRange(t *testing.T) {
	for _, test := range []struct {
		value            string
		start, end       int64
		partial, invalid bool
	}{
		{"", 0, 9, false, false}, {"bytes=0-3", 0, 3, true, false}, {"bytes=5-", 5, 9, true, false}, {"bytes=-3", 7, 9, true, false}, {"bytes=-20", 0, 9, true, false}, {"bytes=20-", 0, 0, false, true}, {"bytes=-0", 0, 0, false, true}, {"bytes=0-1,5-6", 0, 0, false, true},
	} {
		t.Run(test.value, func(t *testing.T) {
			start, end, partial, e := publicationByteRange(test.value, 10)
			if (e != nil) != test.invalid {
				t.Fatalf("error=%v", e)
			}
			if e == nil && (start != test.start || end != test.end || partial != test.partial) {
				t.Fatalf("got %d %d %v", start, end, partial)
			}
		})
	}
}
