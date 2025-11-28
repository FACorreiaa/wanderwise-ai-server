package utils

import (
	"regexp"
	"strings"
)

// CleanLLMResponse cleans and normalizes LLM response text by removing markdown code blocks,
// trimming whitespace, and fixing common JSON formatting issues.
// This is a simple version for basic cleaning.
func CleanLLMResponse(responseText string) string {
	// Trim leading/trailing whitespace
	cleaned := strings.TrimSpace(responseText)

	// Remove markdown code blocks if present
	if strings.HasPrefix(cleaned, "```json") {
		cleaned = strings.TrimPrefix(cleaned, "```json")
		cleaned = strings.TrimSuffix(cleaned, "```")
		cleaned = strings.TrimSpace(cleaned)
	} else if strings.HasPrefix(cleaned, "```") {
		cleaned = strings.TrimPrefix(cleaned, "```")
		cleaned = strings.TrimSuffix(cleaned, "```")
		cleaned = strings.TrimSpace(cleaned)
	}

	// Remove trailing commas before closing braces/brackets (common LLM error)
	cleaned = regexp.MustCompile(`,(\s*[}\]])`).ReplaceAllString(cleaned, "$1")

	return cleaned
}

// CleanJSONResponse performs advanced cleaning of LLM JSON responses.
// It handles markdown code blocks, extracts valid JSON by brace counting,
// and removes extraneous content before/after the JSON object.
// Use this for more robust JSON extraction from LLM responses.
func CleanJSONResponse(response string) string {
	response = strings.TrimSpace(response)

	// Remove markdown code block markers
	if strings.HasPrefix(response, "```json") {
		response = strings.TrimPrefix(response, "```json")
	} else if strings.HasPrefix(response, "```") {
		response = strings.TrimPrefix(response, "```")
	}

	response = strings.TrimSuffix(response, "```")
	response = strings.TrimSpace(response)

	firstBrace := strings.Index(response, "{")
	if firstBrace == -1 {
		return response // No JSON found, return as is
	}

	// Find the matching closing brace by counting braces
	braceCount := 0
	var lastValidBrace int
	for i := firstBrace; i < len(response); i++ {
		switch response[i] {
		case '{':
			braceCount++
		case '}':
			braceCount--
			if braceCount == 0 {
				lastValidBrace = i
				break
			}
		}
	}

	if braceCount != 0 {
		// Fallback to last brace method if brace counting fails
		lastBrace := strings.LastIndex(response, "}")
		if lastBrace == -1 || lastBrace <= firstBrace {
			return response // No valid JSON structure found
		}
		lastValidBrace = lastBrace
	}

	// Extract the JSON portion
	jsonPortion := response[firstBrace : lastValidBrace+1]

	// Remove any remaining backticks that might be within the JSON content
	// This handles cases where the AI includes markdown formatting within JSON strings
	jsonPortion = strings.ReplaceAll(jsonPortion, "`", "")

	// Remove trailing commas before closing braces/brackets (common LLM error)
	jsonPortion = regexp.MustCompile(`,(\s*[}\]])`).ReplaceAllString(jsonPortion, "$1")

	return strings.TrimSpace(jsonPortion)
}
