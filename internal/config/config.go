import (
	"config"
	_ "github.com/joho/godotenv"
	_ "github.com/spf13/viper"
)

type Config struct {
	Env *Env
}

type Env struct {
	DatabaseURL string
}

func LoadConfig() (*Config, error){
	config = &Config{}

	loadEnv, err := LoadEnv(config)
	if err != nil {
		return nil, err
	}
	return config, nil
}

func LoadEnv(config *Config) error {
	config.Env = &Env{
		DatabaseURL: os.Getenv("DATABASE_URL"),
	}
	if config.Env.DatabaseURL == "" {
		return fmt.Errorf("DATABASE_URL is not set")
	}
	return nil
}