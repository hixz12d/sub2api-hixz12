package service

import (
	"context"
	"errors"
)

type requestOriginContextKey struct{}

// WithRequestOrigin is for trusted server-side dispatchers, never HTTP headers
// or payload fields. Missing attribution means an ordinary business request.
func WithRequestOrigin(ctx context.Context, origin RequestOrigin) (context.Context, error) {
	if !origin.Valid() {
		return nil, errors.New("invalid request origin")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, requestOriginContextKey{}, origin), nil
}

func RequestOriginFromContext(ctx context.Context) RequestOrigin {
	if ctx != nil {
		if origin, ok := ctx.Value(requestOriginContextKey{}).(RequestOrigin); ok {
			return origin
		}
	}
	return RequestOriginBusiness
}
