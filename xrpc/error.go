package xrpc

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/pkg/errors"
)

var ErrNotImplemented = &ErrorResponse{
	Code:    MethodNotImplemented,
	Message: "Not Implemented",
	Status:  http.StatusNotImplemented,
}

type ErrorResponse struct {
	Code    Code   `json:"error"`
	Message string `json:"message"`

	// Status is for RPC handlers. Should not be send back to clients.
	Status int `json:"-"`
	// Inner is a private internal error associated with the response.
	Inner error `json:"-"`
}

func (er *ErrorResponse) Error() string {
	if er.Inner != nil {
		return fmt.Sprintf("%s: %v", er.Message, er.Inner)
	}
	return er.Message
}

func (er *ErrorResponse) Unwrap() error {
	return er.Inner
}

// Cause is for [errors.Cause].
func (er *ErrorResponse) Cause() error {
	return er.Inner
}

func Wrapf(err error, code Code, format string, args ...any) *ErrorResponse {
	return Wrap(err, code, fmt.Sprintf(format, args...))
}

func Wrap(err error, code Code, msg string) *ErrorResponse {
	return &ErrorResponse{
		Code:    code,
		Message: msg,
		Inner:   err,
	}
}

func Status(status int, msg string) *ErrorResponse {
	return &ErrorResponse{
		Status:  status,
		Message: msg,
	}
}

// Wrap sets the Inner error field.
func (er *ErrorResponse) Wrap(err error) *ErrorResponse {
	er.Inner = err
	return er
}

// WithStatus sets the Status field.
func (er *ErrorResponse) WithStatus(status int) *ErrorResponse {
	er.Status = status
	return er
}

// WithMsg sets the Message field.
func (er *ErrorResponse) WithMsg(message string) *ErrorResponse {
	e := er.clone()
	e.Message = message
	return e
}

func (er *ErrorResponse) clone() *ErrorResponse {
	return &ErrorResponse{
		Code:    er.Code,
		Message: er.Message,
		Status:  er.Status,
		Inner:   er.Inner,
	}
}

// WithMsgf sets the Message field with an [fmt.Sprintf] format string.
func (er *ErrorResponse) WithMsgf(format string, args ...any) *ErrorResponse {
	er.Message = fmt.Sprintf(format, args...)
	return er
}

func NewInvalidRequest(msg string, args ...any) *ErrorResponse {
	return &ErrorResponse{
		Code:    InvalidRequest,
		Message: fmt.Sprintf(msg, args...),
	}
}

func NewInternalError(msg string, args ...any) *ErrorResponse {
	return &ErrorResponse{
		Code:    InternalServerError,
		Message: fmt.Sprintf(msg, args...),
	}
}

func NewAuthRequired(msg string, args ...any) *ErrorResponse {
	return &ErrorResponse{
		Code:    AuthRequired,
		Message: fmt.Sprintf(msg, args...),
	}
}

func WriteInvalidRequest(l *slog.Logger, w http.ResponseWriter, err error, msg string, args ...any) {
	err = NewInvalidRequest(msg, args...).Wrap(err)
	WriteError(l, w, err, InvalidRequest)
}

type stackTracer interface {
	StackTrace() errors.StackTrace
}

func WriteError(l *slog.Logger, w http.ResponseWriter, err error, defaultStatus Code) {
	if err == nil {
		return
	}
	switch e := err.(type) {
	case *ErrorResponse:
		writeErrorResponse(l, w, e, defaultStatus)
	default:
		inner := errors.Cause(err)
		if inner != nil {
			innerXrpcErr, ok := inner.(*ErrorResponse)
			if ok {
				writeErrorResponse(l, w, innerXrpcErr, defaultStatus)
				return
			}
		}
		var errResp *ErrorResponse
		if errors.As(err, &errResp) {
			writeErrorResponse(l, w, errResp, defaultStatus)
			return
		}
		status := defaultStatus.Status()
		logfn := l.Error
		if status >= 200 && status < 300 {
			logfn = l.Debug
		} else if status >= 400 && status < 500 {
			logfn = l.Info
		}
		logargs := []any{
			slog.Any("error", err),
		}
		if stacker, ok := err.(stackTracer); ok {
			logargs = append(logargs, slog.String("stacktrace", fmt.Sprintf("%+v", stacker.StackTrace())))
		} else {
			cause := errors.Cause(err)
			if cause != nil {
				if stacker, ok := cause.(stackTracer); ok {
					logargs = append(logargs, slog.String("stacktrace", fmt.Sprintf("%+v", stacker.StackTrace())))
				}
			}
		}
		logfn("server error", logargs...)
		w.WriteHeader(defaultStatus.Status())
		err = json.NewEncoder(w).Encode(&ErrorResponse{
			Code:    defaultStatus,
			Message: "Server Error",
		})
		if err != nil {
			l.Error("failed to decode error message", "error", err)
		}
	}
}

func writeErrorResponse(l *slog.Logger, w http.ResponseWriter, e *ErrorResponse, defaultStatus Code) {
	status := e.Status
	if status <= 0 || status >= 600 {
		status = e.Code.Status()
		if status == 0 {
			status = defaultStatus.Status()
		}
	}
	if len(e.Code) == 0 {
		e.Code = CodeFromStatus(status)
	}
	logfn := l.Error
	if status >= 200 && status < 300 {
		logfn = l.Debug
	} else if status >= 400 && status < 500 {
		logfn = l.Info
	}
	if len(e.Message) == 0 {
		e.Message = e.Code.Message()
	}

	logargs := []any{
		slog.Any("inner", e.Inner),
		slog.String("code", e.Code.String()),
		slog.String("message", e.Message),
	}
	cause := errors.Cause(e)
	if cause != nil {
		if stack, ok := cause.(stackTracer); ok {
			logargs = append(logargs, slog.String("stacktrace", fmt.Sprintf("%+v", stack.StackTrace())))
		}
	}
	logfn("xrpc failure", logargs...)
	w.WriteHeader(status)
	err := json.NewEncoder(w).Encode(e)
	if err != nil {
		l.Error("failed to decode error message", "error", err)
	}
}

type Code string

const (
	Unknown              Code = "Unknown"
	InvalidResponse      Code = "InvalidResponse"
	Success              Code = "Success"
	InvalidRequest       Code = "InvalidRequest"
	AuthRequired         Code = "AuthRequired"
	Forbidden            Code = "Forbidden"
	XRPCNotSupported     Code = "XRPCNotSupported"
	NotAcceptable        Code = "NotAcceptable"
	PayloadTooLarge      Code = "PayloadTooLarge"
	UnsupportedMediaType Code = "UnsupportedMediaType"
	RateLimitExceeded    Code = "RateLimitExceeded"
	InternalServerError  Code = "InternalServerError"
	MethodNotImplemented Code = "MethodNotImplemented"
	UpstreamFailure      Code = "UpstreamFailure"
	NotEnoughResources   Code = "NotEnoughResources"
	UpstreamTimeout      Code = "UpstreamTimeout"
)

// Other Codes
const (
	RecordNotFound Code = "RecordNotFound"
	RepoNotFound   Code = "RepoNotFound"
)

func CodeFromStatus(status int) Code {
	switch status {
	case http.StatusOK:
		return Success
	case http.StatusBadRequest:
		return InvalidRequest
	case http.StatusUnauthorized:
		return AuthRequired
	case http.StatusForbidden:
		return Forbidden
	case http.StatusNotFound:
		return XRPCNotSupported
	case http.StatusNotAcceptable:
		return NotAcceptable
	case http.StatusRequestEntityTooLarge:
		return PayloadTooLarge
	case http.StatusUnsupportedMediaType:
		return UnsupportedMediaType
	case http.StatusTooManyRequests:
		return RateLimitExceeded
	case http.StatusInternalServerError:
		return InternalServerError
	case http.StatusNotImplemented:
		return MethodNotImplemented
	case http.StatusBadGateway:
		return UpstreamFailure
	case http.StatusServiceUnavailable:
		return NotEnoughResources
	case http.StatusGatewayTimeout:
		return UpstreamTimeout
	default:
		if status >= 100 && status < 200 {
			return XRPCNotSupported
		} else if status >= 200 && status < 300 {
			return Success
		} else if status >= 300 && status < 400 {
			return XRPCNotSupported
		} else if status >= 400 && status < 500 {
			return InvalidRequest
		} else {
			return InternalServerError
		}
	}
}

func (c Code) Status() int {
	switch c {
	case Unknown:
		return http.StatusBadRequest
	case Success:
		return http.StatusOK
	case InvalidRequest:
		return http.StatusBadRequest
	case AuthRequired:
		return http.StatusUnauthorized
	case Forbidden:
		return http.StatusForbidden
	case XRPCNotSupported:
		return http.StatusNotFound
	case NotAcceptable:
		return http.StatusNotAcceptable
	case PayloadTooLarge:
		return http.StatusRequestEntityTooLarge
	case UnsupportedMediaType:
		return http.StatusUnsupportedMediaType
	case RateLimitExceeded:
		return http.StatusTooManyRequests
	case InternalServerError:
		return http.StatusInternalServerError
	case MethodNotImplemented:
		return http.StatusNotImplemented
	case UpstreamFailure:
		return http.StatusBadGateway
	case NotEnoughResources:
		return http.StatusServiceUnavailable
	case UpstreamTimeout:
		return http.StatusGatewayTimeout
	// Other codes
	case RepoNotFound, RecordNotFound:
		return http.StatusBadRequest
	default:
		return 0
	}
}

func (c Code) MarshalText() ([]byte, error) {
	return []byte(c.String()), nil
}

func (c Code) String() string {
	switch c {
	case Unknown:
		return "Unknown"
	case InvalidResponse:
		return "InvalidResponse"
	case Success:
		return "Success"
	case InvalidRequest:
		return "InvalidRequest"
	case AuthRequired:
		return "AuthRequired"
	case Forbidden:
		return "Forbidden"
	case XRPCNotSupported:
		return "XRPCNotSupported"
	case NotAcceptable:
		return "NotAcceptable"
	case PayloadTooLarge:
		return "PayloadTooLarge"
	case UnsupportedMediaType:
		return "UnsupportedMediaType"
	case RateLimitExceeded:
		return "RateLimitExceeded"
	case InternalServerError:
		return "InternalServerError"
	case MethodNotImplemented:
		return "MethodNotImplemented"
	case UpstreamFailure:
		return "UpstreamFailure"
	case NotEnoughResources:
		return "NotEnoughResources"
	case UpstreamTimeout:
		return "UpstreamTimeout"
	default:
		return string(c)
	}
}

func (c Code) Message() string {
	switch c {
	case Unknown:
		return "Unknown"
	case InvalidResponse:
		return "Invalid Response"
	case Success:
		return "Success"
	case InvalidRequest:
		return "Invalid Request"
	case AuthRequired:
		return "Authentication Required"
	case Forbidden:
		return "Forbidden"
	case XRPCNotSupported:
		return "XRPC Not Supported"
	case PayloadTooLarge:
		return "Payload Too Large"
	case UnsupportedMediaType:
		return "Unsupported Media Type"
	case RateLimitExceeded:
		return "Rate Limit Exceeded"
	case InternalServerError:
		return "Internal Server Error"
	case MethodNotImplemented:
		return "Method Not Implemented"
	case UpstreamFailure:
		return "Upstream Failure"
	case NotEnoughResources:
		return "Not Enough Resources"
	case UpstreamTimeout:
		return "Upstream Timeout"
	default:
		return ""
	}
}
