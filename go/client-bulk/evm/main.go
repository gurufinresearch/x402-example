package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	x402 "github.com/gurufinresearch/x402/go"
	x402http "github.com/gurufinresearch/x402/go/http"
	evm "github.com/gurufinresearch/x402/go/mechanisms/evm/exact/client"
	evmsigners "github.com/gurufinresearch/x402/go/signers/evm"
	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()

	keysEnv := os.Getenv("EVM_PRIVATE_KEYS")
	if keysEnv == "" {
		fmt.Println("EVM_PRIVATE_KEYS environment variable is required")
		os.Exit(1)
	}

	privateKeys := strings.Split(keysEnv, ",")

	serverURL := os.Getenv("SERVER_URL")
	if serverURL == "" {
		serverURL = "http://localhost:4021"
	}

	fmt.Println("x402 Concurrent Payment Client")
	fmt.Printf("Server:   %s\n", serverURL)
	fmt.Printf("Accounts: %d\n\n", len(privateKeys))

	endpoints := []struct {
		path  string
		query string
	}{
		{"/weather", "?city=Tokyo"},
		{"/joke", ""},
		{"/quote", ""},
	}

	var wg sync.WaitGroup

	for i, key := range privateKeys {
		evmSigner, err := evmsigners.NewClientSignerFromPrivateKey(strings.TrimSpace(key))
		if err != nil {
			fmt.Printf("Signer creation failed for account %d: %v\n", i, err)
			continue
		}

		client := x402.Newx402Client()
		client.Register("eip155:631", evm.NewExactEvmScheme(evmSigner))

		httpWrapper := x402http.Newx402HTTPClient(client)
		httpClient := x402http.WrapHTTPClientWithPayment(&http.Client{}, httpWrapper)

		for _, ep := range endpoints {
			wg.Add(1)

			go func(account int, path string, query string, c *http.Client) {
				defer wg.Done()

				url := serverURL + path + query

				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()

				req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
				if err != nil {
					fmt.Printf("[acct %d] request creation failed: %v\n", account, err)
					return
				}

				resp, err := c.Do(req)
				if err != nil {
					fmt.Printf("[acct %d] request failed: %v\n", account, err)
					return
				}
				defer resp.Body.Close()

				var body interface{}
				json.NewDecoder(resp.Body).Decode(&body)

				prettyJSON, _ := json.MarshalIndent(body, "", "  ")

				fmt.Printf("\n[Account %d] GET %s%s\n", account, path, query)
				fmt.Printf("Status: %d\n", resp.StatusCode)
				fmt.Printf("Body:\n%s\n", string(prettyJSON))

				printPaymentReceipt(resp.Header)

			}(i, ep.path, ep.query, httpClient)
		}
	}

	wg.Wait()

	fmt.Println("\nAll requests finished")
}

func printPaymentReceipt(headers http.Header) {
	paymentHeader := headers.Get("PAYMENT-RESPONSE")
	if paymentHeader == "" {
		paymentHeader = headers.Get("X-PAYMENT-RESPONSE")
	}
	if paymentHeader == "" {
		return
	}

	decoded, err := base64.StdEncoding.DecodeString(paymentHeader)
	if err != nil {
		return
	}

	var settle x402.SettleResponse
	if err := json.Unmarshal(decoded, &settle); err != nil {
		return
	}

	fmt.Printf("Payment Receipt:\n")
	fmt.Printf("  Transaction: %s\n", settle.Transaction)
	fmt.Printf("  Network:     %s\n", settle.Network)
	fmt.Printf("  Payer:       %s\n", settle.Payer)
}
