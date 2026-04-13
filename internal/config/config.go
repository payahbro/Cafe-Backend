package config

import (
	"fmt"

	env "github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

type Config struct {
	App      AppConfig
	HTTP     HTTPConfig
	Log      LogConfig
	Redis    RedisConfig
	Database DatabaseConfig
	Internal InternalConfig
}

type AppConfig struct {
	Name                   string `env:"APP_NAME" envDefault:"cafeTelkom-api"`
	Env                    string `env:"APP_ENV" envDefault:"development"`
	Version                string `env:"APP_VERSION" envDefault:"dev"`
	ShutdownTimeoutSeconds int    `env:"APP_SHUTDOWN_TIMEOUT_SECONDS" envDefault:"10"`
}

type HTTPConfig struct {
	Port string `env:"APP_PORT" envDefault:"8080"`
}

type LogConfig struct {
	Level string `env:"LOG_LEVEL" envDefault:"info"`
}

type RedisConfig struct {
	Addr     string `env:"REDIS_ADDR" envDefault:"localhost:6379"`
	Password string `env:"REDIS_PASSWORD"`
	DB       int    `env:"REDIS_DB" envDefault:"0"`
	Required bool   `env:"REDIS_REQUIRED" envDefault:"false"`
}

type DatabaseConfig struct {
	URL               string `env:"DATABASE_URL"`
	Host              string `env:"SUPABASE_DB_HOST" envDefault:"aws-1-ap-northeast-2.pooler.supabase.com"`
	Port              int    `env:"SUPABASE_DB_PORT" envDefault:"5432"`
	Name              string `env:"SUPABASE_DB_NAME" envDefault:"postgres"`
	User              string `env:"SUPABASE_DB_USER" envDefault:"postgres.kangzprbrstwuuejpso"`
	Password          string `env:"SUPABASE_DB_PASSWORD"`
	SSLMode           string `env:"SUPABASE_DB_SSLMODE" envDefault:"require"`
	MaxOpenConns      int32  `env:"DB_MAX_OPEN_CONNS" envDefault:"10"`
	MinIdleConns      int32  `env:"DB_MIN_IDLE_CONNS" envDefault:"2"`
	ConnMaxLifetimeM  int    `env:"DB_CONN_MAX_LIFETIME_MINUTES" envDefault:"30"`
	ConnMaxIdleTimeM  int    `env:"DB_CONN_MAX_IDLE_TIME_MINUTES" envDefault:"5"`
	HealthcheckSecond int    `env:"DB_HEALTHCHECK_SECONDS" envDefault:"5"`
	Required          bool   `env:"DB_REQUIRED" envDefault:"false"`
}

type InternalConfig struct {
	APIKey string `env:"INTERNAL_API_KEY" envDefault:"change-me"`
}

func Load() (Config, error) {
	// Ignore missing .env so runtime env vars still work in containers/CI.
	_ = godotenv.Load()

	cfg := Config{}
	if err := env.Parse(&cfg); err != nil {
		return Config{}, err
	}

	if cfg.HTTP.Port == "" {
		return Config{}, fmt.Errorf("APP_PORT cannot be empty")
	}

	return cfg, nil
}

func (c Config) DatabaseURL() string {
	if c.Database.URL != "" {
		return c.Database.URL
	}

	if c.Database.Password == "" {
		return ""
	}

	return fmt.Sprintf(
		"postgresql://%s:%s@%s:%d/%s?sslmode=%s",
		c.Database.User,
		c.Database.Password,
		c.Database.Host,
		c.Database.Port,
		c.Database.Name,
		c.Database.SSLMode,
	)
}


