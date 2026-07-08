package cmd

import "fmt"

type ExitError struct {
	Code int
	Err  error
}

func NewExitError(code int, err error) *ExitError {
	if code == 0 {
		code = 1
	}
	return &ExitError{Code: code, Err: err}
}

func (e *ExitError) Error() string {
	if e == nil {
		return "exit with code 1"
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	code := e.Code
	if code == 0 {
		code = 1
	}
	return fmt.Sprintf("exit with code %d", code)
}

func (e *ExitError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}
