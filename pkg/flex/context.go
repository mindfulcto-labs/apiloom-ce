package flex

import (
	"io"
	"net/http"

	"github.com/luraproject/lura/v2/proxy"

	"github.com/mindfulcto-labs/apiloom-ce/internal/domain"
)

// RequestContext wraps a Lura proxy.Request with helper methods for plugins
// to inspect and modify the incoming request.
type RequestContext struct {
	req *proxy.Request
}

// NewRequestContext creates a RequestContext wrapping the given proxy request.
func NewRequestContext(req *proxy.Request) *RequestContext {
	return &RequestContext{req: req}
}

// Request returns the underlying proxy.Request.
func (rc *RequestContext) Request() *proxy.Request {
	return rc.req
}

// WithHeader sets a request header, replacing any existing values.
// Initializes the headers map if nil.
func (rc *RequestContext) WithHeader(key, value string) {
	if rc.req.Headers == nil {
		rc.req.Headers = make(map[string][]string)
	}
	rc.req.Headers[key] = []string{value}
}

// WithBody replaces the request body.
func (rc *RequestContext) WithBody(body io.ReadCloser) {
	rc.req.Body = body
}

// RejectRequest returns a ProblemError that short-circuits the middleware chain.
// The error includes the HTTP status, a URN-formatted error type, and detail message.
func (rc *RequestContext) RejectRequest(status int, errType, detail string) error {
	return domain.NewProblemError(status, errType, detail)
}

// ResponseContext wraps a Lura proxy.Response with helper methods for plugins
// to inspect and modify the outgoing response.
type ResponseContext struct {
	resp *proxy.Response
}

// NewResponseContext creates a ResponseContext wrapping the given proxy response.
func NewResponseContext(resp *proxy.Response) *ResponseContext {
	return &ResponseContext{resp: resp}
}

// Response returns the underlying proxy.Response.
func (rc *ResponseContext) Response() *proxy.Response {
	return rc.resp
}

// WithData adds or replaces a key in the response data map.
// Initializes the data map if nil.
func (rc *ResponseContext) WithData(key string, value interface{}) {
	if rc.resp.Data == nil {
		rc.resp.Data = make(map[string]interface{})
	}
	rc.resp.Data[key] = value
}

// StatusCode returns the HTTP status from the response metadata, defaulting to 200.
func (rc *ResponseContext) StatusCode() int {
	if rc.resp.Metadata.StatusCode == 0 {
		return http.StatusOK
	}
	return rc.resp.Metadata.StatusCode
}
