// Package evidence defines and validates the versioned CI Trust evidence contract.
// It does not discover repositories, run checks or contact remote services.
package evidence

// Diagnostic identifies rejected contract data without echoing its contents.
// Path is a JSON pointer into the submitted document; an empty path is its root.
type Diagnostic struct {
	Code    string `json:"code"`
	Path    string `json:"path"`
	Message string `json:"message"`
}
