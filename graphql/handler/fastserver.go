package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/executor"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	transport "github.com/99designs/gqlgen/graphql/handler/fasthttp_transport"
	"github.com/99designs/gqlgen/graphql/handler/lru"
	"github.com/valyala/fasthttp"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

type (
	FastServer struct {
		transports []graphql.FastTransport
		exec       *executor.Executor
	}
)

func NewFastServer(es graphql.ExecutableSchema) *FastServer {
	return &FastServer{
		exec: executor.New(es),
	}
}

func NewDefaultFastServer(es graphql.ExecutableSchema) *FastServer {
	srv := NewFastServer(es)

	srv.AddTransport(transport.Websocket{
		KeepAlivePingInterval: 10 * time.Second,
	})
	srv.AddTransport(transport.Options{})
	srv.AddTransport(transport.GET{})
	srv.AddTransport(transport.POST{})
	srv.AddTransport(transport.MultipartForm{})

	srv.SetQueryCache(lru.New(1000))

	srv.Use(extension.Introspection{})
	srv.Use(extension.AutomaticPersistedQuery{
		Cache: lru.New(100),
	})

	return srv
}

func (s *FastServer) AddTransport(transport graphql.FastTransport) {
	s.transports = append(s.transports, transport)
}

func (s *FastServer) SetErrorPresenter(f graphql.ErrorPresenterFunc) {
	s.exec.SetErrorPresenter(f)
}

func (s *FastServer) SetRecoverFunc(f graphql.RecoverFunc) {
	s.exec.SetRecoverFunc(f)
}

func (s *FastServer) SetQueryCache(cache graphql.Cache) {
	s.exec.SetQueryCache(cache)
}

func (s *FastServer) Use(extension graphql.HandlerExtension) {
	s.exec.Use(extension)
}

// AroundFields is a convenience method for creating an extension that only implements field middleware
func (s *FastServer) AroundFields(f graphql.FieldMiddleware) {
	s.exec.AroundFields(f)
}

// AroundRootFields is a convenience method for creating an extension that only implements field middleware
func (s *FastServer) AroundRootFields(f graphql.RootFieldMiddleware) {
	s.exec.AroundRootFields(f)
}

// AroundOperations is a convenience method for creating an extension that only implements operation middleware
func (s *FastServer) AroundOperations(f graphql.OperationMiddleware) {
	s.exec.AroundOperations(f)
}

// AroundResponses is a convenience method for creating an extension that only implements response middleware
func (s *FastServer) AroundResponses(f graphql.ResponseMiddleware) {
	s.exec.AroundResponses(f)
}

func (s *FastServer) getTransport(r *fasthttp.RequestCtx) graphql.FastTransport {
	for _, t := range s.transports {
		if t.Supports(r) {
			return t
		}
	}
	return nil
}

func (s *FastServer) ServeHTTP(ctx *fasthttp.RequestCtx) {
	defer func() {
		if err := recover(); err != nil {
			err := s.exec.PresentRecoveredError(ctx, err)
			resp := &graphql.Response{Errors: []*gqlerror.Error{err}}
			b, _ := json.Marshal(resp)
			ctx.SetStatusCode(fasthttp.StatusUnprocessableEntity)
			ctx.Write(b)
		}
	}()

	graphCtx := graphql.StartOperationTrace(ctx)

	transport := s.getTransport(ctx)
	if transport == nil {
		fastSendErrorf(ctx, http.StatusBadRequest, "transport not supported")
		return
	}

	transport.Do(ctx, graphCtx, s.exec)
}

func fastSendError(w *fasthttp.RequestCtx, code int, errors ...*gqlerror.Error) {

	w.SetStatusCode(code)
	b, err := json.Marshal(&graphql.Response{Errors: errors})
	if err != nil {
		panic(err)
	}
	w.Write(b)

}

func fastSendErrorf(w *fasthttp.RequestCtx, code int, format string, args ...interface{}) {
	fastSendError(w, code, &gqlerror.Error{Message: fmt.Sprintf(format, args...)})
}

//type OperationFunc func(ctx context.Context, next graphql.OperationHandler) graphql.ResponseHandler

// func (r OperationFunc) ExtensionName() string {
// 	return "InlineOperationFunc"
// }

// func (r OperationFunc) Validate(schema graphql.ExecutableSchema) error {
// 	if r == nil {
// 		return fmt.Errorf("OperationFunc can not be nil")
// 	}
// 	return nil
// }

// func (r OperationFunc) InterceptOperation(ctx context.Context, next graphql.OperationHandler) graphql.ResponseHandler {
// 	return r(ctx, next)
// }

// type ResponseFunc func(ctx context.Context, next graphql.ResponseHandler) *graphql.Response

// func (r ResponseFunc) ExtensionName() string {
// 	return "InlineResponseFunc"
// }

// func (r ResponseFunc) Validate(schema graphql.ExecutableSchema) error {
// 	if r == nil {
// 		return fmt.Errorf("ResponseFunc can not be nil")
// 	}
// 	return nil
// }

// func (r ResponseFunc) InterceptResponse(ctx context.Context, next graphql.ResponseHandler) *graphql.Response {
// 	return r(ctx, next)
// }

// type FieldFunc func(ctx context.Context, next graphql.Resolver) (res interface{}, err error)

// func (f FieldFunc) ExtensionName() string {
// 	return "InlineFieldFunc"
// }

// func (f FieldFunc) Validate(schema graphql.ExecutableSchema) error {
// 	if f == nil {
// 		return fmt.Errorf("FieldFunc can not be nil")
// 	}
// 	return nil
// }

// func (f FieldFunc) InterceptField(ctx context.Context, next graphql.Resolver) (res interface{}, err error) {
// 	return f(ctx, next)
// }
