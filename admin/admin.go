// Copyright 2017 Alexander Zaytsev <thebestzorro@yandex.ru>.
// All rights reserved. Use of this source code is governed
// by s BSD-style license that can be found in the LICENSE file.

// Package admin contains HTTP administration methods.
// Also it includes authentication functions.
package admin

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"encoding/base64"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/garyburd/redigo/redis"
	"github.com/z0rr0/lruss/conf"
	"github.com/z0rr0/lruss/trim"
	"golang.org/x/crypto/bcrypt"
)

const (
	// CSRFTokenName is common CSRF token cookie name.
	CSRFTokenName = "csrftoken"
	// Anonymous is s fake username for anonymous users.
	Anonymous = "anonymous"
	// SessionCookie is cookie name of user authentication session.
	SessionCookie = "session"
	// urlPrefix is length of system short url prefix
	urlPrefix = len("url:")

	// userKey is internal user context key.
	userKey key = "user"
	// csrfLen is CSRF token bytes length.
	csrfLen = 128
	// sessionLen is length of session random key (bytes).
	sessionLen = 128
	// passLen is admin user password length (bytes)
	passLen = 12
)

// key is internal context key.
type key string

// loginForm is login page form data struct.
type loginForm struct {
	User     string
	Password string
	Msg      string
	CSRF     string
	Failed   bool
}

// statistics is administration statistics struct.
type statistics struct {
	LastNum  int64
	LastURL  string
	Sessions string
	CSRF     string
	Locks    []string
}

type shortURLs []string

func (s shortURLs) Len() int      { return len(s) }
func (s shortURLs) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s shortURLs) Less(i, j int) bool {
	di, _ := trim.Decode(s[i][urlPrefix:])
	dj, _ := trim.Decode(s[j][urlPrefix:])
	return di < dj
}

// SetContext writes settings to context.
func SetContext(ctx context.Context, username string) context.Context {
	return context.WithValue(ctx, userKey, username)
}

// GetContext reads settings from context.
func GetContext(ctx context.Context) string {
	username, ok := ctx.Value(userKey).(string)
	if !ok {
		return Anonymous
	}
	return username
}

// GetCSRF returns s CSRF (Сross Site Request Forgery) token.
func GetCSRF(ctx context.Context) (string, error) {
	user := GetContext(ctx)
	cfg, err := conf.GetContext(ctx)
	if err != nil {
		return "", err
	}
	c := cfg.GetConn()
	defer c.Close()

	csrfKey, err := conf.DbKey("csrf", user)
	if err != nil {
		return "", err
	}
	token, err := redis.String(c.Do("GET", csrfKey))
	if (err != nil) && (err != redis.ErrNil) {
		return "", err
	}
	if err == redis.ErrNil {
		// set new token
		b := make([]byte, csrfLen)
		_, err = rand.Read(b)
		if err != nil {
			return "", err
		}
		token = hex.EncodeToString(b)
		// set only if other goroutines don't do it before
		resp, err := redis.Int(c.Do("SETNX", csrfKey, token))
		if err != nil {
			return "", err
		}
		if resp == 1 {
			_, err = c.Do("EXPIRE", csrfKey, cfg.CSRFTimeout)
			if err != nil {
				return "", err
			}
		}
	}
	return token, nil
}

// CheckCSRF verifies s CSRF token.
func CheckCSRF(ctx context.Context, token string) (bool, error) {
	user := GetContext(ctx)
	cfg, err := conf.GetContext(ctx)
	if err != nil {
		return false, err
	}
	c := cfg.GetConn()
	defer c.Close()

	csrfKey, err := conf.DbKey("csrf", user)
	if err != nil {
		return false, err
	}
	value, err := redis.String(c.Do("GET", csrfKey))
	switch {
	case err == redis.ErrNil:
		value = "not found"
	case err != nil:
		return false, err
	}
	// compare two values for equality without leaking timing information
	return hmac.Equal([]byte(token), []byte(value)), nil
}

// Auth checks user's authentication and saves username to the ctx context.
// Session cookie is to be have s value like "username::xxxxx".
func Auth(ctx context.Context, r *http.Request) (context.Context, error) {
	cookie, err := r.Cookie(SessionCookie)
	if err != nil {
		return SetContext(ctx, Anonymous), err
	}
	cfg, err := conf.GetContext(ctx)
	if err != nil {
		return ctx, err
	}
	// expected: values[0] is username, values[1] is session key
	values := strings.SplitN(cookie.Value, "::", 2)
	if len(values) < 2 {
		return ctx, errors.New("invalid cookie value")
	}
	c := cfg.GetConn()
	defer c.Close()
	sessionKey, err := conf.DbKey("session", values[0])
	if err != nil {
		return ctx, err
	}
	found, err := redis.Int(c.Do("SISMEMBER", sessionKey, values[1]))
	if err != nil {
		return ctx, err
	}
	if found == 0 {
		return ctx, errors.New("invalid session key")
	}
	return SetContext(ctx, values[0]), nil
}

// setSession generates and sets new session key, after that creates s new session cookie.
func setSession(w http.ResponseWriter, c redis.Conn, secure bool, username string) error {
	sessionKey, err := conf.DbKey("session", username)
	if err != nil {
		return err
	}
	b := make([]byte, sessionLen)
	_, err = rand.Read(b)
	if err != nil {
		return err
	}
	value := base64.StdEncoding.EncodeToString(b)
	_, err = c.Do("SADD", sessionKey, value)
	if err != nil {
		return err
	}
	cookie := http.Cookie{
		Name:     SessionCookie,
		Value:    fmt.Sprintf("%v::%v", username, value),
		MaxAge:   0,
		HttpOnly: true,
		Path:     "/",
		Secure:   secure,
	}
	http.SetCookie(w, &cookie)
	return nil
}

// CreateOrUpdate creates new user/password pair or updates s current user if it already exists.
func CreateOrUpdate(cfg *conf.Cfg, username string) (string, bool, error) {
	c := cfg.GetConn()
	defer c.Close()

	usernameKey, err := conf.DbKey("user", username)
	if err != nil {
		return "", false, err
	}
	b := make([]byte, passLen)
	_, err = rand.Read(b)
	if err != nil {
		return "", false, err
	}
	h, err := bcrypt.GenerateFromPassword(b, bcrypt.DefaultCost)
	if err != nil {
		return "", false, err
	}
	created, err := redis.Int(c.Do("HSET", usernameKey, "password", hex.EncodeToString(h)))
	if err != nil {
		return "", false, err
	}
	return hex.EncodeToString(b), created == 1, nil
}

// CheckPassword verifies s password of user with name username.
func CheckPassword(c redis.Conn, username, password string) error {
	usernameKey, err := conf.DbKey("user", username)
	if err != nil {
		return err
	}
	b, err := hex.DecodeString(password)
	if err != nil {
		return err
	}
	if len(b) != passLen {
		return errors.New("too short password")
	}
	hash, err := redis.String(c.Do("HGET", usernameKey, "password"))
	if err != nil {
		return err
	}
	h, err := hex.DecodeString(hash)
	if err != nil {
		return err
	}
	return bcrypt.CompareHashAndPassword(h, b)
}

// renderLogin prepares login page template.
func renderLogin(w http.ResponseWriter, f *loginForm, static string) error {
	tpl, err := template.ParseFiles(
		filepath.Join(static, "base.html"),
		filepath.Join(static, "login.html"),
	)
	if err != nil {
		return err
	}
	return tpl.ExecuteTemplate(w, "base", f)
}

// Login shows auth form and checks users' credentials.
func Login(ctx context.Context, w http.ResponseWriter, r *http.Request) (int, error) {
	cfg, err := conf.GetContext(ctx)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	username := GetContext(ctx)
	if username != Anonymous {
		// user is authenticated
		http.Redirect(w, r, "/admin/index/", http.StatusFound)
		return http.StatusFound, nil
	}
	csrfValue, err := GetCSRF(ctx)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	form := &loginForm{CSRF: csrfValue}
	if r.Method == "POST" {
		// csrf is already checked
		form.User, form.Password = r.PostFormValue("user"), r.PostFormValue("password")
		c := cfg.GetConn()
		defer c.Close()

		err = CheckPassword(c, form.User, form.Password)
		if err == nil {
			// authenticated
			secure, err := cfg.IsSecure()
			if err != nil {
				return http.StatusInternalServerError, err
			}
			err = setSession(w, c, secure, form.User)
			if err != nil {
				return http.StatusInternalServerError, err
			}
			http.Redirect(w, r, "/admin/index/", http.StatusFound)
			return http.StatusFound, nil
		}
		form.Msg = "mismatch user or password"
	}
	err = renderLogin(w, form, cfg.Static)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	return http.StatusOK, nil
}

// Logout is used to finish administration session.
func Logout(ctx context.Context, w http.ResponseWriter, r *http.Request) (int, error) {
	cfg, err := conf.GetContext(ctx)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	username := GetContext(ctx)
	if username == Anonymous {
		// user is not authenticated
		http.Redirect(w, r, "/", http.StatusFound)
		return http.StatusFound, nil
	}
	c := cfg.GetConn()
	defer c.Close()

	cookie, err := r.Cookie(SessionCookie)
	if err != nil {
		// impossible case
		http.Redirect(w, r, "/", http.StatusFound)
		return http.StatusFound, nil
	}
	values := strings.SplitN(cookie.Value, "::", 2)
	if len(values) < 2 {
		http.Redirect(w, r, "/", http.StatusFound)
		return http.StatusFound, nil
	}
	sessionKey, err := conf.DbKey("session", username)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	// remove session from db
	_, err = c.Do("SREM", sessionKey, values[1])
	if err != nil {
		return http.StatusInternalServerError, err
	}
	secure, err := cfg.IsSecure()
	if err != nil {
		return http.StatusInternalServerError, err
	}
	// remove session cookie
	cookie = &http.Cookie{
		Name:     SessionCookie,
		Value:    "",
		MaxAge:   -1,
		HttpOnly: true,
		Path:     "/",
		Secure:   secure,
	}
	http.SetCookie(w, cookie)
	http.Redirect(w, r, "/admin/login/", http.StatusFound)
	return http.StatusFound, nil
}

// Index returns admin statistics page.
func Index(ctx context.Context, w http.ResponseWriter, r *http.Request) (int, error) {
	cfg, err := conf.GetContext(ctx)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	csrfValue, err := GetCSRF(ctx)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	data := statistics{CSRF: csrfValue}
	c := cfg.GetConn()
	defer c.Close()

	// last link
	dbKey, err := conf.DbKey("count", "count")
	if err != nil {
		return http.StatusInternalServerError, err
	}
	n, err := redis.Int64(c.Do("GET", dbKey))
	if err != nil {
		return http.StatusInternalServerError, err
	}
	short := trim.Encode(n)
	data.LastNum, data.LastURL = n, cfg.ShortURL(short)

	// sessions
	dbKey, err = conf.DbKey("session", "*")
	if err != nil {
		return http.StatusInternalServerError, err
	}
	keys, err := redis.Strings(c.Do("KEYS", dbKey))
	if err != nil {
		return http.StatusInternalServerError, err
	}
	sessions := make([]string, len(keys))
	for i, key := range keys {
		user := key[len(dbKey)-1:]
		s, err := redis.Strings(c.Do("SMEMBERS", key))
		if err != nil {
			return http.StatusInternalServerError, err
		}
		sessions[i] = fmt.Sprintf("%v (%d)", user, len(s))
	}
	data.Sessions = strings.Join(sessions, ", ")

	// locks
	dbKey, err = conf.DbKey("host", "*")
	if err != nil {
		return http.StatusInternalServerError, err
	}
	keys, err = redis.Strings(c.Do("KEYS", dbKey))
	if err != nil {
		return http.StatusInternalServerError, err
	}
	locks := make([]string, len(keys))
	for i, key := range keys {
		ttl, err := redis.Int(c.Do("TTL", key))
		if err != nil {
			return http.StatusInternalServerError, err
		}
		locks[i] = fmt.Sprintf("[%v sec.] %v", ttl, key[len(dbKey)-1:])
	}
	data.Locks = locks

	// render template
	tpl, err := template.ParseFiles(
		filepath.Join(cfg.Static, "base.html"),
		filepath.Join(cfg.Static, "admin_index.html"),
	)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	err = tpl.ExecuteTemplate(w, "base", data)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	return http.StatusOK, nil
}

// Export returns CSV data of all handled URLs.
func Export(ctx context.Context, w http.ResponseWriter, r *http.Request) (int, error) {
	const layout = "20060102_150405"

	cfg, err := conf.GetContext(ctx)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	c := cfg.GetConn()
	defer c.Close()

	dbKey, err := conf.DbKey("url", "*")
	if err != nil {
		return http.StatusInternalServerError, err
	}
	keys, err := redis.Strings(c.Do("KEYS", dbKey))
	if err != nil {
		return http.StatusInternalServerError, err
	}
	sort.Sort(shortURLs(keys))
	w.Header().Set(
		"Content-disposition",
		fmt.Sprintf("attachment; filename=\"lruss_export_%v.csv\"", time.Now().UTC().Format(layout)),
	)
	w.Header().Set("Content-Type", "text/csv")
	wCSV := csv.NewWriter(w)
	err = wCSV.Write([]string{"short", "origin"})
	if err != nil {
		return http.StatusInternalServerError, err
	}
	for _, key := range keys {
		originURL, err := redis.String(c.Do("GET", key))
		if err != nil {
			return http.StatusInternalServerError, err
		}
		err = wCSV.Write([]string{cfg.ShortURL(key[urlPrefix:]), originURL})
		if err != nil {
			return http.StatusInternalServerError, err
		}
	}
	wCSV.Flush()
	err = wCSV.Error()
	if err != nil {
		return http.StatusInternalServerError, err
	}
	return http.StatusOK, nil
}
