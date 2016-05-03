package redis

import (
	"errors"
	"github.com/xuyu/goredis"
)

var (
	ERRNOSTR       = errors.New("Unable to retrieve object of type string from Redis storage.")
	ERRCANTPERSIST = errors.New("Could not persist key. Key was perhaps not found.")
	ERRNOEXPIRY    = errors.New("Expiry time was not set. Key was perhaps not found.")
	ERRNODELETE    = errors.New("Could not delete")
	ERRNOKEY       = errors.New("Key not found or not expirable")
)

// TODO review get so that we effectively get a string (review HGET)

// Cache defines the redis store object.
// The underlying object should be safe for concurrent use.
type Cache struct {
	*goredis.Redis
}

func New(options ...Option) (*Cache, error) {
	n := new(Cache)
	red, err := redisNew(options...)
	if err != nil {
		return nil, err
	}
	n.Redis = red
	return n, nil
}

// Options is an exported object whose methods are all constructor of
// configuration Option objects for a redis connection.
// SetAddress(string), SetDatabase(int), SetMaxIdle(int),
// SetNetwork(string), SetPassword(string), SetTimeout(int).
var Options Configurator

// Public API

func (c *Cache) Get(id, hkey string) (res []byte, err error) {
	return c.Redis.HGet(id, hkey)
}

func (c *Cache) Put(id string, hkey string, content []byte) error {
	return c.Redis.HSet(id, hkey, string(content))
	c.Redis.HSet
}

func (c *Cache) Delete(id, hkey string) error {
	return c.Redis.HDel(id, hkey)
}

// GetExpiry retrieves the expiration date for a given key, in seconds.
func (c *Cache) GetExpiry(id string) (t int64, err error) {
	r := c.Redis

	res, err := r.ExecuteCommand("TTL", id)
	if err != nil {
		return 0, err
	}
	num, err := res.IntegerValue()
	if err != nil {
		return 0, err
	}
	if num < 0 {
		if num == -2 {
			return num, ERRNOKEY
		}
		return num, ERRNOEXPIRY
	}
	return num, nil
}

// SetExpiry sets the time in seconds before a key should expire.
func (c *Cache) SetExpiry(id string, t int64) error {
	r := c.Redis

	// if t < 0  we persist the Key
	if t < 0 {
		res, err := r.ExecuteCommand("PERSIST", id)
		if err != nil {
			return err
		}
		num, err := res.IntegerValue()
		if err != nil {
			return err
		}
		if num != 1 {
			return ERRCANTPERSIST
		}
		return nil
	}

	// if t = 0, we will delete the key
	if t == 0 {
		res, err := r.ExecuteCommand("DEL", id)
		if err != nil {
			return err
		}
		num, err := res.IntegerValue()
		if err != nil {
			return err
		}
		if num != 1 {
			return ERRNODELETE
		}
		return nil
	}

	// if t > 0 we will expire the key after t seconds.
	res, err := r.ExecuteCommand("EXPIRE", id, t)
	if err != nil {
		return err
	}
	num, err := res.IntegerValue()
	if err != nil {
		return err
	}
	if num != 1 {
		return ERRNOEXPIRY
	}
	return nil
}
