// Copyright 2017 Alexander Zaytsev <thebestzorro@yandex.ru>.
// All rights reserved. Use of this source code is governed
// by a BSD-style license that can be found in the LICENSE file.

//Package web contains main web and API handlers.
package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"net/url"
	"path/filepath"

	"github.com/garyburd/redigo/redis"
	"github.com/z0rr0/lruss/conf"
	"github.com/z0rr0/lruss/trim"
)

const (
	// pathKey is a context key for path.
	pathKey key = "pathKey"
)

var (
	// dbPrefixes is databases prefixes for special variables.
	dbPrefixes = map[string]string{
		"count": "count",
		"host":  "host",
		"tpl":   "tpl",
		"url":   "url",
	}
)

type key string

// Response is API response for URL shorting.
type Response struct {
	URL   string `json:"url"`
	Short string `json:"short"`
}

// dbKey returns a db key including special prefix.
func dbKey(prefix, value string) (string, error) {
	dbPrefix, ok := dbPrefixes[prefix]
	if !ok {
		return "", errors.New("not found db key")
	}
	return fmt.Sprintf("%v:%v", dbPrefix, value), nil
}

// httpError returns error based on text HTTP status code.
func httpError(status int) (int, error) {
	return status, errors.New(http.StatusText(status))
}

// allowedRate checks minute's rate for host address.
func allowedRate(r *http.Request, c redis.Conn, cfg *conf.Cfg) (bool, error) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false, err
	}
	if cfg.Rate.CheckUserAgent {
		host = fmt.Sprintf("%v:ua:%v", host, r.UserAgent())
	}
	hostKey, err := dbKey("host", host)
	if err != nil {
		return false, err
	}
	hostRate, err := redis.Int64(c.Do("INCR", hostKey))
	if err != nil {
		return false, err
	}
	if hostRate == 1 {
		_, err = c.Do("EXPIRE", hostKey, cfg.Rate.Interval)
		if err != nil {
			return false, err
		}
		return true, nil
	}
	if hostRate < cfg.Rate.Count {
		return true, nil
	}
	return false, nil
}

// ResetTplCache resets template cache.
func ResetTplCache(cfg *conf.Cfg) error {
	c := cfg.GetConn()
	defer c.Close()

	tplKey, err := dbKey("tpl", "index.html")
	if err != nil {
		return err
	}
	_, err = c.Do("DEL", tplKey)
	if err == redis.ErrNil {
		return nil
	}
	return err
}

// SetContext writes path to context.
func SetContext(ctx context.Context, path string) context.Context {
	return context.WithValue(ctx, pathKey, path)
}

// GetContext reads path from context.
func GetContext(ctx context.Context) (string, error) {
	c, ok := ctx.Value(pathKey).(string)
	if !ok {
		return "", errors.New("path not found")
	}
	return c, nil
}

// HandleAPI handles API request to get shor url.
func HandleAPI(ctx context.Context, w http.ResponseWriter, r *http.Request) (int, error) {
	cfg, err := conf.GetContext(ctx)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	originURL := r.FormValue("url")
	if originURL == "" {
		return http.StatusBadRequest, errors.New("not url")
	}
	u, err := url.Parse(originURL)
	if err != nil {
		return http.StatusBadRequest, errors.New("invalid url")
	}
	if !u.IsAbs() {
		return http.StatusBadRequest, errors.New("not absolute url")
	}

	c := cfg.GetConn()
	defer c.Close()

	if cfg.Rate.Active {
		allowed, err := allowedRate(r, c, cfg)
		if err != nil {
			return http.StatusInternalServerError, err
		}
		if !allowed {
			return httpError(http.StatusTooManyRequests)
		}
	}
	countKey, err := dbKey("count", "count")
	if err != nil {
		return http.StatusInternalServerError, err
	}
	num, err := redis.Int64(c.Do("INCR", countKey))
	if err != nil {
		return http.StatusInternalServerError, err
	}
	short := trim.Encode(num)
	response := &Response{URL: u.String(), Short: cfg.ShortURL(short)}

	urlKey, err := dbKey("url", short)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	_, err = c.Do("SET", urlKey, response.URL)
	if err != nil {
		return httpError(http.StatusServiceUnavailable)
	}
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	encoder := json.NewEncoder(w)
	if err := encoder.Encode(response); err != nil {
		return http.StatusInternalServerError, err
	}
	return http.StatusOK, nil
}

// HandleRedirect finds short URL and redirects a request.
func HandleRedirect(ctx context.Context, w http.ResponseWriter, r *http.Request) (int, error) {
	cfg, err := conf.GetContext(ctx)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	short, err := GetContext(ctx)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	c := cfg.GetConn()
	defer c.Close()

	urlKey, err := dbKey("url", short)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	originURL, err := redis.String(c.Do("GET", urlKey))
	if err != nil {
		if err != redis.ErrNil {
			return httpError(http.StatusServiceUnavailable)
		}
		return httpError(http.StatusNotFound)
	}
	status := http.StatusFound
	http.Redirect(w, r, originURL, status)
	return status, nil
}

// HandleHTML returns an index HTML page.
func HandleHTML(ctx context.Context, w http.ResponseWriter, r *http.Request) (int, error) {
	var (
		data   interface{}
		tpl    *template.Template
		buffer bytes.Buffer
	)
	cfg, err := conf.GetContext(ctx)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	c := cfg.GetConn()
	defer c.Close()

	tplKey, err := dbKey("tpl", "index.html")
	tplString, err := redis.String(c.Do("GET", tplKey))
	if err != nil {
		tpl, err = template.ParseFiles(
			filepath.Join(cfg.Static, "base.html"),
			filepath.Join(cfg.Static, "index.html"),
		)
		if err != nil {
			return http.StatusInternalServerError, err
		}
		// write to cache
		err = tpl.ExecuteTemplate(&buffer, "base", data)
		if err != nil {
			return http.StatusInternalServerError, err
		}
		tplString = buffer.String()
		_, err = c.Do("SET", tplKey, tplString)
		if err != nil {
			return http.StatusInternalServerError, err
		}
	}
	// http response
	_, err = fmt.Fprint(w, tplString)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	return http.StatusOK, nil
}
