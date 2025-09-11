package gorgon

import (
	"fmt"
	"strings"

	"github.com/couchbaselabs/gorgon/src/gorgon/util"
)

type IntMap []KeyValueInt

func (im IntMap) Get(key string) (i int, ok bool) {
	for _, kv := range im {
		if kv.Key == key {
			return kv.Value, true
		}
	}
	return 0, false
}

func (im IntMap) Put(key string, value int) IntMap {
	for i := range im {
		if im[i].Key == key {
			if im[i].Value == value {
				return im
			}
			ret := make([]KeyValueInt, len(im))
			copy(ret, im)
			ret[i].Value = value
			return ret
		}
	}
	n := len(im)
	ret := make([]KeyValueInt, n+1)
	i := 0
	for ; i < n && im[i].Key < key; i++ {
		ret[i] = im[i]
	}
	ret[i] = KeyValueInt{key, value}
	for ; i < n; i++ {
		ret[i+1] = im[i]
	}
	return ret
}

func (im IntMap) Hash() uint64 {
	var h uint64
	for _, kv := range im {
		h = h*31 + util.Fnv1aHashString(kv.Key)
		h = h*31 + util.HashUint64(uint64(kv.Value))
	}
	return h
}

func (im IntMap) Equals(other IntMap) bool {
	if len(im) != len(other) {
		return false
	}
	for i := range im {
		if im[i] != other[i] {
			return false
		}
	}
	return true
}

func (im IntMap) String() string {
	var sb strings.Builder
	sb.WriteByte('{')
	first := true
	for _, kv := range im {
		if first {
			first = false
			fmt.Fprintf(&sb, "%q: %d", kv.Key, kv.Value)
		} else {
			fmt.Fprintf(&sb, ", %q: %d", kv.Key, kv.Value)
		}
	}
	sb.WriteByte('}')
	return sb.String()
}
