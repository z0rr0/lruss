// Copyright 2017 Alexander Zaytsev <thebestzorro@yandex.ru>.
// All rights reserved. Use of this source code is governed
// by a BSD-style license that can be found in the LICENSE file.

//Package trim implements methods and structures to convert users' URLs.
package conf

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Cfg is rates' configuration settings.
type Cfg struct {
	Host               string `json:"host"`
	Port               uint   `json:"port"`
	Timeout            int64  `json:"timeout"`
	TerminationTimeout int64  `json:"termination"`
	Debug              bool   `json:"debug"`
	timeout            time.Duration
	terminationTimeout time.Duration
	logger             *log.Logger
}

// isValid checks the settings are valid.
func (c *Cfg) isValid() error {
	// required 2 due to external timeout
	if c.Timeout < 1 {
		return errors.New("invalid timeout value")
	}
	c.timeout = time.Duration(c.Timeout) * time.Second
	c.terminationTimeout = time.Duration(c.TerminationTimeout) * time.Second
	return nil
}

// Addr returns service's net address.
func (c *Cfg) Addr() string {
	return net.JoinHostPort(c.Host, fmt.Sprint(c.Port))
}

// HandleTimeout is service timeout.
func (c *Cfg) HandleTimeout() time.Duration {
	return c.timeout
}

// ShutdownTimeout is graceful service shutdown timeout.
func (c *Cfg) ShutdownTimeout() time.Duration {
	return c.terminationTimeout
}

// New returns new rates configuration.
func New(filename string, logger *log.Logger) (*Cfg, error) {
	fullPath, err := filepath.Abs(strings.Trim(filename, " "))
	if err != nil {
		return nil, err
	}
	_, err = os.Stat(fullPath)
	if err != nil {
		return nil, err
	}
	jsonData, err := ioutil.ReadFile(fullPath)
	if err != nil {
		return nil, err
	}
	c := &Cfg{logger: logger}
	err = json.Unmarshal(jsonData, c)
	if err != nil {
		return nil, err
	}
	err = c.isValid()
	if err != nil {
		return nil, err
	}
	if c.Debug {
		c.logger.SetOutput(os.Stdout)
	}
	return c, err
}
