package main

import (
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"

	"github.com/joho/godotenv"
)

//go:embed templates/index.html
var templatesFS embed.FS

var serverURL string

func main() {
	godotenv.Load()

	serverURL = os.Getenv("SERVER_URL")
	if serverURL == "" {
		serverURL = "http://localhost:4021"
	}

	port := os.Getenv("WEB_PORT")
	if port == "" {
		port = "4023"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", handleIndex)
	mux.HandleFunc("/api/discover", handleDiscover)
	mux.HandleFunc("/api/pay", handlePay)

	fmt.Printf("x402 Web Demo (MetaMask) on http://localhost:%s\n", port)
	fmt.Printf("  Proxying to: %s\n\n", serverURL)

	if err := http.ListenAndServe(":"+port, mux); err != nil {
		fmt.Printf("Failed to start: %v\n", err)
		os.Exit(1)
	}
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	tmpl, err := template.ParseFS(templatesFS, "templates/index.html")
	if err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	tmpl.Execute(w, nil)
}

// handleDiscover proxies a request to the x402 server without payment.
// If the server returns 402, the decoded PAYMENT-REQUIRED header is returned
// so the browser can construct a MetaMask signature.
func handleDiscover(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		path = "/health"
	}

	resp, err := http.Get(serverURL + path)
	if err != nil {
		writeErr(w, err.Error())
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")

	if resp.StatusCode == http.StatusPaymentRequired {
		paymentB64 := resp.Header.Get("PAYMENT-REQUIRED")
		if paymentB64 == "" {
			paymentB64 = resp.Header.Get("X-PAYMENT-REQUIRED")
		}
		if paymentB64 != "" {
			decoded, err := base64.StdEncoding.DecodeString(paymentB64)
			if err == nil {
				json.NewEncoder(w).Encode(json.RawMessage(decoded))
				return
			}
		}
		// Fallback: return the 402 body as-is
		body, _ := io.ReadAll(resp.Body)
		var parsed interface{}
		if json.Unmarshal(body, &parsed) == nil {
			json.NewEncoder(w).Encode(parsed)
			return
		}
		writeErr(w, "could not parse payment requirements")
		return
	}

	// Non-402: forward body as-is
	body, _ := io.ReadAll(resp.Body)
	var data interface{}
	if json.Unmarshal(body, &data) == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": resp.StatusCode,
			"data":   data,
		})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"status": resp.StatusCode})
}

// handlePay receives a browser-signed payment payload and forwards it to
// the x402 server via the PAYMENT-SIGNATURE header.
func handlePay(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Path           string          `json:"path"`
		PaymentPayload json.RawMessage `json:"paymentPayload"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, "invalid request body")
		return
	}

	payloadB64 := base64.StdEncoding.EncodeToString(req.PaymentPayload)

	httpReq, err := http.NewRequest("GET", serverURL+req.Path, nil)
	if err != nil {
		writeErr(w, err.Error())
		return
	}
	httpReq.Header.Set("PAYMENT-SIGNATURE", payloadB64)

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		writeErr(w, err.Error())
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	result := map[string]interface{}{
		"status": resp.StatusCode,
	}

	var data interface{}
	if json.Unmarshal(body, &data) == nil {
		result["data"] = data
	}

	// Extract payment settlement receipt
	paymentHeader := resp.Header.Get("PAYMENT-RESPONSE")
	if paymentHeader == "" {
		paymentHeader = resp.Header.Get("X-PAYMENT-RESPONSE")
	}
	if paymentHeader != "" {
		if decoded, err := base64.StdEncoding.DecodeString(paymentHeader); err == nil {
			var settle interface{}
			if json.Unmarshal(decoded, &settle) == nil {
				result["payment"] = settle
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func writeErr(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
