// Package validation collects and checks input errors at API boundaries.
package validation

type Errors map[string]string

const GeneralKey = "general"

func New() Errors {
	return Errors{}
}

func (errs Errors) Add(key, message string) {
	if errs == nil {
		return
	}
	if key == "" || message == "" {
		return
	}
	errs[key] = message
}

func (errs Errors) HasAny() bool {
	return len(errs) > 0
}
