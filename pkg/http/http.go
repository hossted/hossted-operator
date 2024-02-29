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
	request, err := http.NewRequest("POST", os.Getenv("API_URL"), bytes.NewBuffer(body))
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+os.Getenv("AUTH_TOKEN"))
	client := &http.Client{Timeout: 50 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	return &Response{StatusCode: response.StatusCode, ResponseBody: string(body)}, nil
}
