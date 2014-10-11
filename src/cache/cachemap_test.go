//
// Copyright (c) 2014 The pblcache Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
package cache

import (
	"testing"
)

func TestInsert(t *testing.T) {
	cmap := NewCacheMap(2)

	id := uint64(123)
	index, evictkey, evict := cmap.Insert(id)
	assert(t, cmap.bds[0].key == id)
	assert(t, cmap.bds[0].mru == false)
	assert(t, cmap.bds[0].used == true)
	assert(t, index == 0)
	assert(t, evictkey == INVALID_KEY)
	assert(t, evict == false)
}

func TestUsing(t *testing.T) {
	cmap := NewCacheMap(2)

	id := uint64(123)
	index, evictkey, evict := cmap.Insert(id)
	assert(t, cmap.bds[0].key == id)
	assert(t, cmap.bds[0].mru == false)
	assert(t, cmap.bds[0].used == true)
	assert(t, index == 0)
	assert(t, evictkey == INVALID_KEY)
	assert(t, evict == false)

	cmap.Using(index)
	assert(t, cmap.bds[0].key == id)
	assert(t, cmap.bds[0].mru == true)
	assert(t, cmap.bds[0].used == true)
}

func TestFree(t *testing.T) {
	cmap := NewCacheMap(2)

	id := uint64(123)
	index, evictkey, evict := cmap.Insert(id)
	assert(t, cmap.bds[0].key == id)
	assert(t, cmap.bds[0].mru == false)
	assert(t, cmap.bds[0].used == true)
	assert(t, index == 0)
	assert(t, evictkey == INVALID_KEY)
	assert(t, evict == false)

	cmap.Free(index)
	assert(t, cmap.bds[0].mru == false)
	assert(t, cmap.bds[0].used == false)
}

func TestEvictions(t *testing.T) {
	cmap := NewCacheMap(2)

	id1 := uint64(123)
	id2 := uint64(456)
	id3 := uint64(678)

	index, evictkey, evict := cmap.Insert(id1)
	assert(t, cmap.bds[0].key == id1)
	assert(t, cmap.bds[0].mru == false)
	assert(t, cmap.bds[0].used == true)
	assert(t, index == 0)
	assert(t, evictkey == INVALID_KEY)
	assert(t, evict == false)

	index, evictkey, evict = cmap.Insert(id2)
	assert(t, cmap.bds[0].key == id1)
	assert(t, cmap.bds[0].mru == false)
	assert(t, cmap.bds[0].used == true)
	assert(t, cmap.bds[1].key == id2)
	assert(t, cmap.bds[1].mru == false)
	assert(t, cmap.bds[1].used == true)
	assert(t, index == 1)
	assert(t, evictkey == INVALID_KEY)
	assert(t, evict == false)

	cmap.Using(0)
	assert(t, cmap.bds[0].key == id1)
	assert(t, cmap.bds[0].mru == true)
	assert(t, cmap.bds[0].used == true)

	index, evictkey, evict = cmap.Insert(id3)
	assert(t, cmap.bds[0].key == id1)
	assert(t, cmap.bds[0].mru == false)
	assert(t, cmap.bds[0].used == true)
	assert(t, cmap.bds[1].key == id3)
	assert(t, cmap.bds[1].mru == false)
	assert(t, cmap.bds[1].used == true)
	assert(t, index == 1)
	assert(t, evictkey == id2)
	assert(t, evict == true)

	cmap.Free(1)
	assert(t, cmap.bds[1].mru == false)
	assert(t, cmap.bds[1].used == false)

	index, evictkey, evict = cmap.Insert(id2)
	assert(t, cmap.bds[0].key == id2)
	assert(t, cmap.bds[0].mru == false)
	assert(t, cmap.bds[0].used == true)
	assert(t, cmap.bds[1].mru == false)
	assert(t, cmap.bds[1].used == false)
	assert(t, index == 0)
	assert(t, evictkey == id1)
	assert(t, evict == true)
}
