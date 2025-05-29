package openai

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"sync"

	"github.com/joho/godotenv"
)

const OpenAIEndpoint = "https://api.openai.com/v1/chat/completions"

type OpenAIClient struct {
	APIKey string
	Model  string
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	Stream      bool          `json:"stream,omitempty"`
	Temperature float32       `json:"temperature,omitempty"`
}

type ChatResponse struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		Message ChatMessage `json:"message"`
	} `json:"choices"`
}

// ensures .env is only loaded once
var loadEnvOnce sync.Once

// NewClient loads env and returns a configured client
func NewClient(model string) *OpenAIClient {
	loadEnvOnce.Do(func() {
		_ = godotenv.Load()
	})

	return &OpenAIClient{
		APIKey: os.Getenv("OPENAI_API_KEY"),
		Model:  model,
	}
}

func (c *OpenAIClient) Chat(messages []ChatMessage) (string, error) {
	reqBody, _ := json.Marshal(ChatRequest{
		Model:    c.Model,
		Messages: messages,
	})
	req, _ := http.NewRequest("POST", OpenAIEndpoint, bytes.NewBuffer(reqBody))
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", errors.New(string(body))
	}

	var result ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.Choices[0].Message.Content, nil
}

func (c *OpenAIClient) StreamChat(messages []ChatMessage, onDelta func(string)) error {
	reqBody, _ := json.Marshal(ChatRequest{
		Model:    c.Model,
		Messages: messages,
		Stream:   true,
	})
	req, _ := http.NewRequest("POST", OpenAIEndpoint, bytes.NewBuffer(reqBody))
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return errors.New(string(body))
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || line == "data: [DONE]" {
			continue
		}
		var chunk ChatResponse
		if err := json.Unmarshal([]byte(line[6:]), &chunk); err != nil {
			continue
		}
		if len(chunk.Choices) > 0 {
			delta := chunk.Choices[0].Delta.Content
			onDelta(delta)
		}
	}
	return nil
}
