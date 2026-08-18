package ru

import "testing"

func TestPlural(t *testing.T) {
	cases := map[int]string{
		0: "человек", 1: "человек", 2: "человека", 3: "человека", 4: "человека",
		5: "человек", 11: "человек", 12: "человек", 14: "человек",
		21: "человек", 22: "человека", 25: "человек", 101: "человек", 111: "человек",
	}

	for n, want := range cases {
		if got := Plural(n, "человек", "человека", "человек"); got != want {
			t.Errorf("Plural(%d) = %q, want %q", n, got, want)
		}
	}

	forms := map[int]string{1: "лайк", 2: "лайка", 5: "лайков", 21: "лайк", 24: "лайка"}
	for n, want := range forms {
		if got := Plural(n, "лайк", "лайка", "лайков"); got != want {
			t.Errorf("Plural(%d) = %q, want %q", n, got, want)
		}
	}
}
