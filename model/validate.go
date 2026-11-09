package model

import "github.com/go-playground/validator/v10"

// Validator is what generated models call from Validate. Register custom rules
// on it at startup (Validator.RegisterValidation) to use them in struct tags.
var Validator = validator.New(validator.WithRequiredStructEnabled())
