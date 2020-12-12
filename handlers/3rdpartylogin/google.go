package login

import (
	"context"
	"net/http"

	"github.com/atdiar/xhttp/handlers/oauth2"
	"github.com/atdiar/xhttp/handlers/session"
	"golang.org/x/oauth2"
	"gopkg.in/dgrijalva/jwt-go.v3"
	//"google.golang.org/api/googleapi"
	//	oauth "google.golang.org/api/oauth2/v2"
	//"google.golang.org/api/option"
)

type GoogleProvider struct {
	Session     session.Handler
	QueryUser   func(ctx context.Context, userinfo interface{}) (string, error)
	CreateUser  func(ctx context.Context, userinfo interface{}) error
	RedirectURL string
	SignUpURL   string
}

func WithGoogle(s session.Handler, redirectURL string, signupURL string, queryUser func(ctx context.Context, userinfo interface{}) (string, error), createUser func(ctx context.Context, userinfo interface{}) error) GoogleProvider {
	return GoogleProvider{s, queryUser, createUser, redirectURL, signupURL}
}

func (g GoogleProvider) ServeHTTP(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	token, ok := ctx.Value(xoauth2.TokenKey).(*oauth2.Token)
	if !ok {
		http.Error(w, "Failed to sign new user. Token missing.", http.StatusInternalServerError)
		return
	}
	idtokstr, ok := token.Extra("id_token").(string)
	if !ok {
		http.Error(w, "Failed to sign new user. id token missing.", http.StatusInternalServerError)
		return
	}

	idtok, err := jwt.Parse(idtokstr, nil)
	if idtok == nil || err != nil {
		http.Error(w, "Failed to sign new user. id token invalid.", http.StatusInternalServerError)
		return
	}
	claims, ok := idtok.Claims.(jwt.MapClaims)
	if !ok {
		http.Error(w, "Failed to sign new user. id token invalid.", http.StatusInternalServerError)
		return
	}

	userinfo := make(map[string]interface{})
	googleID, ok := claims["sub"]
	email, ok2 := claims["email"]
	name := claims["name"]
	picture := claims["picture"]

	if !ok || !ok2 {
		http.Error(w, "Failed to sign new user. id token incomplete.", http.StatusInternalServerError)
		return
	}
	userinfo["googleid"] = googleID
	userinfo["email"] = email
	userinfo["name"] = name
	userinfo["picture"] = picture

	userid, err := g.QueryUser(ctx, userinfo)

	if err != nil {
		ctx, err = g.Session.Generate(ctx, w, r)
		if err != nil {
			http.Error(w, "Could not create user session: \n"+err.Error(), http.StatusInternalServerError)
		}
		id, err := g.Session.ID()
		if err != nil {
			g.Session.Cookie.Erase(ctx, w, r)
			// TODO revoke session ?
			http.Error(w, "Could not create user session ID: \n"+err.Error(), http.StatusInternalServerError)
		}
		userinfo["id"] = id
		err = g.CreateUser(ctx, userinfo)
		if err != nil {
			g.Session.Cookie.Erase(ctx, w, r)
			// TODO revoke session ?
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		http.Redirect(w, r, g.SignUpURL, http.StatusTemporaryRedirect)
		return
	}
	// TODO review user id should not be saved in cookie  sessionid may . user id and session id may have to be linked transitorily
	// g.Session.SetID(userid)
	g.Session.Put("userid", []byte(userid), 0)
	ctx, err = g.Session.Save(ctx, w, r)
	if err != nil {
		http.Error(w, "Could not load user session: \n"+err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, g.RedirectURL, http.StatusTemporaryRedirect)
	return
}

func (g GoogleProvider) Logout(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	g.Session.Cookie.Erase(ctx, w, r)
	// TODO revoke session
}
