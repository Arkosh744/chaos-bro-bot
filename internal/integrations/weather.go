package integrations

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const weatherBaseURL = "https://wttr.in"

// GetWeather returns current weather for a city using wttr.in.
// The response is a human-readable string in Russian, e.g. "Облачно +5°C 45% ->5km/h".
func GetWeather(city string) (string, error) {
	if city == "" {
		return "", fmt.Errorf("city is empty")
	}

	encoded := url.PathEscape(city)
	reqURL := fmt.Sprintf("%s/%s?format=%%C+%%t+%%h+%%w&lang=ru", weatherBaseURL, encoded)

	client := &http.Client{Timeout: 5 * time.Second}

	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return "", fmt.Errorf("create weather request: %w", err)
	}
	req.Header.Set("User-Agent", "curl/7.0") // wttr.in requires curl-like UA for text output

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch weather for %s: %w", city, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("weather API returned status %d for %s", resp.StatusCode, city)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read weather response: %w", err)
	}

	result := strings.TrimSpace(string(body))
	if result == "" || strings.Contains(result, "Unknown location") {
		return "", fmt.Errorf("unknown city: %s", city)
	}

	return result, nil
}
