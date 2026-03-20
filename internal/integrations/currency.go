package integrations

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	exchangeRateURL = "https://api.exchangerate-api.com/v4/latest/USD"
	coingeckoURL    = "https://api.coingecko.com/api/v3/simple/price?ids=bitcoin&vs_currencies=usd"
)

// exchangeRateResponse represents the JSON structure from exchangerate-api.
type exchangeRateResponse struct {
	Rates map[string]float64 `json:"rates"`
}

// coingeckoResponse represents the JSON structure from CoinGecko.
type coingeckoResponse struct {
	Bitcoin struct {
		USD float64 `json:"usd"`
	} `json:"bitcoin"`
}

// GetRates returns formatted currency rates: USD/RUB, EUR/RUB, and BTC/USD.
func GetRates() (string, error) {
	client := &http.Client{Timeout: 5 * time.Second}

	usdRub, eurRub, fiatErr := fetchFiatRates(client)
	btcUsd, btcErr := fetchBTCRate(client)

	// If both fail, return error
	if fiatErr != nil && btcErr != nil {
		return "", fmt.Errorf("all rate APIs failed: fiat=%w, btc=%v", fiatErr, btcErr)
	}

	var result string
	if fiatErr == nil {
		// Calculate EUR/RUB: RUB per 1 EUR = (RUB per 1 USD) / (EUR per 1 USD)
		eurRubRate := usdRub / eurRub
		result += fmt.Sprintf("USD/RUB: %.1f | EUR/RUB: %.1f", usdRub, eurRubRate)
	} else {
		result += "USD/RUB: n/a | EUR/RUB: n/a"
	}

	if btcErr == nil {
		result += fmt.Sprintf(" | BTC: $%s", formatBTC(btcUsd))
	} else {
		result += " | BTC: n/a"
	}

	return result, nil
}

// fetchFiatRates returns USD/RUB rate and EUR/USD rate from exchangerate-api.
func fetchFiatRates(client *http.Client) (usdRub, eurUsd float64, err error) {
	resp, err := client.Get(exchangeRateURL)
	if err != nil {
		return 0, 0, fmt.Errorf("fetch exchange rates: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, 0, fmt.Errorf("exchange rate API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, 0, fmt.Errorf("read exchange rate response: %w", err)
	}

	var data exchangeRateResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return 0, 0, fmt.Errorf("parse exchange rate JSON: %w", err)
	}

	rubRate, ok := data.Rates["RUB"]
	if !ok {
		return 0, 0, fmt.Errorf("RUB rate not found in response")
	}

	eurRate, ok := data.Rates["EUR"]
	if !ok {
		return 0, 0, fmt.Errorf("EUR rate not found in response")
	}

	return rubRate, eurRate, nil
}

// fetchBTCRate returns the current BTC price in USD from CoinGecko.
func fetchBTCRate(client *http.Client) (float64, error) {
	resp, err := client.Get(coingeckoURL)
	if err != nil {
		return 0, fmt.Errorf("fetch BTC rate: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("coingecko API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("read BTC response: %w", err)
	}

	var data coingeckoResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return 0, fmt.Errorf("parse BTC JSON: %w", err)
	}

	if data.Bitcoin.USD == 0 {
		return 0, fmt.Errorf("BTC price is zero or missing")
	}

	return data.Bitcoin.USD, nil
}

// formatBTC formats BTC price with comma separators, e.g. "67,420".
func formatBTC(price float64) string {
	whole := int64(price)
	s := fmt.Sprintf("%d", whole)

	// Insert commas from right to left
	n := len(s)
	if n <= 3 {
		return s
	}

	var result []byte
	for i, c := range s {
		if i > 0 && (n-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, byte(c))
	}
	return string(result)
}
