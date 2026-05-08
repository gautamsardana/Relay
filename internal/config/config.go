package config

import (
	"fmt"

	"github.com/spf13/viper"
)

type Config struct {
	Env *Env
}

type Env struct {
	DatabaseURL string
	QueueURL string
}


func LoadConfig() (*Config, error){
	config := &Config{}

	err := LoadEnv(config)
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
	}

	if config.Env.DatabaseURL == "" {
		return fmt.Errorf("DATABASE_URL is not set")
	}
	return nil
}