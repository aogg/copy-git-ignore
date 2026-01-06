package main

import (
	"fmt"
	"github.com/aogg/copy-ignore/src/exclude"
	"github.com/bmatcuk/doublestar/v4"
)

func main() {
	m, err := exclude.NewMatcher([]string{"*\\vendor\\*"})
	if err != nil {
		panic(err)
	}

	testPaths := []string{
		"C:\\temp\\repo1\\vendor",
		"C:/temp/repo1/vendor",
		"repo1/vendor",
	}

	fmt.Printf("Patterns: %v\n", m.Patterns())

	for _, path := range testPaths {
		result := m.ShouldExclude(path)
		fmt.Printf("ShouldExclude(%q) = %v\n", path, result)
	}

	// Test doublestar directly
	pattern := "**/vendor/**"
	for _, path := range []string{"C:/temp/repo1/vendor", "repo1/vendor"} {
		matched, _ := doublestar.Match(pattern, path)
		fmt.Printf("doublestar.Match(%q, %q) = %v\n", pattern, path, matched)
	}
}
