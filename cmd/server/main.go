package main

import (
	"fmt"
	"kvstore/internal/store"
)

// Test B*-tree code

// type KvStore struct {
//     tree  *btree.BTree
//     ref   map[string]string // the reference data
//     pages map[uint64]btree.BNode  // in-memory pages
// }

// func newKvStore() *KvStore {
//     pages := map[uint64]btree.BNode{}
//     return &KvStore{
//         tree: btree.MakeBTree(
//             func(ptr uint64) []byte {
//                 node, ok := pages[ptr]
//                 util.Assert(ok)
//                 return node
//             },
//             func(node []byte) uint64 {
//                 ptr := uint64(uintptr(unsafe.Pointer(&node[0])))
//                 util.Assert(pages[ptr] == nil)
//                 pages[ptr] = node
//                 return ptr
//             },
//             func(ptr uint64) {
//                 util.Assert(pages[ptr] != nil)
//                 delete(pages, ptr)
//             }),
//         ref:   map[string]string{},
//         pages: pages,
//     }
// }

const DB_FILE = "KvStore.db"

func main() {
	fmt.Println("Hello, World!")

	validatorMap := make(map[string]string)

	// append 1000 keys
	kvstore := store.NewStore(DB_FILE)
	kvstore.Open()
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("key%d", i)
		val := fmt.Sprintf("val%d", i)
		kvstore.Set([]byte(key), []byte(val))
		validatorMap[key] = val
	}

	// lookup 10 keys
	for i := 990; i < 1000; i++ {
		key := fmt.Sprintf("key%d", i)
		ok, val := kvstore.Get([]byte(key))
		if !ok {
			panic(fmt.Sprintf("key not found: %s", key))
		}
		if string(val) != validatorMap[key] {
			panic(fmt.Sprintf("value mismatch: key=%s, got=%s, want=%s", key, val, validatorMap[key]))
		}
		fmt.Printf("lookup: key=%s, val=%s\n", key, val)
	}
}