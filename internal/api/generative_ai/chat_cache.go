package generativeAI

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"

	"google.golang.org/genai"
)

func print(r any) {
	// Marshal the result to JSON.
	response, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	// Log the output.
	fmt.Println(string(response))
}

func run(ctx context.Context) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{Backend: genai.BackendVertexAI})
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
	page, err := client.Caches.List(ctx, &genai.ListCachedContentsConfig{PageSize: 2})
	// Example 2.2 - Continue to the next page.
	page, err = page.Next(ctx)
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
	ctx := context.Background()
	flag.Parse()
	run(ctx)
}
