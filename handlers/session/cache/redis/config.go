package redis

import (
	"github.com/xuyu/goredis"
	"time"
)

// Option is the type of the configuration parameters accepted by redisNew to create a new Inode connection.
type Option func(*goredis.DialConfig)

// redisNew attempts to create a new redis connection object
func redisNew(options ...Option) (*goredis.Redis, error) {
	c := new(goredis.DialConfig)

	// Defaults
	c.Address = ":6379"
	c.Database = 0
	c.MaxIdle = 1
	c.Network = "tcp"
	c.Password = ""
	c.Timeout = 15 * time.Second

	for _, opt := range options {
		opt(c)
	}

	newredis, err := goredis.Dial(c)
	if err != nil {
		return nil, err
	}
	return newredis, nil
}

// Redis Configuration functions

// Configurator is an empty struct type whose methods are all constructors of configuration Options for *goredis.DialConfig.
type Configurator struct{}

func (o Configurator) SetAddress(s string) func(*goredis.DialConfig) {
	return func(c *goredis.DialConfig) {
		c.Address = s
	}
}

func (o Configurator) SetDatabase(i int) func(*goredis.DialConfig) {
	return func(c *goredis.DialConfig) {
		c.Database = i
	}
}

func (o Configurator) SetMaxIdle(i int) func(*goredis.DialConfig) {
	return func(c *goredis.DialConfig) {
		c.MaxIdle = i
	}
}

func (o Configurator) SetNetwork(s string) func(*goredis.DialConfig) {
	return func(c *goredis.DialConfig) {
		c.Network = s
	}
}

func (o Configurator) SetPassword(s string) func(*goredis.DialConfig) {
	return func(c *goredis.DialConfig) {
		c.Password = s
	}
}

func (o Configurator) SetTimeout(i int) func(*goredis.DialConfig) {
	return func(c *goredis.DialConfig) {
		c.Timeout = time.Duration(i) * time.Second
	}
}
