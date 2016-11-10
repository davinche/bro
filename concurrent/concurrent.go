package concurrent

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// CWalker is a concurrent filepath walker
type CWalker struct {
	pool chan struct{}
	wg   sync.WaitGroup
}

// NewCWalker creates a new concurrent walker given the max number of goroutines
func NewCWalker(n int) *CWalker {
	if n < 1 {
		n = 1
	}
	p := make(chan struct{}, n)
	for i := 0; i < n; i++ {
		p <- struct{}{}
	}

	return &CWalker{pool: p, wg: sync.WaitGroup{}}
}

func (w *CWalker) walk(root string, m *sync.Map) {
	<-w.pool
	defer w.wg.Done()
	filepath.Walk(root, func(path string, f os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if f.IsDir() && path != root {
			w.wg.Add(1)
			go w.walk(path, m)
			return filepath.SkipDir
		}

		if !f.IsDir() {
			m.Store(path, path)
		}
		return nil
	})
	w.pool <- struct{}{}
}

// WalkAndCollect walks a directory and collects all of the files and stores
// it into the specified sync.Map
func (w *CWalker) WalkAndCollect(root string, m *sync.Map) {
	w.wg.Add(1)
	go w.walk(root, m)
	w.wg.Wait()
}

// CCopier is a concurrent file copier
type CCopier struct {
	n    int
	wg   sync.WaitGroup
	feed chan [2]string
}

// Start begins the concurrent by creating the goroutines
func (c *CCopier) Start() {
	c.wg.Add(c.n)
	for i := 0; i < c.n; i++ {
		go c.runCopier()
	}
}

// Copy feeds the source and destinations into the copier
func (c *CCopier) Copy(src, dst string) {
	c.feed <- [2]string{src, dst}
}

func (c *CCopier) runCopier() {
	for {
		pair, more := <-c.feed
		if !more {
			c.wg.Done()
			return
		}
		src, err := os.Open(pair[0])
		if err != nil {
			fmt.Printf("err in opening: %v\n", err)
			continue
		}

		dirPath := filepath.Dir(pair[1])
		_, err = os.Stat(dirPath)
		if err != nil {
			if os.IsNotExist(err) {
				err = os.MkdirAll(dirPath, os.ModePerm)
				if err != nil {
					fmt.Printf("err creating dir: %s", dirPath)
					continue
				}
			} else {
				fmt.Println("stat err")
				continue
			}
		}

		dst, err := os.Create(pair[1])
		if err != nil {
			fmt.Printf("error creating file: %s", pair[1])
			continue
		}
		io.Copy(dst, src)
		dst.Close()
		src.Close()
	}
}

// Wait prevents execution until all files have been copied
func (c *CCopier) Wait() {
	close(c.feed)
	c.wg.Wait()
}

// NewCCopier creates a new concurrent file copier
func NewCCopier(n int) *CCopier {
	if n < 1 {
		n = 1
	}
	feed := make(chan [2]string)
	return &CCopier{n: n, feed: feed, wg: sync.WaitGroup{}}
}
