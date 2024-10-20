package circular

import (
	"github.com/raczu/kube2kafka/pkg/assert"
	"sync"
)

// RingBuffer is a thread-safe circular structure, designed to store most recent data.
// When the buffer is full, the oldest data is overwritten.
type RingBuffer[T any] struct {
	buffer   []T
	capacity int
	size     int
	head     int
	tail     int
	mu       sync.Mutex
}

// NewRingBuffer creates a new ring buffer with the fixed capacity.
func NewRingBuffer[T any](capacity int) *RingBuffer[T] {
	assert.Assert(capacity > 0, "buffer capacity must be greater than 0")
	return &RingBuffer[T]{
		buffer:   make([]T, capacity),
		capacity: capacity,
	}
}

// Write inserts a value to the buffer. When the buffer is full,
// the oldest data is overwritten.
func (rb *RingBuffer[T]) Write(value T) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if rb.size == rb.capacity {
		// Advance the tail to overwrite the oldest data.
		rb.tail = (rb.tail + 1) % rb.capacity
	}

	rb.buffer[rb.head] = value
	rb.head = (rb.head + 1) % rb.capacity
	rb.size++
}

// Read returns the next value from buffer and true if the value
// was read successfully. When the buffer is empty, it returns false.
func (rb *RingBuffer[T]) Read() (T, bool) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if rb.size == 0 {
		var zero T
		return zero, false
	}

	value := rb.buffer[rb.tail]
	rb.tail = (rb.tail + 1) % rb.capacity
	rb.size--

	return value, true
}

// Size returns the number of elements in the buffer.
func (rb *RingBuffer[T]) Size() int {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return rb.size
}
