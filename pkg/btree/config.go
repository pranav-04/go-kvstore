package btree

const (
    BNODE_NODE = 1 // internal nodes with pointers
    BNODE_LEAF = 2 // leaf nodes with values
    HEADER = 4 // header size in bytes (btype + nkeys)
	BTREE_PAGE_SIZE = 4096
    BTREE_MAX_KEY_SIZE = 1000
    BTREE_MAX_VAL_SIZE = 3000
)