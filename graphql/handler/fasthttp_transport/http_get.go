package fasthttp_transport

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/errcode"
	"github.com/valyala/fasthttp"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

// GET implements the GET side of the default HTTP transport
// defined in https://github.com/APIs-guru/graphql-over-http#get
type GET struct{}

var _ graphql.FastTransport = GET{}

func (h GET) Supports(ctx *fasthttp.RequestCtx) bool {

	if string(ctx.Request.Header.Peek("Upgrade")) != "" {
		return false
	}

	return string(ctx.Method()) == "GET"
}

func (h GET) Do(ctx *fasthttp.RequestCtx, graphCtx context.Context, exec graphql.GraphExecutor) {
	ctx.Response.Header.Set("Content-Type", "application/json")

	raw := &graphql.RawParams{
		Query:         string(ctx.URI().QueryArgs().Peek("query")),
		OperationName: string(ctx.URI().QueryArgs().Peek("operationName")),
	}
	raw.ReadTime.Start = graphql.Now()

	if variables := string(ctx.URI().QueryArgs().Peek("variables")); variables != "" {
		if err := jsonDecode(strings.NewReader(variables), &raw.Variables); err != nil {
			ctx.SetStatusCode(fasthttp.StatusBadRequest)
			writeJsonError(ctx, "variables could not be decoded")
			return
		}
	}

	if extensions := string(ctx.URI().QueryArgs().Peek("extensions")); extensions != "" {
		if err := jsonDecode(strings.NewReader(extensions), &raw.Extensions); err != nil {
			ctx.SetStatusCode(fasthttp.StatusBadRequest)
			writeJsonError(ctx, "extensions could not be decoded")
			return
		}
	}

	raw.ReadTime.End = graphql.Now()

	rc, err := exec.CreateOperationContext(graphCtx, raw)
	if err != nil {
		ctx.SetStatusCode(statusFor(err))
		resp := exec.DispatchError(graphql.WithOperationContext(graphCtx, rc), err)
		writeJson(ctx, resp)
		return
	}
	op := rc.Doc.Operations.ForName(rc.OperationName)
	if op.Operation != ast.Query {
		ctx.SetStatusCode(fasthttp.StatusNotAcceptable)
		writeJsonError(ctx, "GET requests only allow query operations")
		return
	}

	responses, ctxPr := exec.DispatchOperation(graphCtx, rc)
	writeJson(ctx, responses(ctxPr))
}

func jsonDecode(r io.Reader, val interface{}) error {
	dec := json.NewDecoder(r)
	dec.UseNumber()
	return dec.Decode(val)
}

func statusFor(errs gqlerror.List) int {
	switch errcode.GetErrorKind(errs) {
	case errcode.KindProtocol:
		return http.StatusUnprocessableEntity
	default:
		return http.StatusOK
	}
}
