package config

import (
	"fmt"

	"github.com/spf13/viper"
)

type Config struct {
	Env *Env
	App App
}

type Env struct {
	DatabaseURL string
	QueueURL string

	GPTApiKey string
	ClaudeApiKey string
	GroqApiKey string

	TavilyApiKey string
}

type App struct {
	AIPrimary      string
	AISecondary    string
	WorkerCount    int
	MaxStepRetries int
}

func LoadConfig() (*Config, error){
	config := &Config{}

	err := LoadEnv(config)
	if err != nil {
		return nil, err
	}

	err = LoadAppConfig(config)
	if err != nil {
		return nil, err
	}

	return config, nil
}

func LoadEnv(config *Config) error {
	viper.SetConfigName(".env")
    viper.SetConfigType("env")
    viper.AddConfigPath("internal/config")
	
	if err := viper.ReadInConfig(); err != nil {
        return fmt.Errorf("error reading .env file: %w", err)
    }

	config.Env = &Env{
		DatabaseURL: viper.GetString("DATABASE_URL"),
		QueueURL: viper.GetString("QUEUE_URL"),

		GPTApiKey: viper.GetString("GPT_API_KEY"),
		ClaudeApiKey: viper.GetString("CLAUDE_API_KEY"),
		GroqApiKey: viper.GetString("GROQ_API_Key"),

		TavilyApiKey: viper.GetString("TAVILY_API_KEY"),
	}

	if config.Env.DatabaseURL == "" {
		return fmt.Errorf("DATABASE_URL is not set")
	}
	return nil
}

func LoadAppConfig(config *Config) error {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("internal/config")

	if err := viper.ReadInConfig(); err != nil {
        return fmt.Errorf("error reading config file: %w", err)
    }

	config.App.AIPrimary = viper.GetString("ai_primary")
	config.App.AISecondary = viper.GetString("ai_secondary")
	config.App.WorkerCount = viper.GetInt("worker_count")
	config.App.MaxStepRetries = viper.GetInt("max_step_retries")
	
	return nil
}