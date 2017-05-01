// Copyright 2017 Alexander Zaytsev <thebestzorro@yandex.ru>.
// All rights reserved. Use of this source code is governed
// by a BSD-style license that can be found in the LICENSE file.

package conf

import (
	"os"
	"path"
	"strings"
	"testing"
)

const (
	programRepo    = "github.com/z0rr0/lruss"
	testConfigName = "config.example.json"
)

func getConfig() string {
	dirs := []string{os.Getenv("GOPATH"), "src"}
	dirs = append(dirs, strings.Split(programRepo, "/")...)
	dirs = append(dirs, testConfigName)
	return path.Join(dirs...)
}

func TestNew(t *testing.T) {
	if _, err := New("/bad_file_path.json"); err == nil {
		t.Error("unexpected behavior")
	}
	cfgFile := getConfig()
	cfg, err := New(cfgFile)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr() == "" {
		t.Error("empty address")
	}
}
