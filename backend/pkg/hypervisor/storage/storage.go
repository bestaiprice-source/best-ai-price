package storage

import (
	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/redis/go-redis/v9"
)

type Storage struct {
	db    *ent.Client
	redis *redis.Client
}

func NewStorage(db *ent.Client, rdb *redis.Client) *Storage {
	return &Storage{
		db:    db,
		redis: rdb,
	}
}

func (s *Storage) DB() *ent.Client {
	return s.db
}

func (s *Storage) Redis() *redis.Client {
	return s.redis
}
