// Copyright 2017 Alexander Zaytsev <thebestzorro@yandex.ru>.
// All rights reserved. Use of this source code is governed
// by a BSD-style license that can be found in the LICENSE file.

//Package web contains main web and API handlers.
package web

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"

	"github.com/garyburd/redigo/redis"
	"github.com/z0rr0/lruss/conf"
	"github.com/z0rr0/lruss/trim"
)

const (
	// countKey is common db counter.
	countKey = "count"
	// pathKey is a context key for path.
	pathKey key = "pathKey"
)

type key string

// Response is API response for URL shorting.
type Response struct {
	URL   string `json:"url"`
	Short string `json:"short"`
}

// allowedRate checks minute's rate for host address.
func allowedRate(r *http.Request, c redis.Conn, rate int64) (bool, error) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false, err
	}
	hostRate, err := redis.Int64(c.Do("INCR", host))
	if err != nil {
		return false, err
	}
	if hostRate == 1 {
		// 60 seconds rate
		_, err = c.Do("EXPIRE", host, 60)
		if err != nil {
			return false, err
		}
		return true, nil
	}
	if hostRate < rate {
		return true, nil
	}
	return false, nil
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

	if cfg.Rate > 0 {
		allowed, err := allowedRate(r, c, cfg.Rate)
		if err != nil {
			return http.StatusInternalServerError, err
		}
		if !allowed {
			return http.StatusTooManyRequests, nil
		}
	}

	num, err := redis.Int64(c.Do("INCR", countKey))
	if err != nil {
		return http.StatusInternalServerError, err
	}

	short := trim.Encode(num)
	response := &Response{URL: u.String(), Short: cfg.ShortURL(short)}

	_, err = c.Do("SET", short, response.URL)
	if err != nil {
		return http.StatusServiceUnavailable, err
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
	path, err := GetContext(ctx)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	c := cfg.GetConn()
	defer c.Close()

	originURL, err := redis.String(c.Do("GET", path))
	if err != nil {
		return http.StatusNotFound, err
	}
	status := http.StatusFound
	http.Redirect(w, r, originURL, status)
	return status, nil
}

//func HandleHTML(ctx context.Context, w http.ResponseWriter, r *http.Request) (int, error) {
//	cfg, err := conf.GetContext(ctx)
//	if err != nil {
//		return http.StatusInternalServerError, err
//	}
//	c := cfg.GetConn()
//	defer c.Close()
//
//	return http.StatusOK, nil
//}
