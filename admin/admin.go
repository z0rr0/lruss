// Copyright 2017 Alexander Zaytsev <thebestzorro@yandex.ru>.
// All rights reserved. Use of this source code is governed
// by a BSD-style license that can be found in the LICENSE file.

// Package admin contains HTTP administration methods.
// Also it includes authentication functions.
package admin

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"

	"github.com/garyburd/redigo/redis"
	"github.com/z0rr0/lruss/conf"
)

const (
	// userKey is internal user context key.
	userKey key = "user"
	// csrfLen is CSRF token bytes length.
	csrfLen = 128
)

// key is internal context key.
type key string

// SetContext writes settings to context.
func SetContext(ctx context.Context, username string) context.Context {
	return context.WithValue(ctx, userKey, username)
}

// GetContext reads settings from context.
func GetContext(ctx context.Context) (string, error) {
	c, ok := ctx.Value(userKey).(string)
	if !ok {
		return "", errors.New("user configuration not found")
	}
	return c, nil
}

// GetCSRF returns a CSRF (Сross Site Request Forgery) token.
func GetCSRF(ctx context.Context) (string, error) {
	user, err := GetContext(ctx)
	if err != nil {
		return "", err
	}
	cfg, err := conf.GetContext(ctx)
	if err != nil {
		return "", err
	}
	c := cfg.GetConn()
	defer c.Close()

	b := make([]byte, csrfLen)
	_, err = rand.Read(b)
	if err != nil {
		return "", err
	}
	csrfKey, err := conf.DbKey("csrf", user)
	if err != nil {
		return "", err
	}
	value := hex.EncodeToString(b)
	token, err := redis.String(c.Do("GETSET", csrfKey, value))
	switch {
	case err == redis.ErrNil:
		// set expire interval for new token
		_, err = c.Do("EXPIRE", csrfKey, cfg.CSRFTimeout)
		if err != nil {
			return "", err
		}
		token = value
	case err != nil:
		return "", err
	}
	return token, nil
}

// CheckCSRF verifies a CSRF token.
func CheckCSRF(ctx context.Context, token string) (bool, error) {
	user, err := GetContext(ctx)
	if err != nil {
		return false, err
	}
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

// Login shows auth form and checks users' credentials.
func Login(ctx context.Context, w http.ResponseWriter, r *http.Request) (int, error) {
	cfg, err := conf.GetContext(ctx)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	c := cfg.GetConn()
	defer c.Close()

	return http.StatusOK, nil
}
