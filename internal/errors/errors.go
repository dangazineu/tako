package errors

import "fmt"

type TakoError struct {
	Code    string
	Message string
	Err     error
}

func (e *TakoError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s - %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *TakoError) Unwrap() error {
	return e.Err
}

func New(code, message string) *TakoError {
	return &TakoError{Code: code, Message: message}
}

func Wrap(err error, code, message string) *TakoError {
	return &TakoError{Code: code, Message: message, Err: err}
}
