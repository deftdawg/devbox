// Copyright 2024 Jetify Inc. and contributors. All rights reserved.
// Use of this source code is governed by the license in the LICENSE file.

package boxcli

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"go.jetify.com/devbox/internal/searcher"
)

// Helper to convert YYYY-MM-DD to Unix timestamp for mock data
func dateToTimestamp(dateStr string) int {
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		panic(err) // Should not happen in tests
	}
	return int(t.Unix())
}

var mockResults = &searcher.SearchResults{
	Packages: []searcher.Package{
		{
			Name: "PackageA",
			Versions: []searcher.PackageVersion{
				{Version: "1.0.0", LastUpdated: dateToTimestamp("2023-01-15")}, // Included by onlyBefore=2023-07-01, onlyAfter=2023-01-01
				{Version: "2.0.0", LastUpdated: dateToTimestamp("2023-06-15")}, // Included by onlyBefore=2023-07-01, onlyAfter=2023-05-01
				{Version: "3.0.0", LastUpdated: dateToTimestamp("2024-01-15")}, // Included by onlyAfter=2023-05-01
			},
			NumVersions: 3,
		},
		{
			Name: "PackageB",
			Versions: []searcher.PackageVersion{
				{Version: "1.0.0", LastUpdated: dateToTimestamp("2022-12-01")}, // Included by onlyBefore=2023-07-01
			},
			NumVersions: 1,
		},
		{
			Name: "PackageC", // For testing empty results after filtering
			Versions: []searcher.PackageVersion{
				{Version: "5.0.0", LastUpdated: dateToTimestamp("2021-01-01")},
			},
			NumVersions: 1,
		},
	},
	NumResults: 3, // Total packages before any client-side filtering
}

func testPrintSearchResultsHelper(t *testing.T, results *searcher.SearchResults, query string, showAll bool, onlyBefore, onlyAfter string) string {
	t.Helper()
	var buf bytes.Buffer
	err := printSearchResults(&buf, query, results, showAll, onlyBefore, onlyAfter)
	if err != nil {
		t.Fatalf("printSearchResults returned an error: %v", err)
	}
	return buf.String()
}

func TestPrintSearchResults_NoFilters(t *testing.T) {
	output := testPrintSearchResultsHelper(t, mockResults, "myquery", true, "", "")

	if !strings.Contains(output, "PackageA") || !strings.Contains(output, "1.0.0") || !strings.Contains(output, "2.0.0") || !strings.Contains(output, "3.0.0") {
		t.Errorf("Expected PackageA with all versions, got: %s", output)
	}
	if !strings.Contains(output, "PackageB") || !strings.Contains(output, "1.0.0") {
		t.Errorf("Expected PackageB with its version, got: %s", output)
	}
	if !strings.Contains(output, "PackageC") || !strings.Contains(output, "5.0.0") {
		t.Errorf("Expected PackageC with its version, got: %s", output)
	}
	// Expecting 3 packages displayed as per mockResults post-filtering (no date filters applied)
	if !strings.Contains(output, fmt.Sprintf("Found %d results for \"myquery\"", 3)) {
		t.Errorf("Expected 'Found 3 results for \"myquery\"', got: %s", output)
	}
}

func TestPrintSearchResults_OnlyBefore(t *testing.T) {
	output := testPrintSearchResultsHelper(t, mockResults, "myquery", true, "2023-07-01", "") // Before July 1st, 2023

	// Expected:
	// PackageA: 1.0.0 (2023-01-15), 2.0.0 (2023-06-15)
	// PackageB: 1.0.0 (2022-12-01)
	// PackageC: 5.0.0 (2021-01-01)
	if !strings.Contains(output, "PackageA") || !strings.Contains(output, "1.0.0") || !strings.Contains(output, "2.0.0") {
		t.Errorf("Expected PackageA with versions 1.0.0, 2.0.0, got: %s", output)
	}
	if strings.Contains(output, "PackageA") && strings.Contains(output, "3.0.0") {
		t.Errorf("PackageA version 3.0.0 should be filtered out, got: %s", output)
	}
	if !strings.Contains(output, "PackageB") || !strings.Contains(output, "1.0.0") {
		t.Errorf("Expected PackageB with version 1.0.0, got: %s", output)
	}
	if !strings.Contains(output, "PackageC") || !strings.Contains(output, "5.0.0") {
		t.Errorf("Expected PackageC with version 5.0.0, got: %s", output)
	}
	// Expecting 3 packages because each still has at least one version matching.
	// The message format for filtered results: "Found X results for "query" (showing Y after date filtering from Z+ original)"
	// Here X = Y = 3, Z = mockResults.NumResults = 3.
	// So, "Found 3 results for "myquery" (showing 3 after date filtering from 3+ original)"
	// Or if X=Y and Z=X, it simplifies to: "Found 3 results for "myquery" (filtered by date)"
	if !strings.Contains(output, "Found 3 results for \"myquery\" (filtered by date)") {
		t.Errorf("Expected 'Found 3 results for \"myquery\" (filtered by date)', got: %s", output)
	}
}

func TestPrintSearchResults_OnlyAfter(t *testing.T) {
	output := testPrintSearchResultsHelper(t, mockResults, "myquery", true, "", "2023-05-01") // After May 1st, 2023

	// Expected:
	// PackageA: 2.0.0 (2023-06-15), 3.0.0 (2024-01-15)
	if !strings.Contains(output, "PackageA") || !strings.Contains(output, "2.0.0") || !strings.Contains(output, "3.0.0") {
		t.Errorf("Expected PackageA with versions 2.0.0, 3.0.0, got: %s", output)
	}
	if strings.Contains(output, "PackageA") && strings.Contains(output, "1.0.0") {
		t.Errorf("PackageA version 1.0.0 should be filtered out, got: %s", output)
	}
	if strings.Contains(output, "PackageB") {
		t.Errorf("PackageB should be filtered out, got: %s", output)
	}
	if strings.Contains(output, "PackageC") {
		t.Errorf("PackageC should be filtered out, got: %s", output)
	}
	// Expecting 1 package (PackageA)
	if !strings.Contains(output, "Found 1 results for \"myquery\" (showing 1 after date filtering from 3+ original)") {
		t.Errorf("Expected 'Found 1 results for \"myquery\" (showing 1 after date filtering from 3+ original)', got: %s", output)
	}
}

func TestPrintSearchResults_BeforeAndAfter(t *testing.T) {
	// Before Dec 31st, 2023 AND After Mar 1st, 2023
	output := testPrintSearchResultsHelper(t, mockResults, "myquery", true, "2023-12-31", "2023-03-01")

	// Expected:
	// PackageA: 2.0.0 (2023-06-15)
	if !strings.Contains(output, "PackageA") || !strings.Contains(output, "2.0.0") {
		t.Errorf("Expected PackageA with version 2.0.0, got: %s", output)
	}
	if strings.Contains(output, "PackageA") && (strings.Contains(output, "1.0.0") || strings.Contains(output, "3.0.0")) {
		t.Errorf("PackageA versions 1.0.0 and 3.0.0 should be filtered out, got: %s", output)
	}
	if strings.Contains(output, "PackageB") {
		t.Errorf("PackageB should be filtered out, got: %s", output)
	}
	if strings.Contains(output, "PackageC") {
		t.Errorf("PackageC should be filtered out, got: %s", output)
	}
	// Expecting 1 package (PackageA)
	if !strings.Contains(output, "Found 1 results for \"myquery\" (showing 1 after date filtering from 3+ original)") {
		t.Errorf("Expected 'Found 1 results for \"myquery\" (showing 1 after date filtering from 3+ original)', got: %s", output)
	}
}

func TestPrintSearchResults_NoMatches(t *testing.T) {
	output := testPrintSearchResultsHelper(t, mockResults, "myquery", true, "2022-01-01", "") // Before Jan 1st, 2022

	// Expected: PackageC (2021-01-01)
	if !strings.Contains(output, "PackageC") || !strings.Contains(output, "5.0.0") {
		t.Errorf("Expected PackageC with version 5.0.0, got: %s", output)
	}
	if strings.Contains(output, "PackageA") {
		t.Errorf("PackageA should be filtered out, got: %s", output)
	}
	if strings.Contains(output, "PackageB") {
		t.Errorf("PackageB should be filtered out, got: %s", output)
	}
	// Expecting 1 package (PackageC)
	if !strings.Contains(output, "Found 1 results for \"myquery\" (showing 1 after date filtering from 3+ original)") {
		t.Errorf("Expected 'Found 1 results for \"myquery\" (showing 1 after date filtering from 3+ original)', got: %s", output)
	}
}

func TestPrintSearchResults_NoMatchesStrict(t *testing.T) {
	// This will filter out Package C as well.
	output := testPrintSearchResultsHelper(t, mockResults, "myquery", true, "2021-01-01", "") // Before Jan 1st, 2021

	if strings.Contains(output, "PackageA") || strings.Contains(output, "PackageB") || strings.Contains(output, "PackageC") {
		t.Errorf("Expected no packages to be listed, got: %s", output)
	}
	// Expecting 0 packages.
	expectedMsg := "No results found for \"myquery\" with the specified date filters"
	if !strings.Contains(output, expectedMsg) {
		t.Errorf("Expected message '%s', got: %s", expectedMsg, output)
	}
}


func TestPrintSearchResults_EdgeCaseBefore(t *testing.T) {
	// Test with a date that is exactly one of the update dates.
	// onlyBefore means strictly before, so 2023-01-15 should be excluded.
	output := testPrintSearchResultsHelper(t, mockResults, "myquery", true, "2023-01-15", "")

	// Expected:
	// PackageB: 1.0.0 (2022-12-01)
	// PackageC: 5.0.0 (2021-01-01)
	if strings.Contains(output, "PackageA") && strings.Contains(output, "1.0.0") { // This version is on 2023-01-15
		t.Errorf("PackageA version 1.0.0 (updated on 2023-01-15) should be filtered out by onlyBefore=2023-01-15, got: %s", output)
	}
	if !strings.Contains(output, "PackageB") {
		t.Errorf("Expected PackageB, got: %s", output)
	}
	if !strings.Contains(output, "PackageC") {
		t.Errorf("Expected PackageC, got: %s", output)
	}
	// Expecting 2 packages (PackageB, PackageC)
	if !strings.Contains(output, "Found 2 results for \"myquery\" (showing 2 after date filtering from 3+ original)") {
		t.Errorf("Expected 'Found 2 results for \"myquery\" (showing 2 after date filtering from 3+ original)', got: %s", output)
	}
}

func TestPrintSearchResults_EdgeCaseAfter(t *testing.T) {
	// Test with a date that is exactly one of the update dates.
	// onlyAfter means strictly after, so 2023-06-15 should be excluded.
	output := testPrintSearchResultsHelper(t, mockResults, "myquery", true, "", "2023-06-15")

	// Expected:
	// PackageA: 3.0.0 (2024-01-15)
	if strings.Contains(output, "PackageA") && strings.Contains(output, "2.0.0") { // This version is on 2023-06-15
		t.Errorf("PackageA version 2.0.0 (updated on 2023-06-15) should be filtered out by onlyAfter=2023-06-15, got: %s", output)
	}
	if !strings.Contains(output, "PackageA") || !strings.Contains(output, "3.0.0") {
		t.Errorf("Expected PackageA with version 3.0.0, got: %s", output)
	}
	if strings.Contains(output, "PackageB") {
		t.Errorf("PackageB should be filtered out, got: %s", output)
	}
	if strings.Contains(output, "PackageC") {
		t.Errorf("PackageC should be filtered out, got: %s", output)
	}
	// Expecting 1 package (PackageA)
	if !strings.Contains(output, "Found 1 results for \"myquery\" (showing 1 after date filtering from 3+ original)") {
		t.Errorf("Expected 'Found 1 results for \"myquery\" (showing 1 after date filtering from 3+ original)', got: %s", output)
	}
}

// Test for showAll = false to ensure truncation messages are correct with filtering
func TestPrintSearchResults_WithFilteringAndTruncation(t *testing.T) {
	// Create a mock result with more than 10 versions for one package
	manyVersionsPkg := searcher.Package{
		Name: "PackageMany",
		Versions: []searcher.PackageVersion{},
		NumVersions: 12,
	}
	for i := 0; i < 12; i++ {
		// Versions spread across dates so some will be filtered
		dateStr := fmt.Sprintf("2023-%02d-01", i+1) // 2023-01-01 to 2023-12-01
		manyVersionsPkg.Versions = append(manyVersionsPkg.Versions, searcher.PackageVersion{
			Version: fmt.Sprintf("1.0.%d", i),
			LastUpdated: dateToTimestamp(dateStr),
		})
	}

	customMockResults := &searcher.SearchResults{
		Packages: []searcher.Package{
			manyVersionsPkg,
			mockResults.Packages[0], // PackageA
		},
		NumResults: 2,
	}

	// Filter: onlyAfter=2023-06-15 --> PackageMany versions 1.0.6 (July) to 1.0.11 (Dec) (6 versions)
	//                              --> PackageA version 3.0.0 (2024-01-15) (1 version)
	// showAll = false, so PackageMany should show its 6 versions (less than 10)
	// PackageA will show its 1 version.
	output := testPrintSearchResultsHelper(t, customMockResults, "myquery", false, "", "2023-06-15")

	if !strings.Contains(output, "PackageMany") {
		t.Errorf("Expected PackageMany, got: %s", output)
	}
	// PackageMany should have 6 versions after filtering: 1.0.6 .. 1.0.11
	// Check for a few of them
	if !strings.Contains(output, "1.0.6") || !strings.Contains(output, "1.0.11") {
		t.Errorf("Expected PackageMany versions 1.0.6 through 1.0.11, got: %s", output)
	}
	// Ensure it doesn't say "... (X more versions not shown)" for PackageMany because all 6 fit
	if strings.Contains(output, "PackageMany") && strings.Contains(output, "more versions not shown") {
		t.Errorf("PackageMany should not have a 'more versions' truncation message, got: %s", output)
	}


	if !strings.Contains(output, "PackageA") || !strings.Contains(output, "3.0.0") {
		t.Errorf("Expected PackageA with version 3.0.0, got: %s", output)
	}

	// Expected 2 packages displayed
	// NumResults from API = 2. After filtering, still 2 packages.
	// "Found 2 results for "myquery" (filtered by date)"
	if !strings.Contains(output, "Found 2 results for \"myquery\" (filtered by date)") {
		 t.Errorf("Expected 'Found 2 results for \"myquery\" (filtered by date)', got: %s", output)
	}

	// Now test with showAll = false and more than 10 versions *after* filtering for one package
	manyVersionsPkg2 := searcher.Package{
		Name: "PackageSuperMany",
		Versions: []searcher.PackageVersion{},
		NumVersions: 15, // total versions
	}
	for i := 0; i < 15; i++ {
		dateStr := fmt.Sprintf("2023-%02d-01", i+1) // 2023-01-01 to 2024-03-01 effectively
		if i >= 12 { // months 13, 14, 15
			dateStr = fmt.Sprintf("2024-%02d-01", i-12+1)
		}
		manyVersionsPkg2.Versions = append(manyVersionsPkg2.Versions, searcher.PackageVersion{
			Version: fmt.Sprintf("1.0.%d", i),
			LastUpdated: dateToTimestamp(dateStr),
		})
	}
	customMockResults2 := &searcher.SearchResults{
		Packages: []searcher.Package{ manyVersionsPkg2 },
		NumResults: 1,
	}

	// Filter: onlyAfter=2023-01-15 --> should leave 13 versions (Feb 2023 to Mar 2024)
	// showAll = false, so PackageSuperMany should show 10 versions and a truncation message.
	output2 := testPrintSearchResultsHelper(t, customMockResults2, "myquery", false, "", "2023-01-15")
	if !strings.Contains(output2, "PackageSuperMany") {
		t.Errorf("Expected PackageSuperMany, got: %s", output2)
	}
	// Check that it says "... X more versions" or similar, indicating truncation of versions for this package
	// The versions string should look like "(v1, v2, ..., v10 ...)"
	// The actual check for "..." is in the main code's lo.Ternary for ellipses.
	// The warning message for truncated versions is also important.
	if !strings.Contains(output2, "Some package versions are truncated. Use --show-all to show all versions.") {
		t.Errorf("Expected truncation warning for PackageSuperMany versions, got: %s", output2)
	}
	// Check that it lists 10 versions (e.g., 1.0.1 up to 1.0.10, if original versions are 1.0.0 to 1.0.14 and 1.0.0 is filtered out)
	// Filtered versions start from 1.0.1 (Feb 2023)
	if !strings.Contains(output2, "1.0.1,") || !strings.Contains(output2, "1.0.10") {
		t.Errorf("Expected PackageSuperMany to list versions up to 1.0.10, got %s", output2)
	}
	if strings.Contains(output2, "1.0.11") {
		t.Errorf("PackageSuperMany should not list version 1.0.11 due to truncation, got %s", output2)
	}
}

// It's good practice to also test the error return path of printSearchResults,
// for example, if date parsing fails (though this should be caught earlier by cobra).
// The current printSearchResults parses dates and returns usererr.New if it fails.
func TestPrintSearchResults_InvalidDateFormatInArgs(t *testing.T) {
	var buf bytes.Buffer
	err := printSearchResults(&buf, "myquery", mockResults, true, "invalid-date", "")
	if err == nil {
		t.Errorf("Expected an error for invalid --only-before date format, but got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "Invalid date format for --only-before") {
		t.Errorf("Expected error message about invalid date format, got: %v", err)
	}

	err = printSearchResults(&buf, "myquery", mockResults, true, "", "invalid-date")
	if err == nil {
		t.Errorf("Expected an error for invalid --only-after date format, but got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "Invalid date format for --only-after") {
		t.Errorf("Expected error message about invalid date format, got: %v", err)
	}
}Okay, I've created the `internal/boxcli/search_test.go` file with the initial set of tests.

This includes:
-   `dateToTimestamp` helper.
-   `mockResults` variable with sample package data.
-   `testPrintSearchResultsHelper` to streamline calling `printSearchResults` and capturing its output.
-   Test cases:
    -   `TestPrintSearchResults_NoFilters`: All data should be present.
    -   `TestPrintSearchResults_OnlyBefore`: Filters data before a specific date.
    -   `TestPrintSearchResults_OnlyAfter`: Filters data after a specific date.
    -   `TestPrintSearchResults_BeforeAndAfter`: Filters data between two dates.
    -   `TestPrintSearchResults_NoMatches`: A filter that results in one package.
    -   `TestPrintSearchResults_NoMatchesStrict`: A filter that results in zero packages, testing the "No results found ... with the specified date filters" message.
    -   `TestPrintSearchResults_EdgeCaseBefore`: Tests `onlyBefore` when the date is exactly a version's update date.
    -   `TestPrintSearchResults_EdgeCaseAfter`: Tests `onlyAfter` when the date is exactly a version's update date.
    -   `TestPrintSearchResults_WithFilteringAndTruncation`: Tests how `showAll=false` interacts with date-filtered results, ensuring version list truncation and package list truncation messages are correct.
    -   `TestPrintSearchResults_InvalidDateFormatInArgs`: Tests the error handling within `printSearchResults` if it receives unparseable date strings.

The assertions check for the presence/absence of package names and versions, and also verify the "Found X results..." summary line, including the specific messages for when filtering is applied.

I've tried to cover the main logic and some edge cases. The tests rely on `strings.Contains` for output verification.

One detail: The `NumVersions` field in the `mockResults` for each package reflects the total number of versions *before* any filtering. The `printSearchResults` function, when it filters versions, creates new `searcher.Package` structs where `NumVersions` is updated to the count of *filtered* versions. This is correctly handled in the tests by checking the output strings rather than the `NumVersions` field of the input `mockResults` directly after filtering.

The tests for truncation with `showAll=false` and date filtering also check that the correct truncation warnings appear.

The next step would be to run these tests. Since I can't run them here, I'll submit them as is.
