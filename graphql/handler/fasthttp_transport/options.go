package fasthttp_transport

import (
	"context"

	"github.com/99designs/gqlgen/graphql"
	"github.com/valyala/fasthttp"
)

// Options responds to http OPTIONS and HEAD requests
type Options struct{}

var _ graphql.FastTransport = Options{}

func (o Options) Supports(ctx *fasthttp.RequestCtx) bool {

	return string(ctx.Method()) == "HEAD" || string(ctx.Method()) == "OPTIONS"
}

func (o Options) Do(ctx *fasthttp.RequestCtx, graphCtx context.Context, exec graphql.GraphExecutor) {
	switch string(ctx.Method()) {
	case fasthttp.MethodOptions:
		ctx.Response.Header.Set("Allow", "OPTIONS, GET, POST")

		ctx.SetStatusCode(fasthttp.StatusOK)
	case fasthttp.MethodHead:
		ctx.SetStatusCode(fasthttp.StatusMethodNotAllowed)
	}
}
