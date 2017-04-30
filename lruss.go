// Copyright 2017 Alexander Zaytsev <thebestzorro@yandex.ru>.
// All rights reserved. Use of this source code is governed
// by a BSD-style license that can be found in the LICENSE file.

// Package main implements main methods of LRUSS service.
package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"context"
	"github.com/z0rr0/lruss/conf"
	"net/http"
	"strings"
	"time"
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
	server := &http.Server{
		Addr:           cfg.Addr(),
		Handler:        http.DefaultServeMux,
		ReadTimeout:    cfg.HandleTimeout(),
		WriteTimeout:   cfg.HandleTimeout(),
		MaxHeaderBytes: 1 << 20, // 1MB
		ErrorLog:       loggerError,
	}
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		start, code := time.Now(), http.StatusOK
		defer func() {
			logger.Printf("%-5v %v\t%-12v\t%v",
				r.Method,
				code,
				time.Since(start),
				r.URL.String(),
			)
		}()
		w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	})
	errc := make(chan error)
	go interrupt(errc)
	go func() {
		errc <- server.ListenAndServe()
	}()
	logger.Printf("running: version=%v [%v %v debug=%v]\nListen: %v\n\n",
		Version, GoVersion, Revision, *debug || cfg.Debug, server.Addr)
	err = <-errc
	logger.Printf("termination: %v [%v] reason: %+v\n", Version, Revision, err)

	ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout())
	defer cancel()

	if msg := err.Error(); strings.HasPrefix(msg, interruptPrefix) {
		logger.Println("graceful shutdown")
		if err := server.Shutdown(ctx); err != nil {
			loggerError.Printf("graceful shutdown error: %v\n", err)
		}

	}

}
