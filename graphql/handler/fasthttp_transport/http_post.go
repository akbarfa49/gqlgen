package fasthttp_transport

import (
	"bytes"
	"context"
	"mime"
	"net/http"

	"github.com/99designs/gqlgen/graphql"
	"github.com/valyala/fasthttp"
)

// POST implements the POST side of the default HTTP transport
// defined in https://github.com/APIs-guru/graphql-over-http#post
type POST struct{}

var _ graphql.FastTransport = POST{}

func (h POST) Supports(rctx *fasthttp.RequestCtx) bool {

	if string(rctx.Request.Header.Peek("Upgrade")) != "" {
		return false
	}

	mediaType, _, err := mime.ParseMediaType(string(rctx.Request.Header.Peek("Content-Type")))
	if err != nil {
		return false
	}

	return string(rctx.Method()) == "POST" && mediaType == "application/json"
}

func (h POST) Do(rctx *fasthttp.RequestCtx, graphCtx context.Context, exec graphql.GraphExecutor) {

	rctx.Response.Header.Set("Content-Type", "application/json")

	var params *graphql.RawParams
	start := graphql.Now()
	if err := jsonDecode(bytes.NewReader(rctx.Request.Body()), &params); err != nil {
		rctx.SetStatusCode(http.StatusBadRequest)
		writeJsonErrorf(rctx, "json body could not be decoded: "+err.Error())
		return
	}
	params.ReadTime = graphql.TraceTiming{
		Start: start,
		End:   graphql.Now(),
	}

	rc, err := exec.CreateOperationContext(graphCtx, params)
	if err != nil {
		rctx.SetStatusCode(statusFor(err))
		resp := exec.DispatchError(graphql.WithOperationContext(graphCtx, rc), err)
		writeJson(rctx, resp)
		return
	}
	responses, ctx := exec.DispatchOperation(graphCtx, rc)
	writeJson(rctx, responses(ctx))
}
