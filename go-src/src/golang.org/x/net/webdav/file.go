// Copyright 2014 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package webdav

import (
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// slashClean is equivalent to but slightly more efficient than
// path.Clean("/" + name).
func slashClean(name string) string {
	if name == "" || name[0] != '/' {
		name = "/" + name
	}
	return path.Clean(name)
}

// A FileSystem implements access to a collection of named files. The elements
// in a file path are separated by slash ('/', U+002F) characters, regardless
// of host operating system convention.
//
// Each method has the same semantics as the os package's function of the same
// name.
type FileSystem interface {
	Mkdir(name string, perm os.FileMode) error
	OpenFile(name string, flag int, perm os.FileMode) (File, error)
	RemoveAll(name string) error
	Rename(oldName, newName string) error
	Stat(name string) (os.FileInfo, error)
}

// A File is returned by a FileSystem's OpenFile method and can be served by a
// Handler.
type File interface {
	http.File
	io.Writer
}

// A Dir implements FileSystem using the native file system restricted to a
// specific directory tree.
//
// While the FileSystem.OpenFile method takes '/'-separated paths, a Dir's
// string value is a filename on the native file system, not a URL, so it is
// separated by filepath.Separator, which isn't necessarily '/'.
//
// An empty Dir is treated as ".".
type Dir string

func (d Dir) resolve(name string) string {
	// This implementation is based on Dir.Open's code in the standard net/http package.
	if filepath.Separator != '/' && strings.IndexRune(name, filepath.Separator) >= 0 ||
		strings.Contains(name, "\x00") {
		return ""
	}
	dir := string(d)
	if dir == "" {
		dir = "."
	}
	return filepath.Join(dir, filepath.FromSlash(slashClean(name)))
}

func (d Dir) Mkdir(name string, perm os.FileMode) error {
	if name = d.resolve(name); name == "" {
		return os.ErrNotExist
	}
	return os.Mkdir(name, perm)
}

func (d Dir) OpenFile(name string, flag int, perm os.FileMode) (File, error) {
	if name = d.resolve(name); name == "" {
		return nil, os.ErrNotExist
	}
	f, err := os.OpenFile(name, flag, perm)
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (d Dir) RemoveAll(name string) error {
	if name = d.resolve(name); name == "" {
		return os.ErrNotExist
	}
	if name == filepath.Clean(string(d)) {
		// Prohibit removing the virtual root directory.
		return os.ErrInvalid
	}
	return os.RemoveAll(name)
}

func (d Dir) Rename(oldName, newName string) error {
	if oldName = d.resolve(oldName); oldName == "" {
		return os.ErrNotExist
	}
	if newName = d.resolve(newName); newName == "" {
		return os.ErrNotExist
	}
	if root := filepath.Clean(string(d)); root == oldName || root == newName {
		// Prohibit renaming from or to the virtual root directory.
		return os.ErrInvalid
	}
	return os.Rename(oldName, newName)
}

func (d Dir) Stat(name string) (os.FileInfo, error) {
	if name = d.resolve(name); name == "" {
		return nil, os.ErrNotExist
	}
	return os.Stat(name)
}

// NewMemFS returns a new in-memory FileSystem implementation.
func NewMemFS() FileSystem {
	return &memFS{
		root: memFSNode{
			children: make(map[string]*memFSNode),
			mode:     0660 | os.ModeDir,
			modTime:  time.Now(),
		},
	}
}

// A memFS implements FileSystem, storing all metadata and actual file data
// in-memory. No limits on filesystem size are used, so it is not recommended
// this be used where the clients are untrusted.
//
// Concurrent access is permitted. The tree structure is protected by a mutex,
// and each node's contents and metadata are protected by a per-node mutex.
//
// TODO: Enforce file permissions.
type memFS struct {
	mu   sync.Mutex
	root memFSNode
}

// TODO: clean up and rationalize the walk/find code.

// walk walks the directory tree for the fullname, calling f at each step. If f
// returns an error, the walk will be aborted and return that same error.
//
// dir is the directory at that step, frag is the name fragment, and final is
// whether it is the final step. For example, walking "/foo/bar/x" will result
// in 3 calls to f:
//   - "/", "foo", false
//   - "/foo/", "bar", false
//   - "/foo/bar/", "x", true
// The frag argument will be empty only if dir is the root node and the walk
// ends at that root node.
func (fs *memFS) walk(op, fullname string, f func(dir *memFSNode, frag string, final bool) error) error {
	original := fullname
	fullname = slashClean(fullname)

	// Strip any leading "/"s to make fullname a relative path, as the walk
	// starts at fs.root.
	if fullname[0] == '/' {
		fullname = fullname[1:]
	}
	dir := &fs.root

	for {
		frag, remaining := fullname, ""
		i := strings.IndexRune(fullname, '/')
		final := i < 0
		if !final {
			frag, remaining = fullname[:i], fullname[i+1:]
		}
		if frag == "" && dir != &fs.root {
			panic("webdav: empty path fragment for a clean path")
		}
		if err := f(dir, frag, final); err != nil {
			return &os.PathError{
				Op:   op,
				Path: original,
				Err:  err,
			}
		}
		if final {
			break
		}
		child := dir.children[frag]
		if child == nil {
			return &os.PathError{
				Op:   op,
				Path: original,
				Err:  os.ErrNotExist,
			}
		}
		if !child.IsDir() {
			return &os.PathError{
				Op:   op,
				Path: original,
				Err:  os.ErrInvalid,
			}
		}
		dir, fullname = child, remaining
	}
	return nil
}

// find returns the parent of the named node and the relative name fragment
// from the parent to the child. For example, if finding "/foo/bar/baz" then
// parent will be the node for "/foo/bar" and frag will be "baz".
//
// If the fullname names the root node, then parent, frag and err will be zero.
//
// find returns an error if the parent does not already exist or the parent
// isn't a directory, but it will not return an error per se if the child does
// not already exist. The error returned is either nil or an *os.PathError
// whose Op is op.
func (fs *memFS) find(op, fullname string) (parent *memFSNode, frag string, err error) {
	err = fs.walk(op, fullname, func(parent0 *memFSNode, frag0 string, final bool) error {
		if !final {
			return nil
		}
		if frag0 != "" {
			parent, frag = parent0, frag0
		}
		return nil
	})
	return parent, frag, err
}

func (fs *memFS) Mkdir(name string, perm os.FileMode) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	dir, frag, err := fs.find("mkdir", name)
	if err != nil {
		return err
	}
	if dir == nil {
		// We can't create the root.
		return os.ErrInvalid
	}
	if _, ok := dir.children[frag]; ok {
		return os.ErrExist
	}
	dir.children[frag] = &memFSNode{
		name:     frag,
		children: make(map[string]*memFSNode),
		mode:     perm.Perm() | os.ModeDir,
		modTime:  time.Now(),
	}
	return nil
}

func (fs *memFS) OpenFile(name string, flag int, perm os.FileMode) (File, error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	dir, frag, err := fs.find("open", name)
	if err != nil {
		return nil, err
	}
	var n *memFSNode
	if dir == nil {
		// We're opening the root.
		if flag&(os.O_WRONLY|os.O_RDWR) != 0 {
			return nil, os.ErrPermission
		}
		n = &fs.root

	} else {
		n = dir.children[frag]
		if flag&(os.O_SYNC|os.O_APPEND) != 0 {
			// memFile doesn't support these flags yet.
			return nil, os.ErrInvalid
		}
		if flag&os.O_CREATE != 0 {
			if flag&os.O_EXCL != 0 && n != nil {
				return nil, os.ErrExist
			}
			if n == nil {
				n = &memFSNode{
					name: frag,
					mode: perm.Perm(),
				}
				dir.children[frag] = n
			}
		}
		if n == nil {
			return nil, os.ErrNotExist
		}
		if flag&(os.O_WRONLY|os.O_RDWR) != 0 && flag&os.O_TRUNC != 0 {
			n.mu.Lock()
			n.data = nil
			n.mu.Unlock()
		}
	}

	children := make([]os.FileInfo, 0, len(n.children))
	for _, c := range n.children {
		children = append(children, c)
	}
	return &memFile{
		n:                n,
		childrenSnapshot: children,
	}, nil
}

func (fs *memFS) RemoveAll(name string) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	dir, frag, err := fs.find("remove", name)
	if err != nil {
		return err
	}
	if dir == nil {
		// We can't remove the root.
		return os.ErrInvalid
	}
	delete(dir.children, frag)
	return nil
}

func (fs *memFS) Rename(oldName, newName string) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	oldName = slashClean(oldName)
	newName = slashClean(newName)
	if oldName == newName {
		return nil
	}
	if strings.HasPrefix(newName, oldName+"/") {
		// We can't rename oldName to be a sub-directory of itself.
		return os.ErrInvalid
	}

	oDir, oFrag, err := fs.find("rename", oldName)
	if err != nil {
		return err
	}
	if oDir == nil {
		// We can't rename from the root.
		return os.ErrInvalid
	}

	nDir, nFrag, err := fs.find("rename", newName)
	if err != nil {
		return err
	}
	if nDir == nil {
		// We can't rename to the root.
		return os.ErrInvalid
	}

	oNode, ok := oDir.children[oFrag]
	if !ok {
		return os.ErrNotExist
	}
	if oNode.IsDir() {
		if nNode, ok := nDir.children[nFrag]; ok {
			nNode.mu.Lock()
			isDir := nNode.mode.IsDir()
			nNode.mu.Unlock()
			if !isDir {
				return errNotADirectory
			}
			if len(nNode.children) != 0 {
				return errDirectoryNotEmpty
			}
		}
	}
	delete(oDir.children, oFrag)
	nDir.children[nFrag] = oNode
	return nil
}

func (fs *memFS) Stat(name string) (os.FileInfo, error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	dir, frag, err := fs.find("stat", name)
	if err != nil {
		return nil, err
	}
	if dir == nil {
		// We're stat'ting the root.
		return &fs.root, nil
	}
	if n, ok := dir.children[frag]; ok {
		return n, nil
	}
	return nil, os.ErrNotExist
}

// A memFSNode represents a single entry in the in-memory filesystem and also
// implements os.FileInfo.
type memFSNode struct {
	name string

	// children is protected by memFS.mu.
	children map[string]*memFSNode

	mu      sync.Mutex
	modTime time.Time
	mode    os.FileMode
	data    []byte
}

func (n *memFSNode) Name() string {
	return n.name
}

func (n *memFSNode) Size() int64 {
	n.mu.Lock()
	defer n.mu.Unlock()
	return int64(len(n.data))
}

func (n *memFSNode) Mode() os.FileMode {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.mode
}

func (n *memFSNode) ModTime() time.Time {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.modTime
}

func (n *memFSNode) IsDir() bool {
	return n.Mode().IsDir()
}

func (n *memFSNode) Sys() interface{} {
	return nil
}

// A memFile is a File implementation for a memFSNode. It is a per-file (not
// per-node) read/write position, and if the node is a directory, a snapshot of
// that node's children.
type memFile struct {
	n *memFSNode
	// childrenSnapshot is a snapshot of n.children.
	childrenSnapshot []os.FileInfo
	// pos is protected by n.mu.
	pos int
}

func (f *memFile) Close() error {
	return nil
}

func (f *memFile) Read(p []byte) (int, error) {
	f.n.mu.Lock()
	defer f.n.mu.Unlock()
	if f.n.mode.IsDir() {
		return 0, os.ErrInvalid
	}
	if f.pos >= len(f.n.data) {
		return 0, io.EOF
	}
	n := copy(p, f.n.data[f.pos:])
	f.pos += n
	return n, nil
}

func (f *memFile) Readdir(count int) ([]os.FileInfo, error) {
	f.n.mu.Lock()
	defer f.n.mu.Unlock()
	if !f.n.mode.IsDir() {
		return nil, os.ErrInvalid
	}
	old := f.pos
	if old >= len(f.childrenSnapshot) {
		// The os.File Readdir docs say that at the end of a directory,
		// the error is io.EOF if count > 0 and nil if count <= 0.
		if count > 0 {
			return nil, io.EOF
		}
		return nil, nil
	}
	if count > 0 {
		f.pos += count
		if f.pos > len(f.childrenSnapshot) {
			f.pos = len(f.childrenSnapshot)
		}
	} else {
		f.pos = len(f.childrenSnapshot)
		old = 0
	}
	return f.childrenSnapshot[old:f.pos], nil
}

func (f *memFile) Seek(offset int64, whence int) (int64, error) {
	f.n.mu.Lock()
	defer f.n.mu.Unlock()
	npos := f.pos
	// TODO: How to handle offsets greater than the size of system int?
	switch whence {
	case os.SEEK_SET:
		npos = int(offset)
	case os.SEEK_CUR:
		npos += int(offset)
	case os.SEEK_END:
		npos = len(f.n.data) + int(offset)
	default:
		npos = -1
	}
	if npos < 0 {
		return 0, os.ErrInvalid
	}
	f.pos = npos
	return int64(f.pos), nil
}

func (f *memFile) Stat() (os.FileInfo, error) {
	return f.n, nil
}

func (f *memFile) Write(p []byte) (int, error) {
	lenp := len(p)
	f.n.mu.Lock()
	defer f.n.mu.Unlock()

	if f.n.mode.IsDir() {
		return 0, os.ErrInvalid
	}
	if f.pos < len(f.n.data) {
		n := copy(f.n.data[f.pos:], p)
		f.pos += n
		p = p[n:]
	} else if f.pos > len(f.n.data) {
		// Write permits the creation of holes, if we've seek'ed past the
		// existing end of file.
		if f.pos <= cap(f.n.data) {
			oldLen := len(f.n.data)
			f.n.data = f.n.data[:f.pos]
			hole := f.n.data[oldLen:]
			for i := range hole {
				hole[i] = 0
			}
		} else {
			d := make([]byte, f.pos, f.pos+len(p))
			copy(d, f.n.data)
			f.n.data = d
		}
	}

	if len(p) > 0 {
		// We should only get here if f.pos == len(f.n.data).
		f.n.data = append(f.n.data, p...)
		f.pos = len(f.n.data)
	}
	f.n.modTime = time.Now()
	return lenp, nil
}
