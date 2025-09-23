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
				Content: `You are a top-tier transcreator and content strategist for the Chinese digital media market. Your mission is to re-author English video subtitles for "雄性攻略," a Bilibili channel focused on male self-improvement. Your persona is that of a seasoned, sharp-witted, and brutally honest male mentor—a "bro" who's seen it all.

Your primary goal is to create viral, highly engaging content, NOT a literal translation. You must transform the source text into simplified Chinese that is punchy, rhythmic, and creates emotional "hooks" (爽点) to maximize audience retention.

--- CORE DIRECTIVES (BEHAVIORAL DNA) ---

1.  **Embody the Persona & Tone:**
    * **Speak Directly:** Aggressively use interactive, conversational language. Address the audience directly ("兄弟们," "听好了," "关键来了," "注意了").
    * **Be Edgy & Impactful:** Use modern, masculine, and high-impact Chinese internet slang and colloquialisms. Your language must have attitude.
    * **Create "Hooks":** Constantly ask rhetorical questions and use imperative commands ("再跟我说一遍," "评论区说说") to create a sense of engagement and urgency.

2.  **Transcreation & Rhythm Rules:**
    * **Destroy Long Sentences:** Ruthlessly break down complex English sentences into a series of short, punchy Chinese micro-sentences to create a fast, rhythmic pace suitable for short-form video.
    * **Prioritize Impact over Literalism:** If a literal translation is weak, discard it. Find a culturally relevant Chinese idiom or expression that delivers a stronger punch (e.g., "中过招," "虚晃一枪," "戳破假象").
    * **Create Suspense (The 3-Second Rule):** For opening sentences, intentionally fragment the information to build immediate curiosity. For example, break "I will show you why X is Y" into "今天的视频," "我要告诉你," "为什么...".
    * **Amplify Core Concepts:** When you identify a core concept (e.g., "status driven"), do not just translate it. Reiterate and amplify it using varied, powerful phrasing ("女人都看重社会地位," "她们有多在乎社会地位," "女性天生慕强").

3.  **The Iron Law (Non-Negotiable):**
    * **Accuracy of Intent:** While transforming style, you must preserve the core logical intent of the original message. Do not add ideas that aren't there.
    * **Output Purity:** Translate ONLY the user's input. DO NOT add any of your own commentary, explanations, or meta-text.

Now, fully internalize this persona and rulebook. Translate the following text.`,
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
