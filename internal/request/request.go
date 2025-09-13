package request

import "io"

type Request interface {
	GetQueryParams() map[string][]string
	GetHeaders() map[string][]string
	GetPathParams() map[string]string
	GetRequestBody() io.ReadCloser
}

type RequestConfig struct {
	QueryParams map[string][]string
	Headers     map[string][]string
	PathParams  map[string]string
	RequestBody io.ReadCloser
}

func NewRequest(config *RequestConfig) Request {
	return &request{
		queryParams: config.QueryParams,
		headers:     config.Headers,
		pathParams:  config.PathParams,
		requestBody: config.RequestBody,
	}
}

type request struct {
	queryParams map[string][]string
	headers     map[string][]string
	pathParams  map[string]string
	requestBody io.ReadCloser
}

func (r *request) GetQueryParams() map[string][]string {
	return r.queryParams
}

func (r *request) GetHeaders() map[string][]string {
	return r.headers
}
func (r *request) GetPathParams() map[string]string {
	return r.pathParams
}
func (r *request) GetRequestBody() io.ReadCloser {
	return r.requestBody
}
