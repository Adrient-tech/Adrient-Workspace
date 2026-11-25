package meet

import (
	"net/http"
	"net/http/httptest"
	"strings"
)

type reqPair struct {
	w *httptest.ResponseRecorder
	r *http.Request
}

func httpReq(body string) reqPair {
	return reqPair{httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))}
}
