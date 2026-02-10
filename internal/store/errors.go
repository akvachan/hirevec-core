package store

import "fmt"

var (
	ErrFailedToConnectDB              = func(err error) error { return fmt.Errorf("failed to connect to database: %w", err) }
	ErrNamesRequired                  = fmt.Errorf("empty names provided")
	ErrFailedToGenerateUsernameSuffix = func(err error) error { return fmt.Errorf("failed to generate a username suffix: %w", err) }
	ErrUserDoesNotExist               = fmt.Errorf("user with the  providerUserID does not exist")
	ErrUserDoesNotHaveARole           = fmt.Errorf("user does not have any role")
)
