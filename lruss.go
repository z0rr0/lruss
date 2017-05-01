// Copyright 2017 Alexander Zaytsev <thebestzorro@yandex.ru>.
// All rights reserved. Use of this source code is governed
// by a BSD-style license that can be found in the LICENSE file.

// Package main implements main methods of LRUSS service.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/z0rr0/lruss/conf"
	"github.com/z0rr0/lruss/trim"
	"github.com/z0rr0/lruss/web"
)

const (
	// Name is a program name
	Name = "LRUSS"
	// Config is default configuration file name
	Config = "config.json"
	// interruptPrefix is constant prefix of interrupt signal
	interruptPrefix = "interrupt signal"
)

var (
	// Version is LUSS version
	Version = ""
	// Revision is revision number
	Revision = ""
	// BuildDate is build date
	BuildDate = ""
	// GoVersion is runtime Go language version
	GoVersion = runtime.Version()

	// internal loggers
	loggerError = log.New(os.Stderr, fmt.Sprintf("ERROR [%v]: ", Name),
		log.Ldate|log.Ltime|log.Lshortfile)
	loggerInfo = log.New(os.Stdout, fmt.Sprintf("INFO [%v]: ", Name),
		log.Ldate|log.Ltime|log.Lshortfile)
)

// interrupt catches custom signals.
func interrupt(errc chan error) {
	c := make(chan os.Signal)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	errc <- fmt.Errorf("%v %v", interruptPrefix, <-c)
}

func main() {
	defer func() {
		if r := recover(); r != nil {
			loggerError.Printf("abnormal termination [%v]: \n\t%v\n", Version, r)
		}
	}()
	version := flag.Bool("version", false, "show version")
	config := flag.String("config", Config, "configuration file")
	flag.Parse()

	if *version {
		fmt.Printf("\tVersion: %v\n\tRevision: %v\n\tBuild date: %v\n\tGo version: %v\n",
			Version, Revision, BuildDate, GoVersion)
		return
	}

	cfg, err := conf.New(*config)
	if err != nil {
		loggerError.Fatalf("configuration error: %v", err)
	}
	err = cfg.SetRedisPool()
	if err != nil {
		loggerError.Fatalf("set redis pool error: %v", err)
	}
	server := &http.Server{
		Addr:           cfg.Addr(),
		Handler:        http.DefaultServeMux,
		ReadTimeout:    cfg.HandleTimeout(),
		WriteTimeout:   cfg.HandleTimeout(),
		MaxHeaderBytes: 1 << 20, // 1MB
		ErrorLog:       loggerError,
	}
	mainCtx := conf.SetContext(context.Background(), cfg)
	handlers := map[string]func(ctx context.Context, w http.ResponseWriter, r *http.Request) (int, error){
		//"/":    HtmlHandler,
		"api/add": web.HandleAPI,
	}
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		var err error
		start, code := time.Now(), http.StatusOK
		defer func() {
			if err != nil {
				http.Error(w, err.Error(), code)
			}
			loggerInfo.Printf("%-5v %v\t%-12v\t%v",
				r.Method,
				code,
				time.Since(start),
				r.URL.String(),
			)
		}()
		path := strings.Trim(r.URL.Path, "/ ")
		handler, ok := handlers[path]
		if ok {
			code, err = handler(mainCtx, w, r)
			if err != nil {
				loggerError.Printf("handler error: %v", err)
			}
			if code != http.StatusBadRequest {
				err = errors.New(http.StatusText(code))
			}
			return
		}
		if trim.IsShort(path) {
			ctx := web.SetContext(mainCtx, path)
			// try to found short url
			code, err = web.HandleRedirect(ctx, w, r)
			if err != nil {
				loggerInfo.Printf("redirect handler error: %v", err)
			}
			if code == http.StatusNotFound {
				err = errors.New(http.StatusText(code))
			}
			return
		}
		http.NotFound(w, r)
		code = http.StatusNotFound
		return
	})
	errCh := make(chan error)
	go interrupt(errCh)
	go func() {
		errCh <- server.ListenAndServe()
	}()
	loggerInfo.Printf("running: version=%v [%v %v debug=%v]\nListen: %v\n\n",
		Version, GoVersion, Revision, cfg.Debug, server.Addr)
	err = <-errCh
	loggerInfo.Printf("termination: %v [%v] reason: %+v\n", Version, Revision, err)

	ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout())
	defer cancel()

	if msg := err.Error(); strings.HasPrefix(msg, interruptPrefix) {
		loggerInfo.Println("graceful shutdown")
		if err := server.Shutdown(ctx); err != nil {
			loggerError.Printf("graceful shutdown error: %v\n", err)
		}
		if err := cfg.CloseRedisPool(); err != nil {
			loggerError.Printf("close pool error: %v\n", err)
		} else {
			loggerInfo.Println("closed connections pool")
		}

	}

}
