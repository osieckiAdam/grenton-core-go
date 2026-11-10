// Package provisioning gets a mobile interface (spec §4): downloaded from
// Object Manager with the interface PIN, read from a saved archive, or
// imported from an Object Manager project file (.omp).
package provisioning

import (
	"errors"
	"fmt"
)

// Kind is the variant of a provisioning [Error] (spec §8).
type Kind int

// The variants of ProvisioningError.
const (
	// Connection: the URL cannot be built, the network failed, or Object
	// Manager answered a status other than 200 and 401.
	Connection Kind = iota + 1
	// AuthenticationFailed: Object Manager answered 401 (a wrong PIN).
	AuthenticationFailed
	// InvalidData: the response, archive or project is not a valid interface.
	InvalidData
	// InterfaceNotFound: no interface of the project has the requested id,
	// or the project has none.
	InterfaceNotFound
	// InterfaceAmbiguous: the project has several interfaces and none was
	// chosen.
	InterfaceAmbiguous
)

func (k Kind) String() string {
	switch k {
	case Connection:
		return "connection"
	case AuthenticationFailed:
		return "authenticationFailed"
	case InvalidData:
		return "invalidData"
	case InterfaceNotFound:
		return "interfaceNotFound"
	case InterfaceAmbiguous:
		return "interfaceAmbiguous"
	}
	return "unknown"
}

// Error is every error of this package. Its text never contains the PIN
// (spec §4.2): transport errors are rebuilt without the request URL.
type Error struct {
	Kind Kind
	// Status is the HTTP status of a Connection error from a response, else 0.
	Status int
	// Detail says what went wrong, for logs.
	Detail string
}

func (e *Error) Error() string {
	if e.Detail == "" {
		return "provisioning: " + e.Kind.String()
	}
	return "provisioning: " + e.Kind.String() + ": " + e.Detail
}

// Is matches the sentinel of the error's kind, so that
// errors.Is(err, provisioning.ErrInvalidData) works.
func (e *Error) Is(target error) bool {
	var t *Error
	return errors.As(target, &t) && t.Status == 0 && t.Detail == "" && t.Kind == e.Kind
}

// Sentinels for errors.Is.
var (
	ErrConnection           error = &Error{Kind: Connection}
	ErrAuthenticationFailed error = &Error{Kind: AuthenticationFailed}
	ErrInvalidData          error = &Error{Kind: InvalidData}
	ErrInterfaceNotFound    error = &Error{Kind: InterfaceNotFound}
	ErrInterfaceAmbiguous   error = &Error{Kind: InterfaceAmbiguous}
)

func invalidData(format string, args ...any) *Error {
	return &Error{Kind: InvalidData, Detail: fmt.Sprintf(format, args...)}
}
