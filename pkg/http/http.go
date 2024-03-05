package http

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"time"
)

type Response struct {
	StatusCode   int
	ResponseBody string
}

func HttpRequest(body []byte) (*Response, error) {
	// Create a new HTTP request
	request, err := http.NewRequest("POST", os.Getenv("HOSSTED_API_URL"), bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %v", err)
	}

	// Set request headers
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Basic "+os.Getenv("HOSSTED_AUTH_TOKEN"))

	// Create an HTTP client with timeout
	client := &http.Client{Timeout: 50 * time.Second}

	// Send the HTTP request
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("failed to send HTTP request: %v", err)
	}
	defer response.Body.Close()

	// Read the response body
	responseBody := new(bytes.Buffer)
	_, err = responseBody.ReadFrom(response.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	// Create and return the Response object
	return &Response{
		StatusCode:   response.StatusCode,
		ResponseBody: responseBody.String(),
	}, nil
}
