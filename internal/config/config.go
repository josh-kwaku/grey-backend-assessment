package config

import (
	"fmt"

	env "github.com/caarlos0/env/v11"
)

type Config struct {
	DatabaseURL     string  `env:"DATABASE_URL,required"`
	JWTSecret       string  `env:"JWT_SECRET,required"`
	FXSpreadPct     float64 `env:"FX_SPREAD_PCT" envDefault:"0.005"`
	MockProviderURL    string  `env:"MOCK_PROVIDER_URL" envDefault:"http://mock-provider:8081"`
	WebhookCallbackURL string  `env:"WEBHOOK_CALLBACK_URL" envDefault:"http://app:8080/api/v1/webhooks/provider"`
	WebhookSecret      string  `env:"WEBHOOK_SECRET,required"`
	Port            int     `env:"PORT" envDefault:"8080"`
	LogLevel        string  `env:"LOG_LEVEL" envDefault:"info"`
	AppEnv          string  `env:"APP_ENV" envDefault:"production"`

	TxLimitUSD int64 `env:"TX_LIMIT_USD" envDefault:"10000000"`
	TxLimitEUR int64 `env:"TX_LIMIT_EUR" envDefault:"9000000"`
	TxLimitGBP int64 `env:"TX_LIMIT_GBP" envDefault:"8000000"`

	DBMaxOpenConns    int `env:"DB_MAX_OPEN_CONNS" envDefault:"25"`
	DBMaxIdleConns    int `env:"DB_MAX_IDLE_CONNS" envDefault:"10"`
	DBConnMaxLifetimeS int `env:"DB_CONN_MAX_LIFETIME_S" envDefault:"300"`
	DBConnMaxIdleTimeS int `env:"DB_CONN_MAX_IDLE_TIME_S" envDefault:"60"`
}

func Load() (*Config, error) {
	cfg, err := env.ParseAs[Config]()
	if err != nil {
		return nil, fmt.Errorf("config.Load: %w", err)
	}
	return &cfg, nil
}
