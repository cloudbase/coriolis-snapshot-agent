// Copyright 2019 Cloudbase Solutions Srl
// All Rights Reserved.

package errors

import "fmt"

var (
	// ErrUnauthorized is returned when a user does not have
	// authorization to perform a request
	ErrUnauthorized = NewUnauthorizedError("Unauthorized")
	// ErrNotFound is returned if an object is not found in
	// the database.
	ErrNotFound = NewNotFoundError("not found")
	// ErrInvalidSession is returned when a session is invalid
	ErrInvalidSession = NewInvalidSessionError("invalid session")
	// ErrBadRequest is returned is a malformed request is sent
	ErrBadRequest = NewBadRequestError("invalid request")
	// ErrNotImplemented returns a not implemented error.
	ErrNotImplemented = fmt.Errorf("not implemented")
	// ErrNoInfo is returned when no info could be found about a resource
	ErrNoInfo = fmt.Errorf("no info available")
)

// ErrInvalidDevice is returned when a device does not meet the
// required criteria to be considered valid.
type ErrInvalidDevice struct {
	message string
}

func (e ErrInvalidDevice) Error() string {
	return e.message
}

// NewInvalidDeviceErr returns a new ErrInvalidDevice
func NewInvalidDeviceErr(msg string) error {
	return &ErrInvalidDevice{
		message: msg,
	}
}

// IsInvalidDevice checks if the supplied error is of type ErrInvalidDevice
func IsInvalidDevice(err error) bool {
	if err == nil {
		return false
	}

	_, ok := err.(*ErrInvalidDevice)
	return ok
}

// ErrVolumeNotFound is returned when a particular volume was not found
type ErrVolumeNotFound struct {
	message string
}

func (e ErrVolumeNotFound) Error() string {
	return e.message
}

// NewVolumeNotFoundErr returns a new ErrVolumeNotFound
func NewVolumeNotFoundErr(msg string) error {
	return &ErrVolumeNotFound{
		message: msg,
	}
}

// IsVolumeNotFound checks if the supplied error is of type ErrVolumeNotFound
func IsVolumeNotFound(err error) bool {
	if err == nil {
		return false
	}

	_, ok := err.(*ErrVolumeNotFound)
	return ok
}

// ErrOperationInterrupted is returned when an operation is interrupted
type ErrOperationInterrupted struct {
	message string
}

func (e ErrOperationInterrupted) Error() string {
	return e.message
}

// NewOperationInterruptedErr returns a new ErrOperationInterrupted error
func NewOperationInterruptedErr(msg string) error {
	return &ErrOperationInterrupted{
		message: msg,
	}
}

// IsOperationInterrupted checks if the supplied error is of type ErrOperationInterrupted
func IsOperationInterrupted(err error) bool {
	if err == nil {
		return false
	}

	_, ok := err.(*ErrOperationInterrupted)
	return ok
}

type baseError struct {
	msg string
}

func (b *baseError) Error() string {
	return b.msg
}

// NewUnauthorizedError returns a new UnauthorizedError
func NewUnauthorizedError(msg string) error {
	return &UnauthorizedError{
		baseError{
			msg: msg,
		},
	}
}

// UnauthorizedError is returned when a request is unauthorized
type UnauthorizedError struct {
	baseError
}

// NewNotFoundError returns a new NotFoundError
func NewNotFoundError(msg string) error {
	return &NotFoundError{
		baseError{
			msg: msg,
		},
	}
}

// NotFoundError is returned when a resource is not found
type NotFoundError struct {
	baseError
}

// NewInvalidSessionError returns a new InvalidSessionError
func NewInvalidSessionError(msg string) error {
	return &InvalidSessionError{
		baseError{
			msg: msg,
		},
	}
}

// InvalidSessionError is returned when a session is invalid
type InvalidSessionError struct {
	baseError
}

// NewBadRequestError returns a new BadRequestError
func NewBadRequestError(msg string, a ...interface{}) error {
	return &BadRequestError{
		baseError{
			msg: fmt.Sprintf(msg, a...),
		},
	}
}

// BadRequestError is returned when a malformed request is received
type BadRequestError struct {
	baseError
}

// NewConflictError returns a new ConflictError
func NewConflictError(msg string, a ...interface{}) error {
	return &ConflictError{
		baseError{
			msg: fmt.Sprintf(msg, a...),
		},
	}
}

// ConflictError is returned when a conflicting request is made
type ConflictError struct {
	baseError
}
