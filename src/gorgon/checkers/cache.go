package checkers

type Cache struct {
	table    []*cacheEntry
	head     *cacheEntry
	tail     *cacheEntry
	count    int
	maxCount int
	hash     func(any) uint64
	equal    func(a, b any) bool
}

func NewCache(maxCount int, hash func(any) uint64, equal func(a, b any) bool) *Cache {
	if maxCount < 2 {
		maxCount = 2
	}
	return &Cache{
		table:    make([]*cacheEntry, maxCount),
		maxCount: maxCount,
		hash:     hash,
		equal:    equal,
	}
}

func (c *Cache) Len() int {
	return c.count
}

func (c *Cache) Clear() {
	for i := range c.table {
		c.table[i] = nil
	}
	for entry := c.head; entry != nil; {
		next := entry.next
		entry.chainNext = nil
		entry.prev = nil
		entry.next = nil
		entry = next
	}
	c.head = nil
	c.tail = nil
	c.count = 0
}

func (c *Cache) Contains(value any) bool {
	hash := c.hash(value)
	for entry := c.table[c.bucket(hash)]; entry != nil; entry = entry.chainNext {
		if entry.hash == hash && c.equal(entry.value, value) {
			return true
		}
	}
	return false
}

func (c *Cache) Insert(value any) bool {
	hash := c.hash(value)
	b := c.bucket(hash)
	headEntry := c.table[b]
	for entry := headEntry; entry != nil; entry = entry.chainNext {
		if entry.hash == hash && c.equal(entry.value, value) {
			c.bringToFront(entry)
			return false
		}
	}
	var newEntry *cacheEntry
	if c.count < c.maxCount {
		newEntry = &cacheEntry{
			chainNext: headEntry,
			next:      c.head,
			value:     value,
			hash:      hash}
	} else {
		newEntry = c.removeTail()
		headEntry = c.table[b]
		newEntry.chainNext = headEntry
		newEntry.prev = nil
		newEntry.next = c.head
		newEntry.value = value
		newEntry.hash = hash
	}
	if c.count == 0 {
		c.head = newEntry
		c.tail = newEntry
		c.count = 1
	} else {
		c.head.prev = newEntry
		c.head = newEntry
		c.count++
	}
	c.table[b] = newEntry
	return true
}

func (c *Cache) removeTail() *cacheEntry {
	c.count--
	tail := c.tail
	c.tail = tail.prev
	tail.remove()
	hash := tail.hash
	b := c.bucket(hash)
	entry := c.table[b]
	if entry == tail {
		c.table[b] = tail.chainNext
		return tail
	}
	for ; entry != nil; entry = entry.chainNext {
		if entry.chainNext == tail {
			entry.chainNext = tail.chainNext
			return tail
		}
	}
	panic("inconsistent cache state")
}

func (c *Cache) bringToFront(entry *cacheEntry) {
	if c.head != entry {
		if c.tail == entry {
			c.tail = entry.prev
		}
		entry.remove()
		entry.prev = nil
		entry.next = c.head
		c.head.prev = entry
		c.head = entry
	}
}

func (c *Cache) bucket(h uint64) int {
	h = (h ^ (h >> 32)) * 0x5851f42d4c957f2d
	h = (h ^ (h >> 32)) & 0xffffffff
	return int((h * uint64(len(c.table))) >> 32)
}

type cacheEntry struct {
	chainNext *cacheEntry
	prev      *cacheEntry
	next      *cacheEntry
	value     any
	hash      uint64
}

func (e *cacheEntry) remove() {
	prev := e.prev
	next := e.next
	if prev != nil {
		prev.next = next
	}
	if next != nil {
		next.prev = prev
	}
}
