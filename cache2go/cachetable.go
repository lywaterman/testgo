package cache2go

import (
	"log"
	"sort"
	"sync"
	"time"
)

type CacheTable struct {
	sync.RWMutex

	name string
	items map[interface{}]*CacheItem

	cleanupTimer *time.Timer
	cleanupInterval time.Duration

	logger *log.Logger

	loadData func(key interface{}, args ...interface{}) *CacheItem

	addedItem []func(item *CacheItem)

	aboutToDeleteItem []func(item *CacheItem)
}

func (table *CacheTable) Count() int {
	table.RLock()
	defer table.RUnlock()
	return len(table.items)
}

func (table *CacheTable) Foreach(trans func(key interface{}, item *CacheItem)) {
	table.RLock()
	defer table.RUnlock()

	for k, v := range table.items {
		trans(k, v)
	}
}

func (table *CacheTable) SetDataLoader(f func(interface{}, ...interface{}) *CacheItem) {
	table.Lock()
	defer table.Unlock()
	table.loadData = f
}

func (table *CacheTable) SetAddedItemCallback(f func(item *CacheItem)) {
	if len(table.addedItem) > 0 {

	}
}

func (table *CacheTable) SetAboutToDeleteItemCallback(f func(item *CacheItem)) {
	if len(table.aboutToDeleteItem) > 0 {

	}
	table.Lock()
	defer table.Unlock()
	table.aboutToDeleteItem = append(table.aboutToDeleteItem, f)
}

type CacheItemPair struct {
	Key interface{}
	AccessCount int64
}

type CacheItemPairList []CacheItemPair

func (p CacheItemPairList) Swap(i, j int) {
	p[i], p[j] = p[j], p[i]
}

func (p CacheItemPairList) Len() int {
	return len(p)
}

func (p CacheItemPairList) Less(i, j int) bool {
	return p[i].AccessCount > p[j].AccessCount
}

func (table *CacheTable) MostAccessed(count int64) []*CacheItem {
	table.RLock()
	defer table.RUnlock()

	p := make(CacheItemPairList, len(table.items))
	i := 0

	for k, v := range table.items {
		p[i] = CacheItemPair{k, v.accessCount}
		i++
	}
	sort.Sort(p)

	var r []*CacheItem
	c := int64(0)

	for _, v := range p {
		if c >= count {
			break
		}

		item, ok := table.items[v.Key]
		if ok {
			r = append(r, item)
		}
		c++
	}
	return r
}

func (table *CacheTable) log(v ...interface{}) {
	if table.logger == nil {
		return
	}
	table.logger.Println(v...)
}