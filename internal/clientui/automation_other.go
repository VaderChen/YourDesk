//go:build !darwin

package clientui

import "net/http"

func (s *server) installAutomation(*http.ServeMux) (func(), error) { return func() {}, nil }
