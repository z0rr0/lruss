// Copyright 2017 Alexander Zaytsev <thebestzorro@yandex.ru>.
// All rights reserved. Use of this source code is governed
// by a BSD-style license that can be found in the LICENSE file.

package admin

import (
	"context"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/z0rr0/lruss/conf"
)

const (
	programRepo    = "github.com/z0rr0/lruss"
	testConfigName = "config.example.json"
)

type fataler interface {
	Fatalf(format string, args ...interface{})
	Fatal(args ...interface{})
}

func getConfig() string {
	dirs := []string{os.Getenv("GOPATH"), "src"}
	dirs = append(dirs, strings.Split(programRepo, "/")...)
	dirs = append(dirs, testConfigName)
	return path.Join(dirs...)
}

func initConfig(f fataler) *conf.Cfg {
	cfgFile := getConfig()
	cfg, err := conf.New(cfgFile)
	if err != nil {
		f.Fatal(err)
	}
	cfg.Redis.Db = 0
	err = cfg.SetRedisPool()
	if err != nil {
		f.Fatalf("set redis pool error: %v", err)
	}
	c := cfg.GetConn()
	defer c.Close()

	_, err = c.Do("FLUSHDB")
	if err != nil {
		f.Fatalf("flushdb error: %v", err)
	}
	return cfg
}

func cleanDb(ctx context.Context) error {
	cfg, err := conf.GetContext(ctx)
	if err != nil {
		return err
	}
	c := cfg.GetConn()
	defer c.Close()
	_, err = c.Do("FLUSHDB")
	return err
}

func TestCheckCSRF(t *testing.T) {
	cfg := initConfig(t)
	ctx := conf.SetContext(context.Background(), cfg)
	ctx = SetContext(ctx, "test")
	defer cleanDb(ctx)

	token, err := GetCSRF(ctx)
	if err != nil {
		t.Errorf("failed token, error: %v", err)
	}
	t.Logf("csrf token=%v\n", token)
	if l := len(token); l != csrfLen*2 {
		t.Errorf("invalid token length %d", l)
	}

	isValid, err := CheckCSRF(ctx, "bad token")
	if err != nil {
		t.Errorf("failed token check: %v", err)
	}
	if isValid {
		t.Error("bad token can't be valid")
	}
	isValid, err = CheckCSRF(ctx, token)
	if err != nil {
		t.Errorf("failed token check: %v", err)
	}
	if !isValid {
		t.Error("failed response for valid token")
	}
}

func BenchmarkGetCSRF(b *testing.B) {
	cfg := initConfig(b)
	ctx := conf.SetContext(context.Background(), cfg)
	ctx = SetContext(ctx, "test")
	defer cleanDb(ctx)

	for i := 0; i < b.N; i++ {
		token, err := GetCSRF(ctx)
		if err != nil {
			b.Errorf("failed token, error: %v", err)
		}
		if l := len(token); l != csrfLen*2 {
			b.Errorf("invalid token length %d", l)
		}
	}
}
