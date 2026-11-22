package typo

import (
	"slices"
	"testing"
)

func TestGenerateCountsUniqueValidDomains(t *testing.T) {
	groups, actual, err := Generate("book.com", []string{"skipLetter", "skipLetter"}, nil, 3)
	if err != nil || len(groups) != 1 || len(actual) != 3 || !slices.Contains(actual, "bok.com") {
		t.Fatalf("unexpected duplicate handling: %v, %v", actual, err)
	}
	if _, _, err := Generate("book.com", []string{"skipLetter"}, nil, 1); err == nil {
		t.Fatal("accepted more results than the configured limit")
	}
}
