package signup

import (
	"context"
	"database/sql"
	"net/http"

	"github.com/atdiar/xhttp"
	"github.com/atdiar/xhttp/handlers/usersigning"
	"golang.org/x/oauth2"
	"google.golang.org/api/googleapi"
	oauth "google.golang.org/api/oauth2/v2"
	"google.golang.org/api/option"
)

// WithGoogle is used to configure the usersigning Handler into a singup handlers
// for user authentication via the google oAuth services.
func WithGoogle(DBCreateUserFunc func(userinfo interface{}) (sql.Result, error)) func(usersigning.Handler) usersigning.Handler {
	return func(s usersigning.Handler) usersigning.Handler {
		s.Handler = xhttp.HandlerFunc(func(ctx context.Context, w http.ResponseWriter, r *http.Request) {
			HTTPClient, ok := ctx.Value(oauth2.HTTPClient).(*http.Client)
			service, err := oauth.NewService(ctx)
			if ok {
				service, err = oauth.NewService(ctx, option.WithHTTPClient(HTTPClient))
			}
			if err != nil {
				http.Error(w, "Failed to instantiate oauth2 service", http.StatusInternalServerError)
				if s.Log != nil {
					s.Log.Print(err)
				}
				return
			}
			userinfoserv := oauth.NewUserinfoV2MeService(service)
			uigetcall := userinfoserv.Get()
			uigetcall = uigetcall.Context(ctx)
			var userinfo *oauth.Userinfoplus
			id, ok := s.Session.Cookie.ID()
			if !ok {
				if s.Log != nil {
					s.Log.Print(err)
				}
				http.Error(w, "GOOGL/SIGNUP: Bad session: expired id", http.StatusInternalServerError)
				return
			}
			userinfo, err = uigetcall.Do(googleapi.QuotaUser(id))
			if err != nil {
				if s.Log != nil {
					s.Log.Print(err)
				}
				http.Error(w, "Unable to access Google profile information.", http.StatusInternalServerError)
			}
			_, err = DBCreateUserFunc(userinfo)
			if err != nil {
				http.Error(w, "Failed to signup new user.", http.StatusInternalServerError)
				if s.Log != nil {
					s.Log.Print(err)
				}
				return
			}
		})
		return s
	}
}
