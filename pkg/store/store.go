package store

import(
	"fmt"
	"encoding/binary"
	"os"
	"path"
	"syscall"

	"golang.org/x/sys/unix"

	"kvstore/btree"
)

const DB_SIG = "BuildYourOwnDB06"

type Mmap struct {
	total  int      // mmap size, can be larger than the file size
	chunks [][]byte // multiple mmaps, can be non-continuous
}

type pageBuffer struct {
	flushed uint64   // database size in number of pages
	temp    [][]byte // newly allocated pages
}

type Store struct {
	path string
	fd int
	tree *btree.BTree
	mmap Mmap
	page pageBuffer
	failed bool // Did the last update fail?
}

func NewStore(filePath string) *Store {
	db := &Store{
        path: filePath,
    }

    db.tree = btree.MakeBTree(
        db.pageRead,
        db.pageAppend,
        db.pageDelete,
    )

    return db
}

func (db *Store) Open() error {
	fd, err := createFileSync(db.path)
	if err != nil {
		return err
	}

	db.fd = fd

	// Determine the current file size.
	var st syscall.Stat_t
	if err := syscall.Fstat(db.fd, &st); err != nil {
		syscall.Close(db.fd)
		return err
	}

	fileSize := int(st.Size)

	// Map the existing database file.
	if fileSize > 0 {
		chunk, err := syscall.Mmap(
			db.fd,
			0,
			fileSize,
			syscall.PROT_READ,
			syscall.MAP_SHARED,
		)
		if err != nil {
			syscall.Close(db.fd)
			return err
		}

		db.mmap.total = fileSize
		db.mmap.chunks = append(db.mmap.chunks, chunk)
	}

	// Load the metadata page (or initialize a new DB).
	if err := readRoot(db, st.Size); err != nil {
		if len(db.mmap.chunks) > 0 {
			syscall.Munmap(db.mmap.chunks[0])
		}
		syscall.Close(db.fd)
		return err
	}

	return nil
}

func (db *Store) Get(key []byte) (bool, []byte) {
    return db.tree.Lookup(key)
}

func (db *Store) Set(key []byte, val []byte) error {
    meta := saveMeta(db) // save the in-memory state (tree root)
    if err := db.tree.Insert(key, val); err != nil {
        return err // length limit
    }
    return updateOrRevert(db, meta)
}

func (db *Store) Del(key []byte) (bool, error) {
    err := db.tree.Delete(key)
	if (err != nil) {
		return false, err
	}
    return true, updateFile(db)
}

func (db *Store) pageRead(ptr uint64) []byte {
    start := uint64(0)
    for _, chunk := range db.mmap.chunks {
        end := start + uint64(len(chunk))/btree.BTREE_PAGE_SIZE
        if ptr < end {
            offset := btree.BTREE_PAGE_SIZE * (ptr - start)
            return chunk[offset : offset+btree.BTREE_PAGE_SIZE]
        }
        start = end
    }
    panic("bad ptr")
}

func (db *Store) pageAppend(node []byte) uint64 {
    ptr := db.page.flushed + uint64(len(db.page.temp)) // just append
    db.page.temp = append(db.page.temp, node)
    return ptr
}

func (db *Store) pageDelete(ptr uint64) {
	
}

func updateFile(db *Store) error {
    // 1. Write new nodes.
    if err := writePages(db); err != nil {
        return err
    }
    // 2. `fsync` to enforce the order between 1 and 3.
    if err := syscall.Fsync(db.fd); err != nil {
        return err
    }
    // 3. Update the root pointer atomically.
    if err := updateRoot(db); err != nil {
        return err
    }
    // 4. `fsync` to make everything persistent.
    return syscall.Fsync(db.fd)
}

func createFileSync(file string) (int, error) {
    // obtain the directory fd
    flags := os.O_RDONLY | syscall.O_DIRECTORY
    dirfd, err := syscall.Open(path.Dir(file), flags, 0)
    if err != nil {
        return -1, fmt.Errorf("open directory: %w", err)
    }
    defer syscall.Close(dirfd)
    // open or create the file
    flags = os.O_RDWR | os.O_CREATE
    fd, err := syscall.Openat(dirfd, path.Base(file), flags, 0o644)
    if err != nil {
        return -1, fmt.Errorf("open file: %w", err)
    }
    // fsync the directory
    if err = syscall.Fsync(dirfd); err != nil {
        _ = syscall.Close(fd)  // may leave an empty file
        return -1, fmt.Errorf("fsync directory: %w", err)
    }
    return fd, nil
}

func extendMmap(db *Store, size int) error {
    if size <= db.mmap.total {
        return nil // enough range
    }
    alloc := db.mmap.total
	if alloc < 64<<20 {
    	alloc = 64 << 20
	}
    for db.mmap.total + alloc < size {
        alloc *= 2 // still not enough?
    }
    chunk, err := syscall.Mmap(
        db.fd, int64(db.mmap.total), alloc,
        syscall.PROT_READ, syscall.MAP_SHARED, // read-only
    )
    if err != nil {
        return fmt.Errorf("mmap: %w", err)
    }
    db.mmap.total += alloc
    db.mmap.chunks = append(db.mmap.chunks, chunk)
    return nil
}

func writePages(db *Store) error {
    // extend the mmap if needed
    size := (int(db.page.flushed) + len(db.page.temp)) * btree.BTREE_PAGE_SIZE
    if err := extendMmap(db, size); err != nil {
        return err
    }
    // write data pages to the file
    offset := int64(db.page.flushed * btree.BTREE_PAGE_SIZE)
    if _, err := unix.Pwritev(db.fd, db.page.temp, offset); err != nil {
        return err
    }
    // discard in-memory data
    db.page.flushed += uint64(len(db.page.temp))
    db.page.temp = db.page.temp[:0]
    return nil
}

func saveMeta(db *Store) []byte {
    var data [32]byte
    copy(data[:16], []byte(DB_SIG))
    binary.LittleEndian.PutUint64(data[16:], db.tree.GetRoot())
    binary.LittleEndian.PutUint64(data[24:], db.page.flushed)
    return data[:]
}

func loadMeta(db *Store, data []byte) {
	db.tree.SetRoot(binary.LittleEndian.Uint64(data[16:24]))
	db.page.flushed = binary.LittleEndian.Uint64(data[24:32])
}

func readRoot(db *Store, fileSize int64) error {
    if fileSize == 0 { // empty file
        db.page.flushed = 1 // the meta page is initialized on the 1st write
        return nil
    }
    // read the page
    data := db.mmap.chunks[0]
    loadMeta(db, data)
    // verify the page
    // ...
    return nil
}

func updateRoot(db *Store) error {
    if _, err := syscall.Pwrite(db.fd, saveMeta(db), 0); err != nil {
        return fmt.Errorf("write meta page: %w", err)
    }
    return nil
}

func updateOrRevert(db *Store, meta []byte) error {
    // Previous update failed while writing the meta page.
    // Restore the previous on-disk metadata first.
    if db.failed {
        if _, err := syscall.Pwrite(db.fd, meta, 0); err != nil {
            return err
        }
        if err := syscall.Fsync(db.fd); err != nil {
            return err
        }
        db.failed = false
    }

    err := updateFile(db)
    if err != nil {
        // Remember that the metadata page on disk may be corrupted.
        db.failed = true

        // Restore the in-memory state.
        loadMeta(db, meta)

        // Discard pages that haven't been flushed.
        db.page.temp = db.page.temp[:0]
    }

    return err
}