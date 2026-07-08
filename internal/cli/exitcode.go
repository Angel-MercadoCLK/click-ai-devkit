package cli

type exitCodeError struct {
	code int
	msg  string
}

func (e *exitCodeError) Error() string {
	return e.msg
}

func (e *exitCodeError) ExitCode() int {
	return e.code
}
