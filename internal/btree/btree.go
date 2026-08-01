package btree

import (
    "bytes"
    "errors"
    "kvstore/internal/util"
)

var (
    ErrKeyTooLarge = errors.New("key is too large")
    ErrValTooLarge = errors.New("value is too large")
)

type BTree struct {
    // root pointer (a nonzero page number)
    root uint64
    // callbacks for managing on-disk pages
    get func(uint64) []byte // read data from a page number
    alloc func([]byte) uint64 // allocate a new page number with data
    del func(uint64)        // deallocate a page number
}

func MakeBTree(get func(uint64) []byte, alloc func([]byte) uint64, del func(uint64)) *BTree {
    return &BTree{
        root: 0,
        get: get,
        alloc: alloc,
        del: del,
    }
}

func (tree *BTree) GetRoot() uint64 {
    return tree.root
}

func (tree *BTree) SetRoot(root uint64) {
    tree.root = root
}

func treeInsert(tree *BTree, node BNode, key []byte, val []byte) BNode {
    // The extra size allows it to exceed 1 page temporarily.
    new := BNode(make([]byte, 2*BTREE_PAGE_SIZE))
    // where to insert the key?
    idx := nodeLookup(node, key)
    switch node.btype() {
        case BNODE_LEAF: // leaf node
            if bytes.Equal(key, node.getKey(idx)) {
                leafUpdate(new, node, idx, key, val)   // found, update it
            } else {
                leafInsert(new, node, idx+1, key, val) // not found, insert
            }
        case BNODE_NODE: // internal node, walk into the child node
            // recursive insertion to the kid node
            kptr := node.getPtr(idx)
            knode := treeInsert(tree, tree.get(kptr), key, val)
            // after insertion, split the result
            nsplit, split := nodeSplit3(knode)
            // deallocate the old kid node
            tree.del(kptr)
            // update the kid links
            nodeReplaceKidN(tree, new, node, idx, split[:nsplit]...)
    }
    return new
}

func nodeReplaceKidN(
	tree *BTree, new BNode, old BNode, idx uint16,
	kids ...BNode,
) {
	inc := uint16(len(kids))
	new.setHeader(old.btype(), old.nkeys() + inc - 1)
	nodeAppendRange(new, old, 0, 0, idx)
	for i, node := range kids {
		nodeAppendKV(new, idx+uint16(i), tree.alloc(node), node.getKey(0), nil)
	}

	nodeAppendRange(new, old, idx+inc, idx+1, old.nkeys()-(idx+1))
}

func nodeReplace2Kid(new BNode, old BNode, idx uint16, ptr uint64, key []byte) {
    new.setHeader(old.btype(), old.nkeys() - 1)
    nodeAppendRange(new, old, 0, 0, idx)
    nodeAppendKV(new, idx, ptr, key, nil)
    nodeAppendRange(new, old, idx+1, idx+2, old.nkeys()-(idx+2))
}

func checkLimit(key []byte, val []byte) error {
    if len(key) > BTREE_MAX_KEY_SIZE {
        return ErrKeyTooLarge
    }
    if len(val) > BTREE_MAX_VAL_SIZE {
        return ErrValTooLarge
    }
    return nil
}

func (tree *BTree) Insert(key []byte, val []byte) error {
    // 1. check the length limit imposed by the node format
    if err := checkLimit(key, val); err != nil {
        return err // the only way for an update to fail
    }
    // 2. create the first node
    if tree.root == 0 {
        root := BNode(make([]byte, BTREE_PAGE_SIZE))
        root.setHeader(BNODE_LEAF, 2)
        // a dummy key, this makes the tree cover the whole key space.
        // thus a lookup can always find a containing node.
        nodeAppendKV(root, 0, 0, nil, nil)
        nodeAppendKV(root, 1, 0, key, val)
        tree.root = tree.alloc(root)
        return nil
    }
    // 3. insert the key
    node := treeInsert(tree, tree.get(tree.root), key, val)
    // 4. grow the tree if the root is split
    nsplit, split := nodeSplit3(node)
    oldRoot := tree.root
    if nsplit > 1 { // the root was split, add a new level.
        root := BNode(make([]byte, BTREE_PAGE_SIZE))
        root.setHeader(BNODE_NODE, nsplit)
        for i, knode := range split[:nsplit] {
            ptr, key := tree.alloc(knode), knode.getKey(0)
            nodeAppendKV(root, uint16(i), ptr, key, nil)
        }

        tree.root = tree.alloc(root)
    } else {
        tree.root = tree.alloc(split[0])
    }
    tree.del(oldRoot)
    return nil
}

// should the updated kid be merged with a sibling?
func shouldMerge(
    tree *BTree, node BNode, idx uint16, updated BNode,
) (int, BNode) {
    if updated.nbytes() > BTREE_PAGE_SIZE/4 {
        return 0, BNode{}
    }
    if idx > 0 {
        sibling := BNode(tree.get(node.getPtr(idx - 1)))
        merged := sibling.nbytes() + updated.nbytes() - HEADER
        if merged <= BTREE_PAGE_SIZE {
            return -1, sibling // left
        }
    }
    if idx+1 < node.nkeys() {
        sibling := BNode(tree.get(node.getPtr(idx + 1)))
        merged := sibling.nbytes() + updated.nbytes() - HEADER
        if merged <= BTREE_PAGE_SIZE {
            return +1, sibling // right
        }
    }
    return 0, BNode{}
}

func treeDelete(tree *BTree, node BNode, key []byte) BNode {
    // where to insert the key?
    idx := nodeLookup(node, key) // node.getKey(idx) <= key
    switch node.btype() {
    case BNODE_LEAF: // leaf node
        if bytes.Equal(key, node.getKey(idx)) {
            new := BNode(make([]byte, BTREE_PAGE_SIZE))
            leafDelete(new, node, idx)   // found, delete it
            return new
        } else {
            return BNode{} // not found
        }
    case BNODE_NODE: // internal node, walk into the child node
        return nodeDelete(tree, node, idx, key)
    }
    return BNode{}
}

// delete a key from an internal node; part of the treeDelete()
func nodeDelete(tree *BTree, node BNode, idx uint16, key []byte) BNode {
    // recurse into the kid
    kptr := node.getPtr(idx)
    updated := treeDelete(tree, tree.get(kptr), key)
    if len(updated) == 0 {
        return BNode{} // not found
    }
    tree.del(kptr)
    // check for merging
    new := BNode(make([]byte, BTREE_PAGE_SIZE))
    mergeDir, sibling := shouldMerge(tree, node, idx, updated)
    switch {
    case mergeDir < 0: // left
        merged := BNode(make([]byte, BTREE_PAGE_SIZE))
        nodeMerge(merged, sibling, updated)
        tree.del(node.getPtr(idx - 1))
        nodeReplace2Kid(new, node, idx-1, tree.alloc(merged), merged.getKey(0))
    case mergeDir > 0: // right
        merged := BNode(make([]byte, BTREE_PAGE_SIZE))
        nodeMerge(merged, updated, sibling)
        tree.del(node.getPtr(idx + 1))
        nodeReplace2Kid(new, node, idx, tree.alloc(merged), merged.getKey(0))
    case mergeDir == 0 && updated.nkeys() == 0:
        util.Assert(node.nkeys() == 1 && idx == 0) // 1 empty child but no sibling
        new.setHeader(BNODE_NODE, 0)          // the parent becomes empty too
    case mergeDir == 0 && updated.nkeys() > 0: // no merge
        nodeReplaceKidN(tree, new, node, idx, updated)
    }
    return new
}

func (tree *BTree) Delete(key []byte) error {
    if tree.root == 0 {
        return nil // empty tree, nothing to delete
    }
    node := treeDelete(tree, tree.get(tree.root), key)
    if len(node) == 0 {
        return nil // not found, nothing to delete
    }
    // the root may be shrunk, but it cannot be merged with a sibling.
    oldRoot := tree.root
    newRoot := tree.alloc(node)    // Allocate and write new root first
    tree.root = newRoot            // Atomic update of root pointer
    tree.del(oldRoot)        
    return nil
}

func treeLookup(tree *BTree, node BNode, key []byte) (bool, []byte) {
    idx := nodeLookup(node, key)
    switch node.btype() {
    case BNODE_LEAF:
        if bytes.Equal(key, node.getKey(idx)) {
            return true, node.getVal(idx)
        }
        return false, nil
    case BNODE_NODE:
        return treeLookup(tree, tree.get(node.getPtr(idx)), key)
    }
    return false, nil
}


func (tree *BTree) Lookup(key []byte) (bool, []byte) {
    if tree.root == 0 {
        return false, nil // empty tree
    }

    return treeLookup(tree, tree.get(tree.root), key)
}