// | type | nkeys |  pointers  |  offsets   | key-values | unused |
// |  2B  |   2B  | nkeys × 8B | nkeys × 2B |     ...    |        |

package btree

import (
    "encoding/binary"
    "bytes"
    "kvstore/internal/util"
)

// type Node struct {
//     keys [][]byte
//     // one of the following
//     vals [][]byte   // for leaf nodes only
//     kids []*Node    // for internal nodes only
// }

type BNode []byte

// getters
func (node BNode) btype() uint16 {
    return binary.LittleEndian.Uint16(node[0:2])
}
func (node BNode) nkeys() uint16 {
    return binary.LittleEndian.Uint16(node[2:4])
}

// setter
func (node BNode) setHeader(btype uint16, nkeys uint16) {
    binary.LittleEndian.PutUint16(node[0:2], btype)
    binary.LittleEndian.PutUint16(node[2:4], nkeys)
}

// read and write the child pointers array
func (node BNode) getPtr(idx uint16) uint64 {
    util.Assert(idx < node.nkeys())
    pos := 4 + 8*idx
    endPos := pos + 8
    return binary.LittleEndian.Uint64(node[pos:endPos])
}
func (node BNode) setPtr(idx uint16, val uint64) {
    util.Assert(idx < node.nkeys())
    pos := 4 + 8*idx
    endPos := pos + 8
    binary.LittleEndian.PutUint64(node[pos:endPos], val)
}

// get offset for nth key
func (node BNode) getOffset(idx uint16) uint16 {
	if idx == 0 {
		return 0
	}

	pos := 4 + 8*node.nkeys() + 2*(idx-1)
	endPos := pos + 2
	return binary.LittleEndian.Uint16(node[pos:endPos])
}

func (node BNode) setOffset(idx uint16, val uint16) {
    util.Assert(idx <= node.nkeys())
    pos := 4 + 8*node.nkeys() + 2*(idx-1)
    endPos := pos + 2
    binary.LittleEndian.PutUint16(node[pos:endPos], val)
}

func (node BNode) kvPos(idx uint16) uint16 {
    util.Assert(idx <= node.nkeys())
    return 4 + 8*node.nkeys() + 2*node.nkeys() + node.getOffset(idx)
}

func (node BNode) getKey(idx uint16) []byte {
	util.Assert(idx < node.nkeys())
	pos := node.kvPos(idx)
	endPos := pos + 2
	klen := binary.LittleEndian.Uint16(node[pos:endPos])
	return node[pos + 4:][:klen]
}

func (node BNode) getVal(idx uint16) []byte {
	util.Assert(idx < node.nkeys())
	pos := node.kvPos(idx)
	endPos := pos + 2
	klen := binary.LittleEndian.Uint16(node[pos:endPos])
	endPos = pos + 4
	vlen := binary.LittleEndian.Uint16(node[pos+2:endPos])
	return node[pos + 4 + klen:][:vlen]
}

// node size in bytes
func (node BNode) nbytes() uint16 {
    return node.kvPos(node.nkeys()) // uses the offset value of the last key
}

func nodeAppendKV(new BNode, idx uint16, ptr uint64, key []byte, val []byte) {
    // ptrs
    new.setPtr(idx, ptr)
    // KVs
    pos := new.kvPos(idx)   // uses the offset value of the previous key
    // 4-bytes KV sizes
    endPos := pos + 2
    binary.LittleEndian.PutUint16(new[pos:endPos], uint16(len(key)))
    endPos = pos + 4
    binary.LittleEndian.PutUint16(new[pos+2:endPos], uint16(len(val)))
    // KV data
    copy(new[pos+4:], key)
    copy(new[pos+4+uint16(len(key)):], val)
    // update the offset value for the next key
    new.setOffset(idx+1, new.getOffset(idx)+4+uint16((len(key)+len(val))))
}

func nodeAppendRange(
	new BNode, old BNode, dstNew uint16, srcOld uint16, n uint16,
) {
    for i := uint16(0); i < n; i++ {
        dst, src := dstNew+i, srcOld+i
        nodeAppendKV(new, dst,
            old.getPtr(src), old.getKey(src), old.getVal(src))
    }
}

func leafInsert(
    new BNode, old BNode, idx uint16, key []byte, val []byte,
) {
    new.setHeader(BNODE_LEAF, old.nkeys()+1)
    nodeAppendRange(new, old, 0, 0, idx)    // copy the keys before `idx`
    nodeAppendKV(new, idx, 0, key, val)     // the new key
    nodeAppendRange(new, old, idx+1, idx, old.nkeys()-idx)  // keys from `idx`
}

func leafUpdate(
    new BNode, old BNode, idx uint16, key []byte, val []byte,
) {
    new.setHeader(BNODE_LEAF, old.nkeys())
    nodeAppendRange(new, old, 0, 0, idx)
    nodeAppendKV(new, idx, 0, key, val)
    nodeAppendRange(new, old, idx+1, idx+1, old.nkeys()-(idx+1))
}

func leafDelete(
    new BNode, old BNode, idx uint16,
) {
    new.setHeader(BNODE_LEAF, old.nkeys()-1)
    nodeAppendRange(new, old, 0, 0, idx)
    nodeAppendRange(new, old, idx, idx+1, old.nkeys()-(idx+1))
}

func nodeLookup(node BNode, key []byte) uint16 {
    lo, hi := uint16(0), node.nkeys()-1
    for lo < hi {
        mid := lo + (hi - lo) / 2
        cmp := bytes.Compare(node.getKey(mid), key)

        if (cmp == 0) {
            return mid
        }

        if cmp < 0 {
            lo = mid + 1
        } else {
            hi = mid
        }
    }
    
    if (bytes.Equal(node.getKey(lo), key)) {
        return lo
    }

    return lo-1
}

// func nodeLookup(node BNode, key []byte) uint16 {
// 	n := node.nkeys()
//     var i uint16
// 	for i = 0; i < n; i++ {
// 		cmp := bytes.Compare(node.getKey(i), key)
//         if cmp == 0 {
//             fmt.Printf("nodeLookup: i=%d, n=%d, key=%s (found)\n", i, n, node.getKey(i))
//             return i
//         }
//         if cmp > 0 {
//             fmt.Printf("nodeLookup: i=%d, n=%d, key=%s (not found)\n", i, n, node.getKey(i))
//             return i - 1
//         }
// 	}

//     fmt.Printf("nodeLookup: i=%d, n=%d, key=%s\n", i, n, key)

// 	return i-1
// }

func KvUpdate(
	new BNode, node BNode, key []byte, val []byte,
) {
	idx := nodeLookup(node, key)
	if bytes.Equal(key, node.getKey(idx)) {
		leafUpdate(new, node, idx, key, val)   // found, update it
	} else {
		leafInsert(new, node, idx+1, key, val) // not found. insert
	}
}

func nodeSplit2(left BNode, right BNode, old BNode) {
	util.Assert(old.nkeys() >= 2)

	nleft := old.nkeys()/2

	left_bytes := func() uint16 {
		return 4 + 8*nleft + 2*nleft + old.getOffset(nleft)
	}

	for left_bytes() > BTREE_PAGE_SIZE {
		nleft--
	}
	util.Assert(nleft >= 1)

	right_bytes := func() uint16 {
		return old.nbytes() - left_bytes() + 4
	}

	for right_bytes() > BTREE_PAGE_SIZE {
		nleft++
	}
	util.Assert(nleft < old.nkeys())

	nright := old.nkeys() - nleft

	left.setHeader(old.btype(), nleft)
	right.setHeader(old.btype(), nright)
	nodeAppendRange(left, old, 0, 0, nleft)
	nodeAppendRange(right, old, 0, nleft, nright)
	util.Assert(right.nbytes() <= BTREE_PAGE_SIZE)
}

// split a node if it's too big. the results are 1~3 nodes.
func nodeSplit3(old BNode) (uint16, [3]BNode) {
    if old.nbytes() <= BTREE_PAGE_SIZE {
        old = old[:BTREE_PAGE_SIZE]
        return 1, [3]BNode{old} // not split
    }
    left := BNode(make([]byte, 2*BTREE_PAGE_SIZE)) // might be split later
    right := BNode(make([]byte, BTREE_PAGE_SIZE))
    nodeSplit2(left, right, old)
    if left.nbytes() <= BTREE_PAGE_SIZE {
        left = left[:BTREE_PAGE_SIZE]
        return 2, [3]BNode{left, right} // 2 nodes
    }
    leftleft := BNode(make([]byte, BTREE_PAGE_SIZE))
    middle := BNode(make([]byte, BTREE_PAGE_SIZE))
    nodeSplit2(leftleft, middle, left)
    util.Assert(leftleft.nbytes() <= BTREE_PAGE_SIZE)
    return 3, [3]BNode{leftleft, middle, right} // 3 nodes
}

func nodeMerge(new BNode, left BNode, right BNode) {
    util.Assert(left.nbytes() + right.nbytes() <= BTREE_PAGE_SIZE);
    new.setHeader(left.btype(), left.nkeys() + right.nkeys());
    nodeAppendRange(new, left, 0, 0, left.nkeys());
    nodeAppendRange(new, right, left.nkeys(), 0, right.nkeys());
}