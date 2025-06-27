// Copyright 2024 Jetify Inc. and contributors. All rights reserved.
// Use of this source code is governed by the license in the LICENSE file.

package boxcli

import (
	"fmt"
	"io"
	"math"
	"strings"
	"time"

	"github.com/samber/lo"
	"github.com/spf13/cobra"

	"go.jetify.com/devbox/internal/boxcli/usererr"
	"go.jetify.com/devbox/internal/searcher"
	"go.jetify.com/devbox/internal/ux"
)

const trimmedVersionsLength = 10

type searchCmdFlags struct {
	showAll    bool
	onlyBefore string
	onlyAfter  string
}

func searchCmd() *cobra.Command {
	flags := &searchCmdFlags{}
	command := &cobra.Command{
		Use:   "search <pkg>",
		Short: "Search for nix packages",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := args[0]

			var onlyBefore, onlyAfter time.Time
			var err error
			if flags.onlyBefore != "" {
				onlyBefore, err = time.Parse("2006-01-02", flags.onlyBefore)
				if err != nil {
					return usererr.New("Invalid date format for --only-before. Please use YYYY-MM-DD.")
				}
			}
			if flags.onlyAfter != "" {
				onlyAfter, err = time.Parse("2006-01-02", flags.onlyAfter)
				if err != nil {
					return usererr.New("Invalid date format for --only-after. Please use YYYY-MM-DD.")
				}
			}

			name, version, isVersioned := searcher.ParseVersionedPackage(query)
			if !isVersioned {
				results, err := searcher.Client().Search(cmd.Context(), query, flags.onlyBefore, flags.onlyAfter)
				if err != nil {
					return err
				}
				return printSearchResults(
					cmd.OutOrStdout(), query, results, flags.showAll, flags.onlyBefore, flags.onlyAfter)
			}
			// TODO: Consider if date filtering should apply to Resolve as well.
			// For now, Resolve does not have date filtering parameters.
			packageVersion, err := searcher.Client().Resolve(name, version)
			if err != nil {
				// This is not ideal. Search service should return valid response we
				// can parse
				return usererr.WithUserMessage(err, "No results found for %q\n", query)
			}
			fmt.Fprintf(
				cmd.OutOrStdout(),
				"%s resolves to: %s@%s\n",
				query,
				packageVersion.Name,
				packageVersion.Version,
			)
			return nil
		},
	}

	command.Flags().BoolVar(
		&flags.showAll, "show-all", false,
		"show all available templates",
	)
	command.Flags().StringVar(
		&flags.onlyBefore, "only-before", "",
		"filter packages to include only those updated before the specified date (YYYY-MM-DD)",
	)
	command.Flags().StringVar(
		&flags.onlyAfter, "only-after", "",
		"filter packages to include only those updated after the specified date (YYYY-MM-DD)",
	)

	return command
}

func printSearchResults(
	w io.Writer,
	query string,
	results *searcher.SearchResults,
	showAll bool,
	onlyBeforeStr string, // YYYY-MM-DD
	onlyAfterStr string, // YYYY-MM-DD
) error {
	var onlyBeforeDate, onlyAfterDate time.Time
	var err error
	if onlyBeforeStr != "" {
		onlyBeforeDate, err = time.Parse("2006-01-02", onlyBeforeStr)
		if err != nil {
			// This error should ideally be caught in searchCmd, but defensive check here.
			return usererr.New("Invalid date format for --only-before. Please use YYYY-MM-DD.")
		}
	}
	if onlyAfterStr != "" {
		onlyAfterDate, err = time.Parse("2006-01-02", onlyAfterStr)
		if err != nil {
			// This error should ideally be caught in searchCmd, but defensive check here.
			return usererr.New("Invalid date format for --only-after. Please use YYYY-MM-DD.")
		}
		// To make the filter inclusive of the 'after' date, we can set the time to the end of the day.
		// However, the task asks for "updated AFTER the specified date".
		// So, if a package was updated on onlyAfterDate, it should be included.
		// To achieve "after <date>", we can make the comparison `updatedAtTime.After(onlyAfterDate)`.
		// But time.Parse results in a time at 00:00:00. So if a package was updated
		// on onlyAfterDate at 10:00:00, it would be included.
		// For "only after YYYY-MM-DD", we want items from YYYY-MM-DD + 1 day onwards.
		// So, we effectively check if updatedAtDate > specified onlyAfterDate.
	}

	filteredPackages := []searcher.Package{}
	if len(results.Packages) > 0 {
		for _, pkg := range results.Packages {
			filteredVersions := []searcher.PackageVersion{}
			for _, v := range pkg.Versions {
				updatedAtTime := time.Unix(int64(v.LastUpdated), 0)

				// Apply onlyBeforeDate filter
				if !onlyBeforeDate.IsZero() && !updatedAtTime.Before(onlyBeforeDate) {
					continue // Not before the specified date
				}

				// Apply onlyAfterDate filter
				// We want packages updated strictly *after* the onlyAfterDate.
				// So, if updatedAtTime is on onlyAfterDate, it's not included.
				if !onlyAfterDate.IsZero() && !updatedAtTime.After(onlyAfterDate) {
					continue // Not after the specified date
				}
				filteredVersions = append(filteredVersions, v)
			}

			if len(filteredVersions) > 0 {
				newPkg := searcher.Package{
					Name:        pkg.Name,
					NumVersions: len(filteredVersions), // Reflects the count of filtered versions
					Versions:    filteredVersions,
					Score:       pkg.Score, // Keep original score for now
				}
				filteredPackages = append(filteredPackages, newPkg)
			}
		}
	}

	if len(filteredPackages) == 0 {
		dateFilterMessage := ""
		if onlyBeforeStr != "" || onlyAfterStr != "" {
			dateFilterMessage = " with the specified date filters"
		}
		fmt.Fprintf(w, "No results found for %q%s\n", query, dateFilterMessage)
		return nil
	}

	foundResultsMsg := fmt.Sprintf("Found %d results for %q", len(filteredPackages), query)
	if onlyBeforeStr != "" || onlyAfterStr != "" {
		foundResultsMsg += " (filtered by date)"
	}
	if results.NumResults > len(filteredPackages) && (onlyBeforeStr != "" || onlyAfterStr != "") {
		// API might have returned more results that were then filtered out locally.
		// Or API itself might have filtered, in which case results.NumResults would be smaller.
		// For now, we assume `results.NumResults` is pre-API filtering.
		foundResultsMsg = fmt.Sprintf(
			"Found %d results for %q (showing %d after date filtering from %d+ original)",
			len(filteredPackages),
			query,
			len(filteredPackages),
			results.NumResults,
		)
	} else if results.NumResults > len(filteredPackages) && !(onlyBeforeStr != "" || onlyAfterStr != "") {
		// This case implies showAll=false might be truncating, even if no date filter.
		// The original message already handles NumResults being potentially larger.
		foundResultsMsg = fmt.Sprintf("Found %d+ results for %q (showing %d)", results.NumResults, query, len(filteredPackages))
	} else {
		foundResultsMsg = fmt.Sprintf("Found %d results for %q", len(filteredPackages), query)
		if onlyBeforeStr != "" || onlyAfterStr != "" {
			foundResultsMsg += " (filtered by date)"
		} else if results.NumResults > len(filteredPackages) {
			foundResultsMsg = fmt.Sprintf("Found %d+ results for %q (showing %d)", results.NumResults, query, len(filteredPackages))
		}
	}
	fmt.Fprintf(w, "%s:\n\n", foundResultsMsg)

	resultsAreTrimmed := false
	pkgsToDisplay := filteredPackages
	if !showAll && len(pkgsToDisplay) > trimmedVersionsLength {
		resultsAreTrimmed = true
		pkgsToDisplay = pkgsToDisplay[:int(math.Min(float64(trimmedVersionsLength), float64(len(pkgsToDisplay))))]
	}

	for _, pkg := range pkgsToDisplay {
		nonEmptyVersions := []string{}
		// Version trimming logic should apply to the versions *within* the pkgToDisplay
		// which are already filtered by date.
		versionsForDisplay := pkg.Versions
		localVersionTrimmed := false
		if !showAll && len(versionsForDisplay) > trimmedVersionsLength {
			localVersionTrimmed = true
			versionsForDisplay = versionsForDisplay[:trimmedVersionsLength]
		}

		for _, v := range versionsForDisplay {
			if v.Version != "" {
				nonEmptyVersions = append(nonEmptyVersions, v.Version)
			}
		}

		versionString := ""
		if len(nonEmptyVersions) > 0 {
			// The pkg.NumVersions for pkgsToDisplay now reflects filtered versions count.
			// So, compare with len(versionsForDisplay) which is max `trimmedVersionsLength` if not showAll.
			ellipses := ""
			if (!showAll && pkg.NumVersions > len(versionsForDisplay)) || (showAll && localVersionTrimmed) {
				ellipses = " ..."
			}

			if showAll {
				versionString = fmt.Sprintf("\n > %s \n", strings.Join(nonEmptyVersions, "\n > "))
				if ellipses != "" { // If showAll is true, ellipses means versions within this package were trimmed
					versionString += fmt.Sprintf(" > ... (%d more versions not shown)\n", pkg.NumVersions-len(nonEmptyVersions))
				}
			} else {
				versionString = fmt.Sprintf(" (%s%s)", strings.Join(nonEmptyVersions, ", "), ellipses)
			}
		}
		fmt.Fprintf(w, "* %s %s\n", pkg.Name, versionString)
	}

	if resultsAreTrimmed { // This means the list of packages was trimmed
		fmt.Println()
		ux.Fwarningf(
			w,
			"Showing top %d packages. Use --show-all to show all packages.\n\n",
			trimmedVersionsLength,
		)
	} else if !showAll { // Check if any individual package's version list was trimmed
		for _, pkg := range pkgsToDisplay {
			if pkg.NumVersions > trimmedVersionsLength {
				fmt.Println()
				ux.Fwarningf(
					w,
					"Some package versions are truncated. Use --show-all to show all versions.\n\n",
				)
				break // Show this warning once
			}
		}
	}
	return nil
}
