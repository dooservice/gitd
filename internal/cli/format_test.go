package cli

import "testing"

func TestNumbersAndCommitsReadAsPeopleReadThem(t *testing.T) {
	for bytes, want := range map[int64]string{0: "0 B", 512: "512 B", 2048: "2.0 KB", 5 << 30: "5.0 GB"} {
		if got := size(bytes); got != want {
			t.Errorf("size(%d) = %q; want %q", bytes, got, want)
		}
	}
	if got := short("4efeed310944064485f024fefa3ab3c8021be23d"); got != "4efeed310944" {
		t.Errorf("short = %q", got)
	}
}
