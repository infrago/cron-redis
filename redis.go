package cron_redis

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/bamgoo/bamgoo"
	"github.com/bamgoo/cron"
	"github.com/redis/go-redis/v9"
)

func init() {
	bamgoo.Register("redis", &redisDriver{})
}

type (
	redisDriver struct{}

	redisConnection struct {
		client     *redis.Client
		ctx        context.Context
		jobKey     string
		logPrefix  string
		lockPrefix string
		logLimit   int64
	}
)

func (d *redisDriver) Connection(inst *cron.Instance) (cron.Connection, error) {
	setting := inst.Config.Setting

	addr := "127.0.0.1:6379"
	host := ""
	port := "6379"
	if v, ok := setting["port"].(string); ok && v != "" {
		port = v
	}
	if v, ok := setting["server"].(string); ok && v != "" {
		host = v
	}
	if v, ok := setting["host"].(string); ok && v != "" {
		host = v
	}
	if host != "" {
		addr = host + ":" + port
	}
	if v, ok := setting["addr"].(string); ok && v != "" {
		addr = v
	}

	username, _ := setting["username"].(string)
	password, _ := setting["password"].(string)

	database := 0
	switch v := setting["database"].(type) {
	case int:
		database = v
	case int64:
		database = int(v)
	case float64:
		database = int(v)
	case string:
		if vv, err := strconv.Atoi(v); err == nil {
			database = vv
		}
	}

	jobKey := "bamgoo:cron:jobs"
	if v, ok := setting["jobs_key"].(string); ok && v != "" {
		jobKey = v
	}

	logPrefix := "bamgoo:cron:logs:"
	if v, ok := setting["logs_prefix"].(string); ok && v != "" {
		logPrefix = v
	}

	lockPrefix := "bamgoo:cron:locks:"
	if v, ok := setting["locks_prefix"].(string); ok && v != "" {
		lockPrefix = v
	}

	var logLimit int64
	switch v := setting["log_limit"].(type) {
	case int:
		logLimit = int64(v)
	case int64:
		logLimit = v
	case float64:
		logLimit = int64(v)
	case string:
		if vv, err := strconv.ParseInt(v, 10, 64); err == nil {
			logLimit = vv
		}
	}

	conn := &redisConnection{
		client: redis.NewClient(&redis.Options{
			Addr:     addr,
			Username: username,
			Password: password,
			DB:       database,
		}),
		ctx:        context.Background(),
		jobKey:     jobKey,
		logPrefix:  logPrefix,
		lockPrefix: lockPrefix,
		logLimit:   logLimit,
	}

	return conn, nil
}

func (c *redisConnection) Open() error {
	return c.client.Ping(c.ctx).Err()
}

func (c *redisConnection) Close() error {
	return c.client.Close()
}

func (c *redisConnection) Add(name string, job cron.Job) error {
	job.Name = name
	data, err := json.Marshal(job)
	if err != nil {
		return err
	}
	return c.client.HSet(c.ctx, c.jobKey, name, data).Err()
}

func (c *redisConnection) Enable(name string) error {
	return c.setDisabled(name, false)
}

func (c *redisConnection) Disable(name string) error {
	return c.setDisabled(name, true)
}

func (c *redisConnection) Remove(name string) error {
	pipe := c.client.Pipeline()
	pipe.HDel(c.ctx, c.jobKey, name)
	pipe.Del(c.ctx, c.logKey(name))
	_, err := pipe.Exec(c.ctx)
	return err
}

func (c *redisConnection) List() (map[string]cron.Job, error) {
	items, err := c.client.HGetAll(c.ctx, c.jobKey).Result()
	if err != nil {
		return nil, err
	}

	out := make(map[string]cron.Job, len(items))
	for name, raw := range items {
		var job cron.Job
		if err := json.Unmarshal([]byte(raw), &job); err != nil {
			continue
		}
		job.Name = name
		out[name] = job
	}
	return out, nil
}

func (c *redisConnection) AppendLog(log cron.Log) error {
	data, err := json.Marshal(log)
	if err != nil {
		return err
	}

	key := c.logKey(log.Job)
	pipe := c.client.Pipeline()
	pipe.LPush(c.ctx, key, data)
	if c.logLimit > 0 {
		pipe.LTrim(c.ctx, key, 0, c.logLimit-1)
	}
	_, err = pipe.Exec(c.ctx)
	return err
}

func (c *redisConnection) History(jobName string, offset, limit int) (int64, []cron.Log, error) {
	key := c.logKey(jobName)

	total, err := c.client.LLen(c.ctx, key).Result()
	if err != nil {
		return 0, nil, err
	}
	if total == 0 {
		return 0, []cron.Log{}, nil
	}

	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = int(total)
	}

	start := int64(offset)
	stop := start + int64(limit) - 1
	values, err := c.client.LRange(c.ctx, key, start, stop).Result()
	if err != nil {
		return 0, nil, err
	}

	logs := make([]cron.Log, 0, len(values))
	for _, raw := range values {
		var log cron.Log
		if err := json.Unmarshal([]byte(raw), &log); err != nil {
			continue
		}
		logs = append(logs, log)
	}

	return total, logs, nil
}

func (c *redisConnection) Lock(key string, ttl time.Duration) (bool, error) {
	if ttl <= 0 {
		ttl = time.Second
	}
	return c.client.SetNX(c.ctx, c.lockKey(key), time.Now().UnixNano(), ttl).Result()
}

func (c *redisConnection) logKey(jobName string) string {
	return c.logPrefix + jobName
}

func (c *redisConnection) lockKey(key string) string {
	return c.lockPrefix + key
}

func (c *redisConnection) setDisabled(name string, disabled bool) error {
	raw, err := c.client.HGet(c.ctx, c.jobKey, name).Result()
	if err == redis.Nil {
		return nil
	}
	if err != nil {
		return err
	}

	var job cron.Job
	if err := json.Unmarshal([]byte(raw), &job); err != nil {
		return err
	}
	job.Name = name
	job.Disabled = disabled

	data, err := json.Marshal(job)
	if err != nil {
		return err
	}
	return c.client.HSet(c.ctx, c.jobKey, name, data).Err()
}
