package controllers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"time"
)

const (
	init_cluster_registeration        = "_HSTD_OPERATOR_CLUSTER_REGISTERATION"
	init_app_registeration            = "_HSTD_OPERATOR_APP_REGISTERATION"
	init_ingress_installation         = "_HSTD_OPERATOR_INGRESS_INSTALLATION"
	init_marketplace_app_installation = "_HSTD_OPERATOR_MARKETPLACE_APP_INSTALLATION"
	init_dns_registeration            = "_HSTD_OPERATOR_DNS_REGISTERATION"
	init_pod_info_collection          = "_HSTD_OPERATOR_POD_INFO_COLLECTION"
	init_service_info_collection      = "_HSTD_OPERATOR_SERVICE_INFO_COLLECTION"
	init_volume_info_collection       = "_HSTD_OPERATOR_VOLUME_INFO_COLLECTION"
	init_ingress_info_collection      = "_HSTD_OPERATOR_INGRESS_INFO_COLLECTION"
	init_security_info_collection     = "_HSTD_OPERATOR_SECURITY_INFO_COLLECTION"
	init_configmap_info_collection    = "_HSTD_OPERATOR_CONFIGMAP_INFO_COLLECTION"
	init_helmvalue_info_collection    = "_HSTD_OPERATOR_HELMVALUE_INFO_COLLECTION"
	init_deployment_info_collection   = "_HSTD_OPERATOR_DEPLOYMENT_INFO_COLLECTION"
	init_statefulset_info_collection  = "_HSTD_OPERATOR_STATEFULSET_INFO_COLLECTION"
	init_secret_info_collection       = "_HSTD_OPERATOR_SECRET_INFO_COLLECTION"
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

func sendEvent(eventType, message, orgID, clusterUUID string) error {
	url := os.Getenv("HOSSTED_API_URL") + "/statuses"

	type event struct {
		WareType string `json:"ware_type"`
		Type     string `json:"type"`
		UUID     string `json:"uuid,omitempty"`
		OrgID    string `json:"org_id"`
		UserID   string `json:"user_id,omitempty"`
		Message  string `json:"message"`
	}

	newEvent := event{
		WareType: "k8s",
		Type:     eventType,
		UUID:     clusterUUID,
		OrgID:    orgID,
		UserID:   os.Getenv("HOSSTED_USER_ID"),
		Message:  message,
	}

	eventByte, err := json.Marshal(newEvent)
	if err != nil {
		return err
	}
	// Create HTTP request
	req, err := http.NewRequest("POST", url, bytes.NewBuffer([]byte(eventByte)))
	if err != nil {
		return err
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")

	// Add Authorization header with Basic authentication
	req.Header.Set("Authorization", fmt.Sprintf("Basic %s", []byte(os.Getenv("HOSSTED_TOKEN"))))
	// Perform the request
	client := http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode != 200 {
		return fmt.Errorf("Error sending event, errcode: %d, resp %s", resp.StatusCode, string(body))
	}

	log.Printf("Event sent successfully: %s, response: %s\n", message, string(body))

	return nil
}
