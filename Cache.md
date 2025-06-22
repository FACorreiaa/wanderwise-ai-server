// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build ignore_vet

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"

	"google.golang.org/genai"
)

var model = flag.String("model", "gemini-1.5-pro-002", "the model name, e.g. gemini-1.5-pro-002")

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

// create get delete

// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build ignore_vet

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"time"

	"google.golang.org/genai"
)

var model = flag.String("model", "gemini-1.5-pro-002", "the model name, e.g. gemini-1.5-pro-002")

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
	// Create a cached content.
	result, err := client.Caches.Create(ctx, *model, &genai.CreateCachedContentConfig{
		TTL: 24 * time.Hour,
		Contents: []*genai.Content{
			{
				Role: "user",
				Parts: []*genai.Part{
					{
						FileData: &genai.FileData{
							MIMEType: "application/pdf",
							FileURI:  "gs://cloud-samples-data/generative-ai/pdf/2312.11805v3.pdf",
						},
					},
					{
						FileData: &genai.FileData{
							MIMEType: "application/pdf",
							FileURI:  "gs://cloud-samples-data/generative-ai/pdf/2312.11805v3.pdf",
						},
					},
				},
			},
		}})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Created cached content:")
	print(result)

	// Get the cached content.
	result, err = client.Caches.Get(ctx, result.Name, &genai.GetCachedContentConfig{})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Retrieved cached content:")
	print(result)

	// Update the cached content.
	result, err = client.Caches.Update(ctx, result.Name, &genai.UpdateCachedContentConfig{
		ExpireTime: time.Now().Add(time.Hour),
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Updated cached content:")
	print(result)

	// Delete the cached content.
	_, err = client.Caches.Delete(ctx, result.Name, &genai.DeleteCachedContentConfig{})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Deleted cached content:", result.Name)
}

func main() {
	ctx := context.Background()
	flag.Parse()
	run(ctx)
}