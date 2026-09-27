package tripservice

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

var requiredVariables = [...]string{
	"HTTP_ADDR",
	"LOG_LEVEL",
	"SHUTDOWN_TIMEOUT",
	"DATABASE_URL",
	"DATABASE_MAX_CONNS",
	"DATABASE_MIN_CONNS",
	"DATABASE_MAX_CONN_LIFETIME",
	"DATABASE_CONNECT_TIMEOUT",
	"DATABASE_QUERY_TIMEOUT",
}

type Config struct {
	HttpAddress             string
	DatabaseUrl             string
	LogLevel                string
	ShutdownTimeout         time.Duration
	DatabaseMaxConns        int
	DatabaseMinConns        int
	DatabaseMaxConnLifetime time.Duration
	DatabaseConnectTimeout  time.Duration
	DatabaseQueryTimeout    time.Duration
}

func assignDuration(field *time.Duration, value string) error {
	duration, err := time.ParseDuration(value)
	if err != nil {
		return err
	}

	*field = duration

	return nil
}

func assignInt(field *int, value string) error {
	number, err := strconv.Atoi(value)
	if err != nil {
		return err
	}

	*field = number

	return nil
}

func (c *Config) AssignValue(name, value string) error {
	var err error

	switch name {
	case "HTTP_ADDR":
		c.HttpAddress = value
	case "DATABASE_URL":
		c.DatabaseUrl = value
	case "LOG_LEVEL":
		c.LogLevel = value
	case "DATABASE_MAX_CONNS":
		err = assignInt(&c.DatabaseMaxConns, value)
	case "DATABASE_MIN_CONNS":
		err = assignInt(&c.DatabaseMinConns, value)
	case "DATABASE_MAX_CONN_LIFETIME":
		err = assignDuration(&c.DatabaseMaxConnLifetime, value)
	case "SHUTDOWN_TIMEOUT":
		err = assignDuration(&c.ShutdownTimeout, value)
	case "DATABASE_CONNECT_TIMEOUT":
		err = assignDuration(&c.DatabaseConnectTimeout, value)
	case "DATABASE_QUERY_TIMEOUT":
		err = assignDuration(&c.DatabaseQueryTimeout, value)
	default:
		return fmt.Errorf("Unknown variable %s", name)
	}

	if err != nil {
		return fmt.Errorf("Unexpected value %s for %s: %w", value, name, err)
	}

	return nil
}

func ParseEnv() (*Config, error) {
	config := Config{}

	for _, variableName := range requiredVariables {
		variableValue := strings.TrimSpace(os.Getenv(variableName))
		if variableValue == "" {
			return nil, fmt.Errorf("Environment parse error: Variable %s is required", variableName)
		}

		if err := config.AssignValue(variableName, variableValue); err != nil {
			return nil, fmt.Errorf("Environment parse error: %w", err)
		}
	}

	if config.DatabaseMaxConns <= 0 {
		return nil, fmt.Errorf("Environment parse error: DATABASE_MAX_CONNS must be greater than zero")
	}
	if config.DatabaseMinConns < 0 || config.DatabaseMinConns > config.DatabaseMaxConns {
		return nil, fmt.Errorf("Environment parse error: DATABASE_MIN_CONNS must be between zero and DATABASE_MAX_CONNS")
	}
	if config.DatabaseMaxConnLifetime <= 0 {
		return nil, fmt.Errorf("Environment parse error: DATABASE_MAX_CONN_LIFETIME must be greater than zero")
	}
	if config.ShutdownTimeout <= 0 {
		return nil, fmt.Errorf("Environment parse error: SHUTDOWN_TIMEOUT must be greater than zero")
	}
	if config.DatabaseQueryTimeout <= 0 {
		return nil, fmt.Errorf("Environment parse error: DATABASE_QUERY_TIMEOUT must be greater than zero")
	}

	return &config, nil
}
