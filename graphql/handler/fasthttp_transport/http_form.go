package fasthttp_transport

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"io/ioutil"
	"mime"
	"os"

	"github.com/99designs/gqlgen/graphql"
	"github.com/valyala/fasthttp"
)

// MultipartForm the Multipart request spec https://github.com/jaydenseric/graphql-multipart-request-spec
type MultipartForm struct {
	// MaxUploadSize sets the maximum number of bytes used to parse a request body
	// as multipart/form-data.
	MaxUploadSize int64

	// MaxMemory defines the maximum number of bytes used to parse a request body
	// as multipart/form-data in memory, with the remainder stored on disk in
	// temporary files.
	MaxMemory int64
}

var _ graphql.FastTransport = MultipartForm{}

func (f MultipartForm) Supports(rctx *fasthttp.RequestCtx) bool {
	if string(rctx.Request.Header.Peek(`Upgrade`)) != `` {
		return false
	}

	mediaType, _, err := mime.ParseMediaType(string(rctx.Request.Header.Peek(`Content-Type`)))
	if err != nil {
		return false
	}

	return string(rctx.Method()) == `POST` && mediaType == `multipart/form-data`
}

func (f MultipartForm) maxUploadSize() int64 {
	if f.MaxUploadSize == 0 {
		return 32 << 20
	}
	return f.MaxUploadSize
}

func (f MultipartForm) maxMemory() int64 {
	if f.MaxMemory == 0 {
		return 32 << 20
	}
	return f.MaxMemory
}

func (f MultipartForm) Do(rctx *fasthttp.RequestCtx, graphCtx context.Context, exec graphql.GraphExecutor) {
	rctx.Response.Header.Set(`Content-Type`, `application/json`)

	start := graphql.Now()

	var err error
	if int64(rctx.Request.Header.ContentLength()) > f.maxUploadSize() {
		writeJsonError(rctx, `failed to parse multipart form, request body too large`)
		return
	}

	// mupart, err := rctx.MultipartForm()

	// if err != nil {
	// 	rctx.SetStatusCode(fasthttp.StatusUnprocessableEntity)
	// 	if strings.Contains(err.Error(), `request body too large`) {
	// 		writeJsonError(rctx, `failed to parse multipart form, request body too large`)
	// 		return
	// 	}
	// 	writeJsonError(rctx, `failed to parse multipart form`)
	// 	return
	// }

	var params graphql.RawParams

	if err = jsonDecode(bytes.NewReader(rctx.FormValue(`operations`)), &params); err != nil {
		rctx.SetStatusCode(fasthttp.StatusUnprocessableEntity)
		writeJsonError(rctx, `operations form field could not be decoded`)
		return
	}

	uploadsMap := map[string][]string{}

	if err = json.Unmarshal(rctx.FormValue(`map`), &uploadsMap); err != nil {
		rctx.SetStatusCode(fasthttp.StatusUnprocessableEntity)
		writeJsonError(rctx, `map form field could not be decoded`)
		return
	}

	var upload graphql.Upload
	for key, paths := range uploadsMap {
		if len(paths) == 0 {
			rctx.SetStatusCode(fasthttp.StatusUnprocessableEntity)
			writeJsonErrorf(rctx, `invalid empty operations paths list for key %s`, key)
			return
		}

		fh, err := rctx.FormFile(key)
		if err != nil {
			rctx.SetStatusCode(fasthttp.StatusUnprocessableEntity)
			writeJsonErrorf(rctx, `failed to get key %s from form`, key)
			return
		}
		fl, err := fh.Open()
		if err != nil {
			rctx.SetStatusCode(fasthttp.StatusUnprocessableEntity)
			writeJsonErrorf(rctx, `failed to open file %s`, fh.Filename)
			return
		}
		defer fl.Close()

		if len(paths) == 1 {
			upload = graphql.Upload{
				File:        fl,
				Size:        fh.Size,
				Filename:    fh.Filename,
				ContentType: fh.Header.Get(`Content-Type`),
			}

			if err := params.AddUpload(upload, key, paths[0]); err != nil {
				rctx.SetStatusCode(fasthttp.StatusUnprocessableEntity)
				writeJsonGraphqlError(rctx, err)
				return
			}
		} else {
			if int64(rctx.Request.Header.ContentLength()) < f.maxMemory() {
				fileBytes, err := ioutil.ReadAll(fl)
				if err != nil {
					rctx.SetStatusCode(fasthttp.StatusUnprocessableEntity)
					writeJsonErrorf(rctx, `failed to read file for key %s`, key)
					return
				}
				for _, path := range paths {
					upload = graphql.Upload{
						File:        &bytesReader{s: &fileBytes, i: 0, prevRune: -1},
						Size:        fh.Size,
						Filename:    fh.Filename,
						ContentType: fh.Header.Get(`Content-Type`),
					}

					if err := params.AddUpload(upload, key, path); err != nil {
						rctx.SetStatusCode(fasthttp.StatusUnprocessableEntity)
						writeJsonGraphqlError(rctx, err)
						return
					}
				}
			} else {
				tmpFile, err := ioutil.TempFile(os.TempDir(), `gqlgen-`)
				if err != nil {
					rctx.SetStatusCode(fasthttp.StatusUnprocessableEntity)
					writeJsonErrorf(rctx, `failed to create temp file for key %s`, key)
					return
				}
				tmpName := tmpFile.Name()
				defer func() {
					_ = os.Remove(tmpName)
				}()
				_, err = io.Copy(tmpFile, fl)
				if err != nil {
					rctx.SetStatusCode(fasthttp.StatusUnprocessableEntity)
					if err := tmpFile.Close(); err != nil {
						writeJsonErrorf(rctx, `failed to copy to temp file and close temp file for key %s`, key)
						return
					}
					writeJsonErrorf(rctx, `failed to copy to temp file for key %s`, key)
					return
				}
				if err := tmpFile.Close(); err != nil {
					rctx.SetStatusCode(fasthttp.StatusUnprocessableEntity)
					writeJsonErrorf(rctx, `failed to close temp file for key %s`, key)
					return
				}
				for _, path := range paths {
					pathTmpFile, err := os.Open(tmpName)
					if err != nil {
						rctx.SetStatusCode(fasthttp.StatusUnprocessableEntity)
						writeJsonErrorf(rctx, `failed to open temp file for key %s`, key)
						return
					}
					defer pathTmpFile.Close()
					upload = graphql.Upload{
						File:        pathTmpFile,
						Size:        fh.Size,
						Filename:    fh.Filename,
						ContentType: fh.Header.Get(`Content-Type`),
					}

					if err := params.AddUpload(upload, key, path); err != nil {
						rctx.SetStatusCode(fasthttp.StatusUnprocessableEntity)
						writeJsonGraphqlError(rctx, err)
						return
					}
				}
			}
		}
	}

	params.ReadTime = graphql.TraceTiming{
		Start: start,
		End:   graphql.Now(),
	}

	rc, gerr := exec.CreateOperationContext(graphCtx, &params)
	if gerr != nil {
		resp := exec.DispatchError(graphql.WithOperationContext(graphCtx, rc), gerr)
		rctx.SetStatusCode(statusFor(gerr))
		writeJson(rctx, resp)
		return
	}
	responses, ctx := exec.DispatchOperation(graphCtx, rc)
	writeJson(rctx, responses(ctx))
}
