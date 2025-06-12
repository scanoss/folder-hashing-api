package errors

import (
	"fmt"
)

// Domain-specific errors for the scan service

// ErrInvalidRequest represents an invalid request error
type ErrInvalidRequest struct {
	Message string
}

func (e *ErrInvalidRequest) Error() string {
	return fmt.Sprintf("invalid request: %s", e.Message)
}

// NewInvalidRequestError creates a new invalid request error
func NewInvalidRequestError(message string) *ErrInvalidRequest {
	return &ErrInvalidRequest{Message: message}
}

// ErrRepositoryFailure represents a repository operation failure
type ErrRepositoryFailure struct {
	Operation string
	Cause     error
}

func (e *ErrRepositoryFailure) Error() string {
	return fmt.Sprintf("repository operation '%s' failed: %v", e.Operation, e.Cause)
}

// NewRepositoryFailureError creates a new repository failure error
func NewRepositoryFailureError(operation string, cause error) *ErrRepositoryFailure {
	return &ErrRepositoryFailure{Operation: operation, Cause: cause}
}

// ErrCollectionNotFound represents a collection not found error
type ErrCollectionNotFound struct {
	CollectionName string
}

func (e *ErrCollectionNotFound) Error() string {
	return fmt.Sprintf("collection '%s' not found", e.CollectionName)
}

// NewCollectionNotFoundError creates a new collection not found error
func NewCollectionNotFoundError(collectionName string) *ErrCollectionNotFound {
	return &ErrCollectionNotFound{CollectionName: collectionName}
}

// ErrInvalidHash represents an invalid hash error
type ErrInvalidHash struct {
	HashValue string
	HashType  string
}

func (e *ErrInvalidHash) Error() string {
	return fmt.Sprintf("invalid %s hash: %s", e.HashType, e.HashValue)
}

// NewInvalidHashError creates a new invalid hash error
func NewInvalidHashError(hashType, hashValue string) *ErrInvalidHash {
	return &ErrInvalidHash{HashType: hashType, HashValue: hashValue}
}

// ErrThresholdNotMet represents a threshold not met error
type ErrThresholdNotMet struct {
	Threshold int32
	MaxScore  float32
}

func (e *ErrThresholdNotMet) Error() string {
	return fmt.Sprintf("no results meet threshold %d (max score: %.4f)", e.Threshold, e.MaxScore)
}

// NewThresholdNotMetError creates a new threshold not met error
func NewThresholdNotMetError(threshold int32, maxScore float32) *ErrThresholdNotMet {
	return &ErrThresholdNotMet{Threshold: threshold, MaxScore: maxScore}
}
