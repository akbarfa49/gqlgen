package fasthttp_transport_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	transport "github.com/99designs/gqlgen/graphql/handler/fasthttp_transport"
	"github.com/99designs/gqlgen/graphql/handler/testserver"
	"github.com/stretchr/testify/assert"
	"github.com/valyala/fasthttp"
)

func TestGET(t *testing.T) {
	h := testserver.NewFast()
	h.AddTransport(transport.GET{})

	lh := `localhost:` + randomPort()

	go func() {
		if err := fasthttp.ListenAndServe(lh, h.ServeHTTP); err != nil {
			panic(err)
		}

	}()
	time.Sleep(1 * time.Second)
	lh = `http://` + lh + `/`
	t.Run("success", func(t *testing.T) {
		resp := fastdoRequest(h, "GET", "/graphql?query={name}", ``)
		assert.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
		assert.Equal(t, `{"data":{"name":"test"}}`, resp.Body.String())
	})

	t.Run("has json content-type header", func(t *testing.T) {
		resp := fastdoRequest(h, "GET", "/graphql?query={name}", ``)
		assert.Equal(t, "application/json", resp.Header().Get("Content-Type"))
	})

	t.Run("decode failure", func(t *testing.T) {
		resp := fastdoRequest(h, "GET", "/graphql?query={name}&variables=notjson", "")
		assert.Equal(t, http.StatusBadRequest, resp.Code, resp.Body.String())
		assert.Equal(t, `{"errors":[{"message":"variables could not be decoded"}],"data":null}`, resp.Body.String())
	})

	t.Run("invalid variable", func(t *testing.T) {
		resp := fastdoRequest(h, "GET", `/graphql?query=query($id:Int!){find(id:$id)}&variables={"id":false}`, "")
		assert.Equal(t, http.StatusUnprocessableEntity, resp.Code, resp.Body.String())
		assert.Equal(t, `{"errors":[{"message":"cannot use bool as Int","path":["variable","id"],"extensions":{"code":"GRAPHQL_VALIDATION_FAILED"}}],"data":null}`, resp.Body.String())
	})

	t.Run("parse failure", func(t *testing.T) {
		resp := fastdoRequest(h, "GET", "/graphql?query=!", "")
		assert.Equal(t, http.StatusUnprocessableEntity, resp.Code, resp.Body.String())
		assert.Equal(t, `{"errors":[{"message":"Unexpected !","locations":[{"line":1,"column":1}],"extensions":{"code":"GRAPHQL_PARSE_FAILED"}}],"data":null}`, resp.Body.String())
	})

	t.Run("no mutations", func(t *testing.T) {
		resp := fastdoRequest(h, "GET", "/graphql?query=mutation{name}", "")
		assert.Equal(t, http.StatusNotAcceptable, resp.Code, resp.Body.String())
		assert.Equal(t, `{"errors":[{"message":"GET requests only allow query operations"}],"data":null}`, resp.Body.String())
	})
}

func fastdoRequest(handler *testserver.FastTestServer, method string, target string, body string) *httptest.ResponseRecorder {
	req := fasthttp.AcquireRequest()
	defer fasthttp.ReleaseRequest(req)
	req.Header.SetMethod(method)
	req.SetRequestURI(target)
	req.Header.Set("Content-Type", "application/json")
	req.SetBody([]byte(body))
	res := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(res)
	ctx := new(fasthttp.RequestCtx)
	ctx.Request = *req
	ctx.Response = *res
	handler.ServeHTTP(ctx)

	n := httptest.NewRecorder()
	n.Body = bytes.NewBuffer(ctx.Response.Body())
	n.WriteHeader(ctx.Response.StatusCode())
	n.Flushed = res.ImmediateHeaderFlush
	ctx.Response.Header.VisitAll(func(key, value []byte) {
		n.HeaderMap.Add(string(key), string(value))
	})

	return n
}
