package circular_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/raczu/kube2kafka/pkg/circular"
)

var _ = Describe("Buffer", func() {
	var (
		buffer   *circular.RingBuffer[int]
		capacity int
	)

	When("creating a new ring buffer", func() {
		BeforeEach(func() {
			capacity = 1
			buffer = circular.NewRingBuffer[int](capacity)
		})

		It("should create a new empty ring buffer", func() {
			Expect(buffer).ToNot(BeNil())
			Expect(buffer.Size()).To(Equal(0))
		})
	})

	When("performing read and write operations", func() {
		var written int

		Context("but the buffer is empty", func() {
			BeforeEach(func() {
				capacity = 10
				buffer = circular.NewRingBuffer[int](capacity)
			})

			It("should return false when reading from the buffer", func() {
				_, ok := buffer.Read()
				Expect(ok).To(BeFalse())
			})

			It("should write values to the buffer and increment its size", func() {
				written = 5
				for i := 0; i < written; i++ {
					buffer.Write(i)
					Expect(buffer.Size()).To(Equal(i + 1))
				}
			})
		})

		Context("and the buffer has some data", func() {
			BeforeEach(func() {
				capacity = 10
				written = 5
				buffer = circular.NewRingBuffer[int](capacity)
				for i := 0; i < written; i++ {
					buffer.Write(i)
				}
			})

			It("should return true when reading from the buffer", func() {
				_, ok := buffer.Read()
				Expect(ok).To(BeTrue())
			})

			It("should read the values in the correct order", func() {
				for i := 0; i < written; i++ {
					value, ok := buffer.Read()
					Expect(ok).To(BeTrue())
					Expect(value).To(Equal(i))
				}
				Expect(buffer.Size()).To(Equal(0))
			})

			It("should correctly update size after each read", func() {
				for i := 0; i < written; i++ {
					buffer.Read()
					Expect(buffer.Size()).To(Equal(written - i - 1))
				}
			})
		})

		Context("and the buffer is full", func() {
			BeforeEach(func() {
				capacity = 10
				written = capacity
				buffer = circular.NewRingBuffer[int](capacity)
				for i := 0; i < written; i++ {
					buffer.Write(i)
				}
			})

			It("should overwrite the oldest data and advance tail", func() {
				buffer.Write(3)
				value, ok := buffer.Read()
				Expect(ok).To(BeTrue())
				Expect(value).To(Equal(1))
			})

			It("should return false when all values are read", func() {
				for i := 0; i < written; i++ {
					_, ok := buffer.Read()
					Expect(ok).To(BeTrue())
				}
				_, ok := buffer.Read()
				Expect(ok).To(BeFalse())
			})
		})
	})
})
