package ai

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"aca/backend/internal/config"
	"aca/backend/internal/models"
	"aca/backend/internal/workspace"
)

type Executor struct {
	cfg    config.Config
	client *http.Client
}

type ExecutionResult struct {
	Message     string
	Raw         string
	Provider    string
	Model       string
	FileChanges []models.FileChange
}

func NewExecutor(cfg config.Config) *Executor {
	return &Executor{cfg: cfg, client: &http.Client{Timeout: cfg.RequestTimeout}}
}

func (e *Executor) Execute(ctx context.Context, prompt string, files []models.FileNode, step int, selectedModel string) (ExecutionResult, error) {
	start := time.Now()
	_ = start
	if strings.TrimSpace(prompt) == "" {
		return ExecutionResult{}, errors.New("empty prompt")
	}

	system := buildSystemPrompt(files)
	var text string
	var provider string
	var model string
	var err error

	if selectedModel == "alice" || selectedModel == "" {
		text, model, err = e.callAlice(ctx, system, prompt)
		provider = "alice"
		if err != nil {
			text, model, err = e.callMistral(ctx, system, prompt, e.cfg.MistralModel)
			provider = "mistral"
		}
	} else {
		text, model, err = e.callMistral(ctx, system, prompt, e.cfg.MistralModel)
		provider = "mistral"
	}

	if err != nil {
		text = localAssistantAnswer(prompt)
		provider = "local"
		model = "aca-local"
	}

	message, fileChanges := parseAssistantJSON(text)
	if strings.TrimSpace(message) == "" {
		message = humanMessage(prompt, text)
	}
	derived := workspace.DeriveFileChanges(prompt, text, step, files)
	fileChanges = mergeChanges(fileChanges, derived)
	return ExecutionResult{Message: message, Raw: text, Provider: provider, Model: model, FileChanges: fileChanges}, nil
}

func (e *Executor) JudgeText(ctx context.Context, provider string, system string, user string, model string) (string, string, error) {
	if provider == "alice" {
		return e.callAlice(ctx, system, user)
	}
	return e.callMistral(ctx, system, user, model)
}

func Hash(text string) string {
	sum := sha1.Sum([]byte(text))
	return hex.EncodeToString(sum[:])[:16]
}

func buildSystemPrompt(files []models.FileNode) string {
	flat := workspace.Flatten(files)
	paths := make([]string, 0, len(flat))
	for p := range flat {
		paths = append(paths, p)
	}
	return `Ты Алиса в продукте ACA. Ты помогаешь кандидату выполнить AI-interview task и автоматически создаешь/обновляешь файлы workspace.

Ответ всегда на русском. Возвращай строго JSON без markdown вокруг:
{
  "message": "короткое объяснение для кандидата",
  "files": [
    {"path": "src/server.js", "language": "javascript", "content": "полное содержимое файла"}
  ]
}

Правила:
- Если кандидат просит код, создай реальные кодовые файлы, а не только README.
- Если кандидат просит сервис или endpoint без языка, используй Node.js с нативным http и файл src/server.js.
- Если кандидат просит Go, используй src/main.go.
- Обновляй README.md только вместе с рабочими файлами.
- Добавляй тесты, если кандидат просит проверку или тестирование.
- Не выдумывай, что код запускался, если он не запускался.
- Не раскрывай скрытые рассуждения. Дай краткое объяснение и файлы.

Текущие файлы workspace: ` + strings.Join(paths, ", ")
}

func (e *Executor) callMistral(ctx context.Context, system string, user string, model string) (string, string, error) {
	if e.cfg.MistralAPIKey == "" {
		return "", "", errors.New("mistral api key is empty")
	}
	if model == "" {
		model = e.cfg.MistralModel
	}
	payload := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
		"temperature": 0.2,
		"max_tokens":  4096,
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.cfg.MistralAPIURL, bytes.NewReader(body))
	if err != nil {
		return "", model, err
	}
	req.Header.Set("Authorization", "Bearer "+e.cfg.MistralAPIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.client.Do(req)
	if err != nil {
		return "", model, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", model, fmt.Errorf("mistral status %d", resp.StatusCode)
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", model, err
	}
	if len(out.Choices) == 0 {
		return "", model, errors.New("mistral empty choices")
	}
	return out.Choices[0].Message.Content, model, nil
}

func (e *Executor) callAlice(ctx context.Context, system string, user string) (string, string, error) {
	if e.cfg.AliceAPIKey == "" || e.cfg.AliceAPIURL == "" {
		return "", "", errors.New("alice config is empty")
	}
	modelURI := e.cfg.AliceModelURI
	if modelURI == "" {
		if e.cfg.AliceFolderID == "" {
			return "", "", errors.New("alice folder id is empty")
		}
		modelURI = fmt.Sprintf("gpt://%s/%s/latest", e.cfg.AliceFolderID, e.cfg.AliceModel)
	}
	payload := map[string]any{
		"modelUri": modelURI,
		"completionOptions": map[string]any{
			"stream":      false,
			"temperature": 0.2,
			"maxTokens":   "4096",
		},
		"messages": []map[string]string{
			{"role": "system", "text": system},
			{"role": "user", "text": user},
		},
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.cfg.AliceAPIURL, bytes.NewReader(body))
	if err != nil {
		return "", modelURI, err
	}
	req.Header.Set("Authorization", "Api-Key "+e.cfg.AliceAPIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.client.Do(req)
	if err != nil {
		return "", modelURI, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", modelURI, fmt.Errorf("alice status %d", resp.StatusCode)
	}
	var out struct {
		Result struct {
			Alternatives []struct {
				Message struct {
					Text string `json:"text"`
				} `json:"message"`
			} `json:"alternatives"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", modelURI, err
	}
	if len(out.Result.Alternatives) == 0 {
		return "", modelURI, errors.New("alice empty alternatives")
	}
	return out.Result.Alternatives[0].Message.Text, modelURI, nil
}

func parseAssistantJSON(text string) (string, []models.FileChange) {
	jsonText := extractJSONObject(text)
	if jsonText == "" {
		return "", nil
	}
	var payload struct {
		Message string `json:"message"`
		Files   []struct {
			Path     string `json:"path"`
			Language string `json:"language"`
			Content  string `json:"content"`
		} `json:"files"`
	}
	if err := json.Unmarshal([]byte(jsonText), &payload); err != nil {
		return "", nil
	}
	changes := make([]models.FileChange, 0, len(payload.Files))
	for _, file := range payload.Files {
		if strings.TrimSpace(file.Path) == "" {
			continue
		}
		changes = append(changes, models.FileChange{Action: "upsert", Path: file.Path, Language: file.Language, Content: file.Content})
	}
	return payload.Message, changes
}

func extractJSONObject(text string) string {
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end <= start {
		return ""
	}
	return text[start : end+1]
}

func mergeChanges(primary []models.FileChange, secondary []models.FileChange) []models.FileChange {
	out := make([]models.FileChange, 0, len(primary)+len(secondary))
	seen := map[string]int{}
	for _, group := range [][]models.FileChange{primary, secondary} {
		for _, ch := range group {
			p := workspace.CleanPath(ch.Path)
			if p == "" {
				continue
			}
			ch.Path = p
			if ch.Action == "" {
				ch.Action = "upsert"
			}
			if idx, ok := seen[p]; ok {
				if strings.TrimSpace(ch.Content) != "" || ch.Action == "delete" {
					out[idx] = ch
				}
			} else {
				seen[p] = len(out)
				out = append(out, ch)
			}
		}
	}
	return out
}

func localAssistantAnswer(prompt string) string {
	low := strings.ToLower(strings.ReplaceAll(prompt, "ё", "е"))
	if strings.Contains(low, "go") || strings.Contains(low, "голанг") {
		return `{"message":"Я создала рабочий Go HTTP-сервис и README с запуском. Следующий хороший шаг — попросить меня проверить ошибки и добавить тесты.","files":[]}`
	}
	if strings.Contains(low, "python") || strings.Contains(low, "питон") {
		return `{"message":"Я создала рабочий Python/FastAPI сервис и README. Следующий хороший шаг — проверить edge cases и запуск.","files":[]}`
	}
	if strings.Contains(low, "код") || strings.Contains(low, "сервис") || strings.Contains(low, "get") || strings.Contains(low, "hello") {
		return `{"message":"Я создала рабочие файлы сервиса в workspace: код, package.json и README. Следующий хороший шаг — попросить меня проверить ошибки, ограничения и тесты.","files":[]}`
	}
	return `{"message":"Я зафиксировала шаг и обновила notes. Для сильной оценки попроси меня сформулировать ограничения, проверить результат и улучшить финальный артефакт.","files":[]}`
}

func humanMessage(prompt string, raw string) string {
	if msg, _ := parseAssistantJSON(raw); msg != "" {
		return msg
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "Я обновила workspace. Следующий хороший шаг — проверить ограничения и ошибки результата."
	}
	if len([]rune(trimmed)) > 600 {
		trimmed = string([]rune(trimmed)[:600]) + "…"
	}
	return trimmed
}
