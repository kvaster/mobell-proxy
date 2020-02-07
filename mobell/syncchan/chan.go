package syncchan

import (
	"errors"
	"sync"
	"sync/atomic"
)

const (
	chanOpened = iota
	chanClosed
)

// Chan is a concurrent channel to avoid the panic when sending data to a closed channel.
// It is safe for multiple goroutines to call a Chan's methods concurrently.
type Chan struct {
	// status is used to mark the status of channel.
	// It is set to chanOpened after created, and set to chanClosed before close.
	status int32

	// ch is the real channel of Chan.
	ch chan interface{}

	// wg is used to count sending data.
	// Before data is sent, do wg.Add(1).
	// After data is sent, do wg.Done().
	// Before close channel, do wg.Wait() to make sure all sending data are sent to channel.
	wg sync.WaitGroup
}

// Create a new Chan.
func MakeChan(size int) *Chan {
	if size < 0 {
		return nil
	}
	sc := &Chan{
		status: chanOpened,
		ch:     make(chan interface{}, size),
	}
	return sc
}

// Get the read-only channel
func (c *Chan) Chan() <-chan interface{} {
	return c.ch
}

// Check if the channel is closed
func (c *Chan) IsClosed() bool {
	return atomic.LoadInt32(&c.status) != chanOpened
}

// Push data into channel
func (c *Chan) Push(data interface{}) error {
	if c.IsClosed() {
		return errors.New("channel is closed")
	}
	c.wg.Add(1)
	c.ch <- data
	c.wg.Done()
	return nil
}

// Get data from channel
func (c *Chan) Get() interface{} {
	return <-c.ch
}

// Close channel
func (c *Chan) Close() {
	// mark channel as closed
	if !atomic.CompareAndSwapInt32(&c.status, chanOpened, chanClosed) {
		return
	}
	go closeChan(c)
}

func closeChan(c *Chan) {
	// avoid panic when send data
	c.wg.Wait()
	close(c.ch)
}
