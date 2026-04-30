package databaseconnector

import "errors"

var (
	ErrConnectorNil       = errors.New("database connector is nil")
	ErrConnectorTypeEmpty = errors.New("database connector type is empty")
	ErrConnectorNotFound  = errors.New("database connector type not found in registry")
)
