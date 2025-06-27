package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"google.golang.org/genai"

	"github.com/joho/godotenv"
)

func cacheStream(ctx context.Context) {
	apiKey := os.Getenv("GOOGLE_GEMINI_API_KEY")
	fmt.Println("GOOGLE_GEMINI_API_KEY:", apiKey)
	if apiKey == "" {
		fmt.Println("GOOGLE_GEMINI_API_KEY environment variable is not set")
		return
	}

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		Backend: genai.BackendVertexAI,
		APIKey:  apiKey})

	if err != nil {
		log.Fatal(err)
	}
	if client.ClientConfig().Backend == genai.BackendVertexAI {
		fmt.Println("Calling VertexAI Backend...")
	} else {
		fmt.Println("Calling GeminiAPI Backend...")
	}

	fmt.Println("Iterating over the cached contents...")
	fmt.Println("Option 1: using the All function.")
	for item, err := range client.Caches.All(ctx) {
		if err != nil {
			log.Fatal(err)
		}
		print(item)
	}

	fmt.Println("Option 2: using the List function.")
	// Example 2.1 - List the first page.
	page, _ := client.Caches.List(ctx, &genai.ListCachedContentsConfig{PageSize: 2})
	// Example 2.2 - Continue to the next page.
	page, _ = page.Next(ctx)
	// Example 2.3 - Resume the page iteration using the next page token.
	page, err = client.Caches.List(ctx, &genai.ListCachedContentsConfig{PageSize: 2, PageToken: page.NextPageToken})
	if err == genai.ErrPageDone {
		fmt.Println("No more cached content to retrieve.")
		return
	}
	if err != nil {
		log.Fatal(err)
	}
	print(page.Items)
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Println("Warning: .env file not found or error loading:", err)
	}
	ctx := context.Background()
	flag.Parse()
	//chatStream(ctx)
	cacheStream(ctx)
}
