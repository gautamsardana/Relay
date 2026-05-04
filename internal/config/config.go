package config

import (
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/spf13/viper"
)

type Config struct {
	Env *Env
	Queue *amqp.Connection
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
	viper.AutomaticEnv()
	
	config.Env = &Env{
		DatabaseURL: viper.GetString("DATABASE_URL"),
		QueueURL: viper.GetString("QUEUE_URL"),
	}

	if config.Env.DatabaseURL == "" {
		return fmt.Errorf("DATABASE_URL is not set")
	}
	return nil
}