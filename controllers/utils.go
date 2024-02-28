package controllers

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
