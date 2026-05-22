package main

import (
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	x402 "github.com/gurufinresearch/x402/go"
	x402http "github.com/gurufinresearch/x402/go/http"
	ginmw "github.com/gurufinresearch/x402/go/http/gin"
	cevmcore "github.com/gurufinresearch/x402/go/mechanisms/cevm"
	cevmserver "github.com/gurufinresearch/x402/go/mechanisms/cevm/exact/server"
	evmserver "github.com/gurufinresearch/x402/go/mechanisms/evm/exact/server"
	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()

	// Must be called before any cevm scheme is instantiated so that
	// NetworkConfigs is populated for ParsePrice / EnhancePaymentRequirements.
	registerCevmNetworkFromEnv()

	payeeAddress := os.Getenv("EVM_PAYEE_ADDRESS")
	if payeeAddress == "" {
		fmt.Println("EVM_PAYEE_ADDRESS environment variable is required")
		os.Exit(1)
	}

	facilitatorURL := os.Getenv("FACILITATOR_URL")
	if facilitatorURL == "" {
		fmt.Println("FACILITATOR_URL environment variable is required")
		os.Exit(1)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "4021"
	}

	// ── EVM networks (CAIP-2 eip155 format) ──────────────────────────────────
	networkEthSepolia := x402.Network("eip155:11155111")
	networkGuru := x402.Network("eip155:631")

	// ── CEVM / Cosmos network (configurable via X402_NETWORK) ────────────────
	networkStr := os.Getenv("X402_NETWORK")
	if networkStr == "" {
		networkStr = "cevm:9001"
	}
	networkCevm := x402.Network(networkStr)

	r := gin.Default()

	facilitator := x402http.NewHTTPFacilitatorClient(&x402http.FacilitatorConfig{
		URL: facilitatorURL,
	})

	// Each paid route accepts all three networks so clients on any chain can pay.
	paymentOptions := x402http.PaymentOptions{
		{Scheme: "exact", Price: "₩100", Network: networkEthSepolia, PayTo: payeeAddress},
		{Scheme: "exact", Price: "₩100", Network: networkGuru, PayTo: payeeAddress},
		{Scheme: "exact", Price: "₩100", Network: networkCevm, PayTo: payeeAddress},
	}

	routes := x402http.RoutesConfig{
		"GET /weather": {
			Accepts:     paymentOptions,
			Description: "Get weather data for a city",
			MimeType:    "application/json",
		},
		"GET /joke": {
			Accepts:     paymentOptions,
			Description: "Get a random joke",
			MimeType:    "application/json",
		},
		"GET /quote": {
			Accepts:     paymentOptions,
			Description: "Get an inspirational quote",
			MimeType:    "application/json",
		},
	}

	r.Use(ginmw.X402Payment(ginmw.Config{
		Routes:      routes,
		Facilitator: facilitator,
		Schemes: []ginmw.SchemeConfig{
			// EVM-compatible chains
			{Network: networkEthSepolia, Server: evmserver.NewExactEvmScheme()},
			{Network: networkGuru, Server: evmserver.NewExactEvmScheme()},
			// Cosmos EVM chain
			{Network: networkCevm, Server: cevmserver.NewExactCevmScheme()},
		},
		Timeout: 30 * time.Second,
	}))

	// ── Free endpoints ────────────────────────────────────────────────────────

	r.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"service": "x402 Test Website",
			"networks": []string{
				"ETH Sepolia (eip155:11155111)",
				"Gurufin Testnet EVM (eip155:631)",
				fmt.Sprintf("Gurufin Testnet Cosmos (%s)", networkCevm),
			},
			"payee": payeeAddress,
			"endpoints": []gin.H{
				{"path": "/weather?city=Tokyo", "cost": "100 KRGX", "method": "GET"},
				{"path": "/joke", "cost": "100 KRGX", "method": "GET"},
				{"path": "/quote", "cost": "100 KRGX", "method": "GET"},
				{"path": "/health", "cost": "free", "method": "GET"},
			},
		})
	})

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
			"time":   time.Now().Format(time.RFC3339),
			"networks": []string{
				string(networkEthSepolia),
				string(networkGuru),
				string(networkCevm),
			},
		})
	})

	// ── Paid endpoints ────────────────────────────────────────────────────────

	r.GET("/weather", func(c *gin.Context) {
		city := c.DefaultQuery("city", "San Francisco")

		weatherDB := map[string]gin.H{
			"San Francisco": {"condition": "foggy", "temp_f": 60, "humidity": 80},
			"New York":      {"condition": "cloudy", "temp_f": 55, "humidity": 65},
			"London":        {"condition": "rainy", "temp_f": 50, "humidity": 90},
			"Tokyo":         {"condition": "clear", "temp_f": 65, "humidity": 55},
			"Sydney":        {"condition": "sunny", "temp_f": 78, "humidity": 45},
		}

		data, exists := weatherDB[city]
		if !exists {
			data = gin.H{"condition": "sunny", "temp_f": 72, "humidity": 50}
		}

		c.JSON(http.StatusOK, gin.H{
			"city":      city,
			"weather":   data,
			"timestamp": time.Now().Format(time.RFC3339),
		})
	})

	r.GET("/joke", func(c *gin.Context) {
		jokes := []gin.H{
			{"setup": "Why do programmers prefer dark mode?", "punchline": "Because light attracts bugs."},
			{"setup": "Why did the blockchain developer quit?", "punchline": "He lost his private key to success."},
			{"setup": "What's a crypto trader's favorite meal?", "punchline": "A dip."},
			{"setup": "Why was the smart contract lonely?", "punchline": "It had no friends, only functions."},
			{"setup": "How does Ethereum say goodbye?", "punchline": "Gas you later!"},
		}

		c.JSON(http.StatusOK, gin.H{
			"joke":      jokes[rand.Intn(len(jokes))],
			"timestamp": time.Now().Format(time.RFC3339),
		})
	})

	r.GET("/quote", func(c *gin.Context) {
		quotes := []gin.H{
			{"text": "The best way to predict the future is to invent it.", "author": "Alan Kay"},
			{"text": "Code is like humor. When you have to explain it, it's bad.", "author": "Cory House"},
			{"text": "First, solve the problem. Then, write the code.", "author": "John Johnson"},
			{"text": "In the middle of difficulty lies opportunity.", "author": "Albert Einstein"},
			{"text": "The only way to do great work is to love what you do.", "author": "Steve Jobs"},
		}

		c.JSON(http.StatusOK, gin.H{
			"quote":     quotes[rand.Intn(len(quotes))],
			"timestamp": time.Now().Format(time.RFC3339),
		})
	})

	fmt.Printf("x402 Test Website starting on port %s\n", port)
	fmt.Printf("  Networks:    %s (ETH Sepolia), %s (Gurufin EVM), %s (Gurufin Cosmos)\n",
		networkEthSepolia, networkGuru, networkCevm)
	fmt.Printf("  Payee:       %s\n", payeeAddress)
	fmt.Printf("  Facilitator: %s\n", facilitatorURL)
	fmt.Printf("  Endpoints:   /weather, /joke, /quote (paid) | /, /health (free)\n\n")

	if err := r.Run(":" + port); err != nil {
		fmt.Printf("Failed to start server: %v\n", err)
		os.Exit(1)
	}
}

// registerCevmNetworkFromEnv populates cevmcore.NetworkConfigs for the Cosmos EVM scheme.
// Must be called before cevmserver.NewExactCevmScheme() is instantiated.
func registerCevmNetworkFromEnv() {
	net := strings.TrimSpace(os.Getenv("X402_NETWORK"))
	if net == "" {
		net = "cevm:9001"
	}
	denom := strings.TrimSpace(os.Getenv("CEVM_DENOM"))
	if denom == "" {
		denom = "atest"
	}
	name := strings.TrimSpace(os.Getenv("CEVM_DENOM_NAME"))
	if name == "" {
		name = "TEST"
	}
	wauth := strings.TrimSpace(os.Getenv("CEVM_WAUTH_CHAIN_ID"))
	if wauth == "" {
		if _, suffix, err := cevmcore.ParseCosmosEVMNetwork(net); err == nil {
			wauth = suffix
		}
	}
	cevmcore.NetworkConfigs[net] = cevmcore.NetworkConfig{
		DefaultAsset: cevmcore.AssetInfo{
			Denom: denom,
			Name:  name,
		},
		WauthChainID: wauth,
	}
}
