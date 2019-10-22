package https

import (
	"net/http"

	"github.com/atdiar/goroutine/execution"
	"github.com/atdiar/xhttp"
)

type Redirect struct {
	next xhttp.Handler
}

func (re Redirect) ServeHTTP(ctx execution.Context, w http.ResponseWriter, r *http.Request) {
	url := r.URL
	sch := url.Scheme
	if sch == "https" {
		if re.next != nil {
			re.next.ServeHTTP(ctx, w, r)
		}
		return
	}
	url.Scheme = "https"
	http.Redirect(w, r, url.String(), http.StatusTemporaryRedirect)
}

func (re Redirect) Link(h xhttp.Handler) xhttp.HandlerLinker {
	re.next = h
	return re
}
