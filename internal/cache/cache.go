// Package cache 提供 RAG 链路的进程内缓存:泛型 LRU(容量 + TTL)与 KB 版本号。
//
// 单实例部署下进程内缓存足够,不引入 Redis 等外部依赖;多副本部署时再替换实现。
// 失效策略:检索结果缓存的 key 携带 KB 版本号,上传/删除文档时版本自增,
// 旧版本条目不再被查到、随 LRU 自然淘汰,无需主动清理。
package cache

import (
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"
)

// LRU 是线程安全的定容 LRU 缓存。ttl 为 0 表示条目不过期。
type LRU[V any] struct {
	mu    sync.Mutex
	cap   int
	ttl   time.Duration
	ll    *list.List // 队首为最近使用
	items map[string]*list.Element
}

type lruEntry[V any] struct {
	key     string
	val     V
	expires time.Time // 零值表示不过期
}

// NewLRU 构造容量为 capacity、条目存活 ttl 的 LRU。capacity<=0 时取 128。
func NewLRU[V any](capacity int, ttl time.Duration) *LRU[V] {
	if capacity <= 0 {
		capacity = 128
	}
	return &LRU[V]{
		cap:   capacity,
		ttl:   ttl,
		ll:    list.New(),
		items: make(map[string]*list.Element, capacity),
	}
}

// Get 返回 key 对应的值;不存在或已过期时 ok 为 false。
func (c *LRU[V]) Get(key string) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var zero V
	el, ok := c.items[key]
	if !ok {
		return zero, false
	}
	ent := el.Value.(*lruEntry[V])
	if !ent.expires.IsZero() && time.Now().After(ent.expires) {
		c.ll.Remove(el)
		delete(c.items, key)
		return zero, false
	}
	c.ll.MoveToFront(el)
	return ent.val, true
}

// Set 写入或更新 key;超容时淘汰最久未使用的条目。
func (c *LRU[V]) Set(key string, val V) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var expires time.Time
	if c.ttl > 0 {
		expires = time.Now().Add(c.ttl)
	}
	if el, ok := c.items[key]; ok {
		ent := el.Value.(*lruEntry[V])
		ent.val = val
		ent.expires = expires
		c.ll.MoveToFront(el)
		return
	}
	el := c.ll.PushFront(&lruEntry[V]{key: key, val: val, expires: expires})
	c.items[key] = el
	if c.ll.Len() > c.cap {
		oldest := c.ll.Back()
		c.ll.Remove(oldest)
		delete(c.items, oldest.Value.(*lruEntry[V]).key)
	}
}

// Versions 维护每个 KB 的版本号(进程内)。文档变更时 Bump,
// 检索缓存 key 携带版本号实现自然失效。重启后归零,与进程内缓存的生命周期一致。
type Versions struct {
	mu sync.Mutex
	m  map[string]uint64
}

func NewVersions() *Versions { return &Versions{m: map[string]uint64{}} }

// Get 返回 KB 当前版本号(未 Bump 过为 0)。
func (v *Versions) Get(id string) uint64 {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.m[id]
}

// Bump 在 KB 文档发生变更(上传/删除)时调用,使其检索缓存整体失效。
func (v *Versions) Bump(id string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.m[id]++
}

// Hash 返回文本的 sha256 十六进制摘要,用作缓存 key 的一部分(避免超长 key)。
func Hash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
