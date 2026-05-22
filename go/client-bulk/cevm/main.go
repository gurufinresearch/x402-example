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
	cevm "github.com/gurufinresearch/x402/go/mechanisms/cevm/exact/client"
	cevmsigners "github.com/gurufinresearch/x402/go/signers/cevm"
	"github.com/joho/godotenv"
)

const numSenders = 10

type endpointResult struct {
	Path   string
	Status int
	Err    error
	Body   interface{}
	Settle *x402.SettleResponse
}

type senderRun struct {
	Index   int
	Results []endpointResult
}

func main() {
	godotenv.Load()

	keys, err := loadPrivateKeys()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	serverURL := os.Getenv("SERVER_URL")
	if serverURL == "" {
		serverURL = "http://localhost:4021"
	}

	x402Network := strings.TrimSpace(os.Getenv("X402_NETWORK"))
	if x402Network == "" {
		x402Network = "cevm:9001"
	}

	bech32Prefix := strings.TrimSpace(os.Getenv("CEVM_BECH32_PREFIX"))
	if bech32Prefix == "" {
		bech32Prefix = "cosmos"
	}

	endpoints := []struct {
		path  string
		query string
	}{
		{"/weather", "?city=Tokyo"},
		{"/joke", ""},
		{"/quote", ""},
	}

	fmt.Println("x402 bulk client (concurrent senders)")
	fmt.Printf("  Server:  %s\n", serverURL)
	fmt.Printf("  Network: %s\n", x402Network)
	fmt.Printf("  Senders: %d (concurrent)\n\n", numSenders)

	ctx := context.Background()
	overall, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	out := make([]senderRun, numSenders)
	for i := 0; i < numSenders; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			out[i] = senderRun{
				Index:   i + 1,
				Results: runSender(overall, keys[i], bech32Prefix, x402Network, serverURL, endpoints),
			}
		}()
	}
	wg.Wait()

	for _, sr := range out {
		printSenderResult(sr)
	}
}

// loadPrivateKeys reads EVM_PRIVATE_KEY_1 … EVM_PRIVATE_KEY_10.
func loadPrivateKeys() ([]string, error) {
	keys := make([]string, numSenders)
	for i := 0; i < numSenders; i++ {
		name := fmt.Sprintf("EVM_PRIVATE_KEY_%d", i+1)
		k := strings.TrimSpace(os.Getenv(name))
		if k == "" {
			return nil, fmt.Errorf("%s is required (set %d hex keys for bulk test)", name, numSenders)
		}
		keys[i] = k
	}
	return keys, nil
}

func newPaymentHTTPClient(privateKey, bech32Prefix, x402Network string) (*http.Client, error) {
	cevmSigner, err := cevmsigners.NewClientSignerFromPrivateKey(privateKey, bech32Prefix)
	if err != nil {
		return nil, err
	}
	c := x402.Newx402Client()
	c.Register(x402.Network(x402Network), cevm.NewExactCevmScheme(cevmSigner))
	w := x402http.Newx402HTTPClient(c)

	// return x402http.WrapHTTPClientWithPayment(http.DefaultClient, w), nil

	// Use a dedicated *http.Client per sender — WrapHTTPClientWithPayment mutates
	// client.Transport. Sharing http.DefaultClient across goroutines leaves only the
	// last wrapper/signing key effective for all concurrent requests.
	base := &http.Client{Timeout: 90 * time.Second}
	return x402http.WrapHTTPClientWithPayment(base, w), nil
}

func runSender(
	ctx context.Context,
	privateKey, bech32Prefix, x402Network, baseURL string,
	endpoints []struct{ path, query string },
) []endpointResult {
	httpClient, err := newPaymentHTTPClient(privateKey, bech32Prefix, x402Network)
	if err != nil {
		return []endpointResult{{
			Path: "(init)",
			Err:  err,
		}}
	}

	out := make([]endpointResult, 0, len(endpoints))
	for _, ep := range endpoints {
		url := baseURL + ep.path + ep.query
		r := endpointResult{Path: ep.path + ep.query}

		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			r.Err = err
			out = append(out, r)
			continue
		}
		resp, err := httpClient.Do(req)
		if err != nil {
			r.Err = err
			out = append(out, r)
			continue
		}
		var body interface{}
		_ = json.NewDecoder(resp.Body).Decode(&body)
		resp.Body.Close()
		r.Status = resp.StatusCode
		r.Body = body
		r.Settle = parseSettleHeader(resp.Header)
		out = append(out, r)
	}
	return out
}

func parseSettleHeader(headers http.Header) *x402.SettleResponse {
	h := headers.Get("PAYMENT-RESPONSE")
	if h == "" {
		h = headers.Get("X-PAYMENT-RESPONSE")
	}
	if h == "" {
		return nil
	}
	raw, err := base64.StdEncoding.DecodeString(h)
	if err != nil {
		return nil
	}
	var settle x402.SettleResponse
	if err := json.Unmarshal(raw, &settle); err != nil {
		return nil
	}
	return &settle
}

func printSenderResult(sr senderRun) {
	fmt.Printf("======== Sender %d / %d ========\n", sr.Index, numSenders)
	for _, e := range sr.Results {
		if e.Path == "(init)" {
			fmt.Printf("  [init] error: %v\n", e.Err)
			continue
		}
		if e.Err != nil {
			fmt.Printf("  GET %s  error: %v\n", e.Path, e.Err)
			continue
		}
		pretty, _ := json.MarshalIndent(e.Body, "  ", "  ")
		fmt.Printf("  GET %s  status=%d\n", e.Path, e.Status)
		fmt.Printf("  body: %s\n", string(pretty))
		if e.Settle != nil {
			fmt.Printf("  payment: tx=%s network=%s payer=%s\n", e.Settle.Transaction, e.Settle.Network, e.Settle.Payer)
		}
	}
	fmt.Println()
}
