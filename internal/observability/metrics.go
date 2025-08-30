package observability

import (
	"expvar"
)

var (
	RequestsTotal       = expvar.NewInt("requests_total")
	RequestErrorsTotal  = expvar.NewInt("request_errors_total")
)

func IncRequests() {
	RequestsTotal.Add(1)
}

func IncRequestErrors() {
	RequestErrorsTotal.Add(1)
}
