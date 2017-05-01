// Copyright 2017 Alexander Zaytsev <thebestzorro@yandex.ru>.
// All rights reserved. Use of this source code is governed
// by a BSD-style license that can be found in the LICENSE file.

//Package conf implements methods setup configuration settings.
package conf

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"context"
	"github.com/garyburd/redigo/redis"
)

const (
	// cfgKey is configuration context key.
	cfgKey key = "cfg"
)

type key string

// rediscfg is configuration redis settings.
type rediscfg struct {
	Host     string `json:"host"`
	Port     uint   `json:"port"`
	Network  string `json:"network"`
	Db       int    `json:"db"`
	Timeout  int64  `json:"timeout"`
	Password string `json:"password"`
	IndleCon int    `json:"indlecon"`
	MaxCon   int    `json:"maxcon"`
	timeout  time.Duration
}

// Cfg is rates' configuration settings.
type Cfg struct {
	Host               string   `json:"host"`
	Port               uint     `json:"port"`
	Site               string   `json:"site"`
	Timeout            int64    `json:"timeout"`
	TerminationTimeout int64    `json:"termination"`
	Redis              rediscfg `json:"redis"`
	timeout            time.Duration
	terminationTimeout time.Duration
	pool               *redis.Pool
}

// isValid checks the settings are valid.
func (c *Cfg) isValid() error {
	// required 2 due to external timeout
	if c.Timeout < 1 {
		return errors.New("invalid timeout value")
	}
	c.timeout = time.Duration(c.Timeout) * time.Second
	if c.TerminationTimeout < 1 {
		return errors.New("invalid termination timeout value")
	}
	c.terminationTimeout = time.Duration(c.TerminationTimeout) * time.Second
	if c.Redis.Timeout < 1 {
		return errors.New("invalid redis 	timeout value")
	}
	c.Redis.timeout = time.Duration(c.Redis.Timeout) * time.Second
	if (c.Redis.IndleCon < 1) || (c.Redis.MaxCon < 1) {
		return errors.New("invalic redis connections settings")
	}
	if c.Redis.Db < 0 {
		return errors.New("invalid db number")
	}
	if c.Site == "" {
		return errors.New("empty site value")
	}
	c.Site = strings.TrimRight(c.Site, "/ ")
	return nil
}

// SetRedisPool sets redis connections pool and checks it.
func (c *Cfg) SetRedisPool() error {
	pool := &redis.Pool{
		MaxIdle:     c.Redis.IndleCon,
		MaxActive:   c.Redis.MaxCon,
		IdleTimeout: c.Redis.timeout,
		Wait:        true,
		Dial: func() (redis.Conn, error) {
			return redis.Dial(
				c.Redis.Network,
				c.RedisAddr(),
				redis.DialConnectTimeout(c.Redis.timeout),
				redis.DialDatabase(c.Redis.Db),
				redis.DialPassword(c.Redis.Password),
			)
		},
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			if time.Since(t) < time.Minute {
				return nil
			}
			_, err := c.Do("PING")
			return err
		},
	}
	conn := pool.Get()
	_, err := conn.Do("PING")
	if err != nil {
		return err
	}
	c.pool = pool
	return conn.Close()
}

// CloseRedisPool releases redis pool.
func (c *Cfg) CloseRedisPool() error {
	return c.pool.Close()
}

// Addr returns service's net address.
func (c *Cfg) Addr() string {
	return net.JoinHostPort(c.Host, fmt.Sprint(c.Port))
}

// RedisAddr returns redis service's net address.
func (c *Cfg) RedisAddr() string {
	return net.JoinHostPort(c.Redis.Host, fmt.Sprint(c.Redis.Port))
}

// HandleTimeout is service timeout.
func (c *Cfg) HandleTimeout() time.Duration {
	return c.timeout
}

// ShutdownTimeout is graceful service shutdown timeout.
func (c *Cfg) ShutdownTimeout() time.Duration {
	return c.terminationTimeout
}

// GetConn returns redis db connection.
func (c *Cfg) GetConn() redis.Conn {
	return c.pool.Get()
}

// ShortURL returns short URL for configured site.
func (c *Cfg) ShortURL(short string) string {
	return fmt.Sprintf("%v/%v", c.Site, short)
}

// New returns new rates configuration.
func New(filename string) (*Cfg, error) {
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
	c := &Cfg{}
	err = json.Unmarshal(jsonData, c)
	if err != nil {
		return nil, err
	}
	err = c.isValid()
	if err != nil {
		return nil, err
	}
	return c, err
}

// SetContext writes settings to context.
func SetContext(ctx context.Context, c *Cfg) context.Context {
	return context.WithValue(ctx, cfgKey, c)
}

// GetContext reads settings from context.
func GetContext(ctx context.Context) (*Cfg, error) {
	c, ok := ctx.Value(cfgKey).(*Cfg)
	if !ok {
		return nil, errors.New("configuration not found")
	}
	return c, nil
}
