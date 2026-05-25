package labs

import "errors"

var (
	ErrInvalidID     = errors.New("invalid lab id")
	ErrLabNotFound   = errors.New("lab not found")
	ErrLabInvalid    = errors.New("lab workshop is invalid")
	ErrNoLabSelected = errors.New("select a lab")
)
