// Copyright 2017 Alexander Zaytsev <thebestzorro@yandex.ru>.
// All rights reserved. Use of this source code is governed
// by a BSD-style license that can be found in the LICENSE file.

//Package web contains main web and API handlers.
package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"

	"github.com/garyburd/redigo/redis"
	"github.com/z0rr0/lruss/conf"
	"github.com/z0rr0/lruss/trim"
)

const (
	// countKey is common db counter.
	countKey = "count"
)

// Response is API response for URL shorting.
type Response struct {
	URL   string `json:"url"`
	Short string `json:"short"`
}

// HandleAPI handles API request to get shor url.
func HandleAPI(ctx context.Context, w http.ResponseWriter, r *http.Request) (int, error) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
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
	encoder := json.NewEncoder(w)
	if err := encoder.Encode(response); err != nil {
		return http.StatusInternalServerError, err
	}
	return http.StatusOK, nil
}
