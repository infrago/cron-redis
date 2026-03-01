# cron-redis

`cron-redis` 是 `cron` 模块的 `redis` 驱动。

## 安装

```bash
go get github.com/infrago/cron@latest
go get github.com/infrago/cron-redis@latest
```

## 接入

```go
import (
    _ "github.com/infrago/cron"
    _ "github.com/infrago/cron-redis"
    "github.com/infrago/infra"
)

func main() {
    infra.Run()
}
```

## 配置示例

```toml
[cron]
driver = "redis"
```

## 公开 API（摘自源码）

- `func (d *redisDriver) Connection(inst *cron.Instance) (cron.Connection, error)`
- `func (c *redisConnection) Open() error`
- `func (c *redisConnection) Close() error`
- `func (c *redisConnection) Add(name string, job cron.Job) error`
- `func (c *redisConnection) Enable(name string) error`
- `func (c *redisConnection) Disable(name string) error`
- `func (c *redisConnection) Remove(name string) error`
- `func (c *redisConnection) List() (map[string]cron.Job, error)`
- `func (c *redisConnection) AppendLog(log cron.Log) error`
- `func (c *redisConnection) History(jobName string, offset, limit int) (int64, []cron.Log, error)`
- `func (c *redisConnection) Lock(key string, ttl time.Duration) (bool, error)`

## 排错

- driver 未生效：确认模块段 `driver` 值与驱动名一致
- 连接失败：检查 endpoint/host/port/鉴权配置
