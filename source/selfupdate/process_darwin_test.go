package selfupdate

import "testing"

func TestDarwinExecutablePathDoesNotUseArgvOrEnvironment(t *testing.T) {
	for _, tc := range []struct {
		raw, want string
	}{
		{"\x02\x00\x00\x00/Users/test/Yunkun With Spaces/bin/yunkun\x00\x00forged-argv\x00KEY=secret", "/Users/test/Yunkun With Spaces/bin/yunkun"},
		{"", ""},
		{"\x01\x00\x00\x00", ""},
		{"\x01\x00\x00\x00\x00/forged", ""},
		{"\x01\x00\x00\x00relative\x00/forged", ""},
		{"\x01\x00\x00\x00/unterminated", ""},
	} {
		if got := darwinExecutablePath([]byte(tc.raw)); got != tc.want {
			t.Fatalf("exec path = %q, want %q", got, tc.want)
		}
	}
}
