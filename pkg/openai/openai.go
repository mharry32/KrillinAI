package openai

import (
	"context"
	"encoding/json"
	"fmt"
	openai "github.com/sashabaranov/go-openai"
	"go.uber.org/zap"
	"io"
	"krillin-ai/config"
	"krillin-ai/log"
	"net/http"
	"os"
	"strings"
)

func (c *Client) ChatCompletion(query string) (string, error) {
	var responseFormat *openai.ChatCompletionResponseFormat

	req := openai.ChatCompletionRequest{
		Model: config.Conf.Llm.Model,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: `You are a professional translator specializing in video subtitles for the Chinese market. Your task is to translate English subtitles into concise, natural, and high-impact simplified Chinese, suitable for a male health education channel on Bilibili.

- Tone: Professional, informative, educational. Use modern and engaging language while maintaining appropriate discretion for platform compliance.

- Style: Use short, clear sentences. Adapt cultural references and idioms to be easily understood by a Chinese audience.

- Content Guidelines for Sensitive Health Topics:
  * Use clinical/medical terminology consistently (e.g., "生理反应", "功能状态", "血液循环")
  * Employ educational framing language (e.g., "了解", "认识", "科普", "改善")
  * Replace explicit terms with professional alternatives commonly used in Chinese health education
  * For intensity descriptions, use neutral medical terms: "理想状态" instead of vivid metaphors
  * Maintain informational value while using platform-friendly vocabulary

- Strictly Avoid:
  * Colloquial metaphors or exaggerated comparisons (e.g., avoid translating phrases like "hard as steel" literally)
  * References to adult content or entertainment
  * Overly graphic descriptions or slang terms
  * Any language that could be flagged as sexual rather than educational

- Preferred Terminology Examples:
  * "erectile quality" → "勃起质量" (not "硬度表现")
  * "firm/hard" → "坚挺" or "充分勃起" (not "硬如XX")
  * "weak/soft" → "部分勃起" or "勃起不充分" (not vivid metaphors)
  * Intensity levels → "轻度/中度/完全勃起状态"

- Rules: Translate the user's input. Do not add any extra commentary, explanations, or text outside of the direct translation.`,
			},
			{
				Role:    openai.ChatMessageRoleUser,
				Content: query,
			},
		},
		Temperature:    0.9,
		Stream:         true,
		MaxTokens:      8192,
		ResponseFormat: responseFormat,
	}

	stream, err := c.client.CreateChatCompletionStream(context.Background(), req)
	if err != nil {
		log.GetLogger().Error("openai create chat completion stream failed", zap.Error(err))
		return "", err
	}
	defer stream.Close()

	var resContent string
	for {
		response, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.GetLogger().Error("openai stream receive failed", zap.Error(err))
			return "", err
		}
		if len(response.Choices) == 0 {
			log.GetLogger().Info("openai stream receive no choices", zap.Any("response", response))
			continue
		}

		resContent += response.Choices[0].Delta.Content
	}

	return resContent, nil
}

func (c *Client) Text2Speech(text, voice string, outputFile string) error {
	baseUrl := config.Conf.Tts.Openai.BaseUrl
	if baseUrl == "" {
		baseUrl = "https://api.openai.com/v1"
	}
	url := baseUrl + "/audio/speech"

	// 创建HTTP请求
	reqBody := fmt.Sprintf(`{
		"model": "tts-1",
		"input": "%s",
		"voice":"%s",
		"response_format": "wav"
	}`, text, voice)
	req, err := http.NewRequest("POST", url, strings.NewReader(reqBody))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", config.Conf.Tts.Openai.ApiKey))

	// 发送HTTP请求
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.GetLogger().Error("openai tts failed", zap.Int("status_code", resp.StatusCode), zap.String("body", string(body)))
		return fmt.Errorf("openai tts none-200 status code: %d", resp.StatusCode)
	}

	file, err := os.Create(outputFile)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return err
	}

	return nil
}

func parseJSONResponse(jsonStr string) (string, error) {
	var response struct {
		Translations []struct {
			Original   string `json:"original_sentence"`
			Translated string `json:"translated_sentence"`
		} `json:"translations"`
	}

	err := json.Unmarshal([]byte(jsonStr), &response)
	if err != nil {
		return "", fmt.Errorf("failed to parse JSON: %v", err)
	}

	var result strings.Builder
	for i, item := range response.Translations {
		result.WriteString(fmt.Sprintf("%d\n%s\n%s\n\n",
			i+1,
			item.Translated,
			item.Original))
	}

	return result.String(), nil
}
