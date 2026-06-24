package router

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type egressPolicyError struct {
	label string
	msg   string
}

func (e *egressPolicyError) Error() string {
	return e.label + " " + e.msg
}

func validateEgressURL(u *url.URL, allowHosts []string, allowHTTP bool, label string) error {
	if u == nil || u.Hostname() == "" {
		return &egressPolicyError{label: label, msg: "URL is invalid"}
	}
	switch strings.ToLower(u.Scheme) {
	case "https":
		return validateEgressHost(u.Hostname(), allowHosts, label)
	case "http":
		if !allowHTTP && !egressHostIsTrustedLocal(u.Hostname()) {
			return &egressPolicyError{label: label, msg: "http requires allow_http for non-local hosts"}
		}
		return validateEgressHost(u.Hostname(), allowHosts, label)
	default:
		return &egressPolicyError{label: label, msg: "URL scheme must be http or https"}
	}
}

func validateEgressHost(host string, allowHosts []string, label string) error {
	if !scriptHostAllowed(host, allowHosts) {
		return &egressPolicyError{label: label, msg: fmt.Sprintf("host %s is not allowed", host)}
	}
	return nil
}

func newEgressHTTPClient(timeout time.Duration, allowHosts []string, allowHTTP bool, label string) *http.Client {
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return &egressPolicyError{label: label, msg: "too many redirects"}
			}
			if err := validateEgressURL(req.URL, allowHosts, allowHTTP, label); err != nil {
				return err
			}
			return nil
		},
	}
}

func auditSafeHTTPError(err error, fallback string) error {
	var policyErr *egressPolicyError
	if errors.As(err, &policyErr) {
		return policyErr
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return fmt.Errorf("%s timed out", fallback)
	}
	return fmt.Errorf("%s", fallback)
}

func egressHostIsTrustedLocal(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
