package metadata

import (
	"errors"
	"fmt"
	"strings"
)

// ErrNotFound is returned when the requested metadata does not exist.
var ErrNotFound = errors.New("metadata not found")

// ErrSessionExpired is returned when the session has expired.
var ErrSessionExpired = errors.New("session expired or invalid")

// SOAPFaultError wraps a SOAP fault response.
type SOAPFaultError struct {
	FaultCode   string
	FaultString string
}

func (e *SOAPFaultError) Error() string {
	return fmt.Sprintf("SOAP fault: %s - %s", e.FaultCode, e.FaultString)
}

// IsSessionExpired checks if the SOAP fault indicates an expired session.
func (e *SOAPFaultError) IsSessionExpired() bool {
	return strings.Contains(e.FaultCode, "INVALID_SESSION_ID") ||
		strings.Contains(e.FaultString, "INVALID_SESSION_ID") ||
		strings.Contains(e.FaultString, "Session expired")
}

// DeployError wraps a deployment failure.
type DeployError struct {
	ID              string
	Status          string
	ErrorMessage    string
	ErrorStatusCode string
	ComponentErrors []ComponentFailure
}

func (e *DeployError) Error() string {
	if e.ErrorMessage != "" {
		if e.ErrorStatusCode != "" {
			return fmt.Sprintf("deploy failed (ID=%s, status=%s, code=%s): %s",
				e.ID, e.Status, e.ErrorStatusCode, e.ErrorMessage)
		}
		return fmt.Sprintf("deploy failed (ID=%s, status=%s): %s", e.ID, e.Status, e.ErrorMessage)
	}
	if len(e.ComponentErrors) > 0 {
		var problems []string
		for _, ce := range e.ComponentErrors {
			loc := ce.FullName
			if ce.FileName != "" {
				loc = ce.FileName
				if ce.LineNumber > 0 {
					if ce.ColumnNumber > 0 {
						loc = fmt.Sprintf("%s:%d:%d", ce.FileName, ce.LineNumber, ce.ColumnNumber)
					} else {
						loc = fmt.Sprintf("%s:%d", ce.FileName, ce.LineNumber)
					}
				}
			}
			problems = append(problems, fmt.Sprintf("%s: %s", loc, ce.Problem))
		}
		return fmt.Sprintf("deploy failed (ID=%s, status=%s): %s", e.ID, e.Status, strings.Join(problems, "; "))
	}
	return fmt.Sprintf("deploy failed (ID=%s, status=%s)", e.ID, e.Status)
}
