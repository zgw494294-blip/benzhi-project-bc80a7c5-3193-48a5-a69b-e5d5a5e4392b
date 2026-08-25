package domain

import "fmt"

type ErrorKind string

const (
	KindValidation ErrorKind = "validation"
	KindConflict   ErrorKind = "conflict"
	KindNotFound   ErrorKind = "not_found"
	KindImmutable  ErrorKind = "immutable"
)

type RuleError struct {
	Kind    ErrorKind `json:"kind"`
	Code    string    `json:"code"`
	Message string    `json:"message"`
}

func (e *RuleError) Error() string { return e.Message }

func Validation(code, message string) error {
	return &RuleError{Kind: KindValidation, Code: code, Message: message}
}

func Conflict(code, message string) error {
	return &RuleError{Kind: KindConflict, Code: code, Message: message}
}

func NotFound(entity, id string) error {
	return &RuleError{Kind: KindNotFound, Code: entity + "_not_found", Message: fmt.Sprintf("未找到%s：%s", entity, id)}
}

func Immutable(message string) error {
	return &RuleError{Kind: KindImmutable, Code: "approved_batch_immutable", Message: message}
}

func AsRuleError(err error) (*RuleError, bool) {
	e, ok := err.(*RuleError)
	return e, ok
}
