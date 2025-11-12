package ai

import (
	"context"
	"fmt"
	"log"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

type Service struct {
	*genai.Client
}

func NewService(apiKey string) (*Service, error) {
	ctx := context.Background()
	if apiKey == "" {
		return nil, fmt.Errorf("no api key provided")
	}

	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("failed to create genai client: %w", err)
	}

	return &Service{client}, nil
}

// Ask sends a prompt to the Gemini API and returns the response.
func (s *Service) Ask(prompt string) (string, error) {
	ctx := context.Background()

	model := s.Client.GenerativeModel("gemini-2.0-flash")
	resp, err := model.GenerateContent(ctx, genai.Text("Answer this question, keeping the response to under 1500 characters: "+prompt))
	if err != nil {
		return "", fmt.Errorf("failed to generate content: %w", err)
	}

	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return "I'm sorry, I couldn't generate a response.", nil
	}

	var responseText string
	for _, part := range resp.Candidates[0].Content.Parts {
		if txt, ok := part.(genai.Text); ok {
			responseText += string(txt)
		}
	}

	if responseText == "" {
		log.Printf("No text part in response: %+v", resp)
		return "I'm sorry, I received an empty response.", nil
	}

	return responseText, nil
}
