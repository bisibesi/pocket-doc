package config

import "errors"

// Common configuration errors
var (
	ErrMissingDBType   = errors.New("database type is required")
	ErrInvalidDBType   = errors.New("invalid database type")
	ErrMissingHost     = errors.New("database host is required")
	ErrMissingDatabase = errors.New("database name is required")
	ErrInvalidPort     = errors.New("invalid port number")
	ErrInvalidFormat   = errors.New("invalid output format")
)
