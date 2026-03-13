package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	x402 "github.com/gurufinresearch/x402/go"
	x402http "github.com/gurufinresearch/x402/go/http"
	evm "github.com/gurufinresearch/x402/go/mechanisms/evm/exact/client"
	evmsigners "github.com/gurufinresearch/x402/go/signers/evm"
	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()

	privateKey := os.Getenv("EVM_PRIVATE_KEY")
	if privateKey == "" {
		fmt.Println("EVM_PRIVATE_KEY environment variable is required")
		os.Exit(1)
	}

	serverURL := os.Getenv("SERVER_URL")
	if serverURL == "" {
		serverURL = "http://localhost:4021"
	}

	// Create EVM signer from private key
	evmSigner, err := evmsigners.NewClientSignerFromPrivateKey(privateKey)
	if err != nil {
		fmt.Printf("Failed to create signer: %v\n", err)
		os.Exit(1)
	}

	// Create x402 client and register EVM scheme for Gurufin Testnet
	client := x402.Newx402Client()
	client.Register("eip155:631", evm.NewExactEvmScheme(evmSigner))

	// Wrap standard HTTP client with automatic x402 payment handling
	httpWrapper := x402http.Newx402HTTPClient(client)
	httpClient := x402http.WrapHTTPClientWithPayment(http.DefaultClient, httpWrapper)

	fmt.Println("x402 Payment Client")
	fmt.Printf("  Server:  %s\n", serverURL)
	fmt.Printf("  Network: Gurufin Testnet (eip155:631)\n\n")

	// Test all paid endpoints
	endpoints := []struct {
		path  string
		query string
	}{
		{"/weather", "?city=Tokyo"},
		{"/joke", ""},
		{"/quote", ""},
	}

	for _, ep := range endpoints {
		url := serverURL + ep.path + ep.query
		fmt.Printf("=== GET %s ===\n", ep.path+ep.query)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			cancel()
			fmt.Printf("  Failed to create request: %v\n\n", err)
			continue
		}

		resp, err := httpClient.Do(req)
		cancel()
		if err != nil {
			fmt.Printf("  Request failed: %v\n\n", err)
			continue
		}

		var body interface{}
		json.NewDecoder(resp.Body).Decode(&body)
		resp.Body.Close()

		prettyJSON, _ := json.MarshalIndent(body, "  ", "  ")
		fmt.Printf("  Status: %d\n", resp.StatusCode)
		fmt.Printf("  Body:\n  %s\n", string(prettyJSON))

		printPaymentReceipt(resp.Header)
		fmt.Println()
	}
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

	fmt.Printf("  Payment Receipt:\n")
	fmt.Printf("    Transaction: %s\n", settle.Transaction)
	fmt.Printf("    Network:     %s\n", settle.Network)
	fmt.Printf("    Payer:       %s\n", settle.Payer)
}
