package controllers

import (
	"crypto/sha256"
	"encoding/hex"
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

func mapsEqual(m1, m2 map[string]interface{}) bool {
	if len(m1) != len(m2) {
		return false
	}
	for k, v1 := range m1 {
		v2, ok := m2[k]
		if !ok || !valuesEqual(v1, v2) {
			return false
		}
	}
	return true
}

func valuesEqual(v1, v2 interface{}) bool {
	switch t1 := v1.(type) {
	case map[string]interface{}:
		t2, ok := v2.(map[string]interface{})
		if !ok || !mapsEqual(t1, t2) {
			return false
		}
	case []interface{}:
		t2, ok := v2.([]interface{})
		if !ok || len(t1) != len(t2) {
			return false
		}
		for i := range t1 {
			if !valuesEqual(t1[i], t2[i]) {
				return false
			}
		}
	default:
		if v1 != v2 {
			return false
		}
	}
	return true
}

func computeSHA256(jsonStr string) string {
	hash := sha256.New()
	hash.Write([]byte(jsonStr))
	hashInBytes := hash.Sum(nil)
	return hex.EncodeToString(hashInBytes)
}
