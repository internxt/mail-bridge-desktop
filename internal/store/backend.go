package store

import "errors"

var ErrNotFound = errors.New("store: key not found")

type backend interface {
	Get(key string) ([]byte, error)
	Set(key string, value []byte) error
	Remove(key string) error
}
