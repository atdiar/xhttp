package https

import (
	"net/http"

	"context"

	"github.com/atdiar/xhttp"
)

// Redirect is the handler which redirects all traffic stqrting with the http
// scheme to https. It just needs to be dropped in the handler chain.
type Redirect struct {
	next xhttp.Handler
}

func (re Redirect) ServeHTTP(ctx context.Context, w http.ResponseWriter, r *http.Request) {
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
