// Package token provides token counting and cost estimation for LLM requests.
package token

import (
	"fmt"
	"math"

	"github.com/tiktoken-go/tokenizer"

	"github.com/mindfulcto-labs/apiloom-ce/internal/ai"
)

// perMessageOverhead is the number of tokens added per message for role/separator
// per OpenAI tokenization documentation.
const perMessageOverhead = 4

// replyPriming is the number of tokens added for the assistant reply priming.
const replyPriming = 2

// codec is the shared cl100k_base tokenizer instance.
// cl100k_base covers GPT-4, GPT-3.5-turbo, and text-embedding-ada-002.
var codec tokenizer.Codec

func init() {
	var err error
	codec, err = tokenizer.Get(tokenizer.Cl100kBase)
	if err != nil {
		panic(fmt.Sprintf("initializing tiktoken cl100k_base: %v", err))
	}
}

// tiktokenCount returns the token count for a single text string using cl100k_base.
func tiktokenCount(text string) (int64, error) {
	if text == "" {
		return 0, nil
	}

	ids, _, err := codec.Encode(text)
	if err != nil {
		// Fall back to character estimation on encoding error.
		return charEstimate(text), nil
	}

	return int64(len(ids)), nil
}

// tiktokenCountMessages counts tokens for a slice of ChatMessages using tiktoken.
// Adds per-message overhead (4 tokens each) and reply priming (2 tokens).
func tiktokenCountMessages(messages []ai.ChatMessage) (int64, error) {
	if len(messages) == 0 {
		return 0, nil
	}

	var total int64
	for _, msg := range messages {
		total += perMessageOverhead // role + separators

		content := extractContent(msg.Content)
		count, err := tiktokenCount(content)
		if err != nil {
			return 0, fmt.Errorf("counting tokens for message: %w", err)
		}
		total += count

		if msg.Name != "" {
			nameCount, err := tiktokenCount(msg.Name)
			if err != nil {
				return 0, fmt.Errorf("counting tokens for name: %w", err)
			}
			total += nameCount
		}
	}

	total += replyPriming

	return total, nil
}

// charEstimate returns ceil(len(text) / 4.0) as a rough token estimate.
func charEstimate(text string) int64 {
	if text == "" {
		return 0
	}
	return int64(math.Ceil(float64(len(text)) / 4.0))
}

// extractContent converts Content (which can be string or other types) to a string.
func extractContent(content any) string {
	if content == nil {
		return ""
	}
	if s, ok := content.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", content)
}
