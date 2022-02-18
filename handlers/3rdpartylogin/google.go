package login

import (
	"context"
	"net/http"
	"encoding/json"

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
	QueryUser   func(ctx context.Context, userinfo map[string]interface{}) (map[string]string, error)
	RedirectURL string
	NewUSerURL   string
}

func WithGoogle(s session.Handler, redirectURL string, createNewUserURL string, queryUser func(ctx context.Context, provideruserinfo map[string]interface{}) (dbuserinfo map[string]string, err error)) GoogleProvider {
	return GoogleProvider{s, queryUser, redirectURL, createNewUserURL}
}

func (g GoogleProvider) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	token, ok := r.Context().Value(xoauth2.TokenKey).(*oauth2.Token)
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

	// Let's generate an authenticated session
	err= g.Session.Generate(w,r)
	if err!= nil{
		http.Error(w,"Unable to create authenticated session", http.StatusInternalServerError)
		return
	}
	rawuserinfo,err:= json.Marshal(userinfo)
	if err!= nil{
		http.Error(w,"Unable to create authenticated session", http.StatusInternalServerError)
		return
	}
	err = g.Session.Put(r.Context(),"user",rawuserinfo,0)
	if err!= nil{
		http.Error(w,"Unable to save authenticated user info in session", http.StatusInternalServerError)
		return
	}

	// Let's query user  in the database
	user, err := g.QueryUser(r.Context(), userinfo)
	if err != nil {
		http.Redirect(w, r, g.NewUSerURL, http.StatusTemporaryRedirect)
		return
	}
	rawuser,err:= json.Marshal(user)
	if err!= nil{
		http.Error(w,"Unable to create authenticated session", http.StatusInternalServerError)
		return
	}

	err = g.Session.Put(r.Context(),"user", rawuser, 0)
	if err!= nil{
		http.Error(w,"Unable to save authenticated user in session", http.StatusInternalServerError)
		return
	}
	err = g.Session.Save(w, r)
	if err != nil {
		http.Error(w, "Could not save authenticated user session.", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, g.RedirectURL, http.StatusTemporaryRedirect)
	return
}

func (g GoogleProvider) Close(w http.ResponseWriter, r *http.Request) {
	g.Session.Cookie.Erase(w, r)
	// TODO revoke session
}
