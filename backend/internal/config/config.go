package config

import (
	"os"
	"strings"
	"time"
)

type Config struct {
	Port              string
	PublicBaseURL     string
	DataDir           string
	CPPAnalyzerURL    string
	MistralAPIKey     string
	MistralAPIURL     string
	MistralModel      string
	MistralJudgeModel string
	AliceAPIKey       string
	AliceAPIURL       string
	AliceFolderID     string
	AliceModelURI     string
	AliceModel        string
	RequestTimeout    time.Duration
}

func Load() Config {
	return Config{
		Port:              env("PORT", "8080"),
		PublicBaseURL:     env("PUBLIC_BASE_URL", "http://localhost:8080"),
		DataDir:           env("DATA_DIR", "./data"),
		CPPAnalyzerURL:    strings.TrimRight(env("CPP_ANALYZER_URL", ""), "/"),
		MistralAPIKey:     env("MISTRAL_API_KEY", ""),
		MistralAPIURL:     env("MISTRAL_API_URL", "https://api.mistral.ai/v1/chat/completions"),
		MistralModel:      env("MISTRAL_MODEL", "mistral-small-latest"),
		MistralJudgeModel: env("MISTRAL_JUDGE_MODEL", env("MISTRAL_MODEL", "mistral-small-latest")),
		AliceAPIKey:       env("ALICE_API_KEY", ""),
		AliceAPIURL:       env("ALICE_API_URL", "https://llm.api.cloud.yandex.net/foundationModels/v1/completion"),
		AliceFolderID:     env("ALICE_FOLDER_ID", ""),
		AliceModelURI:     env("ALICE_MODEL_URI", ""),
		AliceModel:        env("ALICE_MODEL", "yandexgpt-lite"),
		RequestTimeout:    55 * time.Second,
	}
}

func env(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}
