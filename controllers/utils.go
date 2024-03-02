package controllers

import (
	"sort"
	"time"
)

// filter removes namespaces from the list that match the denied namespaces.
func filter(namespaces []string, deniedNamespaces []string) []string {
	filteredNamespaces := make([]string, 0)

	for _, ns := range namespaces {
		// Check if the namespace is not in the denied namespaces list
		if !contains(deniedNamespaces, ns) {
			filteredNamespaces = append(filteredNamespaces, ns)
		}
	}

	return filteredNamespaces
}

// contains checks if a string exists in a slice of strings.
func contains(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}
	return false
}

// getCurrentTimeString returns the current time as a string using the specified layout.
func getCurrentTimeString() string {
	// Get the current time
	currentTime := time.Now()

	// Convert time to string using the specified layout
	timeString := currentTime.Format("2006-01-02 15:04:05")

	return timeString
}

func compareSlices(slice1, slice2 []int) bool {
	// Check if the slices have different lengths
	if len(slice1) != len(slice2) {
		return false
	}

	sort.Ints(slice1)
	sort.Ints(slice2)
	// Iterate over each element of the slices and compare them
	for i := range slice1 {
		if slice1[i] != slice2[i] {
			return false
		}
	}

	// If all elements are equal, return true
	return true
}
