package main

import (
	"fmt"
	"kvstore/store"
)

Test B*-tree code

type KvStore struct {
    tree  *btree.BTree
    ref   map[string]string // the reference data
    pages map[uint64]btree.BNode  // in-memory pages
}

func newKvStore() *KvStore {
    pages := map[uint64]btree.BNode{}
    return &KvStore{
        tree: btree.MakeBTree(
            func(ptr uint64) []byte {
                node, ok := pages[ptr]
                util.Assert(ok)
                return node
            },
            func(node []byte) uint64 {
                ptr := uint64(uintptr(unsafe.Pointer(&node[0])))
                util.Assert(pages[ptr] == nil)
                pages[ptr] = node
                return ptr
            },
            func(ptr uint64) {
                util.Assert(pages[ptr] != nil)
                delete(pages, ptr)
            }),
        ref:   map[string]string{},
        pages: pages,
    }
}

func main() {
	fmt.Println("Hello, World!")
}