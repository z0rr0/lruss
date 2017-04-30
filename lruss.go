// Copyright 2017 Alexander Zaytsev <thebestzorro@yandex.ru>.
// All rights reserved. Use of this source code is governed
// by a BSD-style license that can be found in the LICENSE file.

// Package main implements main methods of LRUSS service.
package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/z0rr0/lruss/conf"
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
	debug := flag.Bool("debug", false, "debug mode")
	version := flag.Bool("version", false, "show version")
	config := flag.String("config", Config, "configuration file")
	flag.Parse()

	if *version {
		fmt.Printf("\tVersion: %v\n\tRevision: %v\n\tBuild date: %v\n\tGo version: %v\n",
			Version, Revision, BuildDate, GoVersion)
		return
	}
	logger := log.New(ioutil.Discard, fmt.Sprintf("DEBUG [%v]: ", Name),
		log.Ldate|log.Lmicroseconds|log.Lshortfile)
	if *debug {
		logger.SetOutput(os.Stdout)
	}

	cfg, err := conf.New(*config, logger)
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
		"/api": web.HandleAPI,
	}
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		var err error
		start, code := time.Now(), http.StatusOK
		defer func() {
			switch {
			case code == http.StatusBadRequest:
				http.Error(w, err.Error(), code)
			case code != http.StatusOK:
				http.Error(w, http.StatusText(code), code)
			}
			loggerInfo.Printf("%-5v %v\t%-12v\t%v",
				r.Method,
				code,
				time.Since(start),
				r.URL.String(),
			)
		}()
		handler, ok := handlers[strings.TrimRight(r.URL.Path, "/ ")]
		if !ok {
			code = http.StatusNotFound
			http.NotFound(w, r)
			return
		}
		ctx, cancel := context.WithTimeout(mainCtx, cfg.HandleTimeout())
		defer cancel()

		code, err = handler(ctx, w, r)
		if err != nil {
			loggerError.Printf("handler error: %v", err)
		}
	})
	errCh := make(chan error)
	go interrupt(errCh)
	go func() {
		errCh <- server.ListenAndServe()
	}()
	loggerInfo.Printf("running: version=%v [%v %v debug=%v]\nListen: %v\n\n",
		Version, GoVersion, Revision, *debug || cfg.Debug, server.Addr)
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
