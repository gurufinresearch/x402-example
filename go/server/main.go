package main

import (
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	x402 "github.com/gurufinresearch/x402/go"
	x402http "github.com/gurufinresearch/x402/go/http"
	ginmw "github.com/gurufinresearch/x402/go/http/gin"
	evm "github.com/gurufinresearch/x402/go/mechanisms/evm/exact/server"
	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()

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

	// Network identifiers (CAIP-2 format)
	networkEthSepolia := x402.Network("eip155:11155111")
	networkGuru := x402.Network("eip155:631")

	r := gin.Default()

	facilitator := x402http.NewHTTPFacilitatorClient(&x402http.FacilitatorConfig{
		URL: facilitatorURL,
	})

	// Payment-protected routes — each costs ₩100 KRGX won (Gurufin Testnet) or ₩100 KRGX won (ETH Sepolia)
	routes := x402http.RoutesConfig{
		"GET /weather": {
			Accepts: x402http.PaymentOptions{
				{
					Scheme:  "exact",
					Price:   "₩100",
					Network: networkEthSepolia,
					PayTo:   payeeAddress,
				},
				{
					Scheme:  "exact",
					Price:   "₩100",
					Network: networkGuru,
					PayTo:   payeeAddress,
				},
			},
			Description: "Get weather data for a city",
			MimeType:    "application/json",
		},
		"GET /joke": {
			Accepts: x402http.PaymentOptions{
				{
					Scheme:  "exact",
					Price:   "₩100",
					Network: networkEthSepolia,
					PayTo:   payeeAddress,
				},
				{
					Scheme:  "exact",
					Price:   "₩100",
					Network: networkGuru,
					PayTo:   payeeAddress,
				},
			},
			Description: "Get a random joke",
			MimeType:    "application/json",
		},
		"GET /quote": {
			Accepts: x402http.PaymentOptions{
				{
					Scheme:  "exact",
					Price:   "₩100",
					Network: networkEthSepolia,
					PayTo:   payeeAddress,
				},
				{
					Scheme:  "exact",
					Price:   "₩100",
					Network: networkGuru,
					PayTo:   payeeAddress,
				},
			},
			Description: "Get an inspirational quote",
			MimeType:    "application/json",
		},
	}

	r.Use(ginmw.X402Payment(ginmw.Config{
		Routes:      routes,
		Facilitator: facilitator,
		Schemes: []ginmw.SchemeConfig{
			{Network: networkEthSepolia, Server: evm.NewExactEvmScheme()},
			{Network: networkGuru, Server: evm.NewExactEvmScheme()},
		},
		Timeout: 30 * time.Second,
	}))

	// --- Free endpoints ---

	r.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"service": "x402 Test Website",
			"networks": []string{
				"Gurufin Testnet (eip155:631)",
				"ETH Sepolia (eip155:11155111)",
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
			},
		})
	})

	// --- Paid endpoints ---

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

		joke := jokes[rand.Intn(len(jokes))]
		c.JSON(http.StatusOK, gin.H{
			"joke":      joke,
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

		quote := quotes[rand.Intn(len(quotes))]
		c.JSON(http.StatusOK, gin.H{
			"quote":     quote,
			"timestamp": time.Now().Format(time.RFC3339),
		})
	})

	fmt.Printf("x402 Test Website starting on port %s\n", port)
	fmt.Printf("  Networks:    %s (Gurufin Testnet), %s (ETH Sepolia)\n", networkGuru, networkEthSepolia)
	fmt.Printf("  Payee:       %s\n", payeeAddress)
	fmt.Printf("  Facilitator: %s\n", facilitatorURL)
	fmt.Printf("  Endpoints:   /weather, /joke, /quote (paid) | /, /health (free)\n\n")

	if err := r.Run(":" + port); err != nil {
		fmt.Printf("Failed to start server: %v\n", err)
		os.Exit(1)
	}
}
