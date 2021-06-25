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

func (b *ErrInvalidDevice) Is(target error) bool {
	if target == nil {
		return false
	}
	_, ok := target.(*ErrInvalidDevice)
	return ok
}

func (e ErrInvalidDevice) Error() string {
	return e.message
}

// NewInvalidDeviceErr returns a new ErrInvalidDevice
func NewInvalidDeviceErr(msg string, a ...interface{}) error {
	return &ErrInvalidDevice{
		message: fmt.Sprintf(msg, a...),
	}
}

// ErrVolumeNotFound is returned when a particular volume was not found
type ErrVolumeNotFound struct {
	message string
}

func (b *ErrVolumeNotFound) Is(target error) bool {
	if target == nil {
		return false
	}
	_, ok := target.(*ErrVolumeNotFound)
	return ok
}

func (e ErrVolumeNotFound) Error() string {
	return e.message
}

// NewVolumeNotFoundErr returns a new ErrVolumeNotFound
func NewVolumeNotFoundErr(msg string, a ...interface{}) error {
	return &ErrVolumeNotFound{
		message: fmt.Sprintf(msg, a...),
	}
}

// ErrOperationInterrupted is returned when an operation is interrupted
type ErrOperationInterrupted struct {
	message string
}

func (b *ErrOperationInterrupted) Is(target error) bool {
	if target == nil {
		return false
	}
	_, ok := target.(*ErrOperationInterrupted)
	return ok
}

func (e ErrOperationInterrupted) Error() string {
	return e.message
}

// NewOperationInterruptedErr returns a new ErrOperationInterrupted error
func NewOperationInterruptedErr(msg string, a ...interface{}) error {
	return &ErrOperationInterrupted{
		message: fmt.Sprintf(msg, a...),
	}
}

type baseError struct {
	msg string
}

func (b *baseError) Error() string {
	return b.msg
}

// NewUnauthorizedError returns a new UnauthorizedError
func NewUnauthorizedError(msg string, a ...interface{}) error {
	return &UnauthorizedError{
		baseError{
			msg: fmt.Sprintf(msg, a...),
		},
	}
}

// UnauthorizedError is returned when a request is unauthorized
type UnauthorizedError struct {
	baseError
}

func (b *UnauthorizedError) Is(target error) bool {
	if target == nil {
		return false
	}
	_, ok := target.(*UnauthorizedError)
	return ok
}

// NewNotFoundError returns a new NotFoundError
func NewNotFoundError(msg string, a ...interface{}) error {
	return &NotFoundError{
		baseError{
			msg: fmt.Sprintf(msg, a...),
		},
	}
}

// NotFoundError is returned when a resource is not found
type NotFoundError struct {
	baseError
}

func (b *NotFoundError) Is(target error) bool {
	if target == nil {
		return false
	}
	_, ok := target.(*NotFoundError)
	return ok
}

// NewInvalidSessionError returns a new InvalidSessionError
func NewInvalidSessionError(msg string, a ...interface{}) error {
	return &InvalidSessionError{
		baseError{
			msg: fmt.Sprintf(msg, a...),
		},
	}
}

// InvalidSessionError is returned when a session is invalid
type InvalidSessionError struct {
	baseError
}

func (b *InvalidSessionError) Is(target error) bool {
	if target == nil {
		return false
	}
	_, ok := target.(*InvalidSessionError)
	return ok
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

func (b *BadRequestError) Is(target error) bool {
	if target == nil {
		return false
	}
	_, ok := target.(*BadRequestError)
	return ok
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

func (b *ConflictError) Is(target error) bool {
	if target == nil {
		return false
	}
	_, ok := target.(*ConflictError)
	return ok
}

// NewValueError returns a new ValueError
func NewValueError(msg string, a ...interface{}) error {
	return &ValueError{
		baseError{
			msg: fmt.Sprintf(msg, a...),
		},
	}
}

// ValueError is returned when a value is invalid.
type ValueError struct {
	baseError
}

func (b *ValueError) Is(target error) bool {
	if target == nil {
		return false
	}
	_, ok := target.(*ValueError)
	return ok
}

// NewSnapStoreOverflowError returns a new ErrSnapStoreOverflow
func NewSnapStoreOverflowError(msg string, a ...interface{}) error {
	return &ErrSnapStoreOverflow{
		baseError{
			msg: fmt.Sprintf(msg, a...),
		},
	}
}

type ErrSnapStoreOverflow struct {
	baseError
}

func (b *ErrSnapStoreOverflow) Is(target error) bool {
	if target == nil {
		return false
	}
	_, ok := target.(*ValueError)
	return ok
}
