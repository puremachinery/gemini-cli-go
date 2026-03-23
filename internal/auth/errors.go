package auth

// FatalAuthenticationError indicates a non-recoverable authentication failure.
type FatalAuthenticationError struct {
	Message string
}

func (e FatalAuthenticationError) Error() string {
	return e.Message
}

// FatalCancellationError indicates the user cancelled authentication.
type FatalCancellationError struct {
	Message string
}

func (e FatalCancellationError) Error() string {
	return e.Message
}
